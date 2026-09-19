// File nodewarn_test.go — ВЫБОР уровня, а не тексты.
//
// Тест смотрит только на логику развилки: какой глиф несёт подстрока, сколько
// кодов она сосчитала, с какой стороны подстроки встаёт иконка «к сведению» и
// в каком порядке идут группы в тултипе. Сами заголовки приезжают из реестра
// контракта и
// живут своей жизнью — сверять их здесь значило бы ломать тест на каждой
// правке текста (память `no-ui-format-tests`).
//
// Коды берутся ИЗ РЕЕСТРА: severity объявлена там, и зашитый в тест список
// разъехался бы с ним на первом же понижении уровня — ровно на том, ради
// которого тест и написан (reality_fp_not_chrome: warning → info).
package nodewarn

import (
	"strconv"
	"strings"
	"testing"

	"singbox-launcher/core/config/registry"
	"singbox-launcher/core/state"
)

// codesBySeverity — по одному живому коду каждого уровня, прямо из реестра.
func codesBySeverity(t *testing.T) (errCode, warnCode, infoCode string) {
	t.Helper()
	reg, err := registry.Get()
	if err != nil {
		t.Fatalf("реестр не прочитался: %v", err)
	}
	// Конкретные коды, а не «первый попавшийся из карты»: обход map в Go не
	// упорядочен, и тест на случайных кодах падал бы через раз.
	for _, c := range []struct {
		code, want string
		dst        *string
	}{
		{"protocol_unsupported", SeverityError, &errCode},
		{"transport_unsupported", SeverityWarning, &warnCode},
		{"reality_fp_not_chrome", SeverityInfo, &infoCode},
	} {
		entry, ok := reg.Warning(c.code)
		if !ok {
			t.Fatalf("кода %q нет в реестре", c.code)
		}
		if entry.Severity != c.want {
			t.Fatalf("severity кода %q = %q, ожидалась %q", c.code, entry.Severity, c.want)
		}
		*c.dst = c.code
	}
	return errCode, warnCode, infoCode
}

func warnings(codes ...string) []state.NodeWarning {
	out := make([]state.NodeWarning, 0, len(codes))
	for _, c := range codes {
		out = append(out, state.NodeWarning{Code: c})
	}
	return out
}

// TestSeverityLevelsDrivePresentation — один набор кодов на входе, все четыре
// решения показа на выходе.
func TestSeverityLevelsDrivePresentation(t *testing.T) {
	e, w, i := codesBySeverity(t)

	cases := []struct {
		name string
		in   []state.NodeWarning
		// mark — глиф подстроки; "" = подстроки нет вовсе.
		mark string
		// plus — сколько кодов сосчитано сверх первого («+N»); -1 = хвоста нет.
		plus int
		info bool
	}{
		{"пусто", nil, "", -1, false},
		{"только info", warnings(i), "", -1, true},
		{"только warning", warnings(w), WarnMark, -1, false},
		{"только error", warnings(e), ErrorMark, -1, false},
		{"два info и warning", warnings(i, i, w), WarnMark, -1, true},
		// Старший уровень решает глиф, даже когда пришёл ВТОРЫМ: порядок
		// внутри уровня детерминирован обходом реестра, но между уровнями
		// важность сильнее исходного порядка.
		{"warning, затем error", warnings(w, e), ErrorMark, 1, false},
		{"error, затем warning", warnings(e, w), ErrorMark, 1, false},
		// info не считается «+N»: подстрока отвечает на «что не так», а info
		// отвечает «ничего».
		{"warning и info", warnings(w, i), WarnMark, -1, true},
		{"error, warning, info", warnings(e, w, i), ErrorMark, 1, true},
		{"два info", warnings(i, i), "", -1, true},
		// Кода нет в реестре — считается warning (спрятать проблему в «к
		// сведению» опаснее, чем показать её сильнее нужного).
		{"неизвестный код", warnings("no_such_code_in_registry"), WarnMark, -1, false},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			sub := Subtitle(c.in)
			if c.mark == "" {
				if sub != "" {
					t.Errorf("подстрока должна быть пустой, получено %q", sub)
				}
			} else {
				if !strings.HasPrefix(sub, c.mark+" ") {
					t.Errorf("подстрока %q не начинается глифом %q", sub, c.mark)
				}
				if got := SubtitleMark(c.in); got != c.mark {
					t.Errorf("SubtitleMark = %q, ожидался %q", got, c.mark)
				}
			}

			// Счётчик «+N» — в подстроке хвостом. Сверяем ЧИСЛО, а не формат:
			// формат («+2») переводится, и сверять его — сверять текст.
			hasPlus := strings.Contains(sub, "+")
			if c.plus < 0 && hasPlus {
				t.Errorf("подстрока %q несёт хвост «+N», хотя считать нечего", sub)
			}
			if c.plus > 0 {
				want := "+" + strconv.Itoa(c.plus)
				if !strings.HasSuffix(sub, want) {
					t.Errorf("подстрока %q не кончается %q", sub, want)
				}
			}

			if got := HasInfo(c.in); got != c.info {
				t.Errorf("HasInfo = %v, ожидалось %v", got, c.info)
			}
			// HasProblems — тот же вопрос, на который отвечает подстрока:
			// «с этим узлом что-то не так?». Расходиться им нельзя, иначе
			// строка красится оранжевым без подстроки (или наоборот), и
			// счётчик шапки объявляет узел испорченным там, где список
			// показывает его здоровым — ровно эта дыра и чинится.
			wantProblems := c.mark != ""
			if got := HasProblems(c.in); got != wantProblems {
				t.Errorf("HasProblems = %v, ожидалось %v (глиф подстроки %q)",
					got, wantProblems, c.mark)
			}
			// InfoOnly — «есть что сказать, и это всё, что есть».
			wantInfoOnly := c.info && !wantProblems
			if got := InfoOnly(c.in); got != wantInfoOnly {
				t.Errorf("InfoOnly = %v, ожидалось %v", got, wantInfoOnly)
			}
			// Иконка info в ПОДСТРОКЕ: есть ли она и с какой стороны текста.
			// Проверяется ВЫБОР МЕСТА, а не вёрстка: правило «начало
			// подстроки принадлежит старшему уровню» — то же, по которому
			// Subtitle ставит ✖/⚠, и разъехаться им нельзя.
			lead, trail := infoIconSides(c.in)
			wantLead := c.info && c.mark == ""
			wantTrail := c.info && c.mark != ""
			if lead != wantLead || trail != wantTrail {
				t.Errorf("иконка info: слева=%v справа=%v, ожидалось слева=%v справа=%v",
					lead, trail, wantLead, wantTrail)
			}
		})
	}
}

// TestToolTipOrdersGroups — тултип показывает ВСЕ коды, по строке, в порядке
// error → warning → info, каждый со своим глифом.
func TestToolTipOrdersGroups(t *testing.T) {
	e, w, i := codesBySeverity(t)

	// Вход намеренно в «неправильном» порядке: тултип обязан переставить.
	lines := strings.Split(ToolTip(warnings(i, w, e)), "\n")
	if len(lines) != 3 {
		t.Fatalf("ожидалось 3 строки, получено %d: %q", len(lines), lines)
	}
	wantMarks := []string{ErrorMark, WarnMark, InfoMark}
	for n, want := range wantMarks {
		if !strings.HasPrefix(lines[n], want+" ") {
			t.Errorf("строка %d = %q, ожидался глиф %q", n, lines[n], want)
		}
	}

	if ToolTip(nil) != "" {
		t.Error("пустой вход обязан давать пустой тултип")
	}

	// Одни info: тултип есть (иначе про них узнать негде), подстроки нет.
	only := ToolTip(warnings(i))
	if !strings.HasPrefix(only, InfoMark+" ") {
		t.Errorf("тултип из одних info = %q", only)
	}
	if Subtitle(warnings(i)) != "" {
		t.Error("узел с одними info не должен получать подстроку")
	}
}

// TestSectionGroupsByLevel — раздел «Уведомления» окна узла собирается на
// любом наборе и молчит на пустом.
//
// Проверяется СТРУКТУРА выбора (есть блок / нет блока), а не вёрстка: обходить
// дерево виджетов ради подписи значило бы писать тест на формат UI.
func TestSectionGroupsByLevel(t *testing.T) {
	e, w, i := codesBySeverity(t)

	if Section(nil) != nil {
		t.Error("пустой вход обязан давать nil-секцию")
	}
	for _, in := range [][]state.NodeWarning{
		warnings(i),
		warnings(w),
		warnings(e),
		warnings(e, w, i),
	} {
		if Section(in) == nil {
			t.Errorf("секция для %v не собралась", in)
		}
	}

	// Порядок групп — разложение, на котором секция и строится.
	errs, warns, infos := byLevel(Describe(warnings(i, e, w)))
	if len(errs) != 1 || len(warns) != 1 || len(infos) != 1 {
		t.Fatalf("разложение по уровням: error=%d warning=%d info=%d", len(errs), len(warns), len(infos))
	}
	if errs[0].Code != e || warns[0].Code != w || infos[0].Code != i {
		t.Errorf("коды разъехались по уровням: %q / %q / %q", errs[0].Code, warns[0].Code, infos[0].Code)
	}
}

// TestSplitNodesCountsByLevel — счёт УЗЛОВ, а не кодов: ровно то число, что
// уходит в «⚠ N» шапки контейнера и в «For your information: N» сводки.
//
// Данные-критично здесь одно: узел не должен попасть в обе группы разом и не
// должен пропасть из обеих. Двенадцать info-узлов, посчитанных как
// предупреждения, — та самая ложная тревога, из-за которой счёт и переехал на
// уровни.
func TestSplitNodesCountsByLevel(t *testing.T) {
	e, w, i := codesBySeverity(t)

	nodes := [][]state.NodeWarning{
		nil,                                  // чистый узел — ни в одну группу
		warnings(i),                          // только «к сведению»
		warnings(i, i),                       // два info у одного узла — всё равно ОДИН узел
		warnings(w),                          // проблема
		warnings(e),                          // проблема
		warnings(e, i),                       // проблема И info: считается проблемой, и только ею
		warnings("no_such_code_in_registry"), // неизвестный код = проблема
	}
	problems, infoOnly := SplitNodes(nodes)
	if problems != 4 {
		t.Errorf("узлов с проблемой = %d, ожидалось 4", problems)
	}
	if infoOnly != 2 {
		t.Errorf("узлов только с «к сведению» = %d, ожидалось 2", infoOnly)
	}
	// Сумма меньше числа узлов ровно на чистые — ни один узел не сосчитан
	// дважды и ни один не потерян.
	if problems+infoOnly != len(nodes)-1 {
		t.Errorf("сумма групп = %d, узлов с кодами = %d", problems+infoOnly, len(nodes)-1)
	}

	if p, io := SplitNodes(nil); p != 0 || io != 0 {
		t.Errorf("пустой вход дал %d/%d", p, io)
	}
}
