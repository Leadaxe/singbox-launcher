package state

import (
	"singbox-launcher/core/config/configtypes"
)

// syncConnectionsFromLegacy обновляет State.Connections на основе
// State.ParserConfig.Proxies. Используется на Save: UI/код мутирует
// legacy-view, мы переносим изменения в canonical v5-секцию.
//
// Стратегия preservation:
//   - Subscription source'ы матчатся по URL — old.id, old.Meta, old.MaxNodes,
//     old.Update, old.Label сохраняются;
//   - Server source'ы матчатся по URI — same;
//   - Новые source'ы (нет matching url/uri в old) получают свежий ULID;
//   - Source'ы которых больше нет в proxies — выпадают из Connections.
//
// Order: повторяет порядок ParserConfig.Proxies (UI rearrangement сохраняется).
//
// Edge case: ParserConfig.Proxies == nil (callsite напрямую мутировал
// Connections, не пройдя через legacy view) → сохраняем Connections как
// canonical, не перезаписываем. Это нужно для test'ов и для будущих
// callsite'ов которые работают сразу с v5-моделью.
func syncConnectionsFromLegacy(s *State) {
	// Если legacy view вообще не был инициализирован (nil-slice), значит
	// caller работает только с Connections. Не трогаем.
	if s.ParserConfig.ParserConfig.Proxies == nil {
		// Sync Outbounds/Defaults в обратную сторону, чтобы legacy-view
		// не был совсем пустым (для UI которое может его открыть).
		syncLegacyFromConnections(s)
		// Восстанавливаем флаг "Proxies is nil" для следующего Save,
		// чтобы на повторных Save'ах мы не overwrite'или Connections.
		// Реально syncLegacyFromConnections выставит Proxies = make([]..., 0)
		// или non-nil; но в этом edge case caller обычно делает один Save,
		// после чего Load восстановит обе view.
		return
	}

	old := s.Connections.Sources

	oldByURL := make(map[string]Source, len(old))
	oldByURI := make(map[string]Source, len(old))
	for _, src := range old {
		switch src.Type {
		case SourceTypeSubscription:
			if src.URL != "" {
				oldByURL[src.URL] = src
			}
		case SourceTypeServer:
			if src.URI != "" {
				oldByURI[src.URI] = src
			}
		}
	}

	newSources := make([]Source, 0, len(s.ParserConfig.ParserConfig.Proxies))
	for _, p := range s.ParserConfig.ParserConfig.Proxies {
		// 1. type=subscription
		if p.Source != "" {
			tag := buildTagSpecFromLegacy(p.TagPrefix, p.TagPostfix, p.TagMask)
			src := Source{
				Type:                    SourceTypeSubscription,
				Enabled:                 !p.Disabled,
				URL:                     p.Source,
				Skip:                    p.Skip,
				Tag:                     tag,
				Outbounds:               p.Outbounds,
				ExcludeFromGlobal:       p.ExcludeFromGlobal,
				ExposeGroupTagsToGlobal: p.ExposeGroupTagsToGlobal,
			}
			if existing, ok := oldByURL[p.Source]; ok {
				src.ID = existing.ID
				src.Label = existing.Label
				src.Meta = existing.Meta
				src.MaxNodes = existing.MaxNodes
				src.Update = existing.Update
			}
			if src.ID == "" {
				src.ID = MakeULID()
			}
			newSources = append(newSources, src)
		}

		// 2. type=server (one per URI in connections[])
		for j, uri := range p.Connections {
			src := Source{
				Type:              SourceTypeServer,
				Enabled:           !p.Disabled,
				URI:               uri,
				ExcludeFromGlobal: p.ExcludeFromGlobal,
			}
			if existing, ok := oldByURI[uri]; ok {
				src.ID = existing.ID
				src.Label = existing.Label
			}
			if src.Label == "" {
				src.Label = serverLabelFromLegacy(uri, j+1, p.TagPrefix, p.TagPostfix)
			}
			if src.ID == "" {
				src.ID = MakeULID()
			}
			newSources = append(newSources, src)
		}
	}

	s.Connections.Sources = newSources

	// Outbounds + Defaults: legacy parser_config.outbounds → connections.outbounds.
	if s.ParserConfig.ParserConfig.Outbounds != nil {
		s.Connections.Outbounds = append([]configtypes.OutboundConfig(nil), s.ParserConfig.ParserConfig.Outbounds...)
	} else if s.Connections.Outbounds == nil {
		s.Connections.Outbounds = []configtypes.OutboundConfig{}
	}

	// Defaults.Reload — следуем legacy parser_config.parser.reload.
	s.Connections.Defaults.Reload = s.ParserConfig.ParserConfig.Parser.Reload
	if s.Connections.Defaults.MaxNodes == 0 {
		s.Connections.Defaults.MaxNodes = DefaultMaxNodes
	}
}
