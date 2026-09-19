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

	TypeOf map[string]string `json:"type_of"`

	// ValueOf / ValueIn — точное значение по пути. Ожидаемое значение —
	// ЛЮБОЙ скаляр JSON, а не только строка: строка сравнивается со строкой
	// регистронезависимо, число с числом, булево с булевым, и ТИПЫ НЕ
	// ПРИВОДЯТСЯ друг к другу. Приведение здесь было бы не удобством, а
	// сменой поведения: `version: "2"` (строкой) прежний конвертер за
	// двойку не считал вовсе (xrayJSONInt строк не читает) и вёл такой
	// элемент в v1 — сматчи предикат строку с числом, узел уехал бы в
	// чужую схему.
	ValueOf map[string]interface{}   `json:"value_of"`
	ValueIn map[string][]interface{} `json:"value_in"`

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

// ForForm — источники, объявленные ДЛЯ ЭТОЙ формы.
//
// Отличается от All() тем, что при записи картой по формам берёт ровно свою
// ветку, а не склейку всех. Склейка годится линтеру («перечисли всё, что
// секция называет»), но исполнению — нет: у vmess метка объявлена `{"v2rayn":
// "json.ps", "legacy": "fragment"}`, и All() отдаёт обе, отсортированные по
// имени формы, — то есть `fragment` раньше `json.ps`. Узел формы v2rayN
// получал имя из фрагмента ссылки, хотя секция прямо пишет, что фрагмент у
// неё не читается вовсе (корпус fragment_after_base64: ждётся `frag` из ps,
// а приходило `MyNodeName` из #).
func (s *SourceRef) ForForm(form string) []string {
	if s == nil {
		return nil
	}
	if len(s.ByForm) > 0 {
		if names, ok := s.ByForm[form]; ok {
			return names
		}
	}
	return s.List
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
	// ValueMapCase — "sensitive", если регистр значения ЗНАЧИМ.
	//
	// Общее правило обратное (живые подписки шлют `security=NONE`), но там,
	// где ядро сравнивает литерал точно, регистронезависимое попадание
	// молча проглатывает негодное значение: `encryption=None` у vless
	// обязано доехать до тела и быть отвергнутым, а не стать «слоя нет».
	ValueMapCase string `json:"value_map_case"`
	Sets     map[string]map[string]interface{} `json:"sets"`
	Implies  map[string]interface{}            `json:"implies"`

	When map[string]interface{} `json:"when"`

	Extract   *Extract               `json:"extract"`
	Compose   *Compose               `json:"compose"`
	List      *ListSpec              `json:"list"`
	SplitInto map[string]interface{} `json:"split_into"`

	// Format — ФОРМАТ значения: задаёт умолчания декода, не судит значение
	// (судит санитайзер). Так политика `+` становится свойством ПОЛЯ, а не
	// заплатой у каждого ключа: `base64*` читает `+` литералом, `pem` —
	// раздельно в заголовке и в теле (PRIMITIVES §0.4a).
	Format string `json:"format"`

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
	// OnWhenFalse — код за ПОДАВЛЕНИЕ значения условием `when`: значение во
	// входе было, но структурное правило не дало ему доехать до тела.
	// Ставится только когда источник действительно что-то дал.
	OnWhenFalse map[string]interface{} `json:"on_when_false"`
	// OnImpliesWritten — код за то, что `implies` И ВПРАВДУ дописал значение,
	// которого во входе не было. Отличается от простого наличия implies: при
	// занятом пути присваивание проигрывает, и сообщать не о чем.
	OnImpliesWritten map[string]interface{} `json:"on_implies_written"`
	OnLenGt       map[string]interface{} `json:"on_len_gt"`
	// OnEmpty — код за ПУСТОЕ либо отсутствующее значение источника.
	//
	// Отличается от `required` тем, что узел ОСТАЁТСЯ: «поля нет» бывает и
	// нормой (у половины схем пустой пароль законен), и признаком протухшей
	// подписки, и различить это может только человек — значит место кода, а
	// не отбраковки. Отличается от `on_invalid` тем, что значения нет вовсе:
	// судить нечего.
	OnEmpty map[string]interface{} `json:"on_empty"`

	EmitWhen    json.RawMessage `json:"emit_when"`
	OmitDefault json.RawMessage `json:"omit_default"`
	Implicit    bool            `json:"implicit"`

	// EmitAs — КАК сериализуется значение на выходе, когда тело хранит его не
	// строкой: `join` (список через разделитель), `bool01` (булев как «1»),
	// `json` (вложенная структура), `raw` (как есть, булев — словом).
	//
	// Угадывать по типу нельзя: булев `true` у одной схемы пишется как `1`
	// (insecure), у другой — словом true (xhttp-флаги Xray), и это свойство
	// ДИАЛЕКТА, а не типа значения.
	EmitAs string `json:"emit_as"`

	// EmitNormalize — обращение `normalize` на ВЫХОДЕ.
	//
	// Обратить `normalize` автоматически нельзя: часть нормализаций
	// необратима (`trim_lower` теряет регистр), а часть обратима лишь
	// частично. Поэтому обращение ОБЪЯВЛЯЕТСЯ, а не выводится: имя правила
	// называет запись, и движок исполняет его так же, как прямое.
	//
	// Сегодня: `port_range_spec_uri` — форма ядра `20000:50000` обратно в
	// написание ссылки `20000-50000` (дефис — конвенция hysteria, и на
	// двоеточии её клиенты спотыкаются).
	EmitNormalize string `json:"emit_normalize"`

	// EmitPairSep — разделитель ВНУТРИ пары у `emit_as: "pairs"` (карта тела
	// → строка пар). Вывести его из регулярки `extract` нельзя: она описывает
	// ЧТЕНИЕ, где после двоеточия допустим любой пробельный хвост, а писать
	// надо ровно одно написание.
	EmitPairSep string `json:"emit_pair_sep"`

	// EmitValueMap — ЯВНАЯ обратная таблица значений: «значение тела →
	// написание ссылки».
	//
	// Нужна там, где прямая таблица НЕИНЪЕКТИВНА: у awg-флагов `on`, `true` и
	// `1` читаются в одно `true`, и обратить их нечем — написание выбирает
	// схема. Объявлять обращение, а не выводить его, приходится и потому, что
	// порядок ключей карты JSON в Go не сохраняется: «первый объявленный»
	// зависел бы от обхода хеш-таблицы.
	EmitValueMap map[string]string `json:"emit_value_map"`

	// RoundTrip — объявленный ОТКАЗ от обратного хода: поле есть в теле, но в
	// ссылке его не пишет ни один клиент. Требует RoundTripWhy.
	//
	// Указатель, а не булев: умолчание «обратный ход есть», и отличить
	// «не объявлено» от явного `true` нужно линтеру, который требует причину
	// ровно у `false`.
	RoundTrip    *bool  `json:"round_trip"`
	RoundTripWhy string `json:"round_trip_why"`

	// RoundTripOnly — запись действует ТОЛЬКО в одну сторону: "emit" —
	// пишется в ссылку, но из неё не читается; "parse" — наоборот.
	//
	// Отличается от `round_trip: false` тем, что там направление ЕСТЬ одно
	// (чтение) и объявлен отказ от второго; здесь объявляется, какое именно
	// единственное. Живой случай — `detour`: поле `managed`, его пишет сборка
	// конфига, санитайзер снимает его из тела узла, но ссылка на узел внутри
	// цепочки обязана нести имя следующего хопа. Читать его обратно нельзя:
	// тег чужого конфига у нас не существует.
	RoundTripOnly string `json:"round_trip_only"`

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

// Compose — обратный шаблон сборки для эмита: обращение `extract`.
//
// Регуляркой обратный ход не выражается (по регулярке нельзя однозначно
// собрать строку), поэтому шаблон объявляется прямо. `From` перечисляет пути
// тела, подставляемые в `Template` по написанию `{путь}`; `OmitWhenEmpty` —
// сегменты, вырезаемые вместе с разделителем, когда их путь пуст.
type Compose struct {
	Template      string   `json:"template"`
	From          []string `json:"from"`
	OmitWhenEmpty []string `json:"omit_when_empty"`
}

// UnmarshalJSON принимает короткое написание (строка-шаблон) и объект.
//
// Короткая форма существует потому, что у большинства шаблонов пути видны в
// самом тексте: `{transport.path}?ed={transport.max_early_data}` называет их
// дословно, и повторять их списком значило бы держать вторую копию.
func (c *Compose) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		if err := json.Unmarshal(data, &c.Template); err != nil {
			return err
		}
		c.From = templatePaths(c.Template)
		return nil
	}
	type alias Compose
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*c = Compose(a)
	if len(c.From) == 0 {
		c.From = templatePaths(c.Template)
	}
	return nil
}

// templatePaths достаёт пути из шаблона: всё в фигурных скобках.
func templatePaths(tpl string) []string {
	var out []string
	rest := tpl
	for {
		i := strings.Index(rest, "{")
		if i < 0 {
			return out
		}
		j := strings.Index(rest[i:], "}")
		if j < 0 {
			return out
		}
		name := rest[i+1 : i+j]
		if name != "" {
			out = append(out, name)
		}
		rest = rest[i+j+1:]
	}
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
	// Emit — писать ли userinfo НА ВЫХОДЕ. Указатель: умолчание «писать», а
	// явный `false` означает, что канон схемы кладёт то же значение в query
	// (у hysteria 1.x секрет канонически едет `auth=`, а userinfo — лишь
	// запасной слот ЧТЕНИЯ), и запись в оба места отдала бы секрет дважды.
	Emit *bool `json:"emit"`

	// Required — ссылка БЕЗ userinfo не узел: разбор отказывает.
	//
	// Объявляется у userinfo, а не у записи: поля, которые он наполняет,
	// приходят позициями into, и у части схем записи под ними вовсе нет
	// (uuid у vless объявлен null). Прежний путь перечислял такие схемы
	// поимённо — node_parser_core.go:403-405.
	Required bool `json:"required"`
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
	// ServerPath / PortPath — где в ТЕЛЕ лежат адрес и порт, если не в
	// корне (`server`/`server_port`). У wireguard узел это ENDPOINT, и
	// адрес сервера живёт в `peers[].address`; шаблон `{scheme}-{server}-
	// {server_port}` без этих путей подставил бы пустоту, а ParsedNode
	// остался бы без Server/Port — по ним работают дедуп, skip-фильтры и
	// UI (PRIMITIVES §0.13).
	ServerPath string `json:"server_path"`
	PortPath   string `json:"port_path"`
	// SchemeSource: "as_written" — по написанию схемы (hy2-… ≠ hysteria2-…);
	// "singbox_type" — по типу тела. Решение владельца 19.09.2026:
	// singbox_type. Меняет identity живых узлов (DELTAS.md D133-6).
	SchemeSource string `json:"scheme_source"`
}

// Overlay — наложенное пространство источников.
//
// `name` — префикс, под которым слой адресуется в `source`; `source` — откуда
// берётся сам текст слоя; `decode` — конвейер декодеров (`json`, `base64?`);
// `flatten` — имена вложенных объектов, чьи члены поднимаются в плоский слой
// (`xmux` у xhttp).
type Overlay struct {
	Name    string    `json:"name"`
	Source  SourceRef `json:"source"`
	Decode  []string  `json:"decode"`
	Flatten []string  `json:"flatten"`
	Impl    string    `json:"impl"`
}

// IniDialect — КАК читать ini-документ: правила разбора, а не отображение.
//
// Заведён потому, что на одном и том же месте жили ТРИ разных поведения:
// движковый `parseINI` вторую `[Peer]` сливал с первой (ключи второй
// перезаписывали ключи первой), прежний `parseWGConfSections` брал ключи
// только первой и МОЛЧА отбрасывал остальные, а норма требует «первая плюс
// код». Пока это решение жило в коде, выбрать между ними было нечем —
// отсюда атрибуты.
//
// Пустая секция = сегодняшний диалект wg-quick: ключи в нижний регистр,
// значения как есть, комментарий только целой строкой (`#`, `;`), повторный
// ключ — последний выигрывает, секции сливаются.
type IniDialect struct {
	// KeyCase — регистр имён ключей: "lower" (по умолчанию) | "preserve".
	KeyCase string `json:"key_case"`
	// ValueCase — регистр значений: "preserve" (по умолчанию) | "lower".
	// Значения трогать нельзя без нужды: `Id` маскировки — это домен.
	ValueCase string `json:"value_case"`

	// LineCommentPrefixes — маркеры комментария, занимающего ВСЮ строку.
	// Алиас `comment_prefixes` — имя из черновика SCHEMES §13.4; читаются
	// оба, чтобы правка черновика не стала правкой поведения.
	LineCommentPrefixes []string `json:"line_comment_prefixes"`
	CommentPrefixes     []string `json:"comment_prefixes"`

	// InlineComments — резать ли комментарий ПОСЛЕ значения. У wg-quick
	// false: '#' законен внутри значения («US-FREE#137»).
	InlineComments bool `json:"inline_comments"`

	// RepeatedKey — повторный ключ ВНУТРИ секции:
	// "last_wins" (по умолчанию) | "first_wins" | "append".
	RepeatedKey string `json:"repeated_key"`

	// Sections — правила отдельных секций по имени (регистр не важен).
	Sections map[string]*IniSection `json:"sections"`

	Impl string `json:"impl"`
}

// IniSection — правило ПОВТОРА секции с одним именем.
type IniSection struct {
	// Repeat — что делать со второй секцией того же имени:
	// "merge" (по умолчанию) | "first_only".
	Repeat string `json:"repeat"`
	// OnExtra — код на отброшенные повторы. Параметр `count` получает
	// ОБЩЕЕ число секций этого имени, как их написал человек.
	OnExtra *IniOnExtra `json:"on_extra"`
	Impl    string      `json:"impl"`
}

// IniOnExtra — код, которым отмечается отброшенный повтор секции.
type IniOnExtra struct {
	Code string `json:"code"`
}

// Prefixes возвращает маркеры строчного комментария с учётом алиаса.
func (d *IniDialect) Prefixes() []string {
	if d != nil {
		if len(d.LineCommentPrefixes) > 0 {
			return d.LineCommentPrefixes
		}
		if len(d.CommentPrefixes) > 0 {
			return d.CommentPrefixes
		}
	}
	return []string{"#", ";"}
}

// Section возвращает правило секции по имени без учёта регистра.
func (d *IniDialect) Section(name string) *IniSection {
	if d == nil || len(d.Sections) == 0 {
		return nil
	}
	for k, v := range d.Sections {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return nil
}

// UnknownKey — что делать с неперечисленным ключом источника.
// Молчание — тот самый дефект, ради которого затеяна кампания.
type UnknownKey struct {
	Action string `json:"action"`
	Code   string `json:"code"`
	// Ignore — имена, которые полем узла НЕ являются и кода не заслуживают
	// (GRAMMAR_SYNC §1 №12, имя LxBox). Нужен там, где элемент входа несёт
	// служебные ключи уровня ДОКУМЕНТА: у Xray-outbound'а это `protocol`,
	// `tag`, `remarks` — их читает обход документа, а не секция, и без
	// списка каждая xray-секция вешала бы узлу json_field_unknown на
	// собственный `protocol`.
	//
	// Отличается от записи со значением null («параметр знаем, читать
	// нечего») ровно тем, что ключ вообще не принадлежит этому уровню:
	// объявить его записью значило бы сказать, что секция его читает.
	Ignore []string `json:"ignore"`
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

	// Names — переименование параметра НА ВЫХОДЕ: канон разбора → написание
	// ссылки. Норма «канон = первое в aliases» описывает ЧТЕНИЕ, а вид ссылки
	// есть де-факто формат СХЕМЫ для чужих клиентов: имя флага «не проверять
	// сертификат» у vless/trojan/http — allowInsecure, у tuic —
	// allow_insecure, у hysteria2 — insecure, и общий блок выбрать за них не
	// может. Действует и по форме-контейнеру (имя gRPC-канала у v2rayN
	// лежит в ключе path).
	Names map[string]string `json:"names"`

	// OmitPort — порт опускается на выходе, когда равен дефолту схемы.
	// Обращать `defaults.server_port` напрямую нельзя: у схемы бывает дефолт
	// РАЗБОРА без права опускать порт на выходе — ссылка перестала бы
	// читаться клиентами, которые дефолта не знают.
	OmitPort bool `json:"omit_port"`

	// UserInfo — выходная форма userinfo. Обратить `userinfo.decode` разбора
	// нельзя: это конвейер ПОПЫТОК, где `base64?` значит «может быть, а может
	// и нет».
	UserInfo *EmitUserInfo `json:"userinfo"`

	// JSONMap — форма-КОНТЕЙНЕР: собирает не query-ссылку, а base64(JSON).
	// Карта «ключ контейнера → путь тела | `$label` | `$param.<имя>` |
	// `=литерал`».
	JSONMap map[string]string `json:"json_map"`
	// JSONAlways — ключи контейнера, которые клиенты ждут ДАЖЕ ПУСТЫМИ:
	// карта «ключ → заполнитель». Опускание такого ключа ломает чтение у
	// панелей, а не экономит байты.
	JSONAlways map[string]interface{} `json:"json_always"`

	// RefuseWhen — условия, при которых узел ссылкой НЕ выражается: движок
	// отказывает, а не отдаёт половину. «Сколько сущностей влезает в ссылку»
	// есть свойство ФОРМАТА схемы, и проверке в коде там не место.
	RefuseWhen []EmitRefuse `json:"refuse_when"`

	Impl string `json:"impl"`
}

// EmitRefuse — одно условие отказа.
type EmitRefuse struct {
	Path  string `json:"path"`
	LenGt int    `json:"len_gt"`
	// Why — причина прозой: она едет ЧЕЛОВЕКУ, и «не поддержано» не
	// отвечает на его вопрос.
	Why string `json:"why"`
}

// EmitUserInfo — выходная форма userinfo.
type EmitUserInfo struct {
	// Form — `raw` | `base64`.
	Form string `json:"form"`
	// Padding — писать ли `=`-паддинг у base64. Указатель: Go пишет с
	// паддингом, Dart срезает, обе стороны читают обе формы, и по правилу
	// эмита ни одна не меняет своё написание молча.
	Padding *bool `json:"padding"`
	// EmptySeparator — писать «@» у узла БЕЗ userinfo: `hy2://@host` —
	// способ схемы сказать «пароля нет».
	EmptySeparator bool `json:"empty_separator"`
	// KeepEmptyTail — писать разделитель у ПУСТОГО хвостового компонента:
	// `socks4://userid:@host`. У socks4 пароля нет по протоколу, но
	// разделитель клиенты пишут всегда, и его отсутствие часть из них читает
	// как «имени нет».
	KeepEmptyTail bool `json:"keep_empty_tail"`
}

// UnmarshalJSON принимает короткое написание (строка-форма) и объект.
func (e *EmitUserInfo) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		return json.Unmarshal(data, &e.Form)
	}
	type alias EmitUserInfo
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*e = EmitUserInfo(a)
	return nil
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

	// IniDialect — правила чтения ini для секций с пространством `ini`.
	// Отсутствие секции означает диалект по умолчанию (см. IniDialect).
	IniDialect *IniDialect `json:"ini_dialect"`

	// Overlays — ДОПОЛНИТЕЛЬНЫЕ пространства источников, распакованные из
	// значения внутри входа: чужой диалект приезжает вложенным слоем (JSON в
	// query-параметре `extra` у xhttp). Запись адресует его тем же `source`
	// под именем слоя (`extra.scMaxEachPostBytes`).
	//
	// Слой ОТДЕЛЬНЫЙ, а не слитый с query: кто из двух побеждает, решает
	// запись порядком своих источников. Слияние приняло бы это решение за неё
	// и одинаково для всех ключей — ровно то, на чём горит базовая тройка
	// mode/path/host, где Xray затирает вложенный слой плоским (D-097).
	Overlays []Overlay `json:"overlays"`

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

	// KindWhen — РОД узла, объявляемый по условию на ВХОД: имя рода →
	// условие ({any_set: [<источники>]} либо {<источник>: {matches: "…"}}).
	// В тело род НЕ пишется; он доезжает до санитайзера контекстом рядом с
	// body_source, и правила реестра читают его оператором
	// `when.source_kind`.
	//
	// Нужен там, где судить по телу нельзя ПО ПОСТРОЕНИЮ: ссылка `awg://` с
	// негодными awg-значениями оставляет тело, неотличимое от обычного
	// WireGuard, а потолок MTU есть свойство ЗАПРОШЕННОГО протокола.
	// Проверяются в порядке объявления карты, отсортированном по имени, и
	// побеждает ПЕРВЫЙ подошедший — поэтому имена родов выбираются так,
	// чтобы порядок не был значимым (у wireguard: awg, awg3).
	KindWhen map[string]map[string]interface{} `json:"kind_when"`

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

// ProtocolFileNames — имена файлов протоколов реестра.
//
// Движку (core/config/linkmap) нужен порядок объявления params, а он живёт
// только в ИСХОДНОМ тексте файла: имя файла по схеме там не всегда угадывается
// (ss→shadowsocks), и перебор идёт по этому списку.
func ProtocolFileNames() []string {
	out := make([]string, len(protocolFiles))
	copy(out, protocolFiles)
	return out
}

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
