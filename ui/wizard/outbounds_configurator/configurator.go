// Package outbounds_configurator provides the Config Outbounds window for the wizard:
// list of all outbounds (global + per-source), Edit/Delete/Add, and apply back to ParserConfig.
package outbounds_configurator

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
	wizardbusiness "singbox-launcher/ui/wizard/business"
)

// outboundRow identifies one outbound in the list (global or per-source).
type outboundRow struct {
	IsGlobal     bool
	SourceIndex  int
	IndexInSlice int
	Outbound     *config.OutboundConfig
	SourceLabel  string
}

// collectRows builds the flat list of outbound rows from ParserConfig.
func collectRows(pc *config.ParserConfig) []outboundRow {
	var rows []outboundRow
	for i := range pc.ParserConfig.Outbounds {
		rows = append(rows, outboundRow{IsGlobal: true, IndexInSlice: i, Outbound: &pc.ParserConfig.Outbounds[i], SourceLabel: "Global"})
	}
	for si, proxy := range pc.ParserConfig.Proxies {
		label := proxy.Source
		if label == "" {
			label = "Source " + strconv.Itoa(si+1)
		}
		if len(label) > 40 {
			label = label[:37] + "..."
		}
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
	return rows
}

// collectAllTags returns all outbound tags in order (for AddOutbounds selection).
func collectAllTags(pc *config.ParserConfig) []string {
	var tags []string
	for i := range pc.ParserConfig.Outbounds {
		tags = append(tags, pc.ParserConfig.Outbounds[i].Tag)
	}
	for si := range pc.ParserConfig.Proxies {
		for i := range pc.ParserConfig.Proxies[si].Outbounds {
			tags = append(tags, pc.ParserConfig.Proxies[si].Outbounds[i].Tag)
		}
	}
	return tags
}

// Show opens the Config Outbounds window. parserConfig is modified in place; onApply is called with it when the user closes the window.
func Show(_ fyne.Window, parserConfig *config.ParserConfig, onApply func(*config.ParserConfig)) {
	debuglog.DebugLog("outbounds_configurator: open")
	w := fyne.CurrentApp().NewWindow("Config Outbounds")
	w.Resize(fyne.NewSize(560, 420))

	var listContent *fyne.Container
	var refreshList func()

	refreshList = func() {
		rows := collectRows(parserConfig)
		items := make([]fyne.CanvasObject, 0, len(rows)+1)
		for _, r := range rows {
			r := r
			label := r.Outbound.Tag + " (" + r.Outbound.Type + ") — " + r.SourceLabel
			editBtn := widget.NewButton("Edit", func() {
				existingTags := collectAllTags(parserConfig)
				ShowEditDialog(w, parserConfig, r.Outbound, r.IsGlobal, r.SourceIndex, existingTags, func(updated *config.OutboundConfig, scopeKind string, sourceIndex int) {
					*r.Outbound = *updated
					refreshList()
				})
			})
			delBtn := widget.NewButton("Delete", func() {
				if r.IsGlobal {
					pc := parserConfig
					pc.ParserConfig.Outbounds = append(pc.ParserConfig.Outbounds[:r.IndexInSlice], pc.ParserConfig.Outbounds[r.IndexInSlice+1:]...)
				} else {
					prox := &parserConfig.ParserConfig.Proxies[r.SourceIndex]
					prox.Outbounds = append(prox.Outbounds[:r.IndexInSlice], prox.Outbounds[r.IndexInSlice+1:]...)
				}
				refreshList()
			})
			row := container.NewHBox(widget.NewLabel(label), layout.NewSpacer(), editBtn, delBtn)
			items = append(items, row)
		}
		listContent.Objects = items
		listContent.Refresh()
	}

	listContent = container.NewVBox()
	refreshList()

	addBtn := widget.NewButton("Add", func() {
		existingTags := collectAllTags(parserConfig)
		ShowEditDialog(w, parserConfig, nil, true, -1, existingTags, func(updated *config.OutboundConfig, scopeKind string, sourceIndex int) {
			if scopeKind == "global" || sourceIndex < 0 {
				parserConfig.ParserConfig.Outbounds = append(parserConfig.ParserConfig.Outbounds, *updated)
			} else {
				for sourceIndex >= len(parserConfig.ParserConfig.Proxies) {
					parserConfig.ParserConfig.Proxies = append(parserConfig.ParserConfig.Proxies, config.ProxySource{})
				}
				parserConfig.ParserConfig.Proxies[sourceIndex].Outbounds = append(parserConfig.ParserConfig.Proxies[sourceIndex].Outbounds, *updated)
			}
			refreshList()
		})
	})

	closeBtn := widget.NewButton("Close", func() {
		serialized, err := wizardbusiness.SerializeParserConfig(parserConfig)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		debuglog.DebugLog("outbounds_configurator: apply and close, len=%d", len(serialized))
		w.Close()
		onApply(parserConfig)
	})

	scroll := container.NewScroll(listContent)
	scroll.SetMinSize(fyne.NewSize(0, 280))
	top := container.NewBorder(nil, nil, nil, addBtn, widget.NewLabel("Outbounds (global first, then per source):"))
	content := container.NewBorder(
		top,
		container.NewHBox(closeBtn),
		nil, nil,
		scroll,
	)
	w.SetContent(content)
	w.CenterOnScreen()
	w.Show()
}
