// Package autogroupform — форма настроек автогруппы (SPEC 104/108).
//
// Одни и те же настройки нужны в двух местах: на вкладке «Автовыбор»
// Направления и на вкладке «Группа» свёрнутой подписки. Это одна и та же
// сущность (configtypes.DirectionAuto) на двух уровнях, и вторая
// реализация формы разъехалась бы с первой на первой же правке — поэтому
// виджеты, разметка и чтение/запись живут здесь.
//
// Пакет намеренно не знает ни о презентере, ни о модели визарда: значения
// переменных шаблона приезжают параметром (VarChoices).
package autogroupform

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/locale"
)

// StickyHashKeys — компоненты sticky_hash в порядке отображения.
var StickyHashKeys = []string{"process", "domain", "source_ip", "dest_ip", "dest_port"}

// LabelWidth — ширина колонки подписей.
//
// Ряд «подпись — поле» строится на Border, а не на GridWithColumns(2):
// сетка делит ширину поровну, и узкие подписи («URL», «Interval») отжимают
// поля на половину окна.
const LabelWidth = 150

// VarChoices — варианты значения одного поля: подписи в порядке показа и
// отображение подпись → сырое значение (число либо "@urltest_tolerance").
type VarChoices struct {
	Labels       []string
	LabelToValue map[string]string
}

// defaultLabel — вариант для незаданного поля: первый в списке, то есть
// placeholder «@urltest_*» («наследовать из Settings»). Пусто только если
// список не собрался — тогда и подставлять нечего.
func (c VarChoices) defaultLabel() string {
	if len(c.Labels) == 0 {
		return ""
	}
	return c.Labels[0]
}

// Умолчания полей пула (SPEC 088). Числовые, а не ссылки на переменные:
// шаблон переменных для пула не объявляет, а нулевой пул — это не
// «наследовать», а группа, которая ничего не выбирает.
const (
	defaultPoolSize      = 3
	defaultPoolTolerance = 0
)

// Choices — списки значений для трёх полей, берущиеся из переменных
// шаблона (`@urltest_*`): пользователь должен иметь возможность выбрать
// «наследовать из Settings», а не только конкретное число.
type Choices struct {
	Interval  VarChoices
	Tolerance VarChoices
	URL       VarChoices
}

// Form — виджеты настроек автогруппы и их разметка.
type Form struct {
	ModeSelect         *widget.Select
	IntervalSelect     *widget.Select
	ToleranceSelect    *widget.Select
	URLEntry           *widget.SelectEntry
	PoolEntry          *widget.Entry
	PoolToleranceEntry *widget.Entry
	StickyChecks       map[string]*widget.Check

	balancerBlock *fyne.Container
	choices       Choices
}

// Row — ряд «подпись — поле» с подписью фиксированной ширины.
func Row(label fyne.CanvasObject, field fyne.CanvasObject) fyne.CanvasObject {
	box := container.NewGridWrap(fyne.NewSize(LabelWidth, label.MinSize().Height), label)
	return container.NewBorder(nil, nil, box, nil, field)
}

// TextRow — то же самое с обычной текстовой подписью.
func TextRow(text string, field fyne.CanvasObject) fyne.CanvasObject {
	return Row(widget.NewLabel(text), field)
}

// New собирает форму. modeLabel передаётся вызывающим, чтобы к подписи
// режима можно было прицепить свою подсказку.
func New(choices Choices) *Form {
	f := &Form{choices: choices}

	f.ModeSelect = widget.NewSelect([]string{
		locale.T("wizard.outbound.auto_mode_least_test"),
		locale.T("wizard.outbound.auto_mode_round_robin"),
	}, nil)
	f.ModeSelect.SetSelected(locale.T("wizard.outbound.auto_mode_least_test"))

	f.IntervalSelect = widget.NewSelect(choices.Interval.Labels, nil)
	f.ToleranceSelect = widget.NewSelect(choices.Tolerance.Labels, nil)
	f.URLEntry = widget.NewSelectEntry(choices.URL.Labels)
	f.URLEntry.SetPlaceHolder("https://cp.cloudflare.com/generate_204")
	f.PoolEntry = widget.NewEntry()
	f.PoolEntry.SetPlaceHolder("3")
	f.PoolToleranceEntry = widget.NewEntry()
	f.PoolToleranceEntry.SetPlaceHolder("0")

	f.StickyChecks = make(map[string]*widget.Check, len(StickyHashKeys))
	// Два ряда, а не одна строка: пять чекбоксов в ширину окна не
	// помещаются, и последний уезжает за край.
	row1 := container.NewHBox()
	row2 := container.NewHBox()
	for i, k := range StickyHashKeys {
		ch := widget.NewCheck(k, nil)
		f.StickyChecks[k] = ch
		if i < 3 {
			row1.Add(ch)
		} else {
			row2.Add(ch)
		}
	}

	f.balancerBlock = container.NewVBox(
		TextRow("Pool size", f.PoolEntry),
		TextRow("Pool tolerance (ms)", f.PoolToleranceEntry),
		widget.NewLabel("Sticky hash"),
		container.NewVBox(row1, row2),
	)

	return f
}

// Content собирает разметку формы. hint — подсказка над полями (может быть
// nil), modeLabel — подпись поля режима.
func (f *Form) Content(hint fyne.CanvasObject, modeLabel fyne.CanvasObject) fyne.CanvasObject {
	form := container.NewVBox()
	if hint != nil {
		form.Add(hint)
	}
	form.Add(Row(modeLabel, f.ModeSelect))
	form.Add(TextRow("Interval", f.IntervalSelect))
	form.Add(TextRow("Tolerance (ms)", f.ToleranceSelect))
	form.Add(TextRow("URL", f.URLEntry))
	form.Add(f.balancerBlock)

	// Та же ширина и отступ под скроллбар, что у соседних вкладок.
	rightGap := canvas.NewRectangle(color.Transparent)
	rightGap.SetMinSize(fyne.NewSize(20, 0))
	widthSpacer := canvas.NewRectangle(color.Transparent)
	widthSpacer.SetMinSize(fyne.NewSize(400, 0))

	f.syncBalancerVisible()
	// Обработчик ПОСЛЕ SetSelected — иначе он сработает на программной
	// установке значения и уведёт форму в рекурсию перестроений.
	f.ModeSelect.OnChanged = func(string) { f.syncBalancerVisible() }

	return container.NewStack(
		widthSpacer,
		container.NewBorder(nil, nil, nil, rightGap, form),
	)
}

func (f *Form) syncBalancerVisible() {
	if f.ModeSelect.Selected == locale.T("wizard.outbound.auto_mode_round_robin") {
		f.balancerBlock.Show()
	} else {
		f.balancerBlock.Hide()
	}
}

// Load заполняет форму из Auto. nil очищает поля.
func (f *Form) Load(a *configtypes.DirectionAuto) {
	f.ModeSelect.OnChanged = nil
	if a != nil && a.Mode == configtypes.AutoModeRoundRobin {
		f.ModeSelect.SetSelected(locale.T("wizard.outbound.auto_mode_round_robin"))
	} else {
		f.ModeSelect.SetSelected(locale.T("wizard.outbound.auto_mode_least_test"))
	}
	f.ModeSelect.OnChanged = func(string) { f.syncBalancerVisible() }

	// Незаданное поле показывает НЕ пустоту, а первый вариант списка —
	// сам placeholder «@urltest_*», то есть «наследовать значение из
	// Settings». Пустой выбор здесь означал бы, что группа уедет в конфиг
	// вообще без интервала и tolerance, и пользователь об этом не узнал
	// бы: «(Select one)» читается как «ещё не выбрано», а не как «ничего».
	f.IntervalSelect.SetSelected(f.choices.Interval.defaultLabel())
	f.ToleranceSelect.SetSelected(f.choices.Tolerance.defaultLabel())
	f.URLEntry.SetText(f.choices.URL.defaultLabel())
	// У пула дефолты числовые: размер 0 — это не «наследовать», а
	// неработающая группа, поэтому подставляем то же, что стояло
	// подсказкой в пустом поле.
	f.PoolEntry.SetText(strconv.Itoa(defaultPoolSize))
	f.PoolToleranceEntry.SetText(strconv.Itoa(defaultPoolTolerance))
	for _, ch := range f.StickyChecks {
		ch.SetChecked(false)
	}

	if a != nil {
		if a.Interval != "" {
			if lbl := labelForValue(f.choices.Interval.LabelToValue, a.Interval); lbl != "" {
				f.IntervalSelect.SetSelected(lbl)
			}
		}
		if v := a.Tolerance.Value(); v != nil {
			if lbl := labelForValue(f.choices.Tolerance.LabelToValue, fmt.Sprint(v)); lbl != "" {
				f.ToleranceSelect.SetSelected(lbl)
			}
		}
		if a.URL != "" {
			f.URLEntry.SetText(a.URL)
		}
		if a.Pool > 0 {
			f.PoolEntry.SetText(strconv.Itoa(a.Pool))
		}
		if n, ok := a.PoolTolerance.Int(); ok {
			f.PoolToleranceEntry.SetText(strconv.Itoa(n))
		}
		for _, k := range a.StickyHash {
			if ch := f.StickyChecks[k]; ch != nil {
				ch.SetChecked(true)
			}
		}
	}
	f.syncBalancerVisible()
}

// Collect читает форму в DirectionAuto.
func (f *Form) Collect() *configtypes.DirectionAuto {
	auto := &configtypes.DirectionAuto{}
	if f.ModeSelect.Selected == locale.T("wizard.outbound.auto_mode_round_robin") {
		auto.Mode = configtypes.AutoModeRoundRobin
	}
	if lbl := f.IntervalSelect.Selected; lbl != "" {
		auto.Interval = f.choices.Interval.LabelToValue[lbl]
	}
	if lbl := f.ToleranceSelect.Selected; lbl != "" {
		auto.Tolerance = parseTemplateInt(f.choices.Tolerance.LabelToValue[lbl])
	}
	auto.URL = strings.TrimSpace(f.URLEntry.Text)
	if auto.Mode == configtypes.AutoModeRoundRobin {
		if n, err := strconv.Atoi(strings.TrimSpace(f.PoolEntry.Text)); err == nil {
			auto.Pool = n
		}
		if n, err := strconv.Atoi(strings.TrimSpace(f.PoolToleranceEntry.Text)); err == nil {
			auto.PoolTolerance = configtypes.NewTemplateInt(n)
		}
		for _, k := range StickyHashKeys {
			if ch := f.StickyChecks[k]; ch != nil && ch.Checked {
				auto.StickyHash = append(auto.StickyHash, k)
			}
		}
	}
	return auto
}

// parseTemplateInt — значение может быть числом или ссылкой на переменную
// («@urltest_tolerance»). Ссылку сохраняем как есть: подстановка идёт на
// сборке, и разворачивать её здесь значило бы зашить в состояние текущее
// значение настройки навсегда.
func parseTemplateInt(v string) *configtypes.TemplateInt {
	if v == "" {
		return nil
	}
	if strings.HasPrefix(v, "@") {
		return configtypes.NewTemplateVar(strings.TrimPrefix(v, "@"))
	}
	if n, err := strconv.Atoi(v); err == nil {
		return configtypes.NewTemplateInt(n)
	}
	return nil
}

func labelForValue(m map[string]string, value string) string {
	for lbl, v := range m {
		if v == value {
			return lbl
		}
	}
	return ""
}
