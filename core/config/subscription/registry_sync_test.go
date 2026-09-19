package subscription

// Sync-тесты реестра контракта (SPEC 103, фаза 2).
//
// Реестр contract/registry/*.json объявлен нормативным источником словарей
// (D-020), но нормативность без проверки — просто текст: словарь в коде
// уезжает, реестр остаётся, и оба приложения расходятся молча. Так gecko
// добавили в парсер, а в allowlists.json он не попал.
//
// Тесты сверяют РЕАЛЬНЫЕ структуры Go с реестром. Расхождение — ошибка, и
// чинить её нужно с обеих сторон осознанно: либо код догоняет реестр, либо
// реестр фиксирует новое решение.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const registryRelPath = "../../../contract/registry"

// Запись реестра: values — нормативный список, note — обоснование.
type allowlistEntry struct {
	Values []string `json:"values"`
	Note   string   `json:"note"`
}

type allowlistsFile struct {
	V          int                       `json:"v"`
	Allowlists map[string]allowlistEntry `json:"allowlists"`
}

func loadAllowlists(t *testing.T) map[string]allowlistEntry {
	t.Helper()
	path := filepath.Join(registryRelPath, "allowlists.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("реестр не найден (%s) — контракт не синхронизирован", path)
	}
	var f allowlistsFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("разбор %s: %v", path, err)
	}
	return f.Allowlists
}

// diffSets возвращает, чего нет в реестре и чего нет в коде.
func diffSets(code, registry []string) (missingInRegistry, missingInCode []string) {
	inRegistry := make(map[string]bool, len(registry))
	for _, v := range registry {
		inRegistry[v] = true
	}
	inCode := make(map[string]bool, len(code))
	for _, v := range code {
		inCode[v] = true
	}
	for _, v := range code {
		if !inRegistry[v] {
			missingInRegistry = append(missingInRegistry, v)
		}
	}
	for _, v := range registry {
		if !inCode[v] {
			missingInCode = append(missingInCode, v)
		}
	}
	sort.Strings(missingInRegistry)
	sort.Strings(missingInCode)
	return missingInRegistry, missingInCode
}

func checkAllowlist(t *testing.T, name string, code []string, registry map[string]allowlistEntry) {
	t.Helper()
	entry, ok := registry[name]
	if !ok {
		t.Fatalf("в реестре нет списка %q", name)
	}
	missingInRegistry, missingInCode := diffSets(code, entry.Values)
	if len(missingInRegistry) > 0 {
		t.Errorf("%s: код принимает значения, которых нет в реестре: %v\n"+
			"  реестр нормативен (D-020): либо внести значения, либо убрать их из кода",
			name, missingInRegistry)
	}
	if len(missingInCode) > 0 {
		t.Errorf("%s: реестр объявляет значения, которых код не принимает: %v",
			name, missingInCode)
	}
}

// uTLS-отпечатки: чужое значение валит ВЕСЬ конфиг, поэтому словарь обязан
// совпадать буквально (SPEC 093).
func TestRegistrySyncUTLSFingerprints(t *testing.T) {
	registry := loadAllowlists(t)
	code := make([]string, 0, len(singboxUTLSFingerprints))
	for fp := range singboxUTLSFingerprints {
		code = append(code, fp)
	}
	checkAllowlist(t, "utls_fingerprints", code, registry)
}

// Словари hysteria2 obfs и TUIC congestion переехали в секции body реестра
// (SPEC 131 W2d): их сверяет TestRegistryBodyAllowlistsMatchEnums в пакете
// core/config, где видны и allowlists.json, и body.values. Здесь их копий в
// Go больше нет — сверять нечего.

// Значение вне словаря отпечатков обязано опознаваться как мусор: на нём
// держится перевод Xray-написаний (hellochrome_120 → chrome), и если он
// начнёт принимать что угодно, санитайзер получит «валидное» значение,
// которого ядро не знает.
func TestRegistryAllowlistsRejectOutsiders(t *testing.T) {
	if _, junk := normalizeUTLSFingerprintEx("garbage"); !junk {
		t.Error("uTLS принял отпечаток вне словаря")
	}
	if canon, junk := normalizeUTLSFingerprintEx("HelloChrome_120"); junk || canon != "chrome" {
		t.Errorf("Xray-написание HelloChrome_120 не развёрнуто: canon=%q junk=%v", canon, junk)
	}
}

// warningsFile — реестр кодов деградации.
type warningsFile struct {
	V        int `json:"v"`
	Warnings map[string]struct {
		Severity string `json:"severity"`
		Desc     string `json:"desc"`
		Go       string `json:"go"`
	} `json:"warnings"`
}

func loadWarningsRegistry(t *testing.T) warningsFile {
	t.Helper()
	path := filepath.Join(registryRelPath, "warnings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("реестр не найден (%s) — контракт не синхронизирован", path)
	}
	var f warningsFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("warnings.json: %v", err)
	}
	return f
}

// goWarningConstants — коды из parse_warnings.go, вычитанные из исходника.
//
// Именно из исходника, а не списком в тесте: список пришлось бы обновлять
// руками, и он разъехался бы с кодом ровно так же, как разъехался реестр.
func goWarningConstants(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile("parse_warnings.go")
	if err != nil {
		t.Fatalf("parse_warnings.go: %v", err)
	}
	re := regexp.MustCompile(`(Warn\w+)\s*=\s*"([^"]+)"`)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		out[m[1]] = m[2]
	}
	if len(out) == 0 {
		t.Fatal("в parse_warnings.go не найдено ни одной константы кода")
	}
	return out
}

// Каждый Go-код обязан быть объявлен в реестре: код, которого реестр не
// знает, — это деградация, о которой вторая сторона (LxBox) не в курсе, и
// сверить конверты становится нечем. Так `ws_early_data_converted` прожил
// весь цикл в Go, отсутствуя в нормативном словаре.
func TestRegistrySyncWarningCodesDeclared(t *testing.T) {
	reg := loadWarningsRegistry(t)
	for name, code := range goWarningConstants(t) {
		if _, ok := reg.Warnings[code]; !ok {
			t.Errorf("код %q (%s) есть в Go, но отсутствует в contract/registry/warnings.json", code, name)
		}
	}
}

// Код, который нигде не ставится на узел, обязан быть severity=error —
// то есть описывать ОТБРОШЕННЫЙ узел, для которого объекта ParsedNode не
// существует. Любой warning/info-код без AddWarning означает обещанную, но
// не выдаваемую диагностику: пользователь и LxBox о деградации не узнают.
//
// «Ставится» — это ДВА способа, а не один: строка `name` в Go-коде и правило
// в секции реестра. После SPEC 133 у схем на движке код объявлен прямо в
// секции (`on_when_false`, `on_implies_written`, `on_invalid`…), движок берёт
// его строкой и Go-имени не знает вовсе — считать такой код «непоставленным»
// значит требовать вторую, рукописную копию правила. Так этот тест и упал на
// xhttp_mode_forced_packet_up / xhttp_param_reset: рукописный транспортный
// вход сняли, а коды всё это время ставил реестр (корпус
// contract/corpus/uri/vless/xhttp_* зелёный на РЕАЛЬНОМ пути).
func TestRegistryWarningCodesAreActuallySet(t *testing.T) {
	reg := loadWarningsRegistry(t)
	consts := goWarningConstants(t)

	// Сторона реестра: коды, названные ЛЮБЫМ правилом секций. Те же помощники,
	// что у TestRegistryWarningCodesHaveAProducer, — один взгляд на «кто ставит».
	registrySets := map[string]bool{}
	for _, f := range registryRuleFiles(t) {
		for _, code := range registryCodesIn(t, f) {
			registrySets[code] = true
		}
	}

	// Где по коду ставятся коды: и прямым node.AddWarning, и через возврат
	// из построителей (санитайзер sing-box, AWG-поля) — их вызывающие
	// вешают код на узел.
	sources := []string{".", "../../../ui", "../../../core"}
	setNames := map[string]bool{}
	for _, dir := range sources {
		_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") {
				return nil
			}
			if strings.HasSuffix(p, "_test.go") {
				return nil
			}
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				return nil
			}
			text := string(data)
			for name := range consts {
				// Объявление в parse_warnings.go не считается использованием.
				if strings.HasSuffix(p, "parse_warnings.go") {
					continue
				}
				if strings.Contains(text, name) {
					setNames[name] = true
				}
			}
			return nil
		})
	}

	for name, code := range consts {
		if setNames[name] || registrySets[code] {
			continue
		}
		entry, ok := reg.Warnings[code]
		if !ok {
			continue // покрыто TestRegistrySyncWarningCodesDeclared
		}
		if entry.Severity != "error" {
			t.Errorf("код %q (%s, severity=%s) не ставится нигде: ни строкой в Go, "+
				"ни правилом секции реестра. Либо проставьте его на узле, либо "+
				"объявите правилом в contract/registry/**, либо зафиксируйте как "+
				"severity=error (узел отбрасывается, вешать код не на что)",
				code, name, entry.Severity)
		}
	}
}

// TestRegistryWarningCodesHaveAProducer — у каждого кода из warnings.json есть
// КТО-ТО, кто его ставит: либо константа Go (парсер видит сведения сам), либо
// правило в секции реестра (санитайзер ставит код по данным контракта).
//
// Зачем отдельно от предыдущего теста: тот идёт от кода к реестру и ловит
// «Go ставит код, которого контракт не знает». Этот идёт обратно и ловит
// «контракт обещает код, которого не существует» — состояние, в котором
// warnings.json прожил всю кампанию с ss_method_invalid и port_invalid
// (DRIFT §4: константы в Go были, но не выставлялись никогда, а реальным
// поведением был жёсткий дроп узла).
//
// После SPEC 131 W2d бо́льшую часть кодов ставит реестр, и грепать один
// parse_warnings.go стало недостаточно: тест, смотрящий только в Go, объявил
// бы дюжину живых правил мёртвыми.
func TestRegistryWarningCodesHaveAProducer(t *testing.T) {
	reg := loadWarningsRegistry(t)

	// Сторона Go: значения объявленных констант.
	producers := map[string]string{}
	for name, code := range goWarningConstants(t) {
		producers[code] = "Go: " + name
	}

	// Сторона реестра: код, названный ЛЮБЫМ правилом секций (on_invalid.code,
	// advisory.code, code у поля/связи, normalize_code). Читаем файлы как
	// сырой JSON и собираем все значения ключей, которыми реестр называет код,
	// — так тест не придётся править при каждом новом виде правила.
	for _, f := range registryRuleFiles(t) {
		for _, code := range registryCodesIn(t, f) {
			if _, has := producers[code]; !has {
				producers[code] = "реестр: " + filepath.Base(f)
			}
		}
	}

	// Ядро санитайзера ставит часть кодов само, без записи в секции: это
	// правила ФОРМЫ, общие для всех полей сразу (ключ вне схемы, значение не
	// того типа, конфликт, недостача). Искать их в реестре бессмысленно —
	// они и есть его исполнение.
	for _, code := range nodeflowBuiltinCodes {
		if _, has := producers[code]; !has {
			producers[code] = "nodeflow: встроенное правило формы"
		}
	}

	for code, entry := range reg.Warnings {
		if _, has := producers[code]; has {
			continue
		}
		// Проверяем только коды ПОЛЕЙ УЗЛА — область этой волны. Коды других
		// подсистем (цепочки, Направления, шаблон, источники) ставятся своими
		// местами, и требовать их здесь значило бы гонять чужой предмет через
		// тест парсера; у каждой из них свой сторож.
		if !nodeFieldCodePrefixes(code) {
			continue
		}
		t.Errorf("код %q (severity=%s) объявлен в warnings.json, но его никто не ставит: "+
			"ни константа Go, ни правило секции реестра", code, entry.Severity)
	}
}

// nodeflowBuiltinCodes — коды, которые санитайзер ставит сам (nodeflow),
// не по записи в секции: правила формы, общие для всех полей.
var nodeflowBuiltinCodes = []string{
	"unknown_key", "type_invalid", "field_missing", "field_conflict",
	"field_requires", "protocol_unsupported", "parse_error",
}

// nodeFieldCodePrefixes — относится ли код к ПОЛЯМ ТЕЛА УЗЛА, то есть к
// области, за которую отвечает реестр протоколов.
//
// Список по именам, а не по префиксам: коды называются по смыслу, и общего
// префикса у них нет. Код, попавший сюда, обязан иметь того, кто его ставит.
func nodeFieldCodePrefixes(code string) bool {
	switch code {
	case "reality_short_id_invalid", "reality_pbk_invalid", "reality_key_share_invalid",
		"reality_fp_not_chrome", "utls_fp_unknown", "packet_encoding_unknown",
		"flow_deprecated", "ss_method_invalid", "ss_method_legacy", "port_invalid",
		"obfs_unknown", "obfs_password_missing", "tuic_congestion_invalid",
		"tuic_udp_relay_mode_invalid", "anytls_min_idle_invalid",
		"tls_field_unsupported_naive", "tls_not_applicable_quic", "ech_ignored",
		"masque_vhttp_invalid":
		return true
	}
	return false
}

// registryRuleFiles — файлы реестра, в которых бывают правила полей.
func registryRuleFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(registryRelPath, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".json") {
			return nil
		}
		if strings.HasSuffix(p, "warnings.json") {
			return nil // словарь, а не правила
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		t.Fatalf("обход реестра: %v", err)
	}
	return out
}

// registryCodesIn собирает значения всех ключей "code"/"normalize_code" файла.
func registryCodesIn(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	var node interface{}
	if err := json.Unmarshal(data, &node); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	var out []string
	var walk func(interface{})
	walk = func(v interface{}) {
		switch t := v.(type) {
		case map[string]interface{}:
			for k, item := range t {
				if k == "code" || k == "normalize_code" {
					if s, ok := item.(string); ok && s != "" {
						out = append(out, s)
					}
					continue
				}
				// forbidden_codes — словарь «схема → код», а не одиночный
				// code: без этой ветки код, названный ТОЛЬКО там
				// (tls_not_applicable_quic), выглядел бы бесхозным.
				if k == "forbidden_codes" {
					if m, ok := item.(map[string]interface{}); ok {
						for _, v := range m {
							if s, ok := v.(string); ok && s != "" {
								out = append(out, s)
							}
						}
					}
					continue
				}
				walk(item)
			}
		case []interface{}:
			for _, item := range t {
				walk(item)
			}
		}
	}
	walk(node)
	return out
}
