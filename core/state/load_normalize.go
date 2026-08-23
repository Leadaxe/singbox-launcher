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
func sanitizeOutboundRefs(outbounds *[]configtypes.Direction) {
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
				// SPEC 104: USER-патч из одних пустых значений ({} / [] / "")
				// — артефакт формы, сохранённой без изменений (pre-058). Он
				// затирает пресетные патчи (russian → `!/(🇷🇺)/i` на
				// proxy-out) и воспроизводит себя на каждом Save. Чистим при
				// загрузке, не дожидаясь, пока пользователь откроет запись.
				//
				// Explicit при этом означает обратное: патч записан
				// сегодняшней формой, и пустое значение в нём — это
				// «пользователь стёр поле». Снять такой значило бы вернуть
				// настройку, которую он убрал (см. OutboundUpdate.Explicit).
				if u.Ref == configtypes.RefUser && !u.Explicit && isAllEmptyPatch(u.Patch) {
					log.Printf("state: dropping empty USER patch on outbound %q (it only masked template/preset values)", ob.Tag)
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

// isAllEmptyPatch — патч, в котором нет ни одного осмысленного значения.
//
// Пустыми считаем nil (кроме ключа `auto`: явный null там означает
// «двойник выключен»), "", {} и []. Нулевые числа и false — осмысленны
// (`disabled:false` включает направление обратно), поэтому патч с ними
// пустым не считается. Зеркало build.dropEmptyOverrides; живёт здесь
// отдельно, потому что state — leaf-пакет и build импортировать не может.
func isAllEmptyPatch(patch map[string]interface{}) bool {
	if len(patch) == 0 {
		return true
	}
	for k, v := range patch {
		switch t := v.(type) {
		case nil:
			if k == "auto" {
				return false
			}
		case string:
			if t != "" {
				return false
			}
		case map[string]interface{}:
			if len(t) > 0 {
				return false
			}
		case []interface{}:
			if len(t) > 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
