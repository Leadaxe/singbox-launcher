// Package nodeflow — ядро конвейера узла (SPEC 131 §3): санитайзер по
// реестру, тупой эмиттер и гейт сборки по версии ядра и платформе.
//
// Ни одного `if scheme == "..."`: все схемные различия живут в реестре
// контракта (contract/registry), код лишь исполняет его правила. Из этого
// следует и порядок warnings — он равен порядку обхода body.order, а значит
// детерминирован и сравним в корпусе (CANON §6).
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package nodeflow

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/registry"
)

// Warning — запись о снятом или приведённом поле (CANON §6).
type Warning = configtypes.Warning

// Result — исход санитайзера.
type Result struct {
	// Clean — карта с каноническими ключами и приведёнными типами. Эмиттер
	// работает только с ней, поэтому ассертов типов в нём быть не должно.
	Clean map[string]interface{}
	// Warnings — коды снятого и приведённого, в порядке обхода body.order.
	Warnings []Warning
	// Drop — узел не собирается (required / on_invalid drop_node). Остальные
	// поля Result при этом заполнены, но телу узла ходу нет.
	Drop *Warning
}

// maskedValue — то, что пишется в Warning.Value вместо секрета.
const maskedValue = "***"

// buildManagedKeys — ключи корня outbound'а, которые пишет СБОРКА конфига, а
// не тело узла: tag берётся из идентичности узла (SPEC 112), type — из его
// схемы. В body-секциях реестра их поэтому нет, и снимать их надо молча:
// unknown_key на них означал бы, что пользователю показывают ⚠ за работу
// самого лаунчера. detour управляется тем же правилом, но у него есть запись
// в реестре с managed:true, и его снимает общий обход.
var buildManagedKeys = map[string]bool{"tag": true, "type": true}

// sanitizer — состояние одного прохода. Хранит схему и накопители, чтобы не
// таскать их шестым аргументом через рекурсию.
type sanitizer struct {
	reg    *registry.Registry
	scheme string
	res    Result
	seen   map[string]bool // дедуп по (code, path)
	// srcRoot — исходная карта тела целиком, cleanRoot — уже собранная
	// чистая. Пути в conflicts/requires реестра пишутся ОТ КОРНЯ тела
	// ("tls.reality.public_key"), поэтому связи считаются по корню, а не по
	// соседям: поиск по последнему сегменту схлопнул бы tls.ech.enabled и
	// tls.reality.enabled в один «enabled» и снимал бы reality на ровном месте.
	srcRoot   map[string]interface{}
	cleanRoot map[string]interface{}
}

// Sanitize приводит карту тела узла к правилам реестра для схемы scheme.
//
// Неизвестная схема — не повод молча пропустить мусор: тело возвращается
// пустым с кодом уровня узла protocol_unsupported.
func Sanitize(scheme string, m map[string]interface{}) Result {
	reg, err := registry.Get()
	if err != nil {
		return Result{
			Clean: map[string]interface{}{},
			Drop:  &Warning{Code: "parse_error", Params: map[string]string{"error": err.Error()}},
		}
	}
	body, ok := reg.Body(scheme)
	if !ok {
		return Result{
			Clean: map[string]interface{}{},
			Drop: &Warning{
				Code:   "protocol_unsupported",
				Params: map[string]string{"scheme": scheme},
			},
		}
	}
	s := &sanitizer{reg: reg, scheme: scheme, seen: map[string]bool{}, srcRoot: m}
	s.cleanRoot = map[string]interface{}{}
	s.res.Clean = s.cleanRoot
	s.object("", body.Order, body.Fields, m)
	return s.res
}

// warn кладёт код в накопитель, снимая дубли по (code, path).
func (s *sanitizer) warn(code, path string, value interface{}, secret bool, params map[string]string) {
	if code == "" {
		return
	}
	key := code + "\x00" + path
	if s.seen[key] {
		return
	}
	s.seen[key] = true
	w := Warning{Code: code, Path: path, Params: params}
	if secret {
		w.Value = maskedValue
	} else if value != nil {
		w.Value = configtypes.TruncateWarningValue(displayValue(value))
	}
	s.res.Warnings = append(s.res.Warnings, w)
}

// dropNode фиксирует отказ по узлу. Первый отказ побеждает: дальнейший обход
// нужен только чтобы собрать остальные коды для диагностики.
func (s *sanitizer) dropNode(code, path string, value interface{}, secret bool, params map[string]string) {
	s.warn(code, path, value, secret, params)
	if s.res.Drop != nil {
		return
	}
	d := Warning{Code: code, Path: path, Params: params}
	if secret {
		d.Value = maskedValue
	} else if value != nil {
		d.Value = configtypes.TruncateWarningValue(displayValue(value))
	}
	s.res.Drop = &d
}

// object обходит один уровень карты по order реестра и возвращает чистую
// карту этого уровня.
func (s *sanitizer) object(prefix string, order []string, fields map[string]*registry.Field, src map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	if prefix == "" && s.cleanRoot != nil {
		out = s.cleanRoot
	}
	if src == nil {
		src = map[string]interface{}{}
	}
	known := make(map[string]bool, len(fields))
	for name := range fields {
		known[name] = true
	}

	for _, name := range order {
		f := fields[name]
		if f == nil {
			continue
		}
		path := joinPath(prefix, name)
		raw, present := src[name]

		// Поля, которые пишет сборка конфига, в теле узла не живут: detour
		// вычисляется из Направлений и цепочек. Снимаем молча — это не
		// ошибка источника (SPEC 131 §3.2).
		if f.Managed {
			continue
		}
		if !s.allowedForScheme(f) {
			if present {
				s.warn(codeOr(f.Code, "unknown_key"), path, raw, f.Secret, map[string]string{"path": path})
			}
			continue
		}
		if !present {
			if f.Required {
				s.dropNode("field_missing", path, nil, false, map[string]string{"field": path})
			}
			continue
		}
		v, ok := s.value(path, prefix, f, raw)
		if ok {
			out[name] = v
			continue
		}
		// Поле было, но не пережило проверки. Если оно обязательное,
		// узел собрать нельзя — код снятия уже поставлен, добавляем отказ
		// по узлу (on_invalid drop_node ставит его сам и побеждает как
		// первый).
		if f.Required && s.res.Drop == nil {
			s.dropNode("field_missing", path, nil, false, map[string]string{"field": path})
		}
	}

	// Всё, чего нет в fields, — вне схемы: ядро отвергнет ключ и не запустит
	// весь конфиг, поэтому снимаем с кодом.
	for _, name := range sortedKeys(src) {
		if known[name] || (prefix == "" && buildManagedKeys[name]) {
			continue
		}
		path := joinPath(prefix, name)
		s.warn("unknown_key", path, src[name], false, map[string]string{"path": path})
	}
	return out
}

// allowedForScheme — разрешено ли поле текущей схеме (allowed_for/forbidden_for).
func (s *sanitizer) allowedForScheme(f *registry.Field) bool {
	for _, sc := range f.ForbiddenFor {
		if sc == s.scheme {
			return false
		}
	}
	if len(f.AllowedFor) > 0 {
		for _, sc := range f.AllowedFor {
			if sc == s.scheme {
				return true
			}
		}
		return false
	}
	return true
}

// value обрабатывает одно поле: связи с соседями, приведение типа, проверки
// значения. Второй результат false — поле снято.
//
// siblings — исходная карта уровня (для conflicts/requires/forbidden_when по
// относительным путям), clean — уже собранная чистая карта того же уровня.
func (s *sanitizer) value(path, prefix string, f *registry.Field, raw interface{}) (interface{}, bool) {
	// Связи полей считаются до приведения типа: снятое поле не должно
	// тратить коды на собственный формат.
	if !s.relationsOK(path, prefix, f) {
		return nil, false
	}

	switch f.Type {
	case "object":
		return s.objectField(path, f, raw)
	case "array":
		return s.arrayField(path, f, raw)
	}

	v, ok := coerce(f, raw)
	if !ok {
		return s.onInvalid(path, f, raw)
	}
	if !s.constraintsOK(f, v) {
		return s.onInvalid(path, f, raw)
	}
	s.advisory(path, f, v)
	return v, true
}

// objectField обрабатывает вложенный объект: по варианту дискриминатора
// (transport), по описанным полям или как свободную карту (headers).
func (s *sanitizer) objectField(path string, f *registry.Field, raw interface{}) (interface{}, bool) {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return s.onInvalid(path, f, raw)
	}
	if len(f.Variants) > 0 {
		return s.variantObject(path, f, m)
	}
	if len(f.Fields) == 0 {
		// Свободная карта (headers, extra_headers): значения не описаны,
		// решать нечего — отдаём как есть.
		return m, true
	}
	out := s.object(path, f.Order, f.Fields, m)
	if f.AllOrNothing {
		s.fillAllOrNothing(path, f, out)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// variantObject выбирает вариант по значению дискриминатора и обходит его
// поля. Неизвестное значение = ядро не стартует, поэтому блок снимается
// целиком с кодом на самом дискриминаторе.
func (s *sanitizer) variantObject(path string, f *registry.Field, m map[string]interface{}) (interface{}, bool) {
	discPath := joinPath(path, f.Discriminator)
	rawDisc, present := m[f.Discriminator]
	if !present {
		s.warn("field_missing", discPath, nil, false, map[string]string{"field": discPath})
		return nil, false
	}
	disc, ok := rawDisc.(string)
	if !ok {
		s.warn("type_invalid", discPath, rawDisc, false, map[string]string{"path": discPath})
		return nil, false
	}
	variant := f.Variants[strings.TrimSpace(disc)]
	if variant == nil {
		s.warn("type_invalid", discPath, disc, false, map[string]string{"path": discPath})
		return nil, false
	}
	rest := make(map[string]interface{}, len(m))
	for k, v := range m {
		if k == f.Discriminator {
			continue
		}
		rest[k] = v
	}
	out := s.object(path, variant.Order, variant.Fields, rest)
	out[f.Discriminator] = strings.TrimSpace(disc)
	return out, true
}

// arrayField обходит массив объектов (wireguard.peers).
func (s *sanitizer) arrayField(path string, f *registry.Field, raw interface{}) (interface{}, bool) {
	list, ok := raw.([]interface{})
	if !ok || f.Items == nil {
		return s.onInvalid(path, f, raw)
	}
	out := make([]interface{}, 0, len(list))
	for i, item := range list {
		itemPath := path + "[" + strconv.Itoa(i) + "]"
		if len(f.Items.Fields) == 0 {
			out = append(out, item)
			continue
		}
		m, ok := item.(map[string]interface{})
		if !ok {
			s.warn("type_invalid", itemPath, item, false, map[string]string{"path": itemPath})
			continue
		}
		out = append(out, s.object(itemPath, f.Items.Order, f.Items.Fields, m))
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// fillAllOrNothing дописывает дефолты соседей у частично заданного объекта.
//
// Правило all_or_nothing: в ядре задание одного поля обнуляет дефолты
// остальных (transport.xmux). Молчаливое «как выглядит, так и работает»
// здесь неверно, поэтому недостающие дефолты пишутся явно.
func (s *sanitizer) fillAllOrNothing(path string, f *registry.Field, out map[string]interface{}) {
	if len(out) == 0 {
		return // секции нет — дефолты ядра в силе, дописывать нечего
	}
	changed := false
	for _, name := range f.Order {
		inner := f.Fields[name]
		if inner == nil || inner.Default == nil {
			continue
		}
		if _, ok := out[name]; ok {
			continue
		}
		v, ok := coerce(inner, inner.Default)
		if !ok {
			continue
		}
		out[name] = v
		changed = true
	}
	if changed {
		s.warn("partial_object_defaulted", path, nil, false, map[string]string{"path": path})
	}
}

// relationsOK проверяет conflicts / requires / forbidden_when.
//
// Конфликт снимает ТЕКУЩЕЕ поле (младшее по order): к моменту проверки
// старший сосед уже прошёл обход и лежит в clean.
func (s *sanitizer) relationsOK(path, prefix string, f *registry.Field) bool {
	for _, c := range f.Conflicts {
		if c.With == "" {
			continue
		}
		if !s.pathPresent(c.With, prefix) {
			continue
		}
		s.warn(codeOr(c.Code, "field_conflict"), path, nil, false,
			map[string]string{"path": path, "with": c.With})
		return false
	}
	for _, rq := range f.Requires {
		if rq.Path == "" {
			continue
		}
		if s.pathPresent(rq.Path, prefix) {
			continue
		}
		s.warn(codeOr(rq.Code, "field_requires"), path, nil, false,
			map[string]string{"path": path, "requires": rq.Path})
		return false
	}
	if fw := f.ForbiddenWhen; fw != nil && fw.Path != "" {
		want := true
		if fw.Present != nil {
			want = *fw.Present
		}
		if s.pathPresent(fw.Path, prefix) == want {
			s.warn(codeOr(fw.Code, "field_conflict"), path, nil, false,
				map[string]string{"path": path, "with": fw.Path})
			return false
		}
	}
	return true
}

// pathPresent — есть ли по пути непустое значение.
//
// Путь реестра пишется от корня тела ("tls.reality.public_key"); ищем его в
// исходной карте и в уже собранной чистой (второе важно для requires: к
// моменту проверки старшее поле обхода уже приведено). Запасной вариант —
// путь относительно текущего объекта: так в реестре записаны связи внутри
// вариантов транспорта ("transport.xmux.max_connections" при обходе
// transport.xmux).
func (s *sanitizer) pathPresent(path, prefix string) bool {
	if s.lookupNonEmpty(path) {
		return true
	}
	if prefix != "" {
		// "transport.mode" при обходе внутри transport — тот же ключ.
		if i := strings.Index(path, "."); i >= 0 {
			head := path[:i]
			if head == prefix || strings.HasSuffix(prefix, "."+head) {
				return s.lookupNonEmpty(joinPath(prefix, path[i+1:]))
			}
		}
		return s.lookupNonEmpty(joinPath(prefix, path))
	}
	return false
}

func (s *sanitizer) lookupNonEmpty(path string) bool {
	parts := strings.Split(path, ".")
	if v, ok := lookupPath(s.cleanRoot, parts); ok {
		return !isEmptyValue(v)
	}
	if v, ok := lookupPath(s.srcRoot, parts); ok {
		return !isEmptyValue(v)
	}
	return false
}

func lookupPath(m map[string]interface{}, parts []string) (interface{}, bool) {
	cur := interface{}(m)
	for _, p := range parts {
		obj, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur, ok = obj[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// advisory ставит информационный код на значении, которое ядро принимает, но
// человеку о нём знать стоит (legacy-шифры shadowsocks).
func (s *sanitizer) advisory(path string, f *registry.Field, v interface{}) {
	for _, a := range f.Advisory {
		for _, want := range a.Values {
			if sameValue(want, v) {
				params := map[string]string{"path": path}
				if !f.Secret {
					params["value"] = displayValue(v)
				}
				s.warn(a.Code, path, v, f.Secret, params)
				return
			}
		}
	}
}

// onInvalid исполняет правило on_invalid: снять, подставить или отбросить
// узел. Без правила — снять с type_invalid: значение, которое ядро отвергает
// фатально, в теле остаться не может (CANON §8).
func (s *sanitizer) onInvalid(path string, f *registry.Field, raw interface{}) (interface{}, bool) {
	oi := f.OnInvalid
	if oi == nil {
		s.warn("type_invalid", path, raw, f.Secret, map[string]string{"path": path})
		return nil, false
	}
	params := map[string]string{"path": path, "field": path}
	if !f.Secret {
		params["value"] = displayValue(raw)
	}
	switch oi.Action {
	case "coerce":
		v, ok := coerce(f, oi.Value)
		if !ok {
			s.warn(codeOr(oi.Code, "type_invalid"), path, raw, f.Secret, params)
			return nil, false
		}
		s.warn(codeOr(oi.Code, "type_invalid"), path, raw, f.Secret, params)
		return v, true
	case "drop_node":
		s.dropNode(codeOr(oi.Code, "type_invalid"), path, raw, f.Secret, params)
		return nil, false
	default: // drop
		s.warn(codeOr(oi.Code, "type_invalid"), path, raw, f.Secret, params)
		return nil, false
	}
}

// constraintsOK — enum, format, min/max, len, len_parity.
func (s *sanitizer) constraintsOK(f *registry.Field, v interface{}) bool {
	if len(f.Values) > 0 {
		switch vv := v.(type) {
		case []string:
			for _, item := range vv {
				if !inValues(f.Values, item) {
					return false
				}
			}
		case string, float64, int, int64, bool:
			if !inValues(f.Values, v) {
				return false
			}
		}
	}
	str, isStr := v.(string)
	if f.Format != "" {
		switch vv := v.(type) {
		case []string:
			for _, item := range vv {
				if !formatOK(f.Format, item) {
					return false
				}
			}
		default:
			if isStr && !formatOK(f.Format, str) {
				return false
			}
			if !isStr && !numericFormatOK(f.Format, vv) {
				return false
			}
		}
	}
	// min/max: у строк это длина, у чисел — значение (реестр так и
	// использует: short_id max:16 — длина, server_port max:65535 — значение).
	if f.Min != nil || f.Max != nil {
		var measure float64
		switch vv := v.(type) {
		case string:
			measure = float64(len(vv))
		case float64:
			measure = vv
		case int:
			measure = float64(vv)
		case int64:
			measure = float64(vv)
		case []string:
			measure = float64(len(vv))
		default:
			measure = 0
		}
		if f.Min != nil && measure < *f.Min {
			return false
		}
		if f.Max != nil && measure > *f.Max {
			return false
		}
	}
	if f.Len != nil {
		switch vv := v.(type) {
		case string:
			if len(vv) != *f.Len {
				return false
			}
		case []string:
			if len(vv) != *f.Len {
				return false
			}
		}
	}
	if f.LenParity != "" && isStr {
		odd := len(str)%2 == 1
		if (f.LenParity == "even" && odd) || (f.LenParity == "odd" && !odd) {
			return false
		}
	}
	return true
}

// codeOr — код правила или запасной, если реестр код не назвал.
func codeOr(code, fallback string) string {
	if code != "" {
		return code
	}
	return fallback
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// sortedKeys — детерминированный порядок для ключей вне схемы: карта в Go
// обходится случайно, а список warnings обязан быть сравним в корпусе.
func sortedKeys(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func isEmptyValue(v interface{}) bool {
	switch vv := v.(type) {
	case nil:
		return true
	case string:
		return vv == ""
	case bool:
		return !vv
	case float64:
		return vv == 0
	case int:
		return vv == 0
	case []interface{}:
		return len(vv) == 0
	case []string:
		return len(vv) == 0
	case map[string]interface{}:
		return len(vv) == 0
	}
	return false
}

func displayValue(v interface{}) string {
	switch vv := v.(type) {
	case string:
		return vv
	case float64:
		if vv == float64(int64(vv)) {
			return strconv.FormatInt(int64(vv), 10)
		}
		return strconv.FormatFloat(vv, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(vv)
	case []string:
		return strings.Join(vv, ",")
	}
	return fmt.Sprintf("%v", v)
}

func sameValue(want, got interface{}) bool {
	return displayValue(want) == displayValue(got)
}

func inValues(values []interface{}, v interface{}) bool {
	got := displayValue(v)
	for _, want := range values {
		if displayValue(want) == got {
			return true
		}
	}
	return false
}

// formatOK — проверка строкового формата. Неизвестный формат пропускается:
// реестр может уехать вперёд кода, и молчаливый отказ был бы хуже.
func formatOK(format, v string) bool {
	switch format {
	case "uuid":
		return uuidOK(v)
	case "hex":
		if v == "" {
			return true
		}
		_, err := hex.DecodeString(v)
		return err == nil
	case "base64":
		if v == "" {
			return true
		}
		if _, err := base64.StdEncoding.DecodeString(v); err == nil {
			return true
		}
		if _, err := base64.RawURLEncoding.DecodeString(v); err == nil {
			return true
		}
		_, err := base64.RawStdEncoding.DecodeString(v)
		return err == nil
	case "host":
		return v != "" && !strings.ContainsAny(v, " \t\r\n/")
	case "port":
		n, err := strconv.Atoi(v)
		return err == nil && n >= 1 && n <= 65535
	case "ipv4":
		ip := net.ParseIP(v)
		return ip != nil && ip.To4() != nil
	case "cidr":
		if _, _, err := net.ParseCIDR(v); err == nil {
			return true
		}
		return net.ParseIP(v) != nil
	}
	return true
}

func numericFormatOK(format string, v interface{}) bool {
	if format != "port" {
		return true
	}
	switch vv := v.(type) {
	case int:
		return vv >= 1 && vv <= 65535
	case int64:
		return vv >= 1 && vv <= 65535
	case float64:
		return vv >= 1 && vv <= 65535
	}
	return true
}

func uuidOK(v string) bool {
	if len(v) != 36 {
		return false
	}
	for i, c := range v {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !isHexDigit(byte(c)) {
				return false
			}
		}
	}
	return true
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
