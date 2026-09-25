// Package registry читает реестр контракта (contract/registry/*.json) и
// отдаёт его санитайзеру и эмиттеру узла в разрешённом виде: ссылки `ref`
// подставлены, `__dialer` влит плоско, вариантные транспорты собраны.
//
// Пакет — только загрузка и индексы: ни одного решения о значениях полей
// здесь нет, все решения лежат в JSON (SPEC 131 §4). Код не знает ни одной
// схемы по имени — имена приходят с диска.
//
// go1.20-совместимо (Win7-джоба собирает весь модуль тулчейном go1.20):
// без slices/maps/min/max/clear.
package registry

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"singbox-launcher/contract"
)

// Field — поле схемы тела. Атрибуты — словарь contract/schema/registry_body.schema.json;
// неизвестные атрибуты молча игнорируются (реестр может уехать вперёд кода).
type Field struct {
	Type   string `json:"type"`
	Ref    string `json:"ref"`
	Inline bool   `json:"inline"`

	// Вложенность.
	Items  *Field            `json:"items"`
	Order  []string          `json:"order"`
	Fields map[string]*Field `json:"fields"`

	// Ограничения значения.
	Values []interface{} `json:"values"`
	Format string        `json:"format"`
	// Pattern — регулярное выражение, которому обязано соответствовать
	// строковое значение. Заводится там, где форма значения ЕСТЬ, но именем
	// формата не выражается: закрытого enum нет, а произвольную строку ядро
	// всё же разбирает по своей грамматике (vless.encryption).
	//
	// Проверяется ПОСЛЕ normalize, то есть по тому значению, которое реально
	// уедет в тело. Значение `on_invalid` при провале — общее с остальными
	// ограничениями.
	//
	// Диалект — общее подмножество Go RE2 и ECMAScript/Dart: без lookaround,
	// без обратных ссылок, без inline-флагов. Совпадение по ВСЕЙ строке
	// задаётся якорями в самом выражении (`^…$`), а не режимом проверки:
	// иначе одно и то же выражение значило бы у сторон разное.
	Pattern string `json:"pattern"`
	// ItemPattern — выражение, которому обязан соответствовать ЭЛЕМЕНТ списка.
	//
	// Формат элемента и формат поля — разные вещи, и `pattern` второго не
	// выражает: он судит значение ЦЕЛИКОМ, то есть провал одного элемента
	// снимает список со всеми остальными. Ядру же одного негодного элемента
	// достаточно, чтобы отвергнуть ВЕСЬ конфиг: `server_ports` с элементом
	// `198.51.100.24:443` даёт «bad port range», и пользователь остаётся без
	// VPN, а не без одного узла.
	//
	// Проверяется только форма МАССИВА: у скалярной записи listable-поля
	// элемент и есть всё значение, и судит его `pattern` — двух имён для
	// одного и того же не заводим. Диалект выражения и место якорей — те же,
	// что у `pattern`.
	ItemPattern string `json:"item_pattern"`
	// OnItemInvalid — что делать с элементом, не прошедшим `item_pattern`.
	// Имя то же, что у одноимённого оператора маппера (`extract` по элементам
	// списка): операция одна, словарей два.
	OnItemInvalid *OnItemInvalid `json:"on_item_invalid"`
	// AbsentValues — литералы, которые означают «этого нет»: значение
	// признаётся эквивалентом отсутствия ключа, поле в тело не пишется, кода
	// нет. Проверяется ПОСЛЕ normalize и ДО остальных ограничений.
	//
	// Нужен там, где выключатель настройки записан СЛОВОМ внутри того же
	// поля, что и значение: у vless.encryption `none` — собственный литерал
	// ядра, выключающий постквантовый слой, и без этого атрибута правило
	// значения судило бы его как строку грамматики и хоронило узел за
	// выключенную настройку.
	//
	// Сравнение ТОЧНОЕ и только строковое — атрибут повторяет литерал ядра
	// буква в букву. Ядро сличает свой `none` с учётом регистра, поэтому
	// `None` сюда не попадает и уходит на общие правила: это не выключатель,
	// а негодное значение.
	AbsentValues []interface{} `json:"absent_values"`
	// AbsentWhen — условие, при котором ОБЪЕКТ считается НЕ ЗАДАННЫМ: если
	// перечисленные ключи объекта равны указанным значениям, объект снимается
	// ЦЕЛИКОМ и ТИХО. Это запись «настройки нет», а не ошибка.
	//
	// Нужен там, где выключатель секции лежит ВНУТРИ неё самой: `tls:
	// {enabled: false}` у ядра значит «TLS не задан» — конструктор возвращает
	// (nil, nil), — а не «TLS с выключенным флагом». Тем же атрибутом
	// описываются вложенные utls / reality / ech со своим `enabled: false`;
	// особого случая ни у одного из них нет. `AbsentValues` это не выражает:
	// он про значение САМОГО поля и только строковый.
	//
	// Порядок исполнения нормативен (CANON §6): объект снимается ДО правил
	// своих полей и ДО связей соседей (conflicts/requires/forbidden_for), для
	// которых он после этого «не задан».
	//
	// Сравнение — по печатной форме скаляра, как у `values` и `advisory`:
	// `false` и `"false"` совпадают, потому что тело приезжает и разбором
	// JSON, и от маппера, где булев флаг бывает строкой. Несколько ключей =
	// объект снимается, только когда совпали ВСЕ.
	AbsentWhen map[string]interface{} `json:"absent_when"`
	Min        *float64               `json:"min"`
	Max        *float64               `json:"max"`
	Len        *int                   `json:"len"`
	LenParity  string                 `json:"len_parity"`
	Normalize  string                 `json:"normalize"`
	// NormalizeCode — код, который ставится, когда normalize РЕАЛЬНО изменил
	// значение (не просто обрезал пробелы или регистр). Нужен чистке, которая
	// теряет данные: hex_only выбрасывает не-hex руны, и `0x1a2` становится
	// `01a2` — другим short_id. Сказать об этом обязаны все входы, а не
	// только URI-путь (DRIFT §2(b), «код ставить на всех путях»).
	NormalizeCode string `json:"normalize_code"`

	// Поведение.
	Required     bool        `json:"required"`
	Secret       bool        `json:"secret"`
	Tristate     bool        `json:"tristate"`
	AllOrNothing bool        `json:"all_or_nothing"`
	Managed      bool        `json:"managed"`
	Deprecated   bool        `json:"deprecated"`
	Skip         string      `json:"skip"`
	Default      interface{} `json:"default"`
	OnInvalid    *OnInvalid  `json:"on_invalid"`
	Advisory     []Advisory  `json:"advisory"`
	DropAlways   bool        `json:"drop_always"`
	Aliases      interface{} `json:"aliases"`

	// Role — роль поля в узле (контракт 1.1.59): по ней общий код находит
	// поле, не зная имени схемы. Только у поля верхнего уровня тела
	// протокола, каждая роль — не больше одного поля на схему (линтер
	// реестра). Словарь — RoleCredential, RolePrivateKey.
	Role string `json:"role"`
	// DefaultWhen — дефолт, который реестр велит МАТЕРИАЛИЗОВАТЬ явно
	// (SPEC §3.2). Обычные `default` в тело не пишутся: дефолты ядра не
	// материализуются. Исключение — поля, без которых ядро не собирает
	// outbound вовсе: у hysteria v1 отсутствующий up_mbps даёт «missing
	// upload speed» ФАТАЛОМ НА ВЕСЬ config.json, и ссылки сплошь и рядом
	// скорость не несут.
	DefaultWhen *DefaultWhen `json:"default_when"`
	// MaxWhen — УСЛОВНЫЙ потолок значения: обычные `min`/`max` действуют на
	// поле всегда, а этот — только когда выполнено `when`.
	//
	// Нужен ровно там, где потолок диктует не поле, а РОД узла: у AmneziaWG
	// накладные расходы на пакет (junk/padding) делают mtu выше 1280
	// нерабочим — рукопожатие проходит, а данные молча не идут («sendmsg:
	// message too long», память awg-mtu-too-high). У plain WireGuard того же
	// поля потолка нет. Выразить это обычным `max` нельзя: он снял бы mtu у
	// каждого обычного WG-узла.
	MaxWhen *MaxWhen `json:"max_when"`
	// MinWhen — УСЛОВНЫЙ минимум значения, зеркало MaxWhen без исключения по
	// входу: у AWG 3.x паддинг s1..s4 обязан быть не ниже 12 ТОЛЬКО когда
	// задан header_protection_key (nonce шифра заголовка берётся из первых 12
	// байт паддинга). Обычным `min` это не записать — он снял бы паддинг у
	// каждого обычного AmneziaWG-узла, где порога нет вовсе.
	//
	// Исключения по входу тут нет и быть не может: ядро отвергает такую пару
	// на загрузке ВСЕГО конфига, а не одного узла, и «сохранить как написал
	// человек» означало бы оставить пользователя без VPN (в отличие от
	// потолка MTU, где узел собирается и работает хуже).
	MinWhen *MinWhen `json:"min_when"`
	// СНЯТО (контракт 1.1.4): ForbiddenWhen. Атрибут `forbidden_when` был
	// объявлен в SPEC 131 §3.2 и реализован в трёх местах (здесь, в
	// санитайзере, в генераторе доков), но НИ ОДНО поле реестра его так и не
	// понесло, и в `schema/registry_body.schema.json` он не попал вовсе —
	// значит валидатор отверг бы файл, который его использует. Единственный
	// случай, ради которого он задумывался (vless.flow при заданном
	// transport), выражен обычным `conflicts`. Мёртвый атрибут в структуре
	// читается как «так тоже можно» и зовёт написать правило, которое схема
	// не примет.

	// Связи со схемой и другими полями.
	AllowedFor   []string `json:"allowed_for"`
	ForbiddenFor []string `json:"forbidden_for"`
	Code         string   `json:"code"`
	// ForbiddenCodes — код запрета ДЛЯ КОНКРЕТНОЙ СХЕМЫ, когда общий `code`
	// поля не годится по исходу. Один запрет, но два разных смысла: у naive
	// снятое TLS-поле — деградация настройки, о которой стоит знать
	// (severity warning), а на QUIC uTLS/REALITY не применились бы в принципе
	// — узел не пострадал, и код там info. Ключ — имя схемы из forbidden_for;
	// схема без записи берёт общий `code`.
	ForbiddenCodes map[string]string `json:"forbidden_codes"`
	Conflicts      []Relation        `json:"conflicts"`
	Requires       []Relation        `json:"requires"`

	// Гейты сборки.
	MinCore  string `json:"min_core"`
	Platform string `json:"platform"`
	LxOnly   bool   `json:"lx_only"`
	BuildTag string `json:"build_tag"`
	// OnCoreUnsupported — что делать, когда заданное поле не по силам
	// текущему ядру (его `build_tag`/`min_core` не выполнены), контракт
	// 1.1.60. Без атрибута поле снимается полевым гейтом (min_core/platform),
	// узел остаётся. С `drop_node` снимается УЗЕЛ с кодом: поле, без которого
	// узел с сервером не договорится, а ядро конфиг с ним не примет.
	OnCoreUnsupported *OnCoreUnsupported `json:"on_core_unsupported"`
	// RangeForm — свои требования ФОРМЫ-ДИАПАЗОНА значения типа awg_range
	// («N-M»), когда они отличаются от числовой формы (контракт 1.1.60).
	RangeForm *RangeForm `json:"range_form"`
	// Level — уровень расширения протокола, о котором говорит наличие поля
	// (подпись узла); словарь и порядок — `levels` тела схемы. LevelMark —
	// суффикс подписи, который поле добавляет к итоговому уровню.
	Level     string `json:"level"`
	LevelMark string `json:"level_mark"`

	// Тексты.
	DescEn string `json:"desc_en"`
	DescRu string `json:"desc_ru"`
	Impl   string `json:"impl"`

	// Variants заполняется при разрешении ref на вариантную секцию
	// (transports): выбор варианта — по значению дискриминатора в карте.
	Variants      map[string]*Field `json:"-"`
	Discriminator string            `json:"-"`
}

// OnCoreUnsupported — действие, когда протокол, поле или форма значения не
// по силам текущему ядру (контракт 1.1.60). Единственное действие —
// `drop_node`: узел снимается на сборке с кодом Code, конфиг собирается без
// него. Исполняет nodeflow.NodeCoreRefusal по возможностям ядра (теги
// сборки + версия).
type OnCoreUnsupported struct {
	Action string `json:"action"`
	Code   string `json:"code"`
}

// CoreUnsupportedDropNode — действие OnCoreUnsupported: снять узел.
const CoreUnsupportedDropNode = "drop_node"

// RangeForm — требования формы-диапазона («N-M») значения awg_range, у
// которой своя граница ядра и свой уровень (контракт 1.1.60). Числовая форма
// того же поля живёт по атрибутам самого поля.
type RangeForm struct {
	MinCore           string             `json:"min_core"`
	BuildTag          string             `json:"build_tag"`
	Level             string             `json:"level"`
	OnCoreUnsupported *OnCoreUnsupported `json:"on_core_unsupported"`
}

// IsRangeValue — значение awg_range записано формой-диапазоном «N-M»
// (строкой с дефисом); число и голое число строкой — числовая форма.
func IsRangeValue(v interface{}) bool {
	s, ok := v.(string)
	return ok && strings.Contains(strings.TrimSpace(s), "-")
}

// OnInvalid — что делать со значением, не прошедшим ограничение поля.
//
// Action `unwrap` (контракт 1.1.57) — значение приехало ОБЁРТКОЙ соседнего
// диалекта: объектом там, где поле ждёт скаляр (у hysteria v1 obfs —
// строка-секрет, а у hysteria2 тот же ключ — объект {type, password}). Если
// значение — объект и его член `key` приводится к типу поля и проходит его
// ограничения, поле получает этот член и код `code`. Иначе поле снимается:
// объект без годного члена — с кодом `else_code` (параметры кода берутся из
// скалярных членов объекта, так `{type}` доезжает до текста), не объект — с
// `type_invalid`, как у поля без on_invalid.
type OnInvalid struct {
	Action   string      `json:"action"`
	Value    interface{} `json:"value"`
	Code     string      `json:"code"`
	Key      string      `json:"key"`
	ElseCode string      `json:"else_code"`
}

// OnItemInvalid — что делать с ЭЛЕМЕНТОМ списка, не прошедшим `item_pattern`.
//
// Исход один — элемент выбрасывается, остальные остаются: списки существуют
// затем, чтобы годная часть доехала до ядра, а снять поле целиком умеет уже
// `pattern`. Поэтому `action` необязателен, а перечень значений закрыт одним
// `drop_item`: второе значение здесь означало бы «уронить то, что и так
// умеет уронить сосед».
type OnItemInvalid struct {
	Action string `json:"action"`
	Code   string `json:"code"`
}

// Advisory — значения, которые ядро принимает, но узел получает код.
//
// `except` — значения-исключения; `when` — условие по соседнему полю. Пара
// нужна правилам вида «отпечаток вне гибридного набора, НО только когда у
// узла есть reality» (reality_fp_not_chrome, D-119): перечислять в `values`
// весь остальной словарь значило бы дублировать enum, а без условия код
// вешался бы на каждый plain-TLS узел.
type Advisory struct {
	Values []interface{} `json:"values"`
	Except []interface{} `json:"except"`
	When   *Relation     `json:"when"`
	Code   string        `json:"code"`
}

// DefaultWhen — правило явной подстановки дефолта.
//
// `absent: true` — подставить, когда поля нет вовсе (единственная форма,
// которая сегодня нужна). `value` — что подставить; `code` — код, которым об
// этом сообщить, если сообщать стоит; `when` — условие, при котором правило
// вообще применяется (у wireguard.mtu дефолт 1280 — только для AmneziaWG,
// plain WG обходится дефолтом ядра и поля не получает вовсе).
type DefaultWhen struct {
	Absent bool        `json:"absent"`
	Value  interface{} `json:"value"`
	Code   string      `json:"code"`
	When   *Condition  `json:"when"`
}

// MaxWhen — условный потолок значения (см. Field.MaxWhen).
//
// `except_sources` — входы, на которых потолок НЕ применяется: значение
// остаётся как пришло, а узел получает информационный код `note_code`.
// Основание не техническое, а по владению: тело в форме ядра (sing-box JSON)
// человек или подписка написали САМИ и сознательно, и переписывать его молча
// лаунчер не вправе — он предупреждает. Значение из ссылки/.conf сочинял
// генератор провайдера, там правило работает заменой (решение владельца
// 18.09.2026, DRIFT §7.23).
type MaxWhen struct {
	Max  float64    `json:"max"`
	Code string     `json:"code"`
	When *Condition `json:"when"`
	// ExceptSources — имена входов из `sources` схемы (uri|singbox|xray|
	// wgconf|amnezia).
	ExceptSources []string `json:"except_sources"`
	NoteCode      string   `json:"note_code"`
}

// MinWhen — условный минимум значения (см. Field.MinWhen).
//
// `action` — что делать со значением ниже порога: "drop" снимает поле,
// "drop_node" хоронит узел. Второе нужно там, где ядро отвергает такую пару
// на загрузке всего конфига: снять поле значило бы отдать ядру узел, который
// оно всё равно не примет, — и уронить чужие узлы вместе с ним.
//
// Порог действует и на ОТСУТСТВУЮЩЕЕ поле, когда стоит `absent_is_zero`:
// ядро читает незаданный s2 как 0, то есть «паддинга нет», и пара
// «ключ заголовка + нет паддинга» так же фатальна, как «ключ + паддинг 5».
type MinWhen struct {
	Min          float64    `json:"min"`
	Code         string     `json:"code"`
	Action       string     `json:"action"`
	AbsentIsZero bool       `json:"absent_is_zero"`
	When         *Condition `json:"when"`
}

// Condition — условие применимости правила значения.
//
// `any_set` — «задано ЛЮБОЕ из перечисленных полей». Одного `Relation.Path`
// здесь мало: «узел AmneziaWG» — это не одно поле, а набор из двадцати с
// лишним, любого из которых достаточно. Пути — от корня тела, как и везде в
// реестре.
//
// `source_kind` — РОД узла, объявленный ВХОДОМ (маппер называет его
// атрибутом `kind_when` по написанию схемы и форме; в тело род не пишется).
// Нужен там, где `any_set` по телу бессилен ПО ПОСТРОЕНИЮ: вход попросил
// AmneziaWG, но ни одно awg-поле не уцелело (негодные значения сняло правило
// поля), и тело стало неотличимо от обычного WireGuard — а потолок MTU есть
// свойство ЗАПРОШЕННОГО протокола, без которого туннель поднимается и не
// несёт данных.
//
// Перечислены оба ключа — условие верно, когда верен ЛЮБОЙ (ИЛИ, не И):
// у входа `singbox` рода от входа нет вовсе, и судить там можно только по
// телу, поэтому `any_set` из правила не уходит никогда.
//
// Остальные ключи — ПУТИ тела с предикатом по ЗНАЧЕНИЮ (контракт 1.1.56):
// скаляр (равенство по печатной форме) либо оператор `{"in": […]}` /
// `{"not_in": […]}` — та же грамматика, что у `when` маппера (PRIMITIVES
// §0.13: второго имени для одной операции не заводится). Предикаты путей
// связаны И между собой и И с ветками any_set/source_kind: правило «дописать
// mode=packet-up» действует только при `uplink_data_placement ∈ {header,
// cookie}` — ядро отвергает вне packet-up только эти два размещения, а
// `body`/`auto` законны в любом режиме. Значение читается из чистой карты
// (в том числе объекта, обход которого ещё идёт), иначе из исходной;
// пустая строка и снятое поле — «не задано»: `in` ложен, `not_in` истинен.
type Condition struct {
	AnySet     []string `json:"any_set"`
	SourceKind []string `json:"source_kind"`
	// Values — предикаты по значению путей тела (см. выше). Ключи — пути от
	// корня, значения — скаляр либо объект-оператор.
	Values map[string]interface{} `json:"-"`
}

// UnmarshalJSON читает известные ключи в поля, остальные — как предикаты
// путей (Values). Отдельного имени для словаря предикатов не заводится: путь
// тела с точкой ни с `any_set`, ни с `source_kind` не столкнётся.
func (c *Condition) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*c = Condition{}
	for key, val := range raw {
		switch key {
		case "any_set":
			if err := json.Unmarshal(val, &c.AnySet); err != nil {
				return fmt.Errorf("condition.any_set: %w", err)
			}
		case "source_kind":
			if err := json.Unmarshal(val, &c.SourceKind); err != nil {
				return fmt.Errorf("condition.source_kind: %w", err)
			}
		default:
			var v interface{}
			if err := json.Unmarshal(val, &v); err != nil {
				return fmt.Errorf("condition.%s: %w", key, err)
			}
			if c.Values == nil {
				c.Values = map[string]interface{}{}
			}
			c.Values[key] = v
		}
	}
	return nil
}

// Relation — связь поля с другим полем (conflicts / requires / advisory.when).
type Relation struct {
	With    string `json:"with"`
	Path    string `json:"path"`
	Present *bool  `json:"present"`
	// Equals — связь не по наличию соседа, а по его ЗНАЧЕНИЮ: поле осмыслено
	// только при таком-то варианте дискриминатора (obfs.min_packet_size есть
	// только у gecko). Без него такие пары описывались бы «наличием», а
	// дискриминатор присутствует всегда.
	Equals interface{} `json:"equals"`
	// UnlessSet — связь НЕ действует, если задан любой из этих путей (от
	// корня тела). Нужна там, где несовместимость снимает третье поле: Vision
	// поверх транспорта бессмыслен, но при слое VLESS Encryption он работает
	// поверх шифрования и транспорт ему не важен.
	UnlessSet []string `json:"unless_set"`
	// When — условие действия связи (контракт 1.1.56): та же Condition, что
	// у правил значения. Нужно там, где связь зависит от СОБСТВЕННОГО
	// значения поля: `uplink_data_placement` требует mode=packet-up только
	// при header/cookie, а body/auto ядро принимает в любом режиме.
	When *Condition `json:"when"`
	Code string     `json:"code"`
}

// section — секция body/common одного файла реестра, как она лежит на диске.
type section struct {
	Core   string            `json:"core"`
	Order  []string          `json:"order"`
	Fields map[string]*Field `json:"fields"`
	// Требования протокола к ядру (у тела протокола): тег сборки и версия.
	// Исполняются, только когда объявлено OnCoreUnsupported (контракт
	// 1.1.60): у прочих схем атрибуты описательные.
	MinCore           string             `json:"min_core"`
	BuildTag          string             `json:"build_tag"`
	OnCoreUnsupported *OnCoreUnsupported `json:"on_core_unsupported"`
	// Levels — словарь уровней расширения протокола по возрастанию (подпись
	// узла, контракт 1.1.60): поля и формы диапазона ссылаются на него
	// атрибутом `level`.
	Levels []string `json:"levels"`
	// AbsentWhen — условие «этой секции нет» для суб-схемы, которую схемы
	// подключают через `ref` (tls). Живёт у СЕКЦИИ, а не у ссылающегося поля:
	// «tls:{enabled:false} = TLS не задан» — правило самой секции, и повторять
	// его в каждом из полутора десятков протоколов значило бы завести ровно ту
	// копию, от которой кампания уходит. При разрешении ref условие переезжает
	// в поле-объект (refAsObject).
	AbsentWhen    map[string]interface{} `json:"absent_when"`
	Skipped       map[string]string      `json:"skipped"`
	Relations     []Relation2            `json:"relations"`
	Discriminator string                 `json:"discriminator"`
	Values        []string               `json:"values"`
	Variants      map[string]*struct {
		Order  []string          `json:"order"`
		Fields map[string]*Field `json:"fields"`
	} `json:"variants"`
}

// Relation2 — именованная связь МЕЖДУ НЕСКОЛЬКИМИ полями тела, которую
// атрибутами одного поля не записать.
//
// `conflicts`/`requires` живут у поля и говорят про пару «я и сосед». Есть
// связи другого рода: у AmneziaWG диапазоны magic-заголовков h1..h4 обязаны
// НЕ пересекаться попарно — это свойство всего набора, и повесить его на h1
// значило бы соврать (виноват может быть любой из четырёх, а снятие h1 пару
// не развело бы).
//
// Виды: `ranges_disjoint` (набор судится по ОДНОМУ свойству — попарной
// непересекаемости) и `cooccurrence` (разнородное условие `when` над разными
// полями: булев флаг и ширина диапазона в одной связи). Новый вид заводится
// вместе с его исполнением в санитайзере: неизвестный вид пропускается молча
// (реестр вправе уехать вперёд кода), и правило-опечатка не должна ронять
// узлы.
type Relation2 struct {
	Kind   string   `json:"kind"`
	Paths  []string `json:"paths"`
	Action string   `json:"action"`
	Code   string   `json:"code"`
	// When — условие срабатывания связи (обязательно у `cooccurrence`, где
	// само по себе сочетание полей ещё не повод для кода). Ключи — пути из
	// Paths, значения — скаляр либо служебный оператор с `$` в имени
	// (`$range_width`); служебное имя с путём тела не сталкивается.
	When map[string]interface{} `json:"when"`
	// Defaults — значение поля, когда ключа в теле нет: у h1..h4 незаданный
	// заголовок равен своему типу сообщения WireGuard (h1=1 … h4=4), и
	// «поля нет» здесь не значит «участника нет».
	Defaults []float64 `json:"defaults"`
	DescEn   string    `json:"desc_en"`
	DescRu   string    `json:"desc_ru"`
	Impl     string    `json:"impl"`
}

// file — файл реестра: секции body/common разбираются структурами, всё
// остальное остаётся сырым (оттуда берётся таблица maps_to).
type file struct {
	Body   *section
	Common *section
	Raw    map[string]json.RawMessage
}

func (f *file) UnmarshalJSON(data []byte) error {
	f.Raw = map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &f.Raw); err != nil {
		return err
	}
	if b, ok := f.Raw["body"]; ok {
		f.Body = &section{}
		if err := json.Unmarshal(b, f.Body); err != nil {
			return err
		}
	}
	if c, ok := f.Raw["common"]; ok {
		f.Common = &section{}
		if err := json.Unmarshal(c, f.Common); err != nil {
			return err
		}
	}
	return nil
}

// BodySchema — разрешённая схема тела одной схемы протокола.
type BodySchema struct {
	Scheme string
	Core   string
	Order  []string
	Fields map[string]*Field
	// Relations — связи между несколькими полями тела (см. Relation2).
	Relations []Relation2
	// Требования протокола к ядру и действие, когда они не выполнены
	// (контракт 1.1.60), — см. section.
	MinCore           string
	BuildTag          string
	OnCoreUnsupported *OnCoreUnsupported
	// Levels — словарь уровней расширения по возрастанию (см. section).
	Levels []string
}

// WarningEntry — запись кода из registry/warnings.json.
//
// Пара «текст / причина / что делать» — три разных вопроса, и путать их
// нельзя: TextEn говорит, ЧТО случилось с узлом, CauseEn — откуда такое
// значение обычно берётся (кривая подписка, чужой диалект, старое ядро), а
// FixEn — что человек может сделать. У info-кодов честный Fix — «ничего не
// нужно».
type WarningEntry struct {
	Severity string   `json:"severity"`
	Params   []string `json:"params"`
	TitleEn  string   `json:"title_en"`
	TitleRu  string   `json:"title_ru"`
	TextEn   string   `json:"text_en"`
	TextRu   string   `json:"text_ru"`
	CauseEn  string   `json:"cause_en"`
	CauseRu  string   `json:"cause_ru"`
	FixEn    []string `json:"fix_en"`
	FixRu    []string `json:"fix_ru"`
	Desc     string   `json:"desc"`
	Doc      string   `json:"doc"`
}

// Limit — запись из registry/limits.json.
type Limit struct {
	Value interface{} `json:"value"`
	Note  string      `json:"note"`
	Impl  string      `json:"impl"`
}

// Registry — весь реестр в разрешённом виде.
type Registry struct {
	bodies   map[string]*BodySchema
	warnings map[string]*WarningEntry
	limits   map[string]Limit
	lists    map[string][]string
	mapsTo   map[string]map[string][]string
	// aliases — имена, под которыми параметр ссылки встречается в реальных
	// подписках: схема → канонический параметр → все его написания
	// (канон первым). Таблица живёт в реестре (`uri.query.<param>.aliases`),
	// а не в коде парсера: до SPEC 131 W2d набор написаний `insecure`
	// отличался в шести парсерах, и один и тот же узел терял `allow_insecure`
	// на одном пути и принимал на другом (DRIFT §2(a)).
	aliases map[string]map[string][]string
	schemes []string
	// singboxTypes — schema → тип узла в config.json ("ss" → "shadowsocks").
	// Тип пишет СБОРКА, а не тело (см. buildManagedKeys в nodeflow), поэтому
	// материализация тела берёт его отсюда, а не из входной карты: иначе
	// ручной JSON диктовал бы имя типа сам.
	singboxTypes map[string]string
	// schemeByType — обратная карта для входа «ручной JSON-объект»: у него
	// схемы нет, есть только "type" тела.
	schemeByType map[string]string
	// kinds — схема → `kind` протокола (outbound | endpoint | group):
	// в какую секцию config.json эмитится узел.
	kinds map[string]string
	// sources — схема → входы, которыми узел приходит (`sources` файла
	// протокола). Схема без входа "singbox" узлом из чужого конфига не
	// становится, даже если её тип ядру известен.
	sources map[string][]string
}

// protocolFiles — схемы протоколов реестра. Список явный: embed.FS читается
// и через ReadDir, но порядок обхода должен быть предсказуем, а появление
// нового файла — осознанным (линтер реестра держит тот же список).
var protocolFiles = []string{
	"anytls", "chain", "http", "hysteria", "hysteria2", "masque",
	"naive", "shadowsocks", "socks", "ssh", "tailscale", "trojan", "tuic",
	"vless", "vmess", "wireguard",
}

var (
	once   sync.Once
	cached *Registry
	cerr   error
)

// Get возвращает реестр из кэша, загружая его при первом обращении.
func Get() (*Registry, error) {
	once.Do(func() { cached, cerr = Load() })
	return cached, cerr
}

// MustGet — Get с паникой на ошибке: реестр вшит в бинарь, и его порча
// означает сломанную сборку, а не ситуацию рантайма.
func MustGet() *Registry {
	r, err := Get()
	if err != nil {
		panic("registry: " + err.Error())
	}
	return r
}

// Load читает реестр из вшитых файлов контракта и разрешает ссылки.
func Load() (*Registry, error) {
	subs := map[string]*section{}
	for _, name := range []string{"tls", "transports", "multiplex", "dialer"} {
		f, err := readFile(name + ".json")
		if err != nil {
			return nil, err
		}
		if f.Body == nil {
			return nil, fmt.Errorf("registry: %s.json: нет секции body", name)
		}
		subs[name] = f.Body
		if f.Common != nil {
			subs[name+".common"] = f.Common
		}
	}

	reg := &Registry{
		bodies:       make(map[string]*BodySchema, len(protocolFiles)),
		warnings:     map[string]*WarningEntry{},
		limits:       map[string]Limit{},
		lists:        map[string][]string{},
		mapsTo:       map[string]map[string][]string{},
		aliases:      map[string]map[string][]string{},
		singboxTypes: map[string]string{},
		schemeByType: map[string]string{},
		kinds:        map[string]string{},
		sources:      map[string][]string{},
	}

	for _, name := range protocolFiles {
		f, err := readFile("protocols/" + name + ".json")
		if err != nil {
			return nil, err
		}
		if f.Body == nil {
			return nil, fmt.Errorf("registry: protocols/%s.json: нет секции body", name)
		}
		// Схема зовётся так, как объявлено ВНУТРИ файла, а не как назван
		// файл: shadowsocks.json несёт scheme "ss", и раздача по имени файла
		// оставляла бы половину лаунчера без правил (ss, socks5, wg, awg —
		// живые схемы парсеров). Алиасы ведут на ту же схему тела.
		scheme := strings.TrimSpace(schemeNameOf(f.Raw))
		if scheme == "" {
			scheme = name
		}
		body, err := resolveSection(scheme, f.Body, subs)
		if err != nil {
			return nil, err
		}
		reg.bodies[scheme] = body
		reg.schemes = append(reg.schemes, scheme)
		reg.mapsTo[scheme] = collectMapsTo(f.Raw)
		reg.aliases[scheme] = collectAliases(f.Raw)
		reg.kinds[scheme] = strings.TrimSpace(rawString(f.Raw, "kind"))
		reg.sources[scheme] = rawStringSlice(f.Raw, "sources")
		sbType := strings.TrimSpace(rawString(f.Raw, "singbox_type"))
		if sbType != "" && !strings.Contains(sbType, "|") {
			reg.singboxTypes[scheme] = sbType
			reg.schemeByType[sbType] = scheme
		}
		// Имя файла и singbox_type тоже ведут на схему: реестр адресуют и по
		// схеме лаунчера ("ss"), и по типу ядра ("shadowsocks").
		aliases := append([]string{name, sbType}, rawStringSlice(f.Raw, "aliases")...)
		for _, alias := range aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" || alias == scheme {
				continue
			}
			if _, taken := reg.bodies[alias]; taken {
				continue
			}
			reg.bodies[alias] = body
			reg.mapsTo[alias] = reg.mapsTo[scheme]
			reg.aliases[alias] = reg.aliases[scheme]
			reg.kinds[alias] = reg.kinds[scheme]
			reg.sources[alias] = reg.sources[scheme]
			if sbType != "" {
				reg.singboxTypes[alias] = sbType
			}
		}
	}

	// Общие суб-схемы дают maps_to тоже (tls.json, transports.json).
	for _, name := range []string{"tls", "transports"} {
		f, err := readFile(name + ".json")
		if err != nil {
			return nil, err
		}
		shared := collectMapsTo(f.Raw)
		for scheme := range reg.mapsTo {
			for param, paths := range shared {
				if _, ok := reg.mapsTo[scheme][param]; !ok {
					reg.mapsTo[scheme][param] = paths
				}
			}
		}
		// Написания общих параметров (sni, insecure, fp, host транспортов)
		// схема переопределяет своими, если объявила: у tuic `allow_insecure`
		// канон, а у остальных он алиас `insecure`.
		sharedAliases := collectAliases(f.Raw)
		for scheme := range reg.aliases {
			for param, names := range sharedAliases {
				if _, ok := reg.aliases[scheme][param]; !ok {
					reg.aliases[scheme][param] = names
				}
			}
		}
	}

	if err := readJSON("warnings.json", &struct {
		Warnings *map[string]*WarningEntry `json:"warnings"`
	}{Warnings: &reg.warnings}); err != nil {
		return nil, err
	}
	if err := readJSON("limits.json", &struct {
		Limits *map[string]Limit `json:"limits"`
	}{Limits: &reg.limits}); err != nil {
		return nil, err
	}
	var lists struct {
		Allowlists map[string]struct {
			Values []string `json:"values"`
			Note   string   `json:"note"`
		} `json:"allowlists"`
	}
	if err := readJSON("allowlists.json", &lists); err != nil {
		return nil, err
	}
	for name, l := range lists.Allowlists {
		reg.lists[name] = l.Values
	}
	return reg, nil
}

func readFile(name string) (*file, error) {
	data, err := contract.ReadRegistry(name)
	if err != nil {
		return nil, fmt.Errorf("registry: %s: %w", name, err)
	}
	f := &file{}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("registry: %s: %w", name, err)
	}
	return f, nil
}

func readJSON(name string, dst interface{}) error {
	data, err := contract.ReadRegistry(name)
	if err != nil {
		return fmt.Errorf("registry: %s: %w", name, err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("registry: %s: %w", name, err)
	}
	return nil
}

// resolveSection разрешает ссылки схемы протокола: `ref` подставляется
// содержимым суб-схемы, inline-ссылка (`__dialer`) вливается плоско в конец
// order, вариантная суб-схема (транспорты) едет как Variants.
func resolveSection(scheme string, sec *section, subs map[string]*section) (*BodySchema, error) {
	out := &BodySchema{
		Scheme:    scheme,
		Core:      sec.Core,
		Order:     make([]string, 0, len(sec.Order)),
		Fields:    make(map[string]*Field, len(sec.Fields)),
		Relations: sec.Relations,

		MinCore:           sec.MinCore,
		BuildTag:          sec.BuildTag,
		OnCoreUnsupported: sec.OnCoreUnsupported,
		Levels:            sec.Levels,
	}
	for _, name := range sec.Order {
		src := sec.Fields[name]
		if src == nil {
			return nil, fmt.Errorf("registry: %s: order упоминает %q, которого нет в fields", scheme, name)
		}
		if src.Type != "ref" {
			out.Order = append(out.Order, name)
			out.Fields[name] = src
			continue
		}
		sub := subs[sectionOfRef(src.Ref)]
		if sub == nil {
			return nil, fmt.Errorf("registry: %s.%s: неизвестный ref %q", scheme, name, src.Ref)
		}
		if src.Inline {
			// Поля суб-схемы вливаются в тело плоско, без обёртки.
			for _, inner := range sub.Order {
				f := sub.Fields[inner]
				if f == nil {
					continue
				}
				if _, dup := out.Fields[inner]; dup {
					continue
				}
				out.Order = append(out.Order, inner)
				out.Fields[inner] = f
			}
			continue
		}
		if sub.Variants == nil && len(sub.Fields) > 0 && strings.Contains(src.Ref, ".") {
			f := resolveNamedRef(name, src, sub)
			if f == nil {
				return nil, fmt.Errorf("registry: %s.%s: в суб-схеме %q нет подходящего поля", scheme, name, src.Ref)
			}
			out.Order = append(out.Order, name)
			out.Fields[name] = mergeRefAttrs(src, f)
			continue
		}
		resolved, err := refAsObject(src, sub)
		if err != nil {
			return nil, fmt.Errorf("registry: %s.%s: %w", scheme, name, err)
		}
		out.Order = append(out.Order, name)
		out.Fields[name] = resolved
	}
	return out, nil
}

// refAsObject превращает ссылку на суб-схему в поле-объект: обычная секция
// даёт order+fields, вариантная (транспорты) — Variants с дискриминатором.
func refAsObject(src *Field, sub *section) (*Field, error) {
	f := &Field{
		Type:     "object",
		Required: src.Required,
		Code:     src.Code,
		DescEn:   src.DescEn,
		DescRu:   src.DescRu,
		Impl:     src.Impl,
		// Условие «секции нет» объявлено один раз, у самой суб-схемы, и
		// переезжает в каждое поле, которое её подключает: tls:{enabled:false}
		// значит «TLS не задан» у любой схемы, а не у той, где не забыли
		// переписать атрибут.
		AbsentWhen: sub.AbsentWhen,
	}
	if len(sub.Variants) == 0 {
		f.Order = sub.Order
		f.Fields = sub.Fields
		return f, nil
	}
	if sub.Discriminator == "" {
		return nil, fmt.Errorf("вариантная суб-схема без discriminator")
	}
	f.Discriminator = sub.Discriminator
	f.Variants = make(map[string]*Field, len(sub.Variants))
	for name, v := range sub.Variants {
		f.Variants[name] = &Field{Type: "object", Order: v.Order, Fields: v.Fields}
	}
	return f, nil
}

// sectionOfRef — имя секции суб-схемы в ссылке. Ссылка бывает двух форм:
// "dialer.common" (поле берётся по собственному имени) и
// "dialer.common.network" (поле названо явно) — вторая форма нужна там, где
// имя поля у схемы своё (masque.network_list).
func sectionOfRef(ref string) string {
	parts := strings.Split(ref, ".")
	if len(parts) >= 3 {
		return strings.Join(parts[:2], ".")
	}
	return ref
}

// resolveNamedRef ищет в суб-схеме поле, на которое ссылается ref.
//
// Порядок: явное имя в ссылке ("dialer.common.network") → собственное имя
// поля → единственное поле суб-схемы с тем же desc_en. Последний шаг — мост
// для masque.network_list, где реестр называет поле иначе, чем суб-схема, а
// явной формы ссылки в контракте 1.1.0 ещё нет; совпадение ищется по данным
// (одинаковый desc_en), и неоднозначность считается ошибкой реестра.
func resolveNamedRef(name string, src *Field, sub *section) *Field {
	if parts := strings.Split(src.Ref, "."); len(parts) >= 3 {
		return sub.Fields[strings.Join(parts[2:], ".")]
	}
	if f := sub.Fields[name]; f != nil {
		return f
	}
	if src.DescEn == "" {
		return nil
	}
	var found *Field
	for _, inner := range sub.Order {
		f := sub.Fields[inner]
		if f == nil || f.DescEn != src.DescEn {
			continue
		}
		if found != nil {
			return nil // неоднозначно — реестр обязан назвать поле явно
		}
		found = f
	}
	return found
}

// mergeRefAttrs накладывает атрибуты ссылки на поле суб-схемы: ссылка может
// ужесточить обязательность (naive: tls required), но не подменяет правила.
func mergeRefAttrs(src, target *Field) *Field {
	if !src.Required && src.Code == "" && len(src.ForbiddenFor) == 0 && len(src.AllowedFor) == 0 && len(src.ForbiddenCodes) == 0 {
		return target
	}
	cp := *target
	if src.Required {
		cp.Required = true
	}
	if src.Code != "" {
		cp.Code = src.Code
	}
	if len(src.ForbiddenFor) > 0 {
		cp.ForbiddenFor = src.ForbiddenFor
	}
	if len(src.ForbiddenCodes) > 0 {
		cp.ForbiddenCodes = src.ForbiddenCodes
	}
	if len(src.AllowedFor) > 0 {
		cp.AllowedFor = src.AllowedFor
	}
	return &cp
}

// collectMapsTo собирает таблицу «параметр ссылки → путь в теле» из файла
// реестра (SPEC 131 §3.1). Параметры лежат в разных секциях и на разной
// глубине — uri.query.sni у протокола, tls.params.sni и tls.reality.pbk в
// общей суб-схеме, transports.<type>.<param> у транспортов, — поэтому обход
// рекурсивный по всему файлу: ключ таблицы = имя параметра ссылки, значение =
// пути в теле. Секция body исключена: там maps_to не бывает, а обходить её
// незачем.
func collectMapsTo(raw map[string]json.RawMessage) map[string][]string {
	out := map[string][]string{}
	var walk func(name string, node interface{})
	walk = func(name string, node interface{}) {
		m, ok := node.(map[string]interface{})
		if !ok {
			return
		}
		if to, ok := m["maps_to"].(string); ok && to != "" && name != "" {
			found := false
			for _, existing := range out[name] {
				if existing == to {
					found = true
					break
				}
			}
			if !found {
				out[name] = append(out[name], to)
			}
		}
		for k, v := range m {
			switch k {
			case "maps_to", "aliases", "values", "allowlist", "default",
				"desc_en", "desc_ru", "impl", "note", "meaning", "type":
				continue
			}
			walk(k, v)
		}
	}
	for section, body := range raw {
		if section == "body" || section == "common" {
			continue
		}
		var node interface{}
		if err := json.Unmarshal(body, &node); err != nil {
			continue
		}
		walk("", node)
	}
	return out
}

// collectAliases собирает таблицу написаний параметра ссылки из файла
// реестра: секции `uri.query.<param>.aliases` и общие `tls.params.*`,
// `transports.<type>.params.*` (SPEC 131 W2d).
//
// Записи реестра писались людьми и местами несут пояснение прямо в строке
// («host (только trojan, 3-я ступень)», «хвост пути '?ed=N'»). Имя параметра —
// это первое слово до пробела или скобки; строка, где после чистки не
// осталось имени параметра (пробелы внутри, кириллица), отбрасывается: такая
// запись описывает форму, а не второе написание ключа.
func collectAliases(raw map[string]json.RawMessage) map[string][]string {
	out := map[string][]string{}
	var walk func(name string, node interface{})
	walk = func(name string, node interface{}) {
		m, ok := node.(map[string]interface{})
		if !ok {
			return
		}
		if list, ok := m["aliases"].([]interface{}); ok && name != "" {
			names := []string{name}
			for _, item := range list {
				s, ok := item.(string)
				if !ok {
					continue
				}
				if alias := aliasName(s); alias != "" {
					names = appendUnique(names, alias)
				}
			}
			if len(names) > 1 {
				for _, existing := range out[name] {
					names = appendUnique(names, existing)
				}
				out[name] = names
			}
		}
		for k, v := range m {
			switch k {
			case "maps_to", "aliases", "values", "allowlist", "default",
				"desc_en", "desc_ru", "impl", "note", "meaning", "type":
				continue
			}
			walk(k, v)
		}
	}
	for section, body := range raw {
		if section == "body" || section == "common" {
			continue
		}
		var node interface{}
		if err := json.Unmarshal(body, &node); err != nil {
			continue
		}
		walk("", node)
	}
	return out
}

// aliasName достаёт имя параметра из записи `aliases`, отбрасывая пояснение
// в скобках. Пустая строка = запись не про имя ключа.
func aliasName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '('); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if s == "" || strings.ContainsAny(s, " \t'\"") {
		return ""
	}
	for _, r := range s {
		if r > 127 {
			return ""
		}
	}
	return s
}

func appendUnique(list []string, v string) []string {
	for _, existing := range list {
		if strings.EqualFold(existing, v) {
			return list
		}
	}
	return append(list, v)
}

// schemeNameOf — имя схемы, объявленное в файле реестра.
func schemeNameOf(raw map[string]json.RawMessage) string {
	return rawString(raw, "scheme")
}

func rawString(raw map[string]json.RawMessage, key string) string {
	b, ok := raw[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return ""
	}
	return s
}

func rawStringSlice(raw map[string]json.RawMessage, key string) []string {
	b, ok := raw[key]
	if !ok {
		return nil
	}
	var out []string
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// Body возвращает разрешённую схему тела схемы протокола.
func (r *Registry) Body(scheme string) (*BodySchema, bool) {
	b, ok := r.bodies[scheme]
	return b, ok
}

// SingboxType — значение ключа "type" в config.json для этой схемы
// ("ss" → "shadowsocks"). Пустая строка = схема реестру неизвестна либо у
// неё несколько типов (группы: selector|urltest).
func (r *Registry) SingboxType(scheme string) string {
	return r.singboxTypes[scheme]
}

// SchemeForSingboxType — обратное направление, для входа «ручной
// JSON-объект»: схемы у него нет, есть только "type" в теле.
func (r *Registry) SchemeForSingboxType(t string) (string, bool) {
	s, ok := r.schemeByType[strings.ToLower(strings.TrimSpace(t))]
	return s, ok
}

// SourceSingbox — имя входа «тело в форме ядра» в `sources` протокола
// (импорт sing-box JSON, ручной JSON, тело из состояния или бэкапа).
const SourceSingbox = "singbox"

// NodeSchemeForSingboxType — схема УЗЛА по типу тела ядра: обратная карта
// `singbox_type`, суженная до схем, которые принимают вход "singbox"
// (`sources`). Тип, известный ядру, но не приходящий узлом (chain — тело
// рождается в форме, `sources` у схемы нет), узлом не становится: false.
func (r *Registry) NodeSchemeForSingboxType(t string) (string, bool) {
	s, ok := r.SchemeForSingboxType(t)
	if !ok || !r.AcceptsSource(s, SourceSingbox) {
		return "", false
	}
	return s, true
}

// AcceptsSource — приходит ли узел схемы входом source (`sources`).
func (r *Registry) AcceptsSource(scheme, source string) bool {
	for _, v := range r.sources[scheme] {
		if v == source {
			return true
		}
	}
	return false
}

// KindEndpoint — `kind` протокола, чьи узлы живут в endpoints[] config.json.
const KindEndpoint = "endpoint"

// ProtocolKind — `kind` протокола: "outbound", "endpoint" или "group";
// пустая строка — схема реестру неизвестна.
func (r *Registry) ProtocolKind(scheme string) string {
	return r.kinds[scheme]
}

// Schemes — схемы реестра в порядке загрузки (алфавитном).
func (r *Registry) Schemes() []string {
	out := make([]string, len(r.schemes))
	copy(out, r.schemes)
	return out
}

// Order — порядок полей тела схемы (порядок структур ядра).
func (r *Registry) Order(scheme string) []string {
	b, ok := r.bodies[scheme]
	if !ok {
		return nil
	}
	out := make([]string, len(b.Order))
	copy(out, b.Order)
	return out
}

// Field ищет поле по пути в теле ("tls.reality.short_id"). У вариантного
// узла (transport) имя варианта пишется отдельным сегментом:
// "transport.xhttp.mode".
func (r *Registry) Field(scheme, path string) (*Field, bool) {
	b, ok := r.bodies[scheme]
	if !ok {
		return nil, false
	}
	parts := strings.Split(path, ".")
	fields := b.Fields
	var variants map[string]*Field
	var cur *Field
	for _, p := range parts {
		var next *Field
		if fields != nil {
			next = fields[p]
		}
		if next == nil && variants != nil {
			next = variants[p]
		}
		if next == nil {
			return nil, false
		}
		cur = next
		fields, variants = next.Fields, next.Variants
		if fields == nil && next.Items != nil {
			fields = next.Items.Fields
		}
	}
	return cur, cur != nil
}

// AllowedForScheme — разрешено ли поле схеме по `allowed_for`/`forbidden_for`.
// Единственная трактовка пары атрибутов: ею пользуются и санитайзер, и
// сборка (глобальные трансформы), и формы, — своего списка схем нигде нет.
func (f *Field) AllowedForScheme(scheme string) bool {
	if f == nil {
		return false
	}
	for _, sc := range f.ForbiddenFor {
		if sc == scheme {
			return false
		}
	}
	if len(f.AllowedFor) == 0 {
		return true
	}
	for _, sc := range f.AllowedFor {
		if sc == scheme {
			return true
		}
	}
	return false
}

// FieldAllowed — есть ли у схемы поле по пути и разрешено ли оно ей.
func (r *Registry) FieldAllowed(scheme, path string) bool {
	f, ok := r.Field(scheme, path)
	return ok && f.AllowedForScheme(scheme)
}

// FieldsWithBuildTag — корневые поля тела схемы, которым нужна сборка ядра
// с тегом tag (`build_tag`), в порядке тела. Так форма узнаёт набор полей
// расширения (AmneziaWG: with_awg) из реестра, а не своим списком.
func (r *Registry) FieldsWithBuildTag(scheme, tag string) []string {
	b, ok := r.bodies[scheme]
	if !ok || tag == "" {
		return nil
	}
	var out []string
	for _, name := range b.Order {
		if f := b.Fields[name]; f != nil && f.BuildTag == tag {
			out = append(out, name)
		}
	}
	return out
}

// WalkPresent обходит поля тела схемы, ЗАДАННЫЕ в body (ключ есть и не
// null), в порядке реестра: вложенные объекты, элементы массивов объектов
// (peers[]) и вариант по дискриминатору. path — путь поля ("tls.utls",
// "peers[].persistent_keepalive_interval"). Общий обход для вопросов «что
// в теле говорит о нужных ядру возможностях и об уровне протокола».
func (r *Registry) WalkPresent(scheme string, body map[string]interface{}, fn func(path string, f *Field, v interface{})) {
	b, ok := r.bodies[scheme]
	if !ok || body == nil {
		return
	}
	walkPresent("", b.Order, b.Fields, body, fn)
}

func walkPresent(prefix string, order []string, fields map[string]*Field, m map[string]interface{}, fn func(string, *Field, interface{})) {
	for _, name := range order {
		f := fields[name]
		if f == nil {
			continue
		}
		v, ok := m[name]
		if !ok || v == nil {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		fn(path, f, v)
		switch inner := v.(type) {
		case map[string]interface{}:
			if len(f.Variants) > 0 {
				disc, _ := inner[f.Discriminator].(string)
				if vf := f.Variants[strings.TrimSpace(disc)]; vf != nil {
					walkPresent(path, vf.Order, vf.Fields, inner, fn)
				}
				continue
			}
			if len(f.Fields) > 0 {
				walkPresent(path, f.Order, f.Fields, inner, fn)
			}
		case []interface{}:
			if f.Items == nil || len(f.Items.Fields) == 0 {
				continue
			}
			for _, it := range inner {
				if im, ok := it.(map[string]interface{}); ok {
					walkPresent(path+"[]", f.Items.Order, f.Items.Fields, im, fn)
				}
			}
		case []map[string]interface{}:
			if f.Items == nil || len(f.Items.Fields) == 0 {
				continue
			}
			for _, im := range inner {
				walkPresent(path+"[]", f.Items.Order, f.Items.Fields, im, fn)
			}
		}
	}
}

// Level — подпись уровня расширения протокола по полям тела (контракт
// 1.1.60): старший из уровней заданных полей (`level`) и их форм-диапазонов
// (`range_form.level`) по словарю `levels` схемы, плюс суффиксы
// `level_mark` заданных полей. Схема без `levels` или тело без размеченных
// полей — "".
func (r *Registry) Level(scheme string, body map[string]interface{}) string {
	b, ok := r.bodies[scheme]
	if !ok || len(b.Levels) == 0 {
		return ""
	}
	rank := make(map[string]int, len(b.Levels))
	for i, l := range b.Levels {
		rank[l] = i + 1
	}
	best := 0
	var marks []string
	raise := func(level string) {
		if n := rank[level]; n > best {
			best = n
		}
	}
	r.WalkPresent(scheme, body, func(_ string, f *Field, v interface{}) {
		raise(f.Level)
		if f.RangeForm != nil && IsRangeValue(v) {
			raise(f.RangeForm.Level)
		}
		if f.LevelMark != "" {
			for _, m := range marks {
				if m == f.LevelMark {
					return
				}
			}
			marks = append(marks, f.LevelMark)
		}
	})
	if best == 0 {
		return ""
	}
	return b.Levels[best-1] + strings.Join(marks, "")
}

// StripBuildTag снимает с тела всё, чему нужна сборка ядра с тегом tag
// (контракт 1.1.60): корневые поля с этим `build_tag` удаляются, а
// форма-диапазон awg_range, которой нужен этот тег (`range_form.build_tag`),
// схлопывается в свою нижнюю границу — само поле остаётся настройкой
// соединения, пропадает только расширение. Негодная граница — поле
// снимается.
func (r *Registry) StripBuildTag(scheme string, body map[string]interface{}, tag string) {
	for _, k := range r.FieldsWithBuildTag(scheme, tag) {
		delete(body, k)
	}
	b, ok := r.bodies[scheme]
	if !ok || tag == "" {
		return
	}
	collapseRangeForms(b.Order, b.Fields, body, tag)
}

func collapseRangeForms(order []string, fields map[string]*Field, m map[string]interface{}, tag string) {
	for _, name := range order {
		f := fields[name]
		v, ok := m[name]
		if f == nil || !ok {
			continue
		}
		if f.RangeForm != nil && f.RangeForm.BuildTag == tag && IsRangeValue(v) {
			lo, _, _ := strings.Cut(strings.TrimSpace(v.(string)), "-")
			if n, err := strconv.Atoi(strings.TrimSpace(lo)); err == nil && n > 0 {
				m[name] = n
			} else {
				delete(m, name)
			}
			continue
		}
		switch inner := v.(type) {
		case map[string]interface{}:
			if len(f.Fields) > 0 {
				collapseRangeForms(f.Order, f.Fields, inner, tag)
			}
		case []interface{}:
			if f.Items == nil || len(f.Items.Fields) == 0 {
				continue
			}
			for _, it := range inner {
				if im, ok := it.(map[string]interface{}); ok {
					collapseRangeForms(f.Items.Order, f.Items.Fields, im, tag)
				}
			}
		}
	}
}

// Роли полей тела (атрибут `role`, контракт 1.1.59).
const (
	// RoleCredential — учётные данные узла: поле учётной записи в слоте
	// userinfo ссылки (uuid, пароль, auth_str, имя пользователя). Шифр
	// ss/vmess учётными данными не считается.
	RoleCredential = "credential"
	// RolePrivateKey — приватный ключ, который уезжает в share-ссылку.
	RolePrivateKey = "private_key"
)

// FieldWithRole — путь поля схемы с ролью role; false — у схемы такого поля
// нет. Схема адресуется и типом ядра ("shadowsocks"), как Body.
func (r *Registry) FieldWithRole(scheme, role string) (string, bool) {
	b, ok := r.bodies[scheme]
	if !ok || role == "" {
		return "", false
	}
	for _, name := range b.Order {
		if f := b.Fields[name]; f != nil && f.Role == role {
			return name, true
		}
	}
	return "", false
}

// Credential — учётные данные узла из тела по роли `credential`: одно
// правило для всех входов (ссылка, sing-box JSON, Xray, тело состояния).
// Схема без роли или поле не строкой — "".
func (r *Registry) Credential(scheme string, body map[string]interface{}) string {
	path, ok := r.FieldWithRole(scheme, RoleCredential)
	if !ok {
		return ""
	}
	s, _ := body[path].(string)
	return s
}

// FieldStrings — `values` поля строками (enum для выпадающих списков форм).
// Пустое значение "" (у реестра оно значит «не задано») пропускается.
func (r *Registry) FieldStrings(scheme, path string) []string {
	f, ok := r.Field(scheme, path)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(f.Values))
	for _, v := range f.Values {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Warning возвращает запись кода из warnings.json.
func (r *Registry) Warning(code string) (*WarningEntry, bool) {
	w, ok := r.warnings[code]
	return w, ok
}

// WarningText отдаёт заголовок и текст кода на языке lang ("ru" — русский,
// всё прочее — английский), подставляя {path}, {value} и параметры.
func (r *Registry) WarningText(code, lang string, params map[string]string) (title, text string, ok bool) {
	w, found := r.warnings[code]
	if !found {
		return "", "", false
	}
	if lang == "ru" {
		title, text = w.TitleRu, w.TextRu
	} else {
		title, text = w.TitleEn, w.TextEn
	}
	return substitute(title, params), substitute(text, params), true
}

// WarningAdvice отдаёт причину и список действий на языке lang ("ru" —
// русский, всё прочее — английский). Подстановки здесь не делаются: причина и
// совет говорят о классе проблемы, а не о конкретном значении, и {path} в них
// не используется.
func (r *Registry) WarningAdvice(code, lang string) (cause string, fixes []string, ok bool) {
	w, found := r.warnings[code]
	if !found {
		return "", nil, false
	}
	if lang == "ru" {
		cause, fixes = w.CauseRu, w.FixRu
	} else {
		cause, fixes = w.CauseEn, w.FixEn
	}
	// Копия: список лежит в кэше реестра на весь процесс, и вызывающий не
	// должен иметь возможности его переписать.
	out := make([]string, len(fixes))
	copy(out, fixes)
	return cause, out, true
}

func substitute(s string, params map[string]string) string {
	if s == "" || len(params) == 0 {
		return s
	}
	for k, v := range params {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

// MapsTo — пути в теле, куда переводится параметр ссылки (SPEC 131 §3.1).
func (r *Registry) MapsTo(scheme, param string) ([]string, bool) {
	t, ok := r.mapsTo[scheme]
	if !ok {
		return nil, false
	}
	paths, ok := t[param]
	if !ok || len(paths) == 0 {
		return nil, false
	}
	out := make([]string, len(paths))
	copy(out, paths)
	return out, true
}

// QueryAliases — все написания параметра ссылки для схемы, канон первым
// (SPEC 131 W2d). Второй результат false — реестр знает только само имя.
//
// Правило симметрии: обратный эмиттер share-URI обязан писать ПЕРВОЕ имя
// списка. Иначе пара парсер/эмиттер расходится — та самая болезнь, из-за
// которой `upmbps` уезжал в конфиг, а читался `up_mbps`.
func (r *Registry) QueryAliases(scheme, param string) ([]string, bool) {
	t, ok := r.aliases[scheme]
	if !ok {
		return nil, false
	}
	names, ok := t[param]
	if !ok || len(names) == 0 {
		return nil, false
	}
	out := make([]string, len(names))
	copy(out, names)
	return out, true
}

// Limits — лимиты контракта (registry/limits.json).
func (r *Registry) Limits() map[string]Limit {
	out := make(map[string]Limit, len(r.limits))
	for k, v := range r.limits {
		out[k] = v
	}
	return out
}

// LimitInt отдаёт числовой лимит по имени.
func (r *Registry) LimitInt(name string) (int, bool) {
	l, ok := r.limits[name]
	if !ok {
		return 0, false
	}
	f, ok := l.Value.(float64)
	if !ok {
		return 0, false
	}
	return int(f), true
}

// Allowlist — канонический список по имени (utls_fingerprints, ss_methods…).
func (r *Registry) Allowlist(name string) []string {
	l, ok := r.lists[name]
	if !ok {
		return nil
	}
	out := make([]string, len(l))
	copy(out, l)
	return out
}
