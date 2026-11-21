package linkmap

// Бинарный контейнер «тег–длина–значение» (контракт 1.1.175).
//
// Декодер формы `tlv` раскрывает пейлоад ссылки в объект JSON по таблице
// диалекта секции (`mappers.<kind>.tlv`), обратный ход `emit.container: "tlv"`
// собирает объект обратно в байты. Кодек не знает ни одного тега: имена,
// номера и типы полей приходят с диска.
//
// Почему таблицей, а не функцией под схему: формат отличается от любого
// соседнего только набором тегов, и «таблицей не выразить» здесь неверно —
// выразить нельзя лишь кодировку целого и DER, а они общие.
//
// go1.20-совместимо: без slices/maps/min/max.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"singbox-launcher/core/config/registry"
)

// Имена грамматики диалекта.
const (
	decoderTLV = "tlv"

	tlvInQuery     = "query"
	tlvInAuthority = "authority"

	tlvTypeVarint     = "varint"
	tlvTypeString     = "string"
	tlvTypeBool       = "bool"
	tlvTypeEnum       = "enum"
	tlvTypeStringList = "string_list"
	tlvTypeDERPEM     = "der_pem"
	tlvTypeHostPort   = "hostport"

	tlvUnknownReject = "reject"

	pemCertBegin = "-----BEGIN CERTIFICATE-----"
	pemCertEnd   = "-----END CERTIFICATE-----"
	pemLineWidth = 64
)

// tlvPayloadOf вырезает из текста ссылки пейлоад контейнера по объявленному
// диалектом месту: всё после `?` либо после `://`, до `#`.
func tlvPayloadOf(d *registry.TLVDialect, text string) (string, error) {
	rest := strings.TrimSpace(text)
	idx := strings.Index(rest, "://")
	if idx < 0 {
		return "", fmt.Errorf("linkmap: tlv: в тексте нет схемы")
	}
	rest = rest[idx+len("://"):]
	if i := strings.Index(rest, "#"); i >= 0 {
		rest = rest[:i]
	}
	if d.In == tlvInQuery {
		i := strings.Index(rest, "?")
		if i < 0 {
			return "", fmt.Errorf("linkmap: tlv: в ссылке нет query")
		}
		rest = rest[i+1:]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", fmt.Errorf("linkmap: tlv: пейлоад пуст")
	}
	return rest, nil
}

// tlvUnwrapBytes снимает текстовую обёртку. Читаются оба алфавита base64 и
// оба написания паддинга: ссылку переписывают руками и мессенджерами, а
// смысл у четырёх написаний один.
func tlvUnwrapBytes(s string) ([]byte, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return nil, fmt.Errorf("linkmap: tlv: обёртка не base64")
}

// readQUICVarint читает целое RFC 9000 §16 с позиции off.
func readQUICVarint(data []byte, off int) (uint64, int, error) {
	if off >= len(data) {
		return 0, 0, fmt.Errorf("linkmap: tlv: обрыв на целом")
	}
	first := data[off]
	width := 1 << (first >> 6)
	if off+width > len(data) {
		return 0, 0, fmt.Errorf("linkmap: tlv: обрыв на целом")
	}
	v := uint64(first & 0x3F)
	for i := 1; i < width; i++ {
		v = v<<8 | uint64(data[off+i])
	}
	return v, off + width, nil
}

// appendQUICVarint дописывает целое кратчайшей шириной.
func appendQUICVarint(dst []byte, v uint64) []byte {
	switch {
	case v <= 0x3F:
		return append(dst, byte(v))
	case v <= 0x3FFF:
		return append(dst, 0x40|byte(v>>8), byte(v))
	case v <= 0x3FFFFFFF:
		return append(dst, 0x80|byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
	return append(dst, 0xC0|byte(v>>56)&0xFF, byte(v>>48), byte(v>>40), byte(v>>32),
		byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// decodeTLV раскрывает текст ссылки в ТЕКСТ объекта JSON.
//
// Текст, а не карта: дальше его разбирает `space: json` тем же путём, что и
// контейнер, приехавший JSON'ом, — второго входа в пространство источников у
// бинарной формы нет.
func decodeTLV(d *registry.TLVDialect, text string) (string, error) {
	if d == nil {
		return "", fmt.Errorf("linkmap: tlv: у секции нет диалекта")
	}
	payload, err := tlvPayloadOf(d, text)
	if err != nil {
		return "", err
	}
	data, err := tlvUnwrapBytes(payload)
	if err != nil {
		return "", err
	}
	obj, err := decodeTLVBytes(d, data)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("linkmap: tlv: %w", err)
	}
	return string(out), nil
}

// decodeTLVBytes — байты контейнера → объект.
func decodeTLVBytes(d *registry.TLVDialect, data []byte) (map[string]interface{}, error) {
	obj := map[string]interface{}{}
	hostSeen := map[string]bool{}
	off := 0
	for off < len(data) {
		tag, next, err := readQUICVarint(data, off)
		if err != nil {
			return nil, err
		}
		size, next, err := readQUICVarint(data, next)
		if err != nil {
			return nil, err
		}
		if size > uint64(len(data)-next) {
			return nil, fmt.Errorf("linkmap: tlv: значение тега %d оборвано", tag)
		}
		val := data[next : next+int(size)]
		off = next + int(size)

		f := d.ByTag(tag)
		if f == nil {
			if d.UnknownTag == tlvUnknownReject {
				return nil, fmt.Errorf("linkmap: tlv: неизвестный тег %d", tag)
			}
			continue
		}
		if f.Type == tlvTypeHostPort {
			if err := putHostPort(obj, f, val, hostSeen); err != nil {
				return nil, err
			}
			continue
		}
		v, has, err := decodeTLVValue(f, val)
		if err != nil {
			return nil, err
		}
		if !has || tlvIsDefault(f, v) {
			delete(obj, f.Name)
			continue
		}
		obj[f.Name] = v
	}
	return obj, nil
}

// putHostPort раскладывает адрес: первое вхождение — три ключа, следующие —
// список `rest`.
func putHostPort(obj map[string]interface{}, f *registry.TLVField, val []byte, seen map[string]bool) error {
	raw := strings.TrimSpace(tlvText(val))
	if raw == "" {
		return nil
	}
	if seen[f.Name] && f.Rest != "" {
		list, _ := obj[f.Rest].([]interface{})
		obj[f.Rest] = append(list, raw)
		return nil
	}
	seen[f.Name] = true
	host, port := splitHostPortLoose(raw)
	obj[f.Name] = raw
	if f.Host != "" {
		obj[f.Host] = host
	}
	if f.Port != "" {
		if port > 0 {
			obj[f.Port] = port
		} else {
			delete(obj, f.Port)
		}
	}
	return nil
}

// splitHostPortLoose режет `хост:порт` / `[v6]:порт` / `хост`. Порт 0 — его
// нет или он не число 1–65535; голый IPv6 без скобок порта не несёт.
func splitHostPortLoose(addr string) (string, int) {
	if strings.HasPrefix(addr, "[") {
		if end := strings.Index(addr, "]"); end > 0 {
			host, rest := addr[1:end], addr[end+1:]
			if strings.HasPrefix(rest, ":") {
				return host, portNumber(rest[1:])
			}
			return host, 0
		}
	}
	colon := strings.LastIndex(addr, ":")
	if colon < 0 || strings.Index(addr, ":") != colon {
		return addr, 0
	}
	return addr[:colon], portNumber(addr[colon+1:])
}

func portNumber(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > 0xFFFF {
		return 0
	}
	return n
}

// tlvText — UTF-8 значения. Битые последовательности раскрытие не роняют:
// серия битых байтов даёт один U+FFFD (MAPPER_ENGINE §1).
func tlvText(val []byte) string {
	return strings.ToValidUTF8(string(val), "�")
}

// decodeTLVValue читает значение поля по его типу. has == false — поля нет
// (пустая строка, пустой список).
func decodeTLVValue(f *registry.TLVField, val []byte) (interface{}, bool, error) {
	switch f.Type {
	case tlvTypeVarint:
		n, _, err := readQUICVarint(val, 0)
		if err != nil {
			return nil, false, err
		}
		if f.Max != nil && n > uint64(*f.Max) {
			return nil, false, fmt.Errorf("linkmap: tlv: %s = %d больше %d", f.Name, n, *f.Max)
		}
		return float64(n), true, nil
	case tlvTypeBool:
		if len(val) != 1 || val[0] > 1 {
			return nil, false, fmt.Errorf("linkmap: tlv: %s не булево", f.Name)
		}
		return val[0] == 1, true, nil
	case tlvTypeEnum:
		if len(val) != 1 {
			return nil, false, fmt.Errorf("linkmap: tlv: %s не один байт", f.Name)
		}
		name, ok := f.Values[strconv.Itoa(int(val[0]))]
		if !ok {
			return nil, false, fmt.Errorf("linkmap: tlv: %s: значение %d не объявлено", f.Name, val[0])
		}
		return name, true, nil
	case tlvTypeStringList:
		var list []interface{}
		off := 0
		for off < len(val) {
			size, next, err := readQUICVarint(val, off)
			if err != nil {
				return nil, false, err
			}
			if size > uint64(len(val)-next) {
				return nil, false, fmt.Errorf("linkmap: tlv: элемент %s оборван", f.Name)
			}
			list = append(list, tlvText(val[next:next+int(size)]))
			off = next + int(size)
		}
		return list, len(list) > 0, nil
	case tlvTypeDERPEM:
		pem, err := derChainToPEM(val)
		if err != nil {
			return nil, false, fmt.Errorf("linkmap: tlv: %s: %w", f.Name, err)
		}
		return pem, pem != "", nil
	case tlvTypeString, "":
		s := tlvText(val)
		if s == "" {
			return nil, false, nil
		}
		if f.Prefix != "" && !strings.HasPrefix(s, f.Prefix) {
			return nil, false, fmt.Errorf("linkmap: tlv: %s не начинается с %q", f.Name, f.Prefix)
		}
		return s, true, nil
	}
	return nil, false, fmt.Errorf("linkmap: tlv: у поля %s неизвестный тип %q", f.Name, f.Type)
}

// tlvIsDefault — значение равно объявленному умолчанию формата.
func tlvIsDefault(f *registry.TLVField, v interface{}) bool {
	if f.Default == nil {
		return false
	}
	switch want := f.Default.(type) {
	case bool:
		got, ok := v.(bool)
		return ok && got == want
	case string:
		got, ok := v.(string)
		return ok && got == want
	case float64:
		got, ok := v.(float64)
		return ok && got == want
	}
	return false
}

// derChainToPEM режет подряд идущие DER-структуры и печатает каждую блоком
// CERTIFICATE: base64 строками по 64 символа, блоки через перевод строки,
// хвостового перевода строки нет.
//
// Резать приходится самим: поле несёт цепочку слитно, а тело узла ждёт PEM,
// где у каждого сертификата свой блок. Разбирается только заголовок ASN.1
// (тег SEQUENCE и длина) — годность сертификата судит ядро.
func derChainToPEM(der []byte) (string, error) {
	var blocks []string
	off := 0
	for off < len(der) {
		size, err := derElementLen(der[off:])
		if err != nil {
			return "", err
		}
		blocks = append(blocks, pemBlock(der[off:off+size]))
		off += size
	}
	return strings.Join(blocks, "\n"), nil
}

// derElementLen — полная длина первого DER-элемента (заголовок + тело).
func derElementLen(b []byte) (int, error) {
	if len(b) < 2 || b[0] != 0x30 {
		return 0, fmt.Errorf("не DER-последовательность")
	}
	first := int(b[1])
	header := 2
	size := first
	if first&0x80 != 0 {
		n := first & 0x7F
		if n == 0 || n > 4 || len(b) < 2+n {
			return 0, fmt.Errorf("длина DER не читается")
		}
		size = 0
		for i := 0; i < n; i++ {
			size = size<<8 | int(b[2+i])
		}
		header = 2 + n
	}
	if size <= 0 || header+size > len(b) {
		return 0, fmt.Errorf("DER оборван")
	}
	return header + size, nil
}

func pemBlock(der []byte) string {
	b64 := base64.StdEncoding.EncodeToString(der)
	var sb strings.Builder
	sb.WriteString(pemCertBegin)
	for i := 0; i < len(b64); i += pemLineWidth {
		end := i + pemLineWidth
		if end > len(b64) {
			end = len(b64)
		}
		sb.WriteByte('\n')
		sb.WriteString(b64[i:end])
	}
	sb.WriteByte('\n')
	sb.WriteString(pemCertEnd)
	return sb.String()
}

// pemToDERChain — обращение derChainToPEM: все блоки CERTIFICATE текста
// подряд. Значение тела бывает строкой и списком строк (listable).
func pemToDERChain(v interface{}) ([]byte, error) {
	text := ""
	switch t := v.(type) {
	case string:
		text = t
	case []interface{}:
		lines := make([]string, 0, len(t))
		for _, item := range t {
			lines = append(lines, toString(item))
		}
		text = strings.Join(lines, "\n")
	case []string:
		text = strings.Join(t, "\n")
	default:
		return nil, fmt.Errorf("значение не текст")
	}
	var out []byte
	rest := text
	for {
		i := strings.Index(rest, pemCertBegin)
		if i < 0 {
			break
		}
		rest = rest[i+len(pemCertBegin):]
		j := strings.Index(rest, pemCertEnd)
		if j < 0 {
			return nil, fmt.Errorf("блок PEM не закрыт")
		}
		body := strings.Join(strings.Fields(rest[:j]), "")
		der, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			return nil, fmt.Errorf("тело PEM не base64")
		}
		out = append(out, der...)
		rest = rest[j+len(pemCertEnd):]
	}
	return out, nil
}

// encodeTLV собирает объект контейнера в текст пейлоада.
//
// Порядок полей — `order` (объявленный секцией `emit.param_order`), затем
// остальные в порядке таблицы диалекта: порядок входит в вид ссылки, а вид
// есть де-факто формат чужих клиентов (MAPPER_ENGINE §9.1).
func encodeTLV(d *registry.TLVDialect, obj map[string]interface{}, order []string) (string, error) {
	if d == nil {
		return "", fmt.Errorf("linkmap: tlv: у секции нет диалекта")
	}
	done := map[string]bool{}
	var names []string
	for _, n := range order {
		if !done[n] && d.Field(n) != nil {
			done[n] = true
			names = append(names, n)
		}
	}
	for i := range d.Fields {
		if n := d.Fields[i].Name; !done[n] {
			done[n] = true
			names = append(names, n)
		}
	}

	var out []byte
	for _, name := range names {
		f := d.Field(name)
		vals, err := encodeTLVField(f, obj)
		if err != nil {
			return "", err
		}
		for _, val := range vals {
			out = appendQUICVarint(out, uint64(f.Tag))
			out = appendQUICVarint(out, uint64(len(val)))
			out = append(out, val...)
		}
	}
	return base64.RawURLEncoding.EncodeToString(out), nil
}

// encodeTLVField — значения одного поля (их несколько только у адреса со
// списком `rest`). Пусто — поля в объекте нет либо оно равно умолчанию.
func encodeTLVField(f *registry.TLVField, obj map[string]interface{}) ([][]byte, error) {
	if f.Type == tlvTypeHostPort {
		return encodeHostPort(f, obj), nil
	}
	v, ok := obj[f.Name]
	if !ok || v == nil {
		return nil, nil
	}
	switch f.Type {
	case tlvTypeVarint:
		n, err := strconv.ParseUint(strings.TrimSpace(toString(normalizeNumber(v))), 10, 62)
		if err != nil {
			return nil, fmt.Errorf("linkmap: tlv: %s не целое", f.Name)
		}
		if tlvIsDefault(f, float64(n)) {
			return nil, nil
		}
		return [][]byte{appendQUICVarint(nil, n)}, nil
	case tlvTypeBool:
		b := boolOf(v)
		if tlvIsDefault(f, b) {
			return nil, nil
		}
		if b {
			return [][]byte{{1}}, nil
		}
		return [][]byte{{0}}, nil
	case tlvTypeEnum:
		name := toString(v)
		if name == "" || tlvIsDefault(f, name) {
			return nil, nil
		}
		for code, n := range f.Values {
			if n != name {
				continue
			}
			b, err := strconv.Atoi(code)
			if err != nil || b < 0 || b > 0xFF {
				break
			}
			return [][]byte{{byte(b)}}, nil
		}
		// Имени нет в таблице — формат этого значения не выражает, и поле
		// на выходе просто не пишется: читатель возьмёт своё умолчание.
		return nil, nil
	case tlvTypeStringList:
		var buf []byte
		for _, item := range tlvStrings(v) {
			buf = appendQUICVarint(buf, uint64(len(item)))
			buf = append(buf, item...)
		}
		if len(buf) == 0 {
			return nil, nil
		}
		return [][]byte{buf}, nil
	case tlvTypeDERPEM:
		der, err := pemToDERChain(v)
		if err != nil {
			return nil, fmt.Errorf("linkmap: tlv: %s: %w", f.Name, err)
		}
		if len(der) == 0 {
			return nil, nil
		}
		return [][]byte{der}, nil
	}
	s := toString(v)
	if s == "" {
		return nil, nil
	}
	return [][]byte{[]byte(s)}, nil
}

// encodeHostPort — адрес узла и адреса списка `rest`.
func encodeHostPort(f *registry.TLVField, obj map[string]interface{}) [][]byte {
	var out [][]byte
	first := toString(obj[f.Name])
	if first == "" && f.Host != "" {
		if host := toString(obj[f.Host]); host != "" {
			port := 0
			if f.Port != "" {
				port = intOf(obj[f.Port])
			}
			first = hostJoin(host, port)
		}
	}
	if first != "" {
		out = append(out, []byte(first))
	}
	if f.Rest != "" {
		for _, item := range tlvStrings(obj[f.Rest]) {
			out = append(out, []byte(item))
		}
	}
	return out
}

// tlvStrings — значение как список непустых строк.
func tlvStrings(v interface{}) []string {
	var out []string
	switch t := v.(type) {
	case []interface{}:
		for _, item := range t {
			if s := toString(item); s != "" {
				out = append(out, s)
			}
		}
	case []string:
		for _, s := range t {
			if s != "" {
				out = append(out, s)
			}
		}
	case string:
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
