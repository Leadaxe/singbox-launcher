// File masque_uri_migration.go — разовая миграция masque-URI со старым
// именем параметра версии HTTP.
//
// До контракта 0.8.0 диалог WARP писал версию HTTP как `?network=h2|h3`, а
// с D-078 парсер этот алиас не читает вовсе: узел молча собирается с
// парсерным дефолтом vhttp=h3. Источники, созданные до 25.08.2026, поэтому
// теряли выбранный h2 при каждой сборке (Home/IRA/Parnas: WARP (MASQUE) с
// network=h2 → vhttp: h3 в config.json).
//
// Чинится ХРАНИМЫЙ URI, а не парсер: директива D-078 остаётся в силе —
// легаси-имя по-прежнему не принимается, просто в состоянии его больше
// нет. Правится только форма (`network=` → `vhttp=`), значение и порядок
// остальных параметров — байт в байт.
package state

import (
	"strings"

	"singbox-launcher/internal/debuglog"
)

// masqueVHTTPValues — значения, которые ядро принимает в vhttp (option/masque.go
// форка: enum h3,h2,auto). Иное в `network=` — не наш старый диалог, URI не
// трогаем.
var masqueVHTTPValues = map[string]bool{"h2": true, "h3": true, "auto": true}

// migrateMasqueNetworkParam переписывает `network=<v>` в `vhttp=<v>` у
// masque-URI, если `vhttp=` в запросе ещё нет. Возвращает (новый URI, true)
// при изменении и (исходный, false) иначе.
func migrateMasqueNetworkParam(raw string) (string, bool) {
	if !strings.HasPrefix(strings.ToLower(raw), "masque://") {
		return raw, false
	}
	head, frag := raw, ""
	if i := strings.Index(raw, "#"); i >= 0 {
		head, frag = raw[:i], raw[i:]
	}
	qi := strings.Index(head, "?")
	if qi < 0 {
		return raw, false
	}
	parts := strings.Split(head[qi+1:], "&")
	networkIdx := -1
	for i, p := range parts {
		switch {
		case strings.HasPrefix(p, "vhttp="):
			// Каноническое имя уже есть — legacy-хвост парсер игнорирует сам.
			return raw, false
		case strings.HasPrefix(p, "network=") && networkIdx < 0:
			networkIdx = i
		}
	}
	if networkIdx < 0 {
		return raw, false
	}
	value := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(parts[networkIdx], "network=")))
	if !masqueVHTTPValues[value] {
		return raw, false
	}
	parts[networkIdx] = "vhttp=" + value
	return head[:qi+1] + strings.Join(parts, "&") + frag, true
}

// rewriteLegacyMasqueURIs проходит все узлы состояния (корневые источники и
// узлы папок/подписок) и переписывает legacy-URI. Тело узла материализуется
// заново через хук парсера — в модели v7 body есть канон, и оставить его от
// старого разбора значило бы сохранить ту самую потерю h2. Без хука
// (изолированные тесты state) переписывается только origin.
//
// Возвращает число переписанных узлов.
func rewriteLegacyMasqueURIs(s *State) int {
	if s == nil {
		return 0
	}
	n := 0
	for i := range s.Sources {
		n += rewriteNodeMasqueURI(&s.Sources[i].Node)
		for j := range s.Sources[i].Nodes {
			n += rewriteNodeMasqueURI(&s.Sources[i].Nodes[j])
		}
	}
	return n
}

func rewriteNodeMasqueURI(node *Node) int {
	if node == nil || node.Origin == nil || node.Origin.Kind != OriginKindURI {
		return 0
	}
	fixed, changed := migrateMasqueNetworkParam(node.Origin.Raw)
	if !changed {
		return 0
	}
	if migrationHooks.MaterializeServer != nil && node.Kind == SourceKindServer {
		res, err := migrationHooks.MaterializeServer(MigrationServerRequest{URI: fixed})
		if err != nil {
			// Разбор нового URI не удался — origin и body оставляем парой
			// от старого разбора, чтобы не разводить их между собой.
			debuglog.WarnLog("state: masque node %q: legacy network= kept, re-parse failed: %v", node.Tag, err)
			return 0
		}
		node.Body = res.Body
	}
	node.Origin.Raw = fixed
	debuglog.InfoLog("state: masque node %q: legacy ?network= rewritten to ?vhttp= (contract 0.8.0)", node.Tag)
	return 1
}
