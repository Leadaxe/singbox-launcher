package utils

import (
	"encoding/json"

	"singbox-launcher/core/config"
)

// OutboundsMatchStrict compares two outbound configurations strictly.
func OutboundsMatchStrict(existing, template *config.OutboundConfig) bool {
	// Compare main fields
	if existing.Tag != template.Tag ||
		existing.Type != template.Type ||
		existing.Comment != template.Comment {
		return false
	}

	// Compare Wizard (support both formats)
	existingHide := existing.IsWizardHidden()
	templateHide := template.IsWizardHidden()
	if existingHide != templateHide {
		return false
	}

	// Compare AddOutbounds
	if !StringSlicesEqual(existing.AddOutbounds, template.AddOutbounds) {
		return false
	}

	// Compare Options (deep comparison)
	if !MapsEqual(existing.Options, template.Options) {
		return false
	}

	// Compare Filters (deep comparison)
	if !MapsEqual(existing.Filters, template.Filters) {
		return false
	}

	// Compare PreferredDefault (deep comparison)
	if !MapsEqual(existing.PreferredDefault, template.PreferredDefault) {
		return false
	}

	return true
}

// StringSlicesEqual checks if two string slices are equal.
func StringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// MapsEqual checks deep equality of two map[string]interface{}.
func MapsEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		if !ValuesEqual(av, bv) {
			return false
		}
	}
	return true
}

// ValuesEqual checks equality of two values (recursively for map and slice).
func ValuesEqual(a, b interface{}) bool {
	// Compare via JSON for reliability
	aJSON, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bJSON, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(aJSON) == string(bJSON)
}
