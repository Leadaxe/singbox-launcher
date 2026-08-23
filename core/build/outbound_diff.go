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
//	options        — per-key diff: пишем только изменённые/новые ключи;
//	                 отсутствующий в form ключ = "оверрайда нет" (берётся base)
//	addOutbounds   — slice equal (order matters here для простоты) → skip; иначе replace
//	preferredDefault — replace целиком
//	wizard         — replace целиком
//	comment        — equal → skip; пустая в form + non-empty в base → пишем "" явно
//
// Если diff пуст — возвращает nil (no-op Save → не создаём USER patch).
//
// Используется только для referenced entries. Для direct entries diff не нужен
// (body хранится inline, перезаписывается напрямую).
func OutboundFieldDiff(form, base configtypes.Direction) map[string]interface{} {
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

	// SPEC 104. Label и Disabled — обычный field-level diff (пустая строка и
	// false пишутся явно: пользователь мог снять имя, данное шаблоном, или
	// снова включить направление, выключенное пресетом).
	if form.Label != base.Label {
		patch["label"] = form.Label
	}
	if form.Disabled != base.Disabled {
		patch["disabled"] = form.Disabled
	}

	// Auto — replace целиком: поля двойника связаны между собой, и
	// по-полевой diff мог бы записать смену режима без параметров пула.
	// nil в форме при непустой базе — это «пользователь выключил двойник»,
	// и такой diff обязан быть записан явно.
	if !reflect.DeepEqual(form.Auto, base.Auto) {
		if form.Auto != nil {
			patch["auto"] = form.Auto
		} else {
			patch["auto"] = nil
		}
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
//
// Ключи, присутствующие в base но отсутствующие в form, НЕ попадают в patch:
// patch — это набор оверрайдов поверх base, и отсутствие ключа означает
// "оверрайда нет" (значение берётся из base), а не "ключ удалён".
//
// Раньше здесь писался nil как tombstone "ключ удалён", но applyOutboundUpdate
// никогда этот контракт не реализовывал — он копировал nil как живое значение,
// и emitter выдавал "interval":null, на котором sing-box падал с
// `invalid duration ""` (issue #91). Удаление ключа выражается не через diff,
// а через тип outbound'а (см. selector-ветку в edit_dialog.go, которая сама
// не переносит urltest-only ключи).
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
// explicit — правку делает ЖИВОЙ пользователь через форму, а не миграция и
// не синхронизация. Только он позволяет отличить осознанную очистку поля от
// артефакта с тем же содержимым; по самому патчу это неразличимо, поэтому
// признак и приходит от вызывающего, а не угадывается здесь.
//
// Используется Edit dialog'ом после OutboundFieldDiff.
func UpsertUserPatch(updates []configtypes.OutboundUpdate, patch map[string]interface{}, explicit bool) []configtypes.OutboundUpdate {
	// Патч из одних «пустых» значений ({}, [], "") приходит в двух РАЗНЫХ
	// случаях, и различить их по содержимому невозможно — оно одинаковое:
	//
	//  1. артефакт: до SPEC 058 форма отдавала пустыми поля, которых
	//     пользователь не трогал. Такой патч затирал пресетный фильтр
	//     (russian → `!/(🇷🇺)/i` на proxy-out) и молча выпускал российский
	//     трафик через российский узел;
	//  2. осознанная очистка: пользователь стёр поле — например, убрал
	//     умолчание группы. Пустое значение здесь и есть правка.
	//
	// Раньше оба случая отбрасывались, и второй молча терялся: убранное
	// умолчание не сохранялось, форма показывала шаблонное значение снова, а
	// в конфиг уезжала мёртвая ссылка (`default: "proxy-out"` при составе
	// без proxy-out не давал ядру стартовать).
	//
	// Различает не содержимое, а происхождение. Сегодняшняя форма считает
	// diff против базы БЕЗ USER-патча, то есть пустое значение в её выводе
	// означает ровно «пользователь убрал то, что было». Помечаем такой патч
	// Explicit и сохраняем целиком; legacy-патчи флага не имеют и чистятся
	// как прежде — их снимает рубеж загрузки (state.sanitizeOutboundRefs).
	if !explicit {
		patch = dropEmptyOverrides(patch)
	}
	explicit = explicit && hasEmptyOverride(patch)
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
			Ref:      configtypes.RefUser,
			Patch:    patch,
			Explicit: explicit,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// hasEmptyOverride — есть ли в патче хоть одно пустое значение, то есть
// «пользователь стёр поле».
//
// Именно наличие пустого, а не «все пустые»: очистка одного поля вместе с
// правкой другого — тот же осознанный ввод, и терять её нельзя.
func hasEmptyOverride(patch map[string]interface{}) bool {
	for k, v := range patch {
		switch t := v.(type) {
		case nil:
			// null у auto — «двойник выключен», осознанное значение, но не
			// очистка поля: оно и так переживает dropEmptyOverrides.
			if k != "auto" {
				return true
			}
		case string:
			if t == "" {
				return true
			}
		case map[string]interface{}:
			if len(t) == 0 {
				return true
			}
		case []interface{}:
			if len(t) == 0 {
				return true
			}
		case []string:
			if len(t) == 0 {
				return true
			}
		}
	}
	return false
}

// dropEmptyOverrides убирает из патча ключи с пустым значением. Если после
// этого ничего не осталось — возвращает nil (патча нет).
//
// Пустым считаем: nil, пустую строку, пустой map, пустой слайс. Нулевые числа
// и false — НЕ пустые: `disabled:false` и `tolerance:0` — осмысленные
// значения (см. applyOutboundUpdatePatch).
func dropEmptyOverrides(patch map[string]interface{}) map[string]interface{} {
	if len(patch) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(patch))
	for k, v := range patch {
		switch t := v.(type) {
		case nil:
			// Явный null у auto — это «двойник выключен» (осознанно); у
			// остальных ключей null ничего не значит и выбрасывается.
			if k == "auto" {
				out[k] = v
			}
		case string:
			if t != "" {
				out[k] = v
			}
		case map[string]interface{}:
			// Вложенный объект чистится тем же правилом: `auto` несёт
			// pool_tolerance/tolerance пустыми, и они сериализуются в null
			// (omitempty на структуре не работает — Go опускает только
			// нулевые скаляры). Такой null в патче читается как правка,
			// которой пользователь не делал.
			if inner := dropEmptyOverrides(t); len(inner) > 0 {
				out[k] = inner
			}
		case []interface{}:
			if len(t) > 0 {
				out[k] = v
			}
		case []string:
			if len(t) > 0 {
				out[k] = v
			}
		default:
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
