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
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
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
	var out []wizardtemplate.TemplateVar
	for _, v := range m.TemplateData.Vars {
		if strings.HasPrefix(v.Name, prefix) {
			out = append(out, v)
		}
	}
	return out
}

// showDNSTemplateVarsDialog открывает параметры шаблонного сервера.
func showDNSTemplateVarsDialog(p *wizardpresentation.WizardPresenter, w fyne.Window, tag string) {
	if w == nil {
		w = p.DialogParent()
	}
	if w == nil {
		return
	}
	vars := dnsServerVarsFor(p, tag)
	if len(vars) == 0 {
		return
	}
	m := p.Model()
	gs := p.GUIState()

	rows := container.NewVBox()
	for _, vd := range vars {
		title := vd.Title
		if title == "" {
			// Имя показываем без служебного префикса: пользователь видит
			// эту переменную в контексте своего сервера, и «dns_google_dot_»
			// в подписи — шум.
			title = strings.TrimPrefix(vd.Name, "dns_"+tag+"_")
		}
		rows.Add(buildSettingsVarRow(p, m, m.TemplateData, vd, title, vd.Tooltip, true, gs))
	}

	hint := widget.NewLabel(locale.T("wizard.dns.template_vars_hint"))
	hint.Wrapping = fyne.TextWrapWord

	var dlg dialog.Dialog
	closeBtn := widget.NewButton(locale.T("wizard.dns.dialog_cancel"), func() {
		if dlg != nil {
			dlg.Hide()
		}
	})
	body := container.NewVScroll(container.NewVBox(hint, rows))
	dlg = dialogs.NewCustom(
		locale.Tf("wizard.dns.template_vars_title", tag),
		body,
		container.NewHBox(layout.NewSpacer(), closeBtn),
		"", w)
	dlg.Resize(fyne.NewSize(560, 420))
	dlg.Show()
}
