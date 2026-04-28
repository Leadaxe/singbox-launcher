package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// parserTimestampRegex — паттерн `"last_updated": "<RFC3339>"` для замены
// в golden-тестах (см. normalizeParserTimestamp).
var parserTimestampRegex = regexp.MustCompile(`"last_updated":\s*"[^"]*"`)

// nodeCommentRegex — `// <flag/tag>` строка-комментарий перед entry.
// Соответствует выводу `core/config.GenerateNodeJSON`, который префиксит
// каждый node его Comment-полем. Cosmetic-only — стрипается из обеих
// сторон при byte-compare.
//
// Pattern: leading horizontal-whitespace (только tabs/spaces, не \n —
// сохраняем разделители строк), затем `// <text>` до конца строки
// (включая \n).
var nodeCommentRegex = regexp.MustCompile(`(?m)^[ \t]*//[^\n]*\n`)

// TestGoldenScenarios — strangler-fig регрессионная защита для BuildConfig.
//
// Контракт каждого сценария в `testdata/golden/<scenario>/`:
//   - template.json        — input wizard_template
//   - state.json           — input state (формат core/state.State)
//   - cache.json           — input outbounds cache (формат core/outboundscache.Snapshot)
//   - expected.config.json — output, с которым сравниваем побайтно
//   - notes.md (опц.)      — описание сценария
//
// При расхождении пишет `actual.config.json` рядом и фейлит тест,
// указывая первый расходящийся байт + контекстное окно.
//
// Папки без всех четырёх обязательных файлов **пропускаются** — это даёт
// возможность сначала закоммитить структуру и постепенно добавлять кейсы.
//
// **Префикс `real-` = work-in-progress.** Сценарии, начинающиеся с `real-`,
// захвачены с реальной установки и тестируют полный путь BuildTemplateConfig
// (initial build), который ещё не портирован в core/build (фаза 5.3 SPEC 045).
// По умолчанию они **пропускаются**, чтобы основной `go test ./core/build`
// был зелёный. Чтобы прогнать их — `GOLDEN_RUN_REAL=1 go test ./core/build`.
// Каждый зелёный real-сценарий = пройденный milestone порта.
//
// Префикс `marker-fill-` = синтетический под-компонентный тест
// (substitute vars + populate-existing-markers); проходит всегда.
func TestGoldenScenarios(t *testing.T) {
	root := "testdata/golden"
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}

	var ran, skipped int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(root, name)
			if !scenarioComplete(dir) {
				skipped++
				t.Skipf("scenario %s incomplete (need template.json / state.json / cache.json / expected.config.json)", name)
				return
			}
			if strings.HasPrefix(name, "real-") && os.Getenv("GOLDEN_RUN_REAL") != "1" {
				skipped++
				t.Skipf("real-* scenario skipped (set GOLDEN_RUN_REAL=1 to enable WIP regression tracking for SPEC 045 phase 5.3 port)")
				return
			}
			runGoldenScenario(t, dir)
			ran++
		})
	}

	if ran == 0 {
		t.Logf("no complete scenarios in %s (skipped=%d); add inputs+expected to enable", root, skipped)
	}
}

// scenarioComplete — все ли четыре обязательных файла на месте.
func scenarioComplete(dir string) bool {
	for _, f := range []string{"template.json", "state.json", "cache.json", "expected.config.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			return false
		}
	}
	return true
}

// runGoldenScenario — основная логика: загрузить три input'а, вызвать
// BuildConfig, сравнить с expected. На расхождении — записать
// actual.config.json и описать первый отличающийся байт.
//
// SPEC 045 phase 5.3: BuildContext конструируется из state + cache + parsed
// template, по той же схеме, как (в будущем) Configurator-presenter будет
// готовить контекст для save-pipeline'а.
func runGoldenScenario(t *testing.T, dir string) {
	t.Helper()

	tmplBytes, err := os.ReadFile(filepath.Join(dir, "template.json"))
	if err != nil {
		t.Fatalf("read template: %v", err)
	}

	stateBytes, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	s, err := state.Parse(stateBytes)
	if err != nil {
		t.Fatalf("parse state: %v", err)
	}

	cacheBytes, err := os.ReadFile(filepath.Join(dir, "cache.json"))
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	// Golden cache.json fixtures хранят legacy-формат outboundscache.Snapshot
	// (Outbounds + Endpoints полей). Парсим в minimal-shape — остальные поля
	// (Version, StateID, UpdatedAt, ...) больше не нужны после SPEC 052 phase 8.
	var cacheRaw struct {
		Outbounds []json.RawMessage `json:"outbounds"`
		Endpoints []json.RawMessage `json:"endpoints"`
	}
	if len(cacheBytes) > 0 {
		if err := json.Unmarshal(cacheBytes, &cacheRaw); err != nil {
			t.Fatalf("parse cache: %v", err)
		}
	}
	cache := ParsedCache{
		Outbounds: cacheRaw.Outbounds,
		Endpoints: cacheRaw.Endpoints,
	}

	expected, err := os.ReadFile(filepath.Join(dir, "expected.config.json"))
	if err != nil {
		t.Fatalf("read expected: %v", err)
	}

	// Парсим шаблон в TemplateData (вынимаем parser_config + остальные поля).
	td, err := parseGoldenTemplate(tmplBytes)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}

	// Стартовый набор vars из state.Vars.
	vars := stateVarsToMap(s)

	dnsCfg := dnsConfigFromState(s)
	routeCfg := routeConfigFromState(s)

	ctx := BuildContext{
		Template:         td,
		Vars:             vars,
		Cache:            &cache,
		ForPreview:       false,
		DNS:              dnsCfg,
		Route:            routeCfg,
	}

	res, err := BuildConfig(ctx)
	if err != nil {
		t.Fatalf("BuildConfig returned error: %v", err)
	}

	// Сравнение с нормализованным timestamp в `parser.last_updated`:
	// `NormalizeParserConfigText` всегда обновляет timestamp на текущий
	// момент, поэтому байт-в-байт парность невозможна. Нормализуем оба
	// дампа перед сравнением.
	actual := normalizeParserTimestamp(res.ConfigJSON)
	expectedNorm := normalizeParserTimestamp(expected)

	if bytes.Equal(actual, expectedNorm) {
		return
	}

	// Расхождение: пишем actual для diff'а в IDE, фейлим с контекстом.
	actualPath := filepath.Join(dir, "actual.config.json")
	if writeErr := os.WriteFile(actualPath, res.ConfigJSON, 0o644); writeErr != nil {
		t.Logf("warning: failed to write %s: %v", actualPath, writeErr)
	}

	idx := firstDiffByte(actual, expectedNorm)
	t.Errorf("golden mismatch at byte %d\n%s\n\nexpected (%d bytes), actual (%d bytes); see %s",
		idx, contextDiff(actual, expectedNorm, idx, 80), len(expectedNorm), len(actual), actualPath)
}

// normalizeParserTimestamp заменяет все `"last_updated": "<RFC3339>"` на
// фиксированную строку, чтобы byte-сравнение игнорировало timestamp drift.
// Применяется только к `parser.last_updated` внутри @ParserConfig блока,
// который `NormalizeParserConfigText` всегда регенерирует.
//
// Также стрипает cosmetic `// <tag>`-комментарии перед outbound/endpoint
// записями: legacy `core/config.GenerateNodeJSON` префиксит ноды с
// `node.Comment` (флаги стран, имена), но это purely UI-affordance —
// семантически идентичная JSON-структура без них.
func normalizeParserTimestamp(raw []byte) []byte {
	out := parserTimestampRegex.ReplaceAll(raw, []byte(`"last_updated": "<NORMALIZED>"`))
	out = nodeCommentRegex.ReplaceAll(out, []byte(""))
	return out
}

// parseGoldenTemplate — порт `template.LoadTemplateData` без файлового I/O.
// Оперирует на сырых байтах wizard_template.json, разворачивает `config`
// wrapper, оставляет `vars`/`params` для GetEffectiveConfig.
func parseGoldenTemplate(raw []byte) (*template.TemplateData, error) {
	var root struct {
		ParserConfig json.RawMessage      `json:"parser_config"`
		Config       json.RawMessage      `json:"config"`
		Params       []template.TemplateParam `json:"params"`
		Vars         []template.TemplateVar   `json:"vars"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	td := &template.TemplateData{
		RawConfig:   root.Config,
		RawTemplate: raw,
		Params:      root.Params,
		Vars:        root.Vars,
	}
	if len(root.ParserConfig) > 0 {
		td.ParserConfig = `{"ParserConfig":` + string(root.ParserConfig) + `}`
	}

	// Парсим Config в map с порядком, через GetEffectiveConfig (применяет
	// params для текущего GOOS + substitute vars defaults). Это даёт нам
	// финальные секции, как их видит wizard на старте без user vars.
	if len(root.Config) > 0 && (len(root.Params) > 0 || len(root.Vars) > 0) {
		applied, order, err := template.GetEffectiveConfig(
			root.Config,
			root.Params,
			runtime.GOOS,
			root.Vars,
			nil, // state vars empty — defaults only
			raw,
		)
		if err == nil {
			td.Config = applied
			td.ConfigOrder = order
		}
	}
	if td.Config == nil {
		// Fallback: парсим Config в map без params-применения.
		var simple map[string]json.RawMessage
		if err := json.Unmarshal(root.Config, &simple); err == nil {
			td.Config = simple
			for _, k := range orderedConfigKeys(root.Config) {
				td.ConfigOrder = append(td.ConfigOrder, k)
			}
		}
	}
	return td, nil
}

// orderedConfigKeys возвращает top-level ключи в порядке появления в raw JSON.
// Использует json.Decoder для сохранения исходного порядка
// (`json.Unmarshal` в map даёт случайный порядок).
func orderedConfigKeys(raw []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var keys []string
	if _, err := dec.Token(); err != nil {
		return nil
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return keys
		}
		key, ok := tok.(string)
		if !ok {
			return keys
		}
		keys = append(keys, key)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return keys
		}
	}
	return keys
}

// stateVarsToMap превращает []state.SettingVar в map[string]string.
func stateVarsToMap(s *state.State) map[string]string {
	out := make(map[string]string, len(s.Vars))
	for _, v := range s.Vars {
		out[v.Name] = v.Value
	}
	return out
}

// dnsConfigFromState — извлекает DNS-related поля из state в DNSConfig.
//
// state.DNSOptions хранит servers / rules; final / strategy / independent_cache
// исторически живут как `dns_*` записи в state.Vars (ApplyDNSVarsFromSettingsToModel
// в wizard читает их при load). Соответственно — берём оба источника.
//
// `RulesText` reconstructируется как JSON-объект {"rules": [...]} для парсера
// в MergeDNSSection.
func dnsConfigFromState(s *state.State) DNSConfig {
	if s == nil {
		return DNSConfig{}
	}
	cfg := DNSConfig{}
	if s.DNSOptions != nil {
		d := s.DNSOptions
		cfg.Servers = d.Servers
		// state.DNSOptions.Final/Strategy/IndependentCache — могут быть пустыми
		// (после миграции в state.Vars); если заполнены — используем как fallback.
		cfg.Final = d.Final
		cfg.Strategy = d.Strategy
		cfg.IndependentCache = d.IndependentCache
		if len(d.Rules) > 0 {
			raw, err := json.Marshal(map[string]interface{}{"rules": d.Rules})
			if err == nil {
				cfg.RulesText = string(raw)
			}
		}
	}
	// state.Vars override (canonical source после миграции SPEC 032).
	for _, v := range s.Vars {
		switch v.Name {
		case "dns_final":
			cfg.Final = v.Value
		case "dns_strategy":
			cfg.Strategy = v.Value
		case "dns_independent_cache":
			b := v.Value == "true"
			cfg.IndependentCache = &b
		}
	}
	return cfg
}

// routeConfigFromState — извлекает custom rules + final outbound из state
// в RouteConfig. Соответствует тому, что wizard передаёт в MergeRouteSection
// (model.CustomRules + model.SelectedFinalOutbound).
//
// Для теста: SelectedFinalOutbound берём из state.Vars["dns_final"]-style
// — но реальный wizard model.SelectedFinalOutbound заполняется из других
// источников. Для golden-парности оставим пустым (template fallback).
func routeConfigFromState(s *state.State) RouteConfig {
	if s == nil {
		return RouteConfig{}
	}
	rules := make([]RouteRule, 0, len(s.CustomRules))
	for _, cr := range s.CustomRules {
		// Аналог wizard `GetEffectiveOutbound`: SelectedOutbound либо
		// DefaultOutbound. У state.CustomRule нет explicit DefaultOutbound,
		// но HasOutbound + DefaultOutbound поля сохраняются.
		outbound := cr.SelectedOutbound
		if outbound == "" {
			outbound = cr.DefaultOutbound
		}
		rules = append(rules, RouteRule{
			Enabled:     cr.Enabled,
			Outbound:    outbound,
			PrimaryRule: cr.Rule,
			RuleSets:    cr.RuleSet,
		})
	}
	return RouteConfig{
		Rules: rules,
	}
}

// firstDiffByte возвращает индекс первого расходящегося байта в a и b.
// Если один короче и совпадает по префиксу — возвращает len короткого.
func firstDiffByte(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// contextDiff форматирует текстовое окно вокруг точки расхождения для логов.
func contextDiff(actual, expected []byte, at, window int) string {
	start := at - window
	if start < 0 {
		start = 0
	}
	endE := at + window
	if endE > len(expected) {
		endE = len(expected)
	}
	endA := at + window
	if endA > len(actual) {
		endA = len(actual)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  expected[%d..%d]: %q\n", start, endE, escapeForLog(expected[start:endE]))
	fmt.Fprintf(&b, "  actual  [%d..%d]: %q", start, endA, escapeForLog(actual[start:endA]))
	return b.String()
}

func escapeForLog(b []byte) string {
	var out strings.Builder
	for _, c := range b {
		switch {
		case c == '\n':
			out.WriteString("\\n")
		case c == '\t':
			out.WriteString("\\t")
		case c == '\r':
			out.WriteString("\\r")
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&out, "\\x%02x", c)
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}
