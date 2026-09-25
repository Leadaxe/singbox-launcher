package template

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Страж боевого пути (SPEC 143, Т4).
//
// Корпус контракта (общий с LxBox) гоняется не через отдельную точку входа
// канона, а через тот же путь, которым собирается главный конфиг:
// params → подстановка → предупреждения (applyTemplateResolved под
// ApplyTemplateWithVarsForWarnings). Расхождений между прод-путём и
// expected.json быть не должно — списка исключений у стража нет.
//
// Загрузочный валидатор здесь не участвует: боевой путь его не зовёт (он
// стоит на загрузке шаблона), а рантайм-ожидания reject/either-кейсов обязаны
// выполняться и при толерантном прогоне — как в TestContractCorpusTemplate.

// runCorpusCaseProdPath прогоняет кейс корпуса через боевой путь главного
// конфига и возвращает конфиг и коды предупреждений.
//
// Кейсы без null идут через ApplyTemplateWithVarsForWarnings целиком: резолв
// значений тоже боевой. Состояние desktop — строки, и optional-var без
// значения (null в vars.json) через него не выразить, поэтому такие кейсы
// входят после резолва — в applyTemplateResolved, тело того же пути.
func runCorpusCaseProdPath(t *testing.T, c corpusCase) corpusExpected {
	t.Helper()
	target := LocalTarget()
	state := c.stateVars()
	if c.template.Changed != "" {
		ApplyOnChange(c.template.Changed, c.template.Vars, state, target)
	}

	var (
		out      []byte
		warnings []TemplateWarning
		err      error
	)
	nulls := c.nullVars()
	if len(nulls) == 0 {
		out, warnings, err = ApplyTemplateWithVarsForWarnings(c.template.Config, nil, c.template.Vars, state, nil, target)
	} else {
		resolved := ResolveTemplateVarsFor(c.template.Vars, state, nil, target)
		MaybeGenerateSecrets(c.template.Vars, resolved)
		for name := range nulls {
			delete(resolved, name)
		}
		out, warnings, err = applyTemplateResolved(c.template.Config, nil, c.template.Vars, resolved, target)
	}
	if err != nil {
		t.Fatalf("боевой путь: %v", err)
	}

	codes := make([]string, 0, len(warnings))
	for _, w := range warnings {
		codes = append(codes, w.Code)
	}
	got := corpusExpected{Config: out, Warnings: normalizeWarnings(codes)}
	if c.template.Changed != "" {
		got.VarsAfter = state
	}
	return got
}

func TestWalkerParityAgainstCorpus(t *testing.T) {
	root := templateCorpusRelPath
	if _, err := os.Stat(root); err != nil {
		t.Skipf("корпус не найден (%s)", root)
	}

	var bases []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".template.json") {
			return nil
		}
		bases = append(bases, strings.TrimSuffix(p, ".template.json"))
		return nil
	})
	if err != nil {
		t.Fatalf("обход корпуса: %v", err)
	}
	if len(bases) == 0 {
		t.Fatal("корпус шаблонов пуст")
	}
	sort.Strings(bases)

	for _, base := range bases {
		// Имя — в слешах на всех ОС: filepath.Walk отдаёт разделители
		// платформы, а сообщение стража должно называть кейс одинаково.
		name := strings.TrimPrefix(filepath.ToSlash(base), filepath.ToSlash(root)+"/")
		t.Run(name, func(t *testing.T) {
			c := loadCorpusCase(t, base)
			got := runCorpusCaseProdPath(t, c)

			ok := jsonEqual(got.Config, c.expected.Config) &&
				reflect.DeepEqual(got.Warnings, normalizeWarnings(c.expected.Warnings))
			if ok && c.template.Changed != "" {
				ok = reflect.DeepEqual(got.VarsAfter, c.expected.VarsAfter)
			}
			if !ok {
				t.Errorf("боевой путь главного конфига расходится с контрактом (общим с LxBox)\n"+
					"  config   получено: %s\n  config   ожидалось: %s\n"+
					"  warnings получено: %v\n  warnings ожидалось: %v\n"+
					"  vars_after получено: %v\n  vars_after ожидалось: %v",
					got.Config, c.expected.Config,
					got.Warnings, normalizeWarnings(c.expected.Warnings),
					got.VarsAfter, c.expected.VarsAfter)
			}
		})
	}
}
