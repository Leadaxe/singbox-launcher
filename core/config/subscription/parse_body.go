// File parse_body.go — ЧИСТЫЙ парсер тела подписки (SPEC 118, PLAN §3.1).
//
// Единственное место разбора тела подписки в модели v7: fetch скачал тело —
// этот парсер разбирает его один раз, результат материализуется в
// Subscription.nodes[]. Без сети, без состояния, без тег-политики.
//
// Стадии в порядке применения к каждой записи (features/sources.md):
//  1. skip[] — отсечка до рождения узла (внутри per-format парсеров, как и
//     раньше);
//  2. дедуп по подписи полной эмиссии (dedup.accept, без тега и detour) —
//     строго ДО тегов; члены групп перепривязываются на выжившего
//     (collapsedInto);
//  3. уникализация СЫРЫХ тегов внутри тела (X, X-2) — StampNodeIdentity;
//     кап capN ограничивает число ПРИНЯТЫХ узлов по ходу стадий — реальный
//     предел разбора, не бейдж; достижение = Truncated.
//
// Здесь НЕТ (переезжает в эмиссию волны W4 или умирает):
//   - тег-политики prefix/postfix/mask (эмиссия);
//   - глобального MakeTagUnique (эмиссия);
//   - ApplySourceDetour (body чист от detour — умирает);
//   - filterDisabledNodes (роль у node.enabled — умирает).
//
// Волна W2 создаёт каркас как библиотеку (им пользуется миграция v6→v7);
// fetch-сервис переключается сюда в W3.
package subscription

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/textnorm"
)

// Словарь Origin.Kind — единый со схемой v7 (state.OriginKind*): парсер
// порождает происхождение ровно в той форме, в которой оно хранится.
const (
	OriginKindURI  = state.OriginKindURI
	OriginKindJSON = state.OriginKindJSON
)

// ParsedBodyEntry — одна принятая запись тела: узел или провайдерская группа.
type ParsedBodyEntry struct {
	// RawTag — сырой тег, уникализированный в пределах тела (идентичность
	// узла в контейнере; merge-ключ волны W3).
	RawTag string
	// Num — порядковый номер принятой записи (1-based) — вход {$num}
	// эмиссионной тег-машины.
	Num int
	// Node — разобранный узел. У группы Scheme == configtypes.SchemeGroup.
	Node *configtypes.ParsedNode
	// OriginKind / OriginRaw — происхождение записи ("uri" — строка
	// URI-списка, "json" — объект sing-box/Xray-тела). Пустой kind =
	// пофрагментного происхождения нет (синтезированные группы Xray,
	// vpn://-контейнеры).
	OriginKind string
	OriginRaw  string
	// Группа: тип, default и члены ПО СЫРЫМ тегам (после дедупа и
	// перепривязки collapsedInto). У обычных узлов пусто.
	GroupType       string
	GroupDefaultRaw string
	MemberRawTags   []string
}

// ParsedBody — результат чистого разбора тела.
type ParsedBody struct {
	Entries []*ParsedBodyEntry
	// Truncated — разбор упёрся в кап: удалять «исчезнувшие» узлы при merge
	// запрещено (SPEC 113-A).
	Truncated bool
	// Warnings — per-record деградации (битые записи, потерянные
	// группы-члены, пустые группы). Персистятся в updateStatus (W3).
	Warnings []string
	// IgnoredSections — секции целого sing-box-конфига, которые импорт не
	// читает (route/dns/inbounds/...).
	IgnoredSections []string
}

// ParseSubscriptionBody разбирает ДЕКОДИРОВАННОЕ тело подписки.
//
// capN — реальный кап принятых записей (резолв «настройка подписки → дефолт
// настроек приложения» делает вызывающий); ≤0 → аварийный потолок. Сверху
// всегда клэмпится константой configtypes.MaxNodesPerSubscription.
func ParseSubscriptionBody(body []byte, skip []map[string]string, capN int) (*ParsedBody, error) {
	if capN <= 0 || capN > configtypes.MaxNodesPerSubscription {
		capN = configtypes.MaxNodesPerSubscription
	}

	res := &ParsedBody{}
	if len(body) == 0 {
		return res, fmt.Errorf("subscription: empty body")
	}

	contentStr := string(body)
	contentStr = strings.ReplaceAll(contentStr, "\r\n", "\n")
	contentStr = strings.ReplaceAll(contentStr, "\r", "\n")
	contentStr = strings.TrimSpace(contentStr)

	st := &bodyParseState{
		res:      res,
		capN:     capN,
		idCounts: make(map[string]int),
		dedup:    newSourceDedup(),
	}

	bodyKind := ClassifySubscriptionBody(contentStr)

	// vpn:// — Amnezia-профиль: все WG/AWG-контейнеры (SPEC 103 §9.B12).
	// Пофрагментного raw у контейнеров нет — origin остаётся пустым.
	if bodyKind == BodyKindVPNLink {
		vpnNodes, skippedContainers, vpnErr := ParseAmneziaVPNLinkAll(contentStr, skip)
		if vpnErr != nil {
			st.warn(fmt.Sprintf("vpn:// body rejected: %v", vpnErr))
		} else {
			if skippedContainers > 0 {
				st.warn(fmt.Sprintf("vpn:// body: %d container(s) skipped", skippedContainers))
			}
			for _, node := range vpnNodes {
				st.accept(node, "", "")
			}
		}
		st.finish()
		return res, nil
	}

	// wg-quick .conf → канонические wireguard://-URI и дальше как URI-список
	// (SPEC 103 B11): разбор INI один, в parseWireGuardURI.
	if bodyKind == BodyKindWGConf {
		wgURIs, skippedBlocks := WGConfBodyToURIs(contentStr)
		if skippedBlocks > 0 {
			st.warn(fmt.Sprintf("wg-quick body: %d block(s) skipped (no [Peer] endpoint)", skippedBlocks))
		}
		if len(wgURIs) == 0 {
			st.warn("wg-quick body: no usable [Interface] block")
		}
		contentStr = strings.Join(wgURIs, "\n")
		bodyKind = BodyKindURIList
	}

	switch {
	case bodyKind.IsSingbox():
		importRes, err := ParseSingboxBody(contentStr, bodyKind, skip)
		if err != nil {
			st.warn(fmt.Sprintf("sing-box JSON body rejected: %v", err))
			break
		}
		res.IgnoredSections = importRes.IgnoredSections
		// Per-record деградации импорта (потерянные члены групп) — в общий
		// поток warnings разбора: fetch персистит их в updateStatus (Т3).
		for _, w := range importRes.Warnings {
			st.warn(w)
		}
		for _, node := range importRes.Nodes {
			// Исходный тег нужен группам для перепривязки состава на сырые
			// теги (тот же приём, что applyTagsToSingboxNode → SourceTag).
			if node != nil && node.SourceTag == "" {
				node.SourceTag = node.Tag
			}
			st.accept(node, OriginKindJSON, marshalNodeOriginJSON(node))
		}

	case bodyKind == BodyKindXrayArray:
		arrayNodes, xrayReasons, err := ParseNodesFromXrayJSONArrayEx(contentStr, skip)
		for _, r := range xrayReasons {
			st.warn(r)
		}
		if err != nil {
			st.warn(fmt.Sprintf("Xray JSON array body rejected: %v", err))
			break
		}
		for _, node := range arrayNodes {
			if node == nil {
				continue
			}
			if node.SourceTag == "" {
				node.SourceTag = node.Tag
			}
			// Синтезированные Xray-группы происхождения не имеют
			// (origin=null — warp-К1/группы); обычные узлы — kind=json.
			originKind, originRaw := OriginKindJSON, marshalNodeOriginJSON(node)
			if node.Scheme == configtypes.SchemeGroup {
				originKind, originRaw = "", ""
			}
			st.accept(node, originKind, originRaw)
		}

	default: // URI-список
		for _, line := range strings.Split(contentStr, "\n") {
			line = NormalizeSubscriptionTextLine(line)
			if line == "" {
				continue
			}
			if st.capReached() {
				continue
			}
			node, err := ParseNode(line, skip)
			if err != nil {
				// Битая запись — деградация записи с warning, не подписки.
				st.warn(fmt.Sprintf("record rejected: %v", err))
				continue
			}
			if node == nil {
				continue // отсечено skip-фильтром
			}
			st.accept(node, OriginKindURI, line)
		}
	}

	st.finish()
	return res, nil
}

// bodyParseState — счётчики одного разбора: кап, дедуп, уникализация.
type bodyParseState struct {
	res      *ParsedBody
	capN     int
	accepted int
	skipped  int // отброшено капом
	idCounts map[string]int
	dedup    *sourceDedup
}

func (st *bodyParseState) warn(msg string) {
	st.res.Warnings = append(st.res.Warnings, msg)
}

func (st *bodyParseState) capReached() bool {
	if st.accepted >= st.capN {
		st.skipped++
		return true
	}
	return false
}

// accept проводит запись через кап → дедуп → уникализацию сырого тега и
// кладёт её в результат. Порядок стадий обязателен (риск Р6): дедуп ДО
// тегов, иначе дубль получил бы уникализованный тег и собственную
// идентичность.
func (st *bodyParseState) accept(node *configtypes.ParsedNode, originKind, originRaw string) {
	if node == nil {
		return
	}
	if st.accepted >= st.capN {
		st.skipped++
		return
	}
	if !st.dedup.accept(node) {
		return
	}

	entry := &ParsedBodyEntry{
		Node:       node,
		OriginKind: originKind,
		OriginRaw:  originRaw,
	}
	if node.Scheme == configtypes.SchemeGroup {
		// Группы идентичности SPEC 112 не имеют, но сырой тег в контейнере
		// v7 обязан быть уникален среди ВСЕХ узлов — уникализируем той же
		// машиной; группа без тега не рождается (features/sources.md).
		raw := strings.TrimSpace(node.Tag)
		if raw == "" {
			st.warn("group without tag skipped")
			return
		}
		entry.RawTag = makeIdentityUnique(raw, st.idCounts)
		if t, ok := node.Outbound["type"].(string); ok {
			entry.GroupType = t
		}
	} else {
		entry.RawTag = StampNodeIdentity(node, st.idCounts)
		if entry.RawTag == "" {
			// Узел без тега: сырой тег — идентичность, без неё узлу не жить
			// в nodes[] (merge-ключа нет). Деградация записи.
			st.warn(fmt.Sprintf("record without tag skipped (%s://%s)", node.Scheme, node.Server))
			return
		}
		if len(node.Chain) > 0 {
			// Xray Jump / вложенная цепочка провайдера: перенос в модель v7
			// делает эмиттер/резолв (W4); каркас W2 фиксирует факт, чтобы
			// потеря не была молчаливой.
			st.warn(fmt.Sprintf("node %q carries provider chain hops — materialized without them (W4)", entry.RawTag))
		}
	}

	st.accepted++
	entry.Num = st.accepted
	st.res.Entries = append(st.res.Entries, entry)
}

// finish дорезолвливает группы (члены → сырые теги выживших) и ставит
// Truncated. Группа, потерявшая всех членов, выбрасывается: пустой urltest
// роняет старт ядра.
func (st *bodyParseState) finish() {
	if st.skipped > 0 {
		st.res.Truncated = true
		st.warn(fmt.Sprintf("body truncated: %d record(s) beyond the cap of %d", st.skipped, st.capN))
	}

	// Исходный тег (SourceTag) → сырой тег принятой записи.
	rawByOriginal := make(map[string]string, len(st.res.Entries))
	for _, e := range st.res.Entries {
		if e.Node == nil {
			continue
		}
		orig := e.Node.SourceTag
		if orig == "" {
			orig = strings.TrimSpace(e.Node.Tag)
		}
		if orig == "" {
			continue
		}
		if _, taken := rawByOriginal[orig]; !taken {
			rawByOriginal[orig] = e.RawTag
		}
	}
	collapsed := st.dedup.collapsedTags()
	resolveMember := func(memberTag string) (string, bool) {
		if raw, ok := rawByOriginal[memberTag]; ok {
			return raw, true
		}
		if survivor, ok := collapsed[memberTag]; ok {
			if raw, ok := rawByOriginal[survivor]; ok {
				return raw, true
			}
		}
		return "", false
	}

	kept := st.res.Entries[:0]
	for _, e := range st.res.Entries {
		if e.Node == nil || e.Node.Scheme != configtypes.SchemeGroup {
			kept = append(kept, e)
			continue
		}
		members := make([]string, 0)
		seen := make(map[string]struct{})
		if rawMembers, ok := e.Node.Outbound[configtypes.GroupMembersKey].([]interface{}); ok {
			for _, item := range rawMembers {
				memberTag, ok := item.(string)
				if !ok {
					continue
				}
				raw, found := resolveMember(memberTag)
				if !found {
					// Вложенная группа-член или потерянный узел — потеря с
					// warning, не молча (SPEC Т3).
					st.warn(fmt.Sprintf("group %q: member %q not resolvable — dropped", e.RawTag, memberTag))
					continue
				}
				if _, dup := seen[raw]; dup {
					continue
				}
				seen[raw] = struct{}{}
				members = append(members, raw)
			}
		}
		if len(members) == 0 {
			st.warn(fmt.Sprintf("group %q lost all members — dropped", e.RawTag))
			continue
		}
		e.MemberRawTags = members
		if def, ok := e.Node.Outbound["default"].(string); ok && def != "" {
			if raw, found := resolveMember(def); found {
				e.GroupDefaultRaw = raw
			} else {
				st.warn(fmt.Sprintf("group %q: default %q not in members — dropped", e.RawTag, def))
			}
		}
		kept = append(kept, e)
	}
	st.res.Entries = kept

	st.dedup.logSummary("(body)")
	debuglog.DebugLog("ParseSubscriptionBody: %d entr(ies), truncated=%v, %d warning(s)",
		len(st.res.Entries), st.res.Truncated, len(st.res.Warnings))
}

// ApplyLegacyTagMachine — СТАРАЯ тег-машина одним вызовом: политика
// prefix/postfix/mask с переменными → нормализация → глобальная
// уникализация.
//
// Экспортирована для миграции v6→v7 (шаги 4–5 строят индекс «финальный тег →
// (folderId, сырой тег)» по старой машине — только так строковые хопы и
// detour-ссылки резолвятся честно, PLAN §5). Эмиссионная тег-машина W4
// воспроизводит тот же порядок применения — на ней держится
// байт-эквивалентность эталонов.
func ApplyLegacyTagMachine(node *configtypes.ParsedNode, prefix, postfix, mask string, nodeNum int, tagCounts map[string]int) string {
	if node == nil {
		return ""
	}
	tag := applyTagPrefixPostfix(node, prefix, postfix, mask, nodeNum)
	tag = textnorm.NormalizeProxyDisplay(tag)
	return MakeTagUnique(tag, tagCounts, "Migration")
}

// marshalNodeOriginJSON — диагностическое происхождение записи JSON-тела:
// канонический маршал Outbound-карты (encoding/json сортирует ключи карт —
// стабильно между запусками). Не «сырое тело провайдера» — его пофрагментно
// не существует; этого достаточно для показа per-node raw в Overview.
func marshalNodeOriginJSON(node *configtypes.ParsedNode) string {
	if node == nil || node.Outbound == nil {
		return ""
	}
	b, err := json.Marshal(node.Outbound)
	if err != nil {
		return ""
	}
	return string(b)
}
