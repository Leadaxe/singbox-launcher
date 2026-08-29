// File migration_v6_to_v7.go — семантическая миграция старых схем в v7
// (SPEC 118 Т7, PLAN §5): строго 8 шагов features/state.md §«Миграция».
//
// Выполняется один раз при загрузке легаси-состояния (v2–v6), ПОВЕРХ
// структурного переноса adoptConnectionsV6 волны W1: легаси-поля уже лежат
// в мостовых деривативах Source, миграция обогащает КАНОН (nodes[],
// Node.Tag, NodeLink-хопы/детуры, Replace) — мост adapter_source.go
// предпочитает канон, поэтому поведение живой сборки не меняется.
//
// Шаг 8 (снос легаси: raw-кэш, карты, defaults → настройки приложения)
// ГЕЙТИТСЯ константой migrationPurgesLegacy (false до W5): мигрированное
// состояние сосуществует со старым build-путём. Код сноса и бэкап-копия
// state.json.v6.bak написаны и проверены сейчас (риск Р5).
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
func migrateLegacyStateToV7(s *State, fromVersion int, lc LoadContext) {
	if s == nil {
		return
	}
	m := &migrationV7{
		s:              s,
		lc:             lc,
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

	// Шаг 8 — под гейтом до W5 (PLAN §6): снос выполняется только после
	// успешной записи v7 (см. Load), здесь лишь помечаем готовность.
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
		src := &m.s.Sources[i]
		if src.Kind != SourceKindSubscription {
			continue
		}
		hasAuto, hasSelect := false, false
		for _, ob := range src.Outbounds {
			if commentIsWizardAuto(ob.Comment) {
				hasAuto = true
			}
			if commentIsWizardSelect(ob.Comment) {
				hasSelect = true
			}
		}
		if src.Fold == nil && (hasAuto || hasSelect) && src.ExposeGroupTagsToGlobal {
			mode := configtypes.FoldModeSelect
			switch {
			case hasAuto && hasSelect:
				mode = configtypes.FoldModeSelectAuto
			case hasAuto:
				mode = configtypes.FoldModeAuto
			}
			src.Fold = &configtypes.SourceFold{Mode: mode}
			if hasAuto {
				src.Fold.Auto = autoFromLegacyGroup(src.Outbounds)
			}
		}
		if hasAuto || hasSelect {
			src.Outbounds = dropWizardGroups(src.Outbounds)
		}
		if src.Fold != nil {
			src.ExposeGroupTagsToGlobal = false
			src.ExcludeFromGlobal = false
		}
	}
}

// ── шаги 1–3: материализация, отметки, теги ──────────────────────

func (m *migrationV7) materializeSources() {
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		switch src.Kind {
		case SourceKindServer:
			// Тег раньше body: hash-индекс материализации ссылается на
			// ФИНАЛЬНЫЙ тег (в т.ч. уникализированный при коллизии).
			m.stampRootTag(src)
			m.materializeServer(src)
		case SourceKindChain:
			m.stampRootTag(src)
		case SourceKindSubscription:
			// Отображаемое имя переезжает в канонический Name; Label живёт
			// мостом до W5.
			if src.Name == "" && src.Label != "" {
				src.Name = src.Label
			}
			// Шаг 3: маска-шаблон подписки упразднена (Т2: TagPolicy — только
			// prefix/postfix). Мост дочитывает её до W5, канон не получает.
			if src.TagPolicy != nil && src.TagPolicy.Mask != "" {
				m.rep.add("подписка %q: маска тегов %q упразднена — после обновления будут действовать только префикс/постфикс", m.sourceName(src), src.TagPolicy.Mask)
			}
			m.materializeSubscription(src)
		}
	}
}

// stampRootTag — шаг 3 для верхних узлов: NodeTag/Label → Node.Tag.
// Канонический сырой тег корневого узла = его финальный тег по старой
// машине (нормализация + глобальная уникализация): именно под ним узел
// значился в правилах и хопах. Добавленный суффикс — коллизия имён верхних
// узлов (шаг 5): фиксируется предупреждением.
func (m *migrationV7) stampRootTag(src *Source) {
	raw := strings.TrimSpace(src.NodeTagOrLabel())
	if raw == "" {
		m.rep.add("источник %s (%s): у узла нет ни тега, ни имени — тег не перенесён", src.ID, src.Kind)
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
		m.rep.add("узел %q переименован в %q: тег занят более ранним источником (коллизия верхних узлов); ссылки по source_id переписаны", normalized, final)
	}
	src.Node.Tag = final
	m.rootFinalByID[src.ID] = final
	m.rootFinalSet[final] = true
}

// materializeServer — body корневого server-источника из URI/config_json
// (канонический дом; URI остаётся в origin.raw байт-в-байт).
func (m *migrationV7) materializeServer(src *Source) {
	if migrationHooks.MaterializeServer == nil {
		return // парсер недоступен (изолированные тесты state) — мост живёт
	}
	if len(src.Body) > 0 {
		return
	}
	if strings.TrimSpace(src.URI) == "" && len(src.ConfigJSON) == 0 {
		return
	}
	res, err := migrationHooks.MaterializeServer(MigrationServerRequest{
		URI:        src.URI,
		ConfigJSON: src.ConfigJSON,
	})
	if err != nil {
		// Узел живёт мостом (URI/config_json на месте) — это деградация
		// материализации, не потеря узла.
		m.rep.add("server %q: body не материализован (%v) — узел продолжает собираться старым путём", src.NodeTagOrLabel(), err)
		return
	}
	src.Body = res.Body
	src.Origin = &Origin{Kind: res.OriginKind, Raw: res.OriginRaw}
	if res.LegacyHash != "" {
		m.hashLinkGlobal[res.LegacyHash] = NodeLink{Tag: m.rootFinalOrRaw(src)}
	}
}

// rootFinalOrRaw — финальный тег корневого узла (может быть ещё не
// проставлен — materializeServer идёт до stampRootTag того же источника).
func (m *migrationV7) rootFinalOrRaw(src *Source) string {
	if t, ok := m.rootFinalByID[src.ID]; ok {
		return t
	}
	return textnorm.NormalizeProxyDisplay(src.NodeTagOrLabel())
}

// materializeSubscription — шаги 1–2 одной подписки: nodes[] из raw-кэша
// новым чистым парсером, затем отметки выключения → enabled=false.
func (m *migrationV7) materializeSubscription(src *Source) {
	subName := src.Name
	if subName == "" {
		subName = src.ID
	}

	m.transferSubUpdateStatus(src)

	if len(src.Nodes) > 0 {
		return // уже материализована (повторная миграция невозможна, но форма легальна)
	}

	if m.lc.SubsDir == "" {
		m.rep.add("подписка %q: raw-кэш недоступен (загрузка без пути) — узлы появятся после первого обновления", subName)
		m.reportUnappliedMarks(src, subName)
		return
	}
	if migrationHooks.MaterializeSubscription == nil {
		m.rep.add("подписка %q: парсер материализации недоступен — узлы появятся после первого обновления", subName)
		m.reportUnappliedMarks(src, subName)
		return
	}

	body, err := ReadRawBody(m.lc.SubsDir, src.ID)
	if err != nil {
		// Кэша нет → nodes[] пуст, подписка ждёт первого fetch (Т7 шаг 1);
		// сопутствующие карты отбрасываются с предупреждением (снос — шаг 8).
		m.rep.add("подписка %q: raw-кэша нет — узлы появятся после первого обновления", subName)
		m.reportUnappliedMarks(src, subName)
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
	var prefix, postfix, mask string
	if src.TagPolicy != nil {
		prefix, postfix, mask = src.TagPolicy.Prefix, src.TagPolicy.Postfix, src.TagPolicy.Mask
	}

	res, err := migrationHooks.MaterializeSubscription(MigrationSubRequest{
		SubID:      src.ID,
		Body:       body,
		Skip:       src.Skip,
		MaxNodes:   capN,
		TagPrefix:  prefix,
		TagPostfix: postfix,
		TagMask:    mask,
		TagCounts:  counts,
	})
	if err != nil {
		m.rep.add("подписка %q: raw-кэш не разобран (%v) — узлы появятся после первого обновления", subName, err)
		m.reportUnappliedMarks(src, subName)
		return
	}
	for _, w := range res.Warnings {
		m.rep.add("подписка %q: %s", subName, w)
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
	// legacy-64hex докручиваются тем же проходом. Мостовая карта при этом
	// переписывается на сырые теги — деривация legacyDisabledNodes из
	// enabled=false тогда не плодит вторых ключей (согласованность моста).
	if len(src.DisabledNodes) > 0 {
		rewritten := make(map[string]int64, len(src.DisabledNodes))
		markDisabled := func(rawTag string, ts int64) {
			for i := range src.Nodes {
				if src.Nodes[i].Tag == rawTag {
					src.Nodes[i].Enabled = false
				}
			}
			if prev, dup := rewritten[rawTag]; !dup || prev < ts {
				rewritten[rawTag] = ts
			}
		}
		for key, ts := range src.DisabledNodes {
			switch {
			case rawSet[key]:
				markDisabled(key, ts)
			case isLegacyDisabledHash(key):
				if rawTag, ok := hashByRaw[key]; ok {
					markDisabled(rawTag, ts)
				} else {
					m.rep.add("подписка %q: отметка выключения (legacy-хэш %s…) не сматчена ни с одним узлом", subName, key[:8])
					rewritten[key] = ts // закон чисток: до достоверного fetch не выбрасываем
				}
			default:
				m.rep.add("подписка %q: отметка выключения %q не сматчена ни с одним узлом", subName, key)
				rewritten[key] = ts
			}
		}
		src.DisabledNodes = rewritten
	}
}

// reportUnappliedMarks — предупреждение о картах, которые не к чему
// применить (Т7 шаг 1: «карты отбрасываются с предупреждением»; физический
// снос — шаг 8 под гейтом).
func (m *migrationV7) reportUnappliedMarks(src *Source, subName string) {
	if len(src.DisabledNodes) > 0 {
		m.rep.add("подписка %q: %d отметк(и) выключения не перенесены — нет материализованных узлов", subName, len(src.DisabledNodes))
	}
}

// transferSubUpdateStatus — история fetch из SubMeta → канонический
// SubUpdateStatus (PLAN §1.2). Мостовые поля SubMeta живут до W5.
func (m *migrationV7) transferSubUpdateStatus(src *Source) {
	if src.Meta == nil || src.UpdateStatus != nil {
		return
	}
	meta := src.Meta
	// Истории не было — пустышку не плодим.
	if meta.URLAtFetch == "" && meta.LastFetchedAt == "" && meta.LastStatus == "" &&
		meta.ErrorCount == 0 && meta.LastErrorMsg == "" && meta.LastErrorURL == "" &&
		meta.HTTPStatusCode == 0 && meta.RawBodyBytes == 0 &&
		meta.NodesCountFetched == 0 && !meta.Truncated {
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
		if src.Kind != SourceKindChain || src.Chain == nil || len(src.Chain.Hops) == 0 {
			continue
		}
		if len(src.Node.Hops) > 0 {
			continue
		}
		links := make([]NodeLink, 0, len(src.Chain.Hops))
		for _, hop := range src.Chain.Hops {
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
				m.rep.add("цепочка %q: хоп %q не резолвится — цепочка деградирует fail-closed", src.NodeTagOrLabel(), hop)
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
		if src.Kind != SourceKindSubscription || src.Fold == nil {
			continue
		}
		prefix := ""
		if src.TagPolicy != nil {
			prefix = src.TagPolicy.Prefix
		}
		if src.Fold.HasSelect() {
			known[configtypes.FoldSelectTag(prefix, i)] = true
		}
		if src.Fold.HasAuto() {
			known[configtypes.FoldAutoTag(prefix, i)] = true
		}
	}
	return known
}

// ── шаг 5: detour-тройня → NodeLink ──────────────────────────────

func (m *migrationV7) migrateDetours() {
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		link := m.resolveLegacyDetour(src)
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
			// его и не собирал (ToProxySourceV4 цепочки тройню не возит).
			m.rep.add("цепочка %q: detour-ссылка упразднена моделью v7 — отброшена", src.NodeTagOrLabel())
		}
	}
}

// resolveLegacyDetour — тройня/тег → NodeLink (nil = переносить нечего или
// нерезолвящийся hash оставлен мосту).
func (m *migrationV7) resolveLegacyDetour(src *Source) *NodeLink {
	sid := strings.TrimSpace(src.DetourNodeSourceID)
	dtag := strings.TrimSpace(src.DetourNodeTag)
	hash := strings.TrimSpace(src.DetourNodeHash)

	switch {
	case sid != "":
		target := m.s.FindSource(sid)
		if target == nil {
			m.rep.add("источник %q: detour-ссылка на несуществующий источник %s — деградирует fail-closed", m.sourceName(src), sid)
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
				m.rep.add("источник %q: detour по legacy-хэшу не сматчен с узлом подписки — ссылка оставлена старому пути", m.sourceName(src))
				return nil
			}
			if raw == "" {
				return nil
			}
			if tags, materialized := m.subRawTags[sid]; materialized && !tags[raw] {
				m.rep.add("источник %q: detour-узел %q не найден в подписке — деградирует fail-closed", m.sourceName(src), raw)
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
		m.rep.add("источник %q: detour по legacy-хэшу не сматчен ни с одним узлом — ссылка оставлена старому пути", m.sourceName(src))
		return nil

	case strings.TrimSpace(src.DetourTag) != "":
		return &NodeLink{Tag: strings.TrimSpace(src.DetourTag)}
	}
	return nil
}

func (m *migrationV7) sourceName(src *Source) string {
	if src.Kind == SourceKindSubscription {
		if src.Name != "" {
			return src.Name
		}
		return src.ID
	}
	return src.NodeTagOrLabel()
}

// ── шаг 6: fold → FolderReplace ──────────────────────────────────

func (m *migrationV7) migrateFolds() {
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		if src.Kind != SourceKindSubscription || src.Fold == nil || src.Replace != nil {
			continue
		}
		prefix := ""
		if src.TagPolicy != nil {
			prefix = src.TagPolicy.Prefix
		}
		selectTag := configtypes.FoldSelectTag(prefix, i)
		autoTag := configtypes.FoldAutoTag(prefix, i)

		rep := &FolderReplace{}
		switch src.Fold.EffectiveMode() {
		case configtypes.FoldModeAuto:
			rep.Mode = FolderReplaceAuto
			rep.Tag = autoTag
		case configtypes.FoldModeSelectAuto:
			rep.Mode = FolderReplaceBoth
			rep.Tag = selectTag
			// Пара тегов старой свёртки (`<PFX>select` + `<PFX>auto`) не
			// выражается моделью `<tag>-auto` — двойник получает
			// `<selectTag>-auto`, ссылки переписываются (риск Р2), выбор в
			// кэше ядра у авто-двойника протухает.
			newAuto := rep.Tag + "-auto"
			if autoTag != newAuto {
				m.renames[autoTag] = newAuto
				m.rep.add("свёртка %q: авто-группа переименована %q → %q (модель v7); ссылки переписаны, выбор в кэше ядра для неё будет сброшен", m.sourceName(src), autoTag, newAuto)
			}
		default: // select
			rep.Mode = FolderReplaceManual
			rep.Tag = selectTag
		}
		if src.Fold.HasAuto() && src.Fold.Auto != nil {
			rep.Strategy = src.Fold.Auto.Clone()
		}
		src.Replace = rep
	}
}

// reportLocalDirections — произвольные локальные Направления источника
// (fold-производные уже развёрнуты в Fold/Replace) упраздняются классом.
func (m *migrationV7) reportLocalDirections() {
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		if len(src.Outbounds) == 0 {
			continue
		}
		tags := make([]string, 0, len(src.Outbounds))
		for _, ob := range src.Outbounds {
			tags = append(tags, ob.Tag)
		}
		m.rep.add("источник %q: локальные Направления источника упразднены — %s будут потеряны (создайте глобальные Направления с фильтрами)", m.sourceName(src), strings.Join(tags, ", "))
	}
}

// reportExcludes — шаг 7: одиночный exclude_from_global (без свёртки).
func (m *migrationV7) reportExcludes() {
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		if !src.ExcludeFromGlobal || src.Fold != nil || src.Replace != nil {
			continue
		}
		m.rep.add("источник %q: флаг «исключить из общего списка» упразднён — узлы останутся в пуле кандидатов (сверните источник в группу, если нужна прежняя картина)", m.sourceName(src))
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

	// 5. Позиции цепочек — И легаси-строки (их читает мост до W4), И
	// канонические NodeLink корневого пространства.
	for i := range m.s.Sources {
		src := &m.s.Sources[i]
		if src.Chain != nil {
			for j, hop := range src.Chain.Hops {
				if to, ok := rename(hop); ok {
					src.Chain.Hops[j] = to
				}
			}
		}
		for j := range src.Node.Hops {
			if src.Node.Hops[j].FolderID != "" {
				continue
			}
			if to, ok := rename(src.Node.Hops[j].Tag); ok {
				src.Node.Hops[j].Tag = to
			}
		}
		// 6. Детуры источников — легаси-тег и канонический NodeLink.
		if to, ok := rename(src.DetourTag); ok {
			src.DetourTag = to
		}
		if src.Node.Detour != nil && src.Node.Detour.FolderID == "" {
			if to, ok := rename(src.Node.Detour.Tag); ok {
				src.Node.Detour.Tag = to
			}
		}
	}
}

// ── шаг 8: снос легаси (гейт до W5) ──────────────────────────────

// purgeLegacyAfterMigration — необратимый шаг 8: raw-кэш, карты выключенных,
// легаси-поля моста и defaults (→ настройки приложения). Вызывается ТОЛЬКО
// после успешной записи v7-файла (Load, гейт migrationPurgesLegacy) —
// до неё исходный материал не трогается (риск Р5).
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

	for i := range s.Sources {
		src := &s.Sources[i]
		if src.Kind == SourceKindSubscription && lc.SubsDir != "" {
			if p, err := rawPath(lc.SubsDir, src.ID); err == nil {
				_ = removeFileIfExists(p)
			}
		}
		src.NodeTag = ""
		src.URI = ""
		src.ConfigJSON = nil
		src.Chain = nil
		src.Outbounds = nil
		src.ExcludeFromGlobal = false
		src.ExposeGroupTagsToGlobal = false
		src.Fold = nil
		src.DetourTag = ""
		src.DetourNodeSourceID = ""
		src.DetourNodeTag = ""
		src.DetourNodeHash = ""
		src.DetourNodeLabel = ""
		src.DisabledNodes = nil
		if src.TagPolicy != nil {
			src.TagPolicy.Mask = ""
			if src.TagPolicy.IsZero() {
				src.TagPolicy = nil
			}
		}
		if src.Meta != nil {
			src.Meta.URLAtFetch = ""
			src.Meta.LastFetchedAt = ""
			src.Meta.LastStatus = ""
			src.Meta.ErrorCount = 0
			src.Meta.LastErrorMsg = ""
			src.Meta.LastErrorURL = ""
			src.Meta.HTTPStatusCode = 0
			src.Meta.RawBodyBytes = 0
			src.Meta.NodesCountFetched = 0
			src.Meta.Truncated = false
			src.Meta.PreviewNodes = nil
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
