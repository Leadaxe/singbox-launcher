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
	"regexp"
	"os"
	"path/filepath"
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

// hysteria2 obfs: ядро реализует salamander и gecko.
func TestRegistrySyncHysteria2Obfs(t *testing.T) {
	registry := loadAllowlists(t)
	var code []string
	for _, candidate := range []string{"salamander", "gecko"} {
		if isValidHysteria2ObfsType(candidate) {
			code = append(code, candidate)
		}
	}
	checkAllowlist(t, "hysteria2_obfs", code, registry)
}

// TUIC congestion control.
func TestRegistrySyncTuicCongestion(t *testing.T) {
	registry := loadAllowlists(t)
	var code []string
	for _, candidate := range []string{"cubic", "new_reno", "bbr"} {
		if isValidTuicCongestionControl(candidate) {
			code = append(code, candidate)
		}
	}
	checkAllowlist(t, "tuic_congestion", code, registry)
}

// Значения ВНЕ словаря обязаны отвергаться — иначе allowlist декоративен.
func TestRegistryAllowlistsRejectOutsiders(t *testing.T) {
	if isValidHysteria2ObfsType("nonsense") {
		t.Error("hysteria2 obfs принял значение вне словаря")
	}
	if isValidTuicCongestionControl("nonsense") {
		t.Error("TUIC congestion принял значение вне словаря")
	}
	if _, junk := normalizeUTLSFingerprintEx("garbage"); !junk {
		t.Error("uTLS принял отпечаток вне словаря")
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
func TestRegistryWarningCodesAreActuallySet(t *testing.T) {
	reg := loadWarningsRegistry(t)
	consts := goWarningConstants(t)

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
		if setNames[name] {
			continue
		}
		entry, ok := reg.Warnings[code]
		if !ok {
			continue // покрыто TestRegistrySyncWarningCodesDeclared
		}
		if entry.Severity != "error" {
			t.Errorf("код %q (%s, severity=%s) не ставится нигде в коде: "+
				"либо проставьте его на узле, либо зафиксируйте в реестре как "+
				"severity=error (узел отбрасывается, вешать код не на что)",
				code, name, entry.Severity)
		}
	}
}
