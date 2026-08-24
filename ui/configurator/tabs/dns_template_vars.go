// File dns_template_vars.go — правка параметров шаблонного DNS-сервера
// (SPEC 109, фаза 3).
//
// Записи серверов, пришедшие из шаблона во вложенной форме, объявляют
// собственные `vars`: канал запроса, адрес провайдера, профиль Safe DNS,
// резолвер имени. При загрузке они разворачиваются в переменные шаблона с
// именем `dns_<tag>_<var>` (core/template.NormalizeDNSOptions) и прячутся
// с вкладки Settings — иначе список настроек распух бы на два десятка
// неотличимых строк «Outbound».
//
// Прятать, не дав другого места правки, значило бы перенести
// параметризацию и тут же её обезвредить. Это место — здесь: кнопка у
// строки шаблонного сервера открывает его переменные.
//
// Строки собирает buildSettingsVarRow — тот же конструктор, что на вкладке
// Settings. Своя реализация формы разошлась бы с ним на первом же новом
// типе переменной.
package tabs

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// dnsServerVarsFor — переменные, объявленные записью сервера с этим тегом.
//
// Отбор по префиксу имени, а не по отдельному списку: префикс проставлен
// при нормализации и является единственной связью переменной с её записью
// (в самой переменной ссылки на сервер нет).
func dnsServerVarsFor(p *wizardpresentation.WizardPresenter, tag string) []wizardtemplate.TemplateVar {
	m := p.Model()
	if m == nil || m.TemplateData == nil || tag == "" {
		return nil
	}
	prefix := "dns_" + tag + "_"
	// Тег, являющийся префиксом другого (google_doh / google_doh_vpn),
	// притягивал бы чужие переменные: dns_google_doh_vpn_x начинается и с
	// dns_google_doh_. Переменная принадлежит САМОМУ ДЛИННОМУ тегу, чей
	// префикс её покрывает.
	longer := make([]string, 0, 2)
	for t := range wizardbusiness.ExtractTemplateDNSTags(m.TemplateData) {
		if t != tag && strings.HasPrefix(t, tag+"_") {
			longer = append(longer, "dns_"+t+"_")
		}
	}
	var out []wizardtemplate.TemplateVar
	for _, v := range m.TemplateData.Vars {
		if !strings.HasPrefix(v.Name, prefix) {
			continue
		}
		owned := true
		for _, lp := range longer {
			if strings.HasPrefix(v.Name, lp) {
				owned = false
				break
			}
		}
		if owned {
			out = append(out, v)
		}
	}
	return out
}

// dnsTemplateVarRows — строки параметров сервера, готовые к встраиванию.
//
// Отдельно от диалога: параметры шаблонной записи показываются прямо в её
// окне, сверху — это единственное, что там можно менять, и прятать их за
// кнопку значило бы требовать лишний клик ради того, за чем окно и
// открывают. nil, если параметров нет.
func dnsTemplateVarRows(p *wizardpresentation.WizardPresenter, tag string) fyne.CanvasObject {
	vars := dnsServerVarsFor(p, tag)
	if len(vars) == 0 {
		return nil
	}
	m := p.Model()
	gs := p.GUIState()

	head := widget.NewLabelWithStyle(
		locale.T("wizard.dns.template_vars_head"),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	rows := container.NewVBox(head)
	for _, vd := range vars {
		title := vd.Title
		if title == "" {
			// Имя без служебного префикса: пользователь видит переменную в
			// контексте своего сервера, и «dns_google_dot_» в подписи — шум.
			title = strings.TrimPrefix(vd.Name, "dns_"+tag+"_")
		}
		rows.Add(buildSettingsVarRow(p, m, m.TemplateData, vd, title, vd.Tooltip, true, gs))
	}
	return rows
}
