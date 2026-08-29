// Package outbounds_configurator provides reusable UI for configuring outbounds in the wizard:
// list of all outbounds (global + per-source) with Edit/Delete/Add.
//
// SPEC 117: все операции пакета выполняются на canonical-модели
// (model.GlobalOutbounds); каждая мутация
// завершается model.BumpRevision(). Legacy-проекция ParserConfig не
// читается и не пишется.
package outbounds_configurator

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardutils "singbox-launcher/ui/configurator/utils"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	outboundEnableTooltipText = "Include this direction in the config. A switched-off direction keeps its settings but is not built and cannot be picked as a rule target."
)

// OutboundEditPresenter is used to register the Edit/Add window with the wizard overlay (single instance, focus redirect).
type OutboundEditPresenter interface {
	OpenOutboundEditWindow() fyne.Window
	SetOutboundEditWindow(fyne.Window)
	ClearOutboundEditWindow()
	UpdateChildOverlay()
	Model() *wizardmodels.WizardModel
}

// NewConfiguratorContent builds a reusable outbounds configurator content for embedding into tabs.
// Направления берутся из canonical-модели (editPresenter.Model().GlobalOutbounds), её же мутируют все операции.
// onApply is called after each mutation (Edit/Add/Delete/drag) so the caller can invalidate caches and sync UI.
// editPresenter is required (Model() supplies the canonical model); when set, the Edit/Add window is registered for overlay.
// The returned refresh function rebuilds the list from the current model (call after outbounds change outside the list, e.g. Sources → Edit).
func NewConfiguratorContent(parent fyne.Window, editPresenter OutboundEditPresenter, onApply func()) (fyne.CanvasObject, func()) {
	listContent := container.NewVBox()

	var refreshList func()
	refreshList = func() {
		model := editPresenter.Model()
		if model == nil {
			listContent.Objects = nil
			listContent.Refresh()
			return
		}
		// SPEC 058-R-N: re-sync на каждый refresh — после любой мутации
		// (Restore missing / Add / Edit / Del / preset toggle) приводит
		// state в правильный shape: новые outbounds получают expected
		// preset update patches; orphan entries дропаются. Idempotent.
		syncOutboundsLocal(model)
		rows := collectRowsForUI(model)
		items := make([]fyne.CanvasObject, 0, len(rows)+2)

		// SPEC 109: перетаскивание вместо ↑/↓ — тот же механизм, что на
		// Rules и DNS. Порядок Направлений для КОНФИГА не значим (генератор
		// сортирует топологически по addOutbounds), но он определяет, что
		// форма предложит в addOutbounds: только записи выше текущей.
		dragGroup := fynewidget.NewDragReorderGroup(func(from, to int) {
			model := editPresenter.Model()
			if model == nil {
				return
			}
			outs := model.GlobalOutbounds
			if from < 0 || from >= len(outs) || to < 0 || to >= len(outs) || from == to {
				return
			}
			moved := outs[from]
			rest := append(outs[:from:from], outs[from+1:]...)
			out := make([]config.Direction, 0, len(outs))
			out = append(out, rest[:to]...)
			out = append(out, moved)
			out = append(out, rest[to:]...)
			model.GlobalOutbounds = out
			model.BumpRevision()
			refreshList()
			if onApply != nil {
				onApply()
			}
		})

		// SPEC 108: заголовки секций убраны вместе со строками групп
		// подписок — в списке остались только Направления, и делить его
		// больше не на что.
		for rowIdx, r := range rows {
			r := r
			rowIdx := rowIdx
			var row *fynewidget.HoverRow
			rowGetter := func() *fynewidget.HoverRow { return row }

			// SPEC 104: строка начинается с ИМЕНИ Направления, тег — рядом
			// в скобках, но только когда имя задано: иначе вышло бы
			// «vpn-1 (vpn-1)». Тег остаётся видимым при любом раскладе — на
			// него ссылаются правила, и прятать его значило бы заставить
			// пользователя гадать, что выбирать в списке целей.
			// Для ссылочных записей (ref != "") имя, тип и Auto живут в
			// шаблоне/пресете, в state лежит тонкая оболочка — берём
			// merged-тело, как это делает редактор. Иначе `proxy-out` с
			// auto{} в шаблоне в списке выглядел бы обычным селектором.
			shown := *r.Outbound
			if r.Outbound.Ref != "" {
				if merged := wizardbusiness.ResolveMergedOutbound(editPresenter.Model(), r.Outbound.Tag); merged != nil {
					shown = *merged
				}
			}
			// Строка начинается с тега — единственного имени Направления
			// (контракт 0.9.0). Прежде здесь стояло «имя (тег)», и одна и
			// та же запись читалась в списке одним именем, а в выпадашке
			// целей правил — другим.
			rawLine := shown.DisplayName()
			// Тип показываем только у самостоятельных urltest-групп шаблона:
			// у Направления он всегда selector, и писать это в каждой
			// строке — шум.
			if shown.Type == "urltest" {
				rawLine += " [" + shown.Type + "]"
			}
			if shown.Auto != nil {
				rawLine += " " + locale.T("· auto")
			}
			if r.Outbound.Disabled {
				rawLine = locale.T("(off)") + " " + rawLine
			}
			if r.SourceLabel != "" {
				rawLine += " — " + r.SourceLabel
			}
			rawLine = strings.ToValidUTF8(rawLine, "")
			displayLine := wizardutils.TruncateStringEllipsis(rawLine, wizardutils.MaxLabelRunes, "...")

			// Add transparent padding on the right so the list scrollbar has a visual strip.
			rightPadding := components.NewScrollGutter()

			nameLabel := ttwidget.NewLabel(displayLine)
			nameLabel.Wrapping = fyne.TextWrapOff
			nameLabel.Truncation = fyne.TextTruncateEllipsis
			nameLabel.SetToolTip(rawLine)

			var leftArrows, rightControls *fyne.Container

			// SPEC 057-R-N + SPEC 109: preset rows — natural slice members с
			// ref. Перетаскивание двигает элемент слайса целиком, поэтому
			// preset binding (ref + updates) переезжает вместе с телом.
			// Edit button — доступен для всех rows включая preset/required.
			// Для preset: scope locked, Ref/Updates preserved (sync-managed
			// metadata, не должны wipe'нуться юзерским body edit).
			editBtn := fynewidget.NewHoverForwardButtonWithIcon(locale.TN(1, "Edit"), theme.DocumentCreateIcon(), func() {
				rowsNow := collectRowsForUI(editPresenter.Model())
				if rowIdx >= len(rowsNow) {
					return
				}
				r2 := rowsNow[rowIdx]
				tagsForAdd := tagsAbove(rowsNow, rowIdx)
				ShowEditDialog(parent, editPresenter, r2.Outbound, r2.IsGlobal, r2.SourceIndex, tagsForAdd, func(updated *config.Direction, scopeKind string, sourceIndex int) {
					model := editPresenter.Model()
					if model == nil {
						return
					}
					// SPEC 118 W5: локальных Направлений источника больше нет —
					// Направление всегда глобальное, и scope выбирать не из
					// чего. Осталась одна ветка: правка тела на месте.
					{
						// In-place body update. SPEC 058-R-N: для referenced
						// entries Edit dialog (Phase 4 applyEditedConfig) уже
						// вычислил USER patch и put его в updated.Updates +
						// strip'нул body. Для direct entries — updated имеет
						// full body inline + сохранённые Updates. В обоих
						// случаях просто перезаписываем r2.Outbound = *updated
						// (указатель смотрит в canonical-слайс).
						*r2.Outbound = *updated
					}
					model.BumpRevision()
					refreshList()
					if onApply != nil {
						onApply()
					}
				})
			}, rowGetter)

			delBtn := fynewidget.NewHoverForwardButtonWithIcon(locale.T("Del"), theme.DeleteIcon(), func() {
				rowsNow := collectRowsForUI(editPresenter.Model())
				if rowIdx >= len(rowsNow) || rowsNow[rowIdx].IsPreset || rowsNow[rowIdx].IsRequired {
					return
				}
				r2 := rowsNow[rowIdx]
				model := editPresenter.Model()
				if model == nil {
					return
				}
				if r2.IsGlobal {
					model.GlobalOutbounds = append(model.GlobalOutbounds[:r2.IndexInSlice], model.GlobalOutbounds[r2.IndexInSlice+1:]...)
				}
				model.BumpRevision()
				refreshList()
				if onApply != nil {
					onApply()
				}
			}, rowGetter)

			// Reset button — clear USER patch для referenced entries (SPEC 058-R-N).
			// После Reset body возвращается к live template/preset defaults
			// (без USER override). Создаём всегда для referenced (чтобы row layout
			// не прыгал), но disable если HasUserPatch=false — нечего ресетить.
			// Для direct entries вообще не создаём (нет base для reset).
			var resetBtn *fynewidget.HoverForwardButton
			if r.IsTemplate || r.IsPreset {
				resetBtn = fynewidget.NewHoverForwardButtonWithIcon(locale.TN(1, "Reset"), theme.ViewRefreshIcon(), func() {
					rowsNow := collectRowsForUI(editPresenter.Model())
					if rowIdx >= len(rowsNow) {
						return
					}
					r2 := rowsNow[rowIdx]
					if !(r2.IsTemplate || r2.IsPreset) || !r2.IsGlobal || r2.IndexInSlice < 0 {
						return
					}
					model := editPresenter.Model()
					if model == nil || r2.IndexInSlice >= len(model.GlobalOutbounds) {
						return
					}
					// Strip USER patch из Updates[]; preset patches preserve.
					model.GlobalOutbounds[r2.IndexInSlice].Updates = build.UpsertUserPatch(
						model.GlobalOutbounds[r2.IndexInSlice].Updates,
						nil,
						false, // Reset снимает патч целиком — признак не при чём
					)
					model.BumpRevision()
					refreshList()
					if onApply != nil {
						onApply()
					}
				}, rowGetter)
				resetBtn.Importance = widget.LowImportance
				fynewidget.SetToolTipSafe(resetBtn, locale.T("Reset — clear your changes, revert to defaults"))
				if !r.HasUserPatch {
					resetBtn.Disable()
				}
			}

			dragHandle := fynewidget.NewDragHandle(dragGroup, rowIdx, rowGetter)
			leftArrows = container.NewHBox(dragHandle)
			// fixedWidthBtn — обёртка, фиксирующая минимальную ширину кнопки
			// (Reset > Del по тексту; без фиксации колонка действий "прыгает"
			// между rows). Stack комбинирует MinSize: max(sizer, btn).
			fixedWidthBtn := func(btn fyne.CanvasObject) fyne.CanvasObject {
				sizer := canvas.NewRectangle(color.Transparent)
				sizer.SetMinSize(fyne.NewSize(78, 0))
				return container.NewStack(sizer, btn)
			}
			// SPEC 104: выключение Направления — свойство записи, а не
			// форма: выключенное не материализуется и не предлагается целью
			// правил, но остаётся в списке со всеми настройками.
			// Служебные группы подписок (isLocal) не выключаются — их
			// жизненным циклом управляет сама подписка.
			var enableCheck *widget.Check
			if r.IsGlobal {
				// Обработчик вешаем ПОСЛЕ SetChecked, а не в конструкторе:
				// widget.Check.SetChecked зовёт OnChanged, тот перестраивает
				// список, новая строка снова зовёт SetChecked — и главный
				// поток уходит в бесконечную рекурсию (окно замирает).
				enableCheck = widget.NewCheck("", nil)
				enableCheck.SetChecked(!r.Outbound.Disabled)
				enableCheck.OnChanged = func(on bool) {
					model := editPresenter.Model()
					if model == nil {
						return
					}
					rowsNow := collectRowsForUI(model)
					if rowIdx >= len(rowsNow) {
						return
					}
					idx := rowsNow[rowIdx].IndexInSlice
					if idx < 0 || idx >= len(model.GlobalOutbounds) {
						return
					}
					if model.GlobalOutbounds[idx].Disabled == !on {
						return // значение уже такое — перестраивать нечего
					}
					model.GlobalOutbounds[idx].Disabled = !on
					model.BumpRevision()
					refreshList()
					if onApply != nil {
						onApply()
					}
				}
				fynewidget.SetToolTipSafe(enableCheck, locale.T(outboundEnableTooltipText))
			}

			switch {
			case r.IsPreset || r.IsRequired:
				// Locked rows: Edit + Reset, без Del.
				rightControls = container.NewHBox(editBtn, fixedWidthBtn(resetBtn), rightPadding)
			default:
				// Regular: Edit + Del.
				rightControls = container.NewHBox(editBtn, fixedWidthBtn(delBtn), rightPadding)
			}
			if enableCheck != nil {
				leftArrows.Add(enableCheck)
			}

			rowInner := container.NewBorder(nil, nil, leftArrows, rightControls, nameLabel)
			row = fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{})
			row.WireTooltipLabelHover(nameLabel)
			// Регистрируем КАЖДУЮ строку, а не только перетаскиваемую:
			// вычисление точки вставки просматривает полосы всех строк.
			dragGroup.Register(rowIdx, row)
			items = append(items, row)
		}
		listContent.Objects = items
		listContent.Refresh()
	}

	refreshList()

	addBtn := widget.NewButtonWithIcon(locale.T("Add"), theme.ContentAddIcon(), func() {
		model := editPresenter.Model()
		if model == nil {
			return
		}
		existingTags := collectAllTags(model)
		ShowEditDialog(parent, editPresenter, nil, true, -1, existingTags, func(updated *config.Direction, scopeKind string, sourceIndex int) {
			model := editPresenter.Model()
			if model == nil {
				return
			}
			// Направление всегда глобальное (SPEC 118 W5).
			model.GlobalOutbounds = append(model.GlobalOutbounds, *updated)
			model.BumpRevision()
			refreshList()
			if onApply != nil {
				onApply()
			}
		})
	})

	// SPEC 057-R-N: Restore missing template outbounds — recovery для случая,
	// когда юзер случайно удалил template-defined entries (auto-proxy-out,
	// vpn ①, vpn ② и т.п.). Walk template.parser_config.outbounds; для
	// каждого tag'а не в current state — append в конец. Required outbound'ы
	// (proxy-out) restore'нутся первыми если отсутствуют.
	// Note: direct-out не в template.parser_config — это sing-box built-in
	// (если нужен — добавится через Add).
	// ttwidget.Button нативно поддерживает SetToolTip (в отличие от
	// fynewidget.HoverForwardButton, который только wraps standard widget.Button).
	// Кнопка standalone (вне row) — hover forwarding не нужен.
	restoreBtn := ttwidget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		model := editPresenter.Model()
		if model == nil {
			return
		}
		existing := make(map[string]bool, len(model.GlobalOutbounds))
		for _, ob := range model.GlobalOutbounds {
			existing[ob.Tag] = true
		}
		tmplOutbounds := templateGlobalOutbounds(model)
		added := 0
		for _, tmplOb := range tmplOutbounds {
			if tmplOb.Tag == "" || existing[tmplOb.Tag] {
				continue
			}
			// SPEC 058-R-N: добавляем thin referenced entry (только tag + ref),
			// body live из template на render. Не копируем полный body.
			model.GlobalOutbounds = append(model.GlobalOutbounds, config.Direction{
				Tag: tmplOb.Tag,
				Ref: config.RefTemplate,
			})
			existing[tmplOb.Tag] = true
			added++
		}
		if added > 0 {
			model.BumpRevision()
			refreshList()
			if onApply != nil {
				onApply()
			}
		}
	})
	restoreBtn.Importance = widget.LowImportance
	restoreBtn.SetToolTip(locale.T("Restore template-defined outbounds that were deleted (e.g. auto-proxy-out, vpn ①, vpn ②). Existing entries unchanged."))

	// Без вложенного Scroll: список групп короткий, а собственный скролл,
	// растянувшись на всю высоту, вмещал все строки и молча съедал колесо
	// мыши (Fyne не пробрасывает wheel наружу из Scroll, которому некуда
	// скроллить) — над списком образовывалась мёртвая зона. Список живёт в
	// общем скролле вкладки Outbounds.
	rightTopButtons := container.NewHBox(restoreBtn, addBtn)
	top := container.NewBorder(nil, nil, nil, rightTopButtons, widget.NewLabel(locale.T("Directions:")))
	return container.NewBorder(
		top,
		nil,
		nil, nil,
		listContent,
	), refreshList
}
