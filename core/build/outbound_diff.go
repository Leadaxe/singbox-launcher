// Package build — File outbound_diff.go (SPEC 058-R-N).
//
// OutboundFieldDiff — field-level diff между form_value (юзерский edit) и
// merged_base (resolved template body + active preset patches).
//
// Используется Edit dialog'ом на Save: вычисляем минимальный USER patch
// который, applied поверх merged_base, даст form_value. Patch хранится в
// outbound.Updates[] с Ref=RefUser, всегда последним (см. sync reorder).
package build

import (
	"reflect"

	"singbox-launcher/core/config/configtypes"
)

// OutboundFieldDiff возвращает map (patch) изменённых полей между form и base.
//
// Семантика по полям (SPEC 058 §"Field-level diff правила"):
//
//	tag, type      — immutable для referenced entries (изменения игнорируются)
//	filters        — equal → skip; иначе replace целиком
//	options        — per-key diff: пишем только изменённые ключи
//	addOutbounds   — slice equal (order matters here для простоты) → skip; иначе replace
//	preferredDefault — replace целиком
//	wizard         — replace целиком
//	comment        — equal → skip; пустая в form + non-empty в base → пишем "" явно
//
// Если diff пуст — возвращает nil (no-op Save → не создаём USER patch).
//
// Используется только для referenced entries. Для direct entries diff не нужен
// (body хранится inline, перезаписывается напрямую).
func OutboundFieldDiff(form, base configtypes.OutboundConfig) map[string]interface{} {
	patch := make(map[string]interface{})

	// Filters — equal? skip. Else replace целиком (map equality через reflect).
	if !outboundMapsEqual(form.Filters, base.Filters) {
		if form.Filters != nil {
			patch["filters"] = form.Filters
		} else {
			// Юзер очистил фильтры — пишем пустой map для override.
			patch["filters"] = map[string]interface{}{}
		}
	}

	// Options — per-key diff.
	if optDiff := mapDiff(form.Options, base.Options); len(optDiff) > 0 {
		patch["options"] = optDiff
	}

	// AddOutbounds — strict slice equal (order matters). Replace целиком при diff.
	if !stringSlicesEqual(form.AddOutbounds, base.AddOutbounds) {
		if form.AddOutbounds == nil {
			patch["addOutbounds"] = []interface{}{}
		} else {
			arr := make([]interface{}, len(form.AddOutbounds))
			for i, s := range form.AddOutbounds {
				arr[i] = s
			}
			patch["addOutbounds"] = arr
		}
	}

	// PreferredDefault — replace целиком.
	if !outboundMapsEqual(form.PreferredDefault, base.PreferredDefault) {
		if form.PreferredDefault != nil {
			patch["preferredDefault"] = form.PreferredDefault
		} else {
			patch["preferredDefault"] = map[string]interface{}{}
		}
	}

	// Comment — string equal. Если разный — пишем (включая пустую строку
	// для явного override non-empty base).
	if form.Comment != base.Comment {
		patch["comment"] = form.Comment
	}

	if len(patch) == 0 {
		return nil
	}
	return patch
}

// outboundMapsEqual — deep equality для map[string]interface{}.
func outboundMapsEqual(a, b map[string]interface{}) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// mapDiff — возвращает per-key diff: ключи изменённых/новых значений из form.
// Удалённые ключи (в base но не в form) тоже включаются как nil (для override).
//
// Если diff пуст — nil.
func mapDiff(form, base map[string]interface{}) map[string]interface{} {
	if len(form) == 0 && len(base) == 0 {
		return nil
	}
	out := make(map[string]interface{})
	// Изменённые / новые keys.
	for k, v := range form {
		if bv, ok := base[k]; !ok || !reflect.DeepEqual(v, bv) {
			out[k] = v
		}
	}
	// Удалённые keys — пишем nil как explicit override.
	for k := range base {
		if _, ok := form[k]; !ok {
			out[k] = nil
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// stringSlicesEqual — strict slice equality.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// UpsertUserPatch — обновляет/добавляет/удаляет USER patch в updates[] стеке.
// Если patch пуст (nil/empty) — удаляет existing USER entry (no-op Save).
// Если присутствует USER entry — replace (всегда один USER на outbound).
// Если отсутствует — append (sync.reorderUpdates переместит в конец).
//
// Используется Edit dialog'ом после OutboundFieldDiff.
func UpsertUserPatch(updates []configtypes.OutboundUpdate, patch map[string]interface{}) []configtypes.OutboundUpdate {
	// Remove existing USER patch (if any).
	out := make([]configtypes.OutboundUpdate, 0, len(updates)+1)
	for _, u := range updates {
		if u.Ref != configtypes.RefUser {
			out = append(out, u)
		}
	}
	// Append new USER patch if non-empty.
	if len(patch) > 0 {
		out = append(out, configtypes.OutboundUpdate{
			Ref:   configtypes.RefUser,
			Patch: patch,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
