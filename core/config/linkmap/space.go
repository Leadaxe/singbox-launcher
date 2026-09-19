package linkmap

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Space — пространство источников: единственное, что видит таблица.
//
// Форма (`forms[]`) распаковывает текст СЮДА, и дальше запись таблицы
// адресует значение строкой `source`. Сырых url.Values/карты JSON у
// исполнителя записи нет — поэтому «поле объявлено, но не читается»
// невозможно по построению (SPEC 133 §1.2).
//
// Имена источников:
//
//	scheme    authority   userinfo[.user|.pass]   host   port   port_raw
//	path      fragment    query.<name>            json.<path>   ini.<S>.<K>
//	ini.$comment.<S>
type Space struct {
	Scheme    string
	Authority string
	UserInfo  string
	UserName  string
	UserPass  string
	Host      string
	Port      int
	// PortRaw — СЫРАЯ строка порта до приведения к int. Из неё читается
	// multi-port hysteria ("443,20000-30000"), который в Port не помещается,
	// и на котором платформенный парсер URL у Dart отказывает целиком.
	PortRaw  string
	Path     string
	Fragment string

	// Hint — имя узла, известное ВЫЗЫВАЮЩЕМУ, но не написанное во входе:
	// описание профиля `vpn://`, имя контейнера, имя файла. Звено `hint`
	// цепочки `label` читает именно его.
	Hint string

	// query — значения с сохранённым ПОРЯДКОМ появления: при двух написаниях
	// одного имени побеждает точное совпадение с каноном, иначе первое по
	// порядку. url.Values этого дать не может (Go-map), и сегодняшний
	// queryGetFold из-за этого недетерминирован.
	query []kv

	json interface{}
	ini  map[string]map[string]string
	// overlays — ДОПОЛНИТЕЛЬНЫЕ пространства, распакованные из значения
	// внутри входа (SPEC 133, примитив `overlay`): чужой диалект приезжает
	// вложенным слоем — JSON в query-параметре, — и адресуется тем же
	// `source`, только под своим префиксом (`extra.scMaxEachPostBytes`).
	//
	// Отдельная карта, а не слияние в query: слой ИМЕННО отдельный, и
	// запись сама решает, кто из двух побеждает, перечисляя источники в
	// нужном порядке. Слияние приняло бы это решение за неё и одинаково для
	// всех ключей — ровно то, на чём горит базовая тройка mode/path/host,
	// где Xray затирает вложенный слой плоским (D-097).
	overlays map[string]map[string]string
	// iniComments — первый комментарий секции: имя узла в .conf живёт под
	// [Peer] и больше нигде (G7).
	iniComments map[string]string

	// iniDropped — секции, чьи ПОВТОРЫ снял диалект (`repeat: first_only`),
	// с общим числом вхождений. Решение принимает разборщик ini, а код
	// ставит исполнитель записи `on_extra` — потеря едет отсюда туда.
	iniDropped []iniRepeat
}

type kv struct {
	key string
	val string
}

// SetJSON кладёт разобранное значение JSON в пространство.
func (s *Space) SetJSON(v interface{}) { s.json = v }

// SetINI кладёт разобранные секции ini и комментарии секций.
func (s *Space) SetINI(sections map[string]map[string]string, comments map[string]string) {
	s.ini = sections
	s.iniComments = comments
}

// AddQuery добавляет параметр, сохраняя порядок появления.
func (s *Space) AddQuery(key, val string) { s.query = append(s.query, kv{key: key, val: val}) }

// Lookup — значение по имени источника.
//
// Второе возвращаемое — «источник присутствует». Пустая строка при true
// означает пустое значение: различие нужно, потому что «пусто = отсутствует»
// — политика записи (`empty`), а не свойство пространства.
func (s *Space) Lookup(name string) (string, bool) {
	if s == nil || name == "" {
		return "", false
	}
	switch {
	case name == "scheme":
		return s.Scheme, s.Scheme != ""
	case name == "authority":
		return s.Authority, s.Authority != ""
	case name == "userinfo":
		return s.UserInfo, s.UserInfo != ""
	case name == "userinfo.user":
		return s.UserName, s.UserName != ""
	case name == "userinfo.pass":
		return s.UserPass, s.UserPass != ""
	case name == "host":
		return s.Host, s.Host != ""
	case name == "port":
		if s.Port == 0 {
			return "", false
		}
		return strconv.Itoa(s.Port), true
	case name == "port_raw":
		return s.PortRaw, s.PortRaw != ""
	case name == "path":
		return s.Path, s.Path != ""
	case name == "fragment":
		return s.Fragment, s.Fragment != ""
	case name == "hint":
		// Имя ОТ ВЫЗЫВАЮЩЕГО: у входа-файла метки внутри может не быть
		// вовсе, а снаружи она известна (описание профиля `vpn://`, имя
		// контейнера, имя файла). Источник НЕОБЯЗАТЕЛЬНЫЙ — не передали,
		// и звено цепочки просто пропускается.
		return s.Hint, s.Hint != ""
	case strings.HasPrefix(name, "query."):
		return s.queryValue(strings.TrimPrefix(name, "query."))
	case strings.HasPrefix(name, "json."):
		return jsonScalar(s.json, strings.TrimPrefix(name, "json."))
	case strings.HasPrefix(name, "ini."):
		return s.iniValue(strings.TrimPrefix(name, "ini."))
	}
	// Наложенный слой: "<имя слоя>.<ключ>". Проверяется ПОСЛЕ встроенных имён,
	// чтобы слой не мог перекрыть `host`/`port`/`query.*`.
	if idx := strings.Index(name, "."); idx > 0 {
		if layer, ok := s.overlays[name[:idx]]; ok {
			return mapGetFold(layer, name[idx+1:])
		}
	}
	return "", false
}

// SetOverlay кладёт наложенное пространство под своим именем.
func (s *Space) SetOverlay(name string, values map[string]string) {
	if s.overlays == nil {
		s.overlays = map[string]map[string]string{}
	}
	s.overlays[name] = values
}

// mapGetFold читает ключ плоской карты регистронезависимо, точное совпадение
// первым: подписки шлют и camelCase, и snake_case одного и того же имени.
func mapGetFold(m map[string]string, key string) (string, bool) {
	if v, ok := m[key]; ok && strings.TrimSpace(v) != "" {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(k, key) && strings.TrimSpace(v) != "" {
			return v, true
		}
	}
	return "", false
}

// LookupRaw — значение без приведения к строке (объекты и массивы JSON).
// Нужен записям с `list`, `coerce`, `flatten`: у них источник не скаляр.
func (s *Space) LookupRaw(name string) (interface{}, bool) {
	if s == nil {
		return nil, false
	}
	if strings.HasPrefix(name, "json.") {
		return lookupPath(s.json, strings.TrimPrefix(name, "json."))
	}
	v, ok := s.Lookup(name)
	if !ok {
		return nil, false
	}
	return v, true
}

// queryValue реализует норму приоритета написаний (PRIMITIVES §15.5):
// сперва ТОЧНОЕ совпадение с каноном, затем первое по порядку среди
// регистронезависимых совпадений.
//
// Без этого правила при `?sni=a&SNI=b` победитель зависел бы от порядка
// обхода Go-map, то есть менялся бы между запусками.
func (s *Space) queryValue(name string) (string, bool) {
	for _, p := range s.query {
		if p.key == name {
			return p.val, true
		}
	}
	for _, p := range s.query {
		if strings.EqualFold(p.key, name) {
			return p.val, true
		}
	}
	return "", false
}

// QueryNames — имена параметров в порядке появления, для кода
// `uri_param_unknown`: движок обязан назвать то, чего не знает.
// QueryValues — параметры пространства в виде url.Values.
//
// Нужны вызывающему за пределами разбора: skip-фильтры подписки и UI
// смотрят «что было написано в ссылке». Брать их повторным разбором
// ИСХОДНОГО текста нельзя — у обёрнутых форм (base64-пейлоад hysteria2,
// контейнер vmess) снаружи нет ни одного параметра, и справка выходила бы
// пустой там, где ссылка их несёт.
//
// Порядок появления теряется (url.Values — карта): это справка, а не вход
// разбора, и правило «первое по порядку» применяет сам движок через Lookup.
func (s *Space) QueryValues() url.Values {
	out := url.Values{}
	if s == nil {
		return out
	}
	for _, p := range s.query {
		out.Add(p.key, p.val)
	}
	return out
}

func (s *Space) QueryNames() []string {
	if s == nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(s.query))
	for _, p := range s.query {
		low := strings.ToLower(p.key)
		if seen[low] {
			continue
		}
		seen[low] = true
		out = append(out, p.key)
	}
	return out
}

func (s *Space) iniValue(rest string) (string, bool) {
	// ini.$comment.<Section> — имя узла из комментария секции (G7).
	if strings.HasPrefix(rest, "$comment.") {
		section := strings.ToLower(strings.TrimPrefix(rest, "$comment."))
		v, ok := s.iniComments[section]
		return v, ok && v != ""
	}
	idx := strings.Index(rest, ".")
	if idx < 0 {
		return "", false
	}
	section := strings.ToLower(rest[:idx])
	key := strings.ToLower(rest[idx+1:])
	kv, ok := s.ini[section]
	if !ok {
		return "", false
	}
	v, ok := kv[key]
	return v, ok && v != ""
}

// jsonScalar приводит значение JSON к строке.
//
// Массив и объект НЕ приводятся (в отличие от сегодняшнего xrayMapString,
// который делает fmt.Sprint и превращает ["a.com"] в "[a.com]" — живой баг
// httpSettings.host). Записи, которым нужен не скаляр, берут LookupRaw.
func jsonScalar(root interface{}, path string) (string, bool) {
	v, ok := lookupPath(root, path)
	if !ok {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case bool:
		if t {
			return "true", true
		}
		return "false", true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	}
	return "", false
}

// INISections — имена разобранных секций, по алфавиту (линтеру и диагностике).
func (s *Space) INISections() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.ini))
	for k := range s.ini {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SplitAuthority режет authority СВОИМ лексером, а не url.Parse.
//
// Норма кампании: платформенный парсер URL на `host:443,20000-30000`
// (multi-port hysteria) у Dart отказывает целиком, поэтому сырой порт обязан
// доезжать до таблицы как строка. Возвращает (userinfo, host, portRaw).
func SplitAuthority(authority string) (string, string, string) {
	userinfo := ""
	rest := authority
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		userinfo = authority[:at]
		rest = authority[at+1:]
	}

	host := rest
	portRaw := ""
	if strings.HasPrefix(rest, "[") {
		// IPv6 в скобках: порт — всё после скобки.
		if end := strings.Index(rest, "]"); end >= 0 {
			host = rest[:end+1]
			if tail := rest[end+1:]; strings.HasPrefix(tail, ":") {
				portRaw = tail[1:]
			}
		}
	} else if idx := strings.LastIndex(rest, ":"); idx >= 0 {
		// Голый IPv6 без скобок несёт несколько ':' — портом это не является
		// (G7). Порт берём только когда двоеточие ОДНО.
		if strings.Count(rest, ":") == 1 {
			host = rest[:idx]
			portRaw = rest[idx+1:]
		}
	}
	return userinfo, host, portRaw
}

// PortOf — числовой порт из сырой строки; 0, если строка не число целиком
// (multi-port "443,20000-30000" читается таблицей из port_raw).
func PortOf(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 || n > 65535 {
		return 0
	}
	return n
}

// ParseQueryOrdered разбирает сырую query, СОХРАНЯЯ порядок и НЕ превращая
// `+` в пробел на уровне пространства.
//
// Политика `+` — свойство записи (`decode_extra.plus_literal`, выводимое из
// `format: base64*`), а не разбора: здесь значение остаётся percent-декодированным
// с сохранённым `+`, а замену на пробел делает применение записи. Иначе
// решение о `+` пришлось бы принимать до того, как известно, какое это поле.
func ParseQueryOrdered(raw string) []kv {
	var out []kv
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		key := part
		val := ""
		if idx := strings.Index(part, "="); idx >= 0 {
			key = part[:idx]
			val = part[idx+1:]
		}
		if k, err := url.PathUnescape(key); err == nil {
			key = k
		}
		if v, err := url.PathUnescape(val); err == nil {
			val = v
		}
		if key == "" {
			continue
		}
		out = append(out, kv{key: key, val: val})
	}
	return out
}

// SetQuery кладёт разобранную query в пространство.
func (s *Space) SetQuery(pairs []kv) { s.query = pairs }

// PlusToSpace применяет форм-семантику `+` → пробел.
//
// Вызывается ТОЛЬКО там, где запись не объявила `plus_literal`: норма
// проекта — `+` в query это пробел (form-encoding), и так же наш эмит пишет;
// исключение адресное и выводится из формата поля (DELTAS D133-7).
func PlusToSpace(v string) string { return strings.ReplaceAll(v, "+", " ") }
