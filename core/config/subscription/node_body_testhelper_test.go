package subscription

// Общий помощник тестов на время кампании SPEC 133.
//
// Тело узла собирал когда-то второй путь — buildOutbound из node.Query и
// node.UUID, — и тесты, написанные до кампании, звали его напрямую. На
// движковом узле он возвращал пустышку: договорённости «парсер досочиняет
// значения обратно в Query» у движка нет и быть не должно.
//
// РУКОПИСНОГО ПУТИ БОЛЬШЕ НЕТ (SPEC 133): buildOutbound удалён вместе с
// последней схемой, которой он был нужен. Помощник остался только потому,
// что делает одну полезную вещь — синхронизирует тег, который тесты нередко
// переназначают ПОСЛЕ разбора. Его вызовы можно менять на node.Outbound по
// мере правки тестов.

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// nodeBody — тело узла, каким его увидит конфиг ядра.
func nodeBody(t *testing.T, node *configtypes.ParsedNode) map[string]interface{} {
	t.Helper()
	if node == nil {
		t.Fatal("nodeBody: узел nil")
	}
	// Тег тесты нередко переназначают ПОСЛЕ разбора и ждут его в теле: у
	// рукописного пути тело строилось позже, у движкового оно уже готово.
	if node.Tag != "" {
		node.Outbound["tag"] = node.Tag
	}
	return node.Outbound
}

// bodyStrings читает список строк тела НЕЗАВИСИМО от его записи в Go.
//
// Рукописные парсеры складывали списки как []string, движок — как
// []interface{} (значение приезжает из таблицы, а не из литерала кода). Для
// конфига ядра разницы нет: json.Marshal печатает оба одинаково, и эмиттеры
// share-URI читают []interface{} первой веткой. Тесты же ассертили
// конкретный Go-тип и на движковом теле падали (а где ассерт был без
// запятой-ok — паниковали).
//
// Помощник снимает ровно эту разницу и ничего больше: значение не той формы
// (скаляр, карта) он не спасает, и такой тест останется красным.
func bodyStrings(v interface{}) ([]string, bool) {
	switch t := v.(type) {
	case []string:
		return t, true
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}

// bodyHeaders — то же для карты заголовков (map[string]string против
// map[string]interface{}).
func bodyHeaders(v interface{}) map[string]string {
	switch t := v.(type) {
	case map[string]string:
		return t
	case map[string]interface{}:
		out := make(map[string]string, len(t))
		for k, item := range t {
			if s, ok := item.(string); ok {
				out[k] = s
			}
		}
		return out
	}
	return nil
}
