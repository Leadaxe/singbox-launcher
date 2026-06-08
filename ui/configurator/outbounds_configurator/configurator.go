// Package outbounds_configurator provides reusable UI for configuring outbounds in the wizard:
// list of all outbounds (global + per-source), Edit/Delete/Add, and helpers to apply changes back to ParserConfig.
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
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardutils "singbox-launcher/ui/configurator/utils"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
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
// ParserConfig is taken from the model (editPresenter.Model()) so the configurator always edits the current config.
// onApply is called after each mutation (Edit/Add/Delete/Up/Down) so the caller can serialize and sync.
// editPresenter is required (Model() is used to get ParserConfig); when set, the Edit/Add window is registered for overlay.
// The returned refresh function rebuilds the list from the current model (call after ParserConfig changes outside the list, e.g. Sources → Edit).
func NewConfiguratorContent(parent fyne.Window, editPresenter OutboundEditPresenter, onApply func()) (fyne.CanvasObject, func()) {
	listContent := container.NewVBox()

	var refreshList func()
	refreshList = func() {
		model := editPresenter.Model()
		if getParserConfig(model) == nil {
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
		items := make([]fyne.CanvasObject, 0, len(rows))
		for rowIdx, r := range rows {
			r := r
			rowIdx := rowIdx
			var row *fynewidget.HoverRow
			rowGetter := func() *fynewidget.HoverRow { return row }

			rawLine := r.Outbound.Tag + " (" + r.Outbound.Type + ")"
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

			// SPEC 057-R-N: preset rows — natural slice members с ref. Up/Down
			// для всех rows работает через direct swap pc.ParserConfig.Outbounds[]
			// (moveOutboundUp/Down) — preset binding (ref + updates) переезжает
			// вместе с body, потому что мы двигаем целиком элемент.
			canUp := rowIdx > 0 && sameScope(rows[rowIdx], rows[rowIdx-1])
			canDown := rowIdx < len(rows)-1 && sameScope(rows[rowIdx], rows[rowIdx+1])

			upBtn := fynewidget.NewHoverForwardButton("↑", func() {
				pc := getParserConfig(editPresenter.Model())
				if pc == nil {
					return
				}
				moveOutboundUp(pc, r)
				refreshList()
				if onApply != nil {
					onApply()
				}
			}, rowGetter)
			if !canUp {
				upBtn.Disable()
				fynewidget.SetToolTipSafe(upBtn, locale.T("wizard.outbound.reorder_up_off"))
			} else {
				fynewidget.SetToolTipSafe(upBtn, locale.T("wizard.outbound.reorder_up"))
			}

			downBtn := fynewidget.NewHoverForwardButton("↓", func() {
				pc := getParserConfig(editPresenter.Model())
				if pc == nil {
					return
				}
				moveOutboundDown(pc, r)
				refreshList()
				if onApply != nil {
					onApply()
				}
			}, rowGetter)
			if !canDown {
				downBtn.Disable()
				fynewidget.SetToolTipSafe(downBtn, locale.T("wizard.outbound.reorder_down_off"))
			} else {
				fynewidget.SetToolTipSafe(downBtn, locale.T("wizard.outbound.reorder_down"))
			}

			// Edit button — доступен для всех rows включая preset/required.
			// Для preset: scope locked, Ref/Updates preserved (sync-managed
			// metadata, не должны wipe'нуться юзерским body edit).
			editBtn := fynewidget.NewHoverForwardButtonWithIcon(locale.T("wizard.shared.button_edit"), theme.DocumentCreateIcon(), func() {
				rowsNow := collectRowsForUI(editPresenter.Model())
				if rowIdx >= len(rowsNow) {
					return
				}
				r2 := rowsNow[rowIdx]
				tagsForAdd := tagsAbove(rowsNow, rowIdx)
				wasGlobal := r2.IsGlobal
				wasSourceIndex := r2.SourceIndex
				parserConfig := getParserConfig(editPresenter.Model())
				ShowEditDialog(parent, editPresenter, r2.Outbound, r2.IsGlobal, r2.SourceIndex, tagsForAdd, func(updated *config.OutboundConfig, scopeKind string, sourceIndex int) {
					newGlobal := scopeKind == "global" || sourceIndex < 0
					scopeChanged := wasGlobal != newGlobal || (!newGlobal && wasSourceIndex != sourceIndex)
					// Preset entries: scope locked (preset должен оставаться
					// global, иначе Sync создаст дубль при следующем вызове —
					// per-source entry останется, plus новый global появится).
					if r2.IsPreset {
						scopeChanged = false
					}
					if scopeChanged {
						if wasGlobal {
							parserConfig.ParserConfig.Outbounds = append(parserConfig.ParserConfig.Outbounds[:r2.IndexInSlice], parserConfig.ParserConfig.Outbounds[r2.IndexInSlice+1:]...)
						} else {
							prox := &parserConfig.ParserConfig.Proxies[wasSourceIndex]
							prox.Outbounds = append(prox.Outbounds[:r2.IndexInSlice], prox.Outbounds[r2.IndexInSlice+1:]...)
						}
						if newGlobal {
							parserConfig.ParserConfig.Outbounds = append(parserConfig.ParserConfig.Outbounds, *updated)
						} else {
							for sourceIndex >= len(parserConfig.ParserConfig.Proxies) {
								parserConfig.ParserConfig.Proxies = append(parserConfig.ParserConfig.Proxies, config.ProxySource{})
							}
							parserConfig.ParserConfig.Proxies[sourceIndex].Outbounds = append(parserConfig.ParserConfig.Proxies[sourceIndex].Outbounds, *updated)
						}
					} else {
						// In-place body update. SPEC 058-R-N: для referenced
						// entries Edit dialog (Phase 4 applyEditedConfig) уже
						// вычислил USER patch и put его в updated.Updates +
						// strip'нул body. Для direct entries — updated имеет
						// full body inline + сохранённые Updates. В обоих
						// случаях просто перезаписываем r2.Outbound = *updated.
						*r2.Outbound = *updated
					}
					refreshList()
					if onApply != nil {
						onApply()
					}
				})
			}, rowGetter)

			delBtn := fynewidget.NewHoverForwardButtonWithIcon(locale.T("wizard.shared.button_del"), theme.DeleteIcon(), func() {
				rowsNow := collectRowsForUI(editPresenter.Model())
				if rowIdx >= len(rowsNow) || rowsNow[rowIdx].IsPreset || rowsNow[rowIdx].IsRequired {
					return
				}
				r2 := rowsNow[rowIdx]
				pc := getParserConfig(editPresenter.Model())
				if r2.IsGlobal {
					pc.ParserConfig.Outbounds = append(pc.ParserConfig.Outbounds[:r2.IndexInSlice], pc.ParserConfig.Outbounds[r2.IndexInSlice+1:]...)
				} else {
					prox := &pc.ParserConfig.Proxies[r2.SourceIndex]
					prox.Outbounds = append(prox.Outbounds[:r2.IndexInSlice], prox.Outbounds[r2.IndexInSlice+1:]...)
				}
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
				resetBtn = fynewidget.NewHoverForwardButtonWithIcon("Reset", theme.ViewRefreshIcon(), func() {
					rowsNow := collectRowsForUI(editPresenter.Model())
					if rowIdx >= len(rowsNow) {
						return
					}
					r2 := rowsNow[rowIdx]
					if !(r2.IsTemplate || r2.IsPreset) || !r2.IsGlobal || r2.IndexInSlice < 0 {
						return
					}
					pc := getParserConfig(editPresenter.Model())
					if pc == nil || r2.IndexInSlice >= len(pc.ParserConfig.Outbounds) {
						return
					}
					// Strip USER patch из Updates[]; preset patches preserve.
					pc.ParserConfig.Outbounds[r2.IndexInSlice].Updates = build.UpsertUserPatch(
						pc.ParserConfig.Outbounds[r2.IndexInSlice].Updates,
						nil,
					)
					refreshList()
					if onApply != nil {
						onApply()
					}
				}, rowGetter)
				resetBtn.Importance = widget.LowImportance
				fynewidget.SetToolTipSafe(resetBtn, "Reset — clear your changes, revert to defaults")
				if !r.HasUserPatch {
					resetBtn.Disable()
				}
			}

			leftArrows = container.NewHBox(upBtn, downBtn)
			// fixedWidthBtn — обёртка, фиксирующая минимальную ширину кнопки
			// (Reset > Del по тексту; без фиксации колонка действий "прыгает"
			// между rows). Stack комбинирует MinSize: max(sizer, btn).
			fixedWidthBtn := func(btn fyne.CanvasObject) fyne.CanvasObject {
				sizer := canvas.NewRectangle(color.Transparent)
				sizer.SetMinSize(fyne.NewSize(78, 0))
				return container.NewStack(sizer, btn)
			}
			if r.IsPreset || r.IsRequired {
				// Locked rows: Edit + Reset, без Del.
				rightControls = container.NewHBox(editBtn, fixedWidthBtn(resetBtn), rightPadding)
			} else {
				// Regular: Edit + Del.
				rightControls = container.NewHBox(editBtn, fixedWidthBtn(delBtn), rightPadding)
			}

			rowInner := container.NewBorder(nil, nil, leftArrows, rightControls, nameLabel)
			row = fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{})
			row.WireTooltipLabelHover(nameLabel)
			items = append(items, row)
		}
		listContent.Objects = items
		listContent.Refresh()
	}

	refreshList()

	addBtn := widget.NewButtonWithIcon(locale.T("wizard.outbound.button_add"), theme.ContentAddIcon(), func() {
		parserConfig := getParserConfig(editPresenter.Model())
		if parserConfig == nil {
			return
		}
		existingTags := collectAllTags(parserConfig)
		ShowEditDialog(parent, editPresenter, nil, true, -1, existingTags, func(updated *config.OutboundConfig, scopeKind string, sourceIndex int) {
			if scopeKind == "global" || sourceIndex < 0 {
				parserConfig.ParserConfig.Outbounds = append(parserConfig.ParserConfig.Outbounds, *updated)
			} else {
				for sourceIndex >= len(parserConfig.ParserConfig.Proxies) {
					parserConfig.ParserConfig.Proxies = append(parserConfig.ParserConfig.Proxies, config.ProxySource{})
				}
				parserConfig.ParserConfig.Proxies[sourceIndex].Outbounds = append(parserConfig.ParserConfig.Proxies[sourceIndex].Outbounds, *updated)
			}
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
		pc := getParserConfig(model)
		if pc == nil {
			return
		}
		existing := make(map[string]bool, len(pc.ParserConfig.Outbounds))
		for _, ob := range pc.ParserConfig.Outbounds {
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
			pc.ParserConfig.Outbounds = append(pc.ParserConfig.Outbounds, config.OutboundConfig{
				Tag: tmplOb.Tag,
				Ref: config.RefTemplate,
			})
			existing[tmplOb.Tag] = true
			added++
		}
		if added > 0 {
			refreshList()
			if onApply != nil {
				onApply()
			}
		}
	})
	restoreBtn.Importance = widget.LowImportance
	restoreBtn.SetToolTip("Restore template-defined outbounds that were deleted (e.g. auto-proxy-out, vpn ①, vpn ②). Existing entries unchanged.")

	scroll := container.NewScroll(listContent)
	scroll.SetMinSize(fyne.NewSize(0, 280))

	rightTopButtons := container.NewHBox(restoreBtn, addBtn)
	top := container.NewBorder(nil, nil, nil, rightTopButtons, widget.NewLabel(locale.T("wizard.outbound.configurator_label")))
	return container.NewBorder(
		top,
		nil,
		nil, nil,
		scroll,
	), refreshList
}
