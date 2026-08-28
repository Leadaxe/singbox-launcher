package state

import (
	"singbox-launcher/core/config/configtypes"
)

// syncLegacyFromConnections — обратная операция: используется на Load v5,
// чтобы заполнить ParserConfig.Proxies из Connections.Sources для backward-
// compat callsite'ов.
//
//   - Subscription Source → ProxySource{source, skip, outbounds, tag_*, ...}
//   - Server Source → ProxySource{connections:[uri], tag_mask=label}
//
// tag_mask=label на server — гарантирует, что parser выставит итоговый tag
// строго равным label (без вычислений prefix+fragment, как на migration v4→v5).
func syncLegacyFromConnections(s *State) {
	proxies := make([]configtypes.ProxySource, 0, len(s.Connections.Sources))
	for _, src := range s.Connections.Sources {
		switch src.Type {
		case SourceTypeSubscription:
			ps := configtypes.ProxySource{
				ID:                      src.ID,    // SPEC 112-A: адресат ссылок на узлы
				Label:                   src.Label, // только для текстов диагностики
				Source:                  src.URL,
				Skip:                    src.Skip,
				Outbounds:               src.Outbounds,
				ExcludeFromGlobal:       src.ExcludeFromGlobal,
				ExposeGroupTagsToGlobal: src.ExposeGroupTagsToGlobal,
				Fold:                    src.Fold, // SPEC 108
				Disabled:                !src.Enabled,
				DetourTag:               src.DetourTag,
				DetourNodeSourceID:      src.DetourNodeSourceID, // SPEC 112-A
				DetourNodeTag:           src.DetourNodeTag,      // SPEC 112
				DetourNodeHash:          src.DetourNodeHash,     // legacy, мигрирует на сборке
				DetourNodeLabel:         src.DetourNodeLabel,    // SPEC 101
				// SPEC 115: сообщение провайдера — только для диагностики,
				// как Label. Провозится здесь же: дальше по конвейеру
				// метаданных источника нет.
				ProviderAnnounce: src.announceMessage(),
			}
			if src.Tag != nil {
				ps.TagPrefix = src.Tag.Prefix
				ps.TagPostfix = src.Tag.Postfix
				ps.TagMask = src.Tag.Mask
			}
			proxies = append(proxies, ps)

		case SourceTypeServer:
			ps := configtypes.ProxySource{
				ID:                 src.ID,    // SPEC 112-A: адресат ссылок на узлы
				Label:              src.Label, // только для текстов диагностики
				Connections:        []string{src.URI},
				TagMask:            src.NodeTagOrLabel(), // тег узла, не подпись
				ExcludeFromGlobal:  src.ExcludeFromGlobal,
				Disabled:           !src.Enabled,
				DetourTag:          src.DetourTag,
				DetourNodeSourceID: src.DetourNodeSourceID, // SPEC 112-A
				DetourNodeTag:      src.DetourNodeTag,      // SPEC 112
				DetourNodeHash:     src.DetourNodeHash,     // legacy, мигрирует на сборке
				DetourNodeLabel:    src.DetourNodeLabel,    // SPEC 101
				ConfigJSON:         src.ConfigJSON,         // ручной outbound JSON
			}
			proxies = append(proxies, ps)

		case SourceTypeChain:
			// SPEC 110: у цепочки нет ни URL, ни URI — TagMask несёт ТЕГ
			// её узла (NodeTag), на который ссылаются фильтры Направлений
			// и позиции других цепочек.
			proxies = append(proxies, configtypes.ProxySource{
				ID:                src.ID,    // SPEC 112-A: адресат ссылок на узлы
				Label:             src.Label, // только для текстов диагностики
				TagMask:           src.NodeTagOrLabel(),
				ExcludeFromGlobal: src.ExcludeFromGlobal,
				Disabled:          !src.Enabled,
				Chain:             src.Chain,
			})
		}
	}

	s.ParserConfig.ParserConfig.Version = configtypes.ParserConfigVersion
	s.ParserConfig.ParserConfig.Proxies = proxies
	if s.Connections.Outbounds != nil {
		s.ParserConfig.ParserConfig.Outbounds = append([]configtypes.Direction(nil), s.Connections.Outbounds...)
	} else {
		s.ParserConfig.ParserConfig.Outbounds = []configtypes.Direction{}
	}
	s.ParserConfig.ParserConfig.Parser.Reload = s.Connections.Defaults.Reload
}
