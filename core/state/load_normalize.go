package state

import (
	"log"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// normalizeNilSlices — приводим nil-slices к пустым для удобства callsite'ов.
func normalizeNilSlices(s *State) {
	if s.ConfigParams == nil {
		s.ConfigParams = []ConfigParam{}
	}
	if s.CustomRules == nil {
		s.CustomRules = []CustomRule{}
	}
	if s.Connections.Sources == nil {
		s.Connections.Sources = []Source{}
	}
	sanitizeOutboundRefs(&s.Connections.Outbounds)
	for i := range s.Connections.Sources {
		sanitizeOutboundRefs(&s.Connections.Sources[i].Outbounds)
	}
}

// sanitizeOutboundRefs валидирует позиционные правила для `ref` полей в outbound
// entries (SPEC 058-R-N). Лениво — дропает невалидные entries / updates и логирует
// в stderr, вместо fail-load. Это безопаснее против hand-edited state.json и
// forward-compat с будущими sentinel'ами.
//
// Правила:
//   - `outbounds[].ref`: принимаем "" (direct), RefTemplate (#TEMPLATE#), либо
//     любое непустое значение НЕ начинающееся на `#` (preset_id). Reject RefUser
//     и unknown #...# sentinel'ы.
//   - `outbounds[].updates[].ref`: принимаем RefUser (#USER#) или любое
//     непустое значение НЕ начинающееся на `#` (preset_id). Reject "",
//     RefTemplate, unknown #...# sentinel'ы.
func sanitizeOutboundRefs(outbounds *[]configtypes.OutboundConfig) {
	if outbounds == nil || *outbounds == nil {
		return
	}
	cleaned := (*outbounds)[:0]
	for _, ob := range *outbounds {
		if !validEntryRef(ob.Ref) {
			log.Printf("state: dropping outbound %q with invalid ref=%q (sentinel rules SPEC 058)", ob.Tag, ob.Ref)
			continue
		}
		if len(ob.Updates) > 0 {
			validUpdates := ob.Updates[:0]
			for _, u := range ob.Updates {
				if !validUpdateRef(u.Ref) {
					log.Printf("state: dropping update on outbound %q with invalid ref=%q", ob.Tag, u.Ref)
					continue
				}
				validUpdates = append(validUpdates, u)
			}
			ob.Updates = validUpdates
		}
		cleaned = append(cleaned, ob)
	}
	*outbounds = cleaned
}

// validEntryRef — допустимые значения state.outbounds[].ref:
// "", RefTemplate, или любая non-#-prefixed строка (preset_id).
func validEntryRef(ref string) bool {
	if ref == "" || ref == configtypes.RefTemplate {
		return true
	}
	if strings.HasPrefix(ref, "#") {
		// #USER# (patch-level only) или unknown sentinel
		return false
	}
	return true // preset_id; validation regex живёт в template loader
}

// validUpdateRef — допустимые значения state.outbounds[].updates[].ref:
// RefUser или любая non-#-prefixed строка (preset_id).
func validUpdateRef(ref string) bool {
	if ref == configtypes.RefUser {
		return true
	}
	if ref == "" || strings.HasPrefix(ref, "#") {
		return false
	}
	return true
}
