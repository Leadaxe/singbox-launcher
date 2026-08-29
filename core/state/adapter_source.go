// File adapter_source.go — МОСТ СБОРКИ: проекция canonical v7 Source →
// legacy configtypes.ProxySource.
//
// TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5 (PLAN §6): пока
// build-пути core (config_service.go, rebuild.go) и парсер подписок ходят
// через legacy-форму ProxySource, единственное место деривации легаси-полей
// из v7-канона — здесь. Правила:
//
//   - что уже переехало в канон (kind, tag_policy, canonical Node.Tag,
//     replace, NodeLink-detour, enabled узлов) — деривируется ИЗ канона;
//   - что ещё не мигрировано (W2) — едет из мостовых легаси-полей Source
//     как есть, поэтому поведение сборки на перенесённых v6-состояниях
//     не меняется вовсе.
package state

import (
	"time"

	"singbox-launcher/core/config/configtypes"
)

// nowUnixForBridge — время отметки для деривированных disabled-ключей.
// Переопределяемо тестами моста (детерминизм).
var nowUnixForBridge = func() int64 { return time.Now().Unix() }

// ToProxySourceV4 — конвертит Source (v7) в legacy configtypes.ProxySource
// для совместимости с существующим парсером (core/config/subscription).
//
//   - subscription → ProxySource{Source, Skip, Outbounds, Tag*, Disabled, ...}
//   - server       → ProxySource{Connections:[URI], TagMask=NodeTagOrLabel, ...}
//   - chain        → ProxySource{TagMask=NodeTagOrLabel, Chain}
//   - folder/auto  → выключенный пустой ProxySource-плейсхолдер: legacy-путь
//     сборки их не умеет (эмиссия из nodes[] — волна W4), а индексный
//     инвариант AsParserConfig (Proxies[i] ↔ Sources[i]) обязан жить.
func (s *Source) ToProxySourceV4() configtypes.ProxySource {
	if s == nil {
		return configtypes.ProxySource{}
	}
	switch s.Kind {
	case SourceKindSubscription:
		ps := configtypes.ProxySource{
			ID:                      s.ID,            // SPEC 112-A: адресат ссылок на узлы
			Label:                   s.displayName(), // только для текстов диагностики
			Source:                  s.URL,
			Skip:                    s.Skip,
			Outbounds:               s.Outbounds, // TEMPORARY BRIDGE: локальные Направления
			ExcludeFromGlobal:       s.ExcludeFromGlobal,
			ExposeGroupTagsToGlobal: s.ExposeGroupTagsToGlobal,
			Fold:                    s.legacyFold(),
			Disabled:                !s.Enabled,
			DisabledNodes:           s.legacyDisabledNodes(),
			// SPEC 115: сообщение провайдера — тоже только для диагностики.
			// Провозится ЗДЕСЬ, потому что дальше по конвейеру метаданных
			// источника уже нет: разбору достаётся только сборочная форма.
			ProviderAnnounce: s.announceMessage(),
		}
		s.applyLegacyDetour(&ps)
		if s.TagPolicy != nil {
			// TagSpec из TagPolicy (PLAN §6): prefix/postfix — канон,
			// mask — мостовое поле до миграции W2.
			ps.TagPrefix = s.TagPolicy.Prefix
			ps.TagPostfix = s.TagPolicy.Postfix
			ps.TagMask = s.TagPolicy.Mask
		}
		ps.Canonical = s.canonicalProjection()
		narrowBridgeForCanonical(&ps)
		return ps

	case SourceKindServer:
		ps := configtypes.ProxySource{
			ID:                s.ID,            // SPEC 112-A: адресат ссылок на узлы
			Label:             s.displayName(), // только для текстов диагностики
			Connections:       []string{s.URI},
			TagMask:           s.NodeTagOrLabel(),
			ExcludeFromGlobal: s.ExcludeFromGlobal,
			Disabled:          !s.Enabled,
			ConfigJSON:        s.ConfigJSON, // ручной outbound JSON
		}
		s.applyLegacyDetour(&ps)
		ps.Canonical = s.canonicalProjection()
		narrowBridgeForCanonical(&ps)
		return ps

	case SourceKindChain:
		// SPEC 110: цепочка не имеет ни URL, ни URI — только позиции.
		// TagMask несёт ТЕГ узла, а не подпись: на тег цепочки ссылаются
		// фильтры Направлений и позиции других цепочек.
		ps := configtypes.ProxySource{
			ID:                s.ID,
			Label:             s.displayName(),
			TagMask:           s.NodeTagOrLabel(),
			ExcludeFromGlobal: s.ExcludeFromGlobal,
			Disabled:          !s.Enabled,
			Chain:             s.Chain, // TEMPORARY BRIDGE: []NodeLink hops — W2/W4
		}
		ps.Canonical = s.canonicalProjection()
		narrowBridgeForCanonical(&ps)
		return ps

	case SourceKindFolder:
		// Папка: у мостового конвейера её формы нет вовсе — узлы едут
		// канонической проекцией (W4). Плейсхолдер остаётся выключенным
		// ТОЛЬКО когда канона нет: индексный инвариант Proxies[i]↔Sources[i]
		// обязан жить в любом случае.
		ps := configtypes.ProxySource{
			ID:       s.ID,
			Label:    s.displayName(),
			Disabled: !s.Enabled,
		}
		if s.TagPolicy != nil {
			ps.TagPrefix = s.TagPolicy.Prefix
			ps.TagPostfix = s.TagPolicy.Postfix
		}
		ps.Canonical = s.canonicalProjection()
		if ps.Canonical == nil {
			ps.Disabled = true
		}
		return ps

	case SourceKindAuto:
		// Корневая провайдерская группа — узел, а не контейнер.
		ps := configtypes.ProxySource{
			ID:       s.ID,
			Label:    s.displayName(),
			TagMask:  s.NodeTagOrLabel(),
			Disabled: !s.Enabled,
		}
		ps.Canonical = s.canonicalProjection()
		if ps.Canonical == nil {
			ps.Disabled = true
		}
		return ps
	}

	// Неизвестный kind: legacy-конвейер его не собирает — отдаём выключенный
	// плейсхолдер, сохраняя позиции индексного инварианта.
	return configtypes.ProxySource{
		ID:       s.ID,
		Label:    s.displayName(),
		Disabled: true,
	}
}

// narrowBridgeForCanonical СУЖАЕТ мост у источника, эмиссия которого уже
// идёт из канона (SPEC 118 W4, PLAN §9: «мост сужается до Load-проекции
// build-путей — детур-деривации умирают»).
//
// Легаси-поля, чью работу забрал канон, обнуляются НЕ ради чистоты, а чтобы
// не сделать её дважды:
//
//   - detour-тройня: её штампует resolveNodeDetours на том же проходе 2,
//     что и ApplyCanonicalNodeLinks, и она перезаписала бы уже разрешённый
//     NodeLink своим ответом — с другой строгостью и другим индексом;
//   - Fold: свёртку разворачивает PrepareFolderReplaces по явному тегу,
//     второй разворот дал бы дубль тега и отказ ядра;
//   - DisabledNodes: выключенные узлы канон в эмиссию не пускает вовсе, и
//     карта здесь уже ни на что не влияет — но оставить её значило бы
//     держать живым ровно тот механизм, который W5 сносит.
//
// Поля, которые канон НЕ забрал (Source/Skip/URL — вход fetch; Label и
// ProviderAnnounce — тексты диагностики; ExcludeFromGlobal у источников без
// канона), остаются: их читают другие стадии.
func narrowBridgeForCanonical(ps *configtypes.ProxySource) {
	if ps == nil || ps.Canonical == nil {
		return
	}
	ps.DetourTag = ""
	ps.DetourNodeSourceID = ""
	ps.DetourNodeTag = ""
	ps.DetourNodeHash = ""
	ps.DetourNodeLabel = ""
	ps.Fold = nil
	ps.DisabledNodes = nil
	// Пул кандидатов решает правило (features/directions.md §2), а не флаги:
	// у канонического источника они больше ничего не значат.
	ps.ExcludeFromGlobal = false
	ps.ExposeGroupTagsToGlobal = false
}

// canonicalProjection — проекция канона v7 в сборочную форму (SPEC 118 W4).
//
// nil = канона нет (материализация ещё не прошла) → сборка идёт мостовым
// путём: парс raw-кэша/URI, как до W4. Непусто → конвейер сборки тел не
// читает и парсеры по подпискам не зовёт (SPEC Т5).
func (s *Source) canonicalProjection() *configtypes.CanonicalSource {
	if s == nil {
		return nil
	}
	switch s.Kind {
	case SourceKindFolder, SourceKindSubscription:
		if len(s.Nodes) == 0 {
			return nil
		}
		cs := &configtypes.CanonicalSource{
			FolderID:     s.ID,
			IsContainer:  true,
			FolderDetour: canonicalLink(s.Detour),
			Replace:      canonicalReplace(s.Replace),
		}
		if s.TagPolicy != nil {
			cs.TagPrefix = s.TagPolicy.Prefix
			cs.TagPostfix = s.TagPolicy.Postfix
		}
		cs.Nodes = make([]configtypes.CanonicalNode, 0, len(s.Nodes))
		for i := range s.Nodes {
			cs.Nodes = append(cs.Nodes, canonicalNodeProjection(&s.Nodes[i]))
		}
		return cs

	case SourceKindServer, SourceKindChain, SourceKindAuto:
		// Корневой узел материализован? У server признак — непустой Body,
		// у chain — Hops, у auto — Group. Без них узел ещё живёт в
		// легаси-полях (URI/ConfigJSON/Chain), и его собирает мост.
		if !s.Node.materialized() {
			return nil
		}
		n := canonicalNodeProjection(&s.Node)
		// Тег корневого узла — тот, под которым его знает конфиг: канон
		// Node.Tag, иначе легаси NodeTag/Label (до миграции W2).
		n.Tag = s.NodeTagOrLabel()
		return &configtypes.CanonicalSource{
			Nodes: []configtypes.CanonicalNode{n},
		}
	}
	return nil
}

// materialized — узел несёт канонические данные эмиссии.
func (n *Node) materialized() bool {
	if n == nil {
		return false
	}
	switch n.Kind {
	case SourceKindServer:
		return len(n.Body) > 0
	case SourceKindChain:
		return len(n.Hops) > 0
	case SourceKindAuto:
		return n.Group != nil
	}
	return false
}

// canonicalNodeProjection — Node (канон) → сборочная форма.
func canonicalNodeProjection(n *Node) configtypes.CanonicalNode {
	out := configtypes.CanonicalNode{
		Kind:    string(n.Kind),
		Tag:     n.Tag,
		Enabled: n.Enabled,
		Body:    n.Body,
		Detour:  canonicalLink(n.Detour),
	}
	if n.Origin != nil {
		out.OriginKind = n.Origin.Kind
		out.OriginRaw = n.Origin.Raw
	}
	if len(n.Hops) > 0 {
		out.Hops = make([]configtypes.NodeLink, 0, len(n.Hops))
		for _, h := range n.Hops {
			out.Hops = append(out.Hops, configtypes.NodeLink{FolderID: h.FolderID, Tag: h.Tag})
		}
	}
	if n.Group != nil {
		g := &configtypes.CanonicalAutoGroup{
			GroupType: n.Group.GroupType,
			Default:   n.Group.Default,
			Options:   AutoStrategyOptions(n.Group.Strategy),
		}
		g.Members = make([]configtypes.NodeLink, 0, len(n.Group.Members))
		for _, m := range n.Group.Members {
			g.Members = append(g.Members, configtypes.NodeLink{FolderID: m.FolderID, Tag: m.Tag})
		}
		out.Group = g
	}
	return out
}

// canonicalLink — NodeLink канона → сборочная форма.
func canonicalLink(l *NodeLink) *configtypes.NodeLink {
	if l == nil {
		return nil
	}
	return &configtypes.NodeLink{FolderID: l.FolderID, Tag: l.Tag}
}

// canonicalReplace — FolderReplace канона → сборочная форма.
func canonicalReplace(r *FolderReplace) *configtypes.FolderReplace {
	if r == nil {
		return nil
	}
	return &configtypes.FolderReplace{
		Mode:     r.Mode,
		Tag:      r.Tag,
		Strategy: r.Strategy.Clone(),
	}
}

// AutoStrategyOptions разворачивает AutoStrategy провайдерской группы в опции
// sing-box-группы — тем же allowlist'ом, каким миграция/fetch их собирали
// (config.autoStrategyFromGroupOptions, обратная сторона).
//
// Живёт здесь, а не в config: проекция обязана быть рядом с типом, из
// которого читает, иначе пакет config получил бы вторую точку знания о форме
// AutoStrategy.
func AutoStrategyOptions(a AutoStrategy) map[string]interface{} {
	opts := make(map[string]interface{}, 5)
	if a.URL != "" {
		opts["url"] = a.URL
	}
	if a.Interval != "" {
		opts["interval"] = a.Interval
	}
	if a.IdleTimeout != "" {
		opts["idle_timeout"] = a.IdleTimeout
	}
	if v := a.Tolerance.Value(); v != nil {
		opts["tolerance"] = v
	}
	if a.InterruptExistConnections != nil {
		opts["interrupt_exist_connections"] = *a.InterruptExistConnections
	}
	if len(opts) == 0 {
		return nil
	}
	return opts
}

// displayName — отображаемое имя источника: канонический Name папки/подписки,
// иначе легаси Label (server/chain и не мигрированные подписки).
func (s *Source) displayName() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Label
}

// legacyFold — Fold из replace (PLAN §6): канонический FolderReplace
// побеждает, легаси-поле — фолбэк до миграции W2.
//
// TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5.
func (s *Source) legacyFold() *configtypes.SourceFold {
	if s.Replace != nil {
		fold := &configtypes.SourceFold{Mode: legacyFoldMode(s.Replace.Mode)}
		fold.Auto = s.Replace.Strategy.Clone()
		// Материализованный тег замены (W2) — вместо позиционных
		// деривативов buildFoldGroups: эмиссия обязана совпадать со
		// ссылками, переписанными миграцией (mode both → `<tag>-auto`).
		switch s.Replace.Mode {
		case FolderReplaceAuto:
			fold.AutoTagOverride = s.Replace.Tag
		case FolderReplaceBoth:
			fold.SelectTagOverride = s.Replace.Tag
			fold.AutoTagOverride = s.Replace.Tag + "-auto"
		default: // manual
			fold.SelectTagOverride = s.Replace.Tag
		}
		return fold
	}
	return s.Fold
}

// legacyFoldMode — режим replace → режим fold (manual↔select, both↔select+auto).
func legacyFoldMode(mode string) string {
	switch mode {
	case FolderReplaceManual:
		return configtypes.FoldModeSelect
	case FolderReplaceAuto:
		return configtypes.FoldModeAuto
	case FolderReplaceBoth:
		return configtypes.FoldModeSelectAuto
	}
	return mode
}

// legacyDisabledNodes — DisabledNodes из enabled=false узлов (PLAN §6) поверх
// легаси-карты. В W1 nodes[] пусты (материализация — W2/W3), так что карта
// едет как есть; после материализации выключенные узлы дополняют её по
// сырым тегам.
//
// TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5.
func (s *Source) legacyDisabledNodes() map[string]int64 {
	if len(s.Nodes) == 0 {
		return s.DisabledNodes
	}
	out := make(map[string]int64, len(s.DisabledNodes)+4)
	for k, v := range s.DisabledNodes {
		out[k] = v
	}
	for _, n := range s.Nodes {
		if !n.Enabled {
			if _, ok := out[n.Tag]; !ok {
				// Отметка без легаси-времени: канон времени не хранит, а
				// TTL-GC легаси-карты жив только до W5 — свежая метка
				// «сейчас» ему и нужна, чтобы отметку не съел сборщик.
				out[n.Tag] = nowUnixForBridge()
			}
		}
	}
	return out
}

// applyLegacyDetour — детур-тройня из NodeLink (PLAN §6): канонический
// Node.Detour побеждает; без него едет легаси-тройня/тег как есть.
//
// TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5.
func (s *Source) applyLegacyDetour(ps *configtypes.ProxySource) {
	if s.Detour != nil {
		if s.Detour.FolderID != "" {
			// Ссылка на узел папки/подписки — прежняя пара source_id + тег.
			ps.DetourNodeSourceID = s.Detour.FolderID
			ps.DetourNodeTag = s.Detour.Tag
		} else {
			// Корневое пространство финальных тегов — прежняя tag-ссылка.
			ps.DetourTag = s.Detour.Tag
		}
		return
	}
	ps.DetourTag = s.DetourTag
	ps.DetourNodeSourceID = s.DetourNodeSourceID
	ps.DetourNodeTag = s.DetourNodeTag
	ps.DetourNodeHash = s.DetourNodeHash // legacy, мигрирует на сборке
	ps.DetourNodeLabel = s.DetourNodeLabel
}

// announceMessage — сообщение провайдера из метаданных источника, обрезанное
// общим правилом (AnnounceMessage); пусто, если провайдер молчал.
//
// Только для подписок: у источника-сервера и цепочки метаданных нет.
func (s *Source) announceMessage() string {
	if s == nil || s.Meta == nil {
		return ""
	}
	return s.Meta.ProviderAnnounce.AnnounceMessage()
}
