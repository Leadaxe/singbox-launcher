// File mapper.go — модель ИСПОЛНЯЕМЫХ секций-мапперов реестра (SPEC 133).
//
// Отличие от секции `mapper` в registry_body.schema.json: та описательная, её
// в рантайме не исполняет никто (в её же докстроке так и написано). Здесь —
// таблица, которую движок `core/config/linkmap` ИСПОЛНЯЕТ: значение доступно
// параметру только через `source`, сырого url.Values у движка нет.
//
// Файл отдельный от registry.go намеренно: registry.go правят параллельно
// другие волны, и новая модель не должна конфликтовать построчно.
//
// Как и весь пакет, код не знает ни одной схемы по имени — имена приходят с
// диска. go1.20-совместимо: без slices/maps/min/max.
package registry

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"singbox-launcher/contract"
)

// Detect — декларативный признак «этот контент — мой».
//
// Один словарь на ДВА уровня (SPEC 133 §3A): вид источника целиком
// (sources.json) и принадлежность элемента протоколу/диалекту
// (mappers.<kind>.detect, forms[].detect). Движок исполняет их одинаково —
// отдельного «документного» языка нет, иначе сниффер формата вернулся бы в
// код через заднюю дверь.
//
// Предикаты внутри одного объекта — конъюнкция.
type Detect struct {
	Regex string `json:"regex"`

	JSON *DetectJSON `json:"json"`
	INI  *DetectINI  `json:"ini"`
	Text *DetectText `json:"text"`

	// SchemeIn — написания схемы ссылки, на которые отзывается секция.
	SchemeIn []string `json:"scheme_in"`
	// InArray — имя массива документа, из которого пришёл элемент
	// ("outbounds" / "endpoints"): различает уровень у sing-box.
	InArray string `json:"in_array"`

	Not *Detect  `json:"not"`
	All []Detect `json:"all"`
	Any []Detect `json:"any"`

	// Default — ветка «всё остальное». Ровно одна на уровень (линтер).
	Default bool `json:"default"`
}

// DetectJSON — предикаты по разобранному JSON. Пути точечные ("settings.vnext").
type DetectJSON struct {
	RequiredKeys []string `json:"required_keys"`
	AnyKeys      []string `json:"any_keys"`
	KeyAbsent    []string `json:"key_absent"`

	TypeOf  map[string]string   `json:"type_of"`
	ValueOf map[string]string   `json:"value_of"`
	ValueIn map[string][]string `json:"value_in"`

	// ArrayElemAnyKeys — хотя бы один элемент массива-документа несёт путь.
	// Так Xray-конфиг отличается от sing-box: элемент Xray несёт
	// outbounds[].protocol.
	ArrayElemAnyKeys []string `json:"array_elem_any_keys"`
}

// DetectINI — предикаты по ini-тексту; имена сравниваются регистронезависимо.
type DetectINI struct {
	Sections []string `json:"sections"`
	Keys     []string `json:"keys"`
	// KeysAny — хотя бы один ключ. Так род узла (awg3/awg/wg) объявляется
	// данными, а не функцией hasAWGParams.
	KeysAny []string `json:"keys_any"`
}

// DetectText — предикаты по сырому тексту.
type DetectText struct {
	PrefixFold string `json:"prefix_fold"`
	LineFold   string `json:"line_fold"`
	Contains   string `json:"contains"`
}

// IsZero сообщает, что предикат пуст: такую запись выбрать нельзя, и линтер
// обязан её поймать (кроме записи с Default).
func (d *Detect) IsZero() bool {
	if d == nil {
		return true
	}
	if d.Default {
		return false
	}
	return d.Regex == "" && d.JSON == nil && d.INI == nil && d.Text == nil &&
		len(d.SchemeIn) == 0 && d.InArray == "" &&
		d.Not == nil && len(d.All) == 0 && len(d.Any) == 0
}

// Regexes собирает все регулярки предиката (включая вложенные) — линтеру,
// который проверяет их компиляцию в обоих диалектах.
func (d *Detect) Regexes() []string {
	if d == nil {
		return nil
	}
	var out []string
	if d.Regex != "" {
		out = append(out, d.Regex)
	}
	out = append(out, d.Not.Regexes()...)
	for i := range d.All {
		out = append(out, d.All[i].Regexes()...)
	}
	for i := range d.Any {
		out = append(out, d.Any[i].Regexes()...)
	}
	return out
}

// Form — оболочка источника: как из текста получить пространство источников.
// Формы пробуются по порядку, первая сработавшая выигрывает.
type Form struct {
	ID     string  `json:"id"`
	Detect *Detect `json:"detect"`
	// Decode — конвейер декодеров: строки ("base64", "base64?", "url", "ini",
	// "json") либо объект ({"reparse": "url"}).
	Decode []json.RawMessage `json:"decode"`
	Space  string            `json:"space"`
	// Base — якорь пути: префикс вместо $base в source. Одна таблица
	// параметров обслуживает vnext[0], servers[0] и плоскую форму, снимая
	// дословный дубль выемки в Xray-ветке.
	Base string `json:"base"`
	// Level — уровень документа sing-box, откуда пришёл элемент.
	Level string `json:"level"`
}

// SourceRef — откуда параметр берёт значение.
//
// ЕДИНСТВЕННЫЙ способ получить значение: сырого url.Values/карты JSON у
// движка нет, поэтому дефект «поле знаем, но не читаем» невозможен по
// построению, а не по грепу.
//
// Три записи: строка (один источник), массив (цепочка приоритета, первый
// найденный выигрывает), карта (по id формы — у vmess две формы с разными
// пространствами и одним набором maps_to).
type SourceRef struct {
	List   []string
	ByForm map[string][]string
}

// UnmarshalJSON принимает все три записи.
func (s *SourceRef) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	switch trimmed[0] {
	case '"':
		var one string
		if err := json.Unmarshal(data, &one); err != nil {
			return err
		}
		s.List = []string{one}
		return nil
	case '[':
		return json.Unmarshal(data, &s.List)
	case '{':
		raw := map[string]json.RawMessage{}
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		s.ByForm = map[string][]string{}
		for form, v := range raw {
			var nested SourceRef
			if err := nested.UnmarshalJSON(v); err != nil {
				return err
			}
			s.ByForm[form] = nested.List
		}
		return nil
	}
	return fmt.Errorf("source: неожиданная запись %s", trimmed)
}

// IsZero — источник не объявлен. Для параметра секции это ошибка линтера:
// «объявлен, но не читается» — тот самый класс дефекта, ради которого
// затеяна кампания.
func (s *SourceRef) IsZero() bool {
	return s == nil || (len(s.List) == 0 && len(s.ByForm) == 0)
}

// All — все объявленные источники (по всем формам), для линтера.
func (s *SourceRef) All() []string {
	if s == nil {
		return nil
	}
	out := append([]string{}, s.List...)
	if len(s.ByForm) > 0 {
		forms := make([]string, 0, len(s.ByForm))
		for form := range s.ByForm {
			forms = append(forms, form)
		}
		sort.Strings(forms)
		for _, form := range forms {
			out = append(out, s.ByForm[form]...)
		}
	}
	return out
}

// DecodeExtra — дополнительный percent-декод поверх декодера формы.
//
// Режимы различаются НАМЕРЕННО: в пути литеральный `+` легален, и
// query-семантика превратила бы `/ws+v2%2Fdata` в `/ws v2/data` (сервер
// отвечает 404, узел «жив» и молча не работает).
type DecodeExtra struct {
	Mode string `json:"mode"`
	// Passes — число проходов либо "until_stable": alpn декодируется до
	// стабилизации (фикстура alpn_multiply_encoded несёт ТРИ уровня), path —
	// ровно 2.
	Passes json.RawMessage `json:"passes"`
	Max    int             `json:"max"`
	// PlusLiteral — не превращать `+` в пробел. По умолчанию выводится из
	// format поля (base64*), явное указание перекрывает.
	PlusLiteral *bool `json:"plus_literal"`
}

// PassCount разбирает Passes: (n, untilStable).
func (d *DecodeExtra) PassCount() (int, bool) {
	if d == nil || len(d.Passes) == 0 {
		return 1, false
	}
	var n int
	if err := json.Unmarshal(d.Passes, &n); err == nil {
		return n, false
	}
	var s string
	if err := json.Unmarshal(d.Passes, &s); err == nil && s == "until_stable" {
		return 0, true
	}
	return 1, false
}

// MapsTo — путь(и) в теле, куда едет значение.
//
// Карта по типу тела нужна там, где ОДИН вход ведёт в разные поля: у
// hysteria `auth` это `auth_str` в v1 и `password` в v2, и версия выбирает
// не значение, а целевое поле.
type MapsTo struct {
	Path   string
	ByType map[string]string
	// Explicit — ключ maps_to присутствовал со значением null: значение
	// осознанно никуда не едет (снимается с кодом). Отличается от отсутствия
	// ключа, где путь просто не объявлен.
	ExplicitNull bool
}

// UnmarshalJSON принимает строку, null и карту по типу тела.
func (m *MapsTo) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "null" {
		m.ExplicitNull = true
		return nil
	}
	if trimmed == "" {
		return nil
	}
	if trimmed[0] == '"' {
		return json.Unmarshal(data, &m.Path)
	}
	return json.Unmarshal(data, &m.ByType)
}

// Paths — все целевые пути, для линтера «maps_to существует в body.fields».
func (m *MapsTo) Paths() []string {
	if m == nil {
		return nil
	}
	if m.Path != "" {
		return []string{m.Path}
	}
	out := make([]string, 0, len(m.ByType))
	for _, p := range m.ByType {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Param — одна запись таблицы маппера.
type Param struct {
	Source SourceRef `json:"source"`
	MapsTo *MapsTo   `json:"maps_to"`

	Aliases  []string `json:"aliases"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`

	// Selector — параметр первого прохода: его значение строит тело, по
	// которому дальше проверяется when у зависимых.
	Selector bool `json:"selector"`
	// Priority — явный порядок двух записей в один путь (меньше = раньше).
	// Правило «sets побеждает» неверно: побеждает не источник, а порядок.
	Priority int    `json:"priority"`
	Merge    string `json:"merge"`

	ValueMap map[string]interface{}            `json:"value_map"`
	Sets     map[string]map[string]interface{} `json:"sets"`
	Implies  map[string]interface{}            `json:"implies"`

	When map[string]interface{} `json:"when"`

	Extract   *Extract               `json:"extract"`
	Compose   string                 `json:"compose"`
	List      *ListSpec              `json:"list"`
	SplitInto map[string]interface{} `json:"split_into"`

	Normalize   string       `json:"normalize"`
	DecodeExtra *DecodeExtra `json:"decode_extra"`

	DefaultFrom json.RawMessage        `json:"default_from"`
	DefaultWhen map[string]interface{} `json:"default_when"`
	// MaterializeDefault — маппер ЗАПИСЫВАЕТ дефолт в тело, даже когда
	// источник молчал. Отличается от default в body.fields: там это правило
	// санитайзера («чем заполнить пустоту»), здесь — обязанность маппера.
	MaterializeDefault bool `json:"materialize_default"`

	Coerce  *Coerce  `json:"coerce"`
	Flatten []string `json:"flatten"`
	Lift    string   `json:"lift"`
	// SortKeys — детерминированный порядок ключей у type:object. Порядок
	// входит в identity, и оставлять его свойством реализации нельзя.
	SortKeys bool `json:"sort_keys"`

	Empty string `json:"empty"`

	OnInvalid     map[string]interface{} `json:"on_invalid"`
	OnPresent     map[string]interface{} `json:"on_present"`
	OnItemInvalid map[string]interface{} `json:"on_item_invalid"`
	OnNoMatch     map[string]interface{} `json:"on_no_match"`
	OnLenGt       map[string]interface{} `json:"on_len_gt"`

	EmitWhen    json.RawMessage `json:"emit_when"`
	OmitDefault json.RawMessage `json:"omit_default"`
	Implicit    bool            `json:"implicit"`

	Since  string `json:"since"`
	DescEN string `json:"desc_en"`
	DescRU string `json:"desc_ru"`
	Impl   string `json:"impl"`
}

// Extract — регулярка с именованными группами, раскладывающая одно значение
// по нескольким путям тела. Диалект — RE2 ∩ ECMAScript.
type Extract struct {
	Re   string                 `json:"re"`
	Into map[string]interface{} `json:"into"`
}

// ListSpec — список через разделитель.
type ListSpec struct {
	Sep  string `json:"sep"`
	Item string `json:"item"`
	Len  int    `json:"len"`
	// CoerceScalar — скаляр принимается как список из одного элемента:
	// Xray httpSettings.host бывает и строкой, и массивом.
	CoerceScalar bool `json:"coerce_scalar"`
}

// Coerce — приведение ФОРМЫ значения (не смысла).
type Coerce struct {
	ObjectToScalar string `json:"object_to_scalar"`
	ScalarToList   bool   `json:"scalar_to_list"`
}

// UserInfo — разбор userinfo ссылки.
type UserInfo struct {
	Decode []string `json:"decode"`
	Split  *struct {
		Sep string `json:"sep"`
		// Limit: 2 — резать по ПЕРВОМУ разделителю: пароль с двоеточием
		// иначе теряется.
		Limit int `json:"limit"`
	} `json:"split"`
	Into       []string `json:"into"`
	SingleInto string   `json:"single_into"`
}

// LabelSpec — метка узла. Входит в identity, поэтому её нормализация
// объявляется, а не остаётся свойством кода.
type LabelSpec struct {
	Source    SourceRef              `json:"source"`
	Normalize []string               `json:"normalize"`
	ValueMap  map[string]interface{} `json:"value_map"`
	Fallback  *LabelFallback         `json:"fallback"`
}

// LabelFallback — имя узла, когда метки во входе нет.
type LabelFallback struct {
	Template string `json:"template"`
	// SchemeSource: "as_written" — по написанию схемы (hy2-… ≠ hysteria2-…);
	// "singbox_type" — по типу тела. Решение владельца 19.09.2026:
	// singbox_type. Меняет identity живых узлов (DELTAS.md D133-6).
	SchemeSource string `json:"scheme_source"`
}

// UnknownKey — что делать с неперечисленным ключом источника.
// Молчание — тот самый дефект, ради которого затеяна кампания.
type UnknownKey struct {
	Action string `json:"action"`
	Code   string `json:"code"`
}

// EmitSpec — обратное направление. Отсутствие секции (null в JSON) означает
// «обратного хода нет» (xray, conf), и линтер не требует param_order.
type EmitSpec struct {
	Form     string                 `json:"form"`
	FormFrom map[string]interface{} `json:"form_from"`
	// ParamOrder — либо список имён, либо правило "alphabetical".
	ParamOrder  json.RawMessage        `json:"param_order"`
	OmitDefault []string               `json:"omit_default"`
	EmitWhen    map[string]interface{} `json:"emit_when"`
}

// Mapper — одна секция-маппер: вид источника у одного протокола.
type Mapper struct {
	Detect *Detect `json:"detect"`
	// BodySource — каким source тело приходит в санитайзер: на это опирается
	// except_sources в правилах реестра. У нас пять значений
	// (uri/singbox/xray/wgconf/amnezia), у LxBox — два, и любое правило,
	// различающее xray и uri, без этого объявления разъедется.
	BodySource string `json:"body_source"`

	Forms    []Form     `json:"forms"`
	UserInfo *UserInfo  `json:"userinfo"`
	Label    *LabelSpec `json:"label"`

	Params map[string]*Param `json:"params"`

	// Include — переиспользуемые блоки с диалектными вариантами
	// ("transports.ws#uri", "tls#xray"): разворачиваются загрузчиком ДО
	// исполнения, конфликт имён — ошибка линтера, а не молчаливое
	// перекрытие.
	Include []string `json:"include"`

	SchemeSets   map[string]map[string]interface{} `json:"scheme_sets"`
	TypeSynonyms map[string]string                 `json:"type_synonyms"`
	Defaults     map[string]interface{}            `json:"defaults"`

	UnknownKey *UnknownKey `json:"unknown_key"`

	// Emit — nil, если ключа не было ИЛИ он был null. Различать не нужно:
	// в обоих случаях обратного хода нет.
	Emit *EmitSpec `json:"emit"`

	// kind — вид источника ("uri"/"xray"/"singbox"/"conf"); заполняет
	// загрузчик по ключу карты.
	kind string
	// scheme — схема протокола, которой принадлежит секция.
	scheme string
}

// Kind — вид источника секции.
func (m *Mapper) Kind() string { return m.kind }

// Scheme — схема протокола, которой принадлежит секция.
func (m *Mapper) Scheme() string { return m.scheme }

// SourceKind — вид источника на уровне ДОКУМЕНТА (registry/sources.json).
type SourceKind struct {
	Kind string `json:"kind"`
	// Priority — порядок проверки, меньше = раньше. При совпадении
	// нескольких detect побеждает меньший priority; уникальность проверяет
	// линтер, иначе порядок зависел бы от порядка строк в файле.
	Priority int     `json:"priority"`
	Mapper   *string `json:"mapper"`
	Detect   *Detect `json:"detect"`

	// Unwrap — оболочка-декодер ("base64", "amnezia_vpn"); Redetect
	// отправляет результат на повторный детект (подписка base64 внутри
	// base64).
	Unwrap   string `json:"unwrap"`
	Redetect bool   `json:"redetect"`
	Split    string `json:"split"`

	DescEN string `json:"desc_en"`
	DescRU string `json:"desc_ru"`
}

// DocumentSpec — уровень документа целиком.
type DocumentSpec struct {
	// MaxUnwrapDepth — предел рекурсии распаковки. Сегодня предела нет
	// вовсе: подписка base64 внутри base64 раскрывается неограниченно.
	MaxUnwrapDepth int `json:"max_unwrap_depth"`
	OnUnrecognized *struct {
		Code            string `json:"code"`
		Severity        string `json:"severity"`
		IncludeFragment int    `json:"include_fragment"`
	} `json:"on_unrecognized"`
	Sources []SourceKind `json:"sources"`
}

// sourcesFileName — файл уровня документа.
const sourcesFileName = "sources.json"

// LoadMappers читает секции `mappers` всех протоколов и уровень документа.
//
// Отсутствие sources.json и отсутствие секций `mappers` — НЕ ошибка: реестр
// переезжает на новую грамматику волнами (SPEC 133), и до перевода схемы
// секции у неё нет. Движок в этом случае просто не находит таблицу и схема
// продолжает идти старым парсером.
func LoadMappers() (*MapperSet, error) {
	set := &MapperSet{
		byScheme: map[string]map[string]*Mapper{},
	}

	for _, name := range protocolFiles {
		f, err := readFile("protocols/" + name + ".json")
		if err != nil {
			return nil, err
		}
		raw, ok := f.Raw["mappers"]
		if !ok {
			continue
		}
		scheme := strings.TrimSpace(schemeNameOf(f.Raw))
		if scheme == "" {
			scheme = name
		}
		byKind := map[string]*Mapper{}
		if err := json.Unmarshal(raw, &byKind); err != nil {
			return nil, fmt.Errorf("registry: protocols/%s.json: mappers: %w", name, err)
		}
		for kind, m := range byKind {
			if m == nil {
				continue
			}
			m.kind = kind
			m.scheme = scheme
		}
		set.byScheme[scheme] = byKind
	}

	// sources.json может ещё не существовать — см. докстроку.
	if data, err := contract.ReadRegistry(sourcesFileName); err == nil {
		var doc struct {
			Document *DocumentSpec `json:"document"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("registry: %s: %w", sourcesFileName, err)
		}
		set.document = doc.Document
	}

	return set, nil
}

// MapperSet — все секции-мапперы реестра плюс уровень документа.
type MapperSet struct {
	byScheme map[string]map[string]*Mapper
	document *DocumentSpec
}

// Mapper возвращает секцию схемы по виду источника.
func (s *MapperSet) Mapper(scheme, kind string) (*Mapper, bool) {
	if s == nil {
		return nil, false
	}
	byKind, ok := s.byScheme[scheme]
	if !ok {
		return nil, false
	}
	m, ok := byKind[kind]
	return m, ok && m != nil
}

// Schemes — схемы, у которых есть хоть одна секция-маппер, по алфавиту.
func (s *MapperSet) Schemes() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.byScheme))
	for scheme := range s.byScheme {
		out = append(out, scheme)
	}
	sort.Strings(out)
	return out
}

// Kinds — виды источника, объявленные у схемы, по алфавиту.
func (s *MapperSet) Kinds(scheme string) []string {
	if s == nil {
		return nil
	}
	byKind, ok := s.byScheme[scheme]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(byKind))
	for kind := range byKind {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}

// Document — уровень документа; nil, пока sources.json не заведён.
func (s *MapperSet) Document() *DocumentSpec {
	if s == nil {
		return nil
	}
	return s.document
}

// SourcesByPriority — виды источника в порядке проверки.
//
// Порядок задаёт priority, а не позиция в файле: перестановка строк не
// должна менять поведение, а совпадение двух detect обязано разрешаться
// объявленным правилом, а не случайностью.
func (s *MapperSet) SourcesByPriority() []SourceKind {
	if s == nil || s.document == nil {
		return nil
	}
	out := append([]SourceKind{}, s.document.Sources...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out
}
