package template

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Паритет двух обходчиков (SPEC 107 §11).
//
// В Go их два: боевой legacy (SubstituteVarsInJSON, им собирается config.json)
// и канонический (SubstituteVarsInJSONCanon), по которому проверяется корпус
// контракта — общий с LxBox. Пока они не унифицированы (это отдельная задача),
// контракт зелёный, а продакшен идёт другим кодом: расхождение существует и
// обязано быть ИЗМЕРЕННЫМ, а не подразумеваемым.
//
// Тест фиксирует известный список расхождений. Он падает в обе стороны:
// появилось новое — движки разъехались дальше молча; исчезло старое — кто-то
// починил, и список пора сократить (а не оставлять протухшим).
//
// Список — только про ЗНАЧЕНИЕ в конфиге. Разница в warning-кодах сюда не
// входит: legacy их не собирает по построению (у него нет канала), и это не
// расхождение поведения, а отсутствие диагностики.
var knownWalkerDivergences = map[string]string{
	"unresolved/null_value_drops_key": "legacy подставляет \"\" вместо удаления ключа (Dropped-каскад §5.1)",

	"unresolved/null_value_drops_array_element": "legacy оставляет \"\" элементом массива вместо его удаления",

	"unresolved/dropped_cascades_bottom_up": "legacy не поднимает Dropped вверх по дереву",

	"unresolved/undeclared_name_stays_placeholder": "legacy подставляет \"\" вместо сохранения '@name'; " +
		"в проде недостижимо — ValidateWizardTemplate отвергает необъявленную @-ссылку в config на загрузке",

	"grammar/if_without_and_or_is_false": "legacy трактует #if без and/or как TRUE (канон и контракт — FALSE); " +
		"в проде недостижимо — ValidateWizardTemplate обходит секцию config тем же " +
		"walkValidateIf и отвергает такой шаблон на загрузке (аудит 2026-08-24)",
}

// walkerParityCase — одна фикстура корпуса, прогнанная через боевой обходчик.
func loadParityCase(t *testing.T, base string) (cfg json.RawMessage, want json.RawMessage, ok bool) {
	t.Helper()
	tplRaw, err := os.ReadFile(base + ".template.json")
	if err != nil {
		return nil, nil, false
	}
	var tpl struct {
		Vars    []TemplateVar   `json:"vars"`
		Config  json.RawMessage `json:"config"`
		Changed string          `json:"_changed"`
	}
	if json.Unmarshal(tplRaw, &tpl) != nil {
		return nil, nil, false
	}
	expRaw, err := os.ReadFile(base + ".expected.json")
	if err != nil {
		return nil, nil, false
	}
	var exp struct {
		Config json.RawMessage `json:"config"`
	}
	if json.Unmarshal(expRaw, &exp) != nil || len(exp.Config) == 0 {
		return nil, nil, false
	}

	varsRaw, _ := os.ReadFile(base + ".vars.json")
	vmap := map[string]interface{}{}
	_ = json.Unmarshal(varsRaw, &vmap)
	state := map[string]string{}
	nulls := map[string]bool{}
	for k, v := range vmap {
		if v == nil {
			nulls[k] = true
			continue
		}
		state[k] = fmt.Sprint(v)
	}

	target := LocalTarget()
	if tpl.Changed != "" {
		ApplyOnChange(tpl.Changed, tpl.Vars, state, target)
	}
	resolved := ResolveTemplateVarsFor(tpl.Vars, state, nil, target)
	for n := range nulls {
		delete(resolved, n)
	}

	got, _, err := substituteVarsInJSONInternal(tpl.Config, tpl.Vars, resolved, target, false)
	if err != nil {
		// Ошибка боевого обходчика — тоже расхождение (канон не роняет сборку).
		return json.RawMessage(`"__legacy_error__"`), exp.Config, true
	}
	return got, exp.Config, true
}

func TestWalkerParityAgainstCorpus(t *testing.T) {
	root := templateCorpusRelPath
	if _, err := os.Stat(root); err != nil {
		t.Skipf("корпус не найден (%s)", root)
	}

	diverged := map[string]bool{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".template.json") {
			return nil
		}
		base := strings.TrimSuffix(p, ".template.json")
		// Имя кейса — ключ в knownWalkerDivergences, и оно обязано совпадать на
		// всех ОС. filepath.Walk отдаёт путь с разделителями платформы, поэтому
		// в слеши приводится ВЕСЬ путь, а не только префикс: иначе на Windows
		// TrimPrefix не срабатывал (префикс уже со слешами, путь ещё с `\`),
		// имена выходили вида `unresolved\null_value_drops_key`, ни один ключ
		// не совпадал — и страж рапортовал, что все расхождения «исчезли».
		name := strings.TrimPrefix(filepath.ToSlash(base), filepath.ToSlash(root)+"/")

		got, want, ok := loadParityCase(t, base)
		if !ok {
			return nil
		}
		var a, b interface{}
		if json.Unmarshal(got, &a) != nil || json.Unmarshal(want, &b) != nil {
			return nil
		}
		if !reflect.DeepEqual(a, b) {
			diverged[name] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("обход корпуса: %v", err)
	}

	// Новое расхождение — движки разъехались дальше, и это надо заметить
	// сразу, а не при следующем аудите.
	var unexpected []string
	for name := range diverged {
		if _, known := knownWalkerDivergences[name]; !known {
			unexpected = append(unexpected, name)
		}
	}
	sort.Strings(unexpected)
	for _, name := range unexpected {
		t.Errorf("НОВОЕ расхождение боевого и канонического обходчиков: %s\n"+
			"    боевой путь собирает конфиг иначе, чем требует контракт (общий с LxBox).\n"+
			"    Либо почините legacy-обходчик, либо осознанно внесите кейс в knownWalkerDivergences.", name)
	}

	// Исчезнувшее расхождение — список протух и вводит в заблуждение.
	var fixed []string
	for name := range knownWalkerDivergences {
		if !diverged[name] {
			fixed = append(fixed, name)
		}
	}
	sort.Strings(fixed)
	for _, name := range fixed {
		t.Errorf("расхождение %s БОЛЬШЕ НЕ ВОСПРОИЗВОДИТСЯ — уберите его из "+
			"knownWalkerDivergences, иначе список перестаёт означать что-либо", name)
	}
}
