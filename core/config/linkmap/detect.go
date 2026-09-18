// Package linkmap — движок, ИСПОЛНЯЮЩИЙ секции-мапперы реестра (SPEC 133).
//
// Один движок на все виды источника: ссылка, Xray-JSON, sing-box-JSON,
// wg-quick ini. Таблицы разные (`mappers.uri` / `.xray` / `.singbox` /
// `.conf`), код — общий: ни одной функции, названной по схеме, протоколу или
// транспорту, и ни одного `if scheme ==`. Это критерий конца кампании
// (SPEC 133 §2.1), а не пожелание к стилю.
//
// Инвариант, обеспеченный КОНСТРУКТИВНО: значение доступно параметру только
// через `source`. Сырых url.Values/карты JSON у исполнителя таблицы нет,
// поэтому дефект «поле объявлено в реестре, но код его не читает» здесь
// невозможен — в отличие от рукописных парсеров, где он случился пять раз
// (SPEC 133 §1.2).
//
// go1.20-совместимо (Win7-джоба собирает весь модуль тулчейном go1.20):
// без slices/maps/min/max/clear.
package linkmap

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"

	"singbox-launcher/core/config/registry"
)

// Content — контент, который предъявляется предикату `detect`.
//
// Один тип на оба уровня (документ и элемент): предикаты исполняются
// одинаково, различается только то, что в нём заполнено. Поля разбираются
// ЛЕНИВО — детект документа гоняет десяток предикатов по одному и тому же
// тексту, и разбирать JSON заново на каждом было бы расточительно.
type Content struct {
	text string

	// scheme — написание схемы ссылки (для detect.scheme_in).
	scheme string
	// inArray — имя массива документа, из которого пришёл элемент.
	inArray string

	once   sync.Once
	asJSON interface{}
	asINI  map[string]map[string]string
	iniOK  bool
}

// NewContent — контент из сырого текста.
func NewContent(text string) *Content {
	return &Content{text: text}
}

// NewElementContent — контент одного элемента документа: разобранное
// значение JSON плюс сведения об уровне, откуда он взят.
func NewElementContent(value interface{}, scheme, inArray string) *Content {
	c := &Content{scheme: scheme, inArray: inArray, asJSON: value}
	c.once.Do(func() {})
	return c
}

// Text — сырой текст контента.
func (c *Content) Text() string {
	if c == nil {
		return ""
	}
	return c.text
}

// parse разбирает текст как JSON и как ini — по одному разу на контент.
func (c *Content) parse() {
	c.once.Do(func() {
		trimmed := strings.TrimSpace(c.text)
		if trimmed != "" {
			switch trimmed[0] {
			case '{', '[':
				var v interface{}
				if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
					c.asJSON = v
				}
			}
			c.asINI, c.iniOK = parseINI(trimmed)
		}
	})
}

// JSON — разобранное значение (nil, если текст не JSON).
func (c *Content) JSON() interface{} {
	if c == nil {
		return nil
	}
	c.parse()
	return c.asJSON
}

// INI — разобранные секции ini.
func (c *Content) INI() (map[string]map[string]string, bool) {
	if c == nil {
		return nil, false
	}
	c.parse()
	return c.asINI, c.iniOK
}

// Matches исполняет предикат `detect` над контентом.
//
// Ветка `default` НЕ проверяется здесь: «всё остальное» — свойство выбора
// среди кандидатов, а не свойство контента. Вызывающий (Select) сам решает,
// когда к ней переходить, иначе default-запись матчила бы всё подряд и
// выигрывала по priority у настоящих предикатов.
func Matches(d *registry.Detect, c *Content) bool {
	if d == nil || c == nil {
		return false
	}
	if d.Default {
		return false
	}

	if d.Regex != "" {
		re, err := compileShared(d.Regex)
		if err != nil || !re.MatchString(c.Text()) {
			return false
		}
	}
	if d.Text != nil && !matchText(d.Text, c.Text()) {
		return false
	}
	if len(d.SchemeIn) > 0 && !containsFold(d.SchemeIn, c.scheme) {
		return false
	}
	if d.InArray != "" && !strings.EqualFold(d.InArray, c.inArray) {
		return false
	}
	if d.JSON != nil && !matchJSON(d.JSON, c.JSON()) {
		return false
	}
	if d.INI != nil {
		sections, ok := c.INI()
		if !ok || !matchINI(d.INI, sections) {
			return false
		}
	}
	if d.Not != nil && Matches(d.Not, c) {
		return false
	}
	for i := range d.All {
		if !Matches(&d.All[i], c) {
			return false
		}
	}
	if len(d.Any) > 0 {
		hit := false
		for i := range d.Any {
			if Matches(&d.Any[i], c) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

func matchText(t *registry.DetectText, text string) bool {
	if t.PrefixFold != "" {
		if len(text) < len(t.PrefixFold) || !strings.EqualFold(text[:len(t.PrefixFold)], t.PrefixFold) {
			return false
		}
	}
	if t.Contains != "" && !strings.Contains(text, t.Contains) {
		return false
	}
	if t.LineFold != "" {
		found := false
		for _, line := range strings.Split(text, "\n") {
			if strings.EqualFold(strings.TrimSpace(strings.TrimRight(line, "\r")), t.LineFold) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func matchJSON(j *registry.DetectJSON, v interface{}) bool {
	if v == nil {
		return false
	}
	for _, p := range j.RequiredKeys {
		if _, ok := lookupPath(v, p); !ok {
			return false
		}
	}
	if len(j.AnyKeys) > 0 {
		hit := false
		for _, p := range j.AnyKeys {
			if _, ok := lookupPath(v, p); ok {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	for _, p := range j.KeyAbsent {
		if _, ok := lookupPath(v, p); ok {
			return false
		}
	}
	for p, want := range j.TypeOf {
		got, ok := lookupPath(v, p)
		if !ok || jsonTypeOf(got) != want {
			return false
		}
	}
	for p, want := range j.ValueOf {
		got, ok := lookupPath(v, p)
		if !ok || !equalFoldValue(got, want) {
			return false
		}
	}
	for p, want := range j.ValueIn {
		got, ok := lookupPath(v, p)
		if !ok {
			return false
		}
		hit := false
		for _, w := range want {
			if equalFoldValue(got, w) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if len(j.ArrayElemAnyKeys) > 0 && !matchArrayElem(v, j.ArrayElemAnyKeys) {
		return false
	}
	return true
}

// matchArrayElem — хотя бы один элемент массива несёт путь.
//
// Путь называет массив явно, через `[]`: "outbounds[].protocol" означает
// «в массиве outbounds есть элемент с ключом protocol». Без `[]` массивом
// считается сам документ — форма для тела-массива.
//
// Так Xray-конфиг отличается от sing-box: элемент Xray несёт
// outbounds[].protocol, и без этой проверки он уехал бы в ветку массива
// конфигов, где разбор не нашёл бы ни одного "type".
func matchArrayElem(v interface{}, paths []string) bool {
	for _, p := range paths {
		arrPath, elemPath := splitArrayPath(p)
		node := v
		if arrPath != "" {
			found, ok := lookupPath(v, arrPath)
			if !ok {
				continue
			}
			node = found
		}
		arr, ok := node.([]interface{})
		if !ok {
			continue
		}
		for _, elem := range arr {
			if elemPath == "" {
				return true
			}
			if _, ok := lookupPath(elem, elemPath); ok {
				return true
			}
		}
	}
	return false
}

// splitArrayPath делит "outbounds[].protocol" на ("outbounds", "protocol").
// Путь без "[]" даёт ("", path): массив — сам документ.
func splitArrayPath(p string) (string, string) {
	idx := strings.Index(p, "[]")
	if idx < 0 {
		return "", p
	}
	arr := strings.TrimSuffix(p[:idx], ".")
	elem := strings.TrimPrefix(p[idx+2:], ".")
	return arr, elem
}

func matchINI(spec *registry.DetectINI, sections map[string]map[string]string) bool {
	for _, want := range spec.Sections {
		if _, ok := sections[strings.ToLower(want)]; !ok {
			return false
		}
	}
	for _, key := range spec.Keys {
		if !iniHasKey(sections, key) {
			return false
		}
	}
	if len(spec.KeysAny) > 0 {
		hit := false
		for _, key := range spec.KeysAny {
			if iniHasKey(sections, key) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

func iniHasKey(sections map[string]map[string]string, key string) bool {
	want := strings.ToLower(strings.TrimSpace(key))
	for _, kv := range sections {
		if _, ok := kv[want]; ok {
			return true
		}
	}
	return false
}

// lookupPath достаёт значение по точечному пути ("settings.vnext.0.users").
//
// Числовой сегмент индексирует массив: путь `outbounds.0.protocol` работает
// так же, как объектный, — чтобы таблица не различала два вида адресации.
func lookupPath(v interface{}, path string) (interface{}, bool) {
	cur := v
	for _, seg := range strings.Split(path, ".") {
		if seg == "" {
			continue
		}
		switch node := cur.(type) {
		case map[string]interface{}:
			next, ok := node[seg]
			if !ok {
				return nil, false
			}
			cur = next
		case []interface{}:
			idx, ok := atoiStrict(seg)
			if !ok || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	if cur == nil {
		return nil, false
	}
	return cur, true
}

func atoiStrict(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

func jsonTypeOf(v interface{}) string {
	switch v.(type) {
	case map[string]interface{}:
		return "object"
	case []interface{}:
		return "array"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	}
	return ""
}

func equalFoldValue(got interface{}, want string) bool {
	s, ok := got.(string)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(s), want)
}

func containsFold(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

// parseINI разбирает ini-текст в секции с ключами в нижнем регистре.
//
// Диалект повторяет сегодняшний разбор wg-quick дословно (ключи lower,
// значения as-is, комментарии `#`/`;` только целой строкой, повторный ключ —
// последний выигрывает), потому что кампания переносит РЕШЕНИЯ в данные, а
// не меняет поведение. Отличия диалекта объявляются в секции (`ini_dialect`),
// а не появляются здесь сами собой.
func parseINI(text string) (map[string]map[string]string, bool) {
	if !strings.Contains(text, "[") {
		return nil, false
	}
	out := map[string]map[string]string{}
	section := ""
	any := false
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			if _, ok := out[section]; !ok {
				out[section] = map[string]string{}
			}
			any = true
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 || section == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		val := strings.TrimSpace(line[idx+1:])
		if key == "" || val == "" {
			continue
		}
		out[section][key] = val
	}
	return out, any
}

var (
	reCacheMu sync.Mutex
	reCache   = map[string]*regexp.Regexp{}
)

// compileShared компилирует регулярку реестра с кэшем.
//
// Диалект — общее подмножество Go RE2 и ECMAScript: линтер отдельно
// проверяет, что выражение валидно у ОБЕИХ сторон, потому что «скомпилировалось
// в Go» не означает «прочтёт Dart».
func compileShared(expr string) (*regexp.Regexp, error) {
	reCacheMu.Lock()
	defer reCacheMu.Unlock()
	if re, ok := reCache[expr]; ok {
		return re, nil
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, err
	}
	reCache[expr] = re
	return re, nil
}
