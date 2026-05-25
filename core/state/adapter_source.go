// File adapter_source.go — Source → ProxySource (legacy view) helper.
//
// Note: основной адаптер ParserConfig ↔ Connections живёт в adapter.go.
// Эта функция — single-source конверсия (один Source → ProxySource) для
// callsite'ов которые работают с одиночными Source.
package state

import (
	"singbox-launcher/core/config/configtypes"
)

// ToProxySourceV4 — конвертит Source в legacy configtypes.ProxySource
// для совместимости с существующим парсером (core/config/subscription).
//
//   - subscription → ProxySource{Source, Skip, Outbounds, Tag*, Disabled, ...}
//   - server       → ProxySource{Connections:[URI], TagMask=Label, Disabled, ExcludeFromGlobal}
//
// Для server-source форсим TagMask = Label, чтобы парсер выставил итоговый
// node tag строго равным label (без вычислений prefix+fragment как раньше).
func (s *Source) ToProxySourceV4() configtypes.ProxySource {
	if s == nil {
		return configtypes.ProxySource{}
	}
	switch s.Type {
	case SourceTypeSubscription:
		ps := configtypes.ProxySource{
			Source:                  s.URL,
			Skip:                    s.Skip,
			Outbounds:               s.Outbounds,
			ExcludeFromGlobal:       s.ExcludeFromGlobal,
			ExposeGroupTagsToGlobal: s.ExposeGroupTagsToGlobal,
			Disabled:                !s.Enabled,
		}
		if s.Tag != nil {
			ps.TagPrefix = s.Tag.Prefix
			ps.TagPostfix = s.Tag.Postfix
			ps.TagMask = s.Tag.Mask
		}
		return ps

	case SourceTypeServer:
		return configtypes.ProxySource{
			Connections:       []string{s.URI},
			TagMask:           s.Label,
			ExcludeFromGlobal: s.ExcludeFromGlobal,
			Disabled:          !s.Enabled,
		}
	}
	return configtypes.ProxySource{}
}
