// Package config: outbound_generator.go — генерация outbounds для sing-box из ParserConfig и подписок.
//
// # Логика работы
//
// Вход: ParserConfig (источники подписок proxies, глобальные селекторы outbounds) и функция загрузки нод.
// Выход: массив JSON-строк для вставки в config.json (ноды + локальные селекторы + глобальные селекторы).
//
// Зачем три прохода:
//   - Селекторы могут ссылаться друг на друга через addOutbounds (например "proxy-out" включает "auto-proxy-out").
//   - Пустой селектор (0 нод и все динамические addOutbounds тоже пустые) не должен попадать в конфиг и не должен
//     учитываться как валидный addOutbound у других. Поэтому сначала собираем все селекторы и только ноды (pass 1),
//     затем в порядке зависимостей считаем «полный» размер каждого и флаг isValid (pass 2), затем генерируем JSON
//     только для валидных и с отфильтрованным списком addOutbounds (pass 3).
//
// Этапы:
//
//  1. Загрузка нод: для каждого proxy source вызывается loadNodesFunc → allNodes, nodesBySource.
//  2. Генерация JSON нод: каждый ParsedNode → одна или две JSON-строки (GenerateNodeJSON; при Jump — SOCKS затем основной с detour).
//  3. Pass 1 — buildOutboundsInfo: по конфигу строим map[tag]*outboundInfo для всех селекторов (локальных и глобальных),
//     для каждого — отфильтрованные ноды и начальный outboundCount = len(filteredNodes). isValid пока false.
//  4. Pass 2 — computeOutboundValidity: топологическая сортировка по графу зависимостей addOutbounds;
//     в этом порядке для каждого селектора считаем outboundCount = nodes + число валидных addOutbounds (динамические
//     с outboundCount > 0 + константы типа direct-out). isValid = (outboundCount > 0).
//  5. Pass 3 — generateSelectorJSONs: для каждого селектора с isValid == true вызываем GenerateSelectorWithFilteredAddOutbounds
//     (в список addOutbounds попадают только валидные динамические и константы). Итог: срез JSON локальных и глобальных селекторов.
//
// Итоговый порядок в OutboundsJSON: [ ноды..., локальные селекторы..., глобальные селекторы... ].
//
// Фильтрация нод для селекторов задаётся в ParserConfig (filters: literal, /regex/i, !literal, !/regex/i по полям tag, host, scheme и т.д.).
// Реализация фильтров — в outbound_filter.go (filterNodesForSelector, matchesFilter, matchesPattern и др.).
//
// Разбиение по файлам (SPEC 070): этот файл — публичные генераторы
// (GenerateNodeJSON, GenerateSelectorWithFilteredAddOutbounds, GenerateEndpointJSON,
// GenerateOutboundsFromParserConfig); трёхпроходный алгоритм валидности —
// outbound_validity.go; низкоуровневые JSON-хелперы — outbound_jsonbuilder.go;
// фильтры нод — outbound_filter.go.
package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/debuglog"
)

// OutboundGenerationResult is the return value of GenerateOutboundsFromParserConfig: slice of JSON strings
// (nodes, then local selectors, then global selectors) and counts for each category.
// WireGuard nodes go to EndpointsJSON (sing-box endpoints), not OutboundsJSON.
type OutboundGenerationResult struct {
	OutboundsJSON        []string // Generated JSON lines for outbounds array (nodes, then local, then global selectors)
	EndpointsJSON        []string // Generated JSON lines for endpoints array (WireGuard nodes only)
	NodesCount           int      // Number of node outbounds (non-WireGuard)
	EndpointsCount       int      // Number of WireGuard endpoint nodes
	LocalSelectorsCount  int      // Number of local (per-source) selectors
	GlobalSelectorsCount int      // Number of global selectors
	// Per-source outcomes (only enabled sources are counted; disabled sources
	// don't participate). A source counts as failed if loadNodesFunc returned
	// an error OR returned zero nodes — silent-empty is failure from the
	// user's point of view.
	TotalSources     int
	SucceededSources int
	FailedSources    int
	// SkippedNaiveNodes — naive nodes dropped because the running sing-box
	// core can't create them (SPEC 044 feature-probe: no with_naive_outbound
	// tag, or a purego build without libcronet next to the binary). One such
	// node would otherwise fail `sing-box check` for the whole config.
	// SkippedNaiveReason carries the probe verdict for UI surfacing.
	SkippedNaiveNodes  int
	SkippedNaiveReason string

	// EmptyDirections — Направления, чей фильтр не поймал ни одного узла
	// (SPEC 104). Отображаемые имена, для превью и статуса: в конфиг такое
	// направление уезжает с запасным составом [block, direct], то есть
	// молча блокирует трафик своих правил — пользователь обязан узнать.
	//
	// Заполняется ТОЛЬКО когда виноват фильтр: пустой фильтр при нуле узлов
	// — это «подписка не загрузилась», и чинить надо её.
	EmptyDirections []string

	// BrokenChains — источники-цепочки (SPEC 110), не ставшие узлами: ядро
	// без `with_lx_chain`, недошедшая позиция, нарушенный инвариант.
	// Пользователь обязан узнать, почему настроенный маршрут не работает, —
	// молча выпавшая цепочка выглядит как «лаунчер потерял настройку».
	BrokenChains []ChainDegradation

	// ChainCycles — цепочки, не вошедшие в состав Направления, через
	// которое проходят (SPEC 110 T9). Ядро на таком не падает, но маршрут
	// вышел бы не тот, что задуман, — пользователь должен узнать, почему
	// его цепочки нет в группе.
	ChainCycles []ChainCycle
}

// NaiveSupportProbe — hook installed by the app layer (core.AppController):
// reports whether the running sing-box core supports naive outbounds and why
// not. nil (parser-level tests, standalone use) → assume supported. Same
// package-level-hook pattern as subscription.LookupCachedBody.
var NaiveSupportProbe func() (supported bool, reason string)

// GenerateNodeJSON returns a single JSON object string for one proxy node (sing-box outbound).
// Field order and presence follow sing-box expectations. Supports: vless, vmess, trojan, shadowsocks, hysteria2, tuic, naive, masque, anytls, ssh, socks.
// Includes optional TLS (including reality), transport (ws/http/grpc), and protocol-specific options.
// Returned string ends with a trailing comma and may include a leading comment line (node label) for readability.
func GenerateNodeJSON(node *ParsedNode) (string, error) {
	// SPEC 094 A5: a group imported from a sing-box config is a node in the
	// subscription's own list, not a separate entry in the wizard's outbound
	// configurator. It carries no server/server_port, so it gets its own
	// emitter rather than threading "skip these fields" through the whole
	// per-scheme switch below.
	if node.Scheme == SchemeGroup {
		return generateGroupNodeJSON(node)
	}

	// Manual config_json node: the map is the source of truth — serializing
	// it as-is is the whole point (types and fields the per-scheme switch
	// below does not know about must survive). Only tag/detour are restamped.
	if node.EmitRaw {
		return generateRawNodeJSON(node)
	}

	// Build JSON with correct field order
	var parts []string

	// 1. tag
	parts = append(parts, fmt.Sprintf(`"tag":%s`, marshalJSONString(node.Tag)))

	// 2. type (sing-box uses "socks" + version for SOCKS5 URIs, not a separate socks5 type)
	if node.Scheme == "ss" {
		parts = append(parts, fmt.Sprintf(`"type":%s`, marshalJSONString("shadowsocks")))
	} else if node.Scheme == "socks" || node.Scheme == "socks5" {
		parts = append(parts, fmt.Sprintf(`"type":%s`, marshalJSONString("socks")))
	} else {
		parts = append(parts, fmt.Sprintf(`"type":%s`, marshalJSONString(node.Scheme)))
	}

	// 3. server
	parts = append(parts, fmt.Sprintf(`"server":%s`, marshalJSONString(node.Server)))

	// 4. server_port (prefer outbound map: buildOutbound may adjust port, e.g. vision-udp443 → 443)
	serverPort := node.Port
	if node.Outbound != nil {
		if sp, ok := node.Outbound["server_port"].(int); ok && sp > 0 {
			serverPort = sp
		}
	}
	parts = append(parts, fmt.Sprintf(`"server_port":%d`, serverPort))

	// 5. uuid (for vless/vmess) or password (for trojan) or method/password (for ss)
	if node.Scheme == "vless" || node.Scheme == "vmess" {
		parts = append(parts, fmt.Sprintf(`"uuid":%s`, marshalJSONString(node.UUID)))

		if node.Scheme == "vmess" {
			if security, ok := node.Outbound["security"].(string); ok && security != "" {
				parts = append(parts, fmt.Sprintf(`"security":%s`, marshalJSONString(security)))
			}
			if alterID, ok := node.Outbound["alter_id"].(int); ok {
				parts = append(parts, fmt.Sprintf(`"alter_id":%d`, alterID))
			}
		}
	} else if node.Scheme == "trojan" {
		parts = append(parts, fmt.Sprintf(`"password":%s`, marshalJSONString(node.UUID)))
	} else if node.Scheme == "hysteria2" {
		// Password is required for Hysteria2
		if password, ok := node.Outbound["password"].(string); ok && password != "" {
			passwordJSON, err := json.Marshal(password)
			if err != nil {
				return "", fmt.Errorf("failed to marshal hysteria2 password: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"password":%s`, string(passwordJSON)))
		}
		// server_ports (optional): sing-box expects each entry as low:high; normalize bare ports from sources.
		if serverPorts, ok := node.Outbound["server_ports"].([]string); ok && len(serverPorts) > 0 {
			serverPorts = subscription.NormalizeHysteria2ServerPortsSlice(serverPorts)
			serverPortsJSON, err := json.Marshal(serverPorts)
			if err != nil {
				return "", fmt.Errorf("failed to marshal hysteria2 server_ports: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"server_ports":%s`, string(serverPortsJSON)))
		}
		// up_mbps (optional)
		if upMbps, ok := node.Outbound["up_mbps"].(int); ok && upMbps > 0 {
			parts = append(parts, fmt.Sprintf(`"up_mbps":%d`, upMbps))
		}
		// down_mbps (optional)
		if downMbps, ok := node.Outbound["down_mbps"].(int); ok && downMbps > 0 {
			parts = append(parts, fmt.Sprintf(`"down_mbps":%d`, downMbps))
		}
		// obfs (optional)
		if obfs, ok := node.Outbound["obfs"].(map[string]interface{}); ok && len(obfs) > 0 {
			var obfsParts []string
			if obfsType, ok := obfs["type"].(string); ok {
				obfsParts = append(obfsParts, fmt.Sprintf(`"type":%s`, marshalJSONString(obfsType)))
			}
			if obfsPassword, ok := obfs["password"].(string); ok && obfsPassword != "" {
				obfsPasswordJSON, err := json.Marshal(obfsPassword)
				if err != nil {
					return "", fmt.Errorf("failed to marshal hysteria2 obfs password: %w", err)
				}
				obfsParts = append(obfsParts, fmt.Sprintf(`"password":%s`, string(obfsPasswordJSON)))
			}
			// Gecko packet-size bounds sit flat inside obfs (option/hysteria2.go,
			// Hysteria2ObfsGecko). Without these the parser reads them and the
			// emitter silently drops them — the emitter/parser pairing trap.
			for _, key := range []string{"min_packet_size", "max_packet_size"} {
				if n := obfsIntField(obfs[key]); n > 0 {
					obfsParts = append(obfsParts, fmt.Sprintf(`"%s":%d`, key, n))
				}
			}
			if len(obfsParts) > 0 {
				obfsJSON := "{" + strings.Join(obfsParts, ",") + "}"
				parts = append(parts, fmt.Sprintf(`"obfs":%s`, obfsJSON))
			}
		}
	} else if node.Scheme == "ss" {
		// Extract method and password from outbound
		// Use json.Marshal to properly escape strings for JSON (handles binary data correctly)
		// This prevents invalid \xXX escape sequences that JSON doesn't support
		if method, ok := node.Outbound["method"].(string); ok && method != "" {
			methodJSON, err := json.Marshal(method)
			if err != nil {
				return "", fmt.Errorf("failed to marshal shadowsocks method: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"method":%s`, string(methodJSON)))
		}
		if password, ok := node.Outbound["password"].(string); ok && password != "" {
			passwordJSON, err := json.Marshal(password)
			if err != nil {
				return "", fmt.Errorf("failed to marshal shadowsocks password: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"password":%s`, string(passwordJSON)))
		}
	} else if (node.Scheme == "socks" || node.Scheme == "socks5") && node.Outbound != nil {
		if ver, ok := node.Outbound["version"].(string); ok && ver != "" {
			parts = append(parts, fmt.Sprintf(`"version":%s`, marshalJSONString(ver)))
		}
		if username, ok := node.Outbound["username"].(string); ok && username != "" {
			usernameJSON, err := json.Marshal(username)
			if err != nil {
				return "", fmt.Errorf("failed to marshal socks username: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"username":%s`, string(usernameJSON)))
		}
		if password, ok := node.Outbound["password"].(string); ok && password != "" {
			passwordJSON, err := json.Marshal(password)
			if err != nil {
				return "", fmt.Errorf("failed to marshal socks password: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"password":%s`, string(passwordJSON)))
		}
	} else if node.Scheme == "naive" && node.Outbound != nil {
		// buildNaiveOutbound (node_parser_naive.go) populates username/password and
		// optional quic / quic_congestion_control / extra_headers; emit them here so
		// sing-box receives a complete naive outbound. Anonymous URIs (no userinfo)
		// legitimately have neither username nor password — both are emitted only when set.
		if username, ok := node.Outbound["username"].(string); ok && username != "" {
			usernameJSON, err := json.Marshal(username)
			if err != nil {
				return "", fmt.Errorf("failed to marshal naive username: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"username":%s`, string(usernameJSON)))
		}
		if password, ok := node.Outbound["password"].(string); ok && password != "" {
			passwordJSON, err := json.Marshal(password)
			if err != nil {
				return "", fmt.Errorf("failed to marshal naive password: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"password":%s`, string(passwordJSON)))
		}
		if quic, ok := node.Outbound["quic"].(bool); ok && quic {
			parts = append(parts, `"quic":true`)
			if cc, ok := node.Outbound["quic_congestion_control"].(string); ok && cc != "" {
				parts = append(parts, fmt.Sprintf(`"quic_congestion_control":%s`, marshalJSONString(cc)))
			}
		}
		if hdrs, ok := node.Outbound["extra_headers"].(map[string]interface{}); ok && len(hdrs) > 0 {
			hdrJSON, err := json.Marshal(hdrs)
			if err != nil {
				return "", fmt.Errorf("failed to marshal naive extra_headers: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"extra_headers":%s`, string(hdrJSON)))
		}
	} else if node.Scheme == "http" && node.Outbound != nil {
		// parseHTTPProxyURI (node_parser_http.go) populates username/password,
		// path and headers; the TLS block (https-form only) is emitted by the
		// shared section below. This branch is required — without it the
		// per-scheme switch falls through to the trailing } and the outbound
		// silently loses everything but {tag,type,server,server_port} (known
		// emitter-parser-pairing trap; also affects sing-box-import http nodes,
		// see singbox_import.go:315).
		if username, ok := node.Outbound["username"].(string); ok && username != "" {
			usernameJSON, err := json.Marshal(username)
			if err != nil {
				return "", fmt.Errorf("failed to marshal http username: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"username":%s`, string(usernameJSON)))
		}
		if password, ok := node.Outbound["password"].(string); ok && password != "" {
			passwordJSON, err := json.Marshal(password)
			if err != nil {
				return "", fmt.Errorf("failed to marshal http password: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"password":%s`, string(passwordJSON)))
		}
		if path, ok := node.Outbound["path"].(string); ok && path != "" {
			parts = append(parts, fmt.Sprintf(`"path":%s`, marshalJSONString(path)))
		}
		if hdrs, ok := node.Outbound["headers"].(map[string]interface{}); ok && len(hdrs) > 0 {
			hdrJSON, err := json.Marshal(hdrs)
			if err != nil {
				return "", fmt.Errorf("failed to marshal http headers: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"headers":%s`, string(hdrJSON)))
		}
	} else if node.Scheme == "tuic" && node.Outbound != nil {
		// uuid + password required; congestion_control / udp_relay_mode /
		// zero_rtt_handshake / heartbeat optional. The TLS block is emitted by the
		// shared section below (buildTuicTLS always sets node.Outbound["tls"]).
		if uuid, ok := node.Outbound["uuid"].(string); ok && uuid != "" {
			parts = append(parts, fmt.Sprintf(`"uuid":%s`, marshalJSONString(uuid)))
		}
		if password, ok := node.Outbound["password"].(string); ok && password != "" {
			passwordJSON, err := json.Marshal(password)
			if err != nil {
				return "", fmt.Errorf("failed to marshal tuic password: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"password":%s`, string(passwordJSON)))
		}
		if cc, ok := node.Outbound["congestion_control"].(string); ok && cc != "" {
			parts = append(parts, fmt.Sprintf(`"congestion_control":%s`, marshalJSONString(cc)))
		}
		if urm, ok := node.Outbound["udp_relay_mode"].(string); ok && urm != "" {
			parts = append(parts, fmt.Sprintf(`"udp_relay_mode":%s`, marshalJSONString(urm)))
		}
		if zr, ok := node.Outbound["zero_rtt_handshake"].(bool); ok && zr {
			parts = append(parts, `"zero_rtt_handshake":true`)
		}
		if hb, ok := node.Outbound["heartbeat"].(string); ok && hb != "" {
			parts = append(parts, fmt.Sprintf(`"heartbeat":%s`, marshalJSONString(hb)))
		}
	} else if node.Scheme == "masque" && node.Outbound != nil {
		// parseMasqueURI / warp.ToMasqueOutbound populate base64-DER keys, ip/ipv6
		// tunnel addresses and transport knobs; sing-box rejects the outbound
		// without them ("at least one of ip/ipv6 is required").
		//
		// The HTTP version is `vhttp` and the server name lives in the shared
		// `tls` block below — masque no longer has its own dialect (core SPEC
		// 062, schema present since 1.14.0-lx.26). The legacy `network`/`sni`
		// spellings are accepted on input by the parsers and normalized there,
		// so nothing flat reaches this point.
		for _, key := range []string{"private_key", "public_key", "ip", "ipv6", "profile", "vhttp", "idle_timeout", "keep_alive_period"} {
			if v, ok := node.Outbound[key].(string); ok && v != "" {
				parts = append(parts, fmt.Sprintf(`%s:%s`, marshalJSONString(key), marshalJSONString(v)))
			}
		}
		if mtu, ok := node.Outbound["mtu"].(int); ok && mtu > 0 {
			parts = append(parts, fmt.Sprintf(`"mtu":%d`, mtu))
		}
	} else if node.Scheme == "anytls" && node.Outbound != nil {
		// buildAnyTLSOutbound stores the credential and session-pool tuning in the
		// outbound map; the mandatory TLS block is emitted by the shared section below.
		for _, key := range []string{"password", "idle_session_check_interval", "idle_session_timeout"} {
			if v, ok := node.Outbound[key].(string); ok && v != "" {
				parts = append(parts, fmt.Sprintf(`%s:%s`, marshalJSONString(key), marshalJSONString(v)))
			}
		}
		if n, ok := node.Outbound["min_idle_session"].(int); ok {
			parts = append(parts, fmt.Sprintf(`"min_idle_session":%d`, n))
		}
	} else if node.Scheme == "ssh" && node.Outbound != nil {
		// buildSSHOutbound stores credentials and host-key material in the outbound map.
		for _, key := range []string{"user", "password", "private_key", "private_key_path", "private_key_passphrase", "client_version"} {
			if v, ok := node.Outbound[key].(string); ok && v != "" {
				parts = append(parts, fmt.Sprintf(`%s:%s`, marshalJSONString(key), marshalJSONString(v)))
			}
		}
		for _, key := range []string{"host_key", "host_key_algorithms"} {
			if list, ok := node.Outbound[key].([]string); ok && len(list) > 0 {
				listJSON, err := json.Marshal(list)
				if err != nil {
					return "", fmt.Errorf("failed to marshal ssh %s: %w", key, err)
				}
				parts = append(parts, fmt.Sprintf(`%s:%s`, marshalJSONString(key), string(listJSON)))
			}
		}
	}

	// 6. flow (if present) — use node.Outbound["flow"] when set so Xray-only values like
	// xtls-rprx-vision-udp443 stay in node.Flow for filters but sing-box gets xtls-rprx-vision
	flowOut := node.Flow
	if node.Outbound != nil {
		if f, ok := node.Outbound["flow"].(string); ok && f != "" {
			flowOut = f
		}
	}
	// flow has exactly two valid outputs in sing-box: "" (plain VLESS) or
	// "xtls-rprx-vision". Two filters enforce that:
	//
	//  1. Transport guard — vision is valid ONLY over "bare" TLS/Reality; it is
	//     incompatible with any v2ray transport (ws/grpc/http/httpupgrade/xhttp),
	//     which sing-box rejects at load time. Drop flow when a transport is
	//     present (a stray flow in the URI, or an xhttp node that also carries
	//     one). See the XHTTP/XTLS-Vision note in sing-box-lx docs/lx-config.md.
	//  2. Value whitelist — only "xtls-rprx-vision" is emitted. Everything else
	//     (literal "none" that x3-ui writes, the removed xtls-rprx-direct/origin/
	//     splice, or any junk) is NOT a value sing-box understands and would make
	//     it reject the config — so it is dropped to "" = plain VLESS. node.Flow
	//     keeps the original value for skip-filters; only emission is filtered.
	//     (xtls-rprx-vision-udp443 was already normalized to xtls-rprx-vision in
	//     buildOutbound, so it passes the whitelist.)
	if flowOut != "" && outboundHasTransport(node.Outbound) {
		debuglog.DebugLog("GenerateNodeJSON: dropping flow=%q on %q — incompatible with a v2ray transport", flowOut, node.Tag)
		flowOut = ""
	}
	if flowOut != "" && flowOut != "xtls-rprx-vision" {
		debuglog.DebugLog("GenerateNodeJSON: dropping unsupported flow=%q on %q — sing-box accepts only \"\" or xtls-rprx-vision", flowOut, node.Tag)
		flowOut = ""
	}
	if flowOut != "" {
		parts = append(parts, fmt.Sprintf(`"flow":%s`, marshalJSONString(flowOut)))
	}
	if node.Scheme == "vless" && node.Outbound != nil {
		if pe, ok := node.Outbound["packet_encoding"].(string); ok && pe != "" {
			parts = append(parts, fmt.Sprintf(`"packet_encoding":%s`, marshalJSONString(pe)))
		}
		// VLESS post-quantum encryption layer (core option/vless.go Encryption).
		// Without this the parser read the field and the emitter dropped it —
		// the emitter/parser pairing trap (SPEC 103).
		if enc, ok := node.Outbound["encryption"].(string); ok && enc != "" {
			parts = append(parts, fmt.Sprintf(`"encryption":%s`, marshalJSONString(enc)))
		}
	}

	parts = appendOutboundTransportParts(parts, node.Outbound)

	// 7. tls (if present) - with correct field order
	if node.Outbound != nil {
		if tlsData, ok := node.Outbound["tls"].(map[string]interface{}); ok {
			if disabled, ok := tlsData["enabled"].(bool); ok && !disabled {
				// Omit the block entirely instead of emitting
				// `"tls":{"enabled":false}`. Both mean "dial plain TCP", but the
				// explicit disabled form crashes sing-box cores
				// 1.14.0-lx.5..lx.18 on the first dial of a trojan/vless node
				// (nil TLS config wrapped in a live dialer — SPEC 045), taking
				// the whole core process down. Backstop for nodes that reach the
				// generator without going through the URI parsers.
			} else {
				var tlsParts []string

				if enabled, ok := tlsData["enabled"].(bool); ok {
					tlsParts = append(tlsParts, fmt.Sprintf(`"enabled":%v`, enabled))
				}

				if serverName, ok := tlsData["server_name"].(string); ok && serverName != "" {
					tlsParts = append(tlsParts, fmt.Sprintf(`"server_name":%s`, marshalJSONString(serverName)))
				}

				if alpn, ok := tlsData["alpn"].([]string); ok && len(alpn) > 0 {
					alpnJSON, _ := json.Marshal(alpn)
					tlsParts = append(tlsParts, fmt.Sprintf(`"alpn":%s`, string(alpnJSON)))
				}

				if utls, ok := tlsData["utls"].(map[string]interface{}); ok {
					var utlsParts []string
					if utlsEnabled, ok := utls["enabled"].(bool); ok {
						utlsParts = append(utlsParts, fmt.Sprintf(`"enabled":%v`, utlsEnabled))
					}
					// Emit fingerprint only when sing-box knows the name. An unknown value
					// (e.g. fp=HelloChrome_120 from a raw uTLS identifier) aborts config
					// load for every outbound; omitting the key lets sing-box pick its
					// default Chrome hello instead.
					if fingerprint, ok := utls["fingerprint"].(string); ok {
						if fingerprint = subscription.NormalizeUTLSFingerprint(fingerprint); fingerprint != "" {
							utlsParts = append(utlsParts, fmt.Sprintf(`"fingerprint":%s`, marshalJSONString(fingerprint)))
						}
					}
					utlsJSON := "{" + strings.Join(utlsParts, ",") + "}"
					tlsParts = append(tlsParts, fmt.Sprintf(`"utls":%s`, utlsJSON))
				}

				if insecure, ok := tlsData["insecure"].(bool); ok && insecure {
					tlsParts = append(tlsParts, fmt.Sprintf(`"insecure":%v`, insecure))
				}

				if reality, ok := tlsData["reality"].(map[string]interface{}); ok {
					var realityParts []string
					if realityEnabled, ok := reality["enabled"].(bool); ok {
						realityParts = append(realityParts, fmt.Sprintf(`"enabled":%v`, realityEnabled))
					}
					if publicKey, ok := reality["public_key"].(string); ok {
						realityParts = append(realityParts, fmt.Sprintf(`"public_key":%s`, marshalJSONString(publicKey)))
					}
					if shortID, ok := reality["short_id"].(string); ok {
						realityParts = append(realityParts, fmt.Sprintf(`"short_id":%s`, marshalJSONString(shortID)))
					}
					realityJSON := "{" + strings.Join(realityParts, ",") + "}"
					tlsParts = append(tlsParts, fmt.Sprintf(`"reality":%s`, realityJSON))
				}

				tlsJSON := "{" + strings.Join(tlsParts, ",") + "}"
				parts = append(parts, fmt.Sprintf(`"tls":%s`, tlsJSON))
			}
		}
	}

	// 8. detour (sing-box dial field; Xray dialerProxy chains)
	if node.Outbound != nil {
		if d, ok := node.Outbound["detour"].(string); ok {
			d = strings.TrimSpace(d)
			if d != "" {
				parts = append(parts, fmt.Sprintf(`"detour":%s`, marshalJSONString(d)))
			}
		}
	}

	// Build final JSON
	jsonStr := "{" + strings.Join(parts, ",") + "}"
	return fmt.Sprintf("\t// %s\n\t%s,", sanitizeOutboundLineComment(node.Label), jsonStr), nil
}

// GenerateSelectorWithFilteredAddOutbounds builds one selector/urltest outbound as a JSON string.
// Used in pass 3: only valid selectors are generated, and addOutbounds are filtered so that
// dynamic refs point only to selectors with isValid == true; constants (e.g. direct-out, auto-proxy-out) are always included.
// Nodes are filtered by outboundConfig.Filters (tag, host, scheme, etc.; literal and /regex/i). default is set from preferredDefault when specified.
// Returned string is one line (or comment + line), with trailing comma, ready to concatenate into the outbounds array.
func GenerateSelectorWithFilteredAddOutbounds(
	allNodes []*ParsedNode,
	outboundConfig Direction,
	outboundsInfo map[string]*outboundInfo,
	forGlobalOutbound bool,
	exposeCandidates []exposeTagCandidate,
) (string, error) {
	// Filter nodes based on filters (version 3)
	filterMap := outboundConfig.Filters
	debuglog.DebugLog("Parser: GenerateSelectorWithFilteredAddOutbounds for '%s' (type: %s): filters=%v, addOutbounds=%v, allNodes=%d",
		outboundConfig.Tag, outboundConfig.Type, filterMap, outboundConfig.AddOutbounds, len(allNodes))

	filteredNodes := filterNodesForSelector(allNodes, filterMap)
	// SPEC 110 T9: тот же запрет, что на проходе 1 — состав считается здесь
	// заново, и без повторной проверки цепочка вернулась бы в группу,
	// через которую сама и проходит.
	filteredNodes, _ = dropChainsThroughDirection(filteredNodes, outboundConfig.Tag, chainHopsByTag(allNodes))
	debuglog.DebugLog("Parser: filterNodesForSelector returned %d nodes for '%s'", len(filteredNodes), outboundConfig.Tag)

	// Build outbounds list with unique tags
	// Pre-allocate with estimated capacity to reduce allocations
	estimatedSize := len(outboundConfig.AddOutbounds) + len(filteredNodes)
	if forGlobalOutbound {
		estimatedSize += len(exposeCandidates)
	}
	outboundsList := make([]string, 0, estimatedSize)
	seenTags := make(map[string]bool, estimatedSize)
	duplicateCountInSelector := 0

	// Add addOutbounds first (version 3) - only valid dynamic ones + all constants
	addOutboundsList := outboundConfig.AddOutbounds
	if len(addOutboundsList) > 0 {
		debuglog.DebugLog("Parser: Processing %d addOutbounds for selector '%s'", len(addOutboundsList), outboundConfig.Tag)
		for _, tag := range addOutboundsList {
			if seenTags[tag] {
				duplicateCountInSelector++
				debuglog.DebugLog("Parser: Skipping duplicate tag '%s' in addOutbounds for selector '%s'", tag, outboundConfig.Tag)
				continue
			}

			if addInfo, exists := outboundsInfo[tag]; exists {
				// This is a dynamically created outbound - check if it's valid
				if addInfo.isValid {
					outboundsList = append(outboundsList, tag)
					seenTags[tag] = true
					debuglog.DebugLog("Parser: Adding valid dynamic addOutbound '%s' to selector '%s'", tag, outboundConfig.Tag)
				} else {
					debuglog.DebugLog("Parser: Skipping invalid (empty) dynamic addOutbound '%s' for selector '%s'", tag, outboundConfig.Tag)
				}
			} else {
				// This is a constant from template (direct-out, auto-proxy-out, etc.)
				// Constants always exist, always add them
				outboundsList = append(outboundsList, tag)
				seenTags[tag] = true
				debuglog.DebugLog("Parser: Adding constant addOutbound '%s' to selector '%s'", tag, outboundConfig.Tag)
			}
		}
	}

	// Add filtered node tags (without duplicates)
	debuglog.DebugLog("Parser: Processing %d filtered nodes for selector '%s'", len(filteredNodes), outboundConfig.Tag)
	for _, node := range filteredNodes {
		if !seenTags[node.Tag] {
			outboundsList = append(outboundsList, node.Tag)
			seenTags[node.Tag] = true
		} else {
			duplicateCountInSelector++
			debuglog.DebugLog("Parser: Skipping duplicate tag '%s' in filtered nodes for selector '%s'", node.Tag, outboundConfig.Tag)
		}
	}

	// SPEC 104: auto-группа Направления не принимает группы подписок (см.
	// addExposeTagEdges) — иначе urltest мерил бы выбор внутренней группы,
	// а не сервер.
	if forGlobalOutbound && len(exposeCandidates) > 0 && outboundConfig.TwinOf == "" {
		exposeSeen := make(map[string]struct{}, len(exposeCandidates))
		for _, c := range exposeCandidates {
			if _, dup := exposeSeen[c.Tag]; dup {
				continue
			}
			if !SelectorFiltersAcceptNode(outboundConfig.Filters, ExposeTagSyntheticNode(c.Tag, c.Comment)) {
				continue
			}
			if addInfo, exists := outboundsInfo[c.Tag]; exists && !addInfo.isValid {
				continue
			}
			exposeSeen[c.Tag] = struct{}{}
			if seenTags[c.Tag] {
				continue
			}
			outboundsList = append(outboundsList, c.Tag)
			seenTags[c.Tag] = true
			debuglog.DebugLog("Parser: Adding expose tag '%s' to global selector '%s'", c.Tag, outboundConfig.Tag)
		}
	}

	// Check if we have any outbounds at all (addOutbounds + filteredNodes + expose)
	if len(outboundsList) == 0 {
		debuglog.DebugLog("Parser: No outbounds (neither addOutbounds nor filteredNodes) for %s '%s'", outboundConfig.Type, outboundConfig.Tag)
		return "", nil
	}

	if duplicateCountInSelector > 0 {
		debuglog.DebugLog("Parser: Removed %d duplicate tags from selector '%s' outbounds list", duplicateCountInSelector, outboundConfig.Tag)
	}
	debuglog.DebugLog("Parser: Selector '%s' will have %d unique outbounds", outboundConfig.Tag, len(outboundsList))

	// Determine default - only if preferredDefault is specified in config (version 3)
	preferredDefaultMap := outboundConfig.PreferredDefault
	defaultTag := ""
	if len(preferredDefaultMap) > 0 {
		// Find first node matching preferredDefault filter
		preferredFilter := convertFilterToStringMap(preferredDefaultMap)
		for _, node := range filteredNodes {
			if matchesFilter(node, preferredFilter) {
				defaultTag = node.Tag
				break
			}
		}
	}
	// Note: We do NOT automatically set default to first node if preferredDefault is not specified
	// This allows urltest/selector to work without a default value when preferredDefault is not configured

	// SPEC 104: у Направления с автовыбором умолчанием становится его
	// auto-группа — ради неё её и включали. Только если пользовательский
	// preferredDefault ничего не поймал: явный выбор узла важнее.
	//
	// Обязательная проверка вхождения в outboundsList: sing-box отвергает
	// конфиг, где `default` не входит в `outbounds`, а двойник мог
	// отфильтроваться как пустой (проход 2).
	if defaultTag == "" && outboundConfig.TwinTag != "" {
		for _, t := range outboundsList {
			if t == outboundConfig.TwinTag {
				defaultTag = t
				break
			}
		}
	}

	// Build selector JSON with correct field order
	var parts []string

	// 1. tag
	parts = append(parts, fmt.Sprintf(`"tag":%s`, marshalJSONString(outboundConfig.Tag)))

	// 2. type
	parts = append(parts, fmt.Sprintf(`"type":%s`, marshalJSONString(outboundConfig.Type)))

	// 3. default (if present) - BEFORE outbounds
	if defaultTag != "" {
		parts = append(parts, fmt.Sprintf(`"default":%s`, marshalJSONString(defaultTag)))
	}

	// 4. outbounds
	outboundsJSON, _ := json.Marshal(outboundsList)
	parts = append(parts, fmt.Sprintf(`"outbounds":%s`, string(outboundsJSON)))

	// 5. interrupt_exist_connections (if present)
	if val, ok := outboundConfig.Options["interrupt_exist_connections"]; ok {
		if boolVal, ok := val.(bool); ok {
			parts = append(parts, fmt.Sprintf(`"interrupt_exist_connections":%v`, boolVal))
		} else {
			valJSON, _ := json.Marshal(val)
			parts = append(parts, fmt.Sprintf(`"interrupt_exist_connections":%s`, string(valJSON)))
		}
	}

	// 6. Other options — emitted in sorted key order for a deterministic config.
	// A map range is unordered, which makes byte-exact golden fixtures flaky
	// (e.g. urltest mode/balancer pair swapping position); sorting fixes that
	// and is harmless for sing-box (object key order is not significant).
	sanitizeBalancerOptions(outboundConfig.Options)
	optKeys := make([]string, 0, len(outboundConfig.Options))
	for key := range outboundConfig.Options {
		if key != "interrupt_exist_connections" {
			optKeys = append(optKeys, key)
		}
	}
	sort.Strings(optKeys)
	for _, key := range optKeys {
		valJSON, _ := json.Marshal(outboundConfig.Options[key])
		parts = append(parts, fmt.Sprintf(`%s:%s`, marshalJSONString(key), string(valJSON)))
	}

	// Build final JSON
	jsonStr := "{" + strings.Join(parts, ",") + "}"

	// Строчная подпись над группой в config.json — для того, кто читает
	// конфиг глазами.
	//
	// SPEC 104: имя Направления важнее комментария. Пользователь вводит
	// одно поле «Имя», и подпись в конфиге обязана совпадать с тем, что он
	// видит в списке; комментарий шаблона остаётся запасным вариантом для
	// записей, которым имя не задавали.
	note := outboundConfig.Label
	if note == "" {
		note = outboundConfig.Comment
	}
	result := ""
	if note != "" {
		result = fmt.Sprintf("\t// %s\n", sanitizeOutboundLineComment(note))
	}
	result += fmt.Sprintf("\t%s,", jsonStr)

	return result, nil
}

// sanitizeBalancerOptions enforces the sing-box-lx urltest load-balancing
// sentinel contract before emit (SPEC 088 / core SPEC 019). The core treats a
// balancer.sticky_hash of length 0 as "omitted" → default stickiness
// (["process","domain"]), NOT as "off". Stickiness is disabled only by the
// explicit sentinel ["none"]. So a bare empty sticky_hash ([]) in the options
// is meaningless and misleading: drop it, letting the core apply its default.
// A malformed balancer (not a map) is left untouched — sing-box check will
// reject it before RebuildConfigIfDirty writes the config (fail-closed).
func sanitizeBalancerOptions(opts map[string]interface{}) {
	if opts == nil {
		return
	}
	balancer, ok := opts["balancer"].(map[string]interface{})
	if !ok {
		return
	}
	sh, ok := balancer["sticky_hash"]
	if !ok {
		return
	}
	empty := false
	switch v := sh.(type) {
	case []interface{}:
		empty = len(v) == 0
	case []string:
		empty = len(v) == 0
	case nil:
		empty = true
	}
	if empty {
		delete(balancer, "sticky_hash")
	}
}

// GenerateEndpointJSON returns a single JSON object string for one WireGuard endpoint (sing-box endpoints array).
// node.Outbound must contain the full endpoint map built by the wireguard URI parser.
// Uses node.Tag (with tag_prefix applied by source) for the endpoint "tag" so selectors can reference it.
// Returned string is pretty-printed (multi-line); trailing comma is added by the caller when inserting into the array.
func GenerateEndpointJSON(node *ParsedNode) (string, error) {
	if node.Scheme != "wireguard" || node.Outbound == nil {
		return "", fmt.Errorf("GenerateEndpointJSON requires wireguard node with Outbound set")
	}
	// Use node.Tag (includes tag_prefix, e.g. "4:wg-parnas") so endpoint tag matches outbound references
	endpoint := make(map[string]interface{})
	for k, v := range node.Outbound {
		endpoint[k] = v
	}
	if node.Tag != "" {
		endpoint["tag"] = node.Tag
	}
	jsonBytes, err := json.MarshalIndent(endpoint, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal wireguard endpoint: %w", err)
	}
	result := ""
	if node.Comment != "" {
		result = "// " + sanitizeOutboundLineComment(node.Comment) + "\n"
	}
	result += string(jsonBytes)
	return result, nil
}

// EmitNodeJSONs renders one parsed node exactly as the final config carries
// it: wireguard → a single endpoints entry, everything else → one or more
// outbounds entries (SPEC 094 B3 detour-chain hops first, then the main
// outbound with "detour" stamped onto a copy of its map).
//
// Shared by the generation pipeline and the Source edit window's JSON tab —
// one emission point, so what the tab shows is byte-for-byte what the build
// puts into config.json.
func EmitNodeJSONs(node *ParsedNode) (outboundJSONs []string, endpointJSON string, err error) {
	if node == nil {
		return nil, "", fmt.Errorf("nil node")
	}

	if node.Scheme == "wireguard" {
		ep, err := GenerateEndpointJSON(node)
		if err != nil {
			return nil, "", err
		}
		return nil, ep, nil
	}

	// Chain — источник правды; Jump продолжает работать для узлов из
	// Xray-пути и из state.json, написанного до SPEC 094.
	chain := chainOfNode(node)

	var jsons []string
	for _, hop := range chain {
		hopJSON, hopErr := GenerateNodeJSON(hop)
		if hopErr != nil {
			return nil, "", fmt.Errorf("chain hop %s: %w", hop.Tag, hopErr)
		}
		jsons = append(jsons, hopJSON)
	}

	origOutbound := node.Outbound
	if len(chain) > 0 {
		cp := make(map[string]interface{}, len(node.Outbound)+1)
		for k, v := range node.Outbound {
			cp[k] = v
		}
		cp["detour"] = chain[0].Tag
		node.Outbound = cp
	}
	mainJSON, err := GenerateNodeJSON(node)
	if len(chain) > 0 {
		node.Outbound = origOutbound
	}
	if err != nil {
		return nil, "", err
	}
	return append(jsons, mainJSON), "", nil
}

// GenerateOutboundsFromParserConfig is the main entry point: loads nodes from all proxy sources via loadNodesFunc,
// generates node JSONs, then runs the three passes (buildOutboundsInfo, computeOutboundValidity, generateSelectorJSONs)
// and returns the concatenated JSON strings (nodes, then local selectors, then global selectors) plus counts.
// progressCallback(0–100, message) is optional for UI progress. tagCounts is passed to loadNodesFunc for deduplication.
func GenerateOutboundsFromParserConfig(
	parserConfig *ParserConfig,
	tagCounts map[string]int,
	progressCallback func(float64, string),
	loadNodesFunc func(ProxySource, map[string]int, func(float64, string), int, int) ([]*ParsedNode, error),
	directions DirectionBuildOptions,
) (*OutboundGenerationResult, error) {
	// SPEC 104, проход 0 — раскрываем Направления: выключенные выпадают,
	// у остальных разворачиваются парные auto-группы. Делается ДО
	// подстановки переменных, чтобы `@urltest_*` в опциях двойника
	// подставились штатно, без отдельной ветки для них.
	PrepareDirections(parserConfig, directions.TwinOptions)

	// SPEC 108, тот же проход 0 — разворачиваем свёрнутые подписки в
	// локальные группы. После Направлений и до подстановки переменных: у
	// автогруппы подписки те же `@urltest_*` в опциях.
	PrepareSourceFolds(parserConfig, directions.TwinOptions)

	// Hotfix v0.8.8.1 — substitute `@varname` placeholders in
	// parser_config.outbounds[].options before generating selector JSONs. See
	// varsubst.go for the rationale; nil substituter falls back to v0.8.6
	// hard-coded defaults for the three known URLTest placeholders.
	SubstituteParserConfigPlaceholders(parserConfig, nil)

	// Step 1: Process all proxy sources and collect nodes
	allNodes := make([]*ParsedNode, 0)
	nodesBySource := make(map[int][]*ParsedNode) // Map source index to its nodes

	// Count only enabled sources as "total" for progress + summary purposes.
	// Disabled sources are skipped entirely (no fetch, no parse) and don't
	// participate in the success/failure counts — they're not something the
	// user is currently trying to get nodes from.
	totalSources := 0
	for _, p := range parserConfig.ParserConfig.Proxies {
		if !p.Disabled {
			totalSources++
		}
	}
	if progressCallback != nil {
		progressCallback(10, fmt.Sprintf("Processing %d sources...", totalSources))
	}

	// SPEC 044 feature-probe: one probe per generation run. When the core
	// can't create naive outbounds, drop those nodes (with a warning) instead
	// of emitting a config that `sing-box check` rejects wholesale.
	naiveSupported, naiveReason := true, ""
	if NaiveSupportProbe != nil {
		naiveSupported, naiveReason = NaiveSupportProbe()
	}
	skippedNaive := 0

	processedIdx := 0
	succeededSources := 0
	failedSources := 0
	for i, proxySource := range parserConfig.ParserConfig.Proxies {
		if proxySource.Disabled {
			debuglog.DebugLog("GenerateOutboundsFromParserConfig: skipping source %d (disabled)", i+1)
			continue
		}
		if progressCallback != nil && totalSources > 0 {
			progressCallback(10+float64(processedIdx)*30.0/float64(totalSources),
				fmt.Sprintf("Processing source %d/%d...", processedIdx+1, totalSources))
		}
		processedIdx++

		// SPEC 094 A5: группы из импортированного sing-box конфига приходят
		// В ЭТОМ ЖЕ списке как обычные узлы (SchemeGroup). Отдельного канала
		// для них нет — вкладка Outbounds остаётся за каналами лаунчера.
		nodesFromSource, err := loadNodesFunc(proxySource, tagCounts, progressCallback, i, totalSources)
		if err != nil {
			debuglog.ErrorLog("GenerateOutboundsFromParserConfig: Error processing source %d/%d: %v", i+1, totalSources, err)
			failedSources++
			continue
		}

		skippedNaiveHere := 0
		if !naiveSupported {
			kept := nodesFromSource[:0]
			for _, n := range nodesFromSource {
				if n.Scheme == "naive" {
					skippedNaiveHere++
					debuglog.WarnLog("GenerateOutboundsFromParserConfig: skipping naive node %q — %s", n.Tag, naiveReason)
					continue
				}
				kept = append(kept, n)
			}
			nodesFromSource = kept
			skippedNaive += skippedNaiveHere
		}

		// A source whose every node was a degraded naive node still fetched
		// and parsed fine — count it as succeeded, not silent-empty.
		if len(nodesFromSource) == 0 && skippedNaiveHere > 0 {
			succeededSources++
			continue
		}

		if len(nodesFromSource) > 0 {
			for _, n := range nodesFromSource {
				n.SourceIndex = i
			}
			allNodes = append(allNodes, nodesFromSource...)
			nodesBySource[i] = nodesFromSource
			succeededSources++
		} else {
			// Silent-empty: source fetched OK but parsed zero nodes. From
			// the user's perspective this is indistinguishable from a hard
			// failure — they expected nodes, got none.
			debuglog.WarnLog("GenerateOutboundsFromParserConfig: source %d/%d returned zero nodes (counted as failed)", i+1, totalSources)
			failedSources++
		}
	}

	if len(allNodes) == 0 {
		if totalSources == 0 {
			return nil, fmt.Errorf("no enabled sources (all subscriptions disabled in wizard)")
		}
		if skippedNaive > 0 {
			return nil, fmt.Errorf("no usable nodes: %d naive node(s) skipped (%s)", skippedNaive, naiveReason)
		}
		return nil, fmt.Errorf("no nodes parsed from any source")
	}

	// SPEC 110: источники-цепочки становятся узлами здесь — их позиции
	// ссылаются на теги, окончательные только после загрузки ВСЕХ
	// источников (префиксы подписок, уникализация дублей). Направления
	// передаются отдельно: они разворачиваются позже, но их теги известны
	// уже сейчас.
	directionTagsForChains := make(map[string]bool, len(parserConfig.ParserConfig.Outbounds))
	for _, d := range parserConfig.ParserConfig.Outbounds {
		if d.Tag != "" && !d.Disabled {
			directionTagsForChains[d.Tag] = true
		}
	}
	allNodes, brokenChains := ResolveChainSources(parserConfig, allNodes, nodesBySource, directionTagsForChains)

	// SPEC 101: resolve hash-addressed source detours (chain through one concrete
	// node) now that every source is loaded and all tags are final. Must run
	// before sanitizeNodeDetours so the stamped tags join cycle/self validation.
	allNodes = resolveNodeHashDetours(parserConfig, nodesBySource, allNodes)
	if len(allNodes) == 0 {
		return nil, fmt.Errorf("no usable nodes: every source was dropped (detour node not found)")
	}

	// SPEC 077: sanitize source-level detours that would break sing-box —
	// self-reference and detour cycles among nodes. Fail-open: the offending
	// detour field is dropped (the node dials directly), generation continues.
	// Dangling detours onto template/preset group tags are NOT pruned here:
	// those tags are merged in only at final config assembly, so they are
	// unknown at this point and pruning them would be a false positive.
	sanitizeNodeDetours(allNodes)

	// Step 2: Generate JSON for all nodes
	if progressCallback != nil {
		progressCallback(40, fmt.Sprintf("Generating JSON for %d nodes...", len(allNodes)))
	}

	selectorsJSON := make([]string, 0)
	endpointsJSON := make([]string, 0)
	nodesCount := 0
	endpointsCount := 0

	for _, node := range allNodes {
		outJSONs, epJSON, err := EmitNodeJSONs(node)
		if err != nil {
			debuglog.WarnLog("GenerateOutboundsFromParserConfig: Failed to generate JSON for node %s: %v", node.Tag, err)
			continue
		}
		if epJSON != "" {
			endpointsJSON = append(endpointsJSON, epJSON)
			endpointsCount++
		} else {
			selectorsJSON = append(selectorsJSON, outJSONs...)
			nodesCount++
		}
	}

	globalPool := FilterNodesExcludeFromGlobal(allNodes, parserConfig.ParserConfig.Proxies)
	exposeCandidates := collectExposeTagCandidates(parserConfig)
	outboundsInfo, chainCycles := buildOutboundsInfo(parserConfig, nodesBySource, globalPool, progressCallback)
	computeOutboundValidity(outboundsInfo, parserConfig, exposeCandidates, progressCallback)
	selectorJSONs, localSelectorsCount, globalSelectorsCount, emptyDirections := generateSelectorJSONs(
		parserConfig, nodesBySource, globalPool, outboundsInfo, exposeCandidates, progressCallback, directions)
	selectorsJSON = append(selectorsJSON, selectorJSONs...)

	return &OutboundGenerationResult{
		OutboundsJSON:        selectorsJSON,
		EndpointsJSON:        endpointsJSON,
		NodesCount:           nodesCount,
		EndpointsCount:       endpointsCount,
		LocalSelectorsCount:  localSelectorsCount,
		GlobalSelectorsCount: globalSelectorsCount,
		TotalSources:         totalSources,
		SucceededSources:     succeededSources,
		FailedSources:        failedSources,
		SkippedNaiveNodes:    skippedNaive,
		EmptyDirections:      emptyDirections,
		BrokenChains:         brokenChains,
		ChainCycles:          chainCycles,
		SkippedNaiveReason:   naiveReason,
	}, nil
}

// sanitizeNodeDetours removes source-level detour fields that would break
// sing-box (SPEC 077), fail-open: the field is dropped and the node dials
// directly. Two cases are handled, both decidable from the node set alone:
//
//   - self-reference: node.detour == node.tag;
//   - cycle among nodes: following node.detour from node to node returns to an
//     already-visited node (A→B→A, longer rings, …). The detour on the node
//     that closes the ring is dropped, which breaks every cycle it belongs to.
//
// Detours pointing at tags outside the node set (template/preset group tags,
// built-in outbounds) are left untouched — those are resolved only at final
// config assembly and are not knowable here.
// generateGroupNodeJSON emits a selector/urltest node imported from a sing-box
// config (SPEC 094 A5).
//
// Unlike every other scheme this node has no server/server_port; its payload is
// the member list plus the group's own options (url/interval/tolerance/…),
// already validated at parse time.
//
// Emitting an empty group is not allowed: sing-box refuses to start on an
// urltest with no outbounds, so a group that lost all its members must have
// been dropped earlier. Returning an error here is the last line of defence.
func generateGroupNodeJSON(node *ParsedNode) (string, error) {
	if node.Outbound == nil {
		return "", fmt.Errorf("group node %q has no outbound data", node.Tag)
	}

	groupType, _ := node.Outbound["type"].(string)
	groupType = strings.TrimSpace(groupType)
	if groupType == "" {
		return "", fmt.Errorf("group node %q has no type", node.Tag)
	}

	members := groupMemberTags(node)
	if len(members) == 0 {
		return "", fmt.Errorf("group node %q has no members", node.Tag)
	}

	var parts []string
	parts = append(parts, fmt.Sprintf(`"tag":%s`, marshalJSONString(node.Tag)))
	parts = append(parts, fmt.Sprintf(`"type":%s`, marshalJSONString(groupType)))

	quoted := make([]string, 0, len(members))
	for _, m := range members {
		quoted = append(quoted, marshalJSONString(m))
	}
	parts = append(parts, fmt.Sprintf(`"outbounds":[%s]`, strings.Join(quoted, ",")))

	// Остальные поля группы эмитятся в отсортированном порядке: детерминизм
	// нужен и golden-тестам, и хешу идентичности узла.
	extraKeys := make([]string, 0, len(node.Outbound))
	for k := range node.Outbound {
		switch k {
		case "tag", "type", GroupMembersKey:
			continue
		}
		extraKeys = append(extraKeys, k)
	}
	sort.Strings(extraKeys)

	for _, k := range extraKeys {
		encoded, err := json.Marshal(node.Outbound[k])
		if err != nil {
			debuglog.WarnLog("GenerateNodeJSON: group %q: dropping unencodable field %q: %v", node.Tag, k, err)
			continue
		}
		parts = append(parts, fmt.Sprintf(`%s:%s`, marshalJSONString(k), string(encoded)))
	}

	body := fmt.Sprintf("\t{%s},", strings.Join(parts, ","))
	if comment := sanitizeOutboundLineComment(node.Label); comment != "" {
		return fmt.Sprintf("\t// %s\n%s", comment, body), nil
	}
	return body, nil
}

// generateRawNodeJSON emits a manual config_json node (ParsedNode.EmitRaw):
// the Outbound map goes into the config verbatim, except tag (restamped with
// the node's final tag — label/mask/uniquify already applied) and detour
// (stamped by subscription.ApplySourceDetour / the chain logic into the map itself).
//
// tag and type lead for readability, the rest is sorted: determinism is
// needed by the node identity hash, which is computed from the emitted JSON.
func generateRawNodeJSON(node *ParsedNode) (string, error) {
	if node.Outbound == nil {
		return "", fmt.Errorf("manual node %q has no outbound data", node.Tag)
	}

	typ, _ := node.Outbound["type"].(string)
	typ = strings.TrimSpace(typ)
	if typ == "" {
		return "", fmt.Errorf("manual node %q has no type", node.Tag)
	}

	var parts []string
	parts = append(parts, fmt.Sprintf(`"tag":%s`, marshalJSONString(node.Tag)))
	parts = append(parts, fmt.Sprintf(`"type":%s`, marshalJSONString(typ)))

	extraKeys := make([]string, 0, len(node.Outbound))
	for k := range node.Outbound {
		if k == "tag" || k == "type" {
			continue
		}
		extraKeys = append(extraKeys, k)
	}
	sort.Strings(extraKeys)

	for _, k := range extraKeys {
		encoded, err := json.Marshal(node.Outbound[k])
		if err != nil {
			debuglog.WarnLog("GenerateNodeJSON: manual node %q: dropping unencodable field %q: %v", node.Tag, k, err)
			continue
		}
		parts = append(parts, fmt.Sprintf(`%s:%s`, marshalJSONString(k), string(encoded)))
	}

	body := fmt.Sprintf("\t{%s},", strings.Join(parts, ","))
	if comment := sanitizeOutboundLineComment(node.Label); comment != "" {
		return fmt.Sprintf("\t// %s\n%s", comment, body), nil
	}
	return body, nil
}

// groupMemberTags returns the group's member tags in order.
func groupMemberTags(node *ParsedNode) []string {
	if node == nil || node.Outbound == nil {
		return nil
	}
	raw, ok := node.Outbound[GroupMembersKey].([]interface{})
	if !ok {
		// Уже нормализованный срез строк — форма после round-trip через state.
		if strs, ok := node.Outbound[GroupMembersKey].([]string); ok {
			return strs
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// chainOfNode returns the node's detour chain, nearest hop first (SPEC 094 B3).
//
// Chain is authoritative. When it is empty the legacy single-hop Jump is
// promoted to a one-element chain, so nodes produced by the Xray dialerProxy
// path — and state.json files written before SPEC 094 — keep chaining exactly
// as before.
//
// Each hop is returned as a ready-to-emit ParsedNode; hops carry their own
// detour to the next one, stamped at parse time.
func chainOfNode(node *ParsedNode) []*ParsedNode {
	if node == nil {
		return nil
	}

	if len(node.Chain) > 0 {
		out := make([]*ParsedNode, 0, len(node.Chain))
		for _, hop := range node.Chain {
			if hop == nil {
				continue
			}
			out = append(out, normalizeChainHop(hop, node))
		}
		return out
	}

	if node.Jump == nil {
		return nil
	}
	return []*ParsedNode{normalizeChainHop(&ParsedNode{
		Tag:      node.Jump.Tag,
		Scheme:   node.Jump.Scheme,
		Server:   node.Jump.Server,
		Port:     node.Jump.Port,
		UUID:     node.Jump.UUID,
		Flow:     node.Jump.Flow,
		Outbound: node.Jump.Outbound,
	}, node)}
}

// normalizeChainHop fills in the defaults a hop needs to emit cleanly.
//
// An empty scheme means SOCKS for backward compatibility (ParsedJump documented
// it that way), and a SOCKS hop without an explicit version must default to 5 —
// sing-box rejects the outbound otherwise.
func normalizeChainHop(hop, owner *ParsedNode) *ParsedNode {
	out := &ParsedNode{
		Tag:      hop.Tag,
		Scheme:   hop.Scheme,
		Server:   hop.Server,
		Port:     hop.Port,
		UUID:     hop.UUID,
		Flow:     hop.Flow,
		Outbound: hop.Outbound,
		Label:    owner.Label,
		Comment:  owner.Comment,
	}
	if out.Scheme == "" {
		out.Scheme = "socks"
	}
	if out.Outbound == nil {
		out.Outbound = map[string]interface{}{}
	}
	if out.Scheme == "socks" {
		if _, ok := out.Outbound["version"]; !ok {
			out.Outbound["version"] = "5"
		}
	}
	return out
}

// resolveNodeHashDetours implements SPEC 101 — a source chained through one
// concrete node addressed by identity hash (ProxySource.DetourNodeHash), the
// node-level sibling of the tag-addressed group detour (SPEC 077).
//
// Runs after all sources are loaded: only here the target's final tag is known
// (prefix/mask/uniquify applied) and the full node set is available for lookup.
// Group nodes are not lookup candidates — chaining through a selector is what
// DetourTag does; the hash path is strictly node-to-node.
//
// Fail-closed: when the hash resolves to nothing (node gone from the
// subscription, credentials changed → new hash, or the node was disabled),
// the dependent source's nodes are DROPPED from the config with a warning.
// The tag-detour path fails open (drop the detour, dial direct), which is
// right for a group that lost a member — but a missing chain hop must not
// silently turn into direct dialing: the user picked the hop to hide this
// source's traffic behind it.
//
// When both DetourNodeHash and DetourTag are set (UI forbids it, hand-edited
// state can), the hash wins: LoadNodesFromSource skips tag stamping for such
// sources and the stamp happens here.
func resolveNodeHashDetours(parserConfig *ParserConfig, nodesBySource map[int][]*ParsedNode, allNodes []*ParsedNode) []*ParsedNode {
	var hashSources []int
	for i, ps := range parserConfig.ParserConfig.Proxies {
		if strings.TrimSpace(ps.DetourNodeHash) != "" && len(nodesBySource[i]) > 0 {
			hashSources = append(hashSources, i)
		}
	}
	if len(hashSources) == 0 {
		return allNodes
	}

	byHash := make(map[string]*ParsedNode)
	for _, n := range allNodes {
		if n == nil || n.Scheme == SchemeGroup || n.Tag == "" {
			continue
		}
		if h := NodeIdentityHash(n); h != "" {
			if _, dup := byHash[h]; !dup {
				byHash[h] = n
			}
		}
	}

	dropSource := make(map[int]bool)
	for _, i := range hashSources {
		ps := parserConfig.ParserConfig.Proxies[i]
		hash := strings.TrimSpace(ps.DetourNodeHash)
		target := byHash[hash]
		if target == nil {
			label := ps.DetourNodeLabel
			if label == "" {
				// Без builtin min: win7-сборка идёт тулчейном go1.20 (go.win7.mod).
				short := hash
				if len(short) > 12 {
					short = short[:12]
				}
				label = short + "…"
			}
			debuglog.WarnLog("Parser: detour node %q for source %d not found — dropping its %d node(s) (fail-closed, traffic must not go direct)",
				label, i+1, len(nodesBySource[i]))
			dropSource[i] = true
			continue
		}
		for _, n := range nodesBySource[i] {
			if n == nil || n == target {
				continue // the hop itself dials direct
			}
			// Same eligibility as subscription.ApplySourceDetour (SPEC 077).
			if n.Jump != nil {
				debuglog.DebugLog("resolveNodeHashDetours: node %q has an Xray Jump — detour %q not applied", n.Tag, target.Tag)
				continue
			}
			if _, hasLP := n.Outbound["listen_port"]; hasLP && n.Scheme == "wireguard" {
				debuglog.WarnLog("Parser: node %q has listen_port — detour %q not applied (core rejects detour+listen_port)", n.Tag, target.Tag)
				continue
			}
			if n.Outbound == nil {
				n.Outbound = map[string]interface{}{}
			}
			n.Outbound["detour"] = target.Tag
		}
	}

	if len(dropSource) == 0 {
		return allNodes
	}
	kept := allNodes[:0]
	for _, n := range allNodes {
		if n != nil && dropSource[n.SourceIndex] {
			continue
		}
		kept = append(kept, n)
	}
	for i := range dropSource {
		delete(nodesBySource, i)
	}
	return kept
}

func sanitizeNodeDetours(nodes []*ParsedNode) {
	// detourOf[tag] = the detour target of the node with that tag, but only
	// when the target is itself a node (intra-node edge). Also drop self-refs.
	nodeByTag := make(map[string]*ParsedNode, len(nodes))
	for _, n := range nodes {
		if n != nil && n.Tag != "" {
			nodeByTag[n.Tag] = n
		}
	}

	detourOf := make(map[string]string)
	for _, n := range nodes {
		if n == nil || n.Outbound == nil {
			continue
		}
		d, _ := n.Outbound["detour"].(string)
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if d == n.Tag {
			debuglog.WarnLog("Parser: node %q detour points at itself — dropping detour (direct dial)", n.Tag)
			delete(n.Outbound, "detour")
			continue
		}
		if _, isNode := nodeByTag[d]; isNode {
			detourOf[n.Tag] = d // only intra-node edges can form a cycle we can see
		}
	}

	// Cycle detection over intra-node detour edges. Classic DFS colouring:
	// walking the unique out-edge of each tag, a back-edge to a node already on
	// the current path is a cycle — drop the closing edge's detour.
	const (
		white = 0 // unvisited
		gray  = 1 // on current path
		black = 2 // fully explored, acyclic
	)
	color := make(map[string]int, len(detourOf))
	for start := range detourOf {
		if color[start] != white {
			continue
		}
		// iterative walk of the single out-edge chain
		path := []string{}
		cur := start
		for {
			if color[cur] == gray {
				// back-edge: cur is on the current path → break the ring here
				if n := nodeByTag[cur]; n != nil && n.Outbound != nil {
					debuglog.WarnLog("Parser: detour cycle through node %q — dropping its detour to break the chain", cur)
					delete(n.Outbound, "detour")
				}
				delete(detourOf, cur)
				break
			}
			if color[cur] == black {
				break // joins an already-cleared, acyclic chain
			}
			color[cur] = gray
			path = append(path, cur)
			next, ok := detourOf[cur]
			if !ok {
				break // chain ends at a non-node target or a node without detour
			}
			cur = next
		}
		for _, t := range path {
			color[t] = black
		}
	}
}

// obfsIntField reads an integer obfs field regardless of whether it arrived as
// an int (URI parser) or a float64 (JSON import). Returns 0 when absent.
func obfsIntField(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	}
	return 0
}
