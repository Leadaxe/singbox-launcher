// File node_sections.go — секции узла на входе бэкапа (SPEC 121 §10.5).
//
// Поле `servers[].sections` едет в файле непрозрачным блоком: разбирает его
// состояние, а не контракт (типизация потеряла бы незнакомое поле молча — у
// DNS-сервера tailscale это `endpoint`). Здесь только выбор формы.
//
// Форм две, и читаются обе:
//
//   - НОВАЯ (§10.1) — записи лаунчера: `rules[]` видов inline|srs со своими
//     `enabled`/`order_num` и `dns{servers,rules}` вида user;
//   - СТАРАЯ (волны 1–2) — сырые фрагменты sing-box `dns_servers`/`dns_rules`/
//     `rules` плюс `rule_num` — позиция единственного якоря на оси.
//
// Перевод старой формы делает ТОТ ЖЕ конвертер, что у state.json
// (state.ConvertLegacyNodeSections): файл, написанный сборкой волн 1–2, обязан
// открыться без потери эмитируемого конфига.
package backup

import (
	"encoding/json"

	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
)

// decodeBackupSections переводит блок из файла в записи состояния.
//
// nodeTag нужен только для сообщений: в самих записях он не участвует —
// ссылки на узел живут плейсхолдером `@self`.
func decodeBackupSections(sec *ServerSections, nodeTag string) *state.NodeSections {
	if sec == nil || len(sec.Raw) == 0 {
		return nil
	}
	if state.LegacyNodeSectionsShape(sec.Raw) {
		// Позиция едет внутри самого блока (`rule_num`), поэтому anchor-пара
		// здесь nil: конвертер прочитает её сам.
		converted, ok := state.ConvertLegacyNodeSections(sec.Raw, nodeTag, nil, nil)
		if !ok {
			return nil
		}
		return converted
	}
	var out state.NodeSections
	if err := json.Unmarshal(sec.Raw, &out); err != nil {
		debuglog.WarnLog("backup import: node %q carries sections that cannot be read (%v) — imported without them", nodeTag, err)
		return nil
	}
	if out.IsEmpty() {
		return nil
	}
	return &out
}
