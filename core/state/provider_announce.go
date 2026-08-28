// File provider_announce.go — ProviderAnnounce struct + IsEmpty helper.
//
// Lives in state (not in core/config/subscription where it semantically
// originates) so SubscriptionMeta can carry a `*ProviderAnnounce` field
// without state → subscription circular import: subscription already imports
// state for SubscriptionMeta, and we need the symmetric direction.
//
// Subscription's ParseAnnounce / FetchHTTPError continue to construct values
// of this type; UI consumers (source_error_dialog) read the same struct
// directly from `Source.Meta.ProviderAnnounce` after a failed Update or a
// success-with-announce fetch.
package state

import "strings"

// ProviderAnnounce — header-derived rationale for an empty / errored
// subscription response, in human-readable form. Surfaced to the user via
// the source-row icon + dialog (SPEC 061 Phase 3).
//
// Fields are optional; callers check IsEmpty before storing / displaying.
//
// HWID semantics (SPEC 061 §"Response headers" §4):
//
//   - HWIDActive — provider has HWID-binding turned on. Informational badge.
//   - HWIDNotSupported — binding on AND we didn't send X-Hwid. After Phase 2
//     UA + headers land this flag should never appear from a fresh install;
//     surviving panels surface it for debugging.
//   - HWIDMaxDevicesReached — user is out of device slots. Actionable: the
//     dialog points the user at Announce-Url ("manage devices in @bot").
//   - HWIDLimit — legacy alias of MaxDevicesReached (v2RayTun / NashVPN). We
//     mirror both ways in ParseAnnounce so the UI can read either field.
type ProviderAnnounce struct {
	Message      string `json:"message,omitempty"`       // decoded `Announce` (base64 → UTF-8)
	URL          string `json:"url,omitempty"`           // `Announce-Url`
	ProfileTitle string `json:"profile_title,omitempty"` // `Profile-Title` decoded — context ("which subscription")

	HWIDActive            bool `json:"hwid_active,omitempty"`
	HWIDNotSupported      bool `json:"hwid_not_supported,omitempty"`
	HWIDMaxDevicesReached bool `json:"hwid_max_devices_reached,omitempty"`
	HWIDLimit             bool `json:"hwid_limit,omitempty"` // legacy alias of MaxDevicesReached
}

// MaxAnnounceRunes — сколько символов сообщения провайдера показываем.
//
// Текст чужой и в длине не ограничен ничем: провайдер вправе прислать хоть
// страницу. Показывать её целиком нельзя — она вытеснит собственную
// диагностику лаунчера и раздует строку списка, — но и молча резать до трёх
// слов тоже: сообщение обязано остаться читаемым. 500 символов покрывают
// живые случаи с запасом.
const MaxAnnounceRunes = 500

// AnnounceMessage — сообщение провайдера, обрезанное до MaxAnnounceRunes.
//
// ГРАНИЦА ДОВЕРИЯ: это текст, написанный чужой стороной. Он показывается как
// ДАННЫЕ — без интерпретации, без превращения в вывод лаунчера и без попыток
// вычитать из него состояние подписки. Единственная обработка — схлопывание
// переводов строки (иначе одна строка UI распадается на семь) и обрезка.
func (a *ProviderAnnounce) AnnounceMessage() string {
	if a == nil {
		return ""
	}
	msg := strings.TrimSpace(a.Message)
	if msg == "" {
		return ""
	}
	msg = strings.Join(strings.Fields(msg), " ")
	runes := []rune(msg)
	if len(runes) > MaxAnnounceRunes {
		msg = strings.TrimSpace(string(runes[:MaxAnnounceRunes])) + "…"
	}
	return msg
}

// IsEmpty — true if no field is populated (provider sent no announce headers
// and the caller should fall back to a generic "empty body" message).
func (a *ProviderAnnounce) IsEmpty() bool {
	if a == nil {
		return true
	}
	return a.Message == "" &&
		a.URL == "" &&
		a.ProfileTitle == "" &&
		!a.HWIDActive &&
		!a.HWIDNotSupported &&
		!a.HWIDMaxDevicesReached &&
		!a.HWIDLimit
}
