package subscription

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// Amnezia vpn:// link import (SPEC 075).
//
// Format (reference: amnezia-vpn/config-decoder, mainwindow.cpp; verified
// reference implementation in scripts/decode_amnezia_vpn.py):
//
//	vpn:// + base64url (alphabet -_, no padding: Qt Base64UrlEncoding|OmitTrailingEquals)
//	payload = qCompress(json, 8): 4-byte big-endian uncompressed size, then a zlib stream
//	json    = full Amnezia profile: containers[] (one per protocol), defaultContainer,
//	          hostName, description, dns1/dns2. The WG/AWG tunnel itself sits inside a
//	          container as last_config — a JSON string whose "config" field holds the
//	          classic [Interface]/[Peer] INI text (incl. AWG Jc/Jmin/.../I1-I5 fields).
//
// Only WireGuard/AmneziaWG containers are importable. The INI is converted to the
// canonical wireguard:// URI and delegated to ParseNode, so CIDR
// normalization, AWG param promotion and the AWG MTU clamp (SPEC 073) all apply
// unchanged.

const (
	// maxAmneziaLinkLength caps the raw vpn:// link. Amnezia profiles bundle whole
	// certificates for some protocols and routinely exceed MaxURILength; 512 KB is
	// far above any real profile yet keeps a hostile link from ballooning memory.
	maxAmneziaLinkLength = 512 * 1024
	// maxAmneziaProfileJSON caps the decompressed profile (zlib-bomb guard).
	maxAmneziaProfileJSON = 8 * 1024 * 1024
	// maxAmneziaScanDepth bounds the recursive [Interface] search: the deepest
	// known nesting is containers[] → container → proto → last_config (JSON
	// string) → config, i.e. 5 levels; +headroom for schema drift.
	maxAmneziaScanDepth = 8
)

// parseAmneziaVPNLink parses a vpn:// link into a WireGuard/AmneziaWG ParsedNode.
func parseAmneziaVPNLink(uri string, skipFilters []map[string]string) (*configtypes.ParsedNode, error) {
	debuglog.DebugLog("parseAmneziaVPNLink: start (link length %d)", len(uri))
	if len(uri) > maxAmneziaLinkLength {
		return nil, fmt.Errorf("vpn:// link length (%d) exceeds maximum (%d)", len(uri), maxAmneziaLinkLength)
	}
	payload := strings.TrimPrefix(strings.TrimSpace(uri), "vpn://")
	profile, err := decodeAmneziaProfile(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode vpn:// profile: %w", err)
	}

	confText, containerName, containerCount := amneziaWGConfText(profile)
	if confText == "" {
		return nil, fmt.Errorf("vpn:// profile has no WireGuard/AmneziaWG config (containers: %s)",
			strings.Join(amneziaContainerNames(profile), ", "))
	}
	debuglog.DebugLog("parseAmneziaVPNLink: using container %q (host %q)", containerName, amneziaString(profile, "hostName"))

	label := amneziaString(profile, "description")
	if label == "" {
		label = amneziaString(profile, "hostName")
	}
	if label == "" {
		label = containerName
	}

	// Текст `.conf` ведёт СЕКЦИЯ реестра напрямую; имя профиля едет
	// источником `hint`, и куда его поставить в цепочке метки, решает сама
	// секция (SPEC 133).
	node, err, known := ParseWGConfByEngineHint(confText, label, skipFilters)
	if !known {
		return nil, fmt.Errorf("vpn:// container %q is not a WireGuard config", containerName)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid WireGuard config in vpn:// container %q: %w", containerName, err)
	}
	if node == nil {
		return nil, nil
	}
	// Одиночный путь отдал один контейнер из нескольких — на узел ставится
	// info-код: остальные локации профиля в этот вызов не попали.
	if containerCount > 1 {
		node.AddWarning(WarnAmneziaContainerChoice)
	}
	return node, nil
}

// decodeAmneziaProfile turns the base64url payload of a vpn:// link into the
// profile JSON. Tolerant input: whitespace/newlines are dropped (links pasted
// from chats get wrapped), padding is stripped, and the standard base64
// alphabet is accepted as a fallback.
func decodeAmneziaProfile(payload string) (map[string]interface{}, error) {
	payload = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, payload)
	payload = strings.TrimRight(payload, "=")
	if payload == "" {
		return nil, fmt.Errorf("empty payload")
	}

	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		var stdErr error
		raw, stdErr = base64.RawStdEncoding.DecodeString(payload)
		if stdErr != nil {
			return nil, fmt.Errorf("invalid base64: %w", err)
		}
	}
	// Несжатый профиль: голый base64(JSON) без qCompress-фрейминга.
	// Amnezia так экспортирует часть ссылок (паритет importController), и
	// LxBox это принимает; проверка идёт ДО фрейминга, потому что первые
	// 4 байта такого payload — начало JSON, а не длина (SPEC 103 §9.B12).
	if plain := bytes.TrimSpace(raw); len(plain) > 0 && plain[0] == '{' {
		if len(plain) > maxAmneziaProfileJSON {
			return nil, fmt.Errorf("uncompressed profile exceeds %d bytes", maxAmneziaProfileJSON)
		}
		var profile map[string]interface{}
		if err := json.Unmarshal(plain, &profile); err == nil {
			return profile, nil
		}
		// Начинается с '{', но не разбирается — это не «почти JSON», а битые
		// данные: qCompress-ветка ниже на них всё равно упадёт, зато сообщение
		// будет про zlib и уведёт диагностику не туда.
		return nil, fmt.Errorf("payload looks like JSON but does not parse")
	}

	// qCompress framing: 4-byte big-endian uncompressed size + zlib stream.
	if len(raw) < 5 {
		return nil, fmt.Errorf("payload too short for qCompress framing (%d bytes)", len(raw))
	}
	expected := binary.BigEndian.Uint32(raw[:4])
	if expected == 0 || expected > maxAmneziaProfileJSON {
		return nil, fmt.Errorf("declared uncompressed size %d out of range", expected)
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw[4:]))
	if err != nil {
		return nil, fmt.Errorf("invalid zlib stream: %w", err)
	}
	defer func() { _ = zr.Close() }()
	data, err := io.ReadAll(io.LimitReader(zr, maxAmneziaProfileJSON+1))
	if err != nil {
		return nil, fmt.Errorf("zlib decompression failed: %w", err)
	}
	if len(data) > maxAmneziaProfileJSON {
		return nil, fmt.Errorf("decompressed profile exceeds %d bytes", maxAmneziaProfileJSON)
	}
	if uint32(len(data)) != expected {
		// Header mismatch is suspicious but not fatal: trust the actual stream.
		debuglog.DebugLog("decodeAmneziaProfile: qCompress header says %d bytes, got %d", expected, len(data))
	}

	var profile map[string]interface{}
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("profile is not valid JSON: %w", err)
	}
	return profile, nil
}

// amneziaWGConfText picks the WG/AWG [Interface]/[Peer] text out of the profile.
// The defaultContainer is tried first, then the rest in array order; the first
// container that yields an [Interface] text wins. Returns the text and the
// container name ("" if nothing found).
// Третьим значением возвращает число найденных WG/AWG-контейнеров: одиночный
// путь отдаёт ОДИН узел (сигнатура ParseNode), и пользователь обязан узнать,
// что в профиле их было больше — иначе остальные локации теряются молча
// (contract/registry/warnings.json: amnezia_container_choice, info).
func amneziaWGConfText(profile map[string]interface{}) (string, string, int) {
	containers, _ := profile["containers"].([]interface{})
	defaultName, _ := profile["defaultContainer"].(string)

	ordered := make([]map[string]interface{}, 0, len(containers))
	for _, c := range containers {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _ := cm["container"].(string); name != "" && name == defaultName {
			ordered = append([]map[string]interface{}{cm}, ordered...)
		} else {
			ordered = append(ordered, cm)
		}
	}

	matched := 0
	confText, containerName := "", ""
	for _, cm := range ordered {
		if txt, owner := findWGIniText(cm, 0); txt != "" {
			matched++
			if confText == "" {
				confText = amneziaPrepareConf(txt, owner, profile)
				containerName, _ = cm["container"].(string)
			}
		}
	}
	if matched > 1 {
		debuglog.WarnLog("Parser: vpn:// profile has %d WireGuard/AWG containers, importing %q (default container preferred)", matched, containerName)
	}
	return confText, containerName, matched
}

// amneziaAllWGConfTexts возвращает ВСЕ WG/AWG-контейнеры профиля в
// детерминированном порядке (дефолтный первым), а не только первый.
//
// SPEC 103 §9.B12: ParseNode отдаёт ровно одну ноду, поэтому одиночный путь
// вынужденно берёт один контейнер и предупреждает об остальных. Но профиль с
// несколькими локациями — штатный случай Amnezia, и терять их при импорте
// тела подписки незачем: LxBox импортирует все, и расхождение решено в его
// пользу (не терять данные пользователя).
func amneziaAllWGConfTexts(profile map[string]interface{}) (texts []string, names []string) {
	containers, _ := profile["containers"].([]interface{})
	defaultName, _ := profile["defaultContainer"].(string)

	ordered := make([]map[string]interface{}, 0, len(containers))
	for _, c := range containers {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _ := cm["container"].(string); name != "" && name == defaultName {
			ordered = append([]map[string]interface{}{cm}, ordered...)
		} else {
			ordered = append(ordered, cm)
		}
	}

	for _, cm := range ordered {
		if txt, owner := findWGIniText(cm, 0); txt != "" {
			name, _ := cm["container"].(string)
			texts = append(texts, amneziaPrepareConf(txt, owner, profile))
			names = append(names, name)
		}
	}
	return texts, names
}

// ParseAmneziaVPNLinkAll разбирает vpn://-ссылку во ВСЕ её WG/AWG-узлы.
//
// Используется там, где вызывающий умеет принять несколько нод: тело
// подписки (BodyKindVPNLink) и импорт из файла. Одиночный ParseNode
// продолжает отдавать один узел — сигнатуру менять нельзя, её зовут из
// построчного разбора URI-списка.
//
// Битый контейнер пропускается со счётчиком: профиль с четырьмя локациями,
// одна из которых без Endpoint, обязан дать три ноды, а не ошибку.
func ParseAmneziaVPNLinkAll(uri string, skipFilters []map[string]string) ([]*configtypes.ParsedNode, int, error) {
	if len(uri) > maxAmneziaLinkLength {
		return nil, 0, fmt.Errorf("vpn:// link length (%d) exceeds maximum (%d)", len(uri), maxAmneziaLinkLength)
	}
	payload := strings.TrimPrefix(strings.TrimSpace(uri), "vpn://")
	profile, err := decodeAmneziaProfile(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to decode vpn:// profile: %w", err)
	}

	texts, names := amneziaAllWGConfTexts(profile)
	if len(texts) == 0 {
		return nil, 0, fmt.Errorf("vpn:// profile has no WireGuard/AmneziaWG config (containers: %s)",
			strings.Join(amneziaContainerNames(profile), ", "))
	}

	baseLabel := amneziaString(profile, "description")
	if baseLabel == "" {
		baseLabel = amneziaString(profile, "hostName")
	}

	nodes := make([]*configtypes.ParsedNode, 0, len(texts))
	skipped := 0
	for i, confText := range texts {
		// Метка узла: описание профиля, а при нескольких контейнерах — с
		// именем контейнера, иначе все локации получат одинаковый тег и
		// MakeTagUnique размножит их в «…-2», «…-3» без смысла.
		label := baseLabel
		if label == "" {
			label = names[i]
		} else if len(texts) > 1 && names[i] != "" {
			label = label + " " + names[i]
		}

		node, parseErr, known := ParseWGConfByEngineHint(confText, label, skipFilters)
		if !known {
			debuglog.WarnLog("Parser: vpn:// container %q: not a WireGuard config", names[i])
			skipped++
			continue
		}
		if parseErr != nil {
			debuglog.WarnLog("Parser: vpn:// container %q: %v", names[i], parseErr)
			skipped++
			continue
		}
		if node != nil {
			nodes = append(nodes, node)
		}
	}
	return nodes, skipped, nil
}

// findWGIniText recursively searches a decoded profile value for a WireGuard
// [Interface]/[Peer] text. JSON-looking strings are unwrapped first: the
// canonical spot is last_config — a JSON *string* whose raw text also contains
// "[Interface]" (with \n escapes), so the nested "config" field must win over
// the wrapper. Known keys are tried before the rest to keep the walk
// deterministic across Go's random map iteration.
// Вторым значением отдаёт карту, НЕПОСРЕДСТВЕННО содержавшую найденный текст
// (Amnezia кладёт рядом с `config` ещё и `mtu`, которого в [Interface] нет).
func findWGIniText(v interface{}, depth int) (string, map[string]interface{}) {
	if depth > maxAmneziaScanDepth {
		return "", nil
	}
	switch t := v.(type) {
	case string:
		if s := strings.TrimSpace(t); strings.HasPrefix(s, "{") {
			var nested interface{}
			if err := json.Unmarshal([]byte(s), &nested); err == nil {
				return findWGIniText(nested, depth+1)
			}
		}
		if strings.Contains(t, "[Interface]") {
			return t, nil
		}
	case map[string]interface{}:
		knownKeys := []string{"config", "last_config", "awg", "wireguard"}
		for _, k := range knownKeys {
			if nv, ok := t[k]; ok {
				if r, owner := findWGIniText(nv, depth+1); r != "" {
					if owner == nil {
						owner = t
					}
					return r, owner
				}
			}
		}
		for k, nv := range t {
			switch k {
			case "config", "last_config", "awg", "wireguard":
				continue
			}
			if r, owner := findWGIniText(nv, depth+1); r != "" {
				if owner == nil {
					owner = t
				}
				return r, owner
			}
		}
	case []interface{}:
		for _, nv := range t {
			if r, owner := findWGIniText(nv, depth+1); r != "" {
				return r, owner
			}
		}
	}
	return "", nil
}

// amneziaPrepareConf доводит [Interface]-текст экспорта до вида, из которого
// секция реестра соберёт полное тело. Две правки, обе — потеря данных без неё:
//
//   - MTU у экспорта Amnezia лежит НЕ в [Interface], а рядом с `config` в
//     last_config ("1376"). Явный MTU в [Interface] приоритетнее — он ближе к
//     туннелю.
//
//     ОБОСНОВАНИЕ ЗДЕСЬ БЫЛО НЕВЕРНЫМ: комментарий утверждал, что подъём mtu
//     из last_config спасает узел от клампа 1280, а кламп срабатывал ровно
//     так же и возвращал 1280 — работа отменяла сама себя (находка №5
//     LEGACY_AUDIT). Настоящая польза правки другая и к AWG отношения не
//     имеет: у ОБЫЧНОГО WireGuard-профиля потолка нет вовсе, и без этой
//     строки его узел терял прописанный сервером MTU целиком. У AWG-узла
//     значение выше 1280 теперь заменяется правилом реестра (max_when) — и
//     заменяется С КОДОМ, то есть человек об этом узнает, а не как раньше.
//
//   - DNS = $PRIMARY_DNS, $SECONDARY_DNS — плейсхолдеры Amnezia; адреса лежат в
//     корне профиля (dns1/dns2). Неразрешённый плейсхолдер выбрасывается из
//     списка, пустой список не пишется вовсе: строка `dns=%24PRIMARY_DNS`
//     уезжала в конфиг как имя сервера.
func amneziaPrepareConf(text string, lastConfig, profile map[string]interface{}) string {
	iface, _ := parseWGConfSections(text)
	if iface["mtu"] == "" {
		if mtu := amneziaMTUValue(lastConfig); mtu != "" {
			text = amneziaSetInterfaceLine(text, "MTU = "+mtu)
		}
	}
	if dns := iface["dns"]; strings.Contains(dns, "$") {
		resolved := make([]string, 0, 2)
		for _, part := range splitAndTrim(dns, ",") {
			switch part {
			case "$PRIMARY_DNS":
				part = amneziaString(profile, "dns1")
			case "$SECONDARY_DNS":
				part = amneziaString(profile, "dns2")
			}
			if part != "" && !strings.Contains(part, "$") {
				resolved = append(resolved, part)
			}
		}
		text = amneziaReplaceDNSLine(text, resolved)
	}
	return text
}

// amneziaMTUValue reads last_config.mtu ("1376" or 1376) as a positive int.
func amneziaMTUValue(lastConfig map[string]interface{}) string {
	if lastConfig == nil {
		return ""
	}
	var n int
	switch v := lastConfig["mtu"].(type) {
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return ""
		}
		n = parsed
	case float64:
		n = int(v)
	default:
		return ""
	}
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// amneziaSetInterfaceLine appends a line to the [Interface] section.
func amneziaSetInterfaceLine(text, line string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, l := range lines {
		if strings.EqualFold(strings.TrimSpace(l), "[Interface]") {
			out := append([]string{}, lines[:i+1]...)
			out = append(out, line)
			return strings.Join(append(out, lines[i+1:]...), "\n")
		}
	}
	return text
}

// amneziaReplaceDNSLine rewrites (or removes, when the list is empty) the DNS
// line of the [Interface] section.
func amneziaReplaceDNSLine(text string, values []string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		key, _, ok := strings.Cut(l, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), "dns") {
			if len(values) == 0 {
				continue
			}
			out = append(out, "DNS = "+strings.Join(values, ", "))
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// parseWGConfSections splits a [Interface]/[Peer] INI text into two maps with
// lower-cased keys. Only the first [Peer] section is honored (multi-peer is not
// supported by the WG share/parse path, see shareuri_wireguard.go).
func parseWGConfSections(text string) (iface, peer map[string]string) {
	iface, peer = map[string]string{}, map[string]string{}
	section := ""
	peerSections := 0
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			if section == "peer" {
				peerSections++
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		switch section {
		case "interface":
			iface[key] = value
		case "peer":
			if peerSections == 1 {
				peer[key] = value
			}
		}
	}
	return iface, peer
}

// amneziaContainerNames lists container names for error messages.
func amneziaContainerNames(profile map[string]interface{}) []string {
	containers, _ := profile["containers"].([]interface{})
	names := make([]string, 0, len(containers))
	for _, c := range containers {
		if cm, ok := c.(map[string]interface{}); ok {
			if name, _ := cm["container"].(string); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// amneziaString reads a trimmed top-level string field from the profile.
func amneziaString(profile map[string]interface{}, key string) string {
	s, _ := profile[key].(string)
	return strings.TrimSpace(s)
}
