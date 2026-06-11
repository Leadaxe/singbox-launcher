// Package business — SPEC 077: detour (proxy-chain) option helpers for the
// Source edit dialog.
package business

import (
	"singbox-launcher/core/config/configtypes"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// DetourOptions builds the dropdown options and the currently-selected value
// for a source's "Detour server" picker (SPEC 077).
//
//   - options[0] is always noneLabel (clears the detour);
//   - the rest are GetAvailableOutbounds(model) minus the source's OWN local
//     group tags (you can't chain a source through its own group — that is the
//     obvious self/cycle case the UI prevents up front);
//   - if the source already points at a tag that is no longer offered (a
//     dangling selection, e.g. the target group was removed), it is appended so
//     the user still sees and can clear it rather than it vanishing silently.
//
// selected is noneLabel when DetourTag is empty, else the DetourTag value.
func DetourOptions(model *wizardmodels.WizardModel, source *configtypes.ProxySource, noneLabel string) (options []string, selected string) {
	own := map[string]struct{}{}
	if source != nil {
		for _, ob := range source.Outbounds {
			if ob.Tag != "" {
				own[ob.Tag] = struct{}{}
			}
			for _, extra := range ob.AddOutbounds {
				if extra != "" {
					own[extra] = struct{}{}
				}
			}
		}
	}

	options = []string{noneLabel}
	inOptions := map[string]struct{}{noneLabel: {}}
	for _, tag := range GetAvailableOutbounds(model) {
		if _, isOwn := own[tag]; isOwn {
			continue
		}
		options = append(options, tag)
		inOptions[tag] = struct{}{}
	}

	selected = noneLabel
	if source != nil && source.DetourTag != "" {
		selected = source.DetourTag
		if _, ok := inOptions[selected]; !ok {
			options = append(options, selected) // dangling — keep visible/clearable
		}
	}
	return options, selected
}
