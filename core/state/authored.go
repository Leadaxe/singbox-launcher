package state

import (
	"encoding/json"
	"strings"
)

// Authored — тело узла АВТОРСКОЕ (контракт 1.1.87, PARSING_PRINCIPLES
// §10.1; решение владельца 27.09.2026, LxBox §577).
//
// Условий четыре, и первое — контейнер — вызывающий обязан проверить сам:
// звать только у своего сервера в корне и у члена папки, не у узла
// подписки. Здесь три остальных: узел — сервер (не группа автовыбора),
// источник — голое тело узла sing-box (вид `singbox_outbound`), и текст
// источника разбирается как JSON-объект.
//
// Условием НЕ являются: кто набрал текст, вид хранения `json`, факт правки.
func (n *Node) Authored() bool {
	if n == nil || n.Kind != SourceKindServer || n.Origin == nil || n.Origin.Kind != OriginKindJSON {
		return false
	}
	return IsBareNodeBody(n.Origin.Raw)
}

// IsBareNodeBody — текст является голым телом узла sing-box (вид источника
// `singbox_outbound`): JSON-объект со строковым непустым `type` и без
// `outbounds`/`endpoints` в корне (это уже документ, `singbox_config`).
func IsBareNodeBody(raw string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &m); err != nil || m == nil {
		return false
	}
	if _, doc := m["outbounds"]; doc {
		return false
	}
	if _, doc := m["endpoints"]; doc {
		return false
	}
	var typ string
	if err := json.Unmarshal(m["type"], &typ); err != nil {
		return false
	}
	return strings.TrimSpace(typ) != ""
}

// NormalizeBareBodyOrigins — старые записи своего сервера и члена папки, у
// которых в источнике лежит документ или массив тел (виды `singbox_config`,
// `singbox_config_array`, `singbox_outbound_array`), получают в источник
// ТЕЛО узла этой записи (контракт 1.1.87, PARSING_PRINCIPLES §11 п.2; LxBox
// §576). Тело, уходившее в ядро, не меняется — меняется только обёртка.
//
// Возвращает число переписанных записей: вызывающему нужно знать, стоит ли
// сохранять файл.
func NormalizeBareBodyOrigins(s *State) int {
	if s == nil {
		return 0
	}
	n := 0
	for i := range s.Sources {
		src := &s.Sources[i]
		switch src.Kind {
		case SourceKindServer:
			if normalizeBareBodyOrigin(&src.Node) {
				n++
			}
		case SourceKindFolder:
			for j := range src.Nodes {
				if normalizeBareBodyOrigin(&src.Nodes[j]) {
					n++
				}
			}
		}
	}
	return n
}

func normalizeBareBodyOrigin(node *Node) bool {
	if node == nil || node.Kind != SourceKindServer || node.Origin == nil || node.Origin.Kind != OriginKindJSON || len(node.Body) == 0 {
		return false
	}
	raw := strings.TrimSpace(node.Origin.Raw)
	if IsBareNodeBody(raw) || !isNodeDocumentOrArray(raw) {
		return false
	}
	var body interface{}
	if err := json.Unmarshal(node.Body, &body); err != nil {
		return false
	}
	pretty, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return false
	}
	node.Origin.Raw = string(pretty)
	return true
}

// isNodeDocumentOrArray — текст является документом sing-box (объект с
// `outbounds`/`endpoints`) или JSON-массивом.
func isNodeDocumentOrArray(raw string) bool {
	if strings.HasPrefix(raw, "[") {
		var arr []json.RawMessage
		return json.Unmarshal([]byte(raw), &arr) == nil
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &m) != nil {
		return false
	}
	_, ob := m["outbounds"]
	_, ep := m["endpoints"]
	return ob || ep
}
