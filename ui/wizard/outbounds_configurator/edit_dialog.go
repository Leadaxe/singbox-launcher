// edit_dialog.go provides the Add/Edit outbound dialog for the configurator.
package outbounds_configurator

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	wizardbusiness "singbox-launcher/ui/wizard/business"
)

// ShowEditDialog opens a modal to add or edit an outbound. existing may be nil for add.
// onSave is called with the new config, scopeKind ("global" or "source") and sourceIndex (when scope is source).
func ShowEditDialog(
	parent fyne.Window,
	parserConfig *config.ParserConfig,
	existing *config.OutboundConfig,
	isGlobal bool,
	sourceIndex int,
	existingTags []string,
	onSave func(updated *config.OutboundConfig, scopeKind string, sourceIndex int),
) {
	isAdd := existing == nil

	tagEntry := widget.NewEntry()
	if existing != nil {
		tagEntry.SetText(existing.Tag)
	}
	tagEntry.SetPlaceHolder("e.g. proxy-out")

	typeSelect := widget.NewSelect([]string{"manual (selector)", "auto (urltest)"}, nil)
	if existing != nil {
		if existing.Type == "urltest" {
			typeSelect.SetSelected("auto (urltest)")
		} else {
			typeSelect.SetSelected("manual (selector)")
		}
	} else {
		typeSelect.SetSelected("manual (selector)")
	}

	commentEntry := widget.NewEntry()
	if existing != nil {
		commentEntry.SetText(existing.Comment)
	}
	commentEntry.SetPlaceHolder("Optional comment")

	// Scope: For all | For source: ...
	scopeOptions := []string{"For all"}
	for i, p := range parserConfig.ParserConfig.Proxies {
		label := p.Source
		if label == "" {
			label = "Source " + strconv.Itoa(i+1)
		}
		if len(label) > 35 {
			label = label[:32] + "..."
		}
		scopeOptions = append(scopeOptions, "For source: "+label)
	}
	scopeSelect := widget.NewSelect(scopeOptions, nil)
	if isAdd {
		scopeSelect.SetSelected("For all")
	} else if isGlobal {
		scopeSelect.SetSelected("For all")
	} else {
		if sourceIndex >= 0 && sourceIndex < len(parserConfig.ParserConfig.Proxies) {
			scopeSelect.SetSelected(scopeOptions[sourceIndex+1])
		} else {
			scopeSelect.SetSelected(scopeOptions[0])
		}
	}

	// Filters: one line "tag" = "!/(🇷🇺)/i" or "/regex/i"
	filterKeyEntry := widget.NewEntry()
	filterKeyEntry.SetText("tag")
	filterKeyEntry.SetPlaceHolder("key: tag, host, scheme, label")
	filterValEntry := widget.NewEntry()
	filterValEntry.SetPlaceHolder("e.g. /🇳🇱/i or !/(🇷🇺)/i")
	if existing != nil && existing.Filters != nil {
		for k, v := range existing.Filters {
			filterKeyEntry.SetText(k)
			if s, ok := v.(string); ok {
				filterValEntry.SetText(s)
			}
			break
		}
	}

	// Preferred default (one key-value)
	defKeyEntry := widget.NewEntry()
	defKeyEntry.SetText("tag")
	defValEntry := widget.NewEntry()
	defValEntry.SetPlaceHolder("e.g. /🇳🇱/i")
	if existing != nil && existing.PreferredDefault != nil {
		for k, v := range existing.PreferredDefault {
			defKeyEntry.SetText(k)
			if s, ok := v.(string); ok {
				defValEntry.SetText(s)
			}
			break
		}
	}

	// AddOutbounds: direct-out, reject checkboxes + checkboxes for other tags
	directCheck := widget.NewCheck("direct-out", nil)
	rejectCheck := widget.NewCheck("reject", nil)
	otherTagChecks := make([]*widget.Check, 0, len(existingTags))
	otherTagsMap := make(map[string]*widget.Check)
	for _, tag := range existingTags {
		c := widget.NewCheck(tag, nil)
		otherTagChecks = append(otherTagChecks, c)
		otherTagsMap[tag] = c
	}
	if existing != nil && len(existing.AddOutbounds) > 0 {
		for _, t := range existing.AddOutbounds {
			if t == "direct-out" {
				directCheck.SetChecked(true)
			} else if t == "reject" {
				rejectCheck.SetChecked(true)
			} else if c, ok := otherTagsMap[t]; ok {
				c.SetChecked(true)
			}
		}
	}

	save := func() {
		tag := strings.TrimSpace(tagEntry.Text)
		if tag == "" {
			dialog.ShowError(fmt.Errorf("tag is required"), parent)
			return
		}
		obType := "selector"
		if typeSelect.Selected == "auto (urltest)" {
			obType = "urltest"
		}
		scopeKind := "global"
		idx := -1
		if scopeSelect.Selected != "" && strings.HasPrefix(scopeSelect.Selected, "For source:") {
			scopeKind = "source"
			for i, opt := range scopeOptions {
				if i > 0 && opt == scopeSelect.Selected {
					idx = i - 1
					break
				}
			}
		}

		cfg := &config.OutboundConfig{
			Tag:     tag,
			Type:    obType,
			Comment: strings.TrimSpace(commentEntry.Text),
		}
		if existing != nil && existing.Options != nil {
			cfg.Options = make(map[string]interface{})
			for k, v := range existing.Options {
				cfg.Options[k] = v
			}
		} else if obType == "selector" {
			cfg.Options = map[string]interface{}{"interrupt_exist_connections": true}
		} else {
			cfg.Options = map[string]interface{}{
				"url": "https://cp.cloudflare.com/generate_204",
				"interval": "5m", "tolerance": 100,
				"interrupt_exist_connections": true,
			}
		}

		filterKey := strings.TrimSpace(filterKeyEntry.Text)
		filterVal := strings.TrimSpace(filterValEntry.Text)
		if filterKey != "" && filterVal != "" {
			cfg.Filters = map[string]interface{}{filterKey: filterVal}
		}
		defKey := strings.TrimSpace(defKeyEntry.Text)
		defVal := strings.TrimSpace(defValEntry.Text)
		if defKey != "" && defVal != "" {
			cfg.PreferredDefault = map[string]interface{}{defKey: defVal}
		}

		var addOb []string
		if directCheck.Checked {
			addOb = append(addOb, "direct-out")
		}
		if rejectCheck.Checked {
			addOb = append(addOb, "reject")
		}
		for _, c := range otherTagChecks {
			if c.Checked {
				addOb = append(addOb, c.Text)
			}
		}
		cfg.AddOutbounds = addOb

		// Preserve wizard if editing
		if existing != nil && existing.Wizard != nil {
			cfg.Wizard = wizardbusiness.CloneOutbound(existing).Wizard
		}
		onSave(cfg, scopeKind, idx)
	}

	otherTagsBox := container.NewVBox()
	for _, c := range otherTagChecks {
		otherTagsBox.Add(c)
	}
	scrollOther := container.NewScroll(otherTagsBox)
	scrollOther.SetMinSize(fyne.NewSize(0, 80))

	d := dialog.NewCustomConfirm("Edit Outbound", "Save", "Cancel", container.NewVBox(
		widget.NewLabel("Scope"),
		scopeSelect,
		widget.NewLabel("Tag"),
		tagEntry,
		widget.NewLabel("Type"),
		typeSelect,
		widget.NewLabel("Comment"),
		commentEntry,
		widget.NewLabel("Filters (key and value; use !/regex/i for negation)"),
		container.NewGridWithColumns(2, filterKeyEntry, filterValEntry),
		widget.NewLabel("Preferred default (filter for default node)"),
		container.NewGridWithColumns(2, defKeyEntry, defValEntry),
		widget.NewLabel("Add outbounds at start (direct-out, reject, others)"),
		container.NewHBox(directCheck, rejectCheck),
		scrollOther,
	), func(ok bool) {
		if ok {
			save()
		}
	}, parent)
	d.Resize(fyne.NewSize(420, 520))
	d.Show()
}
