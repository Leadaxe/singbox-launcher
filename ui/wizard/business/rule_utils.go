// Package business contains business logic for wizard configuration.
//
// File rule_utils.go contains shared utilities for rule processing:
//   - ApplyOutboundToRule - applies outbound selection to a rule (handles reject/drop/regular outbound)
//   - FormatRuleAsJSON - formats a rule as JSON string with proper indentation
//   - CloneRuleRaw - creates a deep copy of rule's raw data
//
// These functions eliminate duplication across route_text_merger.go, selectable_rule_processor.go, and generator.go
package business

import (
	"encoding/json"

	wizardmodels "singbox-launcher/ui/wizard/models"
)

// ApplyOutboundToRule applies outbound selection to a cloned rule.
// Handles three cases:
//   - "reject" → sets action: reject (without method)
//   - "drop" → sets action: reject with method: drop
//   - regular outbound → sets outbound field, removes action/method
//
// Returns a new map with applied changes, does not modify the input.
func ApplyOutboundToRule(ruleRaw map[string]interface{}, outbound string) map[string]interface{} {
	cloned := CloneRuleRaw(ruleRaw)

	switch outbound {
	case wizardmodels.RejectActionName:
		// User selected reject - set action: reject without method, remove outbound
		delete(cloned, "outbound")
		cloned["action"] = wizardmodels.RejectActionName
		delete(cloned, "method")

	case "drop":
		// User selected drop - set action: reject with method: drop, remove outbound
		delete(cloned, "outbound")
		cloned["action"] = wizardmodels.RejectActionName
		cloned["method"] = wizardmodels.RejectActionMethod

	default:
		if outbound != "" {
			// User selected regular outbound - set outbound, remove action and method
			cloned["outbound"] = outbound
			delete(cloned, "action")
			delete(cloned, "method")
		}
	}

	return cloned
}

// FormatRuleAsJSON formats a rule as JSON string with 2-space indentation.
// Applies outbound selection before formatting.
func FormatRuleAsJSON(ruleRaw map[string]interface{}, outbound string) (string, error) {
	applied := ApplyOutboundToRule(ruleRaw, outbound)
	jsonBytes, err := json.MarshalIndent(applied, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// CloneRuleRaw creates a deep copy of rule's raw data.
// Only does shallow copy of map - assumes values are JSON-compatible primitives.
func CloneRuleRaw(ruleRaw map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(ruleRaw))
	for key, value := range ruleRaw {
		cloned[key] = value
	}
	return cloned
}
