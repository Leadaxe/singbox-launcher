// File manual_config.go — нода из ручного config_json (Source.ConfigJSON).
//
// Server-source может нести готовый sing-box outbound/endpoint объект вместо
// share-URI: для протоколов, у которых нет URI-схемы / парсера / конвертера,
// пользователь собирает JSON руками во вкладке JSON окна Source.
//
// Принципиальное отличие от sing-box импорта подписок (singbox_import.go):
// объект НЕ прогоняется через санитайзы и НЕ ограничен известными типами.
// Это осознанный ввод пользователя — он уходит в config.json как есть
// (ParsedNode.EmitRaw), лаунчер перештамповывает только tag и detour.
// Ошибочный ввод поймает sing-box check при сборке, как и любую ручную
// правку конфига.
package subscription

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// NodeFromManualConfigJSON строит ParsedNode из ручного sing-box объекта.
//
// Требования к вводу минимальны: валидный JSON-объект с непустым "type".
// Server/Port/UUID заполняются best-effort — они нужны только UI-спискам и
// skip-фильтрам, сборка их не пересобирает.
//
// Scheme: для известных sing-box типов — каноническая схема лаунчера (та же,
// что у URI-парсера: shadowsocks→ss, …), чтобы feature-гейты (naive probe,
// wireguard→endpoints) работали и для ручных нод. Неизвестный тип остаётся
// схемой как есть — такие ноды эмитятся passthrough и ни один per-scheme
// switch их не трогает.
func NodeFromManualConfigJSON(raw []byte) (*configtypes.ParsedNode, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, fmt.Errorf("config_json is empty")
	}

	var ob map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &ob); err != nil {
		return nil, fmt.Errorf("config_json: %w", err)
	}

	entryType := strings.ToLower(strings.TrimSpace(mapString(ob, "type")))
	if entryType == "" {
		return nil, fmt.Errorf("config_json: missing \"type\"")
	}

	scheme := entryType
	if s, ok := singboxTypeToScheme(entryType); ok {
		scheme = s
	}

	// Пустой tag в config.json валит check; для server-source тег обычно
	// перештампует тег из модели, но при пустом теге нужен свой fallback.
	tag := mapString(ob, "tag")
	if tag == "" {
		tag = scheme
	}

	node := &configtypes.ParsedNode{
		Tag:         tag,
		Scheme:      scheme,
		Server:      mapString(ob, "server"),
		Port:        xrayJSONInt(ob["server_port"]),
		Label:       tag,
		Outbound:    ob,
		SourceIndex: configtypes.UnsetSourceIndex,
		EmitRaw:     true,
	}
	node.UUID = singboxCredentialFromMap(ob, scheme)
	node.Flow = mapString(ob, "flow")
	return node, nil
}
