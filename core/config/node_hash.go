package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/debuglog"
)

// SPEC 112 — идентичность узла есть его ТЕГ, уникальный в рамках источника.
//
// Единственный ключ идентичности — сырой провайдерский тег, уникализированный
// внутри источника (первый `X`, следующий `X-2`) и снятый ДО применения наших
// tag_prefix / tag_postfix / tag_mask. Содержимое узла (server, port, ключи,
// SNI, транспорт, mtu) в идентичность НЕ входит.
//
// Почему так (решение 2026-08-26):
//   - Тег — имя, которым провайдер управляет узлом. Смена сервера под тем же
//     именем (ротация IP, обход белых списков) — это ТОТ ЖЕ узел; привязка
//     идентичности к содержимому ломала логику провайдера.
//   - Контент-хеш зависел от эмиттера и от формы хранения: один и тот же узел,
//     записанный как `uri` и как `config_json`, давал разные хеши. Из-за этого
//     detour-ссылка на узел (SPEC 101) молча протухала, и зависимый источник
//     fail-closed выпадал из конфига целиком. Класс багов снимается только
//     сносом контент-хеша.
//
// Смена tag_prefix / tag_mask источника идентичность НЕ меняет — отметки
// выключения переживают правку тегов. Узлы-группы (SchemeGroup) идентичности
// не имеют: цепляться через selector — задача DetourTag (SPEC 077).
//
// Контент-дедуп упразднён вместе с хешем: при дублях работает уникализация
// тегов, полные копии узла больше не схлопываются.

// init подставляет вычисление идентичности в парсер.
//
// Раньше это делал контроллер при старте приложения, и любой путь, который
// разбирает подписку раньше (превью источника в визарде, снимок preview_nodes
// в фетчере), работал без идентичности. init снимает вопрос «успел ли кто-то
// выставить хук» — пакет config всё равно импортируется всеми этими путями.
//
// Цикла импорта нет: config уже импортирует subscription (см. outbound_share.go).
func init() {
	subscription.NodeIdentityFunc = NodeIdentity
	subscription.LegacyNodeIdentityHashFunc = LegacyNodeIdentityHash
}

// NodeIdentity возвращает идентичность узла (SPEC 112) — его тег в рамках
// источника, снятый до применения tag_prefix / tag_mask.
//
// Контракт стабильности:
//   - смена tag_prefix / tag_postfix / tag_mask источника идентичность НЕ меняет;
//   - смена формы хранения узла (uri ↔ config_json) её НЕ меняет;
//   - правка mtu / SNI / ключей / сервера её НЕ меняет;
//   - переименование узла провайдером её МЕНЯЕТ — это и есть смена имени.
//
// Возвращает "" для узлов без снятой идентичности (собраны не парсером
// источника) и для узлов-групп. Вызывающие обязаны трактовать "" как
// «идентичности нет» и пропускать узел, а не сливать все такие узлы в один ключ.
func NodeIdentity(node *ParsedNode) string {
	if node == nil {
		return ""
	}
	if node.Scheme == SchemeGroup {
		return ""
	}
	if id := strings.TrimSpace(node.IdentityTag); id != "" {
		return id
	}
	// Идентичность не снималась — узел пришёл из пути, который тегов ещё не
	// стемпил (ручной config_json без источника, тесты). Тег на этот момент
	// ещё сырой, и он же станет идентичностью после стемпинга.
	return strings.TrimSpace(node.Tag)
}

// nodeHashIgnoredFields — поля, не участвовавшие в legacy-хеше идентичности.
//
// tag    — ремарка провайдера плюс наши tag_prefix/tag_mask/-2 от уникализации;
// detour — тег соседнего узла: контекст, а не сам узел.
//
// Оставлено ради воспроизведения старых ключей при миграции (SPEC 112).
var nodeHashIgnoredFields = map[string]struct{}{
	"tag":    {},
	"detour": {},
}

// LegacyNodeIdentityHash воспроизводит УПРАЗДНЁННУЮ идентичность SPEC 094 D /
// SPEC 101: sha256 от эмитированного outbound-JSON узла без полей tag/detour, с
// рекурсивно отсортированными ключами.
//
// Существует ТОЛЬКО ради миграции (SPEC 112): по нему опознаются ключи
// disabled-отметок и `detour_node_hash` из state.json и бэкапов, записанных до
// перехода на тег. Новый код идентичность так не считает — используйте
// NodeIdentity.
//
// Экспортирована, потому что миграция живёт в пакете subscription (парсер знает
// момент первого разбора), а эмиттер — здесь; прямой вызов оттуда дал бы цикл
// импорта, поэтому функция уезжает туда хуком LegacyNodeIdentityHashFunc.
func LegacyNodeIdentityHash(node *ParsedNode) string {
	if node == nil {
		return ""
	}

	// WireGuard-узлы — endpoint'ы и эмитятся через GenerateEndpointJSON:
	// per-scheme switch outbound'ов ветки wireguard не имеет и обрезал бы их
	// до {tag,type,server,server_port} (SPEC 101).
	var emitted string
	var err error
	if node.Scheme == "wireguard" {
		emitted, err = GenerateEndpointJSONBare(node)
	} else {
		emitted, err = GenerateNodeJSONBare(node)
	}
	if err != nil {
		debuglog.DebugLog("LegacyNodeIdentityHash: cannot emit node %q: %v", node.Tag, err)
		return ""
	}

	// Голый режим эмиттера, а не вырезание обёртки строковым поиском первой
	// `{`: имя узла печаталось комментарием ПЕРЕД объектом, и «SG {премиум} 1»
	// уводил разбор внутрь имени — подписи у таких узлов просто не было
	// (SPEC 113-A, находка аудита C3).
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(emitted), &obj); err != nil {
		debuglog.DebugLog("LegacyNodeIdentityHash: emitted outbound for %q is not decodable JSON: %v", node.Tag, err)
		return ""
	}

	for field := range nodeHashIgnoredFields {
		delete(obj, field)
	}

	canonical, err := marshalCanonicalJSON(obj)
	if err != nil {
		debuglog.DebugLog("LegacyNodeIdentityHash: cannot canonicalize node %q: %v", node.Tag, err)
		return ""
	}

	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// marshalCanonicalJSON serializes v with every object key sorted, recursively.
//
// encoding/json already sorts map keys, but only for map[string]any — it keeps
// slice order (correct: alpn order is meaningful) and it cannot see through
// nested values typed as interface{}. Normalizing first makes the byte output
// independent of the emitter's insertion order.
func marshalCanonicalJSON(v interface{}) ([]byte, error) {
	return json.Marshal(canonicalizeJSONValue(v))
}

func canonicalizeJSONValue(v interface{}) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]interface{}, len(x))
		for _, k := range keys {
			out[k] = canonicalizeJSONValue(x[k])
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, item := range x {
			// Порядок элементов сохраняется: в alpn и allowed_ips он значим.
			out[i] = canonicalizeJSONValue(item)
		}
		return out
	default:
		return v
	}
}
