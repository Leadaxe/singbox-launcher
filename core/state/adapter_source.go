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
		return ps

	case SourceKindChain:
		// SPEC 110: цепочка не имеет ни URL, ни URI — только позиции.
		// TagMask несёт ТЕГ узла, а не подпись: на тег цепочки ссылаются
		// фильтры Направлений и позиции других цепочек.
		return configtypes.ProxySource{
			ID:                s.ID,
			Label:             s.displayName(),
			TagMask:           s.NodeTagOrLabel(),
			ExcludeFromGlobal: s.ExcludeFromGlobal,
			Disabled:          !s.Enabled,
			Chain:             s.Chain, // TEMPORARY BRIDGE: []NodeLink hops — W2/W4
		}
	}

	// folder / auto: legacy-конвейер их не собирает до W4 — отдаём
	// выключенный плейсхолдер, сохраняя позиции индексного инварианта.
	return configtypes.ProxySource{
		ID:       s.ID,
		Label:    s.displayName(),
		Disabled: true,
	}
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
		if s.Replace.Strategy != nil {
			strat := *s.Replace.Strategy
			fold.Auto = &strat
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
