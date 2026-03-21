// Package outbounds_configurator provides reusable UI for configuring outbounds in the wizard:
// list of all outbounds (global + per-source), Edit/Delete/Add, and helpers to apply changes back to ParserConfig.
package outbounds_configurator

import (
	"encoding/json"
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardutils "singbox-launcher/ui/wizard/utils"
)

// OutboundEditPresenter is used to register the Edit/Add window with the wizard overlay (single instance, focus redirect).
type OutboundEditPresenter interface {
	OpenOutboundEditWindow() fyne.Window
	SetOutboundEditWindow(fyne.Window)
	ClearOutboundEditWindow()
	UpdateChildOverlay()
	Model() *wizardmodels.WizardModel
}

// outboundRow identifies one outbound in the list (global or per-source).
type outboundRow struct {
	IsGlobal     bool
	SourceIndex  int
	IndexInSlice int
	Outbound     *config.OutboundConfig
	SourceLabel  string
}

// collectRows builds the flat list: local outbounds first (per source), then global.
// Order matters: lower items can reference upper items (e.g. in addOutbounds), not the other way around.
func collectRows(pc *config.ParserConfig) []outboundRow {
	var rows []outboundRow
	for si, proxy := range pc.ParserConfig.Proxies {
		label := proxy.Source
		if label == "" {
			label = locale.T("wizard.outbound.label_source") + strconv.Itoa(si+1)
		}
		label = wizardutils.TruncateStringEllipsis(label, wizardutils.MaxLabelRunes, "...")
		for i := range proxy.Outbounds {
			rows = append(rows, outboundRow{
				IsGlobal:     false,
				SourceIndex:  si,
				IndexInSlice: i,
				Outbound:     &pc.ParserConfig.Proxies[si].Outbounds[i],
				SourceLabel:  label,
			})
		}
	}
	for i := range pc.ParserConfig.Outbounds {
		rows = append(rows, outboundRow{IsGlobal: true, IndexInSlice: i, Outbound: &pc.ParserConfig.Outbounds[i], SourceLabel: "Global"})
	}
	return rows
}

// collectAllTags returns all outbound tags in display order (local first, then global).
func collectAllTags(pc *config.ParserConfig) []string {
	var tags []string
	for si := range pc.ParserConfig.Proxies {
		for i := range pc.ParserConfig.Proxies[si].Outbounds {
			tags = append(tags, pc.ParserConfig.Proxies[si].Outbounds[i].Tag)
		}
	}
	for i := range pc.ParserConfig.Outbounds {
		tags = append(tags, pc.ParserConfig.Outbounds[i].Tag)
	}
	return tags
}

// tagsAbove returns tags of rows that appear before rowIndex (only those can be used in addOutbounds).
func tagsAbove(rows []outboundRow, rowIndex int) []string {
	if rowIndex <= 0 {
		return nil
	}
	tags := make([]string, 0, rowIndex)
	for i := 0; i < rowIndex; i++ {
		tags = append(tags, rows[i].Outbound.Tag)
	}
	return tags
}

// getParserConfig returns the model's ParserConfig, ensuring it is set from ParserConfigJSON when nil.
func getParserConfig(model *wizardmodels.WizardModel) *config.ParserConfig {
	if model == nil {
		return nil
	}
	if model.ParserConfig != nil {
		return model.ParserConfig
	}
	raw := strings.TrimSpace(model.ParserConfigJSON)
	if raw == "" {
		return nil
	}
	var pc config.ParserConfig
	if err := json.Unmarshal([]byte(raw), &pc); err != nil {
		return nil
	}
	model.ParserConfig = &pc
	return model.ParserConfig
}

// sameScope returns true if both rows are in the same scope (same source or both global).
func sameScope(a, b outboundRow) bool {
	if a.IsGlobal && b.IsGlobal {
		return true
	}
	return !a.IsGlobal && !b.IsGlobal && a.SourceIndex == b.SourceIndex
}

// moveOutboundUp swaps the outbound with the previous one in the same scope.
func moveOutboundUp(parserConfig *config.ParserConfig, r outboundRow) {
	if r.IsGlobal {
		if r.IndexInSlice <= 0 {
			return
		}
		s := parserConfig.ParserConfig.Outbounds
		s[r.IndexInSlice], s[r.IndexInSlice-1] = s[r.IndexInSlice-1], s[r.IndexInSlice]
	} else {
		prox := &parserConfig.ParserConfig.Proxies[r.SourceIndex]
		if r.IndexInSlice <= 0 {
			return
		}
		prox.Outbounds[r.IndexInSlice], prox.Outbounds[r.IndexInSlice-1] = prox.Outbounds[r.IndexInSlice-1], prox.Outbounds[r.IndexInSlice]
	}
}

// moveOutboundDown swaps the outbound with the next one in the same scope.
func moveOutboundDown(parserConfig *config.ParserConfig, r outboundRow) {
	if r.IsGlobal {
		s := parserConfig.ParserConfig.Outbounds
		if r.IndexInSlice >= len(s)-1 {
			return
		}
		s[r.IndexInSlice], s[r.IndexInSlice+1] = s[r.IndexInSlice+1], s[r.IndexInSlice]
	} else {
		prox := &parserConfig.ParserConfig.Proxies[r.SourceIndex]
		if r.IndexInSlice >= len(prox.Outbounds)-1 {
			return
		}
		prox.Outbounds[r.IndexInSlice], prox.Outbounds[r.IndexInSlice+1] = prox.Outbounds[r.IndexInSlice+1], prox.Outbounds[r.IndexInSlice]
	}
}

// NewConfiguratorContent builds a reusable outbounds configurator content for embedding into tabs.
// ParserConfig is taken from the model (editPresenter.Model()) so the configurator always edits the current config.
// onApply is called after each mutation (Edit/Add/Delete/Up/Down) so the caller can serialize and sync.
// editPresenter is required (Model() is used to get ParserConfig); when set, the Edit/Add window is registered for overlay.
func NewConfiguratorContent(parent fyne.Window, editPresenter OutboundEditPresenter, onApply func()) fyne.CanvasObject {
	listContent := container.NewVBox()

	var refreshList func()
	refreshList = func() {
		parserConfig := getParserConfig(editPresenter.Model())
		if parserConfig == nil {
			return
		}
		rows := collectRows(parserConfig)
		items := make([]fyne.CanvasObject, 0, len(rows))
		for rowIdx, r := range rows {
			r := r
			rowIdx := rowIdx
			label := r.Outbound.Tag + " (" + r.Outbound.Type + ") — " + r.SourceLabel
			label = strings.ToValidUTF8(label, "")
			label = wizardutils.TruncateStringEllipsis(label, wizardutils.MaxLabelRunes, "...")
			canUp := rowIdx > 0 && sameScope(rows[rowIdx], rows[rowIdx-1])
			canDown := rowIdx < len(rows)-1 && sameScope(rows[rowIdx], rows[rowIdx+1])

			upBtn := widget.NewButton("↑", func() {
				parserConfig := getParserConfig(editPresenter.Model())
				if parserConfig == nil {
					return
				}
				rowsNow := collectRows(parserConfig)
				if rowIdx >= len(rowsNow) {
					return
				}
				idx := rowIdx
				if idx <= 0 || !sameScope(rowsNow[idx], rowsNow[idx-1]) {
					return
				}
				moveOutboundUp(parserConfig, rowsNow[idx])
				refreshList()
				if onApply != nil {
					onApply()
				}
			})
			if !canUp {
				upBtn.Disable()
			}

			downBtn := widget.NewButton("↓", func() {
				parserConfig := getParserConfig(editPresenter.Model())
				if parserConfig == nil {
					return
				}
				rowsNow := collectRows(parserConfig)
				if rowIdx >= len(rowsNow) {
					return
				}
				idx := rowIdx
				if idx < 0 || idx >= len(rowsNow)-1 || !sameScope(rowsNow[idx], rowsNow[idx+1]) {
					return
				}
				moveOutboundDown(parserConfig, rowsNow[idx])
				refreshList()
				if onApply != nil {
					onApply()
				}
			})
			if !canDown {
				downBtn.Disable()
			}

			editBtn := widget.NewButtonWithIcon(locale.T("wizard.shared.button_edit"), theme.DocumentCreateIcon(), func() {
				parserConfig := getParserConfig(editPresenter.Model())
				if parserConfig == nil {
					return
				}
				rowsNow := collectRows(parserConfig)
				if rowIdx >= len(rowsNow) {
					return
				}
				r2 := rowsNow[rowIdx]
				tagsForAdd := tagsAbove(rowsNow, rowIdx)
				wasGlobal := r2.IsGlobal
				wasSourceIndex := r2.SourceIndex
				ShowEditDialog(parent, editPresenter, r2.Outbound, r2.IsGlobal, r2.SourceIndex, tagsForAdd, func(updated *config.OutboundConfig, scopeKind string, sourceIndex int) {
					newGlobal := scopeKind == "global" || sourceIndex < 0
					scopeChanged := wasGlobal != newGlobal || (!newGlobal && wasSourceIndex != sourceIndex)
					if scopeChanged {
						// Remove from old scope
						if wasGlobal {
							parserConfig.ParserConfig.Outbounds = append(parserConfig.ParserConfig.Outbounds[:r2.IndexInSlice], parserConfig.ParserConfig.Outbounds[r2.IndexInSlice+1:]...)
						} else {
							prox := &parserConfig.ParserConfig.Proxies[wasSourceIndex]
							prox.Outbounds = append(prox.Outbounds[:r2.IndexInSlice], prox.Outbounds[r2.IndexInSlice+1:]...)
						}
						// Add to new scope
						if newGlobal {
							parserConfig.ParserConfig.Outbounds = append(parserConfig.ParserConfig.Outbounds, *updated)
						} else {
							for sourceIndex >= len(parserConfig.ParserConfig.Proxies) {
								parserConfig.ParserConfig.Proxies = append(parserConfig.ParserConfig.Proxies, config.ProxySource{})
							}
							parserConfig.ParserConfig.Proxies[sourceIndex].Outbounds = append(parserConfig.ParserConfig.Proxies[sourceIndex].Outbounds, *updated)
						}
					} else {
						*r2.Outbound = *updated
					}
					refreshList()
					if onApply != nil {
						onApply()
					}
				})
			})

			delBtn := widget.NewButtonWithIcon(locale.T("wizard.shared.button_del"), theme.DeleteIcon(), func() {
				parserConfig := getParserConfig(editPresenter.Model())
				if parserConfig == nil {
					return
				}
				rowsNow := collectRows(parserConfig)
				if rowIdx >= len(rowsNow) {
					return
				}
				r2 := rowsNow[rowIdx]
				if r2.IsGlobal {
					pc := parserConfig
					pc.ParserConfig.Outbounds = append(pc.ParserConfig.Outbounds[:r2.IndexInSlice], pc.ParserConfig.Outbounds[r2.IndexInSlice+1:]...)
				} else {
					prox := &parserConfig.ParserConfig.Proxies[r2.SourceIndex]
					prox.Outbounds = append(prox.Outbounds[:r2.IndexInSlice], prox.Outbounds[r2.IndexInSlice+1:]...)
				}
				refreshList()
				if onApply != nil {
					onApply()
				}
			})

			// Add fixed 30px transparent padding on the right inside the row,
			// so scrollbar has its own visual strip without increasing label width.
			rightPadding := canvas.NewRectangle(color.Transparent)
			rightPadding.SetMinSize(fyne.NewSize(10, 0))

			row := container.NewHBox(upBtn, downBtn, widget.NewLabel(label), layout.NewSpacer(), editBtn, delBtn, rightPadding)
			items = append(items, row)
		}
		listContent.Objects = items
		listContent.Refresh()
	}

	refreshList()

	addBtn := widget.NewButton(locale.T("wizard.outbound.button_add"), func() {
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

	scroll := container.NewScroll(listContent)
	scroll.SetMinSize(fyne.NewSize(0, 280))

	top := container.NewBorder(nil, nil, nil, addBtn, widget.NewLabel(locale.T("wizard.outbound.configurator_label")))
	return container.NewBorder(
		top,
		nil,
		nil, nil,
		scroll,
	)
}
