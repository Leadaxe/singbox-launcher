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
	"singbox-launcher/core/config/linkmap"
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
		return nil, linkmap.NewReject(WarnURITooLong,
			map[string]string{"length": strconv.Itoa(len(uri)), "limit": strconv.Itoa(maxAmneziaLinkLength)},
			fmt.Errorf("vpn:// link length (%d) exceeds maximum (%d)", len(uri), maxAmneziaLinkLength))
	}
	payload := amneziaPayload(uri)
	profile, bareConf, err := decodeAmneziaPayload(payload)
	if err != nil {
		return nil, linkmap.NewReject(linkmap.CodeFormUnrecognized, nil, fmt.Errorf("failed to decode vpn:// profile: %w", err))
	}
	// Голый `.conf` под оболочкой — форма `bare_conf` реестра: контейнеров
	// тут нет вовсе, и судить его надо веткой `.conf`, а не искать в нём
	// профиль. Метку даёт сам конфиг (комментарий над [Interface] либо хост
	// Endpoint), потому что подсказывать её из профиля нечем.
	if profile == nil && bareConf != "" {
		node, parseErr, known := ParseWGConfByEngineHint(bareConf, "", skipFilters)
		if !known {
			return nil, fmt.Errorf("vpn:// payload is not a WireGuard config")
		}
		if parseErr != nil {
			return nil, fmt.Errorf("invalid WireGuard config in vpn:// payload: %w", parseErr)
		}
		return node, nil
	}

	confText, confContext, containerName, containerCount := amneziaWGConfText(profile)
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
	node, err, known := ParseWGConfByEngineContext(confText, label, confContext, skipFilters)
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

// decodeAmneziaPayload снимает оболочку `vpn://` (base64url либо base64,
// пробелы и переводы строк отбрасываются — ссылку из чата переносят с
// переносами) и говорит, ЧЕМ оказалась
// полезная нагрузка: профилем Amnezia (первое значение) либо голым
// `.conf`-текстом wg-quick/AmneziaWG (второе).
//
// Норма — contract/registry/source_kinds.json, ветка `amnezia_link`,
// `payload_forms`: форма `profile_json` (голый JSON либо qCompress-фрейминг) и
// форма `bare_conf` (первая НЕ-комментарная секция — `[Interface]`, тот же
// предикат, что у вида источника `wireguard_conf`). Панели раздают под
// `vpn://` именно вторую форму, и прежде она уходила в ветку qCompress, где
// первые четыре байта INI читались как объявленная длина: пользователь
// получал ноль узлов с диагнозом «declared uncompressed size … out of range»
// — сообщением, уводившим чинить не то (контракт 1.1.48).
//
// Паддинг `=` необязателен: панели его срезают (наблюдалось на живой ссылке),
// поэтому обе азбуки пробуются в Raw-виде, а хвостовые `=` снимаются до
// декода.
func decodeAmneziaPayload(payload string) (map[string]interface{}, string, error) {
	payload = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, payload)
	payload = strings.TrimRight(payload, "=")
	if payload == "" {
		return nil, "", fmt.Errorf("empty payload")
	}

	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		var stdErr error
		raw, stdErr = base64.RawStdEncoding.DecodeString(payload)
		if stdErr != nil {
			return nil, "", fmt.Errorf("invalid base64: %w", err)
		}
	}
	plain := bytes.TrimSpace(raw)
	// Несжатый профиль: голый base64(JSON) без qCompress-фрейминга.
	// Amnezia так экспортирует часть ссылок (паритет importController), и
	// LxBox это принимает; проверка идёт ДО фрейминга, потому что первые
	// 4 байта такого payload — начало JSON, а не длина (SPEC 103 §9.B12).
	if len(plain) > 0 && plain[0] == '{' {
		if len(plain) > maxAmneziaProfileJSON {
			return nil, "", fmt.Errorf("uncompressed profile exceeds %d bytes", maxAmneziaProfileJSON)
		}
		var profile map[string]interface{}
		if err := json.Unmarshal(plain, &profile); err == nil {
			return profile, "", nil
		}
		// Начинается с '{', но не разбирается — это не «почти JSON», а битые
		// данные: qCompress-ветка ниже на них всё равно упадёт, зато сообщение
		// будет про zlib и уведёт диагностику не туда.
		return nil, "", fmt.Errorf("payload looks like JSON but does not parse")
	}
	// Голый `.conf`: под оболочкой не пакет-профиль, а сам wg-quick/AWG INI
	// (форма `bare_conf` реестра). Признак — ТОТ ЖЕ, что у вида источника
	// `wireguard_conf`: первая не-комментарная секция — `[Interface]`, и
	// второго определения «что такое .conf» тут не заводится. Проверка идёт
	// до фрейминга по той же причине, что и JSON: первые байты INI — это
	// текст, а не объявленная длина.
	if len(plain) > 0 && plain[0] == '[' && looksLikeWGConf(string(plain)) {
		if len(plain) > maxAmneziaProfileJSON {
			return nil, "", fmt.Errorf("bare config exceeds %d bytes", maxAmneziaProfileJSON)
		}
		return nil, string(plain), nil
	}

	// qCompress framing: 4-byte big-endian uncompressed size + zlib stream.
	if len(raw) < 5 {
		return nil, "", fmt.Errorf("payload too short for qCompress framing (%d bytes)", len(raw))
	}
	expected := binary.BigEndian.Uint32(raw[:4])
	if expected == 0 || expected > maxAmneziaProfileJSON {
		return nil, "", fmt.Errorf("declared uncompressed size %d out of range", expected)
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw[4:]))
	if err != nil {
		return nil, "", fmt.Errorf("invalid zlib stream: %w", err)
	}
	defer func() { _ = zr.Close() }()
	data, err := io.ReadAll(io.LimitReader(zr, maxAmneziaProfileJSON+1))
	if err != nil {
		return nil, "", fmt.Errorf("zlib decompression failed: %w", err)
	}
	if len(data) > maxAmneziaProfileJSON {
		return nil, "", fmt.Errorf("decompressed profile exceeds %d bytes", maxAmneziaProfileJSON)
	}
	if uint32(len(data)) != expected {
		// Header mismatch is suspicious but not fatal: trust the actual stream.
		debuglog.DebugLog("decodeAmneziaPayload: qCompress header says %d bytes, got %d", expected, len(data))
	}

	var profile map[string]interface{}
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, "", fmt.Errorf("profile is not valid JSON: %w", err)
	}
	return profile, "", nil
}

// amneziaWGConfText picks the WG/AWG [Interface]/[Peer] text out of the profile.
// The defaultContainer is tried first, then the rest in array order; the first
// container that yields an [Interface] text wins. Returns the text and the
// container name ("" if nothing found).
// Третьим значением возвращает число найденных WG/AWG-контейнеров: одиночный
// путь отдаёт ОДИН узел (сигнатура ParseNode), и пользователь обязан узнать,
// что в профиле их было больше — иначе остальные локации теряются молча
// (contract/registry/warnings.json: amnezia_container_choice, info).
func amneziaWGConfText(profile map[string]interface{}) (string, map[string]interface{}, string, int) {
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
	var confContext map[string]interface{}
	for _, cm := range ordered {
		if txt, owner := findWGIniText(cm, 0); txt != "" {
			matched++
			if confText == "" {
				confText = txt
				confContext = amneziaConfContext(owner, profile)
				containerName, _ = cm["container"].(string)
			}
		}
	}
	if matched > 1 {
		debuglog.WarnLog("Parser: vpn:// profile has %d WireGuard/AWG containers, importing %q (default container preferred)", matched, containerName)
	}
	return confText, confContext, containerName, matched
}

// amneziaAllWGConfTexts возвращает ВСЕ WG/AWG-контейнеры профиля в
// детерминированном порядке (дефолтный первым), а не только первый.
//
// SPEC 103 §9.B12: ParseNode отдаёт ровно одну ноду, поэтому одиночный путь
// вынужденно берёт один контейнер и предупреждает об остальных. Но профиль с
// несколькими локациями — штатный случай Amnezia, и терять их при импорте
// тела подписки незачем: LxBox импортирует все, и расхождение решено в его
// пользу (не терять данные пользователя).
func amneziaAllWGConfTexts(profile map[string]interface{}) (texts []string, contexts []map[string]interface{}, names []string) {
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
			texts = append(texts, txt)
			contexts = append(contexts, amneziaConfContext(owner, profile))
			names = append(names, name)
		}
	}
	return texts, contexts, names
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
		return nil, 0, linkmap.NewReject(WarnURITooLong,
			map[string]string{"length": strconv.Itoa(len(uri)), "limit": strconv.Itoa(maxAmneziaLinkLength)},
			fmt.Errorf("vpn:// link length (%d) exceeds maximum (%d)", len(uri), maxAmneziaLinkLength))
	}
	payload := amneziaPayload(uri)
	profile, bareConf, err := decodeAmneziaPayload(payload)
	if err != nil {
		return nil, 0, linkmap.NewReject(linkmap.CodeFormUnrecognized, nil, fmt.Errorf("failed to decode vpn:// profile: %w", err))
	}
	// Голый `.conf` (форма `bare_conf` реестра) даёт РОВНО один узел:
	// контейнеров у него нет, и множественный путь отличается от одиночного
	// только тем, что отдаёт список из одного элемента.
	if profile == nil && bareConf != "" {
		node, parseErr, known := ParseWGConfByEngineHint(bareConf, "", skipFilters)
		if !known {
			return nil, 0, fmt.Errorf("vpn:// payload is not a WireGuard config")
		}
		if parseErr != nil {
			return nil, 0, fmt.Errorf("invalid WireGuard config in vpn:// payload: %w", parseErr)
		}
		if node == nil {
			return nil, 0, nil
		}
		return []*configtypes.ParsedNode{node}, 0, nil
	}

	texts, contexts, names := amneziaAllWGConfTexts(profile)
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

		node, parseErr, known := ParseWGConfByEngineContext(confText, label, contexts[i], skipFilters)
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

// amneziaConfContext — то, что профиль Amnezia знает о `.conf`-тексте, но в
// самом тексте нет: объект, непосредственно содержавший текст (у экспорта
// Amnezia это last_config, где рядом с `config` лежит `mtu`), и корень
// профиля (`dns1`/`dns2` для плейсхолдеров `$PRIMARY_DNS`/`$SECONDARY_DNS`).
//
// Распаковщик НЕ решает, что из этого поднять в узел: он лишь отдаёт
// контекст секции реестра источником `context.<путь>` (контракт 1.1.63), а
// правила — MTU из `context.container.mtu`, когда в [Interface] его нет, и
// подстановка DNS — записи `mtu_container` и `dns` секции `conf` протокола
// wireguard. Прежде обе правки делал код здесь, переписывая INI-текст.
func amneziaConfContext(owner, profile map[string]interface{}) map[string]interface{} {
	ctx := map[string]interface{}{}
	if owner != nil {
		ctx["container"] = owner
	}
	if profile != nil {
		ctx["profile"] = profile
	}
	return ctx
}

// amneziaPayload — полезная нагрузка ссылки-контейнера: всё после «://».
// Признак самого контейнера (префикс) судит реестр — detect вида источника
// с распаковщиком `amnezia_vpn` (source_kinds.json); здесь схема ссылки лишь
// отрезается, какой бы она ни была написана.
func amneziaPayload(uri string) string {
	t := strings.TrimSpace(uri)
	if i := strings.Index(t, "://"); i >= 0 {
		return t[i+3:]
	}
	return t
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
