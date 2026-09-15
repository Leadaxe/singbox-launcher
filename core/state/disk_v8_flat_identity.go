// File disk_v8_flat_identity.go — терпимое чтение v8-файлов, записанных
// dev-сборкой волны 1 SPEC 127 с плоской идентичностью подписки.
//
// Волна 1 (bfd5fe15) писала v8 с четырьмя плоскими ключами подписки:
// user_agent, send_hwid, hwid, hash_device_model. В объект identity их свела
// волна 2 (768ef591), и перенос живёт в миграции v7→v8
// (migrateV8SourceIdentity). Файл, который сборка волны 1 УЖЕ перевела в v8,
// миграцию больше не проходит, а тип Source плоских ключей не знает: UA
// подписки молча пропадал на первом же Save. У владельца так терялся
// Happ/3.3.6, под которым провайдер отдаёт другой набор узлов.
package state

import (
	"bytes"
	"encoding/json"

	"singbox-launcher/internal/debuglog"
)

// flatIdentityKeys — плоские ключи подписки, которые волна 2 свела в identity.
// Тот же список, что у migrateV8SourceIdentity.
var flatIdentityKeys = []string{"user_agent", "send_hwid", "hwid", "hash_device_model"}

// liftFlatSubscriptionIdentity переносит плоские ключи подписки в identity.
//
// Правило на КАЖДЫЙ ключ отдельно: плоское значение встаёт в identity, только
// если там этой настройки нет (у строки — пусто, у флага — не задано). Если
// identity её уже несёт, плоский ключ отбрасывается: identity писала сборка
// новее. Пустая строка и null значат «как в системе» — переносить нечего, как
// и в миграции. Ключ чужого типа отбрасывается.
//
// Только у подписки: у папки и узлов этих ключей не бывало и в волне 1
// (normalizeSourceShape их стирал).
//
// Файл на загрузке не перезаписывается: перенос идемпотентен, и identity
// уедет на диск с первым Save.
func liftFlatSubscriptionIdentity(data []byte, sources []Source) {
	if !containsAnyFlatIdentityKey(data) {
		return
	}
	var probe struct {
		Sources []map[string]json.RawMessage `json:"sources"`
	}
	if err := json.Unmarshal(data, &probe); err != nil || len(probe.Sources) != len(sources) {
		return
	}
	for i := range sources {
		src := &sources[i]
		if src.Kind != SourceKindSubscription {
			continue
		}
		for _, key := range flatIdentityKeys {
			raw, ok := probe.Sources[i][key]
			if !ok {
				continue
			}
			if liftFlatIdentityKey(src, key, raw) {
				debuglog.DebugLog("state v8: subscription %s: flat %s moved into identity", src.ID, key)
			}
		}
	}
}

// liftFlatIdentityKey — один плоский ключ; true, если значение перенесено.
func liftFlatIdentityKey(src *Source, key string, raw json.RawMessage) bool {
	switch key {
	case "user_agent", "hwid":
		var v string
		if json.Unmarshal(raw, &v) != nil || v == "" {
			return false
		}
		if key == "user_agent" {
			if src.IdentityUserAgent() != "" {
				return false
			}
			src.SetIdentityUserAgent(v)
			return true
		}
		if src.IdentityHWID() != "" {
			return false
		}
		src.SetIdentityHWID(v)
		return true
	case "send_hwid", "hash_device_model":
		var v *bool
		if json.Unmarshal(raw, &v) != nil || v == nil {
			return false
		}
		if key == "send_hwid" {
			if src.IdentitySendHWID() != nil {
				return false
			}
			src.SetIdentitySendHWID(v)
			return true
		}
		if src.IdentityHashDeviceModel() != nil {
			return false
		}
		src.SetIdentityHashDeviceModel(v)
		return true
	}
	return false
}

// containsAnyFlatIdentityKey — дешёвый отсев: без этих имён в файле второй
// разбор не нужен. Имя может стоять и внутри identity — тогда разбор
// просто ничего не найдёт.
func containsAnyFlatIdentityKey(data []byte) bool {
	for _, key := range flatIdentityKeys {
		if bytes.Contains(data, []byte(`"`+key+`"`)) {
			return true
		}
	}
	return false
}
