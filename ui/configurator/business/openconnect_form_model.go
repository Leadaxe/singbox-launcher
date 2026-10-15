// File openconnect_form_model.go — общая модель формы узла OpenConnect
// (SPEC 167 §2.3): конструктор «Add server → OpenConnect» и блок правки у
// готового узла раскладываются по ней одной, чтобы не разойтись.
//
// Что берётся из реестра (reg.Form("openconnect") и атрибуты `form` полей):
// блок поля, видимость (visible_when), обязательность (required /
// required_unless), диапазон формы (min/max), тип значения, секрет, enum,
// коды advisory и группа выбора проверки сертификата (choices). Вычисление
// видимости, обязательности и активного варианта — функции реестра
// (FormState, FieldVisible, FormRequired, ActiveFormOption); модель их
// только вызывает.
//
// Что своё у стороны: подписи и подсказки (ключи locale, не desc_* реестра —
// решение SPEC §2.2), порядок строк внутри блока (макет SPEC §2.3) и набор
// полей первой волны. Поле с `form`, для которого у стороны нет подписи,
// форма не рисует и не трогает: так в первую волну не попадают token.*,
// клиентский сертификат и путь к УЦ (SPEC §2.3 «Не в форме»).
//
// Форма меняет ТОЛЬКО свои пути. Остальное тело (cookie, token, csd/hip,
// detour, сквозные ключи wildcard) копируется как есть. Скрытое поле из тела
// не снимается: что пришло из JSON, уходит в ядро (REGISTRY_DESIGN §3).
//
// go1.20-совместимо: без slices/maps/min/max.
package business

import (
	"fmt"
	"strconv"
	"strings"

	"singbox-launcher/core"
	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/nodewarn"
)

// OpenConnectScheme — схема реестра узла OpenConnect.
const OpenConnectScheme = "openconnect"

// OpenConnectFieldKind — вид поля формы (по типу поля в реестре).
type OpenConnectFieldKind string

const (
	OpenConnectKindString   OpenConnectFieldKind = "string"
	OpenConnectKindEnum     OpenConnectFieldKind = "enum"
	OpenConnectKindBool     OpenConnectFieldKind = "bool"
	OpenConnectKindInt      OpenConnectFieldKind = "int"
	OpenConnectKindDuration OpenConnectFieldKind = "duration"
	// OpenConnectKindList — listable_string: в форме строка через запятую,
	// в теле массив.
	OpenConnectKindList OpenConnectFieldKind = "list"
)

// OpenConnectField — поле формы: путь тела и всё, что нужно для строки.
type OpenConnectField struct {
	// Path — путь от корня тела ("tls.server_name").
	Path string
	// Block — id блока из body.form.blocks; у поля варианта группы выбора
	// это блок группы.
	Block string
	Kind  OpenConnectFieldKind
	// Label, Hint, Placeholder — английские ключи locale (подпись строки,
	// пояснение под строкой, подсказка в пустом поле).
	Label       string
	Hint        string
	Placeholder string
	// Secret — значение маскируется (password entry с глазком).
	Secret bool
	// Values — допустимые значения enum.
	Values []string
	// Default — строковый дефолт поля в реестре (у flavor — anyconnect:
	// что ядро берёт без ключа).
	Default string
	// Min/Max — диапазон ФОРМЫ (не ядра).
	Min *float64
	Max *float64
	// ChoiceID/OptionID — поле занимает вариант группы выбора: пишется,
	// только когда вариант активен.
	ChoiceID string
	OptionID string

	reg *registry.Field
}

// OpenConnectChoiceOption — вариант группы выбора.
type OpenConnectChoiceOption struct {
	ID    string
	Label string // ключ locale
	Paths []string
	Set   map[string]interface{}
	// Advisory — коды advisory путей Set: вариант окрашивается
	// предупреждением (insecure → tls_insecure).
	Advisory []string
}

// OpenConnectChoice — группа взаимоисключающих вариантов.
type OpenConnectChoice struct {
	ID      string
	Block   string
	Label   string // ключ locale
	Options []OpenConnectChoiceOption
}

// OpenConnectBlock — блок формы в порядке реестра.
type OpenConnectBlock struct {
	ID    string
	Title string // ключ locale; пусто — блок без заголовка
	// Expanded — блок раскрыт при открытии формы (макет SPEC §2.3).
	Expanded bool
	// Collapsible — блок сворачивается (аккордеон).
	Collapsible bool
	// Choices — группы выбора блока (рисуются первыми, как в макете).
	Choices []*OpenConnectChoice
	// Fields — поля блока, не принадлежащие группам выбора, в порядке
	// макета.
	Fields []*OpenConnectField
}

// OpenConnectFormModel — модель формы.
type OpenConnectFormModel struct {
	Blocks []OpenConnectBlock
	// Fields — все поля формы (и обычные, и поля вариантов).
	Fields []*OpenConnectField
	byPath map[string]*OpenConnectField
	reg    *registry.Registry
}

// OpenConnectFormValues — что набрано в форме.
type OpenConnectFormValues struct {
	// Fields — путь → текст поля; у bool "true" или пусто.
	Fields map[string]string
	// Choices — id группы → id выбранного варианта.
	Choices map[string]string
}

// openConnectFieldSide — то, что у стороны своё: подпись и подсказки поля.
type openConnectFieldSide struct {
	path        string
	label       string
	hint        string
	placeholder string
}

// openConnectFieldSides — поля первой волны в порядке макета SPEC §2.3.
// Порядок внутри блока задаёт макет, блок — реестр (Field.Form.Block).
var openConnectFieldSides = []openConnectFieldSide{
	{
		path:  "flavor",
		label: "Gateway", // l10n-key
	},
	{
		path:  "server",
		label: "Server",                                     // l10n-key
		hint:  "host:port or full URL; port 443 if omitted", // l10n-key
	},
	{
		path:  "username",
		label: "Username", // l10n-key
	},
	{
		path:  "password",
		label: "Password",                                                                           // l10n-key
		hint:  "If the gateway asks for a code, you will be prompted on connect (daemon mode only)", // l10n-key
	},
	{
		path:        "tls.peer_fingerprint",
		label:       "Pinned fingerprint",                    // l10n-key
		hint:        "Several fingerprints: comma-separated", // l10n-key
		placeholder: "sha256:…",                              // l10n-exempt: fingerprint format sample
	},
	{
		path:  "tls.server_name",
		label: "SNI",                                     // l10n-key
		hint:  "Only if it differs from the server host", // l10n-key
	},
	{
		path:  "auth_group",
		label: "Group / Realm", // l10n-key
	},
	{
		path:  "external_auth_disabled",
		label: "Disable SSO (external auth)", // l10n-key
	},
	{
		path:  "no_udp",
		label: "TCP only (no DTLS)",                                    // l10n-key
		hint:  "Use when UDP is blocked and the tunnel keeps dropping", // l10n-key
	},
	{
		path:  "user_agent",
		label: "User-Agent",                                        // l10n-key
		hint:  "Empty = core default (FortiGate: Mozilla/5.0 SV1)", // l10n-key
	},
	{
		path:  "fortinet_host_check.hostcheck",
		label: "Hostcheck",      // l10n-key
		hint:  "FortiGate only", // l10n-key
	},
	{
		path:  "mtu",
		label: "MTU",                                                       // l10n-key
		hint:  "Empty = from gateway; lower it if HTTPS stalls (576…1500)", // l10n-key
	},
	{
		path:  "dpd_interval",
		label: "DPD interval",                                            // l10n-key
		hint:  "Keep-alive probe period, e.g. 30s. Empty = from gateway", // l10n-key
	},
	{
		path:  "reconnect_timeout",
		label: "Reconnect timeout",                                                      // l10n-key
		hint:  "How long to retry a dropped tunnel, e.g. 5m. Empty = core default (5m)", // l10n-key
	},
	{
		path:  "ipv6_disabled",
		label: "Disable IPv6", // l10n-key
	},
	{
		path:  "allow_insecure_crypto",
		label: "Allow legacy crypto ⚠",           // l10n-key
		hint:  "Only for old gateways (TLS 1.0)", // l10n-key
	},
}

// openConnectBlockSide — заголовок и раскрытие блока (макет SPEC §2.3:
// первый блок всегда виден, Security раскрыт, Advanced свёрнут).
type openConnectBlockSide struct {
	title       string
	expanded    bool
	collapsible bool
}

var openConnectBlockSides = map[string]openConnectBlockSide{
	"connection": {},
	"security": {
		title:       "Security", // l10n-key
		expanded:    true,
		collapsible: true,
	},
	"advanced": {
		title:       "Advanced", // l10n-key
		collapsible: true,
	},
}

// openConnectChoiceLabel — подпись группы выбора (ключ locale).
func openConnectChoiceLabel(choiceID string) string {
	switch choiceID {
	case "server_cert_check":
		return "Server certificate" // l10n-key
	}
	return choiceID
}

// openConnectOptionLabel — подпись варианта группы выбора (ключ locale).
func openConnectOptionLabel(choiceID, optionID string) string {
	if choiceID == "server_cert_check" {
		switch optionID {
		case "system":
			return "System CA" // l10n-key
		case "pinned":
			return "Pinned fingerprint" // l10n-key
		case "insecure":
			return "Do not verify ⚠" // l10n-key
		}
	}
	return optionID
}

// openConnectFlavorNames — имена продуктов для значений flavor (подпись
// пункта «FortiGate (fortinet)»). Имена продуктов не переводятся.
var openConnectFlavorNames = map[string]string{
	"fortinet":   "FortiGate",        // l10n-exempt: product name
	"anyconnect": "Cisco AnyConnect", // l10n-exempt: product name
	"gp":         "GlobalProtect",    // l10n-exempt: product name
	"f5":         "F5",               // l10n-exempt: product name
	"pulse":      "Pulse/Ivanti",     // l10n-exempt: product name
	"nc":         "Juniper NC",       // l10n-exempt: product name
}

// OpenConnectFlavorLabel — подпись значения flavor в выпадающем списке.
func OpenConnectFlavorLabel(v string) string {
	if name, ok := openConnectFlavorNames[v]; ok {
		return fmt.Sprintf("%s (%s)", name, v)
	}
	return v
}

// NewOpenConnectFormModel строит модель по реестру.
func NewOpenConnectFormModel() (*OpenConnectFormModel, error) {
	reg, err := registry.Get()
	if err != nil {
		return nil, err
	}
	form := reg.Form(OpenConnectScheme)
	if form == nil {
		return nil, fmt.Errorf("registry: scheme %s has no form", OpenConnectScheme)
	}
	m := &OpenConnectFormModel{byPath: map[string]*OpenConnectField{}, reg: reg}

	// Пути вариантов групп выбора: поле такого пути рисует группа.
	optionOf := map[string][2]string{}
	for _, c := range form.Choices {
		for _, o := range c.Options {
			for _, p := range o.Paths {
				optionOf[p] = [2]string{c.ID, o.ID}
			}
		}
	}
	choiceBlock := map[string]string{}
	for _, c := range form.Choices {
		choiceBlock[c.ID] = c.Block
	}

	for _, side := range openConnectFieldSides {
		rf, ok := reg.Field(OpenConnectScheme, side.path)
		if !ok || rf.Form == nil {
			// Реестр поле из формы убрал — сторона его не рисует.
			continue
		}
		f := &OpenConnectField{
			Path:        side.path,
			Block:       rf.Form.Block,
			Kind:        openConnectKindOf(rf),
			Label:       side.label,
			Hint:        side.hint,
			Placeholder: side.placeholder,
			Secret:      rf.Secret,
			Min:         rf.Form.Min,
			Max:         rf.Form.Max,
			reg:         rf,
		}
		if f.Kind == OpenConnectKindEnum {
			f.Values = reg.FieldStrings(OpenConnectScheme, side.path)
		}
		if d, ok := rf.Default.(string); ok {
			f.Default = d
		}
		if ch, ok := optionOf[side.path]; ok {
			f.ChoiceID, f.OptionID = ch[0], ch[1]
			f.Block = choiceBlock[ch[0]]
		}
		m.Fields = append(m.Fields, f)
		m.byPath[f.Path] = f
	}

	for _, id := range form.Blocks {
		bs := openConnectBlockSides[id]
		blk := OpenConnectBlock{ID: id, Title: bs.title, Expanded: bs.expanded, Collapsible: bs.collapsible}
		for _, c := range form.Choices {
			if c.Block != id {
				continue
			}
			oc := &OpenConnectChoice{ID: c.ID, Block: c.Block, Label: openConnectChoiceLabel(c.ID)}
			for _, o := range c.Options {
				opt := OpenConnectChoiceOption{ID: o.ID, Paths: o.Paths, Set: o.Set}
				opt.Label = openConnectOptionLabel(c.ID, o.ID)
				for p, v := range o.Set {
					if rf, ok := reg.Field(OpenConnectScheme, p); ok {
						opt.Advisory = append(opt.Advisory, advisoryCodes(rf, v)...)
					}
				}
				oc.Options = append(oc.Options, opt)
			}
			blk.Choices = append(blk.Choices, oc)
		}
		for _, f := range m.Fields {
			if f.Block == id && f.ChoiceID == "" {
				blk.Fields = append(blk.Fields, f)
			}
		}
		m.Blocks = append(m.Blocks, blk)
	}
	return m, nil
}

// Field — поле модели по пути.
func (m *OpenConnectFormModel) Field(path string) *OpenConnectField {
	return m.byPath[path]
}

// ChoiceField — поле, которое занимает вариант группы (nil — вариант полей
// не несёт).
func (m *OpenConnectFormModel) ChoiceField(choiceID, optionID string) *OpenConnectField {
	for _, f := range m.Fields {
		if f.ChoiceID == choiceID && f.OptionID == optionID {
			return f
		}
	}
	return nil
}

func openConnectKindOf(f *registry.Field) OpenConnectFieldKind {
	switch f.Type {
	case "enum":
		return OpenConnectKindEnum
	case "bool":
		return OpenConnectKindBool
	case "int", "uint16", "uint32":
		return OpenConnectKindInt
	case "duration":
		return OpenConnectKindDuration
	case "listable_string":
		return OpenConnectKindList
	}
	return OpenConnectKindString
}

// advisoryCodes — коды advisory поля, под которые попадает значение v.
func advisoryCodes(f *registry.Field, v interface{}) []string {
	if f == nil || v == nil {
		return nil
	}
	got := fmt.Sprintf("%v", v)
	var out []string
	for _, a := range f.Advisory {
		if a.When != nil {
			continue // условные advisory судит санитайзер, не форма
		}
		hit := len(a.Values) == 0
		for _, w := range a.Values {
			if fmt.Sprintf("%v", w) == got {
				hit = true
				break
			}
		}
		for _, w := range a.Except {
			if fmt.Sprintf("%v", w) == got {
				hit = false
				break
			}
		}
		if hit && a.Code != "" {
			out = append(out, a.Code)
		}
	}
	return out
}

// AdvisoryText — заголовок первого кода advisory, под который попадает
// набранное значение поля (пусто — предупреждать не о чем).
func (m *OpenConnectFormModel) AdvisoryText(path, text string) string {
	f := m.byPath[path]
	if f == nil {
		return ""
	}
	v, ok, _ := f.parse(text)
	if !ok {
		return ""
	}
	return m.advisoryTitle(advisoryCodes(f.reg, v))
}

// OptionAdvisoryText — заголовок кода advisory варианта группы выбора.
func (m *OpenConnectFormModel) OptionAdvisoryText(opt OpenConnectChoiceOption) string {
	return m.advisoryTitle(opt.Advisory)
}

func (m *OpenConnectFormModel) advisoryTitle(codes []string) string {
	for _, c := range codes {
		if title, _, ok := m.reg.WarningText(c, locale.GetLang(), nil); ok && title != "" {
			return title
		}
		return c
	}
	return ""
}

// parse переводит текст поля в значение тела. ok=false — поле пустое (ключ
// не пишется); err — текст не разбирается.
func (f *OpenConnectField) parse(text string) (interface{}, bool, error) {
	trimmed := strings.TrimSpace(text)
	switch f.Kind {
	case OpenConnectKindBool:
		on := trimmed == "true"
		def, _ := f.reg.Default.(bool)
		if on == def {
			// Значение совпало с дефолтом ядра — ключ не пишется.
			return nil, false, nil
		}
		return on, true, nil
	case OpenConnectKindInt:
		if trimmed == "" {
			return nil, false, nil
		}
		n, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, false, fmt.Errorf("%s", locale.Tf("%s must be a whole number", locale.T(f.Label)))
		}
		return n, true, nil
	case OpenConnectKindList:
		parts := strings.Split(text, ",")
		out := make([]interface{}, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		if len(out) == 0 {
			return nil, false, nil
		}
		return out, true, nil
	}
	if trimmed == "" {
		return nil, false, nil
	}
	if f.Secret {
		// Секрет не обрезается: пробелы в пароле легальны.
		return text, true, nil
	}
	return trimmed, true, nil
}

// FormFromBody — значения полей формы из тела узла.
func (m *OpenConnectFormModel) FormFromBody(body map[string]interface{}) OpenConnectFormValues {
	v := OpenConnectFormValues{Fields: map[string]string{}, Choices: map[string]string{}}
	for _, f := range m.Fields {
		raw, ok := getBodyPath(body, f.Path)
		if !ok || raw == nil {
			continue
		}
		v.Fields[f.Path] = formTextOf(f, raw)
	}
	if form := m.reg.Form(OpenConnectScheme); form != nil {
		for _, c := range form.Choices {
			v.Choices[c.ID] = registry.ActiveFormOption(c, body)
		}
	}
	return v
}

func formTextOf(f *OpenConnectField, raw interface{}) string {
	switch t := raw.(type) {
	case bool:
		if t {
			return "true"
		}
		return ""
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, it := range t {
			parts = append(parts, fmt.Sprint(it))
		}
		return strings.Join(parts, ", ")
	case []string:
		return strings.Join(t, ", ")
	case string:
		return t
	}
	return fmt.Sprint(raw)
}

// BodyFromForm — тело узла: base (текущее тело, у нового узла пустое) с
// путями формы, переписанными по values.
//
// Пустое поле ключ не пишет (дефолт за шлюзом/ядром), bool, равный дефолту
// ядра, тоже. Смена варианта группы выбора чистит пути остальных вариантов
// и пишет `set` выбранного. Поле, скрытое visible_when на итоговом теле,
// остаётся таким, каким было в base. Прочие ключи base копируются как есть.
func (m *OpenConnectFormModel) BodyFromForm(base map[string]interface{}, values OpenConnectFormValues) (map[string]interface{}, error) {
	out := deepCopyMap(base)
	if out == nil {
		out = map[string]interface{}{}
	}
	var firstErr error
	written := map[string]bool{}

	for _, f := range m.Fields {
		deleteBodyPath(out, f.Path)
		if f.ChoiceID != "" && values.Choices[f.ChoiceID] != f.OptionID {
			continue // вариант не выбран — его путь снят
		}
		v, ok, err := f.parse(values.Fields[f.Path])
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if ok {
			setBodyPath(out, f.Path, v)
			written[f.Path] = true
		}
	}

	if form := m.reg.Form(OpenConnectScheme); form != nil {
		for _, c := range form.Choices {
			sel := values.Choices[c.ID]
			for _, o := range c.Options {
				if o.ID == sel {
					continue
				}
				for _, p := range o.Paths {
					deleteBodyPath(out, p)
				}
			}
			for _, o := range c.Options {
				if o.ID != sel {
					continue
				}
				for p, v := range o.Set {
					setBodyPath(out, p, v)
				}
			}
		}
	}

	// Скрытое поле не правится формой: значение base возвращается на место.
	if st, ok := m.reg.FormState(OpenConnectScheme, out); ok {
		visible := map[string]bool{}
		for _, p := range st.Visible {
			visible[p] = true
		}
		for _, f := range m.Fields {
			if f.ChoiceID != "" || visible[f.Path] {
				continue
			}
			deleteBodyPath(out, f.Path)
			if old, had := getBodyPath(base, f.Path); had {
				setBodyPath(out, f.Path, deepCopyValue(old))
			}
		}
	}
	pruneEmptyObjects(out, m.Fields)
	return out, firstErr
}

// Visible — какие поля формы видны на теле (по visible_when реестра).
func (m *OpenConnectFormModel) Visible(body map[string]interface{}) map[string]bool {
	out := map[string]bool{}
	st, ok := m.reg.FormState(OpenConnectScheme, body)
	if !ok {
		return out
	}
	for _, p := range st.Visible {
		out[p] = true
	}
	return out
}

// Verdict — проверка тела перед сохранением: обязательные поля формы,
// диапазоны формы и санитайзер реестра (как у остальных форм узлов:
// nodeflow.Sanitize → nodewarn.Summary). Пусто — тело годно.
//
// Санитайзер судит тело целиком: отказ уровня узла (Drop) — ошибка; снятый
// им путь формы — ошибка с текстом кода. Коды на путях вне формы (ключи из
// JSON) форму не останавливают — их покажет узел.
func (m *OpenConnectFormModel) Verdict(body map[string]interface{}) error {
	if st, ok := m.reg.FormState(OpenConnectScheme, body); ok {
		var missing []string
		for _, p := range st.Missing {
			if f := m.byPath[p]; f != nil {
				missing = append(missing, locale.T(f.Label))
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s", locale.Tf("Required: %s", strings.Join(missing, ", ")))
		}
		for _, r := range st.OutOfRange {
			f := m.byPath[r.Path]
			if f == nil {
				continue
			}
			lo, hi := "", ""
			if r.Min != nil {
				lo = strconv.FormatFloat(*r.Min, 'f', -1, 64)
			}
			if r.Max != nil {
				hi = strconv.FormatFloat(*r.Max, 'f', -1, 64)
			}
			return fmt.Errorf("%s", locale.Tf("%s must be within %s…%s", locale.T(f.Label), lo, hi))
		}
	}

	res := nodeflow.Sanitize(OpenConnectScheme, body)
	if res.Drop != nil {
		if msg := nodewarn.Summary(nodewarn.FromParsed([]nodeflow.Warning{*res.Drop})); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("%s", res.Drop.Code)
	}
	for _, f := range m.Fields {
		sent, has := getBodyPath(body, f.Path)
		if !has || sent == nil {
			continue
		}
		kept, still := getBodyPath(res.Clean, f.Path)
		if still && sameListLen(sent, kept) {
			continue
		}
		for _, w := range res.Warnings {
			if w.Path != f.Path && !strings.HasPrefix(w.Path, f.Path+"[") && !strings.HasPrefix(w.Path, f.Path+".") {
				continue
			}
			if msg := nodewarn.Summary(nodewarn.FromParsed([]nodeflow.Warning{w})); msg != "" {
				return fmt.Errorf("%s", msg)
			}
			return fmt.Errorf("%s", w.Code)
		}
		if !still {
			return fmt.Errorf("%s", locale.Tf("%s is invalid", locale.T(f.Label)))
		}
	}
	return nil
}

// sameListLen — у списка санитайзер не снял ни одного элемента (снятый
// элемент отпечатка — тоже отказ формы: человек его набрал).
func sameListLen(a, b interface{}) bool {
	la, okA := a.([]interface{})
	if !okA {
		return true
	}
	switch lb := b.(type) {
	case []interface{}:
		return len(la) == len(lb)
	case []string:
		return len(la) == len(lb)
	}
	return true
}

// OpenConnectClassicBackend — ядро запущено лаунчером в классическом режиме
// (`sing-box run`): канала челленджа второго фактора нет (SPEC 167 §2.5),
// форма добавляет об этом строку под паролем.
func OpenConnectClassicBackend() bool {
	ac := core.GetController()
	return ac != nil && ac.BackendMode() == core.BackendClassic
}

// OpenConnectDefaultTag — тег нового узла, когда Name пуст: хост сервера
// (без схемы, порта и пути), иначе имя схемы.
func OpenConnectDefaultTag(server string) string {
	s := strings.TrimSpace(server)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if strings.HasPrefix(s, "[") {
		if i := strings.Index(s, "]"); i > 0 {
			s = s[1:i]
		}
	} else if i := strings.LastIndex(s, ":"); i >= 0 && strings.Count(s, ":") == 1 {
		s = s[:i]
	}
	if s == "" {
		return OpenConnectScheme
	}
	return s
}

// --- пути тела ---

func getBodyPath(body map[string]interface{}, path string) (interface{}, bool) {
	var cur interface{} = body
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func setBodyPath(body map[string]interface{}, path string, v interface{}) {
	parts := strings.Split(path, ".")
	cur := body
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			cur[part] = next
		}
		cur = next
	}
	cur[parts[len(parts)-1]] = v
}

func deleteBodyPath(body map[string]interface{}, path string) {
	parts := strings.Split(path, ".")
	cur := body
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]interface{})
		if !ok {
			return
		}
		cur = next
	}
	delete(cur, parts[len(parts)-1])
}

// pruneEmptyObjects снимает объекты-предки путей формы, ставшие пустыми
// (tls без единого ключа ядру не нужен).
func pruneEmptyObjects(body map[string]interface{}, fields []*OpenConnectField) {
	for _, f := range fields {
		parts := strings.Split(f.Path, ".")
		for depth := len(parts) - 1; depth >= 1; depth-- {
			parent := strings.Join(parts[:depth], ".")
			v, ok := getBodyPath(body, parent)
			if !ok {
				continue
			}
			if mm, isMap := v.(map[string]interface{}); isMap && len(mm) == 0 {
				deleteBodyPath(body, parent)
			}
		}
	}
}

func deepCopyMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = deepCopyValue(v)
	}
	return out
}

func deepCopyValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		return deepCopyMap(t)
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, it := range t {
			out[i] = deepCopyValue(it)
		}
		return out
	}
	return v
}
