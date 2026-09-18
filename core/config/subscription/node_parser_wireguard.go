package subscription

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/textnorm"
)

// parseWireGuardURI parses wireguard:// URI into ParsedNode with sing-box endpoint in Outbound.
// Format: wireguard://<PRIVATE_KEY>@<SERVER_IP>:<PORT>?publickey=...&address=...&allowedips=...
// Required query: publickey, address, allowedips. Optional: mtu, keepalive, presharedkey, listenport, name, dns.
func parseWireGuardURI(uri string, skipFilters []map[string]string) (*configtypes.ParsedNode, error) {
	debuglog.DebugLog("parseWireGuardURI: start")
	if len(uri) > MaxURILength {
		debuglog.DebugLog("parseWireGuardURI: error URI length exceeded")
		return nil, fmt.Errorf("URI length (%d) exceeds maximum (%d)", len(uri), MaxURILength)
	}
	// Extract fragment from raw URI; url.Parse may not set Fragment for non-standard schemes.
	fragmentFromRaw := ""
	if i := strings.LastIndex(uri, "#"); i >= 0 {
		fragmentFromRaw = strings.TrimSpace(uri[i+1:])
	}
	// A standard base64 private key may contain a raw '/' in the userinfo
	// (`wireguard://AbC/DeF...@host`). url.Parse treats that '/' as the start of
	// the path and drops the userinfo, so the key would be lost. Percent-encode
	// raw '/' in the userinfo before parsing; the PathUnescape below restores it.
	uri = percentEncodeWGUserinfoSlashes(uri)
	parsedURL, err := url.Parse(uri)
	if err != nil {
		debuglog.DebugLog("parseWireGuardURI: error parse URL: %v", err)
		return nil, fmt.Errorf("failed to parse wireguard URI: %w", err)
	}
	if parsedURL.Hostname() == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing hostname")
		return nil, fmt.Errorf("invalid wireguard URI: missing hostname")
	}
	// The key normally lives in the userinfo, but some clients put it in the
	// query instead (`?privatekey=`/`?private_key=`); LxBox accepts both, so a
	// subscription parsed there and here must yield the same node (SPEC 103,
	// D-021). Reject only when neither slot carries a key.
	rawPrivateKey := ""
	if parsedURL.User != nil {
		rawPrivateKey = parsedURL.User.Username()
	}
	if rawPrivateKey == "" {
		qq := parsedURL.Query()
		if rawPrivateKey = queryGetFold(qq, "privatekey"); rawPrivateKey == "" {
			rawPrivateKey = queryGetFold(qq, "private_key")
		}
		// Query() уже превратил `+` base64-ключа в пробел (семантика
		// application/x-www-form-urlencoded) — возвращаем обратно, иначе
		// normalizeWGKey бракует ключ как «not base64» и узел пропадает.
		// Внутри base64 пробел легально не встречается, замена безопасна.
		rawPrivateKey = strings.ReplaceAll(rawPrivateKey, " ", "+")
	}
	if rawPrivateKey == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing private key (userinfo/query)")
		return nil, fmt.Errorf("invalid wireguard URI: missing private key (userinfo or privatekey= query)")
	}
	// Use PathUnescape so + in base64 is preserved (QueryUnescape would turn + into space and break the key)
	privateKey, err := url.PathUnescape(rawPrivateKey)
	if err != nil {
		privateKey = rawPrivateKey
	}
	privateKey = strings.TrimSpace(privateKey)
	if privateKey == "" {
		return nil, fmt.Errorf("invalid wireguard URI: empty private key")
	}
	// Только ПЕРЕВОД НАПИСАНИЯ: ядро декодирует ключи base64.StdEncoding
	// (transport/wireguard), а панели пишут тот же ключ url-safe и без
	// паддинга. ГОДНОСТЬ ключа тут больше не судится — это правило значения, и
	// живёт оно в реестре (wireguard.body.private_key, format base64_32,
	// on_invalid drop_node с кодом wg_key_invalid). Пока проверка стояла
	// здесь, узел пропадал МОЛЧА, и то же самое значение на входе sing-box
	// уезжало в ядро (находка №4 LEGACY_AUDIT, решение владельца 19.09.2026).
	privateKey = normalizeWGKey(privateKey)

	port := 51820
	if p := parsedURL.Port(); p != "" {
		if pi, err := strconv.Atoi(p); err == nil {
			port = pi
		}
	}

	q := parsedURL.Query()
	// Preserve + in base64 (query parser would decode + as space)
	publicKey := queryParamPreservePlus(parsedURL, "publickey")
	if publicKey == "" {
		publicKey = q.Get("publickey")
	}
	addressParam := q.Get("address")
	allowedipsParam := q.Get("allowedips")
	if publicKey == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing publickey")
		return nil, fmt.Errorf("invalid wireguard URI: missing required query parameter publickey")
	}
	if addressParam == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing address")
		return nil, fmt.Errorf("invalid wireguard URI: missing required query parameter address")
	}
	// Перевод написания, без суда о годности (см. private_key выше).
	publicKey = normalizeWGKey(publicKey)

	addressDecoded, _ := url.QueryUnescape(addressParam)
	allowedipsDecoded, _ := url.QueryUnescape(allowedipsParam)
	// sing-box wants CIDRs (netip.Prefix): a bare IP like "172.16.0.2" (common in
	// AmneziaWG/.conf exports) fails to load with `ParsePrefix: no '/'`. Default a
	// bare address to /32 (IPv4) or /128 (IPv6).
	addressList := normalizeWGPrefixes(splitAndTrim(addressDecoded, ","))
	allowedipsList := normalizeWGPrefixes(splitAndTrim(allowedipsDecoded, ","))
	if len(addressList) == 0 {
		return nil, fmt.Errorf("invalid wireguard URI: address empty after parse")
	}

	// MTU только ПЕРЕНОСИТСЯ из ссылки в тело. Ни дефолта, ни потолка здесь
	// больше нет: и то и другое — правила ЗНАЧЕНИЯ, и живут они в реестре
	// (contract/registry/protocols/wireguard.json, body.fields.mtu:
	// default_when даёт AWG-узлу 1280, когда mtu не задан, max_when держит
	// потолок 1280). Пока правило жило здесь, оно не применялось к телу из
	// sing-box-импорта — один и тот же узел ссылкой и объектом получал разный
	// MTU (находка №5 LEGACY_AUDIT, контракт 1.1.5).
	//
	// Plain WireGuard без mtu= поля не получает вовсе: ядро само ставит 1408
	// (transport/wireguard/endpoint.go), и свой 1420 и противоречил бы ядру, и
	// давал узлу другой identity-хеш, чем у LxBox (SPEC 103, D-026 / CANON §2.4).
	mtu := 0
	if m := q.Get("mtu"); m != "" {
		if mi, err := strconv.Atoi(m); err == nil {
			mtu = mi
		}
	}
	listenport := 0
	if lp := q.Get("listenport"); lp != "" {
		if lpi, err := strconv.Atoi(lp); err == nil {
			listenport = lpi
		}
	}
	// Emitted only when the URI carries it: both `name` and `system` are
	// omitempty in the core, and writing defaults changes the node's identity
	// hash relative to LxBox (SPEC 103, D-010 / CANON §2.4).
	name := q.Get("name")
	if decoded, err := url.QueryUnescape(name); err == nil {
		name = decoded
	}

	peer := map[string]interface{}{
		"address":    parsedURL.Hostname(),
		"port":       port,
		"public_key": publicKey,
	}
	// allowedips= нет — ключ в тело не пишем вовсе: «маршрутизировать всё»
	// подставит РЕЕСТР (peers[].allowed_ips.default_when, D-022). Прежде
	// дефолт стоял здесь и второй копией в парсере профиля Amnezia, а на
	// входе sing-box не работал совсем: тело без allowed_ips узел терял
	// вместо того, чтобы дополниться (находка №27 LEGACY_AUDIT).
	//
	// Пустой список писать нельзя: для ядра это «missing allowed ips for peer
	// N» фаталом на весь конфиг, а для default_when — заданное значение,
	// которое он не тронет.
	if len(allowedipsList) > 0 {
		peer["allowed_ips"] = allowedipsList
	}
	// keepalive: число как раньше; AWG3 добавил диапазон "25-35", который ядро
	// перевыбирает при каждом взводе таймера. Мусор пропускается (как раньше).
	if keepalive := strings.TrimSpace(q.Get("keepalive")); keepalive != "" {
		if ki, err := strconv.Atoi(keepalive); err == nil {
			peer["persistent_keepalive_interval"] = ki
		} else if rng, ok := parseAWG3Range(keepalive); ok {
			peer["persistent_keepalive_interval"] = rng
		}
	}
	psk := queryParamPreservePlus(parsedURL, "presharedkey")
	if psk == "" {
		psk = q.Get("presharedkey")
	}
	if psk != "" {
		// Перевод написания. Негодный psk роняет УЗЕЛ, но решает это реестр
		// (peers[].pre_shared_key, on_invalid drop_node, код wg_key_invalid):
		// туннель без ожидаемого сервером PSK — тихо сломанный туннель.
		peer["pre_shared_key"] = normalizeWGKey(psk)
	}
	// reserved (Cloudflare WARP): 3 decimal bytes "b0,b1,b2" derived from the
	// account client_id. sing-box prepends them to every WireGuard packet, which
	// WARP requires to route to the right anycast device. A malformed value is
	// skipped (forward-compat: a WARP node still works without reserved on many
	// paths, matching the mtu/keepalive drop-one-param policy).
	if reserved := parseReservedTriplet(q.Get("reserved")); len(reserved) == 3 {
		peer["reserved"] = reserved
	}

	endpoint := map[string]interface{}{
		"type":        "wireguard",
		"tag":         "", // set below after tag is computed
		"address":     addressList,
		"private_key": privateKey,
		"peers":       []map[string]interface{}{peer},
	}
	if mtu > 0 {
		endpoint["mtu"] = mtu
	}
	if name != "" {
		endpoint["name"] = name
	}
	if listenport != 0 {
		endpoint["listen_port"] = listenport
	}

	// AmneziaWG (SPEC 073): promote obfuscation params from the query into the
	// endpoint root (sing-box-lx with_awg shape). No-op for a plain WG URI.
	awgCodes := applyAWGFields(endpoint, q)
	// AmneziaWG 3.x (SPEC 123): защита заголовка, паддинг содержимого, хвосты
	// и тайминги — там же, на корне endpoint.
	awgCodes = append(awgCodes, applyAWG3Fields(endpoint, parsedURL, q)...)
	// Пересечение magic-заголовков h1..h4 и порог паддинга при заданном
	// header_protection_key тоже роняют узел — но решает это РЕЕСТР, а не
	// парсер: связь body.relations ranges_disjoint (код awg_headers_overlap) и
	// min_when у s1..s4 (код awg3_padding_too_short), негодный ключ защиты
	// заголовка — on_invalid drop_node (код awg3_header_key_invalid).
	//
	// Прежде тут стояли рукописные awgHeaderOverlap и validateAWG3: они
	// роняли узел МОЛЧА (все три кода были объявлены в warnings.json и не
	// ставились НИКОГДА), и на входе sing-box не работали вовсе — то же тело
	// уезжало в ядро и валило весь конфиг. Находка №8 LEGACY_AUDIT, запрос
	// LxBox (4), контракт 1.1.11.

	label := parsedURL.Fragment
	if label == "" && fragmentFromRaw != "" {
		label = fragmentFromRaw
	}
	if label == "" {
		label = name
	}
	if decoded, err := url.QueryUnescape(label); err == nil {
		label = decoded
	}
	label = sanitizeForDisplay(label)
	label = textnorm.NormalizeProxyDisplay(label)
	tag, comment := extractTagAndComment(label)
	if tag == "" {
		tag = generateDefaultTag("wireguard", parsedURL.Hostname(), port)
		comment = tag
	}
	tag = normalizeFlagTag(tag)
	endpoint["tag"] = tag

	node := &configtypes.ParsedNode{
		Scheme:   "wireguard",
		Tag:      tag,
		Server:   parsedURL.Hostname(),
		Port:     port,
		Label:    label,
		Comment:  comment,
		Query:    q,
		Outbound: endpoint,
	}

	if shouldSkipNode(node, skipFilters) {
		return nil, nil
	}
	// Деградации AWG — на узел: конверт узла едет в UI и в LxBox.
	for _, code := range awgCodes {
		node.AddWarning(code)
	}
	debuglog.DebugLog("parseWireGuardURI: success tag=%s", node.Tag)
	return node, nil
}

// percentEncodeWGUserinfoSlashes percent-encodes a raw '/' in the userinfo of a
// wireguard:// / awg:// URI (between "://" and the authority's '@') so url.Parse
// does not mistake the base64 private key's '/' for a path separator and drop the
// key. Already-encoded URIs (no raw '/' in userinfo) are returned unchanged.
func percentEncodeWGUserinfoSlashes(uri string) string {
	const sep = "://"
	si := strings.Index(uri, sep)
	if si < 0 {
		return uri
	}
	start := si + len(sep)
	rest := uri[start:]
	at := strings.IndexByte(rest, '@')
	if at < 0 {
		return uri
	}
	// The '@' must belong to the authority, not the query/fragment.
	if strings.ContainsAny(rest[:at], "?#") {
		return uri
	}
	userinfo := rest[:at]
	if !strings.Contains(userinfo, "/") {
		return uri
	}
	return uri[:start] + strings.ReplaceAll(userinfo, "/", "%2F") + uri[start+at:]
}

// normalizeWGKey переводит НАПИСАНИЕ ключа WireGuard (приватного, публичного,
// PSK) в ту форму base64, которую читает ядро: transport/wireguard декодирует
// исключительно base64.StdEncoding, а панели пишут тот же ключ url-safe и без
// паддинга. Одни и те же 32 байта — четыре написания.
//
// Это ПЕРЕВОД ДИАЛЕКТА и ничего больше (секция mapper реестра,
// wg_key_spelling_to_std_base64). О годности значения функция не судит:
// значение, которое не декодируется или декодируется не в 32 байта, она
// возвращает КАК ЕСТЬ, и судит его реестр — wireguard.body.private_key,
// peers[].public_key, peers[].pre_shared_key (format base64_32, on_invalid
// drop_node, код wg_key_invalid).
//
// Прежде функция возвращала ошибку, а вызывающий ронял узел МОЛЧА. Цена была
// двойная: человек не знал, почему узел исчез, и то же самое значение,
// приехавшее телом sing-box, проверки не проходило вовсе и уезжало в ядро —
// расхождение входов, находка №4 LEGACY_AUDIT (решение владельца 19.09.2026).
func normalizeWGKey(value string) string {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.URLEncoding,
		base64.RawStdEncoding, base64.RawURLEncoding,
	} {
		raw, err := enc.DecodeString(value)
		if err != nil || len(raw) != 32 {
			continue
		}
		return base64.StdEncoding.EncodeToString(raw)
	}
	return value
}

// normalizeWGPrefixes ensures every entry is a CIDR (netip.Prefix): a bare IP
// gets /32 (IPv4) or /128 (IPv6). sing-box rejects an address/allowed_ip without
// a prefix length (`netip.ParsePrefix("172.16.0.2"): no '/'`).
func normalizeWGPrefixes(addrs []string) []string {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if !strings.Contains(a, "/") {
			if strings.Contains(a, ":") {
				a += "/128" // IPv6
			} else {
				a += "/32" // IPv4
			}
		}
		out = append(out, a)
	}
	return out
}

// queryParamPreservePlus returns the first value for key in u.RawQuery, decoded with PathUnescape.
// This preserves '+' in base64 (QueryUnescape decodes '+' as space and would break keys).
func queryParamPreservePlus(u *url.URL, key string) string {
	for _, pair := range strings.Split(u.RawQuery, "&") {
		if i := strings.Index(pair, "="); i >= 0 {
			k := strings.TrimSpace(pair[:i])
			if k != key {
				continue
			}
			val := pair[i+1:]
			if d, err := url.PathUnescape(val); err == nil {
				return d
			}
			return val
		}
	}
	return ""
}

// AmneziaWG (AWG 2.0) field names, promoted to the WireGuard endpoint root in
// the sing-box-lx `with_awg` config shape (SPEC 073). Shared by the URI parser
// (applyAWGFields) and the share-URI encoder (shareuri_wireguard.go).
//   - numeric: jc/jmin/jmax, s1–s4, h1–h4 — uint32 (emitted as JSON number)
//   - string:  i1–i5 — case-sensitive tag strings (<b 0xHEX>, <r N>, <c>, …)
//   - masquerade: ip/id/ib — id/ip/ib sugar (SPEC 009); the core expands them
//     into i1 (and i2 for quic). Mutually exclusive with an explicit i1. Used by
//     the WARP generator (SPEC 084) and AmneziaWG/Amnezia imports.
var (
	awgNumericFields    = []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4"}
	awgStringFields     = []string{"i1", "i2", "i3", "i4", "i5"}
	awgMasqueradeFields = []string{"ip", "id", "ib"}
)

// hasAWGParams reports whether the query carries any AmneziaWG obfuscation field
// (numeric jc/jmin/jmax/s/h, string i1-i5, or the masquerade sugar ip/id/ib).
//
// ПОЛИТИКУ MTU ЭТА ФУНКЦИЯ БОЛЬШЕ НЕ ВЕДЁТ. Потолок и дефолт AWG-узла —
// правила значения реестра (wireguard.body.fields.mtu, max_when/default_when),
// и условие «узел AmneziaWG» там выражено тем же набором полей, но по ТЕЛУ, а
// не по query: так оно работает на всех входах, а не только на ссылке
// (контракт 1.1.5). Здесь предикат остался для решений уровня РАЗБОРА ссылки —
// нужно ли вообще читать AWG-поля из query.
//
// The masquerade sugar counts on its own: a link carrying only ip/id/ib is an
// AWG endpoint too (the core expands the sugar into i1).
// An AWG3 marker (header protection, timings, ranged keepalive — SPEC 123)
// counts too: such a link is an AmneziaWG endpoint even without a single AWG2
// field.
func hasAWGParams(q url.Values) bool {
	for _, list := range [][]string{awgNumericFields, awgStringFields, awgMasqueradeFields} {
		for _, k := range list {
			if strings.TrimSpace(q.Get(k)) != "" {
				return true
			}
		}
	}
	return hasAWG3Params(q)
}

// applyAWGFields extracts AmneziaWG obfuscation params from a wireguard:// (or
// awg://) query and promotes them to the endpoint root. Numeric fields are
// stored as int64 (full uint32 range, safe on 32-bit, marshals as a JSON
// number); h1–h4 may instead carry an AWG 2.0 randomization range "lo-hi",
// stored as a normalized string (SPEC 073.2, core >= lx.6 picks an in-range
// value per handshake); i1–i5 are stored as non-empty strings with their tag
// case preserved. A bad value is skipped with a debug log (forward-compat: one
// broken param must not drop the whole node, matching the mtu/keepalive
// policy). A plain WireGuard URI (no AWG params) leaves endpoint untouched.
// Возвращает коды деградаций (contract/registry/warnings.json): функция
// работает с сырым endpoint и узла не знает, а код обязан оказаться НА УЗЛЕ —
// конверт узла едет в UI и в LxBox, лог видит только читающий его.
func applyAWGFields(endpoint map[string]interface{}, q url.Values) []string {
	var codes []string
	for _, k := range awgNumericFields {
		raw := strings.TrimSpace(q.Get(k))
		if raw == "" {
			continue
		}
		if n, err := strconv.ParseUint(raw, 10, 32); err == nil {
			endpoint[k] = int64(n)
			continue
		}
		// Дальше значение — уже НЕ голое число. Судить его маппер больше не
		// вправе: прежде он ронял h1–h4 с кодом, а jc/jmin/jmax/s1–s4 —
		// МОЛЧА (асимметрия внутри одной функции, находка №9 LEGACY_AUDIT), и
		// на входе sing-box не работал ни тот ни другой случай. Значение
		// переносится КАК ЕСТЬ, поле снимает реестр своим on_invalid с кодом
		// (awg_header_invalid у jc/jmin/jmax/h1–h4/s1–s2, awg3_field_invalid у
		// s3–s4) — решение владельца 19.09.2026: «снятие реестром с
		// сообщением, а не тихое».
		//
		// SPEC 073.2: h1–h4 законно несут диапазон "lo-hi"; его ФОРМУ знает
		// тип реестра awg_range, а порядок границ там же и нормализуется
		// (normalize range_order, тихий своп).
		endpoint[k] = raw
	}
	for _, k := range awgStringFields {
		// q.Get already URL-decodes (incl. '+' → space and %3C → '<'); the tag
		// case must be preserved exactly, so do NOT lower-case.
		v := strings.TrimSpace(q.Get(k))
		if v == "" {
			continue
		}
		endpoint[k] = v
	}
	// Masquerade sugar id/ip/ib (SPEC 009): the core synthesizes i1 from these.
	// Case must be preserved (id is a domain). Do not set when an explicit i1 is
	// already present — the core rejects id/ip/ib alongside a literal i1.
	if _, hasI1 := endpoint["i1"]; !hasI1 {
		for _, k := range awgMasqueradeFields {
			v := strings.TrimSpace(q.Get(k))
			if v == "" {
				continue
			}
			endpoint[k] = v
		}
	}
	return codes
}

// parseReservedTriplet parses a Cloudflare WARP reserved value "b0,b1,b2"
// (three decimal bytes, each 0–255) into a []int for the peer's reserved field.
// Returns nil for empty input or any malformed/out-of-range component, so a bad
// value is skipped rather than dropping the whole node.
func parseReservedTriplet(raw string) []int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) != 3 {
		return nil
	}
	out := make([]int, 0, 3)
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 || n > 255 {
			return nil
		}
		out = append(out, n)
	}
	return out
}

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
