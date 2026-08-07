package subscription

import (
	"encoding/json"
	"strings"
)

// BodyKind — распознанный формат тела подписки (SPEC 094 A1).
//
// Единственный источник правды о том, чем является тело: до SPEC 094 решение
// принимали независимо IsXrayJSONArrayBody (ветка source_loader) и
// DecodeSubscriptionContent (ветка fetcher), и они расходились — второй
// отвергал любое тело, начинающееся с '{', ещё до парсера.
type BodyKind int

const (
	// BodyKindURIList — построчный список share-URI (vless://, ss:// и т.д.).
	BodyKindURIList BodyKind = iota
	// BodyKindXrayArray — JSON-массив Xray-конфигов (элементы несут "protocol").
	BodyKindXrayArray
	// BodyKindSingboxOutbound — одиночный sing-box outbound: {"type":"vless",...}.
	BodyKindSingboxOutbound
	// BodyKindSingboxOutboundArray — массив sing-box outbound'ов.
	BodyKindSingboxOutboundArray
	// BodyKindSingboxConfig — целый sing-box конфиг (outbounds и/или endpoints).
	BodyKindSingboxConfig
	// BodyKindSingboxConfigArray — массив целых sing-box конфигов.
	BodyKindSingboxConfigArray
)

// String — человекочитаемое имя для логов.
func (k BodyKind) String() string {
	switch k {
	case BodyKindXrayArray:
		return "xray-array"
	case BodyKindSingboxOutbound:
		return "singbox-outbound"
	case BodyKindSingboxOutboundArray:
		return "singbox-outbound-array"
	case BodyKindSingboxConfig:
		return "singbox-config"
	case BodyKindSingboxConfigArray:
		return "singbox-config-array"
	default:
		return "uri-list"
	}
}

// IsSingbox сообщает, разбирается ли тело импортёром sing-box JSON.
func (k BodyKind) IsSingbox() bool {
	switch k {
	case BodyKindSingboxOutbound, BodyKindSingboxOutboundArray,
		BodyKindSingboxConfig, BodyKindSingboxConfigArray:
		return true
	default:
		return false
	}
}

// ClassifySubscriptionBody определяет формат тела подписки.
//
// Порядок проверок значим и зафиксирован в SPEC 094 A1:
//
//	"type" проверяется РАНЬШЕ "outbounds", потому что одиночный selector несёт
//	оба поля сразу — и это outbound, а не конфиг. Обратный порядок превратил бы
//	вставленный селектор в конфиг с самим собой внутри.
//
// Xray-массив проверяется раньше sing-box-массивов: элемент Xray-конфига несёт
// "outbounds" с "protocol" внутри, и без этой проверки уехал бы в ветку
// массива конфигов, где sing-box-разбор не нашёл бы ни одного "type".
//
// Всё нераспознанное — BodyKindURIList: построчный разбор остаётся поведением
// по умолчанию, как и до SPEC 094.
func ClassifySubscriptionBody(body string) BodyKind {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return BodyKindURIList
	}

	switch trimmed[0] {
	case '[':
		return classifyJSONArrayBody(trimmed)
	case '{':
		return classifyJSONObjectBody(trimmed)
	default:
		return BodyKindURIList
	}
}

// classifyJSONArrayBody разбирает тело-массив.
func classifyJSONArrayBody(trimmed string) BodyKind {
	var elems []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &elems); err != nil {
		return BodyKindURIList
	}
	if len(elems) == 0 {
		// Пустой массив: ни узлов, ни признаков формата. URI-ветка на нём
		// просто не найдёт ссылок — это честнее, чем выдумывать формат.
		return BodyKindURIList
	}

	// Первый элемент, по которому вообще можно судить о формате, и решает.
	// Проверка Xray идёт по наличию "protocol" внутри outbounds, а НЕ по
	// IsXrayJSONArrayBody: та отвечает лишь «это валидный JSON-массив» и
	// на массиве sing-box-outbound'ов тоже вернула бы true.
	for _, raw := range elems {
		var obj map[string]interface{}
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		// "type" раньше "outbounds" — см. докстрок ClassifySubscriptionBody.
		if _, ok := obj["type"].(string); ok {
			return BodyKindSingboxOutboundArray
		}
		if xrayElementHasProtocolOutbounds(obj) {
			return BodyKindXrayArray
		}
		if hasJSONArrayField(obj, "outbounds") || hasJSONArrayField(obj, "endpoints") {
			return BodyKindSingboxConfigArray
		}
	}

	return BodyKindURIList
}

// classifyJSONObjectBody разбирает тело-объект.
func classifyJSONObjectBody(trimmed string) BodyKind {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return BodyKindURIList
	}

	// "type" раньше "outbounds": одиночный selector несёт оба поля.
	if _, ok := obj["type"].(string); ok {
		return BodyKindSingboxOutbound
	}
	if hasJSONArrayField(obj, "outbounds") || hasJSONArrayField(obj, "endpoints") {
		return BodyKindSingboxConfig
	}

	return BodyKindURIList
}

// hasJSONArrayField сообщает, есть ли в объекте поле-массив с данным именем.
func hasJSONArrayField(obj map[string]interface{}, name string) bool {
	v, ok := obj[name]
	if !ok {
		return false
	}
	_, isArray := v.([]interface{})
	return isArray
}
