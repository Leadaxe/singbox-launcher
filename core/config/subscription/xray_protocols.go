package subscription

import (
	"errors"
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 094 фаза C — конвертеры Xray-протоколов помимо VLESS.
//
// До SPEC 094 Xray-массив разбирал ТОЛЬКО vless и брал один узел на элемент:
// провайдер, отдающий vmess/trojan/shadowsocks/hysteria2, терял их целиком.
//
// Все конвертеры возвращают ParsedNode с уже заполненной Outbound-map в
// sing-box форме. TLS и транспорт берутся из общих
// xrayVLESSTLSFromStreamSettings / xrayTransportFromStreamSettings: они не
// протокол-специфичны, несмотря на историческое имя первого.

// Причины отбраковки элемента — тексты для ЧЕЛОВЕКА, а не для лога.
//
// Раньше здесь стояли лаконичные «missing user id» / «missing vnext»: они
// точно называют отсутствующее поле и ничего не говорят о том, что пошло не
// так у пользователя. Самый частый живой случай — сервер отдал заглушку вместо
// креденшла (подписка протухла); формулировка обязана вести к этому выводу, а
// не к чтению JSON-схемы Xray.
//
// Английские строки: причина уезжает и в лог, и в отчёт сборки, и в отчёт её
// переводит слой UI (locale) — парсер про локаль не знает.
const (
	xrayReasonEmptyUserID = "empty user id — the server returned a placeholder, subscription may be expired"
	xrayReasonNoVNext     = "no server section (settings.vnext) — the element carries no endpoint"
	xrayReasonNoServers   = "no server section (settings.servers) — the element carries no endpoint"
	xrayReasonNoSettings  = "no settings section — the element carries no endpoint"
	xrayReasonNoAddress   = "empty server address"
	xrayReasonBadPort     = "server port is missing or out of range"
	xrayReasonNoPassword  = "empty password — the server returned a placeholder, subscription may be expired"
)

// xrayServiceProtocols — служебные Xray-протоколы, не являющиеся узлами.
// Собственный набор, не пересекающийся с sing-box (direct/block/dns).
var xrayServiceProtocols = map[string]struct{}{
	"freedom": {}, "blackhole": {}, "dns": {}, "loopback": {},
}

// IsXrayServiceProtocol сообщает, служебный ли это протокол.
func IsXrayServiceProtocol(p string) bool {
	_, ok := xrayServiceProtocols[strings.ToLower(strings.TrimSpace(p))]
	return ok
}

// xrayUnsupportedProtocolError — «этот протокол лаунчер не умеет».
//
// Отдельный тип, а не текст ошибки, потому что вызывающий обязан РАЗЛИЧАТЬ два
// класса отбраковки, и различать их подстрокой значило бы поставить диагностику
// в зависимость от формулировки. Класс «протокол не поддержан» — свойство
// лаунчера, чинить его пользователю нечем; класс «поддерживаемый протокол,
// битый элемент» (пустой id, нет vnext, кривой порт) — свойство ПОДПИСКИ, и
// пользователь обязан увидеть настоящую причину. До этого разделения любая
// ошибка конверсии превращалась в «unsupported protocol "vless" skipped» —
// сообщение, которое врало и уводило от протухшей подписки.
type xrayUnsupportedProtocolError struct {
	Protocol string
}

func (e *xrayUnsupportedProtocolError) Error() string {
	return fmt.Sprintf("unsupported protocol %q", e.Protocol)
}

// xrayUnsupportedProtocol возвращает имя протокола, если ошибка именно этого
// класса; иначе пусто.
func xrayUnsupportedProtocol(err error) (string, bool) {
	var e *xrayUnsupportedProtocolError
	if errors.As(err, &e) {
		return e.Protocol, true
	}
	return "", false
}

// xrayNodeFromOutbound конвертирует любой поддерживаемый Xray outbound в узел.
//
// Возвращает (nil, nil) для служебных протоколов — это не ошибка, они просто
// не узлы. Для неподдерживаемых возвращает *xrayUnsupportedProtocolError, чтобы
// вызывающий записал протокол в список пропущенных, а не потерял его молча (C1);
// для поддерживаемого протокола с битым содержимым — обычную ошибку с настоящей
// причиной.
func xrayNodeFromOutbound(ob map[string]interface{}, label string) (*configtypes.ParsedNode, error) {
	protocol := strings.ToLower(strings.TrimSpace(xrayMapString(ob, "protocol")))
	if protocol == "" {
		return nil, fmt.Errorf("missing protocol")
	}
	if IsXrayServiceProtocol(protocol) {
		return nil, nil
	}

	switch protocol {
	case "vless":
		return xrayBuildVLESSFromOutbound(ob, label)
	case "vmess":
		return xrayBuildVMessFromOutbound(ob, label)
	case "trojan":
		return xrayBuildTrojanFromOutbound(ob, label)
	case "shadowsocks":
		return xrayBuildShadowsocksFromOutbound(ob, label)
	case "hysteria":
		return xrayBuildHysteriaFromOutbound(ob, label)
	case "hysteria2":
		return xrayBuildHysteria2FromOutbound(ob, label)
	default:
		return nil, &xrayUnsupportedProtocolError{Protocol: protocol}
	}
}

// xrayVNextEndpoint достаёт address/port/users[0] — общая форма vmess и vless.
func xrayVNextEndpoint(ob map[string]interface{}) (addr string, port int, user map[string]interface{}, err error) {
	settings, _ := ob["settings"].(map[string]interface{})
	if settings == nil {
		return "", 0, nil, errors.New(xrayReasonNoSettings)
	}
	vnextRaw, ok := settings["vnext"].([]interface{})
	if !ok || len(vnextRaw) == 0 {
		return "", 0, nil, errors.New(xrayReasonNoVNext)
	}
	vn0, ok := vnextRaw[0].(map[string]interface{})
	if !ok {
		return "", 0, nil, errors.New(xrayReasonNoVNext)
	}
	addr = xrayMapString(vn0, "address")
	if addr == "" {
		return "", 0, nil, errors.New(xrayReasonNoAddress)
	}
	port = xrayJSONInt(vn0["port"])
	if port <= 0 || port > 65535 {
		return "", 0, nil, errors.New(xrayReasonBadPort)
	}
	users, _ := vn0["users"].([]interface{})
	if len(users) == 0 {
		return "", 0, nil, errors.New("no users in the server section — the element carries no credentials")
	}
	u0, ok := users[0].(map[string]interface{})
	if !ok {
		return "", 0, nil, errors.New("malformed user record in the server section")
	}
	return addr, port, u0, nil
}

// xrayServerEndpoint достаёт address/port/первый server — форма trojan,
// shadowsocks и hysteria2 (settings.servers[]).
func xrayServerEndpoint(ob map[string]interface{}) (addr string, port int, server map[string]interface{}, err error) {
	settings, _ := ob["settings"].(map[string]interface{})
	if settings == nil {
		return "", 0, nil, errors.New(xrayReasonNoSettings)
	}
	serversRaw, ok := settings["servers"].([]interface{})
	if !ok || len(serversRaw) == 0 {
		return "", 0, nil, errors.New(xrayReasonNoServers)
	}
	s0, ok := serversRaw[0].(map[string]interface{})
	if !ok {
		return "", 0, nil, errors.New(xrayReasonNoServers)
	}
	addr = xrayMapString(s0, "address")
	if addr == "" {
		return "", 0, nil, errors.New(xrayReasonNoAddress)
	}
	port = xrayJSONInt(s0["port"])
	if port <= 0 || port > 65535 {
		return "", 0, nil, errors.New(xrayReasonBadPort)
	}
	return addr, port, s0, nil
}

// xrayStreamParts возвращает network/security/tls/transport одним вызовом.
func xrayStreamParts(ob map[string]interface{}) (tls, transport map[string]interface{}) {
	streamSettings, _ := ob["streamSettings"].(map[string]interface{})
	network := strings.ToLower(xrayMapString(streamSettings, "network"))
	if network == "" {
		network = "tcp"
	}
	security := strings.ToLower(xrayMapString(streamSettings, "security"))
	return xrayVLESSTLSFromStreamSettings(streamSettings, security),
		xrayTransportFromStreamSettings(streamSettings, network)
}

// xrayBuildVMessFromOutbound — vmess через settings.vnext.
func xrayBuildVMessFromOutbound(ob map[string]interface{}, label string) (*configtypes.ParsedNode, error) {
	addr, port, user, err := xrayVNextEndpoint(ob)
	if err != nil {
		return nil, err
	}
	uuid := xrayMapString(user, "id")
	if uuid == "" {
		return nil, errors.New(xrayReasonEmptyUserID)
	}

	outbound := map[string]interface{}{
		"tag":         xrayMapString(ob, "tag"),
		"type":        "vmess",
		"server":      addr,
		"server_port": port,
		"uuid":        uuid,
	}

	// security у vmess — шифр канала, не TLS-режим; allowlist общий с URI-путём.
	if security := strings.TrimSpace(xrayMapString(user, "security")); security != "" {
		outbound["security"] = normalizeVMessSecurityValue(security)
	} else {
		outbound["security"] = "auto"
	}
	if alterID := xrayJSONInt(user["alterId"]); alterID > 0 {
		outbound["alter_id"] = alterID
	}

	tls, transport := xrayStreamParts(ob)
	if tls != nil {
		outbound["tls"] = tls
	}
	if transport != nil {
		outbound["transport"] = transport
	}

	node := &configtypes.ParsedNode{
		Tag:      xrayTagOrDefault(ob, "vmess"),
		Scheme:   "vmess",
		Server:   addr,
		Port:     port,
		UUID:     uuid,
		Label:    label,
		Outbound: outbound,
	}
	noteXHTTPPlacementGuard(node, transport)
	return node, nil
}

// xrayBuildTrojanFromOutbound — trojan через settings.servers.
func xrayBuildTrojanFromOutbound(ob map[string]interface{}, label string) (*configtypes.ParsedNode, error) {
	addr, port, server, err := xrayServerEndpoint(ob)
	if err != nil {
		return nil, err
	}
	password := xrayMapString(server, "password")
	if password == "" {
		return nil, errors.New(xrayReasonNoPassword)
	}

	outbound := map[string]interface{}{
		"tag":         xrayMapString(ob, "tag"),
		"type":        "trojan",
		"server":      addr,
		"server_port": port,
		"password":    password,
	}

	tls, transport := xrayStreamParts(ob)
	if tls != nil {
		outbound["tls"] = tls
	}
	if transport != nil {
		outbound["transport"] = transport
	}

	node := &configtypes.ParsedNode{
		Tag:    xrayTagOrDefault(ob, "trojan"),
		Scheme: "trojan",
		Server: addr,
		Port:   port,
		// ParsedNode.UUID исторически хранит «главный секрет» узла независимо
		// от протокола — GenerateNodeJSON читает пароль trojan именно оттуда.
		UUID:     password,
		Label:    label,
		Outbound: outbound,
	}
	noteXHTTPPlacementGuard(node, transport)
	return node, nil
}

// xrayBuildShadowsocksFromOutbound — shadowsocks через settings.servers.
func xrayBuildShadowsocksFromOutbound(ob map[string]interface{}, label string) (*configtypes.ParsedNode, error) {
	addr, port, server, err := xrayServerEndpoint(ob)
	if err != nil {
		return nil, err
	}
	method := strings.TrimSpace(xrayMapString(server, "method"))
	password := xrayMapString(server, "password")
	if method == "" || password == "" {
		return nil, errors.New("empty shadowsocks method or password — the server returned a placeholder, subscription may be expired")
	}
	if !isValidShadowsocksMethod(method) {
		// Неподдерживаемый метод роняет весь конфиг — узел отбрасывается,
		// как и в URI-пути.
		return nil, fmt.Errorf("unsupported shadowsocks method %q", method)
	}

	outbound := map[string]interface{}{
		"tag":         xrayMapString(ob, "tag"),
		"type":        "shadowsocks",
		"server":      addr,
		"server_port": port,
		"method":      method,
		"password":    password,
	}

	return &configtypes.ParsedNode{
		Tag:      xrayTagOrDefault(ob, "ss"),
		Scheme:   "ss",
		Server:   addr,
		Port:     port,
		UUID:     password,
		Label:    label,
		Outbound: outbound,
	}, nil
}

// xrayBuildHysteria2FromOutbound — hysteria2 через settings.servers.
func xrayBuildHysteria2FromOutbound(ob map[string]interface{}, label string) (*configtypes.ParsedNode, error) {
	addr, port, server, err := xrayServerEndpoint(ob)
	if err != nil {
		return nil, err
	}
	password := xrayMapString(server, "password")

	outbound := map[string]interface{}{
		"tag":         xrayMapString(ob, "tag"),
		"type":        "hysteria2",
		"server":      addr,
		"server_port": port,
	}
	if password != "" {
		outbound["password"] = password
	}

	// hysteria2 работает поверх QUIC: uTLS и REALITY ядро на нём не примет,
	// поэтому TLS-блок чистится тем же санитайзом, что и при импорте sing-box.
	if tls, _ := xrayStreamParts(ob); tls != nil {
		outbound["tls"] = tls
	}
	sanitizeCodes := SanitizeSingboxOutboundMap(outbound, xrayMapString(ob, "tag"))

	node := &configtypes.ParsedNode{
		Tag:      xrayTagOrDefault(ob, "hysteria2"),
		Scheme:   "hysteria2",
		Server:   addr,
		Port:     port,
		UUID:     password,
		Label:    label,
		Outbound: outbound,
	}
	// Деградации санитайзера — на узел (см. SanitizeSingboxOutboundMap).
	for _, code := range sanitizeCodes {
		node.AddWarning(code)
	}
	return node, nil
}

// xrayTagOrDefault возвращает тег outbound'а либо запасное имя по протоколу.
func xrayTagOrDefault(ob map[string]interface{}, fallback string) string {
	if tag := xrayMapString(ob, "tag"); tag != "" {
		return tag
	}
	return fallback
}

// normalizeVMessSecurityValue приводит шифр vmess к принимаемому ядром виду.
//
// Значения вне allowlist ядро отвергает вместе со всем конфигом; "auto" —
// безопасный дефолт, который сервер согласует сам.
func normalizeVMessSecurityValue(security string) string {
	switch strings.ToLower(strings.TrimSpace(security)) {
	case "auto", "none", "zero", "aes-128-gcm", "chacha20-poly1305":
		return strings.ToLower(strings.TrimSpace(security))
	case "chacha20-ietf-poly1305":
		return "chacha20-poly1305"
	default:
		return "auto"
	}
}
