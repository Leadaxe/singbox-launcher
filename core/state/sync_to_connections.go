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
	// Цепочка сопоставляется по ТЕГУ: другого стабильного ключа у неё нет
	// (позиции пользователь правит, и матчинг по ним менял бы ID на каждой
	// правке маршрута — вместе со всем, что на ID завязано). Именно по
	// тегу, а не по подписи: подпись правится свободно, и ключом она
	// теряла бы ID при каждом переименовании.
	oldByChainTag := make(map[string]Source, len(old))
	// SPEC 112-A: legacy-форма теперь везёт ULID (ProxySource.ID), и он —
	// точнее любого матчинга по URL/URI: правка адреса подписки или URI сервера
	// больше не выдаёт источнику новый ID, из-под которого уехали бы ссылки на
	// его узел. URL/URI-карты остаются запасным путём для форм, где ID нет
	// (ручная правка вкладки JSON, состояния до этой версии).
	oldByID := make(map[string]Source, len(old))
	for _, src := range old {
		if src.ID != "" {
			oldByID[src.ID] = src
		}
		if src.Type == SourceTypeChain {
			oldByChainTag[src.NodeTagOrLabel()] = src
			continue
		}
		switch src.Type {
		case SourceTypeSubscription:
			if src.URL != "" {
				oldByURL[src.URL] = src
			}
		case SourceTypeServer:
			if src.URI != "" {
				oldByURI[src.URI] = src
			} else if len(src.ConfigJSON) > 0 {
				// Server-source без URI (только ручной config_json) матчится
				// по телу JSON — иначе он получал бы новый ULID на каждом Save.
				oldByURI[serverConfigJSONKey(src.ConfigJSON)] = src
			}
		}
	}

	newSources := make([]Source, 0, len(s.ParserConfig.ParserConfig.Proxies))
	for _, p := range s.ParserConfig.ParserConfig.Proxies {
		// 0. type=chain (SPEC 110) — ни URL, ни Connections, только позиции.
		// Проверяется первым: у цепочки Source пуст, и без этой ветки она
		// молча выпала бы из Connections на Save.
		if p.Chain != nil {
			src := Source{
				Type:              SourceTypeChain,
				Enabled:           !p.Disabled,
				NodeTag:           p.TagMask,
				ExcludeFromGlobal: p.ExcludeFromGlobal,
				Chain:             p.Chain,
			}
			if existing, ok := oldByID[p.ID]; ok && p.ID != "" {
				src.ID = existing.ID
				src.Label = existing.Label
			} else if existing, ok := oldByChainTag[p.TagMask]; ok {
				src.ID = existing.ID
				// Подпись живёт только здесь: в legacy-форме её негде
				// хранить, и без переноса она терялась бы на каждом Save.
				src.Label = existing.Label
			}
			if src.ID == "" {
				src.ID = MakeULID()
			}
			newSources = append(newSources, src)
			continue
		}

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
				Fold:                    p.Fold, // SPEC 108
				DetourTag:               p.DetourTag,
				DetourNodeSourceID:      p.DetourNodeSourceID, // SPEC 112-A
				DetourNodeTag:           p.DetourNodeTag,      // SPEC 112
				DetourNodeHash:          p.DetourNodeHash,     // legacy, мигрирует на сборке
				DetourNodeLabel:         p.DetourNodeLabel,    // SPEC 101
			}
			carryOver := func(existing Source) {
				src.ID = existing.ID
				src.Label = existing.Label
				src.Meta = existing.Meta
				src.MaxNodes = existing.MaxNodes
				src.Update = existing.Update
			}
			if existing, ok := oldByID[p.ID]; ok && p.ID != "" {
				carryOver(existing)
			} else if existing, ok := oldByURL[p.Source]; ok {
				carryOver(existing)
			}
			if src.ID == "" {
				src.ID = MakeULID()
			}
			newSources = append(newSources, src)
		}

		// 2. type=server (one per URI in connections[])
		//
		// Ручной config_json (см. Source.ConfigJSON): источник может вообще
		// не иметь URI — синтезируем одну запись с пустым URI, иначе он
		// выпал бы из Connections на Save. JSON прикрепляется только когда
		// запись одна: на legacy multi-connection источник один ручной
		// объект не размножаем.
		conns := p.Connections
		if len(conns) == 0 && len(p.ConfigJSON) > 0 {
			conns = []string{""}
		}
		for j, uri := range conns {
			src := Source{
				Type:               SourceTypeServer,
				Enabled:            !p.Disabled,
				URI:                uri,
				ExcludeFromGlobal:  p.ExcludeFromGlobal,
				DetourTag:          p.DetourTag,
				DetourNodeSourceID: p.DetourNodeSourceID, // SPEC 112-A
				DetourNodeTag:      p.DetourNodeTag,      // SPEC 112
				DetourNodeHash:     p.DetourNodeHash,     // legacy, мигрирует на сборке
				DetourNodeLabel:    p.DetourNodeLabel,    // SPEC 101
			}
			if len(conns) == 1 {
				src.ConfigJSON = p.ConfigJSON
			}
			key := uri
			if key == "" && len(src.ConfigJSON) > 0 {
				key = serverConfigJSONKey(src.ConfigJSON)
			}
			// TagMask legacy-формы — это тег узла (ToProxySourceV4 кладёт
			// туда NodeTagOrLabel), поэтому обратно он и возвращается тегом.
			src.NodeTag = p.TagMask
			// ID берётся из legacy-формы только у ОДНОГО источника на запись:
			// legacy multi-connection ProxySource разворачивается в несколько
			// Source, и раздать им один ULID значило бы склеить разные узлы под
			// одной идентичностью — ссылки на них перестали бы различаться.
			if existing, ok := oldByID[p.ID]; ok && p.ID != "" && len(conns) == 1 {
				src.ID = existing.ID
				src.Label = existing.Label
			} else if existing, ok := oldByURI[key]; ok && key != "" {
				src.ID = existing.ID
				src.Label = existing.Label
			}
			if src.NodeTag == "" {
				src.NodeTag = serverLabelFromLegacy(uri, j+1, p.TagPrefix, p.TagPostfix)
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
		s.Connections.Outbounds = append([]configtypes.Direction(nil), s.ParserConfig.ParserConfig.Outbounds...)
	} else if s.Connections.Outbounds == nil {
		s.Connections.Outbounds = []configtypes.Direction{}
	}

	// Defaults.Reload — следуем legacy parser_config.parser.reload.
	s.Connections.Defaults.Reload = s.ParserConfig.ParserConfig.Parser.Reload
	if s.Connections.Defaults.MaxNodes == 0 {
		s.Connections.Defaults.MaxNodes = DefaultMaxNodes
	}
}
