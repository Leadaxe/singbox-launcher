// Package business — SPEC 077: detour (proxy-chain) option helpers for the
// Source edit dialog.
package business

import (
	"encoding/json"
	"strings"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// detourExcludedBuiltins are service/auto outbounds that make no sense as a
// proxy-chain hop and are never offered as detour targets (SPEC 077):
//   - direct-out / reject / drop — built-in service outbounds;
//   - auto-proxy-out — the default template's urltest auto-select group.
//
// Note: auto-proxy-out is the default template's tag; a custom template that
// renames its auto group would have that group leak in (acceptable — chaining
// through an urltest group still works, it just isn't filtered).
var detourExcludedBuiltins = map[string]struct{}{
	wizardmodels.DefaultOutboundTag: {}, // direct-out
	wizardmodels.RejectActionName:   {}, // reject
	"drop":                          {},
	"auto-proxy-out":                {},
}

// DetourOptions builds the dropdown options and the currently-selected value
// for a source's "Detour server" picker (SPEC 077).
//
// Offered targets are deliberately narrow — only stable, user-meaningful
// outbounds you'd intentionally chain through:
//   - manual global outbound selectors (the groups you build on the Outbounds
//     tab) and active preset groups;
//   - NOT the built-in/service outbounds (direct-out / reject / drop) nor the
//     template's auto-select group (auto-proxy-out) — see detourExcludedBuiltins;
//   - NOT a subscription's own local auto/select groups (those are service
//     groups over a whole subscription, not chain hops);
//   - NOT individual subscription nodes (their tags are runtime-generated);
//   - NOT single servers yet (their tags are runtime-only too — deferred).
//
// options[0] is always noneLabel (clears the detour). A dangling prior
// selection (target no longer offered) is appended so it stays visible/clearable.
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
	localSub := localSubscriptionGroupTags(model)

	options = []string{noneLabel}
	inOptions := map[string]struct{}{noneLabel: {}}
	for _, tag := range GetAvailableOutbounds(model) {
		if _, isBuiltin := detourExcludedBuiltins[tag]; isBuiltin {
			continue // service/auto outbound — never a chain hop
		}
		if _, isOwn := own[tag]; isOwn {
			continue
		}
		if _, isLocalSub := localSub[tag]; isLocalSub {
			continue // a subscription's own group is not a chain target
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

// DetourChoice describes what one picker option means (SPEC 077 + SPEC 101).
// Exactly one of Tag / NodeHash is set: Tag targets a group outbound (stamped
// as-is), NodeHash targets one concrete node and is resolved to its final tag
// at generation time.
type DetourChoice struct {
	Tag       string
	NodeHash  string
	NodeLabel string
}

// detourNodeMarker prefixes single-node options so they read differently from
// group tags in the same dropdown.
const detourNodeMarker = "» "

// DetourOptionsWithNodes builds the Source dialog's "Detour server" options:
// the SPEC 077 group targets (see DetourOptions) plus, per SPEC 101, every
// single-server source (state Connections URI) other than this source itself.
// Subscription nodes are deliberately not offered — subscriptions carry
// hundreds of unstable nodes; chaining through one of them is what a group is
// for. options[0] is always noneLabel; choices maps a displayed option to its
// meaning (absent for noneLabel).
//
// A dangling prior node selection (the server was deleted or its URI edited →
// hash no longer parseable from any candidate) stays visible via its stored
// label so the user can see and clear it, mirroring the dangling-tag behavior.
func DetourOptionsWithNodes(model *wizardmodels.WizardModel, source *configtypes.ProxySource, noneLabel string) (options []string, selected string, choices map[string]DetourChoice) {
	options, selected = DetourOptions(model, source, noneLabel)
	choices = make(map[string]DetourChoice, len(options))
	for _, tag := range options {
		if tag != noneLabel {
			choices[tag] = DetourChoice{Tag: tag}
		}
	}

	ownURIs := map[string]struct{}{}
	if source != nil {
		for _, uri := range source.Connections {
			ownURIs[strings.TrimSpace(uri)] = struct{}{}
		}
	}

	selectedHash := ""
	if source != nil {
		selectedHash = strings.TrimSpace(source.DetourNodeHash)
	}
	selectedDisplay := ""
	if model != nil {
		for _, s := range model.Sources {
			if s.Type != wizardmodels.SourceTypeServer || strings.TrimSpace(s.URI) == "" {
				continue
			}
			uri := strings.TrimSpace(s.URI)
			if _, own := ownURIs[uri]; own {
				continue
			}
			hash := nodeHashForURI(uri)
			if hash == "" {
				continue // unparseable / unhashable — cannot be addressed
			}
			label := s.Label
			if label == "" {
				label = uri
			}
			display := detourNodeMarker + label
			if _, dup := choices[display]; dup {
				display += " (2)"
			}
			options = append(options, display)
			choices[display] = DetourChoice{NodeHash: hash, NodeLabel: label}
			if selectedHash != "" && hash == selectedHash {
				selectedDisplay = display
			}
		}
	}

	if selectedHash != "" {
		if selectedDisplay == "" {
			// dangling: keep the stored label visible and clearable
			label := source.DetourNodeLabel
			if label == "" {
				label = selectedHash[:min(12, len(selectedHash))] + "…"
			}
			selectedDisplay = detourNodeMarker + label
			if _, dup := choices[selectedDisplay]; !dup {
				options = append(options, selectedDisplay)
				choices[selectedDisplay] = DetourChoice{NodeHash: selectedHash, NodeLabel: label}
			}
		}
		selected = selectedDisplay
	}
	return options, selected, choices
}

// nodeHashForURI parses a single share-URI and returns its identity hash
// (config.NodeIdentityHash), "" when the URI does not parse or emit.
func nodeHashForURI(uri string) string {
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		return ""
	}
	return config.NodeIdentityHash(node)
}

// localSubscriptionGroupTags collects every local group tag declared by a
// proxy source (proxySource.Outbounds / addOutbounds). These are the
// per-subscription auto/select groups that GetAvailableOutbounds also returns
// for the Rules picker, but which must NOT be offered as detour chain targets.
func localSubscriptionGroupTags(model *wizardmodels.WizardModel) map[string]struct{} {
	res := map[string]struct{}{}
	if model == nil {
		return res
	}
	var parserCfg *config.ParserConfig
	if model.ParserConfig != nil {
		parserCfg = model.ParserConfig
	} else if jsonStr := strings.TrimSpace(model.ParserConfigJSON); jsonStr != "" {
		var parsed config.ParserConfig
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
			parserCfg = &parsed
		}
	}
	if parserCfg == nil {
		return res
	}
	for _, proxySource := range parserCfg.ParserConfig.Proxies {
		for _, ob := range proxySource.Outbounds {
			if ob.Tag != "" {
				res[ob.Tag] = struct{}{}
			}
			for _, extra := range ob.AddOutbounds {
				if extra != "" {
					res[extra] = struct{}{}
				}
			}
		}
	}
	return res
}
