// File server_conn_key.go — ключ «то же подключение» и дедуп записей источника.
//
// SPEC 112-B часть A. Это НЕ идентичность узла (та с SPEC 112 — тег,
// config.NodeIdentity) и не отметка выключения: ключ живёт ровно один разбор
// источника, в состояние не пишется и ни на что пользовательское не влияет.
// Он отвечает на единственный вопрос: «эта запись подписки — байтовый повтор
// уже принятой?».
package subscription

import (
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// serverConnKey — `схема|сервер|порт|креденшл`.
//
// Обобщение xrayServerKey (тот теперь зовёт эту функцию): семейство одно, и
// оно же у LxBox nodeIdentityKey — контракт IDENTITY.md §4.
//
// Транспорт, TLS и SNI в ключ НЕ входят — решение пользователя (SPEC 112-B):
// один сервер с одним паролем под разными SNI считается одной записью, как на
// мобиле. Прежний контент-хеш держал такие записи раздельно; семантики двух
// приложений сходятся намеренно.
//
// Пустая строка — «подключение не определено» (нет сервера или креденшла, либо
// это узел-группа): такие записи не схлопываются никогда, иначе все безымянные
// сложились бы в одну.
func serverConnKey(node *configtypes.ParsedNode) string {
	// requireCred=true: ключ без креденшла — это «любой узел на этом адресе»,
	// под него попали бы разные аккаунты одного шлюза. Не схлопываем.
	return buildServerConnKey(node, true)
}

// buildServerConnKey — общее тело ключа. requireCred разводит двух
// потребителей: дедуп записей (нужен креденшл, иначе не схлопываем) и
// xray-ownership, где безымянный по секрету сервер всё равно закрепляется за
// элементом — там ключ решает «чей адрес», а не «та же ли это запись».
func buildServerConnKey(node *configtypes.ParsedNode, requireCred bool) string {
	if node == nil || node.Scheme == configtypes.SchemeGroup {
		return ""
	}
	server := strings.TrimSpace(node.Server)
	if server == "" {
		return ""
	}
	cred := strings.TrimSpace(node.UUID)
	if cred == "" && node.Outbound != nil {
		// ss/trojan/hy2 несут секрет не в UUID — берём первое, что есть.
		for _, field := range []string{"password", "uuid", "private_key", "auth_str"} {
			if v, ok := node.Outbound[field].(string); ok && strings.TrimSpace(v) != "" {
				cred = strings.TrimSpace(v)
				break
			}
		}
	}
	if cred == "" && requireCred {
		return ""
	}
	return fmt.Sprintf("%s|%s|%d|%s", strings.ToLower(node.Scheme), server, node.Port, cred)
}

// sourceDedup — состояние per-source дедупа записей.
//
// Заводится ОДИН на разбор источника и опрашивается ДО простановки тегов
// (SPEC 094 D3): пропусти проверку — и дубль сначала получит уникализованный
// тег «X-2», а значит и чужую идентичность, и снять его отметку пользователь
// уже не сможет.
//
// Нулевое значение непригодно — заводить через newSourceDedup.
type sourceDedup struct {
	seen    map[string]string // ключ подключения → тег ПЕРВОЙ принятой записи
	dropped int
}

func newSourceDedup() *sourceDedup {
	return &sourceDedup{seen: make(map[string]string)}
}

// accept решает, брать ли запись. false — это байтовый повтор уже принятой.
//
// Остаётся ПЕРВАЯ запись: её подпись пользователь и увидит («Хорватия» из
// darkline-подписки, а не «LTE 7»).
func (d *sourceDedup) accept(node *configtypes.ParsedNode) bool {
	if d == nil || node == nil {
		return true
	}
	key := serverConnKey(node)
	if key == "" {
		return true
	}
	if firstTag, dup := d.seen[key]; dup {
		d.dropped++
		debuglog.DebugLog("Parser: duplicate entry %q collapsed into %q (same server credentials)",
			node.Tag, firstTag)
		return false
	}
	d.seen[key] = node.Tag
	return true
}

// logSummary — один INFO-итог на источник; молчит, когда схлопывать было нечего.
func (d *sourceDedup) logSummary(source string) {
	if d == nil || d.dropped == 0 {
		return
	}
	debuglog.InfoLog("Parser: %s: %d duplicate entries collapsed", sourceLogName(source), d.dropped)
}

// sourceLogName — как источник зовут в логе дедупа. Пустой Source (прямые
// ссылки, ручной JSON) — «source».
func sourceLogName(source string) string {
	if s := strings.TrimSpace(source); s != "" {
		return s
	}
	return "source"
}

// dedupParsedNodes схлопывает готовый список записей по ключу подключения,
// сохраняя порядок.
//
// Боевой путь его НЕ зовёт: там дедуп идёт по одной записи (sourceDedup.accept)
// строго ДО простановки тегов, и списком его не подменить. Функция —
// единственный способ применить то же правило к уже разобранному набору;
// нужна тем, кто получил узлы, а не тело источника. Превью к таким не
// относится: оно ходит тем же LoadNodesFromSource, что и сборка, и дедуп у
// него уже применён (иначе счётчик на строке источника разошёлся бы со
// сборкой — память lazy-cache-vs-lost-state).
func dedupParsedNodes(nodes []*configtypes.ParsedNode) []*configtypes.ParsedNode {
	if len(nodes) < 2 {
		return nodes
	}
	d := newSourceDedup()
	kept := make([]*configtypes.ParsedNode, 0, len(nodes))
	for _, n := range nodes {
		if !d.accept(n) {
			continue
		}
		kept = append(kept, n)
	}
	return kept
}
