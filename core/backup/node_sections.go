// File node_sections.go — секции узла на входе бэкапа (SPEC 121 §10.5).
//
// Поле `servers[].sections` едет в файле непрозрачным блоком: разбирает его
// состояние, а не контракт (типизация потеряла бы незнакомое поле молча — у
// DNS-сервера tailscale это `endpoint`).
//
// Форма одна (§10.1) — записи лаунчера: `rules[]` видов inline|srs со своими
// `enabled`/`num` и `dns{servers,rules}` вида user.
//
// SPEC 127 (state v8): внутри блока теперь целевая форма — `num`, `name`,
// `refs[]` на уровне записи и `body` = объект sing-box. Файл 0.12 на это не
// рассчитан (его схема обещает `sections.rules[]` в форме `rules[]` бэкапа),
// но блок и сегодня едет непрозрачным проносом, поэтому расхождение
// формальное: читает его тот же `state.NodeSections`, что и писал. Волна 2
// (бэкап 1.0) делает это расхождение осознанным, приводя схему к v8.
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
