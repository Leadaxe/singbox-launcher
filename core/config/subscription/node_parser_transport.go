package subscription

import (
	"net/url"
	"strconv"
	"strings"
)

// queryGetFold returns the first value for a query key, matching case-insensitively.
// Subscriptions use allowinsecure=0, AllowInsecure=1, etc.
func queryGetFold(q url.Values, name string) string {
	for k, vs := range q {
		if strings.EqualFold(k, name) && len(vs) > 0 {
			return vs[0]
		}
	}
	return ""
}

// singboxUTLSFingerprints are the names sing-box accepts in tls.utls.fingerprint.
// Mirrors uTLSClientHelloID in sing-box common/tls/utls_client.go — anything else
// aborts config load with "unknown uTLS fingerprint".
var singboxUTLSFingerprints = map[string]struct{}{
	"chrome": {}, "firefox": {}, "edge": {}, "safari": {},
	"360": {}, "qq": {}, "ios": {}, "android": {},
	"random": {}, "randomized": {},
	// Chrome ClientHello variants sing-box maps onto HelloChrome_Auto.
	"chrome_psk": {}, "chrome_psk_shuffle": {}, "chrome_padding_psk_shuffle": {},
	"chrome_pq": {}, "chrome_pq_psk": {},
}

// utlsAliasPrefixes maps uTLS library ClientHelloID names (HelloChrome_120,
// HelloFirefox_Auto, …) onto the browser family sing-box understands. Some lists
// export the Go identifier verbatim instead of the sing-box name.
var utlsAliasPrefixes = []struct {
	prefix string
	name   string
}{
	{"hellochrome", "chrome"},
	{"hellofirefox", "firefox"},
	{"helloedge", "edge"},
	{"hellosafari", "safari"},
	{"helloios", "ios"},
	{"helloandroid", "android"},
	{"hello360", "360"},
	{"helloqq", "qq"},
	{"hellorandomized", "randomized"},
	{"hellorandom", "random"},
}

// NormalizeUTLSFingerprint maps subscription variants to sing-box utls names (lowercase).
// sing-box rejects values like "QQ"; the canonical name is "qq".
//
// Values outside the sing-box allowlist are dropped (""), not passed through: a single
// node carrying e.g. fp=HelloChrome_120 made sing-box abort the whole config with
// "initialize outbound[N]: unknown uTLS fingerprint" so the VPN never started. Callers
// treat "" as "no utls block", degrading that node instead of poisoning config.json.
func NormalizeUTLSFingerprint(fp string) string {
	canon, _ := normalizeUTLSFingerprintEx(fp)
	return canon
}

// normalizeUTLSFingerprintEx additionally reports whether a non-empty value was
// junk. Junk must not silently become something else: the fingerprint is client
// -side camouflage the server never checks, so the node itself is almost
// certainly fine — it gets `chrome` plus a warning rather than a dropped utls
// block or a random per-start identity (SPEC 103, D-029).
func normalizeUTLSFingerprintEx(fp string) (canon string, junk bool) {
	fp = strings.TrimSpace(strings.ToLower(fp))
	if fp == "" {
		return "", false
	}
	if _, ok := singboxUTLSFingerprints[fp]; ok {
		return fp, false
	}
	// uTLS Go identifiers: HelloChrome_120, hellofirefox_auto, HelloChrome-106, …
	bare := strings.NewReplacer("_", "", "-", "", " ", "").Replace(fp)
	for _, alias := range utlsAliasPrefixes {
		if strings.HasPrefix(bare, alias.prefix) {
			return alias.name, false
		}
	}
	return "", true
}

// utlsJunkFallback — канонический отпечаток, которым реестр заменяет мусор
// (`tls.utls.fingerprint.on_invalid: coerce chrome`). Здесь он нужен только
// сборке конфига (EnforceRealityFingerprint) — парсер значений не решает.
const utlsJunkFallback = "chrome"

// wsEarlyDataHeaderName is the header sing-box must use to stay compatible with
// Xray-core WebSocket 0-RTT. Xray sends early data in this header; with an empty
// early_data_header_name sing-box would instead append it to the path (the V2Ray
// convention), which an Xray server does not understand. See the sing-box
// v2ray-transport docs and issue #96.
const wsEarlyDataHeaderName = "Sec-WebSocket-Protocol"

// splitWSEarlyData separates Xray's `?ed=N` tail from a WebSocket path. Xray
// encodes WebSocket Early Data into the path (`/api/v2/channel?ed=2560`) instead
// of a dedicated field, but sing-box treats the whole string as a literal path
// and the server answers 404 (issue #96). We return the clean path plus the
// max_early_data value (0 = not set / not a number).
//
// The `?`-tail is always stripped from the path — any query tail breaks the WS
// route match — but a malformed `ed` (missing, non-numeric) only zeroes the
// early-data value rather than dropping the node, matching how we degrade other
// broken share-URI fields instead of poisoning the config.
func splitWSEarlyData(path string) (string, int) {
	path = strings.TrimSpace(path)
	i := strings.IndexByte(path, '?')
	if i < 0 {
		return path, 0
	}
	tail := path[i+1:]
	clean := path[:i]
	q, err := url.ParseQuery(tail)
	if err != nil {
		return clean, 0
	}
	// Xray writes lowercase `ed`, but read it case-insensitively to match how the
	// rest of the parser folds query keys (queryGetFold).
	ed := strings.TrimSpace(queryGetFold(q, "ed"))
	if ed == "" {
		return clean, 0
	}
	n, err := strconv.Atoi(ed)
	if err != nil || n <= 0 {
		return clean, 0
	}
	return clean, n
}

// applyWSEarlyData sets the WebSocket transport path and, when a positive
// max_early_data was parsed from the Xray `?ed=N` tail, the two sing-box early
// data fields. Shared by every WS transport builder (URI, Xray JSON, VMess) so
// the `?ed=` conversion stays consistent. An empty path leaves the key unset.
// Возвращает true, когда хвост `?ed=N` был реально разложен: путь из ссылки
// в конфиг попадает не буквально, и узел вправе сообщить об этом кодом
// ws_early_data_converted (info — узел работает, но описан иначе, чем в URI).
func applyWSEarlyData(tr map[string]interface{}, rawPath string) bool {
	clean, maxED := splitWSEarlyData(decodeResidualPercent(rawPath))
	if clean != "" {
		tr["path"] = clean
	}
	if maxED > 0 {
		tr["max_early_data"] = maxED
		tr["early_data_header_name"] = wsEarlyDataHeaderName
		return true
	}
	return false
}

// decodeResidualPercent strips leftover percent-encoding from a path that was
// encoded twice by the provider's panel (`path=%2F%252Fassignment`). The core
// hands the path to the server verbatim (transport/v2raywebsocket/client.go →
// net/url.setPath), so a leftover `%2F` travels as `%252F` and the server 404s
// on a path it never published. Bounded to two passes, matching LxBox
// (transport.dart decodeResidualPercent) — SPEC 103, D-028.
func decodeResidualPercent(raw string) string {
	v := raw
	for i := 0; i < 2; i++ {
		if !strings.Contains(v, "%") {
			break
		}
		// PathUnescape, а не QueryUnescape: значение — ПУТЬ, и литеральный
		// `+` в нём легален. QueryUnescape превращал `/ws+v2%2Fdata` в
		// `/ws v2/data` — сервер отвечает 404, узел «жив» и молча не
		// работает. `%2F` и прочие проценты обе функции декодируют одинаково.
		dec, err := url.PathUnescape(v)
		if err != nil || dec == v {
			break
		}
		v = dec
	}
	return v
}

// EnforceRealityFingerprint дописывает uTLS-блок у tls, где РЕАЛЬНО эмитится
// reality, и ставит отпечаток там, где его не выбирал никто (D-119, заменяет
// D-104).
//
// Явный отпечаток узла уходит в конфиг как есть: отпечаток — выбор подписки,
// и лаунчер делает так, как она велит (решение владельца). D-104 подменял всё
// вне chrome-семейства на chrome, исходя из того, что КАЖДЫЙ REALITY-сервер —
// Xray ≥ v26.9.8, которому нужен key_share X25519MLKEM768; это не так, и
// подмена чинила одни узлы ценой чужого выбора. Требование новых серверов
// пользователь видит подсказкой на узле (reality_fp_not_chrome).
//
// Что правится:
//   - нет uTLS-блока или он выключен — блок включается: без него ядро падает
//     «uTLS is required by reality client»;
//   - пустой отпечаток — пишется chrome явно (ядро трактует пустой как chrome,
//     но конфиг читают и другие инструменты);
//   - `random` — наш неявный дефолт пустого fp у vless/anytls (D-009), от
//     явного неотличим; против Xray ≥ v26.9.8 он мёртв в 4 случаях из 5, и это
//     наш выбор, а не провайдера — становится chrome.
//
// Правит tlsData на месте; возвращает (исходный отпечаток, была ли подмена
// непустого значения). Место вызова — сборка конфига: значение в узле
// нормативно (CANON §2), LxBox правит на том же шаге
// (heal_unknown_utls_fingerprints.dart).
func EnforceRealityFingerprint(tlsData map[string]interface{}) (original string, changed bool) {
	if tlsData == nil {
		return "", false
	}
	// Проверки «а вдруг reality выключен» здесь БОЛЬШЕ НЕТ (контракт 1.1.12):
	// блок с `enabled: false` до сборки не доезжает — его снимает санитайзер
	// правилом реестра `absent_when` у tls.reality, на всех входах сразу.
	// Рукописная копия того же правила означала бы, что одно место знает про
	// выключенный блок, а остальные нет.
	if _, ok := tlsData["reality"].(map[string]interface{}); !ok {
		return "", false
	}
	// REALITY без uTLS-блока — fatal «uTLS is required by reality client» при
	// создании outbound, поэтому блок не только правим, но и заводим.
	utls, ok := tlsData["utls"].(map[string]interface{})
	if !ok {
		utls = map[string]interface{}{"enabled": true}
		tlsData["utls"] = utls
	} else if en, has := utls["enabled"].(bool); !has || !en {
		utls["enabled"] = true
	}
	cur, _ := utls["fingerprint"].(string)
	switch cur {
	case "":
		utls["fingerprint"] = utlsJunkFallback
		return "", false
	case "random":
		utls["fingerprint"] = utlsJunkFallback
		return cur, true
	}
	return cur, false
}
