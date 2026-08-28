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
			ID:                      s.ID,    // SPEC 112-A: адресат ссылок на узлы
			Label:                   s.Label, // только для текстов диагностики
			Source:                  s.URL,
			Skip:                    s.Skip,
			Outbounds:               s.Outbounds,
			ExcludeFromGlobal:       s.ExcludeFromGlobal,
			ExposeGroupTagsToGlobal: s.ExposeGroupTagsToGlobal,
			Fold:                    s.Fold, // SPEC 108
			Disabled:                !s.Enabled,
			DetourTag:               s.DetourTag,
			DetourNodeSourceID:      s.DetourNodeSourceID, // SPEC 112-A
			DetourNodeTag:           s.DetourNodeTag,      // SPEC 112
			DetourNodeHash:          s.DetourNodeHash,     // legacy, мигрирует на сборке
			DetourNodeLabel:         s.DetourNodeLabel,    // SPEC 101
			DisabledNodes:           s.DisabledNodes,      // SPEC 094 D4
			// SPEC 115: сообщение провайдера — тоже только для диагностики.
			// Провозится ЗДЕСЬ, потому что дальше по конвейеру метаданных
			// источника уже нет: разбору достаётся только сборочная форма.
			ProviderAnnounce: s.announceMessage(),
		}
		if s.Tag != nil {
			ps.TagPrefix = s.Tag.Prefix
			ps.TagPostfix = s.Tag.Postfix
			ps.TagMask = s.Tag.Mask
		}
		return ps

	case SourceTypeServer:
		return configtypes.ProxySource{
			ID:                 s.ID,    // SPEC 112-A: адресат ссылок на узлы
			Label:              s.Label, // только для текстов диагностики
			Connections:        []string{s.URI},
			TagMask:            s.NodeTagOrLabel(),
			ExcludeFromGlobal:  s.ExcludeFromGlobal,
			Disabled:           !s.Enabled,
			DetourTag:          s.DetourTag,
			DetourNodeSourceID: s.DetourNodeSourceID, // SPEC 112-A
			DetourNodeTag:      s.DetourNodeTag,      // SPEC 112
			DetourNodeHash:     s.DetourNodeHash,     // legacy, мигрирует на сборке
			DetourNodeLabel:    s.DetourNodeLabel,    // SPEC 101
			ConfigJSON:         s.ConfigJSON,         // ручной outbound JSON
		}

	case SourceTypeChain:
		// SPEC 110: цепочка не имеет ни URL, ни URI — только позиции.
		// TagMask несёт ТЕГ узла (NodeTag), а не подпись: на тег цепочки
		// ссылаются фильтры Направлений и позиции других цепочек, поэтому
		// переименование в списке его менять не должно.
		return configtypes.ProxySource{
			ID:                s.ID,    // SPEC 112-A: адресат ссылок на узлы
			Label:             s.Label, // только для текстов диагностики
			TagMask:           s.NodeTagOrLabel(),
			ExcludeFromGlobal: s.ExcludeFromGlobal,
			Disabled:          !s.Enabled,
			Chain:             s.Chain,
		}
	}
	return configtypes.ProxySource{}
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
