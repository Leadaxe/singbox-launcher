// Package nodewarn — показ предупреждений узла на языке пользователя
// (SPEC 131 §6, волна W3).
//
// # Почему отдельный пакет, а не хелпер в ui
//
// Строки узлов рисуются в ТРЁХ пакетах: `ui` (вкладка Servers, окно Core
// runtime, окно Info), `ui/configurator/tabs` (контейнер источника, список
// Preview) и вокруг них. Текст одного и того же кода обязан быть одинаковым
// во всех трёх — иначе пользователь читает про одно событие разными словами,
// и мы возвращаемся ровно к тому, от чего уходит вся кампания: к трём копиям
// одного правила.
//
// # Откуда берутся тексты
//
// ТОЛЬКО из реестра контракта (`registry.WarningText`), не из
// `bin/locale/*.json`: один источник на лаунчер, LxBox и генерацию
// документации. В locale живёт лишь обвязка — слова «Предупреждения»,
// «Подробнее» и счётчик «+N», у которых в реестре дома нет.
//
// # Ловушка подстановки
//
// `path` и `value` объявлены в реестре как НЕЯВНЫЕ параметры шаблона
// (`warnings.json` → `text_params_implicit`), то есть в `Params` записи их
// нет, а в тексте `{path}`/`{value}` встречаются. Реестровый `WarningText`
// подставляет ровно то, что ему передали, и без этого слияния пользователь
// читал бы «поле {path} снято». Слияние делается здесь, один раз на все
// поверхности.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package nodewarn

import (
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/registry"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/locale"
)

// Глифы уровней — ОДНО место на все поверхности.
//
// Проверено по шрифтам, которые Fyne 2.8.1 несёт с собой
// (`theme/font/`, цепочка «шрифт темы → эмодзи → символы»):
//
//   - «⚠» U+26A0 — есть в Inter, DejaVu и эмодзи-шрифте; уже используется;
//   - «✖» U+2716 — есть в эмодзи-шрифте и DejaVu; рисуется наравне с
//     «❌»/«✅», которыми проект пользуется давно;
//   - «ⓘ» U+24D8 — НЕТ НИ В ОДНОМ встроенном шрифте. Системный шрифт по
//     руне Fyne подобрать пытается, но на Win7-сборке рассчитывать на это
//     нельзя, и знак превратился бы в пустой прямоугольник ровно там, где
//     он должен успокаивать.
//
// Поэтому info говорит не глифом, а ИКОНКОЙ ТЕМЫ (`InfoIconCell`, section.go):
// `theme.InfoIcon()` — тот же «i в кружке», но SVG из ресурсов Fyne, одинаковый
// на всех платформах. Текстовый «(i)» остался ровно там, куда виджет не
// вставить: тултип строки (`ToolTip`) — обычная строка, и другого способа
// пометить в ней уровень нет.
const (
	// ErrorMark — узел отброшен либо непригоден.
	ErrorMark = "✖"
	// WarnMark — узел живой, но его поведение изменено.
	WarnMark = "⚠"
	// InfoMark — к сведению; делать ничего не нужно. ТОЛЬКО для текстовых
	// мест (тултип): в строке узла и в карточке info показывает иконка темы.
	InfoMark = "(i)"
)

// Mark — глиф предупреждения общего вида. Оставлен под прежним именем: на него
// смотрят поверхности, которые рисуют «что-то не так» без разбора уровня
// (заголовок секции, сводки превью).
const Mark = WarnMark

// Уровни важности кода. Значения совпадают с полем `severity` реестра
// (contract/registry/warnings.json) — переводить их в свою шкалу незачем.
const (
	// SeverityError — узел отброшен: ядро такую запись не приняло бы.
	SeverityError = "error"
	// SeverityWarning — узел живой, но конвейер что-то снял или заменил.
	SeverityWarning = "warning"
	// SeverityInfo — к сведению: поведение узла не изменилось.
	SeverityInfo = "info"
)

// rank — вес уровня для сортировки: меньше = важнее.
//
// Неизвестный уровень (код есть в состоянии, но не в реестре, либо реестр не
// прочитался) считается WARNING, а не INFO: показать проблему сильнее, чем
// нужно, — досадно; спрятать её в «к сведению» — опасно.
func rank(sev string) int {
	switch sev {
	case SeverityError:
		return 0
	case SeverityInfo:
		return 2
	default:
		return 1
	}
}

// MarkOf — глиф уровня.
func MarkOf(sev string) string {
	switch sev {
	case SeverityError:
		return ErrorMark
	case SeverityInfo:
		return InfoMark
	default:
		return WarnMark
	}
}

// Text — предупреждение, готовое к показу.
type Text struct {
	// Code — код реестра; он же якорь в документации и ключ кнопки «Подробнее».
	Code string
	// Severity — уровень кода из реестра: error / warning / info. У кода,
	// которого реестр не знает, здесь SeverityWarning (см. rank).
	Severity string
	// Title — короткий заголовок (строка списка, тултип).
	Title string
	// Body — полное объяснение с подстановками. Длина произвольная: показывать
	// только в Label с TextWrapWord (Л19, fyne-label-minwidth-trap).
	Body string
	// Path — путь поля в теле; пуст у кодов уровня узла.
	Path string
	// Cause — откуда такое берётся: не «что случилось с узлом» (это Body), а
	// почему подписка вообще прислала такое значение. Пусто, если реестр
	// причины не даёт.
	Cause string
	// Fixes — что человек может сделать, по одному действию на пункт. У
	// info-кодов первый пункт честно говорит «ничего не нужно».
	Fixes []string
	// DocURL — якорь кода в сгенерированной документации.
	DocURL string
}

// DocURL — адрес якоря кода в документации контракта.
func DocURL(code string) string {
	if code == "" {
		return ""
	}
	return constants.ContractWarningsDocBaseURL + code
}

// Describe переводит записи состояния в тексты на текущем языке UI.
//
// Код, которого в реестре нет (состояние старше сборки, либо реестр не
// прочитался), не проглатывается: он показывается САМИМ КОДОМ. Молча
// пропустить его значило бы показать узлу ⚠ без единой причины — ровно тот
// тупик, от которого уходит волна.
func Describe(in []state.NodeWarning) []Text {
	if len(in) == 0 {
		return nil
	}
	lang := locale.GetLang()
	reg, err := registry.Get()
	out := make([]Text, 0, len(in))
	for _, w := range in {
		t := Text{Code: w.Code, Path: w.Path, DocURL: DocURL(w.Code), Severity: SeverityWarning}
		if err == nil && reg != nil {
			if entry, ok := reg.Warning(w.Code); ok && entry.Severity != "" {
				t.Severity = entry.Severity
			}
			if title, body, ok := reg.WarningText(w.Code, lang, paramsOf(w)); ok {
				t.Title, t.Body = title, body
			}
			// Причина и решения — отдельные поля реестра: подстановок они не
			// несут (говорят о КЛАССЕ проблемы, не об этом значении), поэтому
			// берутся как есть.
			if cause, fixes, ok := reg.WarningAdvice(w.Code, lang); ok {
				t.Cause, t.Fixes = cause, fixes
			}
		}
		if t.Title == "" {
			t.Title = w.Code
		}
		if t.Body == "" {
			t.Body = w.Code
		}
		out = append(out, t)
	}
	return out
}

// paramsOf — параметры подстановки записи вместе с неявными path/value.
//
// Копия, а не правка `w.Params`: карта приехала из состояния, и дописывать в
// неё значило бы менять запись узла на показе.
func paramsOf(w state.NodeWarning) map[string]string {
	p := make(map[string]string, len(w.Params)+2)
	for k, v := range w.Params {
		p[k] = v
	}
	// Явный параметр записи сильнее неявного: если конвейер уже положил свой
	// path/value, перезаписывать его адресом поля нельзя.
	if _, has := p["path"]; !has && w.Path != "" {
		p["path"] = w.Path
	}
	if _, has := p["value"]; !has && w.Value != "" {
		p["value"] = w.Value
	}
	return p
}

// byLevel раскладывает описания на три группы, СОХРАНЯЯ внутри каждой
// прежний порядок.
//
// Порядок внутри уровня — это обход body.order реестра (PARSING_PRINCIPLES §6), он
// детерминирован, и пересортировывать его нельзя: ровно он отвечает на «какое
// поле раньше». Уровень добавляет к нему второе измерение — «что важнее», —
// и только его.
//
// Отдельная функция, а не sort.SliceStable на месте: устойчивую сортировку
// пришлось бы звать из трёх мест (подстрока, тултип, секция), и один
// забытый вызов дал бы три разных порядка у одного узла.
func byLevel(texts []Text) (errs, warns, infos []Text) {
	for _, t := range texts {
		switch rank(t.Severity) {
		case 0:
			errs = append(errs, t)
		case 2:
			infos = append(infos, t)
		default:
			warns = append(warns, t)
		}
	}
	return errs, warns, infos
}

// Summary — краткая подпись для СТРОКИ узла: заголовок СТАРШЕГО кода, а при
// нескольких — с хвостом «+N».
//
// Считаются только error и warning: подстрока строки отвечает на вопрос «с
// этим узлом что-то не так?», и info говорит «нет, всё так». Узел, у которого
// info — единственное, что есть, подстроки не получает вовсе и показывает свой
// обычный состав; про info сообщает иконка в начале подстроки (InfoIconCell)
// и тултип.
//
// Пустая строка = показывать нечего.
func Summary(in []state.NodeWarning) string {
	errs, warns, _ := byLevel(Describe(in))
	shown := make([]Text, 0, len(errs)+len(warns))
	shown = append(shown, errs...)
	shown = append(shown, warns...)
	if len(shown) == 0 {
		return ""
	}
	s := shown[0].Title
	if len(shown) > 1 {
		s += " " + locale.Tf("+%d", len(shown)-1)
	}
	return s
}

// SubtitleMark — глиф подстроки: старший уровень среди error/warning.
//
// Пусто, когда показывать в подстроке нечего (кодов нет либо они все info).
func SubtitleMark(in []state.NodeWarning) string {
	errs, warns, _ := byLevel(Describe(in))
	switch {
	case len(errs) > 0:
		return ErrorMark
	case len(warns) > 0:
		return WarnMark
	default:
		return ""
	}
}

// Subtitle — та же подпись, но с глифом своего уровня впереди: подстрока
// строки узла.
//
// Describe зовётся ОДИН раз на обе половины: он ходит в реестр и делает
// подстановки на каждый код, а строка узла перерисовывается на каждом
// обновлении списка.
func Subtitle(in []state.NodeWarning) string {
	errs, warns, _ := byLevel(Describe(in))
	shown := make([]Text, 0, len(errs)+len(warns))
	shown = append(shown, errs...)
	shown = append(shown, warns...)
	if len(shown) == 0 {
		return ""
	}
	mark := WarnMark
	if len(errs) > 0 {
		mark = ErrorMark
	}
	s := mark + " " + shown[0].Title
	if len(shown) > 1 {
		s += " " + locale.Tf("+%d", len(shown)-1)
	}
	return s
}

// HasInfo — есть ли у узла хоть один info-код.
func HasInfo(in []state.NodeWarning) bool {
	_, _, infos := byLevel(Describe(in))
	return len(infos) > 0
}

// HasProblems — есть ли у узла хоть одна ПРОБЛЕМА: код уровня error или
// warning.
//
// Единственный предикат для всякого решения «выделить строку / поставить ⚠ /
// сосчитать узел в счётчик предупреждений». Прежний `len(node.Warnings) > 0`
// отвечал на другой вопрос — «есть ли у узла хоть какая-то запись», — и узел,
// у которого единственный код info («к сведению, делать ничего не нужно»),
// получал оранжевую подстроку и место в счётчике «⚠ 12». Это ровно та
// тревога на пустом месте, от которой уходило разведение уровней: раз info
// не меняет поведения узла, он не вправе окрашивать ни строку, ни шапку.
//
// Неизвестный код считается warning (см. rank), то есть ПРОБЛЕМОЙ: спрятать
// незнакомую деградацию опаснее, чем показать лишнюю.
func HasProblems(in []state.NodeWarning) bool {
	if len(in) == 0 {
		return false
	}
	errs, warns, _ := byLevel(Describe(in))
	return len(errs)+len(warns) > 0
}

// InfoOnly — у узла есть коды, и ВСЕ они info.
//
// Отдельный предикат, а не `!HasProblems && len(in) > 0`: то выражение
// считает «есть коды» по сырому списку, а `HasProblems` — по разобранному, и
// два разных источника правды разъехались бы на первой же записи, которую
// Describe отбрасывает.
func InfoOnly(in []state.NodeWarning) bool {
	errs, warns, infos := byLevel(Describe(in))
	return len(errs)+len(warns) == 0 && len(infos) > 0
}

// SplitNodes — разложить узлы по тому, ЧТО им показывать.
//
// Вход — список списков кодов (по одному на узел), выход — сколько узлов
// несут проблему и сколько несут только «к сведению». Счётчики шапок и сводок
// спрашивают ровно это, и считать его каждый своим циклом значило бы завести
// три разных ответа на один вопрос.
func SplitNodes(nodes [][]state.NodeWarning) (problems, infoOnly int) {
	for _, w := range nodes {
		switch {
		case HasProblems(w):
			problems++
		case HasInfo(w):
			infoOnly++
		}
	}
	return problems, infoOnly
}

// ToolTip — ВСЕ заголовки списком, по строке на код, каждый со своим глифом,
// в порядке error → warning → info.
//
// Тултип — единственное место, где узел показывает свои коды целиком, не
// открывая окна: подстрока держит только старший уровень, а info в неё не
// попадает вовсе. Полные объяснения сюда не идут — тексты реестра абзацами
// накрыли бы пол-экрана; их дом — секция в окне узла.
func ToolTip(in []state.NodeWarning) string {
	errs, warns, infos := byLevel(Describe(in))
	ordered := make([]Text, 0, len(errs)+len(warns)+len(infos))
	ordered = append(ordered, errs...)
	ordered = append(ordered, warns...)
	ordered = append(ordered, infos...)
	if len(ordered) == 0 {
		return ""
	}
	s := ""
	for i, t := range ordered {
		if i > 0 {
			s += "\n"
		}
		s += MarkOf(t.Severity) + " " + t.Title
		if t.Path != "" {
			s += "  ·  " + t.Path
		}
	}
	return s
}

// FromParsed — записи конвейера (`configtypes.Warning`) в форме состояния.
//
// Конвертер живёт здесь, а не рядом со своим близнецом `stateWarnings`
// (`core/config/migrate_materialize.go`): тот обслуживает МАТЕРИАЛИЗАЦИЮ —
// воронку «текст → узел», — а этот нужен UI-форме, которая пересчитывает
// warnings уже собранного узла. Форма у типов одна (PARSING_PRINCIPLES §6), конверсия
// механическая, и тянуть ради неё зависимость ui → core/config незачем.
func FromParsed(in []configtypes.Warning) []state.NodeWarning {
	if len(in) == 0 {
		return nil
	}
	out := make([]state.NodeWarning, 0, len(in))
	for _, w := range in {
		nw := state.NodeWarning{Code: w.Code, Path: w.Path, Value: w.Value}
		if len(w.Params) > 0 {
			// Копия карты: общая карта у двух записей означала бы правку
			// одной через другую.
			p := make(map[string]string, len(w.Params))
			for k, v := range w.Params {
				p[k] = v
			}
			nw.Params = p
		}
		out = append(out, nw)
	}
	return out
}

// FieldsStripped — сколько предупреждений адресуют КОНКРЕТНОЕ поле (у них
// непустой Path) и сколько — узел целиком.
//
// Развилка нужна сводке превью: «снято N полей» про код уровня узла было бы
// неправдой — там снято не поле, а разобрана вся запись.
func FieldsStripped(in []state.NodeWarning) (fields, nodeLevel int) {
	for _, w := range in {
		if w.Path != "" {
			fields++
			continue
		}
		nodeLevel++
	}
	return fields, nodeLevel
}
