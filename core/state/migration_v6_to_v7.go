// File migration_v6_to_v7.go — семантическая миграция старых схем в v7
// (SPEC 118 Т7, PLAN §5): строго 8 шагов features/state.md §«Миграция».
//
// Выполняется один раз при загрузке легаси-состояния (v2–v6), ПОВЕРХ
// структурного переноса adoptConnectionsV6: канонические поля уже разложены
// по домам, легаси-значения приезжают сайдкаром legacySourceV6, и миграция
// переносит их в КАНОН (nodes[], Node.Tag, NodeLink-хопы/детуры, Replace).
//
// Шаг 8 (снос легаси: raw-кэш, defaults → настройки приложения) выполняется
// ТОЛЬКО после успешной записи v7-файла (Load, migrationPurgesLegacy) —
// до неё исходный материал не трогается, а рядом лежит state.json.v6.bak
// (риск Р5).
//
// Идемпотентность — по построению: v7-файл роутится мимо миграции
// (load_router), повторного прохода не существует.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/textnorm"
)


// LoadContext — контекст загрузки состояния: пути, которых нет у голых
// байтов. Load выводит их из пути state-файла (deriveLoadContext); Parse
// работает с нулевым контекстом — материализация тогда невозможна и
// миграция честно предупреждает.
type LoadContext struct {
	// StatePath — путь загружаемого state.json ("" — Parse из байтов).
	StatePath string
	// SubsDir — каталог raw-кэша подписок этого состояния.
	SubsDir string
	// BinDir — bin/ приложения (настройки bin/settings.json; общий на все
	// состояния — умолчания одни, features/state.md).
	BinDir string
}

// migrationV7 — рабочее состояние одной миграции.
type migrationV7 struct {
	s   *State
	lc  LoadContext
	rep *MigrationReport

	// legacy — сайдкар легаси-полей v6 (индекс = индекс s.Sources).
	legacy []legacySourceV6

	// tagCounts — общий счётчик СТАРОЙ тег-машины (глобальная уникализация
	// финальных тегов в порядке источников v6) — по нему строится индекс
	// резолва хопов и детуров.
	tagCounts map[string]int

	// rootFinalByID — финальный (он же канонический сырой) тег верхнего
	// server/chain-узла по ID источника.
	rootFinalByID map[string]string
	// rootFinalSet — финальные теги включённых верхних узлов.
	rootFinalSet map[string]bool
	// subLinkByFinal — финальный тег узла включённой подписки → NodeLink.
	subLinkByFinal map[string]NodeLink
	// subRawTags — сырые теги материализованных узлов по ID подписки.
	subRawTags map[string]map[string]bool
	// hashLinkBySub / hashLinkGlobal — legacy-контент-хэш → NodeLink.
	hashLinkBySub  map[string]map[string]NodeLink
	hashLinkGlobal map[string]NodeLink

	// renames — перепись ссылок (fold both: `<PFX>auto` → `<tag>-auto`).
	renames map[string]string
}

// migrateLegacyStateToV7 — вход миграции; вызывается парсерами старых схем
// ПОСЛЕ adoptConnectionsV6 и ДО построения legacy-проекции.
func migrateLegacyStateToV7(s *State, fromVersion int, lc LoadContext, legacy []legacySourceV6) {
	if s == nil {
		return
	}
	// Сайдкар обязан идти в ногу с каноном: короче — дополняем пустыми, чтобы
	// индексная адресация ниже не требовала проверок на каждом шаге.
	for len(legacy) < len(s.Sources) {
		legacy = append(legacy, legacySourceV6{})
	}
	m := &migrationV7{
		s:              s,
		lc:             lc,
		legacy:         legacy,
		rep:            &MigrationReport{FromVersion: fromVersion},
		tagCounts:      make(map[string]int),
		rootFinalByID:  make(map[string]string),
		rootFinalSet:   make(map[string]bool),
		subLinkByFinal: make(map[string]NodeLink),
		subRawTags:     make(map[string]map[string]bool),
		hashLinkBySub:  make(map[string]map[string]NodeLink),
		hashLinkGlobal: make(map[string]NodeLink),
		renames:        make(map[string]string),
	}

	m.adoptWizardMarkerFolds() // прежние флаги/маркеры → Fold (v5/v2–4 путь)
	m.materializeSources()     // шаги 1–3 (+ SubUpdateStatus, Name)
	m.migrateChainHops()       // шаг 4
	m.migrateDetours()         // шаг 5
	m.migrateFolds()           // шаг 6 (+ renames для mode both)
	m.reportLocalDirections()  // шаг 6, хвост: произвольные локальные Направления
	m.reportExcludes()         // шаг 7
	m.applyRenames()           // перепись ссылок (Р2)

	// Шаг 8 (снос raw-кэша и переезд defaults) выполняется только после
	// успешной записи v7-файла — см. Load; здесь лишь помечаем готовность.
	s.Migration = m.rep
}

// ── маркеры → Fold (пред-шаг для v5/v2–4) ────────────────────────

// adoptWizardMarkerFolds — порт state.migrateSourceFolds на v7-форму:
// v6-парс уже развернул флаги до переноса, а v5/v2–4 пути исторически нет —
// без этого их fold-производные локальные Направления не легли бы в Replace
// (SPEC Т7 шаг 6). Идемпотентно: у уже свёрнутых только вычищаются остатки
// групп.
func (m *migrationV7) adoptWizardMarkerFolds() {
	for i := range m.s.Sources {
		if m.s.Sources[i].Kind != SourceKindSubscription {
			continue
		}
		leg := &m.legacy[i]
		hasAuto, hasSelect := false, false
		for _, ob := range leg.Outbounds {
			if commentIsWizardAuto(ob.Comment) {
				hasAuto = true
			}
			if commentIsWizardSelect(ob.Comment) {
				hasSelect = true
			}
		}
		if leg.Fold == nil && (hasAuto || hasSelect) && leg.ExposeGroupTagsToGlobal {
			mode := legacyFoldModeSelect
			switch {
			case hasAuto && hasSelect:
				mode = legacyFoldModeSelectAuto
			case hasAuto:
				mode = legacyFoldModeAuto
			}
			leg.Fold = &legacyFold{Mode: mode}
			if hasAuto {
				leg.Fold.Auto = autoFromLegacyGroup(leg.Outbounds)
			}
		}
		if hasAuto || hasSelect {
			leg.Outbounds = dropWizardGroups(leg.Outbounds)
		}
		if leg.Fold != nil {
			leg.ExposeGroupTagsToGlobal = false
			leg.ExcludeFromGlobal = false
		}
	}
}

// ── шаги 1–3: материализация, отметки, теги ──────────────────────

func (m *migrationV7) materializeSources() {
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		leg := &m.legacy[i]
		switch src.Kind {
		case SourceKindServer:
			// Тег раньше body: hash-индекс материализации ссылается на
			// ФИНАЛЬНЫЙ тег (в т.ч. уникализированный при коллизии).
			m.stampRootTag(src, leg)
			m.materializeServer(src, leg)
		case SourceKindChain:
			m.stampRootTag(src, leg)
			m.materializeChain(src, leg)
		case SourceKindSubscription:
			// Отображаемое имя переезжает в канонический Name.
			if src.Name == "" && leg.Label != "" {
				src.Name = leg.Label
			}
			// Шаг 3: маска-шаблон подписки упразднена (Т2: TagPolicy — только
			// prefix/postfix).
			if leg.TagMask != "" {
				m.rep.add("subscription %q: tag mask %q is gone — only prefix/postfix apply after the update", m.sourceName(src, leg), leg.TagMask)
			}
			m.materializeSubscription(src, leg)
		}
	}
}

// legacyNodeTag — прежний системный тег узла source-уровня: канонический
// Node.Tag (если уже проставлен) → NodeTag → mask (у server/chain она хранила
// именно тег) → Label.
func legacyNodeTag(src *Source, leg *legacySourceV6) string {
	if src.Tag != "" {
		return src.Tag
	}
	if leg.NodeTag != "" {
		return leg.NodeTag
	}
	if leg.TagMask != "" {
		return leg.TagMask
	}
	return leg.Label
}

// stampRootTag — шаг 3 для верхних узлов: NodeTag/mask/Label → Node.Tag.
// Канонический сырой тег корневого узла = его финальный тег по старой
// машине (нормализация + глобальная уникализация): именно под ним узел
// значился в правилах и хопах. Добавленный суффикс — коллизия имён верхних
// узлов (шаг 5): фиксируется предупреждением.
func (m *migrationV7) stampRootTag(src *Source, leg *legacySourceV6) {
	raw := strings.TrimSpace(legacyNodeTag(src, leg))
	if raw == "" {
		m.rep.add("source %s (%s): the node has neither tag nor name — tag not carried over", src.ID, src.Kind)
		return
	}
	normalized := textnorm.NormalizeProxyDisplay(raw)
	if !src.Enabled {
		// Выключенный источник в старой машине имени не занимал.
		src.Node.Tag = normalized
		m.rootFinalByID[src.ID] = normalized
		return
	}
	final := uniquifyTagAgainstCounts(normalized, m.tagCounts)
	if final != normalized {
		m.rep.add("node %q renamed to %q: the tag is taken by an earlier source (root node collision); source_id references were repointed", normalized, final)
	}
	src.Node.Tag = final
	m.rootFinalByID[src.ID] = final
	m.rootFinalSet[final] = true
}

// materializeChain — body корневой цепочки: настройки маршрута без позиций
// (idle_timeout / strip_evasion / strip / rewrite). Позиции переезжают
// отдельно, шагом 4 (migrateChainHops): они ссылки, а не значения.
func (m *migrationV7) materializeChain(src *Source, leg *legacySourceV6) {
	if len(src.Body) > 0 || leg.Chain == nil {
		return
	}
	src.Body = configtypes.ChainBody(leg.Chain)
}

// materializeServer — body корневого server-источника из URI/config_json
// (канонический дом; URI остаётся в origin.raw байт-в-байт).
func (m *migrationV7) materializeServer(src *Source, leg *legacySourceV6) {
	if migrationHooks.MaterializeServer == nil {
		return // парсер недоступен (изолированные тесты state)
	}
	if len(src.Body) > 0 {
		return
	}
	if strings.TrimSpace(leg.URI) == "" && len(leg.ConfigJSON) == 0 {
		return
	}
	res, err := migrationHooks.MaterializeServer(MigrationServerRequest{
		URI:        leg.URI,
		ConfigJSON: leg.ConfigJSON,
	})
	if err != nil {
		// Узла без тела в модели v7 не существует: собирать его не из чего,
		// и молча уронить его в конфиг нельзя — источник остаётся в состоянии
		// выключенным, с явной строкой в отчёте.
		src.Enabled = false
		m.rep.add("server %q: node body could not be parsed (%v) — source disabled, set the URI or JSON again", legacyNodeTag(src, leg), err)
		return
	}
	src.Body = res.Body
	src.Origin = &Origin{Kind: res.OriginKind, Raw: res.OriginRaw}
	if res.LegacyHash != "" {
		m.hashLinkGlobal[res.LegacyHash] = NodeLink{Tag: m.rootFinalOrRaw(src, leg)}
	}
}

// rootFinalOrRaw — финальный тег корневого узла (может быть ещё не
// проставлен — materializeServer идёт до stampRootTag того же источника).
func (m *migrationV7) rootFinalOrRaw(src *Source, leg *legacySourceV6) string {
	if t, ok := m.rootFinalByID[src.ID]; ok {
		return t
	}
	return textnorm.NormalizeProxyDisplay(legacyNodeTag(src, leg))
}

// materializeSubscription — шаги 1–2 одной подписки: nodes[] из raw-кэша
// новым чистым парсером, затем отметки выключения → enabled=false.
func (m *migrationV7) materializeSubscription(src *Source, leg *legacySourceV6) {
	subName := src.Name
	if subName == "" {
		subName = src.ID
	}

	m.transferSubUpdateStatus(src, leg)

	if len(src.Nodes) > 0 {
		return // уже материализована (повторная миграция невозможна, но форма легальна)
	}

	if m.lc.SubsDir == "" {
		m.rep.add("subscription %q: raw cache unavailable (load without a path) — nodes will appear after the first update", subName)
		m.reportUnappliedMarks(leg, subName)
		return
	}
	if migrationHooks.MaterializeSubscription == nil {
		m.rep.add("subscription %q: materialization parser unavailable — nodes will appear after the first update", subName)
		m.reportUnappliedMarks(leg, subName)
		return
	}

	body, err := readLegacyRawBody(m.lc.SubsDir, src.ID)
	if err != nil {
		// Кэша нет → nodes[] пуст, подписка ждёт первого fetch (Т7 шаг 1);
		// сопутствующие карты отбрасываются с предупреждением (снос — шаг 8).
		m.rep.add("subscription %q: no raw cache — nodes will appear after the first update", subName)
		m.reportUnappliedMarks(leg, subName)
		return
	}

	// Кап: настройка подписки → прежний глобальный дефолт → потолок 3000.
	capN := src.MaxNodes
	if capN <= 0 {
		capN = m.s.Defaults.MaxNodes
	}

	counts := m.tagCounts
	if !src.Enabled {
		// Выключенная подписка в старой машине тегов не занимала — её
		// финальные теги считаем в стороне, не искажая общий порядок.
		counts = make(map[string]int)
	}
	prefix, postfix := "", ""
	if src.TagPolicy != nil {
		prefix, postfix = src.TagPolicy.Prefix, src.TagPolicy.Postfix
	}

	res, err := migrationHooks.MaterializeSubscription(MigrationSubRequest{
		SubID:      src.ID,
		Body:       body,
		Skip:       src.Skip,
		MaxNodes:   capN,
		TagPrefix:  prefix,
		TagPostfix: postfix,
		TagMask:    leg.TagMask,
		TagCounts:  counts,
	})
	if err != nil {
		m.rep.add("subscription %q: raw cache could not be parsed (%v) — nodes will appear after the first update", subName, err)
		m.reportUnappliedMarks(leg, subName)
		return
	}
	for _, w := range res.Warnings {
		m.rep.add("subscription %q: %s", subName, w)
	}

	rawSet := make(map[string]bool, len(res.Nodes))
	hashByRaw := make(map[string]string, len(res.Nodes))
	src.Nodes = make([]Node, 0, len(res.Nodes))
	for _, mat := range res.Nodes {
		src.Nodes = append(src.Nodes, mat.Node)
		rawSet[mat.Node.Tag] = true
		if mat.LegacyHash != "" {
			hashByRaw[mat.LegacyHash] = mat.Node.Tag
			link := NodeLink{FolderID: src.ID, Tag: mat.Node.Tag}
			if _, taken := m.hashLinkBySub[src.ID]; !taken {
				m.hashLinkBySub[src.ID] = make(map[string]NodeLink)
			}
			m.hashLinkBySub[src.ID][mat.LegacyHash] = link
			if _, taken := m.hashLinkGlobal[mat.LegacyHash]; !taken {
				m.hashLinkGlobal[mat.LegacyHash] = link
			}
		}
		if src.Enabled {
			m.subLinkByFinal[mat.FinalTag] = NodeLink{FolderID: src.ID, Tag: mat.Node.Tag}
		}
	}
	m.subRawTags[src.ID] = rawSet
	if res.Truncated {
		if src.UpdateStatus == nil {
			src.UpdateStatus = &SubUpdateStatus{}
		}
		src.UpdateStatus.Truncated = true
	}

	// Шаг 2: карта выключенных → enabled=false по identity-ключам; ключи
	// legacy-64hex докручиваются тем же проходом. Несматченные ключи живут
	// в PendingDisabled: узел мог остаться за капом разбора, и до первого
	// достоверного fetch отметку не выбрасывают (закон чисток, вердикт O2).
	if len(leg.DisabledNodes) > 0 {
		var pending []string
		markDisabled := func(rawTag string) {
			for i := range src.Nodes {
				if src.Nodes[i].Tag == rawTag {
					src.Nodes[i].Enabled = false
				}
			}
		}
		for key := range leg.DisabledNodes {
			switch {
			case rawSet[key]:
				markDisabled(key)
			case isLegacyDisabledHash(key):
				if rawTag, ok := hashByRaw[key]; ok {
					markDisabled(rawTag)
				} else {
					m.rep.add("subscription %q: disable mark (legacy hash %s…) matched no node", subName, key[:8])
				}
			default:
				m.rep.add("subscription %q: disable mark %q matched no node", subName, key)
				pending = append(pending, key)
			}
		}
		if len(pending) > 0 {
			src.PendingDisabled = appendUniqueStrings(src.PendingDisabled, pending)
		}
	}
}

// reportUnappliedMarks — предупреждение о картах, которые не к чему
// применить (Т7 шаг 1: «карты отбрасываются с предупреждением»; физический
// снос — шаг 8 под гейтом).
func (m *migrationV7) reportUnappliedMarks(leg *legacySourceV6, subName string) {
	if len(leg.DisabledNodes) > 0 {
		m.rep.add("subscription %q: %d disable mark(s) not carried over — no materialized nodes", subName, len(leg.DisabledNodes))
	}
}

// transferSubUpdateStatus — история fetch из v6-меты → канонический
// SubUpdateStatus (PLAN §1.2).
func (m *migrationV7) transferSubUpdateStatus(src *Source, leg *legacySourceV6) {
	if src.UpdateStatus != nil {
		return
	}
	meta := leg.MetaHistory
	// Истории не было — пустышку не плодим.
	if meta.IsEmpty() {
		return
	}
	st := &SubUpdateStatus{
		URLAtFetch:        meta.URLAtFetch,
		LastAttemptAt:     meta.LastFetchedAt,
		LastStatus:        meta.LastStatus,
		ErrorCount:        meta.ErrorCount,
		LastErrorMsg:      meta.LastErrorMsg,
		LastErrorURL:      meta.LastErrorURL,
		HTTPStatusCode:    meta.HTTPStatusCode,
		RawBodyBytes:      meta.RawBodyBytes,
		NodesCountFetched: meta.NodesCountFetched,
		Truncated:         meta.Truncated,
	}
	if meta.LastStatus == "ok" {
		st.LastSuccessAt = meta.LastFetchedAt
	}
	src.UpdateStatus = st
}

// ── шаг 4: хопы цепочек ──────────────────────────────────────────

func (m *migrationV7) migrateChainHops() {
	known := m.knownRootTargets()
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		leg := &m.legacy[i]
		if src.Kind != SourceKindChain || leg.Chain == nil || len(leg.Chain.Hops) == 0 {
			continue
		}
		if len(src.Node.Hops) > 0 {
			continue
		}
		links := make([]NodeLink, 0, len(leg.Chain.Hops))
		for _, hop := range leg.Chain.Hops {
			hop = strings.TrimSpace(hop)
			if hop == "" {
				continue
			}
			if link, ok := m.subLinkByFinal[hop]; ok {
				links = append(links, link)
				continue
			}
			if !known[hop] {
				// Нерезолвящийся хоп остаётся ссылкой на несуществующий тег —
				// fail-closed на сборке (Т7 шаг 4).
				m.rep.add("chain %q: hop %q does not resolve — the chain degrades fail-closed", legacyNodeTag(src, leg), hop)
			}
			links = append(links, NodeLink{Tag: hop})
		}
		src.Node.Hops = links
	}
}

// knownRootTargets — легальные цели корневого пространства финальных тегов:
// верхние узлы, Направления и их двойники, теги свёрток.
func (m *migrationV7) knownRootTargets() map[string]bool {
	known := make(map[string]bool, len(m.rootFinalSet)+len(m.s.Directions)*2)
	for tag := range m.rootFinalSet {
		known[tag] = true
	}
	for i := range m.s.Directions {
		d := &m.s.Directions[i]
		if d.Tag == "" {
			continue
		}
		known[d.Tag] = true
		if d.Auto != nil {
			known[d.AutoTag()] = true
		}
	}
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		leg := &m.legacy[i]
		if src.Kind != SourceKindSubscription || leg.Fold == nil {
			continue
		}
		prefix := ""
		if src.TagPolicy != nil {
			prefix = src.TagPolicy.Prefix
		}
		if leg.Fold.HasSelect() {
			known[legacyFoldSelectTag(prefix, i)] = true
		}
		if leg.Fold.HasAuto() {
			known[legacyFoldAutoTag(prefix, i)] = true
		}
	}
	return known
}

// ── шаг 5: detour-тройня → NodeLink ──────────────────────────────

func (m *migrationV7) migrateDetours() {
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		leg := &m.legacy[i]
		link := m.resolveLegacyDetour(src, leg)
		if link == nil {
			continue
		}
		switch src.Kind {
		case SourceKindServer, SourceKindSubscription:
			if src.Node.Detour == nil {
				src.Node.Detour = link
			}
		case SourceKindChain:
			// detour у Chain не существует типом (SPEC Т2); старый build
			// его и не собирал.
			m.rep.add("chain %q: the detour link is gone in the v7 model — dropped", legacyNodeTag(src, leg))
		}
	}
}

// resolveLegacyDetour — тройня/тег → NodeLink (nil = переносить нечего или
// hash не сматчен ни с одним узлом — ссылка теряется с warning).
func (m *migrationV7) resolveLegacyDetour(src *Source, leg *legacySourceV6) *NodeLink {
	sid := strings.TrimSpace(leg.DetourNodeSourceID)
	dtag := strings.TrimSpace(leg.DetourNodeTag)
	hash := strings.TrimSpace(leg.DetourNodeHash)

	switch {
	case sid != "":
		target := m.s.FindSource(sid)
		if target == nil {
			m.rep.add("source %q: detour link to a missing source %s — degrades fail-closed", m.sourceName(src, leg), sid)
			if dtag != "" {
				return &NodeLink{Tag: dtag}
			}
			return nil
		}
		if target.Kind == SourceKindSubscription {
			raw := dtag
			if raw == "" && hash != "" {
				if link, ok := m.hashLinkBySub[sid][hash]; ok {
					return &NodeLink{FolderID: link.FolderID, Tag: link.Tag}
				}
				m.rep.add("source %q: legacy-hash detour matched no subscription node — the link is lost, set it again", m.sourceName(src, leg))
				return nil
			}
			if raw == "" {
				return nil
			}
			if tags, materialized := m.subRawTags[sid]; materialized && !tags[raw] {
				m.rep.add("source %q: detour node %q not found in the subscription — degrades fail-closed", m.sourceName(src, leg), raw)
			}
			return &NodeLink{FolderID: sid, Tag: raw}
		}
		// Верхний узел: ссылка по source_id сплющивается в голый финальный
		// тег корневого пространства (включая уникализированный при
		// коллизии — перепись ссылок шага 5).
		final := m.rootFinalByID[sid]
		if final == "" {
			final = dtag
		}
		if final == "" {
			return nil
		}
		return &NodeLink{Tag: final}

	case dtag != "":
		// Переходная форма «тег без source_id» (deps-К6): формы совпадают.
		return &NodeLink{Tag: dtag}

	case hash != "":
		if link, ok := m.hashLinkGlobal[hash]; ok {
			return &NodeLink{FolderID: link.FolderID, Tag: link.Tag}
		}
		m.rep.add("source %q: legacy-hash detour matched no node — the link is lost, set it again", m.sourceName(src, leg))
		return nil

	case strings.TrimSpace(leg.DetourTag) != "":
		return &NodeLink{Tag: strings.TrimSpace(leg.DetourTag)}
	}
	return nil
}

func (m *migrationV7) sourceName(src *Source, leg *legacySourceV6) string {
	if src.Kind == SourceKindSubscription {
		if src.Name != "" {
			return src.Name
		}
		return src.ID
	}
	return legacyNodeTag(src, leg)
}

// ── шаг 6: fold → FolderReplace ──────────────────────────────────

func (m *migrationV7) migrateFolds() {
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		leg := &m.legacy[i]
		if src.Kind != SourceKindSubscription || leg.Fold == nil || src.Replace != nil {
			continue
		}
		prefix := ""
		if src.TagPolicy != nil {
			prefix = src.TagPolicy.Prefix
		}
		selectTag := legacyFoldSelectTag(prefix, i)
		autoTag := legacyFoldAutoTag(prefix, i)

		rep := &FolderReplace{}
		switch leg.Fold.EffectiveMode() {
		case legacyFoldModeAuto:
			rep.Mode = FolderReplaceAuto
			rep.Tag = autoTag
		case legacyFoldModeSelectAuto:
			rep.Mode = FolderReplaceBoth
			rep.Tag = selectTag
			// Пара тегов старой свёртки (`<PFX>select` + `<PFX>auto`) не
			// выражается моделью `<tag>-auto` — двойник получает
			// `<selectTag>-auto`, ссылки переписываются (риск Р2), выбор в
			// кэше ядра у авто-двойника протухает.
			newAuto := rep.Tag + "-auto"
			if autoTag != newAuto {
				m.renames[autoTag] = newAuto
				m.rep.add("fold %q: auto group renamed %q → %q (v7 model); references were repointed, and the core's cached pick for it will reset", m.sourceName(src, leg), autoTag, newAuto)
			}
		default: // select
			rep.Mode = FolderReplaceManual
			rep.Tag = selectTag
		}
		if leg.Fold.HasAuto() && leg.Fold.Auto != nil {
			rep.Strategy = leg.Fold.Auto.Clone()
		}
		src.Replace = rep
	}
}

// reportLocalDirections — произвольные локальные Направления источника
// (fold-производные уже развёрнуты в Fold/Replace) упраздняются классом.
func (m *migrationV7) reportLocalDirections() {
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		leg := &m.legacy[i]
		if len(leg.Outbounds) == 0 {
			continue
		}
		tags := make([]string, 0, len(leg.Outbounds))
		for _, ob := range leg.Outbounds {
			tags = append(tags, ob.Tag)
		}
		m.rep.add("source %q: per-source Directions are gone — %s will be lost (create global Directions with filters instead)", m.sourceName(src, leg), strings.Join(tags, ", "))
	}
}

// reportExcludes — шаг 7: одиночный exclude_from_global (без свёртки).
func (m *migrationV7) reportExcludes() {
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		leg := &m.legacy[i]
		if !leg.ExcludeFromGlobal || leg.Fold != nil || src.Replace != nil {
			continue
		}
		m.rep.add("source %q: the “exclude from the global list” flag is gone — its nodes stay in the candidate pool (fold the source into a group for the previous behaviour)", m.sourceName(src, leg))
	}
}

// ── перепись ссылок ──────────────────────────────────────────────

// applyRenames — единый проход по всем видам ссылок на теги (тот же список
// мест, что у RenameDirection в UI: правила, route_final, опции Направлений,
// detour DNS-серверов, позиции цепочек, детуры источников). Новое место
// ссылки обязано добавляться сюда, а не частной правкой (правило
// build-graph-sanitizer).
func (m *migrationV7) applyRenames() {
	if len(m.renames) == 0 {
		return
	}
	rename := func(tag string) (string, bool) {
		if to, ok := m.renames[tag]; ok {
			return to, true
		}
		return tag, false
	}

	// 1. Цели правил (canonical Rules; legacy CustomRules-вид строится
	// парсером ПОСЛЕ миграции — но переписываем и его: v5/v2–4 пути
	// наполняют его до миграции).
	for i := range m.s.Rules {
		r := &m.s.Rules[i]
		body, err := r.DecodeBody()
		if err != nil {
			continue
		}
		changed := false
		switch b := body.(type) {
		case *InlineBody:
			if to, ok := rename(b.Outbound); ok {
				b.Outbound = to
				changed = true
			}
		case *SrsBody:
			if to, ok := rename(b.Outbound); ok {
				b.Outbound = to
				changed = true
			}
		case *PresetBody:
			// Переменные пресета несут теги значениями (в т.ч. 'outbound') —
			// тот же список мест, что у RenameDirection (находка ревью W2:
			// пропуск повесил бы ссылку [PFX]auto в var молча).
			for name, val := range b.Vars {
				if to, ok := rename(val); ok {
					b.Vars[name] = to
					changed = true
				}
			}
		}
		if changed {
			if raw, err := json.Marshal(body); err == nil {
				r.Body = raw
			}
		}
	}
	for i := range m.s.CustomRules {
		cr := &m.s.CustomRules[i]
		if to, ok := rename(cr.SelectedOutbound); ok {
			cr.SelectedOutbound = to
		}
		if cr.Rule != nil {
			if cur, ok := cr.Rule["outbound"].(string); ok {
				if to, renamed := rename(cur); renamed {
					cr.Rule["outbound"] = to
				}
			}
		}
	}

	// 2. Маршрут по умолчанию.
	for i := range m.s.Vars {
		if m.s.Vars[i].Name == "route_final" {
			if to, ok := rename(m.s.Vars[i].Value); ok {
				m.s.Vars[i].Value = to
			}
		}
	}

	// 3. Опции Направлений.
	for i := range m.s.Directions {
		d := &m.s.Directions[i]
		for j, opt := range d.AddOutbounds {
			if to, ok := rename(opt); ok {
				d.AddOutbounds[j] = to
			}
		}
		if d.Options != nil {
			if def, ok := d.Options["default"].(string); ok {
				if to, renamed := rename(def); renamed {
					d.Options["default"] = to
				}
			}
		}
	}

	// 4. detour DNS-серверов (kind=user, Body-карта).
	for i := range m.s.DNS.Servers {
		srv := &m.s.DNS.Servers[i]
		if srv.Body == nil {
			continue
		}
		if det, ok := srv.Body["detour"].(string); ok {
			if to, renamed := rename(det); renamed {
				srv.Body["detour"] = to
			}
		}
	}

	// 5. Позиции цепочек и 6. детуры источников — канонические NodeLink
	// корневого пространства (легаси-формы к этому моменту уже переведены
	// в канон шагами 4–5).
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		for j := range src.Node.Hops {
			if src.Node.Hops[j].FolderID != "" {
				continue
			}
			if to, ok := rename(src.Node.Hops[j].Tag); ok {
				src.Node.Hops[j].Tag = to
			}
		}
		if src.Node.Detour != nil && src.Node.Detour.FolderID == "" {
			if to, ok := rename(src.Node.Detour.Tag); ok {
				src.Node.Detour.Tag = to
			}
		}
	}
}

// ── шаг 8: снос легаси ───────────────────────────────────────────

// purgeLegacyAfterMigration — необратимый шаг 8: raw-кэш подписок и
// умолчания подключений (→ настройки приложения). Вызывается ТОЛЬКО после
// успешной записи v7-файла (Load, migrationPurgesLegacy) — до неё исходный
// материал не трогается (риск Р5), а рядом уже лежит `<state>.v6.bak`.
//
// Легаси-ПОЛЕЙ здесь сносить нечего: их больше нет в типе Source — они
// умерли вместе с сайдкаром миграции (legacySourceV6), как только Load
// завершился. На диск v7-файл их и не пишет.
func purgeLegacyAfterMigration(s *State, lc LoadContext) {
	if s == nil {
		return
	}

	// defaults → bin/settings.json, не перетирая явно выставленные.
	if lc.BinDir != "" && s.Defaults != (Defaults{}) {
		settings := locale.LoadSettings(lc.BinDir)
		changed := false
		if settings.DefaultSubscriptionReload == "" && s.Defaults.Reload != "" {
			settings.DefaultSubscriptionReload = s.Defaults.Reload
			changed = true
		}
		if settings.DefaultSubscriptionMaxNodes == 0 && s.Defaults.MaxNodes > 0 {
			settings.DefaultSubscriptionMaxNodes = s.Defaults.MaxNodes
			changed = true
		}
		if changed {
			_ = locale.SaveSettings(lc.BinDir, settings)
		}
	}
	s.Defaults = Defaults{}

	if lc.SubsDir == "" {
		return
	}
	for i := range s.Sources {
		src := &s.Sources[i]
		if src.Kind != SourceKindSubscription {
			continue
		}
		if p, err := legacyRawPath(lc.SubsDir, src.ID); err == nil {
			_ = removeFileIfExists(p)
		}
	}
}

// ── вспомогательное ──────────────────────────────────────────────

// uniquifyTagAgainstCounts — та же схема, что subscription.MakeTagUnique
// (`X`, `X-2`, кандидат проверяется на занятость — SPEC 113-A §5); копия
// здесь, потому что state не может импортировать пакет парсера (цикл).
// Читатель миграции — санкционированное исключение grep-инвариантов §4.A.
func uniquifyTagAgainstCounts(name string, counts map[string]int) string {
	if counts[name] == 0 {
		counts[name] = 1
		return name
	}
	for {
		counts[name]++
		candidate := fmt.Sprintf("%s-%d", name, counts[name])
		if counts[candidate] == 0 {
			counts[candidate] = 1
			return candidate
		}
	}
}

// removeFileIfExists — os.Remove, где «файла нет» не ошибка.
func removeFileIfExists(path string) error {
	err := os.Remove(path)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

// isLegacyDisabledHash — ключ выглядит как упразднённый контент-хэш
// SPEC 094/101 (64 hex). Порт isLegacyIdentityHash из парсера — прод-копия
// из source_loader переезжает сюда насовсем в W5 (Т6).
func isLegacyDisabledHash(key string) bool {
	if len(key) != 64 {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			continue
		}
		return false
	}
	return true
}

// appendUniqueStrings — дописать значения, не заводя дублей (порядок
// существующих сохраняется). Без slices/maps: go1.20-гард.
func appendUniqueStrings(dst, add []string) []string {
	if len(add) == 0 {
		return dst
	}
	seen := make(map[string]bool, len(dst)+len(add))
	for _, v := range dst {
		seen[v] = true
	}
	for _, v := range add {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		dst = append(dst, v)
	}
	return dst
}
