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

// Mark — глиф предупреждения. Тот же «⚠» U+26A0, которым уже отмечены
// неразобранные записи, недоступные цели detour и потери бэкапа: волна новых
// глифов не заводит (SPEC 131 §6).
const Mark = "⚠"

// Text — предупреждение, готовое к показу.
type Text struct {
	// Code — код реестра; он же якорь в документации и ключ кнопки «Подробнее».
	Code string
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
		t := Text{Code: w.Code, Path: w.Path, DocURL: DocURL(w.Code)}
		if err == nil && reg != nil {
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

// Summary — краткая подпись для СТРОКИ узла: заголовок первого кода, а при
// нескольких — с хвостом «+N».
//
// Первый, а не «самый важный»: порядок warnings задан обходом body.order
// реестра и потому детерминирован (CANON §6). Сортировать его по severity
// здесь значило бы завести четвёртое место, где живёт приоритет кодов.
//
// Пустая строка = показывать нечего.
func Summary(in []state.NodeWarning) string {
	texts := Describe(in)
	if len(texts) == 0 {
		return ""
	}
	s := texts[0].Title
	if len(texts) > 1 {
		s += " " + locale.Tf("+%d", len(texts)-1)
	}
	return s
}

// Subtitle — та же подпись, но с глифом впереди: подстрока строки узла.
func Subtitle(in []state.NodeWarning) string {
	s := Summary(in)
	if s == "" {
		return ""
	}
	return Mark + " " + s
}

// ToolTip — все заголовки списком, по строке на код.
//
// Полное объяснение сюда не идёт: тексты реестра — абзацы, и тултип из них
// накрыл бы пол-экрана. Их дом — секция в окне узла, где рядом лежит и
// кнопка на документацию.
func ToolTip(in []state.NodeWarning) string {
	texts := Describe(in)
	if len(texts) == 0 {
		return ""
	}
	s := ""
	for i, t := range texts {
		if i > 0 {
			s += "\n"
		}
		s += Mark + " " + t.Title
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
// warnings уже собранного узла. Форма у типов одна (CANON §6), конверсия
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
