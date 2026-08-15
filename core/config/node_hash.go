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

// SPEC 094 фаза D — стабильная идентичность узла.
//
// Единственный механизм идентичности: один хеш обслуживает и дедупликацию, и
// привязку пользовательского выбора (выключенные ноды). Отдельного строкового
// ключа `scheme|server|port|credential` намеренно нет — два механизма с разной
// строгостью разошлись бы в поведении, и пользователю пришлось бы помнить,
// какой где действует.
//
// Отличие от LxBox зафиксировано сознательно: там дедуп идёт по ключу без
// транспорта и TLS, и один сервер с двумя SNI схлопывается. Здесь не
// схлопывается — разные SNI это разные способы пройти фильтрацию, и объединение
// потеряло бы рабочий запасной вариант.

// init подставляет вычисление идентичности в парсер.
//
// Раньше это делал контроллер при старте приложения, и любой путь, который
// разбирает подписку раньше (превью источника в визарде, снимок preview_nodes
// в фетчере), работал без дедупа: узлы приезжали с техническими тегами
// провайдера и без узлов-групп. init снимает вопрос «успел ли кто-то выставить
// хук» — пакет config всё равно импортируется всеми этими путями.
//
// Цикла импорта нет: config уже импортирует subscription (см. outbound_share.go).
func init() {
	subscription.NodeIdentityHashFunc = NodeIdentityHash
}

// nodeHashIgnoredFields — поля, не участвующие в идентичности узла.
//
// tag    — ремарка провайдера плюс наши tag_prefix/tag_mask/-2 от уникализации;
//
//	отображение, а не свойство сервера.
//
// detour — тег соседнего узла: контекст, а не сам узел. Иначе смена джампа
//
//	отвязала бы пользовательский выбор от ноды.
//
// Всё прочее в хеш входит, включая server_name (SNI), transport.headers.Host,
// utls.fingerprint и reality.*.
var nodeHashIgnoredFields = map[string]struct{}{
	"tag":    {},
	"detour": {},
}

// NodeIdentityHash returns a stable identity for a parsed node: sha256 over its
// emitted outbound JSON with tag/detour removed and every object key sorted.
//
// Stability contract:
//   - a provider renaming the node does NOT change the hash;
//   - the source's tag_prefix / tag_mask do NOT change it;
//   - changing port, uuid, password, SNI, transport or reality DOES.
//
// The hash is computed from the decoded map, never from the emitted string:
// GenerateNodeJSON assembles JSON by hand and its field order is not a contract,
// so a string hash would drift on the first edit to the emitter.
//
// Returns "" when the node cannot be emitted; callers must treat that as "no
// identity" and skip dedup rather than collapsing unrelated nodes onto "".
func NodeIdentityHash(node *ParsedNode) string {
	if node == nil {
		return ""
	}

	// WireGuard nodes are endpoints and emit through GenerateEndpointJSON; the
	// per-scheme outbound switch has no wireguard branch and would truncate them
	// to {tag,type,server,server_port}, collapsing every WG node on one
	// server:port into a single identity (SPEC 101). Note: this changed existing
	// WG hashes once — pre-101 disabled-node marks on WG nodes detach (they were
	// unsound anyway: one mark covered all WG nodes of the server).
	var emitted string
	var err error
	if node.Scheme == "wireguard" {
		emitted, err = GenerateEndpointJSON(node)
	} else {
		emitted, err = GenerateNodeJSON(node)
	}
	if err != nil {
		debuglog.DebugLog("NodeIdentityHash: cannot emit node %q: %v", node.Tag, err)
		return ""
	}

	obj, ok := decodeEmittedOutbound(emitted)
	if !ok {
		debuglog.DebugLog("NodeIdentityHash: emitted outbound for %q is not decodable JSON", node.Tag)
		return ""
	}

	for field := range nodeHashIgnoredFields {
		delete(obj, field)
	}

	canonical, err := marshalCanonicalJSON(obj)
	if err != nil {
		debuglog.DebugLog("NodeIdentityHash: cannot canonicalize node %q: %v", node.Tag, err)
		return ""
	}

	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// decodeEmittedOutbound strips the generator's wrapping and decodes the object.
//
// GenerateNodeJSON returns a fragment shaped like "\t// <label>\n\t{...},":
// a leading line comment (the node label, which is display text and must not
// reach the hash) and a trailing comma for list assembly.
func decodeEmittedOutbound(emitted string) (map[string]interface{}, bool) {
	start := strings.Index(emitted, "{")
	if start < 0 {
		return nil, false
	}
	body := strings.TrimSpace(emitted[start:])
	body = strings.TrimSuffix(body, ",")

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		return nil, false
	}
	return obj, true
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
