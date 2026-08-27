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

// dedupSignature — подпись «та же запись» для дедупа: полная эмиссия узла
// без tag/detour (LegacyNodeIdentityHashFunc), НЕ ключ подключения.
//
// Вердикт пользователя (SPEC 112-B, уточнение 26.08): разные транспорты
// одного сервера с одним креденшлом (grpc/xhttp, разные SNI) — это разные
// соединения и разные схемы обхода блокировок, их НЕ схлопывать. Первая
// редакция дедупила по кредам (`схема|сервер|порт|креденшл`) и склеила
// grpc-вариант узла с его xhttp-вариантом — откатились к семантике v1.5.1: схлопывается только
// байтовый повтор записи (различие лишь в #подписи). Ключ по кредам остался
// у единственного потребителя — xray-ownership (buildServerConnKey ниже).
//
// Зависимость подписи от эмиттера здесь безвредна: она живёт один разбор
// одного тела — обе копии эмитятся одним кодом из одной формы.
//
// Пустая строка — «подписи нет» (группа, эмиссия не удалась): такие записи
// не схлопываются никогда, иначе все безымянные сложились бы в одну.
func dedupSignature(node *configtypes.ParsedNode) string {
	if node == nil || node.Scheme == configtypes.SchemeGroup {
		return ""
	}
	if LegacyNodeIdentityHashFunc == nil {
		return ""
	}
	return LegacyNodeIdentityHashFunc(node)
}

// buildServerConnKey — ключ подключения `схема|сервер|порт|креденшл`.
// Единственный потребитель — xray-ownership (xrayServerKey): там ключ решает
// «чей адрес», а не «та же ли это запись», поэтому безымянный по секрету
// сервер тоже получает ключ (пустой креденшл в хвосте). Дедуп записей им НЕ
// пользуется — см. dedupSignature (вердикт про транспорты).
func buildServerConnKey(node *configtypes.ParsedNode) string {
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
	seen    map[string]string // подпись содержимого → тег ПЕРВОЙ принятой записи
	dropped int
}

func newSourceDedup() *sourceDedup {
	return &sourceDedup{seen: make(map[string]string)}
}

// accept решает, брать ли запись. false — это байтовый повтор уже принятой.
//
// Остаётся ПЕРВАЯ запись: её подпись пользователь и увидит (первое имя
// провайдера, а не последнее из хвоста дублей).
func (d *sourceDedup) accept(node *configtypes.ParsedNode) bool {
	if d == nil || node == nil {
		return true
	}
	key := dedupSignature(node)
	if key == "" {
		return true
	}
	if firstTag, dup := d.seen[key]; dup {
		d.dropped++
		debuglog.DebugLog("Parser: duplicate entry %q collapsed into %q (identical content)",
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

// DedupParsedNodes схлопывает готовый список записей по подписи содержимого,
// сохраняя порядок.
//
// Боевой путь его НЕ зовёт: там дедуп идёт по одной записи (sourceDedup.accept)
// строго ДО простановки тегов, и списком его не подменить. Функция для тех,
// кто получил узлы, а не тело источника: вкладка Preview окна источника
// (parsePreviewNodesFromBody) парсит тело своим путём и обязана показывать
// то же, что соберётся, — иначе пользователь видит 39 строк там, где в
// конфиг уедет 8 (превью ≡ боевой разбор).
func DedupParsedNodes(nodes []*configtypes.ParsedNode) []*configtypes.ParsedNode {
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
