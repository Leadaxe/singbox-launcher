// File server_conn_key.go — подпись содержимого и дедуп записей источника.
//
// SPEC 112-B часть A, SPEC 113-A §2. Это НЕ идентичность узла (та с SPEC 112 —
// тег, config.NodeIdentity) и не отметка выключения: подпись живёт ровно один
// разбор источника, в состояние не пишется и ни на что пользовательское не
// влияет. Она отвечает на единственный вопрос: «эта запись подписки —
// байтовый повтор уже принятой?».
package subscription

import (
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// dedupSignature — подпись «та же запись»: полная эмиссия узла без tag/detour
// (LegacyNodeIdentityHashFunc). ЕДИНСТВЕННЫЙ ключ дедупа во всём парсере
// (SPEC 113, решение 1) — и для записей источника, и для xray-ownership.
//
// Вердикт пользователя (SPEC 112-B, уточнение 26.08): разные транспорты
// одного сервера с одним креденшлом (grpc/xhttp, разные SNI) — это разные
// соединения и разные схемы обхода блокировок, их НЕ схлопывать. Ключ по
// кредам (`схема|сервер|порт|креденшл`) склеивал grpc-вариант узла с его
// xhttp-вариантом и потому упразднён совсем (SPEC 113-A §2).
//
// Зависимость подписи от эмиттера здесь безвредна: она живёт один разбор
// одного тела — обе копии эмитятся одним кодом из одной формы.
//
// Пустая строка — «подписи нет» (группа, эмиссия не удалась, хук не
// установлен): такие записи не схлопываются никогда, иначе все безымянные
// сложились бы в одну.
func dedupSignature(node *configtypes.ParsedNode) string {
	if node == nil || node.Scheme == configtypes.SchemeGroup {
		return ""
	}
	if LegacyNodeIdentityHashFunc == nil {
		return ""
	}
	return LegacyNodeIdentityHashFunc(node)
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
	seen map[string]string // подпись содержимого → тег ПЕРВОЙ принятой записи
	// collapsedInto — исходный тег выброшенного дубля → исходный тег
	// выжившего. Нужен узлам-группам (SPEC 113-A §4, находка аудита M1):
	// группа перечисляет членов исходными тегами, и член, схлопнутый дедупом,
	// иначе просто выпадал из состава — а группа, потерявшая ВСЕХ членов,
	// удалялась целиком.
	collapsedInto map[string]string
	dropped       int
}

func newSourceDedup() *sourceDedup {
	return &sourceDedup{
		seen:          make(map[string]string),
		collapsedInto: make(map[string]string),
	}
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
		if d.collapsedInto != nil && node.Tag != "" {
			d.collapsedInto[node.Tag] = firstTag
		}
		return false
	}
	d.seen[key] = node.Tag
	return true
}

// collapsedTags — карта «тег выброшенного дубля → тег выжившего» в ИСХОДНЫХ
// тегах источника. Пустая карта, если схлопывать было нечего.
func (d *sourceDedup) collapsedTags() map[string]string {
	if d == nil {
		return nil
	}
	return d.collapsedInto
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
	// Схлопнутый член группы перепривязывается на выжившую копию (SPEC 113-A
	// §4). Без этого превью показывало бы группу, ссылающуюся в пустоту, или
	// вовсе теряло её — а боевой разбор группу сохраняет.
	return rebindCollapsedGroupMembers(kept, d.collapsedTags())
}

// rebindCollapsedGroupMembers переписывает состав узлов-групп со схлопнутых
// копий на выживших. Работает с ТЕКУЩИМИ тегами (превью тегов не переставляет),
// в отличие от rebindImportedGroupNodes, который идёт от SourceTag к итоговому.
func rebindCollapsedGroupMembers(
	nodes []*configtypes.ParsedNode,
	collapsedInto map[string]string,
) []*configtypes.ParsedNode {
	if len(collapsedInto) == 0 {
		return nodes
	}
	alive := make(map[string]struct{}, len(nodes))
	for _, n := range nodes {
		if n != nil {
			alive[n.Tag] = struct{}{}
		}
	}
	for _, n := range nodes {
		if n == nil || n.Scheme != configtypes.SchemeGroup || n.Outbound == nil {
			continue
		}
		raw, ok := n.Outbound[configtypes.GroupMembersKey].([]interface{})
		if !ok {
			continue
		}
		members := make([]interface{}, 0, len(raw))
		seen := make(map[string]struct{}, len(raw))
		for _, item := range raw {
			tag, ok := item.(string)
			if !ok {
				continue
			}
			if _, live := alive[tag]; !live {
				if survivor, collapsed := collapsedInto[tag]; collapsed {
					tag = survivor
				}
			}
			if _, dup := seen[tag]; dup {
				continue
			}
			seen[tag] = struct{}{}
			members = append(members, tag)
		}
		n.Outbound[configtypes.GroupMembersKey] = members
	}
	return nodes
}
