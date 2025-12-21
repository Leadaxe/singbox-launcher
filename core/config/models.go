package config

import (
	"net/url"
	"time"
)

// ParserConfigVersion is the current version of ParserConfig format
const ParserConfigVersion = 4

// SubscriptionUserAgent is the User-Agent string used for fetching subscriptions
// Using neutral User-Agent to avoid server detecting sing-box and returning JSON config
const SubscriptionUserAgent = "SubscriptionParserClient"

// MaxNodesPerSubscription limits the maximum number of nodes parsed from a single subscription
// This prevents memory issues with very large subscriptions
const MaxNodesPerSubscription = 500

// ParserConfig represents the configuration structure from @ParserConfig block
// Clean structure for version 4 (legacy versions are migrated automatically)
type ParserConfig struct {
	ParserConfig struct {
		Version   int              `json:"version,omitempty"`
		Proxies   []ProxySource    `json:"proxies"`
		Outbounds []OutboundConfig `json:"outbounds"`
		Parser    struct {
			Reload      string `json:"reload,omitempty"`       // Интервал автоматического обновления
			LastUpdated string `json:"last_updated,omitempty"` // Время последнего обновления (RFC3339, UTC)
		} `json:"parser,omitempty"`
	} `json:"ParserConfig"`
}

// ProxySource represents a proxy subscription source
type ProxySource struct {
	Source      string              `json:"source,omitempty"`
	Connections []string            `json:"connections,omitempty"`
	Skip        []map[string]string `json:"skip,omitempty"`
	Outbounds   []OutboundConfig    `json:"outbounds,omitempty"`   // Local outbounds for this source (version 4)
	TagPrefix   string              `json:"tag_prefix,omitempty"`  // Prefix to add to all node tags from this source
	TagPostfix  string              `json:"tag_postfix,omitempty"` // Postfix to add to all node tags from this source
	TagMask     string              `json:"tag_mask,omitempty"`    // Mask to replace entire tag (ignores tag_prefix and tag_postfix if set)
}

// OutboundConfig represents an outbound selector configuration (version 3)
// Clean structure without legacy fields - used in main codebase
// WizardConfig represents the wizard configuration for outbounds
// Supports both old format ("wizard":"hide") and new format ("wizard":{"hide":true, "required":2})
type WizardConfig struct {
	Hide     bool `json:"hide,omitempty"`     // Hide outbound from wizard second tab
	Required int  `json:"required,omitempty"` // Optional: 0 or missing=ignore, 1=check presence only, >1=strict match from template
}

type OutboundConfig struct {
	Tag              string                 `json:"tag"`
	Type             string                 `json:"type"`
	Options          map[string]interface{} `json:"options,omitempty"`
	Filters          map[string]interface{} `json:"filters,omitempty"`
	AddOutbounds     []string               `json:"addOutbounds,omitempty"`
	PreferredDefault map[string]interface{} `json:"preferredDefault,omitempty"`
	Comment          string                 `json:"comment,omitempty"`
	Wizard           interface{}            `json:"wizard,omitempty"` // Supports both "hide" (string) and {"hide":true, "required":2} (object) for backward compatibility
}

// IsWizardHidden checks if outbound should be hidden from wizard
// Supports both old format ("wizard":"hide") and new format ("wizard":{"hide":true})
func (oc *OutboundConfig) IsWizardHidden() bool {
	if oc.Wizard == nil {
		return false
	}

	// Old format: "wizard":"hide"
	if wizardStr, ok := oc.Wizard.(string); ok {
		return wizardStr == "hide"
	}

	// New format: "wizard":{"hide":true, ...}
	if wizardMap, ok := oc.Wizard.(map[string]interface{}); ok {
		if hideVal, ok := wizardMap["hide"]; ok {
			if hideBool, ok := hideVal.(bool); ok {
				return hideBool
			}
		}
	}

	return false
}

// GetWizardRequired returns the required value from wizard config
// Only checks wizard.required from new format ("wizard": {"hide": true, "required": 2})
func (oc *OutboundConfig) GetWizardRequired() int {
	if oc.Wizard != nil {
		if wizardMap, ok := oc.Wizard.(map[string]interface{}); ok {
			if requiredVal, ok := wizardMap["required"]; ok {
				if requiredInt, ok := requiredVal.(float64); ok {
					return int(requiredInt)
				}
			}
		}
	}

	// No required field found - return 0 (ignore)
	return 0
}

// ParsedNode represents a parsed proxy node with all extracted information.
// It contains protocol-specific fields (UUID, Flow, etc.) and the generated
// outbound configuration ready for JSON serialization.
type ParsedNode struct {
	Tag      string
	Scheme   string
	Server   string
	Port     int
	UUID     string
	Flow     string
	Label    string
	Comment  string
	Query    url.Values
	Outbound map[string]interface{}
}

// NormalizeParserConfig normalizes ParserConfig structure:
// - Ensures version is set to ParserConfigVersion
// - Sets default reload to "4h" if not specified
// - Optionally updates last_updated timestamp (if updateLastUpdated is true)
// Note: Migration is handled in ExtractParserConfig
// This function works with already-migrated clean ParserConfig
func NormalizeParserConfig(parserConfig *ParserConfig, updateLastUpdated bool) {
	if parserConfig == nil {
		return
	}

	// Ensure version is set to current version (always update to latest)
	parserConfig.ParserConfig.Version = ParserConfigVersion

	// Ensure parser object exists (create if missing)
	// Set default reload to "4h" if not specified
	if parserConfig.ParserConfig.Parser.Reload == "" {
		parserConfig.ParserConfig.Parser.Reload = "4h"
	}

	// Optionally update last_updated timestamp
	if updateLastUpdated {
		parserConfig.ParserConfig.Parser.LastUpdated = time.Now().UTC().Format(time.RFC3339)
	}
}
