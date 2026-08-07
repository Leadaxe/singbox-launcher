package subscription

import (
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// SPEC 094 A5 — импорт selector/urltest как ОБЫЧНЫХ УЗЛОВ списка.
//
// Группа из чужого конфига становится ParsedNode со схемой SchemeGroup, а не
// записью во вкладке Outbounds. Разграничение принципиальное:
//
//   - вкладка Outbounds — это КАНАЛЫ лаунчера: на них ссылаются правила
//     роутинга, пользователь настраивает их состав, они значимая сущность
//     конфигуратора;
//   - импортированная группа — рядовой узел без привилегий: её нет во вкладке
//     Outbounds, на неё не ссылается роутинг, пользователь её не редактирует.
//     В списке она выглядит как ещё одна нода.
//
// Для sing-box она при этом остаётся настоящим selector/urltest внутри массива
// outbounds — эмиссией занимается generateGroupNodeJSON.
//
// Отсюда же следует, что механика Ref/Updates из SPEC 057/058 здесь не
// применяется вовсе: она про биндинг каналов к template/preset.

// singboxGroupOptionKeys — поля группы, переносимые в Options как есть.
//
// Список закрытый: незнакомое поле, попавшее в конфиг, ядро отвергает целиком
// (unknown field), поэтому «прокинуть всё подряд» здесь небезопасно.
var singboxGroupOptionKeys = []string{
	"url",
	"interval",
	"tolerance",
	"idle_timeout",
	"interrupt_exist_connections",
}

// singboxGroupToNode конвертирует selector/urltest в узел списка (SchemeGroup).
//
// nodeByTag сопоставляет сырой тег исходного конфига импортированному узлу:
// состав резолвится по фактически импортированным узлам, а не по тегам из
// файла, иначе ссылки на отброшенные/переименованные узлы дали бы группу,
// указывающую в пустоту.
//
// Возвращает false, если группу эмитить нельзя.
func singboxGroupToNode(
	entry map[string]interface{},
	nodeByTag map[string]*configtypes.ParsedNode,
) (*configtypes.ParsedNode, bool) {
	groupType := strings.ToLower(strings.TrimSpace(mapString(entry, "type")))
	tag := strings.TrimSpace(mapString(entry, "tag"))
	if tag == "" {
		debuglog.WarnLog("Parser: singbox import: %s group without tag — skipped", groupType)
		return nil, false
	}

	membersRaw, _ := entry["outbounds"].([]interface{})
	members := make([]string, 0, len(membersRaw))
	seen := make(map[string]struct{}, len(membersRaw))
	lost := 0

	for _, raw := range membersRaw {
		memberTag, ok := raw.(string)
		if !ok {
			lost++
			continue
		}
		memberTag = strings.TrimSpace(memberTag)
		if memberTag == "" {
			lost++
			continue
		}
		node, ok := nodeByTag[memberTag]
		if !ok || node == nil {
			// Вложенная группа, служебный тип, отброшенный или битый узел.
			lost++
			continue
		}
		if _, dup := seen[node.Tag]; dup {
			continue
		}
		seen[node.Tag] = struct{}{}
		members = append(members, node.Tag)
	}

	if lost > 0 {
		debuglog.WarnLog("Parser: singbox import: group %q: %d member(s) could not be resolved", tag, lost)
	}

	if len(members) == 0 {
		// Пустой urltest роняет старт ядра — не эмитим вовсе (A5).
		debuglog.WarnLog("Parser: singbox import: group %q has no resolvable members — skipped", tag)
		return nil, false
	}

	// Состав хранится как []interface{} — та же форма, в которой он приходит
	// из JSON и в которой переживает round-trip через state.json.
	memberList := make([]interface{}, 0, len(members))
	for _, m := range members {
		memberList = append(memberList, m)
	}

	outbound := map[string]interface{}{
		"tag":                    tag,
		"type":                   groupType,
		configtypes.GroupMembersKey: memberList,
	}

	for _, key := range singboxGroupOptionKeys {
		if v, ok := entry[key]; ok && v != nil {
			outbound[key] = v
		}
	}

	// default валиден только если он реально входит в состав: ядро отвергает
	// селектор, чей default не найден среди outbounds.
	if def := strings.TrimSpace(mapString(entry, "default")); def != "" {
		if node, ok := nodeByTag[def]; ok && node != nil {
			if _, inMembers := seen[node.Tag]; inMembers {
				outbound["default"] = node.Tag
			}
		}
	}

	return &configtypes.ParsedNode{
		Tag:    tag,
		Scheme: configtypes.SchemeGroup,
		// server/server_port у группы нет — она не соединение.
		Label:       tag,
		Outbound:    outbound,
		SourceIndex: configtypes.UnsetSourceIndex,
	}, true
}
