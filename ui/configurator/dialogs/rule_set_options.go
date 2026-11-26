package dialogs

// rule_set_options.go — блок «Rule-set updates» в окнах правила SRS и пресета
// (SPEC 185 §2.5): кто качает наборы правила (лаунчер или ядро), как часто и
// через какой выход. Пустое значение поля — взять общее из Settings.

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
)

const ruleSetRemoteWarningText = "Remote: the core downloads the rule-set itself. If the first download fails and there is no cached copy, the core will not start."

// RuleSetOptionsForm — виджеты блока и чтение значения.
type RuleSetOptionsForm struct {
	// Box — готовый блок для вставки в форму.
	Box *fyne.Container

	mode     *widget.Select
	interval *widget.Entry
	detour   *widget.Select
	initial  corestate.RuleSetOptions
}

func ruleSetModeDefault() string { return locale.T("Default (from Settings)") }
func ruleSetModeLocal() string   { return locale.T("Local — the launcher downloads") }
func ruleSetModeRemote() string  { return locale.T("Remote — the core downloads") }

// NewRuleSetOptionsForm строит блок. outbounds — теги выходов для выбора
// «Download via». onChanged (может быть nil) зовётся на любую правку.
func NewRuleSetOptionsForm(initial corestate.RuleSetOptions, outbounds []string, onChanged func()) *RuleSetOptionsForm {
	f := &RuleSetOptionsForm{initial: initial.Clone()}
	changed := func() {
		if onChanged != nil {
			onChanged()
		}
	}

	warning := widget.NewLabel(locale.T(ruleSetRemoteWarningText))
	warning.Wrapping = fyne.TextWrapWord
	warning.Importance = widget.WarningImportance

	detourLabel := widget.NewLabel(locale.T("Download via (remote only):"))
	detourOptions := append([]string{ruleSetModeDefault()}, outbounds...)
	// Сохранённая цель, которой уже нет среди выходов, остаётся в списке —
	// молча подменить её значит незаметно сменить маршрут загрузки.
	if initial.DownloadDetour != "" && !containsString(outbounds, initial.DownloadDetour) {
		detourOptions = append(detourOptions, initial.DownloadDetour)
	}
	f.detour = widget.NewSelect(detourOptions, func(string) { changed() })
	if initial.DownloadDetour != "" {
		f.detour.SetSelected(initial.DownloadDetour)
	} else {
		f.detour.SetSelected(ruleSetModeDefault())
	}

	syncRemote := func(sel string) {
		if sel == ruleSetModeLocal() {
			detourLabel.Hide()
			f.detour.Hide()
		} else {
			detourLabel.Show()
			f.detour.Show()
		}
		if sel == ruleSetModeRemote() {
			warning.Show()
		} else {
			warning.Hide()
		}
	}

	f.mode = widget.NewSelect([]string{ruleSetModeDefault(), ruleSetModeLocal(), ruleSetModeRemote()}, func(sel string) {
		syncRemote(sel)
		changed()
	})
	switch initial.Mode {
	case corestate.SrsModeLocal:
		f.mode.SetSelected(ruleSetModeLocal())
	case corestate.SrsModeRemote:
		f.mode.SetSelected(ruleSetModeRemote())
	default:
		f.mode.SetSelected(ruleSetModeDefault())
	}

	f.interval = widget.NewEntry()
	f.interval.SetPlaceHolder(locale.T("default"))
	if initial.IntervalHours != nil {
		f.interval.SetText(strconv.Itoa(*initial.IntervalHours))
	}
	f.interval.OnChanged = func(string) { changed() }

	title := widget.NewLabel(locale.T("Rule-set updates"))
	title.TextStyle = fyne.TextStyle{Bold: true}

	f.Box = container.NewVBox(
		widget.NewSeparator(),
		title,
		widget.NewLabel(locale.T("Mode:")),
		f.mode,
		warning,
		widget.NewLabel(locale.T("Update interval, hours (empty — default, 0 — never):")),
		f.interval,
		detourLabel,
		f.detour,
	)
	syncRemote(f.mode.Selected)
	return f
}

// Value — настройки из виджетов. Нечисловой или отрицательный период читается
// как «общее».
func (f *RuleSetOptionsForm) Value() corestate.RuleSetOptions {
	var o corestate.RuleSetOptions
	switch f.mode.Selected {
	case ruleSetModeLocal():
		o.Mode = corestate.SrsModeLocal
	case ruleSetModeRemote():
		o.Mode = corestate.SrsModeRemote
	}
	if raw := strings.TrimSpace(f.interval.Text); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			o.IntervalHours = &n
		}
	}
	if sel := f.detour.Selected; sel != "" && sel != ruleSetModeDefault() {
		o.DownloadDetour = sel
	}
	return o
}

// SetModeLocked запирает выбор режима (SPEC 187): правило с набором-файлом
// (file://) всегда идёт в режиме local, что бы ни стояло в поле. Значение
// поля не меняется — убрали файл из правила, выбор вернулся.
func (f *RuleSetOptionsForm) SetModeLocked(locked bool) {
	if locked {
		f.mode.Disable()
		return
	}
	f.mode.Enable()
}
