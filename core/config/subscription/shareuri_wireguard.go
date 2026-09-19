package subscription

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// --- WireGuard (sing-box endpoints[]) ---

// AmneziaWG (AWG 2.0) field names, promoted to the WireGuard endpoint root in
// the sing-box-lx `with_awg` config shape (SPEC 073):
//   - numeric: jc/jmin/jmax, s1–s4, h1–h4 — uint32 (emitted as JSON number)
//   - string:  i1–i5 — case-sensitive tag strings (<b 0xHEX>, <r N>, <c>, …)
//   - masquerade: ip/id/ib — id/ip/ib sugar (SPEC 009); the core expands them
//     into i1 (and i2 for quic). Mutually exclusive with an explicit i1.
//
// Таблицы живут ЗДЕСЬ, рядом с эмиттером, потому что читающая сторона их
// больше не использует: разбор ссылки ведёт секция реестра, где те же имена
// перечислены записями (SPEC 133). Остались два потребителя — этот эмиттер и
// конвертер .conf → канонический URI (node_parser_amnezia.go), то есть обе
// стороны ЗАПИСИ ссылки.
var (
	awgNumericFields    = []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4"}
	awgStringFields     = []string{"i1", "i2", "i3", "i4", "i5"}
	awgMasqueradeFields = []string{"ip", "id", "ib"}
)

// splitAndTrim splits a string by separator, trims whitespace from each part,
// and returns only non-empty parts.
func splitAndTrim(s string, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ShareURIFromWireGuardEndpoint builds wireguard:// from one sing-box endpoint object in config.json `endpoints[]`
// (same shape as produced by the registry mapper / GenerateEndpointJSON). Only **single-peer** endpoints are supported:
// subscription-style URIs have one remote server; multiple peers return ErrShareURINotSupported.
func ShareURIFromWireGuardEndpoint(ep map[string]interface{}) (string, error) {
	if ep == nil {
		return "", fmt.Errorf("%w: nil endpoint", ErrShareURINotSupported)
	}
	if strings.ToLower(strings.TrimSpace(mapGetString(ep, "type"))) != "wireguard" {
		return "", fmt.Errorf("%w: endpoint type is not wireguard", ErrShareURINotSupported)
	}
	priv := mapGetString(ep, "private_key")
	if priv == "" {
		return "", fmt.Errorf("%w: wireguard missing private_key", ErrShareURINotSupported)
	}
	peers, err := wireGuardPeerMaps(ep)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrShareURINotSupported, err)
	}
	if len(peers) > 1 {
		return "", fmt.Errorf("%w: wireguard with multiple peers cannot be encoded as one subscription URI", ErrShareURINotSupported)
	}
	peer := peers[0]
	server := mapGetString(peer, "address")
	port := mapGetInt(peer, "port")
	if server == "" {
		return "", fmt.Errorf("%w: wireguard peer missing address", ErrShareURINotSupported)
	}
	if port <= 0 {
		port = 51820
	}
	pub := mapGetString(peer, "public_key")
	if pub == "" {
		return "", fmt.Errorf("%w: wireguard peer missing public_key", ErrShareURINotSupported)
	}
	allowed := stringSliceFromWireGuardField(peer["allowed_ips"])
	if len(allowed) == 0 {
		return "", fmt.Errorf("%w: wireguard peer missing allowed_ips", ErrShareURINotSupported)
	}
	addrList := stringSliceFromWireGuardField(ep["address"])
	if len(addrList) == 0 {
		return "", fmt.Errorf("%w: wireguard missing address", ErrShareURINotSupported)
	}
	q := url.Values{}
	q.Set("publickey", pub)
	q.Set("address", strings.Join(addrList, ","))
	q.Set("allowedips", strings.Join(allowed, ","))
	// MTU эмитится, если он есть в узле. Раньше значение 1420 умалчивалось
	// как «дефолтное», но D-026 отдал ДЕФОЛТ ядру: если mtu лежит в узле,
	// его туда положил пользователь или подписка, и ссылка обязана его
	// донести — иначе round-trip молча роняет настройку.
	if mtu := mapGetInt(ep, "mtu"); mtu > 0 {
		q.Set("mtu", strconv.Itoa(mtu))
	}
	// keepalive: mapGetInt отдавал 0 на AWG3-диапазоне "25-35" и round-trip
	// молча терял настройку — awgNumericString умеет обе формы.
	if ka, ok := awgNumericString(peer["persistent_keepalive_interval"]); ok && ka != "" && ka != "0" {
		q.Set("keepalive", ka)
	}
	if psk := mapGetString(peer, "pre_shared_key"); psk != "" {
		q.Set("presharedkey", psk)
	}
	// reserved (Cloudflare WARP): re-emit "b0,b1,b2" so endpoint->URI->endpoint round-trips.
	if res := intSliceFromWireGuardField(peer["reserved"]); len(res) == 3 {
		q.Set("reserved", fmt.Sprintf("%d,%d,%d", res[0], res[1], res[2]))
	}
	if lp := mapGetInt(ep, "listen_port"); lp > 0 {
		q.Set("listenport", strconv.Itoa(lp))
	}
	if name := mapGetString(ep, "name"); name != "" && name != "singbox-wg0" {
		q.Set("name", name)
	}
	if dnsStr := wireGuardDNSToQuery(ep["dns"]); dnsStr != "" {
		q.Set("dns", dnsStr)
	}
	// AmneziaWG (SPEC 073): re-emit obfuscation params so endpoint→URI→endpoint
	// round-trips losslessly. Numeric fields are emitted by PRESENCE (so an
	// explicit jc=0 / junk-off survives); i1–i5 only when non-empty. url.Values
	// escapes the tag chars (<, >, space); the parser decodes them back.
	for _, k := range awgNumericFields {
		if raw, ok := ep[k]; ok {
			if s, ok2 := awgNumericString(raw); ok2 {
				q.Set(k, s)
			}
		}
	}
	for _, k := range awgStringFields {
		if s := mapGetString(ep, k); s != "" {
			q.Set(k, s)
		}
	}
	// Masquerade sugar id/ip/ib (SPEC 009): re-emit so endpoint→URI→endpoint
	// round-trips for WARP/AmneziaWG obfuscated nodes.
	for _, k := range awgMasqueradeFields {
		if s := mapGetString(ep, k); s != "" {
			q.Set(k, s)
		}
	}
	// AmneziaWG 3.x (SPEC 123): ключ защиты заголовка, тайминги и булевы —
	// эмиттер и парсер ходят парой, иначе endpoint→URI→endpoint теряет весь
	// AWG3-набор и узел «настроен», но не соединяется.
	if s := mapGetString(ep, awg3HeaderKeyField.JSON); s != "" {
		q.Set(awg3HeaderKeyField.Param, s)
	}
	for _, f := range awg3RangeFields {
		if raw, ok := ep[f.JSON]; ok {
			if s, ok2 := awgNumericString(raw); ok2 && s != "" {
				q.Set(f.Param, s)
			}
		}
	}
	for _, f := range awg3BoolFields {
		// false/отсутствие ключа не эмитим: разбор такой формы даёт то же
		// самое, а лишний параметр менял бы идентичность узла.
		if v, _ := ep[f.JSON].(bool); v {
			q.Set(f.Param, "on")
		}
	}
	u := &url.URL{
		Scheme: "wireguard",
		// Ключ кладётся СЫРЫМ: url.User сам percent-энкодит то, что в
		// userinfo литералом стоять не может, при сериализации — `/`
		// внутри std-base64 уезжает как `%2F`.
		//
		// Прежде здесь стоял ещё и PathEscape, и кодирование шло ДВАЖДЫ:
		// ссылка получала `%252F`, то есть литеральные символы «%», «2»,
		// «F» вместо слэша. Чужой клиент читал такой ключ как мусор, а наш
		// собственный обратный разбор спасала лишь симметричная ошибка —
		// старый путь распаковывал userinfo вторым PathUnescape поверх
		// того, что уже сделала платформа. Разбор через движок делает ровно
		// один проход (как и всякий обычный клиент), и двойное
		// кодирование стало видно кругом parse(emit(node)).
		User:     url.User(priv),
		Host:     net.JoinHostPort(server, strconv.Itoa(port)),
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(ep),
	}
	return u.String(), nil
}

func wireGuardPeerMaps(ep map[string]interface{}) ([]map[string]interface{}, error) {
	v, ok := ep["peers"]
	if !ok {
		return nil, fmt.Errorf("missing peers")
	}
	if typed, ok := v.([]map[string]interface{}); ok {
		if len(typed) == 0 {
			return nil, fmt.Errorf("peers must be a non-empty array")
		}
		return typed, nil
	}
	arr, ok := v.([]interface{})
	if !ok || len(arr) == 0 {
		return nil, fmt.Errorf("peers must be a non-empty array")
	}
	out := make([]map[string]interface{}, 0, len(arr))
	for _, e := range arr {
		m, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid peer objects")
	}
	return out, nil
}

func stringSliceFromWireGuardField(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case string:
		x = strings.TrimSpace(x)
		if x == "" {
			return nil
		}
		return []string{x}
	case []string:
		return x
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, e := range x {
			s := wireGuardJSONElemToString(e)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// intSliceFromWireGuardField coerces a WireGuard peer numeric slice (e.g.
// reserved) to []int. Handles []int and []interface{} carrying int / int64 /
// float64 (JSON-decoded numbers). Non-numeric or nil input yields nil.
func intSliceFromWireGuardField(v interface{}) []int {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []int:
		return x
	case []interface{}:
		out := make([]int, 0, len(x))
		for _, e := range x {
			switch n := e.(type) {
			case int:
				out = append(out, n)
			case int64:
				out = append(out, int(n))
			case float64:
				out = append(out, int(n))
			default:
				return nil
			}
		}
		return out
	default:
		return nil
	}
}

func wireGuardJSONElemToString(e interface{}) string {
	if e == nil {
		return ""
	}
	switch x := e.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case json.Number:
		s := strings.TrimSpace(x.String())
		return s
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

// awgNumericString formats an AmneziaWG numeric endpoint field for a share-URI
// query. It accepts every shape the value can take depending on provenance:
// freshly parsed (int64), JSON-decoded from state (float64 / json.Number), or
// hand-built (int / uint32). Returns ok=false for nil / non-numeric / empty so
// the caller skips the param. Avoids mapGetInt — that returns `int` (would
// overflow large h-values on 32-bit) and doesn't handle uint32.
func awgNumericString(v interface{}) (string, bool) {
	switch t := v.(type) {
	case int64:
		return strconv.FormatInt(t, 10), true
	case int:
		return strconv.Itoa(t), true
	case uint32:
		return strconv.FormatUint(uint64(t), 10), true
	case uint64:
		return strconv.FormatUint(t, 10), true
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10), true
		}
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case json.Number:
		s := strings.TrimSpace(t.String())
		return s, s != ""
	case string:
		s := strings.TrimSpace(t)
		return s, s != ""
	default:
		return "", false
	}
}

func wireGuardDNSToQuery(v interface{}) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case []string:
		return strings.Join(x, ",")
	case []interface{}:
		return strings.Join(stringSliceFromWireGuardField(x), ",")
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}
