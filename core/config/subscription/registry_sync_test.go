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
	"sort"
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
