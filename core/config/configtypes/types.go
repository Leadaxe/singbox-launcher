// Package configtypes contains shared data types for configuration parsing.
// Extracted to its own package to break the circular dependency between
// core/config and core/config/subscription: both packages import configtypes
// for shared types, while core/config can now safely import subscription.
package configtypes

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
const MaxNodesPerSubscription = 3000

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
	// ExcludeFromGlobal: when true, nodes from this source are omitted from the pool for global ParserConfig.outbounds (generation-time only).
	ExcludeFromGlobal bool `json:"exclude_from_global,omitempty"`
	// ExposeGroupTagsToGlobal: when true, tags of wizard-marked local outbounds are merged into each global outbound at generation time (SPEC 026).
	ExposeGroupTagsToGlobal bool `json:"expose_group_tags_to_global,omitempty"`
	// Disabled: quick on/off toggle exposed in the wizard Sources list.
	// When true, the parser pipeline skips this source entirely (no fetch,
	// no parse, no nodes generated). The source stays in the file so the
	// user can re-enable it without re-entering its URL / skip rules / etc.
	// Omit-when-default so legacy ParserConfig files (no field) are treated
	// as enabled, matching prior behavior.
	Disabled bool `json:"disabled,omitempty"`
}

// WizardConfig represents the wizard configuration for outbounds
// Supports both old format ("wizard":"hide") and new format ("wizard":{"hide":true, "required":2})
type WizardConfig struct {
	Hide     bool `json:"hide,omitempty"`     // Hide outbound from wizard second tab
	Required int  `json:"required,omitempty"` // Optional: 0 or missing=ignore, 1=check presence only, >1=strict match from template
}

// OutboundConfig represents an outbound selector configuration (version 3)
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

	return 0
}

// UnsetSourceIndex means SourceIndex was not assigned; exclude_from_global must not apply.
const UnsetSourceIndex = -1

// ParsedJump is an optional first hop for Xray dialerProxy → sing-box detour (SOCKS, VLESS, …).
// Scheme empty means "socks" (backward compatibility). UUID/Flow are set for vless/vmess hops when GenerateNodeJSON needs them.
type ParsedJump struct {
	Tag      string
	Scheme   string // socks, vless, …
	Server   string
	Port     int
	UUID     string
	Flow     string
	Outbound map[string]interface{}
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
	// Jump is set when the subscription node uses a chain (e.g. Xray dialerProxy → SOCKS before main outbound).
	Jump *ParsedJump
	// SourceIndex is the index into ParserConfig.proxies for this node; UnsetSourceIndex if unknown.
	SourceIndex int
}

// NormalizeParserConfig normalizes ParserConfig structure:
// - Ensures version is set to ParserConfigVersion
// - Sets default reload to "4h" if not specified
// - Optionally updates last_updated timestamp (if updateLastUpdated is true)
func NormalizeParserConfig(parserConfig *ParserConfig, updateLastUpdated bool) {
	if parserConfig == nil {
		return
	}

	parserConfig.ParserConfig.Version = ParserConfigVersion

	if parserConfig.ParserConfig.Parser.Reload == "" {
		parserConfig.ParserConfig.Parser.Reload = "4h"
	}

	if updateLastUpdated {
		parserConfig.ParserConfig.Parser.LastUpdated = time.Now().UTC().Format(time.RFC3339)
	}
}
