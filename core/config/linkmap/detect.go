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
	"sort"
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
	// iniFirst — имя первой не-комментарной секции, в нижнем регистре.
	// Порядок секций теряется в карте asINI, а предикату он нужен.
	iniFirst string
}

// bomPrefix — маркер порядка байтов U+FEFF. Записан escape-последовательностью
// намеренно: литерал BOM вне первого символа файла Go отвергает.
const bomPrefix = "\uFEFF"

// NewContent — контент из сырого текста.
//
// Написание схемы вынимается ЗДЕСЬ, а не оставляется пустым: без него
// предикат `scheme_in` не срабатывал ни разу, и секции протоколов не
// опознавали собственные ссылки — уровень документа вынужден был бы выбирать
// схему по префиксу, то есть кодом, знающим имена схем.
//
// Здесь же снимается BOM, и ровно один раз: невидимый префикс U+FEFF сдвигает
// первый значащий символ, и документ, начинающийся с `{` или `[Interface]`,
// перестаёт быть таковым сразу для ВСЕХ предикатов — JSON уезжает в список
// ссылок, `.conf` туда же. Место снятия одно, потому что BOM не признак
// формата, а мусор кодировки файла (DELTAS D133-43).
func NewContent(text string) *Content {
	text = strings.TrimPrefix(text, bomPrefix)
	return &Content{text: text, scheme: schemeOfText(text)}
}

// schemeOfText — написание схемы ссылки, как оно стоит в тексте.
//
// Только форма `scheme://`: у `mailto:`-подобной формы без слэшей узлов не
// бывает, и принимать её значило бы опознавать схемой произвольный текст с
// двоеточием.
func schemeOfText(text string) string {
	t := strings.TrimSpace(text)
	i := strings.Index(t, "://")
	if i <= 0 {
		return ""
	}
	for j := 0; j < i; j++ {
		c := t[j]
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.'
		if !ok {
			return ""
		}
	}
	return strings.ToLower(t[:i])
}

// SchemeOfText — схема ссылки формы `scheme://` в нижнем регистре; "" —
// текст этой формы не имеет. Слою подписки нужна та же граница, что у
// движка: отказ `scheme_unsupported` ставится только строке `xxx://`
// (PARSING_PRINCIPLES §4.1), прочий непрочитанный текст — `form_unrecognized`.
func SchemeOfText(text string) string { return schemeOfText(text) }

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
			c.iniFirst = firstINISection(trimmed)
		}
	})
}

// firstINISection — имя первой не-комментарной секции ini, в нижнем регистре.
//
// Комментарии над первой секцией законны и несут имя узла, поэтому строки с
// маркером комментария пропускаются; первая же непустая строка, не являющаяся
// секцией, означает, что секции впереди нет — текст не ini.
func firstINISection(text string) string {
	var d *registry.IniDialect
	prefixes := d.Prefixes()
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || hasAnyPrefix(line, prefixes) {
			continue
		}
		if strings.HasPrefix(line, "[") {
			return strings.ToLower(strings.Trim(line, "[]"))
		}
		return ""
	}
	return ""
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
		if d.INI.FirstSectionFold != "" &&
			!strings.EqualFold(c.iniFirst, strings.TrimSpace(d.INI.FirstSectionFold)) {
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
	if t.PrefixTrim != "" {
		trimmed := strings.TrimSpace(text)
		if len(trimmed) < len(t.PrefixTrim) || trimmed[:len(t.PrefixTrim)] != t.PrefixTrim {
			return false
		}
	}
	if t.MinLen > 0 && len(strings.TrimSpace(text)) < t.MinLen {
		return false
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
//
// Сегментов `[]` в пути может быть несколько: "[].outbounds[].protocol"
// означает «в массиве-документе есть конфиг, у которого в outbounds есть
// элемент с protocol» — так вид источника «массив Xray-конфигов» отличается
// от «массив outbound-ов sing-box», не заводя второго примитива.
func matchArrayElem(v interface{}, paths []string) bool {
	for _, p := range paths {
		if pathExistsAny(v, p) {
			return true
		}
	}
	return false
}

// pathExistsAny — есть ли по пути хотя бы одно значение. Сегмент `[]`
// означает «любой элемент массива» и разветвляет обход.
func pathExistsAny(v interface{}, path string) bool {
	arrPath, elemPath, split := cutArrayPath(path)
	if !split {
		if path == "" {
			return v != nil
		}
		_, ok := lookupPath(v, path)
		return ok
	}
	node := v
	if arrPath != "" {
		found, ok := lookupPath(v, arrPath)
		if !ok {
			return false
		}
		node = found
	}
	arr, ok := node.([]interface{})
	if !ok {
		return false
	}
	for _, elem := range arr {
		if elemPath == "" {
			return true
		}
		if pathExistsAny(elem, elemPath) {
			return true
		}
	}
	return false
}

// cutArrayPath делит путь по ПЕРВОМУ "[]": "outbounds[].protocol" даёт
// ("outbounds", "protocol", true). Пустое имя массива означает сам документ
// (форма "[].outbounds[]" для тела-массива). Путь без "[]" — ("", "", false).
func cutArrayPath(p string) (string, string, bool) {
	idx := strings.Index(p, "[]")
	if idx < 0 {
		return "", "", false
	}
	arr := strings.TrimSuffix(p[:idx], ".")
	elem := strings.TrimPrefix(p[idx+2:], ".")
	return arr, elem, true
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
	// "$root" — САМ документ, а не ключ в нём. Нужен предикатам о форме
	// документа целиком: `type_of: {"$root": "object"}` отличает объект
	// v2rayN у vmess от cleartext-строки, и выразить это именем ключа
	// нельзя — вопрос не «есть ли поле», а «что это вообще такое».
	if strings.TrimSpace(path) == "$root" {
		if cur == nil {
			return nil, false
		}
		return cur, true
	}
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

// equalFoldValue сравнивает значение по пути с ожиданием предиката.
//
// Ожидание — скаляр JSON, и ТИП ЗНАЧИМ: строка сходится только со строкой
// (регистронезависимо, с обрезкой пробелов), число только с числом, булево
// только с булевым. Приведение типов друг к другу здесь запрещено, потому
// что диалекты пишут скаляры по-разному, и «2» строкой означает не то же,
// что 2 числом: прежний конвертер строковую версию за двойку не считал
// (xrayJSONInt читает только числа) и вёл такой элемент в v1.
func equalFoldValue(got interface{}, want interface{}) bool {
	switch w := want.(type) {
	case string:
		s, ok := got.(string)
		if !ok {
			return false
		}
		return strings.EqualFold(strings.TrimSpace(s), w)
	case float64:
		n, ok := got.(float64)
		return ok && n == w
	case bool:
		b, ok := got.(bool)
		return ok && b == w
	}
	return false
}

func containsFold(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

// parseINI разбирает ini-текст диалектом ПО УМОЛЧАНИЮ.
//
// Зовётся там, где секции ещё нет и спросить диалект не у кого: предикат
// `detect` выбирает саму секцию, а до выбора её атрибуты недоступны. Для
// опознания этого довольно — предикат смотрит на имена секций и ключей,
// которые во всех диалектах одни.
func parseINI(text string) (map[string]map[string]string, bool) {
	sections, _, ok := parseINIDialect(text, nil)
	return sections, ok
}

// iniRepeat — сколько раз встретилась секция с этим именем.
type iniRepeat struct {
	Name  string
	Count int
}

// parseINIDialect разбирает ini-текст правилами объявленного диалекта.
//
// Второй результат — секции, чьи ПОВТОРЫ отброшены правилом `repeat`,
// вместе с общим числом вхождений: из него запись `on_extra` делает код с
// параметром `count`. Пустой диалект (nil) = сегодняшний разбор wg-quick
// дословно: ключи lower, значения as-is, комментарии `#`/`;` только целой
// строкой, повторный ключ — последний выигрывает, секции сливаются.
func parseINIDialect(text string, d *registry.IniDialect) (map[string]map[string]string, []iniRepeat, bool) {
	if !strings.Contains(text, "[") {
		return nil, nil, false
	}
	prefixes := d.Prefixes()
	keepKeyCase := d != nil && d.KeyCase == "preserve"
	lowerValue := d != nil && d.ValueCase == "lower"
	inline := d != nil && d.InlineComments
	repeatedKey := "last_wins"
	if d != nil && d.RepeatedKey != "" {
		repeatedKey = d.RepeatedKey
	}

	out := map[string]map[string]string{}
	counts := map[string]int{}
	// closed — секции, добор ключей в которые запрещён правилом
	// `repeat: first_only`: имя уже встречалось, и первое вхождение
	// объявлено единственным.
	closed := map[string]bool{}
	section := ""
	any := false

	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || hasAnyPrefix(line, prefixes) {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			counts[section]++
			if _, ok := out[section]; !ok {
				out[section] = map[string]string{}
			} else if rule := d.Section(section); rule != nil && rule.Repeat == "first_only" {
				closed[section] = true
			}
			any = true
			continue
		}
		if section == "" || closed[section] {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		if !keepKeyCase {
			key = strings.ToLower(key)
		}
		val := strings.TrimSpace(line[idx+1:])
		if inline {
			val = strings.TrimSpace(cutInlineComment(val, prefixes))
		}
		if lowerValue {
			val = strings.ToLower(val)
		}
		if key == "" || val == "" {
			continue
		}
		if prev, seen := out[section][key]; seen {
			switch repeatedKey {
			case "first_wins":
				continue
			case "append":
				val = prev + "," + val
			}
		}
		out[section][key] = val
	}

	var dropped []iniRepeat
	for name, n := range counts {
		if n > 1 && closed[name] {
			dropped = append(dropped, iniRepeat{Name: name, Count: n})
		}
	}
	sort.Slice(dropped, func(i, j int) bool { return dropped[i].Name < dropped[j].Name })
	return out, dropped, any
}

// hasAnyPrefix — начинается ли строка с любого из маркеров.
func hasAnyPrefix(line string, prefixes []string) bool {
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// cutInlineComment режет хвост-комментарий после значения.
//
// Нужен только диалектам, объявившим `inline_comments`: у wg-quick '#' —
// законный символ значения («US-FREE#137»), и резать его нельзя.
func cutInlineComment(val string, prefixes []string) string {
	cut := len(val)
	for _, p := range prefixes {
		if p == "" {
			continue
		}
		if i := strings.Index(val, p); i >= 0 && i < cut {
			cut = i
		}
	}
	return val[:cut]
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
