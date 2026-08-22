// Package configtypes: direction_filter.go — сахар между формой Направления
// и языком фильтров (SPEC 104).
//
// Состав направления и его узел по умолчанию хранятся в тех же полях, что и
// раньше (`filters.tag`, `preferredDefault.tag`) и в том же языке паттернов
// (`matcher.go`): литерал, `!литерал`, `/regex/i`, `!/regex/i`. Так пресеты,
// генератор, превью и валидность остаются нетронутыми — шаблонный пресет
// `russian` уже патчит именно `filters.tag`.
//
// Форма же показывает пользователю ТОЛЬКО тело регулярки и галку инверсии
// (решение D-5/D-6): писать `/…/i` руками — лишнее знание, а флаг регистра
// не выбор пользователя, а свойство: имена узлов приходят из подписок в
// произвольном регистре.
package configtypes

import "strings"

// DirectionFilterBody разбирает паттерн в форму «тело + инверсия».
//
// ok == false означает, что паттерн не в форме `/…/`: литерал, пустая строка
// или мусор. Тогда форма показывает его как есть — терять чужое значение
// нельзя, а сохранение перепишет его в каноническую форму.
func DirectionFilterBody(pattern string) (body string, invert bool, ok bool) {
	invert = strings.HasPrefix(pattern, "!")
	rest := strings.TrimPrefix(pattern, "!")
	if regexStr, _, isRegex := trimRegexPattern(rest); isRegex {
		return regexStr, invert, true
	}
	return rest, invert, false
}

// DirectionFilterPattern собирает паттерн из формы.
//
// Флаг `i` ставится всегда: направление отбирает узлы по видимому имени, а
// регистр в именах подписок непредсказуем (см. §3.7 SPEC 104). Пустое тело
// даёт пустой паттерн — вызывающий обязан не класть такой ключ в фильтр
// вовсе, иначе пустая регулярка совпала бы со всем и смысл «фильтра нет»
// потерялся бы в отдельной ветке кода.
func DirectionFilterPattern(body string, invert bool) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if invert {
		return "!/" + body + "/i"
	}
	return "/" + body + "/i"
}

// DirectionFilterTag достаёт тело и инверсию из карты фильтров направления.
//
// Читается только ключ `tag`: остальные ключи (`host`, `scheme`, …) остаются
// доступны через JSON-вкладку редактора и переживают сохранение формы
// нетронутыми.
func DirectionFilterTag(filters map[string]interface{}) (body string, invert bool) {
	raw, _ := filters["tag"].(string)
	body, invert, _ = DirectionFilterBody(raw)
	return body, invert
}

// SetDirectionFilterTag кладёт (или убирает) ключ `tag` в карте фильтров.
//
// Возвращает карту, готовую к записи в Direction.Filters: nil, когда ключей
// не осталось — пустая карта и её отсутствие для генератора значат одно и то
// же, но в state.json пустой объект выглядел бы настройкой, которой нет.
func SetDirectionFilterTag(filters map[string]interface{}, body string, invert bool) map[string]interface{} {
	pattern := DirectionFilterPattern(body, invert)
	out := make(map[string]interface{}, len(filters)+1)
	for k, v := range filters {
		if k == "tag" {
			continue
		}
		out[k] = v
	}
	if pattern != "" {
		out["tag"] = pattern
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
