// File canonical_emit.go — сборка узлов ИЗ материализованных nodes[]
// (SPEC 118 W4, Т5; PLAN §4).
//
// # Что изменилось
//
// До W4 конвейер сборки на каждом билде заново разбирал тела подписок
// (LoadNodesFromSourceEx: скачать/взять из raw-кэша → классифицировать →
// распарсить каждую запись → тег-политика → уникализация). С W4 тела
// разобраны ОДИН раз — при fetch (W3) или миграции (W2), — и лежат в
// `Subscription.nodes[].body`. Сборке остаётся эмиссия.
//
// # Почему тело эмитится как есть
//
// `body` — это ровно то, что эмиттер лаунчера написал бы в config.json,
// минус ключи `tag` и `detour`: их владелец — модель (тег в Node.Tag,
// маршрут в NodeLink). Значит сборке достаточно вернуть эти два ключа на
// прежние места (body_keyorder.go) — и байты совпадут со старым движком.
// Гонять body обратно через per-scheme эмиттер было бы вторым источником
// правды о форме outbound'а и разъехалось бы при первой же правке эмиттера.
//
// # Порядок тег-машины
//
// Тот же, что у старой (риск Р1, на нём держатся эталоны W2):
//
//	политика(prefix + сырой тег + postfix, переменные) →
//	NormalizeProxyDisplay → MakeTagUnique (глобальный счётчик сборки).
//
// Сырой тег при этом НЕ меняется: правка префикса папки двигает финальные
// теги и не трогает ни merge-ключи, ни отметки, ни ссылки (§4.E.1).
package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/textnorm"
)

// CanonicalEmitResult — узлы одного канонического источника плюс то, что
// эмиссия обязана рассказать пользователю.
type CanonicalEmitResult struct {
	// Nodes — узлы в порядке модели, с ФИНАЛЬНЫМИ тегами.
	Nodes []*ParsedNode
	// Warnings — эмиссионные деградации (битое тело, пустая Auto-группа,
	// снятый default). Текут в отчёт сборки.
	Warnings []string
}

// EmitCanonicalSource строит узлы источника из канона v7.
//
// tagCounts — ГЛОБАЛЬНЫЙ счётчик уникализации финальных тегов на всю сборку
// (тот же, что у старого пути): теги обязаны быть уникальны между
// источниками, и порядок обхода источников определяет, кто получит «-2».
//
// Выключенные узлы (`enabled=false`) в конфиг не идут — их роль прежней
// disabled-карты. Ссылки NodeLink здесь НЕ резолвятся: их разрешает единый
// резолв на проходе 2 (nodelink_resolve.go), когда финальные теги известны у
// всех источников.
//
// # Выключенный узел ПРОХОДИТ тег-машину и отбрасывается ПОСЛЕ
//
// Старый движок фильтровал выключенные узлы в самом конце разбора
// (отбор шёл уже после простановки тегов), поэтому выключенный
// узел успевал потребить и номер `{$num}`, и слот глобальной уникализации:
// в подписке [A, выключенный B, B] третий узел получал финальный тег `B-2`.
// Выброси его РАНЬШЕ машины — и сосед переехал бы на `B`, то есть у половины
// пользователей сменились бы конфиговые имена: протухли бы выборы в cache.db
// ядра и ссылки, адресующие финальный тег. Поэтому отбор здесь идёт последним
// действием цикла, а не первым.
func EmitCanonicalSource(ps ProxySource, sourceIndex int, tagCounts map[string]int) *CanonicalEmitResult {
	res := &CanonicalEmitResult{}
	cs := ps.Canonical
	if cs == nil {
		return res
	}

	// Номер узла ({$num}) считается по всем узлам, ПРОШЕДШИМ тег-машину, —
	// включая выключенные и исключая несобравшиеся. Ровно так рос
	// nodesFromThisSource у старого движка: он инкрементировался после
	// удачного разбора и штампа тега, а фильтрация выключенных шла позже.
	emittedNum := 0
	for i := range cs.Nodes {
		cn := &cs.Nodes[i]
		node, err := buildCanonicalNode(cs, cn, emittedNum+1, tagCounts)
		if err == errCanonicalChainDeferred {
			// Цепочка тег-машину здесь не проходит (её тег задан прямо и
			// уникализации не подлежит) — значит и номера не потребляет.
			continue
		}
		if err != nil {
			// Битая запись — деградация записи, не источника (SPEC Т3).
			// Про выключенный узел молчим: он не «сломался», его выключили.
			if cn.Enabled {
				res.Warnings = append(res.Warnings,
					locale.Tf(emitNodeNotEmittableText, cn.Tag, err))
				debuglog.WarnLog("canonical: node %q is not emitted: %v", cn.Tag, err)
			}
			continue
		}
		emittedNum++
		if !cn.Enabled {
			// Тег-машина уже отработала (номер и слот уникализации
			// потреблены) — узел выбрасывается ровно здесь, как в старом
			// движке.
			debuglog.DebugLog("canonical: node %q is disabled — not going into the config (final tag %q consumed)", cn.Tag, node.Tag)
			continue
		}
		node.SourceIndex = sourceIndex
		applyCanonicalDetourLink(cs, cn, node)
		res.Nodes = append(res.Nodes, node)
	}
	return res
}

// canonicalSourceLabel — как назвать источник пользователю в отчёте.
func canonicalSourceLabel(ps ProxySource) string {
	if s := strings.TrimSpace(ps.Label); s != "" {
		return s
	}
	if s := strings.TrimSpace(ps.Source); s != "" {
		return s
	}
	return ps.ID
}

// applyCanonicalDetourLink навешивает узлу ссылку detour: личную, а при её
// отсутствии — ОБЩУЮ ссылку папки.
//
// Общий detour папки применяется только к Server-узлам и ПРОПУСКАЕТ Chain и
// Auto (features/directions.md §7): у selector/urltest dial-полей нет вовсе,
// а маршрут цепочки целиком выражен её позициями. Само ребро резолвится
// позже, единым резолвом (nodelink_resolve.go).
func applyCanonicalDetourLink(cs *configtypes.CanonicalSource, cn *configtypes.CanonicalNode, node *ParsedNode) {
	if cn.Detour != nil {
		node.CanonicalDetour = cn.Detour
		return
	}
	if cs.FolderDetour == nil || cn.Kind != canonicalKindServer {
		return
	}
	node.CanonicalDetour = cs.FolderDetour
}

// buildCanonicalNode — один канонический узел → ParsedNode с финальным тегом.
func buildCanonicalNode(cs *configtypes.CanonicalSource, cn *configtypes.CanonicalNode, num int, tagCounts map[string]int) (*ParsedNode, error) {
	switch cn.Kind {
	case canonicalKindServer:
		return buildCanonicalServer(cs, cn, num, tagCounts)
	case canonicalKindAuto:
		return buildCanonicalAuto(cs, cn, num, tagCounts)
	case canonicalKindChain:
		// Цепочка узлом здесь НЕ становится: её позиции ссылаются на теги,
		// окончательные только после обхода ВСЕХ источников, поэтому её
		// собирает проход 2 (ResolveChainSources) — а её ссылки NodeLink
		// резолвит ResolveCanonicalChainHops там же. Тег цепочки задаётся
		// пользователем напрямую и уникализации не подлежит.
		return nil, errCanonicalChainDeferred
	}
	return nil, fmt.Errorf("unknown node kind %q", cn.Kind)
}

// Значения state.SourceKind, продублированные строками: пакет config не
// импортирует state (цикл — см. migrate_materialize.go).
const (
	canonicalKindServer = "server"
	canonicalKindChain  = "chain"
	canonicalKindAuto   = "auto"
)

// buildCanonicalServer — server-узел: тело как есть, метаданные для фильтров
// и переменных — из тела и происхождения.
func buildCanonicalServer(cs *configtypes.CanonicalSource, cn *configtypes.CanonicalNode, num int, tagCounts map[string]int) (*ParsedNode, error) {
	if len(cn.Body) == 0 {
		return nil, fmt.Errorf("server node has no body")
	}
	obj, err := decodeOrderedJSONObject(cn.Body)
	if err != nil {
		return nil, err
	}
	obType := strings.ToLower(strings.TrimSpace(obj.stringValue("type")))
	if obType == "" {
		return nil, fmt.Errorf("body has no %q", "type")
	}

	// Outbound-карта нужна фильтрам Направлений, санитайзеру и проверкам
	// цепочек — они читают поля узла, а не строку. Эмиссию она НЕ определяет
	// (её определяет EmitBody), поэтому потери типов при разборе безвредны.
	var outbound map[string]interface{}
	if err := json.Unmarshal(cn.Body, &outbound); err != nil {
		return nil, fmt.Errorf("body is not a JSON object: %w", err)
	}

	scheme := canonicalSchemeFromType(obType)
	node := &ParsedNode{
		Scheme:      scheme,
		Server:      canonicalString(outbound["server"]),
		Port:        canonicalInt(outbound["server_port"]),
		Outbound:    outbound,
		EmitBody:    cn.Body,
		IdentityTag: cn.Tag,
		Service:     cn.Service,
		SourceIndex: configtypes.UnsetSourceIndex,
	}
	node.UUID = canonicalCredential(outbound, scheme)
	node.Flow = canonicalString(outbound["flow"])
	applyCanonicalDisplay(node, cn)
	node.Tag = applyEmissionTagMachine(node, cs, cn, num, tagCounts)
	return node, nil
}

// buildCanonicalAuto — провайдерская Auto-группа. Состав резолвится на
// проходе 2 (члены — NodeLink); здесь фиксируются тип, опции и финальный тег.
func buildCanonicalAuto(cs *configtypes.CanonicalSource, cn *configtypes.CanonicalNode, num int, tagCounts map[string]int) (*ParsedNode, error) {
	if cn.Group == nil {
		return nil, fmt.Errorf("auto node has no group")
	}
	groupType := strings.TrimSpace(cn.Group.GroupType)
	if groupType == "" {
		return nil, fmt.Errorf("auto node has no group type")
	}
	outbound := map[string]interface{}{"type": groupType}
	for k, v := range cn.Group.Options {
		outbound[k] = v
	}
	// Состав и default проставит резолв (проход 2); ключ заводим сразу,
	// чтобы форма узла-группы совпадала с формой импортированной группы.
	outbound[configtypes.GroupMembersKey] = []interface{}{}

	node := &ParsedNode{
		Scheme:      configtypes.SchemeGroup,
		Outbound:    outbound,
		SourceIndex: configtypes.UnsetSourceIndex,
	}
	applyCanonicalDisplay(node, cn)
	node.Tag = applyEmissionTagMachine(node, cs, cn, num, tagCounts)
	outbound["tag"] = node.Tag
	// Members/default канона — сырые теги СВОЕЙ папки; ссылка без folderId
	// у узла папки означает корень (features/directions.md §6).
	node.CanonicalGroupMembers = normalizeCanonicalLinks(cn.Group.Members, cs.FolderID)
	node.CanonicalGroupDefault = strings.TrimSpace(cn.Group.Default)
	return node, nil
}

// errCanonicalChainDeferred — узел-цепочка эмитируется не здесь, а на
// проходе 2 (ResolveChainSources): не ошибка, а перенос.
var errCanonicalChainDeferred = fmt.Errorf("chain node is built on pass 2")

// ResolveCanonicalChainHops переписывает позиции цепочек из ссылок NodeLink
// в ФИНАЛЬНЫЕ теги — проход 2, до ResolveChainSources (SPEC 118 W4, Т5).
//
// Хопы fail-closed: нерезолвнутая позиция оставляется КАК ЕСТЬ (сырым тегом),
// и ResolveChainSources роняет цепочку целиком со своей причиной — «position
// X not found among nodes and Directions». Подменять непойманную ссылку
// молча нельзя: маршрут без хопа — другой маршрут.
//
// Мутирует parserConfig — как и остальные проходы 0/2, по копии, собранной
// для генерации.
func ResolveCanonicalChainHops(parserConfig *ParserConfig, targets *NodeLinkTargets) []EmissionWarning {
	if parserConfig == nil || targets == nil {
		return nil
	}
	var warnings []EmissionWarning
	for i := range parserConfig.ParserConfig.Proxies {
		ps := &parserConfig.ParserConfig.Proxies[i]
		cs := ps.Canonical
		if cs == nil || ps.Disabled {
			continue
		}
		for ni := range cs.Nodes {
			cn := &cs.Nodes[ni]
			if cn.Kind != canonicalKindChain || !cn.Enabled || len(cn.Hops) == 0 {
				continue
			}
			hops := make([]string, 0, len(cn.Hops))
			for _, link := range cn.Hops {
				res := targets.Resolve(link)
				if res.Problem != "" {
					// Позиция помечается НЕРЕЗОЛВИМОЙ, а не оставляется сырым
					// тегом: `known` в ResolveChainSources — пространство
					// ФИНАЛЬНЫХ тегов корня, и сырой тег в нём вполне может
					// совпасть по имени с чужим узлом. Хоп {FolderID:"F1",
					// Tag:"US-1"} при выключенной папке F1 и корневом узле
					// `US-1` так и уводил цепочку через чужой сервер — молча
					// и без единого слова о подмене. Маркер `known` не
					// содержит НИКОГДА, поэтому деградация fail-closed
					// наступает независимо от совпадений имён; в тексте
					// причины маркер снимается (chainHopDisplayTag).
					hops = append(hops, markChainHopUnresolved(link.Tag))
					w := locale.Tf(emitChainHopUnresolvedText, cn.Tag, link.Tag, res.Problem)
					// Адресат — сам источник-цепочка: чинят позицию в его окне.
					warnings = append(warnings, EmissionWarning{
						Text:        w,
						SourceID:    strings.TrimSpace(ps.ID),
						SourceLabel: sourceDisplayName(*ps, i),
					})
					debuglog.WarnLog("nodelink: %s", w)
					continue
				}
				hops = append(hops, res.Tag)
			}
			// Настройки маршрута — из тела узла, позиции — свежерезолвнутые.
			ps.Chain = configtypes.ChainFromBody(cn.Body, hops)
		}
	}
	return warnings
}

// applyCanonicalDisplay восстанавливает подпись, комментарий и вход
// тег-политики.
//
// Канон подписи не хранит (второе имя разъехалось бы с тегом — SPEC 112), но
// эмиссии она нужна: `{$label}`/`{$comment}` тег-политики и фильтры
// Направлений по `label`/`fragment`/`comment`. Источник — `origin.raw`.
//
// Вход тег-политики (`node.Tag`, он же `{$tag}`) — ПРОВАЙДЕРСКИЙ тег из той
// же подписи, а НЕ сырой тег канона: сырой уже уникализирован внутри тела
// (`NL-1`, `NL-1-2` — идентичность и merge-ключ), а старая машина применяла
// политику к неуникализированному имени и уникализировала уже ФИНАЛЬНЫЙ тег
// (`[P] NL-1 •`, `[P] NL-1 •-2`). Подмени одно другим — и у половины узлов
// сменились бы конфиговые имена, протухнув в кэше ядра.
//
// Без происхождения (JSON-тело, синтезированная группа) подписью и входом
// политики служит сырой тег: он и есть лучшее известное имя.
func applyCanonicalDisplay(node *ParsedNode, cn *configtypes.CanonicalNode) {
	label := ""
	if cn.OriginKind == originKindURI {
		label = subscription.LabelFromOriginURI(cn.OriginRaw)
	}
	providerTag := ""
	if label != "" {
		providerTag = subscription.ProviderTagFromLabel(label)
	}
	if label == "" {
		label = cn.Tag
	}
	if providerTag == "" {
		providerTag = cn.Tag
	}
	node.Label = label
	node.Comment = subscription.CommentFromLabel(label)
	// Тег узла — ЕГО СОБСТВЕННОЕ имя (cn.Tag), а не то, что выведено из
	// `#fragment` исходной ссылки. Фрагмент участвует только как {$label} в
	// тег-политике и в подписи.
	//
	// Раньше здесь стоял providerTag, и у контейнера он побеждал: вход
	// тег-политики берёт node.Tag (canonicalPolicyInput), то есть уже
	// переписанное значение. Узел, перенесённый в папку, терял имя — у
	// Proton-ссылок фрагмент это IP, и «Proton NL» превращался в
	// «185.107.56.148», а ссылки на прежнее имя рвались.
	//
	// У КОРНЕВОГО узла политики нет и вход брался из cn.Tag, поэтому в корне
	// имя держалось — отсюда и «сломалось при переносе в папку».
	if strings.TrimSpace(cn.Tag) != "" {
		node.Tag = cn.Tag
	} else {
		node.Tag = providerTag
	}
}

// originKindURI — значение state.OriginKindURI (строкой: state сюда не
// импортируется).
const originKindURI = "uri"

// canonicalPolicyInput — что подставляется в тег-политику и становится
// финальным тегом у корневого узла.
//
// У КОНТЕЙНЕРА (папка/подписка) это провайдерский тег из подписи (см.
// applyCanonicalDisplay). У КОРНЕВОГО узла политики нет вовсе: его имя задал
// пользователь (Node.Tag), и подменять его фрагментом URI нельзя — узел
// переименовался бы сам собой при первой же сборке.
func canonicalPolicyInput(cs *configtypes.CanonicalSource, cn *configtypes.CanonicalNode, node *ParsedNode) string {
	if cs.IsContainer {
		return node.Tag
	}
	return cn.Tag
}

// applyEmissionTagMachine — ЭМИССИОННАЯ тег-машина (SPEC Т5).
//
// Порядок применения обязан совпадать со старой машиной
// (subscription.ApplyLegacyTagMachine) — на этом держится байт-равенство
// эталонов W2 (риск Р1): политика с переменными → NormalizeProxyDisplay →
// глобальный MakeTagUnique.
//
// У корневого узла политики нет: финальный тег = сырой (SPEC Т2). Маски в
// каноне не существует — её место занял сырой тег.
func applyEmissionTagMachine(node *ParsedNode, cs *configtypes.CanonicalSource, cn *configtypes.CanonicalNode, num int, tagCounts map[string]int) string {
	tag := canonicalPolicyInput(cs, cn, node)
	if cs.IsContainer {
		node.Tag = tag
		tag = applyCanonicalTagPolicy(node, cs.TagPrefix, cs.TagPostfix, num)
	}
	tag = textnorm.NormalizeProxyDisplay(tag)
	if tagCounts == nil {
		return tag
	}
	return subscription.MakeTagUnique(tag, tagCounts, "Parser")
}

// applyCanonicalTagPolicy — prefix + сырой тег + postfix с раскрытием
// переменных (зеркало applyTagPrefixPostfix старой машины, без mask).
func applyCanonicalTagPolicy(node *ParsedNode, prefix, postfix string, num int) string {
	tag := node.Tag
	if prefix != "" {
		tag = replaceCanonicalTagVariables(prefix, node, num) + tag
	}
	if postfix != "" {
		tag += replaceCanonicalTagVariables(postfix, node, num)
	}
	return tag
}

// replaceCanonicalTagVariables — переменные тег-политики (RECON):
// {$tag} {$scheme} {$protocol} {$server} {$port} {$label} {$comment} {$num}.
//
// Дословное зеркало subscription.replaceTagVariables: словарь один на обе
// машины, и разойтись им нельзя — иначе префикс с переменной давал бы разные
// теги до и после материализации.
func replaceCanonicalTagVariables(tpl string, node *ParsedNode, num int) string {
	r := tpl
	r = strings.ReplaceAll(r, "{$tag}", node.Tag)
	r = strings.ReplaceAll(r, "{$scheme}", node.Scheme)
	r = strings.ReplaceAll(r, "{$protocol}", node.Scheme)
	r = strings.ReplaceAll(r, "{$server}", node.Server)
	r = strings.ReplaceAll(r, "{$port}", strconv.Itoa(node.Port))
	r = strings.ReplaceAll(r, "{$label}", node.Label)
	r = strings.ReplaceAll(r, "{$comment}", node.Comment)
	r = strings.ReplaceAll(r, "{$num}", strconv.Itoa(num))
	return r
}

// normalizeCanonicalLinks — ссылки узла папки: пустой folderId внутри папки
// значит «свой контейнер», если такой узел там есть; иначе — корень. Решение
// принимает резолв, здесь ссылки лишь снабжаются контекстом контейнера.
func normalizeCanonicalLinks(links []configtypes.NodeLink, ownerFolderID string) []configtypes.NodeLink {
	if len(links) == 0 {
		return nil
	}
	out := make([]configtypes.NodeLink, 0, len(links))
	for _, l := range links {
		if l.FolderID == "" && ownerFolderID != "" {
			// Члены провайдерской группы адресуют СВОЮ подписку (fetch их
			// так и резолвит); пустой folderId у такой ссылки — форма из
			// ручной правки состояния, и трактовать её как корень значило бы
			// молча увести членство наружу.
			l.FolderID = ownerFolderID
		}
		out = append(out, l)
	}
	return out
}

// canonicalSchemeFromType — sing-box type → внутренняя схема лаунчера.
// Неизвестный тип остаётся собой: тело эмитится как есть, а схему читают
// только фильтры.
func canonicalSchemeFromType(t string) string {
	if s, ok := subscription.SchemeFromSingboxType(t); ok {
		return s
	}
	return t
}

// canonicalCredential — «главный секрет» узла в поле UUID (его читают
// фильтры и превью, эмиссия — нет).
func canonicalCredential(ob map[string]interface{}, scheme string) string {
	switch scheme {
	case "vless", "vmess", "tuic":
		return canonicalString(ob["uuid"])
	case "trojan", "hysteria2", "anytls", "ss":
		return canonicalString(ob["password"])
	case "hysteria":
		// v1 держит секрет в auth_str — см. singboxCredentialFromMap.
		return canonicalString(ob["auth_str"])
	}
	return ""
}

func canonicalString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func canonicalInt(v interface{}) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case json.Number:
		n, err := x.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	}
	return 0
}
