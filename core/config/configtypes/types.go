// Package configtypes contains shared data types for configuration parsing.
// Extracted to its own package to break the circular dependency between
// core/config and core/config/subscription: both packages import configtypes
// for shared types, while core/config can now safely import subscription.
package configtypes

import (
	"encoding/json"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"time"

	"singbox-launcher/internal/constants"
)

// ParserConfigVersion is the current version of ParserConfig format
const ParserConfigVersion = 4

// canonicalGOOSName returns the canonical-case OS name used in our
// subscription request headers (`X-Device-OS`) and User-Agent.
//
// Form is `macOS` / `windows` / `linux` to match the Remnawave HWID docs
// (https://docs.rw/docs/features/hwid-device-limit/), which is the panel
// generation that actually parses these fields. Unknown GOOS (rare:
// `freebsd`, `netbsd`, …) falls through unchanged — better than masking
// it as "linux" and breaking eventual support if those builds ship.
func canonicalGOOSName(goos string) string {
	switch goos {
	case "darwin":
		return "macOS"
	case "windows":
		return "windows"
	case "linux":
		return "linux"
	default:
		return goos
	}
}

// BuildSubscriptionUserAgent returns the User-Agent string sent on every
// subscription fetch. Format follows the de-facto product/version (platform)
// convention used by Mozilla / v2rayNG / hiddify and required by HWID-binding
// panels (Remnawave / Marzneshin) which reject unknown clients like our
// previous `SubscriptionParserClient` and return 0-byte bodies.
//
// Examples:
//
//	LxBox/1.1.4 (desktop; macOS)
//	LxBox/1.1.4 (desktop; windows)
//	LxBox/1.1.4 (desktop; linux)
//
// Product brand token is "LxBox" with a "desktop" variant tag (distinguishes
// from the Android LxBox build). The UA must NOT contain a bare "singbox" (no
// hyphen): some subscription panels (Remnawave/Marzban-style) route the
// response body by a substring match on the User-Agent, and a bare "singbox"
// is matched as a non-sing-box client — the panel then serves a full
// client-config JSON instead of the base64/URI subscription list the launcher
// can ingest. The regression test in useragent_test.go guards against it.
//
// See SPEC 061-F-N §"Request headers" §1.
func BuildSubscriptionUserAgent() string {
	ver := strings.TrimSpace(constants.AppVersion)
	ver = strings.TrimPrefix(ver, "v")
	if ver == "" {
		ver = "unknown"
	}
	return fmt.Sprintf("LxBox/%s (desktop; %s)", ver, canonicalGOOSName(runtime.GOOS))
}

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
	// DetourTag: SPEC 077 — tag of another outbound that this source's nodes
	// dial through (proxy chain). Empty = direct. Promoted to node.Outbound
	// ["detour"] for each non-WireGuard node at parse time; validated (dangling/
	// cycle/self → dropped) at generation time.
	DetourTag string `json:"detour_tag,omitempty"`
	// DetourNodeHash: SPEC 101 — identity hash (config.NodeIdentityHash) of a
	// single node this source's nodes dial through. The alternative to DetourTag
	// for chaining through one concrete node (e.g. a WARP endpoint) instead of a
	// group: node tags are runtime-generated (prefix/mask/uniquify) and would go
	// stale, the hash survives renames and reorders — same reasoning as
	// DisabledNodes. Mutually exclusive with DetourTag (UI enforces; if both are
	// set, DetourNodeHash wins). Resolved hash→tag at generation time, when all
	// sources are loaded and tags are final; an unresolved hash drops this
	// source's nodes from the config (fail-closed — traffic must not silently go
	// direct).
	DetourNodeHash string `json:"detour_node_hash,omitempty"`
	// DetourNodeLabel: display-only snapshot of the picked node's tag at pick
	// time, so the Source dialog can show a human label without resolving the
	// hash (the node may be temporarily absent). Never used for resolution.
	DetourNodeLabel string `json:"detour_node_label,omitempty"`
	// DisabledNodes: SPEC 094 D4 — per-node off switch, keyed by the node's
	// identity hash (config.NodeIdentityHash) and valued with the unix time the
	// mark was last confirmed.
	//
	// Keyed by hash rather than by tag or position because both are unstable:
	// providers rename nodes between refreshes and reorder them freely, so a
	// tag-keyed mark would silently jump to a different server. The hash covers
	// everything that describes the connection, so the mark follows the actual
	// node and survives refreshes, restarts and provider-side renames.
	//
	// The timestamp exists for garbage collection: a node that has been gone
	// from the subscription longer than the TTL drops out of the map, otherwise
	// it would grow without bound over a subscription's lifetime.
	DisabledNodes map[string]int64 `json:"disabled_nodes,omitempty"`
	// ConfigJSON: ручной sing-box outbound/endpoint объект (server-source).
	// Если задан, LoadNodesFromSource строит ноду из него (Connections не
	// парсятся), а генератор эмитит map passthrough — включая типы и поля,
	// которых лаунчер не знает. Tag и detour перештамповываются как обычно.
	ConfigJSON json.RawMessage `json:"config_json,omitempty"`
}

// Sentinel ref values for OutboundConfig (SPEC 058-R-N STATE_AS_TEMPLATE_DIFF).
//
// Outbound entries в state.connections.outbounds[] делятся на два класса:
//   - **Direct (прямые):** self-contained body. `Ref` пустой (поле отсутствует в JSON).
//   - **Referenced (ссылочные):** body живёт в template или preset, в state только tag + ref.
//
// Для ссылочных entries `Ref` принимает одно из двух значений:
//   - `RefTemplate` — body из `template.parser_config.outbounds[tag]`.
//   - `<preset_id>` — body из `template.presets[id].outbounds` (mode=add).
//
// Update-level (`OutboundUpdate.Ref`) — `<preset_id>` для preset patch'ей либо
// `RefUser` для пользовательского field-level diff поверх referenced body.
//
// Validation: preset.id regex `^[a-z0-9_-]+$` не пересекается с этими константами
// (UPPERCASE + `#`) by construction — collision невозможна.
const (
	RefTemplate = "#TEMPLATE#" // только в state.outbounds[].ref — referenced template entry
	RefUser     = "#USER#"     // только в state.outbounds[].updates[].ref — user patch
)

// OutboundConfig represents an outbound selector configuration.
//
// **Origin class (SPEC 058-R-N):**
//   - `Ref == ""` (поле отсутствует) — direct entry, body inline в state. Full ownership.
//   - `Ref == RefTemplate` — referenced template entry. Body live из
//     `template.parser_config.outbounds[tag]`; body-поля в state НЕ хранятся
//     (omitempty). USER edit становится field-level diff в `Updates[]` с ref=RefUser.
//   - `Ref == "<preset_id>"` — referenced preset add entry. Body live из
//     `template.presets[id].outbounds` (mode=add). USER edit аналогично через USER patch.
//
// **Updates stack (SPEC 057-R-N):** стек patches от `preset.outbounds[mode=update]`
// + опциональный USER patch. Merged body для emit вычисляется через ResolveOutbound /
// MergeOutboundUpdatesInPlace (base + apply updates в order; USER patch всегда последний).
//
// **Required:** template-only flag — указывает что outbound обязателен и
// не должен быть полностью удалён (UI блокирует Del, но Edit + Reset OK).
// В state.json приходит из миграции wizard.required (legacy) → required.
// В template — `required: true` на уровне outbound.
type OutboundConfig struct {
	Tag              string                 `json:"tag"`
	Type             string                 `json:"type,omitempty"`
	Options          map[string]interface{} `json:"options,omitempty"`
	Filters          map[string]interface{} `json:"filters,omitempty"`
	AddOutbounds     []string               `json:"addOutbounds,omitempty"`
	PreferredDefault map[string]interface{} `json:"preferredDefault,omitempty"`
	Comment          string                 `json:"comment,omitempty"`
	Required         bool                   `json:"required,omitempty"` // template-only marker (см. RequiredOutboundTags)

	// SPEC 057/058-R-N: preset/template binding.
	Ref     string           `json:"ref,omitempty"`     // "" (direct) | "#TEMPLATE#" | "<preset_id>"
	Updates []OutboundUpdate `json:"updates,omitempty"` // стек patches: preset patches в rule order + опц. USER patch (всегда последний)
}

// OutboundUpdate — одна запись в стеке `OutboundConfig.Updates` (SPEC 057/058-R-N).
//
// `Ref` принимает:
//   - `<preset_id>` — patch от активного preset'а (mode=update). Stale → drop через sync.
//   - `RefUser` — пользовательский field-level diff от merged_base. Один на outbound,
//     replace при каждом Save, всегда последний в order.
//
// Merged body вычисляется через ResolveOutbound:
// `merged = base; for each u in Updates: merged = applyOutboundUpdatePatch(merged, u.Patch)`.
type OutboundUpdate struct {
	Ref   string                 `json:"ref"`   // <preset_id> | RefUser
	Patch map[string]interface{} `json:"patch"` // patch fields (filters, options, addOutbounds, ...)
}

// IsReferenced возвращает true если entry — referenced (#TEMPLATE# или preset_id),
// false для direct (пустой Ref). Body для referenced live из template/preset.
func (oc *OutboundConfig) IsReferenced() bool {
	return oc.Ref != ""
}

// IsTemplateRef возвращает true если entry ссылается на template global outbound.
func (oc *OutboundConfig) IsTemplateRef() bool {
	return oc.Ref == RefTemplate
}

// IsPresetRef возвращает true если entry ссылается на preset add outbound.
func (oc *OutboundConfig) IsPresetRef() bool {
	return oc.Ref != "" && oc.Ref != RefTemplate
}

// UnsetSourceIndex means SourceIndex was not assigned; exclude_from_global must not apply.
const UnsetSourceIndex = -1

// SchemeGroup marks a ParsedNode that is an outbound group (selector/urltest)
// imported from a sing-box config (SPEC 094 A5).
//
// Such a node is an ORDINARY entry in the subscription's node list with no
// privileges: it does not appear in the wizard's Outbounds tab, routing rules
// never reference it, and the user does not configure its membership. That tab
// stays reserved for the launcher's own channels, which do drive routing.
//
// For sing-box it emits as a real selector/urltest inside the outbounds array;
// for the launcher it is just another node the user can pick.
const SchemeGroup = "group"

// GroupMembersKey is the ParsedNode.Outbound field holding the group's member
// tags. Mirrors sing-box's own "outbounds" field on selector/urltest.
const GroupMembersKey = "outbounds"

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
	//
	// Deprecated (SPEC 094 B1): holds only the first hop. Use Chain, which
	// carries the full path. Jump stays in sync with Chain[0] so existing
	// readers and state.json files written before SPEC 094 keep working.
	Jump *ParsedJump
	// SourceTag is the node's tag exactly as it appeared in an imported
	// sing-box config, before prefix/mask/uniquification (SPEC 094 A5).
	//
	// Imported selector/urltest groups list their members by that original
	// tag, so rebinding them to the final tags needs this untouched copy.
	// Empty for nodes that did not come from a sing-box config.
	SourceTag string
	// Chain is the ordered detour path from the nearest hop outwards
	// (SPEC 094 B1). Empty means the node dials directly.
	//
	// A node reaches its server through Chain[0] → Chain[1] → … → node, i.e.
	// the emitted outbound carries detour=Chain[0].Tag, and each hop carries
	// detour of the next one. Depth is capped at 8 hops.
	Chain []*ParsedNode
	// SourceIndex is the index into ParserConfig.proxies for this node; UnsetSourceIndex if unknown.
	SourceIndex int
	// EmitRaw marks a node built from a manual config_json (ProxySource.
	// ConfigJSON): the generator serializes Outbound as-is (tag/detour
	// restamped) instead of reassembling fields through the per-scheme
	// emitter — the whole point is carrying types and fields the emitter
	// does not know about.
	EmitRaw bool
	// Warnings — коды деградаций, применённых к узлу при разборе
	// (SPEC 103, фаза 2). Словарь кодов — contract/registry/warnings.json.
	//
	// До этого деградация уходила только в debuglog: пользователь видел
	// «нода есть», но не знал, что у неё срезали обфускацию или заменили
	// отпечаток. Коды позволяют показать это в UI и сверять поведение
	// обоих приложений по общему корпусу, а не по тексту лога.
	//
	// Порядок не нормируется, дубли не хранятся: код отвечает на вопрос
	// «что случилось», а не «сколько раз».
	Warnings []string
}

// AddWarning помечает узел кодом деградации, не создавая дублей.
func (n *ParsedNode) AddWarning(code string) {
	if n == nil || code == "" {
		return
	}
	for _, existing := range n.Warnings {
		if existing == code {
			return
		}
	}
	n.Warnings = append(n.Warnings, code)
}

// SyncJumpFromChain refreshes the deprecated Jump field from Chain[0].
//
// SPEC 094 B1: Chain is the source of truth; Jump is kept alive so code paths
// and persisted state predating the chain model keep reading a valid first hop.
func (n *ParsedNode) SyncJumpFromChain() {
	if n == nil {
		return
	}
	if len(n.Chain) == 0 {
		n.Jump = nil
		return
	}
	hop := n.Chain[0]
	if hop == nil {
		n.Jump = nil
		return
	}
	n.Jump = &ParsedJump{
		Tag:      hop.Tag,
		Scheme:   hop.Scheme,
		Server:   hop.Server,
		Port:     hop.Port,
		UUID:     hop.UUID,
		Flow:     hop.Flow,
		Outbound: hop.Outbound,
	}
}

// AdoptLegacyJump rebuilds Chain from a legacy single-hop Jump.
//
// SPEC 094 B1 migration: state.json written before the chain model carries only
// Jump. Reading it must not lose the hop, so an empty Chain plus a non-nil Jump
// is promoted to a one-element Chain. No-op when Chain is already populated.
func (n *ParsedNode) AdoptLegacyJump() {
	if n == nil || len(n.Chain) > 0 || n.Jump == nil {
		return
	}
	n.Chain = []*ParsedNode{{
		Tag:         n.Jump.Tag,
		Scheme:      n.Jump.Scheme,
		Server:      n.Jump.Server,
		Port:        n.Jump.Port,
		UUID:        n.Jump.UUID,
		Flow:        n.Jump.Flow,
		Outbound:    n.Jump.Outbound,
		SourceIndex: UnsetSourceIndex,
	}}
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
