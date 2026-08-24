// l10n_check — CI-гейт натуральной локализации (SPEC 111, этап 4).
//
// AST-скан вызовов locale.T/Tf/TN/TfN/Plural/PluralN по всем пакетам
// проекта и сверка с каталогом bin/locale/ru.json:
//
//	missing        ключ из кода отсутствует в каталоге     warn, --strict → fail
//	orphan         запись каталога не используется в коде  warn, --strict → fail
//	orphan-special special["N"] не вызывается с формой N   warn, --strict → fail
//	usage-conflict ключ зовут и T-, и Plural-семейством    fail всегда
//	shape          T на plural-объекте; Plural без полного набора форм
//	               резолвера; использованная форма N отсутствует
//	                                                       fail всегда
//	arity          наборы плейсхолдеров расходятся         fail всегда
//
// Переходный режим: ключи, существующие во вшитом internal/locale/en.json
// (легаси-формат), исключаются из orphan-проверки, пока файл не удалён.
//
// Запуск из корня: go run ./tools/l10n/l10n_check [--strict]
package main

import (
	"encoding/json"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"singbox-launcher/internal/locale"
)

const (
	ruPath      = "bin/locale/ru.json"
	legacyPath  = "internal/locale/en.json" // переходный: снимется вместе с файлом
	helpersPath = "tools/l10n/l10n_helpers.json"
	displayName = "_display_name"
)

var skipDirs = map[string]bool{"tools": true, "dist": true, "temp": true, ".git": true}

// entry — минимальный разбор записи каталога (та же семантика, что
// locale.Entry; дублируется намеренно: чекер обязан читать файл как есть,
// без толерантных деградаций рантайма).
type entry struct {
	Value   json.RawMessage  `json:"value"`
	Special map[string]entry `json:"special"`
}

type catRecord struct {
	text    string
	forms   map[string]string
	special map[string]catRecord
	legacy  bool // голая строка — легаси-формат
}

func parseRecord(raw json.RawMessage) (catRecord, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return catRecord{text: s, legacy: true}, nil
	}
	var e entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return catRecord{}, err
	}
	rec := catRecord{}
	if err := json.Unmarshal(e.Value, &rec.text); err != nil {
		if err := json.Unmarshal(e.Value, &rec.forms); err != nil {
			return catRecord{}, fmt.Errorf("value is neither string nor form map")
		}
	}
	if len(e.Special) > 0 {
		rec.special = map[string]catRecord{}
		for k, sp := range e.Special {
			var srec catRecord
			if err := json.Unmarshal(sp.Value, &srec.text); err != nil {
				if err := json.Unmarshal(sp.Value, &srec.forms); err != nil {
					return catRecord{}, fmt.Errorf("special[%s]: bad value", k)
				}
			}
			rec.special[k] = srec
		}
	}
	return rec, nil
}

// placeholderRe — Go-верби формата, включая позиционные %[1]s; %% — литерал.
var placeholderRe = regexp.MustCompile(`%(\[\d+\])?[-+# 0]*[*]?[0-9]*(\.[0-9*]*)?[vTtbcdoOqxXUeEfFgGsp]|%%`)

func placeholders(s string) []string {
	var out []string
	for _, m := range placeholderRe.FindAllString(s, -1) {
		if m != "%%" {
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

func sameArity(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type finding struct {
	kind string // missing | orphan | orphan-special | usage-conflict | shape | arity
	msg  string
}

func main() {
	strict := len(os.Args) > 1 && os.Args[1] == "--strict"

	// --- каталог ---
	ruRaw, err := os.ReadFile(ruPath)
	if err != nil {
		fatal("read %s: %v", ruPath, err)
	}
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(ruRaw, &rawMap); err != nil {
		fatal("parse %s: %v", ruPath, err)
	}
	catalog := map[string]catRecord{}
	var hard []finding
	for k, raw := range rawMap {
		rec, err := parseRecord(raw)
		if err != nil {
			hard = append(hard, finding{"shape", fmt.Sprintf("%q: %v", k, err)})
			continue
		}
		catalog[k] = rec
	}

	// Переходный режим: легаси-ключи en.json вне orphan-проверки.
	legacyKeys := map[string]bool{}
	if data, err := os.ReadFile(legacyPath); err == nil {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(data, &m); err == nil {
			for k := range m {
				legacyKeys[k] = true
			}
		}
	}

	// --- реестр хелперов ---
	helpers := Helpers{}
	if data, err := os.ReadFile(helpersPath); err == nil {
		if err := json.Unmarshal(data, &helpers); err != nil {
			fatal("parse %s: %v", helpersPath, err)
		}
	}

	// --- скан исходников ---
	res := &ScanResult{Used: map[string]*Usage{}}
	fset := token.NewFileSet()
	err = filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") && path != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return ScanSource(fset, path, src, helpers, res)
	})
	if err != nil {
		fatal("scan: %v", err)
	}

	// --- проверки ---
	var warns []finding
	resolver := locale.RuPluralResolver{}

	for key, u := range res.Used {
		rec, ok := catalog[key]
		if !ok {
			warns = append(warns, finding{"missing", fmt.Sprintf("%q (%s)", key, first(u.Sites))})
			continue
		}
		if u.TFamily && u.PluralFamily {
			hard = append(hard, finding{"usage-conflict", fmt.Sprintf("%q used by both T and Plural (%s)", key, first(u.Sites))})
		}
		if u.TFamily && len(rec.forms) > 0 {
			hard = append(hard, finding{"shape", fmt.Sprintf("%q: T-call on plural entry (%s)", key, first(u.Sites))})
		}
		if u.PluralFamily {
			checkPluralForms(key, rec, resolver.Forms(), &hard)
		}
		for _, f := range u.Forms {
			if _, ok := rec.special[strconv.Itoa(f)]; !ok {
				hard = append(hard, finding{"shape", fmt.Sprintf("%q: form %d used but special[%d] missing (%s)", key, f, f, first(u.Sites))})
			}
		}
		checkArity(key, rec, &hard)
	}

	usedForms := map[string]map[string]bool{}
	for key, u := range res.Used {
		m := map[string]bool{}
		for _, f := range u.Forms {
			m[strconv.Itoa(f)] = true
		}
		usedForms[key] = m
	}
	for key, rec := range catalog {
		if key == displayName || rec.legacy || legacyKeys[key] {
			continue
		}
		u, used := res.Used[key]
		if !used {
			warns = append(warns, finding{"orphan", fmt.Sprintf("%q", key)})
			continue
		}
		_ = u
		for n := range rec.special {
			if !usedForms[key][n] {
				warns = append(warns, finding{"orphan-special", fmt.Sprintf("%q special[%s]", key, n)})
			}
		}
	}

	// --- отчёт ---
	sortF := func(fs []finding) {
		sort.Slice(fs, func(i, j int) bool {
			return fs[i].kind+fs[i].msg < fs[j].kind+fs[j].msg
		})
	}
	sortF(hard)
	sortF(warns)
	for _, f := range hard {
		fmt.Printf("FAIL %-14s %s\n", f.kind, f.msg)
	}
	for _, f := range warns {
		fmt.Printf("warn %-14s %s\n", f.kind, f.msg)
	}
	fmt.Printf("[l10n_check] keys used: %d, catalog: %d (legacy excluded: %d), missing+orphan warns: %d, hard fails: %d, dynamic: %d\n",
		len(res.Used), len(catalog), len(legacyKeys), len(warns), len(hard), len(res.Dynamic))

	if len(hard) > 0 || (strict && len(warns) > 0) {
		os.Exit(1)
	}
}

func checkPluralForms(key string, rec catRecord, required []string, hard *[]finding) {
	check := func(label string, forms map[string]string, text string) {
		if len(forms) == 0 {
			// одна строка на plural-ключ — допустимая деградация (рантайм
			// её принимает), но под чекером это shape: перевод неполон
			*hard = append(*hard, finding{"shape", fmt.Sprintf("%q%s: plural key translated with a single string", key, label)})
			return
		}
		for _, f := range required {
			if forms[f] == "" {
				*hard = append(*hard, finding{"shape", fmt.Sprintf("%q%s: plural form %q missing", key, label, f)})
			}
		}
	}
	check("", rec.forms, rec.text)
	for n, sp := range rec.special {
		check(" special["+n+"]", sp.forms, sp.text)
	}
}

func checkArity(key string, rec catRecord, hard *[]finding) {
	ref := placeholders(key)
	verify := func(label, tmpl string) {
		if tmpl == "" {
			return
		}
		if got := placeholders(tmpl); !sameArity(ref, got) {
			*hard = append(*hard, finding{"arity", fmt.Sprintf("%q%s: key has %v, translation has %v", key, label, ref, got)})
		}
	}
	verify("", rec.text)
	for f, t := range rec.forms {
		verify(" form "+f, t)
	}
	for n, sp := range rec.special {
		verify(" special["+n+"]", sp.text)
		for f, t := range sp.forms {
			verify(" special["+n+"] form "+f, t)
		}
	}
}

func first(s []string) string {
	if len(s) == 0 {
		return "?"
	}
	return s[0]
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "l10n_check: "+format+"\n", args...)
	os.Exit(2)
}
