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
				Source:                  src.URL,
				Skip:                    src.Skip,
				Outbounds:               src.Outbounds,
				ExcludeFromGlobal:       src.ExcludeFromGlobal,
				ExposeGroupTagsToGlobal: src.ExposeGroupTagsToGlobal,
				Disabled:                !src.Enabled,
			}
			if src.Tag != nil {
				ps.TagPrefix = src.Tag.Prefix
				ps.TagPostfix = src.Tag.Postfix
				ps.TagMask = src.Tag.Mask
			}
			proxies = append(proxies, ps)

		case SourceTypeServer:
			ps := configtypes.ProxySource{
				Connections:       []string{src.URI},
				TagMask:           src.Label, // force tag = label
				ExcludeFromGlobal: src.ExcludeFromGlobal,
				Disabled:          !src.Enabled,
			}
			proxies = append(proxies, ps)
		}
	}

	s.ParserConfig.ParserConfig.Version = configtypes.ParserConfigVersion
	s.ParserConfig.ParserConfig.Proxies = proxies
	if s.Connections.Outbounds != nil {
		s.ParserConfig.ParserConfig.Outbounds = append([]configtypes.OutboundConfig(nil), s.Connections.Outbounds...)
	} else {
		s.ParserConfig.ParserConfig.Outbounds = []configtypes.OutboundConfig{}
	}
	s.ParserConfig.ParserConfig.Parser.Reload = s.Connections.Defaults.Reload
}
