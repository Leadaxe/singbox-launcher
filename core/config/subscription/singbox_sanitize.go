package subscription

import (
	"strings"

	"singbox-launcher/internal/debuglog"
)

// SPEC 094 A2 / Р2 — санитайзы над ГОТОВОЙ sing-box outbound map.
//
// Импорт sing-box JSON не разбирает outbound на поля и не собирает обратно:
// входная map уже имеет тот же формат, который эмитит лаунчер. Вместо
// реэмиссии она прогоняется через те же проверки, что и URI-путь, — иначе
// один битый узел из чужого конфига валит весь config.json (ядро отвергает
// конфиг целиком на unknown field / невалидном значении).
//
// Каждая функция правит map на месте и возвращает признак «что-то изменено»
// только там, где вызывающему это важно для лога.

// singboxServiceTypes — служебные типы, которые не являются узлами.
// Собственный набор, намеренно не переиспользующий Xray-список
// (у Xray это freedom/blackhole/loopback, у sing-box — direct/block/dns).
var singboxServiceTypes = map[string]struct{}{
	"direct": {}, "block": {}, "dns": {},
}

// singboxGroupTypes — типы outbound-групп.
var singboxGroupTypes = map[string]struct{}{
	"selector": {}, "urltest": {},
}

// IsSingboxServiceType сообщает, является ли тип служебным (не узел).
func IsSingboxServiceType(t string) bool {
	_, ok := singboxServiceTypes[strings.ToLower(strings.TrimSpace(t))]
	return ok
}

// IsSingboxGroupType сообщает, является ли тип группой (selector/urltest).
func IsSingboxGroupType(t string) bool {
	_, ok := singboxGroupTypes[strings.ToLower(strings.TrimSpace(t))]
	return ok
}

// СНЯТО (SPEC 131, контракт 1.1.4): частный набор quicOutboundTypes и срез
// utls/reality по нему. Правило переехало в реестр — tls.json
// body.fields.utls/reality, `forbidden_for` с четырьмя QUIC-схемами и
// `forbidden_codes` → tls_not_applicable_quic. Исполняет его санитайзер
// конвейера (core/config/nodeflow), одинаково для ссылки, JSON-тела и
// Xray-объекта, и — главное — С КОДОМ: здесь срез был молчаливым (только
// debuglog), и пользователь не узнавал, что отпечаток из подписки не сработал.

// SanitizeSingboxOutboundMap приводит импортированный outbound к форме,
// которую ядро гарантированно принимает. Правит ob на месте.
//
// tag используется только в логах.
//
// Возвращает коды применённых деградаций (contract/registry/warnings.json).
// Санитайзер работает с сырой map и узла не знает, а код обязан оказаться
// НА УЗЛЕ: лог читает только тот, кто в него смотрит, а конверт узла едет
// в UI и в LxBox — обе стороны должны сообщать об одной деградации
// одинаково. Вызывающий вешает возвращённое через node.AddWarning.
func SanitizeSingboxOutboundMap(ob map[string]interface{}, tag string) []string {
	if ob == nil {
		return nil
	}
	obType := strings.ToLower(strings.TrimSpace(mapString(ob, "type")))

	sanitizeSingboxMasqueLegacy(ob, obType, tag)
	sanitizeSingboxHysteriaObfs(ob, obType, tag)
	return nil
}

// sanitizeSingboxMasqueLegacy СТРИПАЕТ у masque-outbound ключи чужого
// диалекта (плоские `network`/`sni`/`skip_cert_verify`) — без переноса
// значений. Legacy-приём снесён контрактом 0.8.0 (D-078): значения больше
// не читаются; узел живёт на канонических `vhttp` + вложенном `tls`, а при
// их отсутствии — на дефолтах.
//
// Удалять ключи всё равно обязательно, просто убрать функцию нельзя:
// плоский `sni` рядом с `tls.server_name` — два источника имени, и при
// расхождении ядро падает fail-fast'ом; протащенный legacy-ключ ронял бы
// конфиг целиком вместо тихой деградации одного узла.
func sanitizeSingboxMasqueLegacy(ob map[string]interface{}, obType, tag string) {
	if obType != "masque" {
		return
	}
	stripped := false
	for _, k := range []string{"network", "sni", "skip_cert_verify"} {
		if _, has := ob[k]; has {
			delete(ob, k)
			stripped = true
		}
	}
	if stripped {
		debuglog.DebugLog("Parser: singbox import %q: masque legacy flat keys stripped (0.8.0, values ignored)", tag)
	}
}

// СНЯТО (SPEC 142 A2): sanitizeSingboxTLS. tls не объект или пустой объект
// судит реестр — у секции tls `type: object`, и санитайзер конвейера снимает
// негодную форму с кодом type_invalid (пустой объект — молча) одинаково на
// всех входах, а не только на импорте sing-box.

// Дефолт полосы Hysteria v1 здесь БОЛЬШЕ НЕ ПОДСТАВЛЯЕТСЯ: его подставляет
// реестр (default_when у hysteria.body.up_mbps / down_mbps, SPEC 131 W2d) —
// одинаково для ссылки, JSON-тела и Xray-объекта. Прежде та же константа 100
// лежала здесь, в URI-парсере и в Xray-конвертере тремя копиями, и узел
// получал её не на всех дорогах.

// sanitizeSingboxHysteriaObfs приводит obfs узла Hysteria v1 к форме ядра.
//
// У v1 obfs — ПЛОСКАЯ строка-секрет (option/hysteria.go), а у hysteria2 —
// объект {type,password}. Провайдеры, конвертирующие конфиги автоматически,
// иногда кладут в v1 объект от v2: ядро отвергает такой outbound на разборе
// («json: cannot unmarshal object into ... string») и это роняет ВЕСЬ конфиг,
// а не одну ноду. Достаём секрет, если он там есть, иначе снимаем блок.
func sanitizeSingboxHysteriaObfs(ob map[string]interface{}, obType, tag string) {
	if obType != "hysteria" {
		return
	}
	obfsRaw, ok := ob["obfs"]
	if !ok {
		return
	}
	switch v := obfsRaw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			delete(ob, "obfs")
		}
	case map[string]interface{}:
		password := strings.TrimSpace(mapString(v, "password"))
		if password == "" {
			debuglog.WarnLog("Parser: singbox import %q: hysteria obfs object without password — dropping obfs", tag)
			delete(ob, "obfs")
			return
		}
		debuglog.WarnLog("Parser: singbox import %q: hysteria obfs given as object — using its password as the obfs string", tag)
		ob["obfs"] = password
	default:
		debuglog.WarnLog("Parser: singbox import %q: hysteria obfs of unexpected shape — dropping obfs", tag)
		delete(ob, "obfs")
	}
}

// СНЯТО (SPEC 131, аудит остатков): sanitizeSingboxHysteria2Obfs.
//
// Все четыре его решения выражены в реестре и исполняются санитайзером:
// obfs не объект и тип вне набора — hysteria2.json body.obfs.type (enum
// salamander|gecko, on_invalid drop + obfs_unknown); пустой пароль —
// body.obfs.password (required + code obfs_password_missing), а
// nodeflow.objectField снимает необязательный объект целиком, когда в нём
// не собралось required-поле. Здесь то же самое делалось МОЛЧА (WarnLog),
// то есть код съедался: два объявленных кода не доезжали до узла, и
// пользователь не узнавал, что обфускация из подписки не сработала.

// mapString возвращает строковое поле map или "".
func mapString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}
