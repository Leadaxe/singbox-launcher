// File dns_server_form.go — формы DNS-серверов (SPEC 109, фаза 1).
//
// До этого сервер добавлялся ТОЛЬКО написанием JSON: пустое поле и одна из
// двух заготовок, всё остальное — по памяти (порт, путь DoH, tls.server_name,
// domain_resolver, detour). Ядро поддерживает двенадцать типов, форм не было
// ни для одного.
//
// Формы покрывают пять типов, осмысленных для пользователя: udp, tcp, tls
// (DoT), https (DoH) и group. Прочие (quic, h3, hosts, fakeip, dhcp,
// tailscale, legacy*) правятся на вкладке JSON — она остаётся запасным
// путём, как у Направления, иначе типы без формы стали бы недоступны.
//
// Форма собирает map и отдаёт его в applyDNSServerJSON — тот же путь, что у
// ручного JSON. Второй путь записи означал бы вторую реализацию проверок
// (уникальность тега, блокировка шаблонных записей).
package tabs

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// Типы DNS-серверов, для которых есть форма (constant/dns.go ядра).
const (
	dnsTypeUDP   = "udp"
	dnsTypeTCP   = "tcp"
	dnsTypeTLS   = "tls"
	dnsTypeHTTPS = "https"
	dnsTypeGroup = "group"
)

// dnsFormTypes — порядок в выпадающем списке: от простого к составному.
var dnsFormTypes = []string{dnsTypeUDP, dnsTypeTCP, dnsTypeTLS, dnsTypeHTTPS, dnsTypeGroup}

// dnsDefaultPort — порт по умолчанию для типа. Пользователь его почти
// никогда не меняет, но подставить обязаны: сервер без порта ядро
// отвергает.
func dnsDefaultPort(typ string) int {
	switch typ {
	case dnsTypeTLS:
		return 853
	case dnsTypeHTTPS:
		return 443
	default:
		return 53
	}
}

// dnsNoDetour — подпись пункта «без детура» в списке Направлений.
func dnsNoDetour() string { return locale.T("wizard.dns.form_detour_none") }

// dnsServerForm — виджеты формы и её текущий тип.
type dnsServerForm struct {
	typeSelect     *widget.Select
	tagEntry       *widget.Entry
	serverEntry    *widget.Entry
	portEntry      *widget.Entry
	pathEntry      *widget.Entry
	sniEntry       *widget.Entry
	detourSelect   *widget.Select
	resolverSelect *widget.Select

	// Группа: участники — не текстовое поле, а список выбранных тегов с
	// кнопкой «+». Вписывать тег руками так же неверно, как вписывать
	// детур: имена приходят из шаблона и подписок, пользователь их не
	// сочиняет, а опечатка даёт ссылку в никуда.
	members       []string
	membersBox    *fyne.Container
	membersAddBtn *widget.Button
	modeSelect    *widget.Select
	errorTTLEntry *widget.Entry
	winTTLEntry   *widget.Entry

	rows    map[string]fyne.CanvasObject
	content *fyne.Container

	// enabledByTag — включённость серверов на момент открытия окна. Нужна,
	// чтобы вычеркнуть в составе тех, кто выключен: они не попадут в конфиг
	// (pruneDNSGroupMembers их отбросит), и группа окажется уже, чем
	// выглядит. Убирать их из состава за пользователя нельзя — он мог
	// выключить сервер временно.
	enabledByTag map[string]bool

	// readOnly — форма показывает шаблонную запись: поля заблокированы, у
	// участников группы нет кнопки удаления.
	readOnly bool

	// applying — правка идёт из кода. Без флага programmatic SetSelected
	// вызвал бы OnChanged, тот — перестроение формы, и она ушла бы в
	// рекурсию (ловушка SPEC 104).
	applying bool
}

// newDNSServerForm собирает форму. selfTag исключается из списка резолверов
// (сервер, резолвящий собственное имя, вешает старт).
func newDNSServerForm(p *wizardpresentation.WizardPresenter, selfTag string) *dnsServerForm {
	f := &dnsServerForm{
		rows:         map[string]fyne.CanvasObject{},
		enabledByTag: dnsEnabledByTag(p.Model().DNSServers),
	}

	f.typeSelect = widget.NewSelect(dnsFormTypes, nil)
	f.tagEntry = widget.NewEntry()
	f.tagEntry.SetPlaceHolder("my-dns")
	f.serverEntry = widget.NewEntry()
	f.serverEntry.SetPlaceHolder("1.1.1.1")
	f.portEntry = widget.NewEntry()
	f.pathEntry = widget.NewEntry()
	f.pathEntry.SetPlaceHolder("/dns-query")
	f.sniEntry = widget.NewEntry()
	f.sniEntry.SetPlaceHolder("cloudflare-dns.com")

	// Детур — тот же список, что цель правила: без групп подписок (SPEC 108)
	// и без парных auto-групп (SPEC 104). Писать тег руками пользователь не
	// обязан — в правилах он его выбирает.
	detourOpts := append([]string{dnsNoDetour()}, wizardbusiness.GetAvailableOutbounds(p.Model())...)
	f.detourSelect = widget.NewSelect(detourOpts, nil)
	f.detourSelect.SetSelected(dnsNoDetour())

	f.resolverSelect = widget.NewSelect(
		append([]string{dnsNoDetour()}, dnsResolverCandidates(p, selfTag)...), nil)
	f.resolverSelect.SetSelected(dnsNoDetour())

	f.membersBox = container.NewVBox()
	f.membersAddBtn = widget.NewButtonWithIcon(locale.T("wizard.dns.form_members_add"), theme.ContentAddIcon(), func() {
		f.pickMember(p, selfTag)
	})
	f.modeSelect = widget.NewSelect([]string{"fastest", "random"}, nil)
	f.modeSelect.SetSelected("fastest")
	f.errorTTLEntry = widget.NewEntry()
	f.errorTTLEntry.SetPlaceHolder("5m")
	f.winTTLEntry = widget.NewEntry()
	f.winTTLEntry.SetPlaceHolder("5m")

	f.rows["server"] = dnsFormRow(locale.T("wizard.dns.form_address"), f.serverEntry)
	f.rows["port"] = dnsFormRow(locale.T("wizard.dns.form_port"), f.portEntry)
	f.rows["path"] = dnsFormRow(locale.T("wizard.dns.form_path"), f.pathEntry)
	f.rows["sni"] = dnsFormRow(locale.T("wizard.dns.form_sni"), f.sniEntry)
	f.rows["detour"] = dnsFormRow(locale.T("wizard.dns.form_detour"), f.detourSelect)
	f.rows["resolver"] = dnsFormRow(locale.T("wizard.dns.form_resolver"), f.resolverSelect)
	f.rows["members"] = dnsFormRow(locale.T("wizard.dns.form_members"),
		container.NewVBox(f.membersBox, container.NewHBox(f.membersAddBtn)))
	f.rows["mode"] = dnsFormRow(locale.T("wizard.dns.form_mode"), f.modeSelect)
	f.rows["error_ttl"] = dnsFormRow(locale.T("wizard.dns.form_error_ttl"), f.errorTTLEntry)
	f.rows["win_ttl"] = dnsFormRow(locale.T("wizard.dns.form_win_ttl"), f.winTTLEntry)

	f.rows["type"] = dnsFormRow(locale.T("wizard.dns.form_type"), f.typeSelect)
	f.rows["tag"] = dnsFormRow(locale.T("wizard.dns.form_tag"), f.tagEntry)

	f.content = container.NewVBox(
		f.rows["type"],
		f.rows["tag"],
		f.rows["server"], f.rows["port"], f.rows["path"], f.rows["sni"],
		f.rows["detour"], f.rows["resolver"],
		f.rows["members"], f.rows["mode"], f.rows["error_ttl"], f.rows["win_ttl"],
	)

	// Обработчик ПОСЛЕ установки значения.
	f.typeSelect.SetSelected(dnsTypeUDP)
	f.syncRows()
	f.typeSelect.OnChanged = func(string) {
		if f.applying {
			return
		}
		f.portEntry.SetText(strconv.Itoa(dnsDefaultPort(f.typeSelect.Selected)))
		f.syncRows()
	}
	f.portEntry.SetText(strconv.Itoa(dnsDefaultPort(dnsTypeUDP)))

	return f
}

// syncRows показывает поля, осмысленные для текущего типа.
func (f *dnsServerForm) syncRows() {
	typ := f.typeSelect.Selected
	group := typ == dnsTypeGroup
	tlsLike := typ == dnsTypeTLS || typ == dnsTypeHTTPS

	show := func(key string, on bool) {
		if o := f.rows[key]; o != nil {
			if on {
				o.Show()
			} else {
				o.Hide()
			}
		}
	}
	show("server", !group)
	show("port", !group)
	show("path", typ == dnsTypeHTTPS)
	show("sni", tlsLike)
	// Группа не ходит в сеть — ходят её участники, и каждый своим детуром.
	// Детур на самой группе ядро не применяет; поле здесь означало бы
	// настройку, которая ничего не делает (частность перенесена из LxBox,
	// задача 319).
	show("detour", !group)
	show("resolver", !group)
	show("members", group)
	show("mode", group)
	show("error_ttl", group)
	show("win_ttl", group)
	f.content.Refresh()
}

// SetReadOnly блокирует поля формы, оставляя их читаемыми.
//
// «Тело править нельзя» не значит «показывать нечем»: у шаблонной записи те
// же адрес, порт, канал и резолвер, и пользователь вправе увидеть их в том
// же виде, что у своей. Иначе один и тот же сервер выглядит формой у себя и
// сырым JSON у шаблона — а разница между ними только в том, кто владелец.
func (f *dnsServerForm) SetReadOnly() {
	f.readOnly = true

	// Значения показываем обычными Label, а не гашёными виджетами:
	// Disable() у Fyne приглушает текст до нечитаемого на тёмной теме —
	// «только для чтения» превращалось в «не разобрать, что написано».
	// Label берёт цвет обычного текста и остаётся выделяемым глазом.
	replace := func(key string, value string) {
		row, ok := f.rows[key]
		if !ok || row == nil {
			return
		}
		border, ok := row.(*fyne.Container)
		if !ok || len(border.Objects) == 0 {
			return
		}
		if value == "" {
			value = "—"
		}
		lbl := widget.NewLabel(value)
		lbl.Wrapping = fyne.TextWrapWord
		// Objects[0] у Border — центральный объект (само поле).
		border.Objects[0] = lbl
		border.Refresh()
	}

	replace("server", f.serverEntry.Text)
	replace("port", f.portEntry.Text)
	replace("path", f.pathEntry.Text)
	replace("sni", f.sniEntry.Text)
	replace("detour", selectedOrDash(f.detourSelect))
	replace("resolver", selectedOrDash(f.resolverSelect))
	replace("mode", selectedOrDash(f.modeSelect))
	replace("error_ttl", f.errorTTLEntry.Text)
	replace("win_ttl", f.winTTLEntry.Text)

	replace("type", f.typeSelect.Selected)
	replace("tag", f.tagEntry.Text)
	f.membersAddBtn.Disable()

	f.rebuildMembers()
}

// selectedOrDash — выбранное значение списка либо прочерк.
func selectedOrDash(sel *widget.Select) string {
	if sel == nil || sel.Selected == "" || sel.Selected == dnsNoDetour() {
		return ""
	}
	return sel.Selected
}

// Load заполняет форму из тела сервера. Возвращает false, если тип не
// покрыт формой — тогда вызывающий показывает только вкладку JSON.
func (f *dnsServerForm) Load(obj map[string]interface{}) bool {
	typ := dnsJSONStringField(obj, "type")
	if typ == "" {
		typ = dnsTypeUDP
	}
	known := false
	for _, t := range dnsFormTypes {
		if t == typ {
			known = true
			break
		}
	}
	if !known {
		return false
	}

	f.applying = true
	defer func() { f.applying = false }()

	f.typeSelect.SetSelected(typ)
	f.tagEntry.SetText(dnsJSONStringField(obj, "tag"))
	f.serverEntry.SetText(dnsJSONStringField(obj, "server"))
	if n, ok := dnsJSONNumberField(obj, "server_port"); ok {
		f.portEntry.SetText(strconv.Itoa(n))
	} else {
		f.portEntry.SetText(strconv.Itoa(dnsDefaultPort(typ)))
	}
	f.pathEntry.SetText(dnsJSONStringField(obj, "path"))
	if tlsObj, ok := obj["tls"].(map[string]interface{}); ok {
		f.sniEntry.SetText(dnsJSONStringField(tlsObj, "server_name"))
	} else {
		f.sniEntry.SetText("")
	}
	f.detourSelect.SetSelected(orNone(dnsJSONStringField(obj, "detour")))
	f.resolverSelect.SetSelected(orNone(dnsJSONStringField(obj, "domain_resolver")))

	f.setMembers(dnsJSONStringList(obj, "servers"))
	if m := dnsJSONStringField(obj, "mode"); m != "" {
		f.modeSelect.SetSelected(m)
	}
	f.errorTTLEntry.SetText(dnsJSONStringField(obj, "error_ttl"))
	f.winTTLEntry.SetText(dnsJSONStringField(obj, "win_ttl"))

	f.syncRows()
	return true
}

// Collect собирает тело сервера из формы.
//
// Незаполненные поля НЕ пишутся: ядро подставляет свои умолчания, а пустая
// строка в `path` или `domain_resolver` — не «по умолчанию», а явное пустое
// значение, на котором конфиг ломается.
func (f *dnsServerForm) Collect() map[string]interface{} {
	typ := f.typeSelect.Selected
	out := map[string]interface{}{
		"type": typ,
		"tag":  strings.TrimSpace(f.tagEntry.Text),
	}

	if typ == dnsTypeGroup {
		out["servers"] = append([]string(nil), f.members...)
		if m := f.modeSelect.Selected; m != "" {
			out["mode"] = m
		}
		putIfSet(out, "error_ttl", f.errorTTLEntry.Text)
		putIfSet(out, "win_ttl", f.winTTLEntry.Text)
		return out
	}

	putIfSet(out, "server", f.serverEntry.Text)
	if n, err := strconv.Atoi(strings.TrimSpace(f.portEntry.Text)); err == nil && n > 0 {
		out["server_port"] = n
	}
	if typ == dnsTypeHTTPS {
		putIfSet(out, "path", f.pathEntry.Text)
	}
	if sni := strings.TrimSpace(f.sniEntry.Text); sni != "" &&
		(typ == dnsTypeTLS || typ == dnsTypeHTTPS) {
		out["tls"] = map[string]interface{}{"server_name": sni}
	}
	if d := f.detourSelect.Selected; d != "" && d != dnsNoDetour() {
		out["detour"] = d
	}
	if r := f.resolverSelect.Selected; r != "" && r != dnsNoDetour() {
		out["domain_resolver"] = r
	}
	return out
}

// dnsResolverCandidates — теги серверов, годных в `domain_resolver`.
//
// Себя исключаем: сервер, резолвящий собственное имя, замыкает цепочку и
// вешает старт. Группы тоже: резолвер имени должен быть конкретным
// сервером, иначе цепочка удлиняется без пользы.
func dnsResolverCandidates(p *wizardpresentation.WizardPresenter, selfTag string) []string {
	m := p.Model()
	if m == nil {
		return nil
	}
	var out []string
	for _, raw := range m.DNSServers {
		var o struct {
			Tag  string `json:"tag"`
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &o) != nil || o.Tag == "" {
			continue
		}
		if o.Tag == selfTag || o.Type == dnsTypeGroup {
			continue
		}
		out = append(out, o.Tag)
	}
	sort.Strings(out)
	return out
}

// setMembers перестраивает список участников.
func (f *dnsServerForm) setMembers(tags []string) {
	f.members = append([]string(nil), tags...)
	f.rebuildMembers()
}

// rebuildMembers перерисовывает строки участников: тег и кнопка удаления.
func (f *dnsServerForm) rebuildMembers() {
	f.membersBox.Objects = f.membersBox.Objects[:0]
	for i, tag := range f.members {
		idx := i
		text := tag
		var label *widget.Label
		if f.enabledByTag[tag] {
			label = widget.NewLabel(text)
		} else {
			// Выключенный участник: вычеркнут и подписан. Он останется в
			// составе (пользователь мог выключить сервер временно), но в
			// конфиг не попадёт — и об этом надо сказать здесь, а не дать
			// обнаружить по факту укоротившейся группы.
			label = widget.NewLabel(strikeThrough(text) + "   " +
				locale.T("wizard.dns.form_member_off"))
			label.Importance = widget.LowImportance
		}
		if f.readOnly {
			f.membersBox.Add(label)
			continue
		}
		del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
			f.members = append(f.members[:idx:idx], f.members[idx+1:]...)
			f.rebuildMembers()
		})
		del.Importance = widget.LowImportance
		f.membersBox.Add(container.NewBorder(nil, nil, nil, del, label))
	}
	if len(f.members) == 0 {
		empty := widget.NewLabel(locale.T("wizard.dns.form_members_empty"))
		empty.Importance = widget.LowImportance
		f.membersBox.Add(empty)
	}
	f.membersBox.Refresh()
}

// pickMember показывает список серверов, которых ещё нет в составе.
//
// Уже выбранные и сама группа исключаются: дубль в составе бессмыслен, а
// группа внутри себя — цикл. Вложенные группы тоже не предлагаем: ядро
// ждёт в составе конкретные серверы.
func (f *dnsServerForm) pickMember(p *wizardpresentation.WizardPresenter, selfTag string) {
	chosen := make(map[string]bool, len(f.members))
	for _, t := range f.members {
		chosen[t] = true
	}
	var options []string
	for _, t := range dnsResolverCandidates(p, selfTag) {
		if !chosen[t] {
			options = append(options, t)
		}
	}
	w := p.DialogParent()
	if len(options) == 0 {
		dialog.ShowInformation(
			locale.T("wizard.dns.form_members"),
			locale.T("wizard.dns.form_members_none"), w)
		return
	}
	sel := widget.NewSelect(options, nil)
	sel.SetSelected(options[0])
	dialog.ShowCustomConfirm(
		locale.T("wizard.dns.form_members_add"), locale.T("wizard.dns.dialog_save"),
		locale.T("wizard.dns.dialog_cancel"), sel,
		func(ok bool) {
			if !ok || sel.Selected == "" {
				return
			}
			f.members = append(f.members, sel.Selected)
			f.rebuildMembers()
		}, w)
}

func dnsFormRow(label string, field fyne.CanvasObject) fyne.CanvasObject {
	const labelWidth = 150
	l := widget.NewLabel(label)
	box := container.NewGridWrap(fyne.NewSize(labelWidth, l.MinSize().Height), l)
	return container.NewBorder(nil, nil, box, nil, field)
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return dnsNoDetour()
	}
	return s
}

func putIfSet(m map[string]interface{}, key, val string) {
	if v := strings.TrimSpace(val); v != "" {
		m[key] = v
	}
}

func dnsJSONNumberField(m map[string]interface{}, key string) (int, bool) {
	switch t := m[key].(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	}
	return 0, false
}

func dnsJSONStringList(m map[string]interface{}, key string) []string {
	arr, ok := m[key].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
