package dialogs

// rule_dialog_dns.go — вкладка DNS окна правила (SPEC 172, TEMPLATE_LANG
// §6.11, SPEC 188 §5): сервер для DNS-запросов под правило, локальный ответ,
// глушилка AAAA, Block HTTPS. Это поле записи, а не тела sing-box, поэтому
// своя вкладка.
//
// Опции доступны всегда (SPEC 188, PLAN вопрос 11): правило, у которого нет
// условия, видимого DNS-слою, сохраняется, а вкладка показывает ⚠ «DNS
// options have no effect…». Матчер — тот же обход дерева, что у сборки
// (build.RuleDNSMirrorForBody), по телу записи с `refs`.

import (
	"fmt"
	"net"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config/registry"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/nodewarn"
	"singbox-launcher/ui/components"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// ruleDNSNoEffectText — ⚠ пустого зеркала (TEMPLATE_LANG §6.11): тот же
// текст в окне правила и в строке правила на вкладке Rules.
const ruleDNSNoEffectText = "DNS options have no effect: the rule has no condition DNS can see"

// RuleDNSNoEffectMark — подпись ⚠ пустого зеркала для строки правила.
func RuleDNSNoEffectMark() string {
	return nodewarn.WarnMark + " " + locale.T(ruleDNSNoEffectText)
}

// ruleDNSTab — виджеты вкладки DNS.
type ruleDNSTab struct {
	box fyne.CanvasObject

	answer        *widget.RadioGroup
	answerDefault string
	answerServer  string
	answerFixed   string
	server        *widget.Select
	serverOptions []string
	fixed         *widget.Entry
	fixedRow      fyne.CanvasObject
	fixedNote     *widget.Label
	fixedError    *widget.Label
	forceIPv4     *widget.Check
	blockHTTPS    *widget.Check
	narrowNote    *widget.Label
	noEffect      *widget.Label
	hint          *widget.Label

	// hadDomains — у правила было доменное условие при прошлой проверке:
	// Block HTTPS ставится сам, когда домены появились из пустого (решение
	// 1.1.173); при открытии — состояние сохранённого правила.
	hadDomains bool
}

func newRuleDNSTab(model *wizardmodels.WizardModel, initial *corestate.RuleDNS, initialBody map[string]interface{}, wheel *components.WheelThrough, onChange func()) *ruleDNSTab {
	d := &ruleDNSTab{
		answerDefault: locale.T("Default — queries go to DNS Final"),
		answerServer:  locale.T("Dedicated server"),
		answerFixed:   locale.T("Fixed IP"),
	}
	d.serverOptions = wizardbusiness.DNSEnabledTagOptions(model)
	if initial != nil && initial.ServerTag != "" && !containsString(d.serverOptions, initial.ServerTag) {
		d.serverOptions = append(d.serverOptions, initial.ServerTag)
	}
	// Ответ на DNS-запросы под правило — один из трёх (контракт 1.1.139):
	// DNS Final, свой сервер или локальный ответ адресами fixedIps. Тег
	// сервера хранится при любом выборе; адреса пишутся только в режиме
	// Fixed IP.
	d.answer = widget.NewRadioGroup([]string{d.answerDefault, d.answerServer, d.answerFixed}, nil)
	d.answer.Required = true
	d.server = widget.NewSelect(d.serverOptions, nil)
	d.fixed = widget.NewMultiLineEntry()
	d.fixed.SetPlaceHolder("192.168.1.10")
	d.fixed.SetMinRowsVisible(ruleEntryRows)
	d.fixedNote = widget.NewLabel(locale.T("One address per line, IPv4 or IPv6."))
	d.fixedNote.Importance = widget.LowImportance
	d.fixedError = widget.NewLabel("")
	d.fixedError.Wrapping = fyne.TextWrapWord
	d.fixedError.Importance = widget.DangerImportance
	d.fixedError.Hide()
	d.forceIPv4 = widget.NewCheck(locale.T("Force IPv4 (drop AAAA)"), nil)
	// Block HTTPS (контракт 1.1.173): HTTPS/SVCB под правило — пустым
	// ответом, чтобы браузер не взял ECH и SNI остался открытым.
	d.blockHTTPS = widget.NewCheck(locale.T("Block HTTPS/SVCB records (no ECH)"), nil)
	d.narrowNote = widget.NewLabel(locale.T("DNS options apply to these domains on any port and protocol."))
	d.narrowNote.Wrapping = fyne.TextWrapWord
	d.narrowNote.Importance = widget.LowImportance
	d.narrowNote.Hide()
	d.noEffect = widget.NewLabel(RuleDNSNoEffectMark())
	d.noEffect.Wrapping = fyne.TextWrapWord
	d.noEffect.Importance = widget.WarningImportance
	d.noEffect.Hide()
	d.hint = widget.NewLabel("")
	d.hint.Wrapping = fyne.TextWrapWord
	d.hint.Importance = widget.LowImportance
	d.hint.Hide()

	d.answer.SetSelected(d.answerDefault)
	if initial != nil {
		d.server.SetSelected(initial.ServerTag)
		d.forceIPv4.SetChecked(initial.ForceIPv4)
		d.blockHTTPS.SetChecked(initial.BlockHTTPS)
		d.fixed.SetText(strings.Join(initial.FixedIPs, "\n"))
		switch {
		case len(initial.FixedIPs) > 0:
			d.answer.SetSelected(d.answerFixed)
		case initial.Enabled:
			d.answer.SetSelected(d.answerServer)
		}
	}
	d.hadDomains = ruleBodyHasDomainKeys(initialBody)

	changed := func() {
		if onChange != nil {
			onChange()
		}
	}
	d.answer.OnChanged = func(mode string) {
		if mode == d.answerServer && d.server.Selected == "" && len(d.serverOptions) > 0 {
			d.server.SetSelected(d.serverOptions[0])
		}
		changed()
	}
	d.fixed.OnChanged = func(string) { changed() }
	d.server.OnChanged = func(string) { changed() }
	d.forceIPv4.OnChanged = func(bool) { changed() }
	d.blockHTTPS.OnChanged = func(bool) { changed() }
	// Колесо над полем адресов двигает вкладку (WheelThrough окна); прячется
	// и показывается строка целиком, вместе со слоем.
	d.fixedRow = wheelEntry(wheel, d.fixed)

	d.box = container.NewVBox(
		widget.NewLabel(locale.T("Answer")),
		d.answer,
		d.server,
		d.fixedRow,
		d.fixedNote,
		d.fixedError,
		d.forceIPv4,
		d.blockHTTPS,
		d.noEffect,
		d.narrowNote,
		d.hint,
	)
	return d
}

// fixedIPs — адреса поля по строкам без пустых; bad — номер первой строки
// (с 1), не разбираемой как IP, 0 — все годные.
func (d *ruleDNSTab) fixedIPs() (ips []string, bad int) {
	for i, line := range strings.Split(d.fixed.Text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if net.ParseIP(line) == nil && bad == 0 {
			bad = i + 1
		}
		ips = append(ips, line)
	}
	return ips, bad
}

// answerError — почему Save недоступен из-за DNS; пусто — годно.
func (d *ruleDNSTab) answerError() string {
	if d.answer.Selected != d.answerFixed {
		return ""
	}
	ips, bad := d.fixedIPs()
	switch {
	case bad > 0:
		return fmt.Sprintf(locale.T("Line %d is not an IPv4 or IPv6 address."), bad)
	case len(ips) == 0:
		return locale.T("Enter an IP address.")
	}
	return ""
}

// value — объект dns записи.
func (d *ruleDNSTab) value() *corestate.RuleDNS {
	v := &corestate.RuleDNS{
		Enabled:    d.answer.Selected == d.answerServer,
		ServerTag:  d.server.Selected,
		ForceIPv4:  d.forceIPv4.Checked,
		BlockHTTPS: d.blockHTTPS.Checked,
	}
	if d.answer.Selected == d.answerFixed {
		v.FixedIPs, _ = d.fixedIPs()
	}
	return corestate.CloneRuleDNS(v)
}

// update — подсказки и доступность по телу правила (с `refs`, с целью);
// body nil — правило не заполнено.
func (d *ruleDNSTab) update(body map[string]interface{}) {
	mirror, _ := build.RuleDNSMirrorForBody(body)
	hasDomains := ruleBodyHasDomainKeys(body)
	rose := hasDomains && !d.hadDomains
	d.hadDomains = hasDomains
	if rose {
		d.blockHTTPS.SetChecked(true)
	}
	if build.RuleDNSBlockHTTPSAvailable(body, false) {
		d.blockHTTPS.Enable()
	} else {
		d.blockHTTPS.Disable()
	}
	if mirror != nil && build.RuleDNSMirrorNarrowed(body) {
		d.narrowNote.Show()
	} else {
		d.narrowNote.Hide()
	}
	if body != nil && build.RuleDNSMirrorEmpty(body, d.value()) {
		d.noEffect.Show()
	} else {
		d.noEffect.Hide()
	}

	mode := d.answer.Selected
	if mode == d.answerServer {
		d.server.Show()
	} else {
		d.server.Hide()
	}
	if mode == d.answerFixed {
		d.fixedRow.Show()
		d.fixedNote.Show()
		// Ответ при Fixed IP даёт само правило: глушилке AAAA нечего глушить.
		d.forceIPv4.Disable()
	} else {
		d.fixedRow.Hide()
		d.fixedNote.Hide()
		d.forceIPv4.Enable()
	}
	if msg := d.answerError(); msg != "" {
		d.fixedError.SetText(msg)
		d.fixedError.Show()
	} else {
		d.fixedError.Hide()
	}
	if mirror != nil && mode != d.answerDefault && len(corestate.BodyRefs(body)) > 0 {
		d.hint.SetText(locale.T("Works only if the rule set contains domains."))
		d.hint.Show()
	} else {
		d.hint.Hide()
	}
}

// ruleBodyHasDomainKeys — хоть в одном простом узле дерева есть доменный ключ
// (группа domain таблицы условий). Ссылки на наборы сюда не входят: галку
// Block HTTPS сама ставит только поява доменов, как до SPEC 188.
func ruleBodyHasDomainKeys(body map[string]interface{}) bool {
	reg, err := registry.Get()
	if err != nil || body == nil {
		return false
	}
	domains := reg.RuleDomainKeys()
	found := false
	corestate.WalkRuleTree(body, func(_ []int, node map[string]interface{}, logical bool) bool {
		if logical {
			return true
		}
		for k, v := range node {
			if domains[k] && !wizardmodels.RuleCondValueEmpty(v) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
