package subscription

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/registry"
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

// normalizePercentDecodeLoop applies URL-unescape until stable (fixes multiply-encoded alpn, etc.).
func normalizePercentDecodeLoop(s string) string {
	for {
		dec, err := url.QueryUnescape(s)
		if err != nil || dec == s {
			break
		}
		s = dec
	}
	return s
}

// tlsInsecureTrue — включён ли `insecure` по ЛЮБОМУ из написаний реестра.
//
// Схема нужна, потому что канон параметра у неё свой: у tuic это
// `allow_insecure`, у остальных — `insecure`, а набор написаний один и тот же
// (registry/tls.json tls.params.insecure). До W2d набор был зашит в шести
// местах и в каждом свой (DRIFT §2(a)).
func tlsInsecureTrue(q url.Values, scheme string) bool {
	if queryFlagTrue(q, scheme, "insecure") {
		return true
	}
	// Схемы, объявившие каноном другое имя (tuic), читаются по нему.
	return queryFlagTrue(q, scheme, "allow_insecure")
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

// utlsFingerprintFromQuery читает отпечаток по всем написаниям параметра из
// реестра и переводит написание ЗНАЧЕНИЯ: Xray-идентификаторы uTLS
// (`HelloChrome_120`, `hellofirefox_auto`) — это то же имя семейства в чужом
// диалекте, и развернуть его обязан маппер, иначе санитайзер увидит мусор там,
// где ссылка назвала валидное значение.
//
// Решения о значении здесь больше нет (SPEC 131 W2d): значение вне словаря
// ядра уезжает КАК ЕСТЬ, а снимет или заменит его санитайзер по правилу
// реестра (`utls_fp_unknown`). Канон берётся из ПЕРВОГО распознанного
// написания — мусорный `fp=qwerty` не должен перебивать явный
// `fingerprint=firefox` (D-029).
func utlsFingerprintFromQuery(q url.Values, scheme string) string {
	junkRaw := ""
	for _, k := range queryParamNames(scheme, "fp") {
		raw := strings.TrimSpace(queryGetFold(q, k))
		if raw == "" {
			continue
		}
		canon, junk := normalizeUTLSFingerprintEx(raw)
		if !junk {
			return canon
		}
		if junkRaw == "" {
			junkRaw = raw
		}
	}
	return junkRaw
}

// plaintextVLESSPorts are common subscription ports where TLS is typically off (plain HTTP / CF HTTP).
var plaintextVLESSPorts = map[int]struct{}{
	80: {}, 8080: {}, 8880: {}, 2052: {}, 2082: {}, 2086: {}, 2095: {},
}

func shouldVLESSSkipTLSForPort(port int) bool {
	_, ok := plaintextVLESSPorts[port]
	return ok
}

// uriTransportFromQuery builds sing-box V2Ray transport for VLESS/Trojan from URI query.
// See: https://sing-box.sagernet.org/configuration/shared/v2ray-transport/
func uriTransportFromQuery(q url.Values) (map[string]interface{}, bool) {
	typ := strings.ToLower(strings.TrimSpace(queryGetFold(q, "type")))
	headerType := strings.ToLower(strings.TrimSpace(queryGetFold(q, "headerType")))

	// Xray: TCP/raw with HTTP header camouflage → sing-box "http" transport (not plain TCP).
	if (typ == "raw" || typ == "tcp") && headerType == "http" {
		t := map[string]interface{}{"type": "http"}
		if p := queryGetFold(q, "path"); p != "" {
			t["path"] = p
		}
		if host := queryGetFold(q, "host"); host != "" {
			t["host"] = []string{host}
		}
		return t, true
	}

	switch typ {
	case "ws":
		t := map[string]interface{}{"type": "ws"}
		// path may carry Xray's `?ed=N` early-data tail; split it into the
		// sing-box max_early_data / early_data_header_name fields (issue #96).
		if p := queryGetFold(q, "path"); p != "" {
			applyWSEarlyData(t, p)
		}
		// Second spelling seen in the wild: flat `ed`/`eh` query params. The
		// path tail wins — it addresses one path, the flat pair the whole link
		// (SPEC 103, §9.E). `eh` without `ed` means nothing: the core enables
		// early data on max_early_data > 0.
		if _, already := t["max_early_data"]; !already {
			if ed, err := strconv.Atoi(strings.TrimSpace(queryGetFold(q, "ed"))); err == nil && ed > 0 {
				t["max_early_data"] = ed
				header := strings.TrimSpace(queryGetFold(q, "eh"))
				if header == "" {
					header = wsEarlyDataHeaderName
				}
				t["early_data_header_name"] = header
			}
		}
		// Many subscriptions set only sni= for TLS; reverse proxies expect WS Host to match vhost.
		host := strings.TrimSpace(queryGetFold(q, "host"))
		if host == "" {
			host = strings.TrimSpace(queryGetFold(q, "sni"))
		}
		if host == "" {
			host = strings.TrimSpace(queryGetFold(q, "obfsParam"))
		}
		if host != "" {
			t["headers"] = map[string]string{"Host": host}
		}
		return t, true
	case "grpc":
		t := map[string]interface{}{"type": "grpc"}
		sn := queryGetFold(q, "serviceName")
		if sn == "" {
			sn = queryGetFold(q, "service_name")
		}
		if sn != "" {
			t["service_name"] = sn
		} else if p := queryGetFold(q, "path"); p != "" {
			t["service_name"] = p
		}
		return t, true
	case "http":
		// HTTP transport: "host" is a list in sing-box (not a plain Host header).
		t := map[string]interface{}{"type": "http"}
		if p := queryGetFold(q, "path"); p != "" {
			t["path"] = p
		}
		if host := queryGetFold(q, "host"); host != "" {
			t["host"] = []string{host}
		}
		return t, true
	case "xhttp":
		// Xray "xhttp" (splithttp) → sing-box-lx "xhttp" transport. Distinct
		// wire protocol from httpupgrade; requires a core built with_xhttp
		// (sing-box-lx). See SPEC 071.
		return xhttpTransportFromQuery(q), true
	case "httpupgrade":
		// sing-box "httpupgrade" (HTTP/1.1 Upgrade). Kept separate from xhttp.
		t := map[string]interface{}{"type": "httpupgrade"}
		if p := queryGetFold(q, "path"); p != "" {
			// httpupgrade has no early data in sing-box: strip the Xray `?ed=N`
			// tail (and any residual encoding) instead of shipping it inside the
			// path, which the server answers with 404 (SPEC 103, D-028).
			//
			// Путь, состоящий ТОЛЬКО из хвоста (`path=?ed=2048` — реальный
			// паттерн панелей с корневым путём), даёт clean == "" — фолбэк
			// на исходную строку вернул бы `?ed=` обратно, ровно то, от чего
			// D-028 защищался. Корень — честный эквивалент.
			clean, _ := splitWSEarlyData(decodeResidualPercent(p))
			if clean == "" {
				clean = "/"
			}
			t["path"] = clean
		}
		if host := queryGetFold(q, "host"); host != "" {
			t["host"] = host
		}
		return t, true
	case "raw", "tcp", "":
		return nil, false
	default:
		return nil, false
	}
}

// xhttpStringField maps a transport JSON key (snake_case) to the URL spellings
// it may arrive under. The first non-empty source wins; queryGetFold already
// folds case, so we only list distinct spellings (snake vs camelCase).
type xhttpStringField struct {
	jsonKey string
	urlKeys []string
}

// xhttpStringFields are the v2 string-valued XHTTP transport fields (SPEC 002 v2,
// PARAM_MAP). mode/path/host are handled separately (path needs ?-tail trimming,
// host falls back differently, и вся тройка читается только из плоского слоя — D-097); these are pure passthrough — read as-is, emit
// under jsonKey. Value validation against the allowed sets is left to the core.
var xhttpStringFields = []xhttpStringField{
	{"session_placement", []string{"session_placement", "sessionPlacement"}},
	{"session_key", []string{"session_key", "sessionKey"}},
	{"seq_placement", []string{"seq_placement", "seqPlacement"}},
	{"seq_key", []string{"seq_key", "seqKey"}},
	{"uplink_data_placement", []string{"uplink_data_placement", "uplinkDataPlacement"}},
	{"uplink_data_key", []string{"uplink_data_key", "uplinkDataKey"}},
	{"uplink_chunk_size", []string{"uplink_chunk_size", "uplinkChunkSize"}},
	{"uplink_http_method", []string{"uplink_http_method", "uplinkHTTPMethod"}},
	{"x_padding_key", []string{"x_padding_key", "xPaddingKey"}},
	{"x_padding_header", []string{"x_padding_header", "xPaddingHeader"}},
	{"x_padding_placement", []string{"x_padding_placement", "xPaddingPlacement"}},
	{"x_padding_method", []string{"x_padding_method", "xPaddingMethod"}},
}

// xhttpRangeFields are sc*-fields the core expects as a "min-max" string but
// which real subscriptions often send as a bare number (or a float like 30.0)
// in the extra-JSON. xhttpGet normalizes those to strings before we read them.
var xhttpRangeFields = []xhttpStringField{
	{"sc_max_each_post_bytes", []string{"sc_max_each_post_bytes", "scMaxEachPostBytes"}},
	{"sc_min_posts_interval_ms", []string{"sc_min_posts_interval_ms", "scMinPostsIntervalMs"}},
	{"sc_stream_up_server_secs", []string{"sc_stream_up_server_secs", "scStreamUpServerSecs"}},
}

// xhttpIntFields are XHTTP fields the core decodes as int64 rather than as a
// string, so they must reach the transport as a number (SPEC 102).
var xhttpIntFields = []xhttpStringField{
	{"sc_max_buffered_posts", []string{"sc_max_buffered_posts", "scMaxBufferedPosts"}},
}

// xhttpBoolFields are the XHTTP flags emitted only when true; the core's default
// is the absent field.
var xhttpBoolFields = []xhttpStringField{
	{"no_grpc_header", []string{"no_grpc_header", "noGRPCHeader"}},
	{"no_sse_header", []string{"no_sse_header", "noSSEHeader"}},
	{"x_padding_obfs_mode", []string{"x_padding_obfs_mode", "xPaddingObfsMode"}},
}

// xhttpXmuxFields maps Xray's xmux members onto the core's snake_case names.
// h_keep_alive_period is an int for the core; the rest are strings (usually
// "min-max" ranges).
var xhttpXmuxFields = []xhttpStringField{
	{"max_concurrency", []string{"max_concurrency", "maxConcurrency"}},
	{"max_connections", []string{"max_connections", "maxConnections"}},
	{"c_max_reuse_times", []string{"c_max_reuse_times", "cMaxReuseTimes"}},
	{"h_max_request_times", []string{"h_max_request_times", "hMaxRequestTimes"}},
	{"h_max_reusable_secs", []string{"h_max_reusable_secs", "hMaxReusableSecs"}},
}

// xhttpXmuxIntFields are the xmux members the core decodes as int.
var xhttpXmuxIntFields = []xhttpStringField{
	{"h_keep_alive_period", []string{"h_keep_alive_period", "hKeepAlivePeriod"}},
}

// xhttpTransportFromQuery builds a sing-box-lx "xhttp" (Xray splithttp) transport
// from a VLESS/Trojan/VMess URI query. Distinct from "httpupgrade". Covers the
// full SPEC 002 v2 field set: the base trio (mode/path/host), padding, placement
// and key fields, x-padding obfs, and packet-up tuning. Values come from two
// sources merged into one lookup: flat query params and the `extra` URL-encoded
// JSON (extra wins for its keys — кроме базовой тройки mode/path/host, которую
// Xray всегда берёт из плоских, см. xhttpBuildTransport). Value normalization is
// otherwise left to the core. See SPEC 071 / sing-box-lx SPEC 002.
func xhttpTransportFromQuery(q url.Values) map[string]interface{} {
	return xhttpBuildTransport(xhttpMergeSource(q), xhttpFlattenQuery(q))
}

// xhttpBuildTransport is the single place where an XHTTP transport object is
// assembled, shared by the share-URI parser and the Xray-JSON converter so both
// branches support exactly the same field set (SPEC 102 R2). Values arrive
// pre-stringified in two layers: `primary` wins over `fallback` for keys present
// in both (SPEC 002 §1.5 — Xray's `extra` overrides the flat settings). Единственное
// исключение — mode/path/host: там всё наоборот, плоский слой перекрывает extra
// (D-097, разбор ниже у самой тройки).
//
// Callers are responsible for flattening their own source into these maps:
// the URI branch decodes the `extra` JSON and folds the query string, the Xray
// branch flattens `xhttpSettings` and its nested `extra` object.
func xhttpBuildTransport(primary, fallback map[string]string) map[string]interface{} {
	t := map[string]interface{}{"type": "xhttp"}

	// Базовая тройка host/path/mode — исключение из правила «extra побеждает»:
	// сам Xray в infra/conf/transport_method.go (SplitHTTPConfig.Build) после
	// разбора extra безусловно затирает её внешними значениями —
	//   extra.Host = c.Host; extra.Path = c.Path; extra.Mode = c.Mode
	// — то есть плоские поля выигрывают ДАЖЕ будучи пустыми. Поэтому здесь
	// читается только fallback (плоский слой), а одноимённые ключи из extra
	// игнорируются целиком. Кейс 4PDA #1755: ссылка несла плоские
	// mode=packet-up&path=/hls/v2/track/.../&host=media... и extra с пустыми
	// host/path/mode; со слиянием «extra побеждает» path съезжал на "/" (сервер
	// отвечал 404 unexpected upload status), а пустой mode давал auto → ядро
	// падало на «uplink_data_placement can be header only in packet-up mode».
	// Прочие ключи extra по-прежнему база (см. xhttpLookup ниже).
	if v := xhttpMapGetFold(fallback, "mode"); v != "" {
		t["mode"] = v
	}
	if p := xhttpCleanPath(xhttpMapGetFold(fallback, "path")); p != "" {
		t["path"] = p
	}
	if host := xhttpMapGetFold(fallback, "host"); host != "" {
		t["host"] = host
	}
	if pad := xhttpLookup(primary, fallback, "x_padding_bytes", "xPaddingBytes"); pad != "" {
		// "0-0" is a meaningful value (padding disabled), not an empty field, so
		// the guard tests for a non-empty string rather than a non-zero range.
		t["x_padding_bytes"] = pad
	}
	for _, f := range xhttpBoolFields {
		if xhttpLookupBool(primary, fallback, f.urlKeys...) {
			t[f.jsonKey] = true
		}
	}
	for _, f := range xhttpStringFields {
		if v := xhttpLookup(primary, fallback, f.urlKeys...); v != "" {
			t[f.jsonKey] = v
		}
	}
	for _, f := range xhttpRangeFields {
		if v := xhttpRange(xhttpLookup(primary, fallback, f.urlKeys...)); v != "" {
			t[f.jsonKey] = v
		}
	}
	for _, f := range xhttpIntFields {
		if n, ok := xhttpLookupInt(primary, fallback, f.urlKeys...); ok {
			t[f.jsonKey] = n
		}
	}
	if xmux := xhttpXmuxFromSource(primary, fallback); len(xmux) > 0 {
		t["xmux"] = xmux
	}
	return t
}

// xhttpModeInRegistryEnum — валиден ли `mode` по реестру
// (contract/registry/transports.json, вариант xhttp).
//
// Значений здесь НЕТ: закрытый enum и его исполнение (снять поле + код
// xhttp_param_reset с путём и значением) живут в реестре и исполняются
// санитайзером nodeflow — парсер их не дублирует (SPEC 131 §3.1). Спрашиваем
// реестр только для структурного правила ниже, которому нужно отличить
// «режим задан явно» от «режима нет», а мусорный режим — это «нет»: он до
// тела всё равно не доедет.
//
// Реестр недоступен (сломан файл) → считаем режим валидным: молча ослаблять
// структурный гард нельзя, а поднимать ошибку разбора из-за реестра — тем
// более.
func xhttpModeInRegistryEnum(mode string) bool {
	reg, err := registry.Get()
	if err != nil {
		return true
	}
	// Вариант xhttp у vless/vmess/trojan один и тот же (общая суб-схема
	// транспортов), поэтому схема для поиска роли не играет.
	f, ok := reg.Field("vless", "transport.xhttp.mode")
	if !ok || len(f.Values) == 0 {
		return true
	}
	for _, v := range f.Values {
		if str, isStr := v.(string); isStr && str == mode {
			return true
		}
	}
	return false
}

// xhttpGuardUplinkPlacement приводит пару (mode, uplink_data_placement) к
// форме, которую ядро принимает.
//
// Ядро отвергает ВЕСЬ конфиг, а не узел:
//
//	initialize outbound[N]: create client transport: xhttp:
//	v2ray-xhttp: uplink_data_placement can be header only in packet-up mode
//
// Проверено на 1.14.0-lx.30: `header` с `stream-up` и `header` без режима —
// fatal; `header` с `packet-up` — принимается. То есть одна запись подписки
// оставляет человека вообще без VPN, и молчать об этом нельзя.
//
// Два исхода, и они разные по смыслу:
//
//   - режима НЕТ (или пуст) → дописываем `packet-up`. `header` осмыслен
//     только в этом режиме, значит источник его подразумевал: мы
//     доопределяем недосказанное, а не спорим с автором ссылки;
//   - режим задан ЯВНО и он не `packet-up` → снимаем placement, режим не
//     трогаем. Переписать явный режим значило бы сменить проволочный
//     протокол узла — это хуже, чем снять одно поле (правило «не
//     переписывать явное значение пользователя»).
//
// Возвращает код предупреждения для узла (пусто = ничего не делали):
// сам гард узла не видит — он собирает транспорт из голых карт, — поэтому
// сообщает вызывающему, что произошло, а тот вешает пометку на узел.
func xhttpGuardUplinkPlacement(t map[string]interface{}) string {
	if t == nil {
		return ""
	}
	placement, _ := t["uplink_data_placement"].(string)
	if placement != "header" {
		return ""
	}
	mode, _ := t["mode"].(string)
	if mode != "" && !xhttpModeInRegistryEnum(mode) {
		// Мусорный режим санитайзер снимет по реестру, и до ядра доедет
		// узел БЕЗ режима. Значит это ветка «режима нет»: считать такой
		// режим «явно заданным не-packet-up» означало бы снять валидный
		// placement заодно с мусором (кейс mode=garbage + header).
		mode = ""
	}
	switch mode {
	case "packet-up":
		// Рабочая пара — не трогаем и молчим.
		return ""
	case "":
		t["mode"] = "packet-up"
		return WarnXHTTPModeForcedPacketUp
	default:
		delete(t, "uplink_data_placement")
		return WarnXHTTPParamReset
	}
}

// noteXHTTPPlacementGuard вешает на узел пометку о правке, сделанной гардом.
//
// Отдельная функция по образцу noteWSEarlyDataConverted: точек, где транспорт
// уже собран, а узел под рукой, несколько (URI vless/trojan, Xray
// streamSettings), и правило «что показать пользователю» должно жить в одном
// месте.
func noteXHTTPPlacementGuard(node *configtypes.ParsedNode, transport map[string]interface{}) {
	if node == nil || transport == nil {
		return
	}
	if code := xhttpGuardUplinkPlacement(transport); code != "" {
		node.AddWarning(code)
	}
}

// xhttpXmuxFromSource assembles the nested xmux object. Xray ships it as a
// sub-object of `extra`; callers flatten it into the same layers under its own
// member names, so the lookup is identical to the top-level fields.
func xhttpXmuxFromSource(primary, fallback map[string]string) map[string]interface{} {
	xmux := make(map[string]interface{}, len(xhttpXmuxFields))
	for _, f := range xhttpXmuxFields {
		if v := xhttpLookup(primary, fallback, f.urlKeys...); v != "" {
			xmux[f.jsonKey] = v
		}
	}
	for _, f := range xhttpXmuxIntFields {
		if n, ok := xhttpLookupInt(primary, fallback, f.urlKeys...); ok {
			xmux[f.jsonKey] = n
		}
	}
	if len(xmux) == 0 {
		return nil
	}
	return xmux
}

// xhttpLookupInt reads a numeric field. Values arrive as strings from both
// sources (the JSON layers are stringified on the way in), so "30" and "30.0"
// both yield 30. Reports false when the key is absent or not a number, leaving
// the field unset rather than writing a zero the core would act on.
func xhttpLookupInt(primary, fallback map[string]string, keys ...string) (int, bool) {
	raw := xhttpLookup(primary, fallback, keys...)
	if raw == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n, true
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return int(f), true
	}
	return 0, false
}

// xhttpFlattenQuery folds a URL query into the flat string map the shared
// builder consumes. Repeated keys keep the first value, matching queryGetFold.
func xhttpFlattenQuery(q url.Values) map[string]string {
	if len(q) == 0 {
		return nil
	}
	out := make(map[string]string, len(q))
	for k, vs := range q {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

// xhttpLookup returns the first non-empty value for any of the given key
// spellings, preferring the primary layer over the fallback for each spelling in
// turn. Keys are matched case-insensitively, so a subscription may ship either
// camelCase or snake_case.
func xhttpLookup(primary, fallback map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := xhttpMapGetFold(primary, k); v != "" {
			return v
		}
		if v := xhttpMapGetFold(fallback, k); v != "" {
			return v
		}
	}
	return ""
}

// xhttpMapGetFold reads a key from a flat map case-insensitively. The exact hit
// is tried first so the common path avoids scanning the map.
func xhttpMapGetFold(m map[string]string, key string) string {
	if len(m) == 0 {
		return ""
	}
	if v, ok := m[key]; ok {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
	}
	return ""
}

// xhttpLookupBool reads a flag under any of the given spellings, treating
// 1/true/yes as true (case-insensitive).
func xhttpLookupBool(primary, fallback map[string]string, keys ...string) bool {
	v := strings.ToLower(xhttpLookup(primary, fallback, keys...))
	return v == "1" || v == "true" || v == "yes"
}

// xhttpMergeSource decodes the `extra` query param (URL-encoded JSON) into a
// flat map of stringified values. Numbers become their canonical string ("30.0"
// → "30", "1000000" → "1000000"), bools become "true"/"false". Returns nil when
// there is no usable extra. Flat query params are read separately via xhttpGet,
// so this map only carries the extra-only keys.
//
// `xmux` — единственный вложенный объект, который XHTTP определяет, и Xray
// пишет его в `extra` именно объектом. Его члены разворачиваются в тот же
// плоский слой (их имена не конфликтуют с верхнеуровневыми, а builder собирает
// объект обратно) — ровно как это делает xrayFlattenScalars для JSON-ветки.
// Без этого вложенная форма молча терялась: share-URI понимал только плоскую,
// а импорт того же узла из Xray-конфига — обе (SPEC 102 R2).
func xhttpMergeSource(q url.Values) map[string]string {
	raw := strings.TrimSpace(queryGetFold(q, "extra"))
	if raw == "" {
		return nil
	}
	// queryGetFold returns the already percent-decoded value; the surviving
	// payload is the JSON object itself. Guard against double-encoded inputs.
	if !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		if dec, err := url.QueryUnescape(raw); err == nil {
			raw = dec
		}
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil
	}
	out := make(map[string]string, len(obj))
	for k, v := range obj {
		if nested, ok := v.(map[string]interface{}); ok {
			if strings.EqualFold(k, "xmux") {
				for nk, nv := range nested {
					if s := xhttpStringifyJSON(nv); s != "" {
						out[nk] = s
					}
				}
			}
			// Прочие вложенные объекты не несут полей, которые мы эмитим:
			// класть их сюда строкой значило бы кормить lookup мусором.
			continue
		}
		out[k] = xhttpStringifyJSON(v)
	}
	return out
}

// xhttpStringifyJSON renders a JSON scalar from `extra` as the string sing-box
// wants. Floats drop a redundant ".0" (encoding/json decodes every JSON number
// as float64), so 30.0 → "30" and 1000000 → "1000000" rather than "1e+06".
func xhttpStringifyJSON(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case nil:
		return ""
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// xhttpCleanPath strips a query-string tail from an XHTTP path. Real nodes ship
// path=/GaMeOpTiMiZeR?ed=2048 — the part after `?` is not the path (SPEC 002
// §4.1). The core normalizes the path itself, but the `?` is trimmed here.
func xhttpCleanPath(p string) string {
	p = strings.TrimSpace(p)
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	return p
}

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

// appendEarlyDataToPath re-encodes a positive max_early_data back into an Xray
// `?ed=N` path tail. Used by the share-URI exporters so a node → share-link →
// node round-trip preserves WebSocket early data (the inverse of splitWSEarlyData).
func appendEarlyDataToPath(path string, maxED int) string {
	if maxED <= 0 {
		return path
	}
	sep := "?"
	if strings.ContainsRune(path, '?') {
		sep = "&"
	}
	return path + sep + "ed=" + strconv.Itoa(maxED)
}

// xhttpRange normalizes an sc*-range value to the "min-max" string the core
// wants. A bare number N is left as "N" (the core accepts "N" and "N-N" alike);
// xhttpStringifyJSON has already dropped any ".0" float tail. Empty stays empty.
func xhttpRange(v string) string {
	return strings.TrimSpace(v)
}

// Чистка и проверка REALITY-полей (short_id, public_key) переехали в реестр
// (SPEC 131 W2d): short_id — normalize hex_only + max/len_parity, public_key —
// format base64_32 с required внутри блока reality. Прежние
// normalizeRealityShortID и isValidRealityPublicKey сняты: держать вторую
// копию правила рядом с реестром значило бы снова их рассинхронизировать.

// NormalizeRealityKeyShare приводит значение tls.reality.key_share к
// каноническому виду ядра: trim + lower-case, и только два значения enum'а
// (SPEC 089 ядра, sing-box-lx ≥ 1.14.1-lx.4, option/tls.go
// OutboundRealityOptions.KeyShare `enum:"hybrid,classical"`).
//
// Возвращает ("", false) на пустом значении — «как несёт отпечаток», ключа в
// конфиге просто нет, и это НЕ деградация. Возвращает ("", true) на мусоре:
// enum ядра закрытый, и чужое значение — ошибка загрузки ВСЕГО конфига (не
// узла), проверено `sing-box check` бинарём lx.4. Поэтому деградирует ПОЛЕ:
// ключ снимается, узел живёт с REALITY и поведением по умолчанию отпечатка
// (мягче гейта pbk, где невалидный ключ роняет весь REALITY-блок).
//
// Второе значение — «значение было испорчено», по образцу
// realityShortIDWouldDegrade: нормализатор зовут и с узлом под рукой
// (URI-парсеры), и без него (санитайзер импорта), поэтому код вешает
// вызывающий.
func NormalizeRealityKeyShare(s string) (value string, degraded bool) {
	switch v := strings.ToLower(strings.TrimSpace(s)); v {
	case "hybrid", "classical":
		return v, false
	case "":
		return "", false
	default:
		return "", true
	}
}

// applyTLSQueryExtras переводит общие TLS-параметры ссылки в поля тела.
//
// Обе операции — перевод диалекта, не решение: `alpn` в ссылке это строка
// через запятую, в теле — список (форма записи), а `insecure` приезжает под
// девятью именами (registry/tls.json tls.params.insecure). Значения элементов
// списка парсер не судит: элемент вне смысла снимет санитайзер.
func applyTLSQueryExtras(q url.Values, scheme string, tlsData map[string]interface{}) {
	if alpn := queryParam(q, scheme, "alpn"); alpn != "" {
		alpn = normalizePercentDecodeLoop(alpn)
		alpnList := strings.Split(alpn, ",")
		for i := range alpnList {
			alpnList[i] = strings.TrimSpace(alpnList[i])
		}
		tlsData["alpn"] = alpnList
	}
	if tlsInsecureTrue(q, scheme) {
		tlsData["insecure"] = true
	}
}

// noteECHIgnored вешает ech_ignored, если ссылка несла Xray-параметр `ech=`.
//
// Единственный URI-параметр, у которого НЕТ адреса в теле (SPEC 131 W2d,
// решение D-122): Xray-форма `ech=<public_name>+<resolver>` несёт ключ ЧУЖОГО
// клиента (public_name ≠ SNI узла), и рукопожатие с ним не состоится —
// device-verified, §320 LxBox. Поэтому параметр не переводится никуда, а
// снимается здесь с кодом.
//
// Блок `tls.ech{}` из sing-box-JSON при этом проходит НЕТРОНУТЫМ: ECH в ядре
// скомпилирован всегда (common/tls/ech.go под //go:build go1.24, тег with_ech
// намеренно отравлен), и нативная конфигурация рабочая. Различить эти два
// случая может только маппер: он один знает, ОТКУДА пришло значение, —
// санитайзер видит уже готовое тело (DRIFT §7.2).
//
// Прежде Go не читал `ech=` вовсе: параметр молча исчезал на allowlist-эмиттере,
// и кейс корпуса держал per-app override, потому что Dart код ставил, а Go нет.
func noteECHIgnored(node *configtypes.ParsedNode) {
	if node == nil {
		return
	}
	// Только само имя `ech`: `echfq` в реестре объявлен алиасом с пометкой
	// «не читается намеренно» (legacy pq-опция, снята в sing-box 1.13) — он
	// не Xray-форма ECH, и кода за него быть не должно.
	raw := strings.TrimSpace(queryGetFold(node.Query, "ech"))
	if raw == "" {
		return
	}
	// `ech=none` — способ подписки сказать «ECH выключен», то есть сообщать
	// не о чем: ничего не снято. Тот же разбор у LxBox (§320).
	if strings.EqualFold(raw, "none") {
		return
	}
	node.AddWarningWithParams(WarnECHIgnored, map[string]string{"query_name": "ech"})
}

// vlessTLSFromNode — карта tls для vless и признак «блок вообще есть».
//
// Маппер (SPEC 131 §3.1): переводит параметры ссылки в пути тела и решает
// ровно один вопрос — ЕСТЬ ли у узла блок tls. Этот вопрос санитайзеру
// недоступен (отсутствующий ключ для него неотличим от «не задан»), и он же
// единственный, где ссылка несёт структуру, а не значение:
//
//   - `security=none` → ключа tls нет ВОВСЕ (не `enabled:false`: явный
//     выключенный блок роняет ядра 1.14.0-lx.5..lx.18 в SIGSEGV, SPEC 045);
//   - `security=”` на портах открытого HTTP → того же вида «TLS не
//     предполагался» (registry/tls.json policy.plaintext_ports);
//   - `pbk=` в ссылке → в теле появляется блок reality.
//
// Значения полей дальше не судятся: мусорный pbk снимет санитайзер правилом
// `reality_pbk_invalid` (а с ним и весь блок reality — public_key там
// required), sid — `reality_short_id_invalid`, key_share — своим кодом,
// отпечаток вне словаря станет `chrome` с `utls_fp_unknown`.
func vlessTLSFromNode(node *configtypes.ParsedNode) (map[string]interface{}, bool) {
	q := node.Query
	const scheme = "vless"
	sec := strings.ToLower(queryParam(q, scheme, "security"))

	if sec == "none" {
		return nil, false
	}
	if sec == "" && shouldVLESSSkipTLSForPort(node.Port) {
		return nil, false
	}

	sni := queryParam(q, scheme, "sni")
	if sni == "" {
		sni = node.Server
	}
	tlsData := map[string]interface{}{
		"enabled":     true,
		"server_name": sni,
		"utls": map[string]interface{}{
			"enabled": true,
			// Пустой fp у vless — дефолт `random` (D-009, паритет с LxBox).
			// Это не дефолт ЯДРА (у него пустой fp = chrome), а конвенция
			// обеих сторон, поэтому материализуется здесь: реестр выражает
			// дефолты только через default_when, которого у этого поля нет.
			"fingerprint": utlsFingerprintOrDefault(q, scheme, "random"),
		},
	}
	// Блок reality заводится по НАЛИЧИЮ pbk, а не по security=reality: живые
	// xhttp+reality-ссылки несут ключ без явного security. Пустой pbk блока
	// не создаёт — иначе у каждого plain-TLS узла появлялся бы reality с
	// required-полем и кодом на ровном месте.
	if pbk := queryParam(q, scheme, "pbk"); pbk != "" {
		reality := map[string]interface{}{
			"enabled":    true,
			"public_key": pbk,
		}
		if sid := queryParam(q, scheme, "sid"); sid != "" {
			reality["short_id"] = sid
		}
		if ks := queryParam(q, scheme, "key_share"); ks != "" {
			reality["key_share"] = ks
		}
		tlsData["reality"] = reality
	}
	applyTLSQueryExtras(q, scheme, tlsData)
	return tlsData, true
}

// applyTLSCamouflageFromQuery переводит в тело маскировочные параметры ссылки:
// `fp` → блок tls.utls, `pbk`/`sid`/`key_share` → блок tls.reality.
//
// Маппер, а не суждение: он решает только вопрос «есть ли блок», который
// санитайзеру недоступен (отсутствующий ключ неотличим от «не задано»), —
// ровно тот же вопрос, что у vlessTLSFromNode. Годность значений и
// применимость блока к схеме судит реестр: мусорный отпечаток станет `chrome`
// с utls_fp_unknown, мусорный pbk снимет блок целиком (reality_pbk_invalid),
// а на QUIC-протоколах оба блока запрещены (forbidden_for + forbidden_codes,
// код tls_not_applicable_quic) и снимаются с кодом.
//
// Зовут её QUIC-схемы, где прежде параметры пропадали МОЛЧА: hysteria2 и tuic
// не читали `fp` вовсе с комментарием «uTLS на QUIC не читается», а импорт
// того же узла телом снимал блок частной веткой quicOutboundTypes. Один и тот
// же узел давал на двух входах одно тело, но разные наборы кодов — ссылка не
// говорила пользователю ничего. Пустого дефолта отпечатка здесь нет (в отличие
// от vless `random`, D-009): его нечему материализовать — блока на QUIC не
// будет в любом случае.
func applyTLSCamouflageFromQuery(q url.Values, scheme string, tlsData map[string]interface{}) {
	if fp := utlsFingerprintFromQuery(q, scheme); fp != "" {
		tlsData["utls"] = map[string]interface{}{
			"enabled":     true,
			"fingerprint": fp,
		}
	}
	if pbk := queryParam(q, scheme, "pbk"); pbk != "" {
		reality := map[string]interface{}{
			"enabled":    true,
			"public_key": pbk,
		}
		if sid := queryParam(q, scheme, "sid"); sid != "" {
			reality["short_id"] = sid
		}
		if ks := queryParam(q, scheme, "key_share"); ks != "" {
			reality["key_share"] = ks
		}
		tlsData["reality"] = reality
	}
}

// utlsFingerprintOrDefault — отпечаток из ссылки либо конвенция схемы.
func utlsFingerprintOrDefault(q url.Values, scheme, def string) string {
	if fp := utlsFingerprintFromQuery(q, scheme); fp != "" {
		return fp
	}
	return def
}

// tlsServerNameFromQuery — SNI узла: `sni` (со всеми написаниями реестра),
// иначе адрес сервера.
//
// Это выбор ИСТОЧНИКА поля, а не суждение о значении: ядро принимает любой
// server_name (вердикт D), и снимать его санитайзеру не за что. Но значение,
// которое не может быть именем хоста, — не имя хоста, а мусор подписки, и
// подставлять его в SNI значит отправить рукопожатие в никуда: узел «есть» и
// молча не работает (DRIFT §7.5, решение владельца — эвристика на всех
// TLS-схемах). Признак «может быть именем хоста» — наличие точки (домен) или
// двоеточия (IPv6); таковы обе стороны с самого начала.
//
// Одна функция на все схемы вместо четырёх копий: у anytls копия отличалась
// (ловила только пустую строку) и пропускала `🔒` в конфиг.
func tlsServerNameFromQuery(q url.Values, scheme, server string) string {
	sni := queryParam(q, scheme, "sni")
	if sni != "" && strings.ContainsAny(sni, ".:") {
		return sni
	}
	return server
}

// trojanTLSFromNode returns the sing-box tls map for Trojan (WebSocket/raw over
// TLS) and whether a tls block should be emitted at all.
//
// security=none omits the key entirely rather than emitting
// `"tls":{"enabled":false}` — same contract as vlessTLSFromNode. The explicit
// disabled block is what sing-box cores 1.14.0-lx.5..lx.18 crash on: the
// upstream ECH-retry commit builds a TLS dialer whenever a tls block is
// present, while the config constructor returns (nil, nil) for enabled:false,
// so the dialer wraps a nil config and SIGSEGVs on the first dial — URL test
// included, killing the whole core process (sing-box-lx SPEC 045). Omitting
// the key yields the same plain-TCP dial on every core version.
// Схему передаёт вызывающий: ту же функцию зовёт http-proxy-парсер, а
// написания параметров (sni→peer→host, девять имён insecure) берутся из
// секции реестра ИМЕННО этой схемы.
func trojanTLSFromNode(node *configtypes.ParsedNode, scheme string) (map[string]interface{}, bool) {
	q := node.Query
	if strings.ToLower(queryParam(q, scheme, "security")) == "none" {
		return nil, false
	}

	sni := queryParam(q, scheme, "sni")
	if sni == "" {
		sni = node.Server
	}

	tlsData := map[string]interface{}{
		"enabled":     true,
		"server_name": sni,
	}
	// У trojan/http дефолта отпечатка нет (в отличие от vless, D-009): нет
	// параметра — нет и блока utls.
	if fp := utlsFingerprintFromQuery(q, scheme); fp != "" {
		tlsData["utls"] = map[string]interface{}{
			"enabled":     true,
			"fingerprint": fp,
		}
	}
	applyTLSQueryExtras(q, scheme, tlsData)
	return tlsData, true
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
	reality, ok := tlsData["reality"].(map[string]interface{})
	if !ok {
		return "", false
	}
	if enabled, has := reality["enabled"].(bool); has && !enabled {
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

// Набор отпечатков с гибридным key share (D-119) переехал в реестр:
// tls.json, body/utls/fingerprint/advisory — правило «любой отпечаток, кроме
// перечисленных, при наличии tls.reality.enabled → код reality_fp_not_chrome».
// Прежде тот же список лежал здесь и ставился четырьмя копиями в парсерах
// (vless, anytls, Xray-конверт, sing-box-импорт); одна из копий отставала от
// решения владельца о firefox/safari, и узел получал подсказку на одном входе
// и не получал на другом (SPEC 131 W2d).
