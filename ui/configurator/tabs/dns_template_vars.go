// File dns_template_vars.go — правка параметров шаблонного DNS-сервера
// (SPEC 109, фаза 3; SPEC 129).
//
// Записи серверов, пришедшие из шаблона во вложенной форме, объявляют
// собственные `vars`: канал запроса, адрес провайдера, профиль Safe DNS,
// резолвер имени. Объявления живут при сервере
// (TemplateData.DNSServerVars[tag]), значения — в записи состояния
// `dns.servers[kind=template].vars`, в модели — model.DNSTemplateVars[tag].
// До SPEC 129 они склеивались в переменные шаблона `dns_<tag>_<var>` и
// прятались с вкладки Settings.
//
// Место правки — окно самого сервера: параметры шаблонной записи
// показываются прямо в нём, сверху.
//
// Строки собирает buildVarRow — тот же конструктор, что на вкладке Settings,
// над хранилищем записи (varRowStore): своя реализация формы разошлась бы с
// ним на первом же новом типе переменной.
package tabs

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// dnsServerVarsFor — переменные, объявленные записью сервера с этим тегом.
func dnsServerVarsFor(p *wizardpresentation.WizardPresenter, tag string) []wizardtemplate.TemplateVar {
	m := p.Model()
	if m == nil || m.TemplateData == nil || tag == "" {
		return nil
	}
	return m.TemplateData.DNSServerVars[tag]
}

// dnsServerVarRowStore — хранилище строк параметров сервера: значения записи
// в model.DNSTemplateVars[tag] по нормам SPEC 129.
//
// Выбор значения, равного умолчанию сервера для таргета модели, и сброс
// снимают ключ (Н4: отсутствие ключа — «следовать шаблону»); пустое после
// подрезки — тоже (Н3). Модель помечается изменённой только при реальной
// смене карты: построение строки (SetSelected зовёт OnChanged) выбора не
// меняет (Л4).
//
// rebuildRows — перестройка строк окна после сброса: сброс не проходит через
// виджет, и без перестройки строка показывала бы снятое значение.
func dnsServerVarRowStore(p *wizardpresentation.WizardPresenter, m *wizardmodels.WizardModel, td *wizardtemplate.TemplateData, tag string, decls []wizardtemplate.TemplateVar, rebuildRows func()) varRowStore {
	target := m.Target.Normalized()
	defaultOf := func(name string) string {
		return wizardtemplate.DNSServerVarDefault(decls, name, td.Vars, m.SettingsVars, target)
	}
	record := func() map[string]string { return m.DNSTemplateVars[tag] }
	remove := func(name string) bool {
		vars := record()
		if _, had := vars[name]; !had {
			return false
		}
		delete(vars, name)
		if len(vars) == 0 {
			delete(m.DNSTemplateVars, tag)
		}
		return true
	}

	// Значения для показа: переменные шаблона — из Settings, имена сервера —
	// из записи поверх (локальное имя затеняет глобальное).
	values := make(map[string]string, len(m.SettingsVars)+len(decls))
	for k, v := range m.SettingsVars {
		values[k] = v
	}
	local := make(map[string]bool, len(decls))
	for _, d := range decls {
		local[d.Name] = true
		delete(values, d.Name)
	}
	for k, v := range record() {
		if local[k] {
			values[k] = v
		}
	}

	return varRowStore{
		values: values,
		vars:   wizardtemplate.DNSServerVarScope(decls, td.Vars),
		stored: func(name string) (string, bool) {
			v, ok := record()[name]
			return v, ok
		},
		set: func(name, value string) bool {
			value = strings.TrimSpace(value)
			if value == "" || value == defaultOf(name) {
				delete(values, name)
				return remove(name)
			}
			values[name] = value
			if cur, ok := record()[name]; ok && cur == value {
				return false
			}
			if m.DNSTemplateVars == nil {
				m.DNSTemplateVars = make(map[string]map[string]string)
			}
			if m.DNSTemplateVars[tag] == nil {
				m.DNSTemplateVars[tag] = make(map[string]string)
			}
			m.DNSTemplateVars[tag][name] = value
			return true
		},
		reset: func(name string) bool {
			delete(values, name)
			return remove(name)
		},
		// Подпись строки списка показывает подставленные значения — после
		// правки параметра её надо перерисовать.
		afterChange: func(_ string, changed, _ bool) {
			if changed {
				p.RefreshDNSListAndSelects()
			}
		},
		afterReset: func(string) {
			p.RefreshDNSListAndSelects()
			if rebuildRows != nil {
				rebuildRows()
			}
		},
		keepForeignEnum: true,
	}
}

// dnsTemplateVarRows — строки параметров сервера, готовые к встраиванию.
//
// Отдельно от диалога: параметры шаблонной записи показываются прямо в её
// окне, сверху — это единственное, что там можно менять, и прятать их за
// кнопку значило бы требовать лишний клик ради того, за чем окно и
// открывают. nil, если параметров нет.
func dnsTemplateVarRows(p *wizardpresentation.WizardPresenter, tag string) fyne.CanvasObject {
	decls := dnsServerVarsFor(p, tag)
	if len(decls) == 0 {
		return nil
	}
	m := p.Model()
	gs := p.GUIState()
	td := m.TemplateData

	rows := container.NewVBox()
	var rebuild func()
	rebuild = func() {
		rows.Objects = nil
		rows.Add(widget.NewLabelWithStyle(
			locale.T("Parameters"),
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		for _, vd := range decls {
			title := vd.Title
			if title == "" {
				title = vd.Name
			}
			store := dnsServerVarRowStore(p, m, td, tag, decls, rebuild)
			rows.Add(buildVarRow(p, m, td, vd, title, vd.Tooltip, true, gs, store))
		}
		rows.Refresh()
	}
	rebuild()
	return rows
}
