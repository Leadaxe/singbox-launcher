package business

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/config/configtypes"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

const none = "(none)"

func modelWithOutbounds(t *testing.T, tags ...string) *wizardmodels.WizardModel {
	t.Helper()
	obs := make([]configtypes.Direction, 0, len(tags))
	for _, tag := range tags {
		obs = append(obs, configtypes.Direction{Tag: tag, Type: "selector"})
	}
	// SPEC 117: модель строится сразу canonical — GlobalOutbounds, без
	// строкового транспорта.
	return &wizardmodels.WizardModel{GlobalOutbounds: obs}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestDetourOptions_NoneFirstAndSelected(t *testing.T) {
	m := modelWithOutbounds(t, "proxy", "ru-vpn")
	opts, sel := DetourOptions(m, &configtypes.ProxySource{}, none)
	if len(opts) == 0 || opts[0] != none {
		t.Fatalf("first option must be %q, got %v", none, opts)
	}
	if sel != none {
		t.Errorf("empty DetourTag → selected %q, want %q", sel, none)
	}
	if !contains(opts, "proxy") || !contains(opts, "ru-vpn") {
		t.Errorf("available group tags must be offered, got %v", opts)
	}
}

func TestDetourOptions_ExcludesOwnGroups(t *testing.T) {
	m := modelWithOutbounds(t, "proxy", "ru-vpn")
	src := &configtypes.ProxySource{
		Outbounds: []configtypes.Direction{{Tag: "my-local-auto", Type: "urltest"}},
	}
	opts, _ := DetourOptions(m, src, none)
	if contains(opts, "my-local-auto") {
		t.Errorf("source's own local group must be excluded, got %v", opts)
	}
}

// A subscription's own local groups (any subscription, not just the edited one)
// must NOT be offered as detour targets — only global selectors / presets.
func TestDetourOptions_ExcludesAllSubscriptionLocalGroups(t *testing.T) {
	m := modelWithOutbounds(t, "proxy")
	m.Sources = []wizardmodels.Source{{
		Type:    wizardmodels.SourceTypeSubscription,
		Enabled: true,
		URL:     "https://x/sub1",
		Outbounds: []configtypes.Direction{
			{Tag: "sub-auto", Type: "urltest"},
		},
	}}

	opts, _ := DetourOptions(m, nil, none)
	if contains(opts, "sub-auto") {
		t.Errorf("subscription-local group 'sub-auto' must NOT be a detour target, got %v", opts)
	}
	if !contains(opts, "proxy") {
		t.Errorf("global selector 'proxy' must still be offered, got %v", opts)
	}
}

// Built-in/service outbounds (direct-out/reject/drop) and the template's
// auto-select group (auto-proxy-out) are never offered as detour targets.
func TestDetourOptions_ExcludesBuiltinsAndAuto(t *testing.T) {
	// modelWithOutbounds always seeds direct-out/reject/drop via
	// GetAvailableOutbounds; add the auto group + a manual selector explicitly.
	m := modelWithOutbounds(t, "auto-proxy-out", "proxy-out")
	opts, _ := DetourOptions(m, nil, none)
	for _, banned := range []string{"direct-out", "reject", "drop", "auto-proxy-out"} {
		if contains(opts, banned) {
			t.Errorf("%q must not be a detour target, got %v", banned, opts)
		}
	}
	if !contains(opts, "proxy-out") {
		t.Errorf("manual selector 'proxy-out' must still be offered, got %v", opts)
	}
}

func TestDetourOptions_DanglingSelectionKept(t *testing.T) {
	m := modelWithOutbounds(t, "proxy")
	src := &configtypes.ProxySource{DetourTag: "ghost-group"} // not in available
	opts, sel := DetourOptions(m, src, none)
	if sel != "ghost-group" {
		t.Errorf("selected = %q, want the dangling tag", sel)
	}
	if !contains(opts, "ghost-group") {
		t.Errorf("dangling selection must stay visible/clearable, got %v", opts)
	}
}

// --- SPEC 101 + SPEC 112: single-node detour targets, адресуемые тегом ---

const detourTestServerURI = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@h.example.com:443?encryption=none&security=tls&sni=h.example.com#hop"

func modelWithServerSource(t *testing.T, label, uri string, groupTags ...string) *wizardmodels.WizardModel {
	t.Helper()
	m := modelWithOutbounds(t, groupTags...)
	m.Sources = []wizardmodels.Source{{
		// ID обязателен: SPEC 112-A адресует узел парой «source_id + тег»,
		// и пикер обязан записать в выбор именно его.
		ID:      "01SRV0000000000000000000",
		Type:    wizardmodels.SourceTypeServer,
		Enabled: true,
		Label:   label,
		URI:     uri,
	}}
	return m
}

func TestDetourOptionsWithNodes_OffersServerSources(t *testing.T) {
	m := modelWithServerSource(t, "WARP hop", detourTestServerURI, "proxy")
	opts, sel, choices := DetourOptionsWithNodes(m, &configtypes.ProxySource{}, none)
	if sel != none {
		t.Errorf("selected = %q, want %q", sel, none)
	}
	want := detourNodeMarker + "WARP hop"
	if !contains(opts, want) {
		t.Fatalf("server source must be offered as %q, got %v", want, opts)
	}
	c := choices[want]
	if c.NodeTag == "" || c.Tag != "" {
		t.Errorf("node option must carry NodeTag only, got %+v", c)
	}
	// SPEC 112-A: ссылка — объект, и id источника-цели в выбор попадает.
	if c.NodeSourceID != "01SRV0000000000000000000" {
		t.Errorf("node option must carry the target source id, got %+v", c)
	}
	// Тег узла server-источника — NodeTag, а при пустом откат на Label
	// (Source.NodeTagOrLabel): именно под этим именем узел войдёт в конфиг.
	if c.NodeTag != "WARP hop" {
		t.Errorf("NodeTag = %q, want the source's node tag", c.NodeTag)
	}
	if c.NodeLabel != "WARP hop" {
		t.Errorf("NodeLabel = %q, want the source label", c.NodeLabel)
	}
	if g := choices["proxy"]; g.Tag != "proxy" || g.NodeTag != "" {
		t.Errorf("group option must stay tag-addressed, got %+v", g)
	}
}

// Явно заданный NodeTag побеждает подпись: адресуется тег, а показывается
// подпись — разные роли (SPEC 112 + tag-vs-label).
func TestDetourOptionsWithNodes_UsesNodeTagOverLabel(t *testing.T) {
	m := modelWithServerSource(t, "WARP hop", detourTestServerURI)
	m.Sources[0].NodeTag = "🔥🎭 WARP (MASQUE)"

	_, _, choices := DetourOptionsWithNodes(m, &configtypes.ProxySource{}, none)
	c := choices[detourNodeMarker+"WARP hop"]
	if c.NodeTag != "🔥🎭 WARP (MASQUE)" {
		t.Errorf("NodeTag = %q, ожидался тег узла, а не подпись", c.NodeTag)
	}
}

// Источник, у которого только ручной config_json (URI нет), тоже узел и
// хопом быть вправе: именно так лежал WARP в сломавшемся стейте.
func TestDetourOptionsWithNodes_OffersConfigJSONOnlySource(t *testing.T) {
	m := modelWithOutbounds(t, "proxy")
	m.Sources = []wizardmodels.Source{{
		Type:       wizardmodels.SourceTypeServer,
		Enabled:    true,
		Label:      "WARP hop",
		NodeTag:    "🔥🎭 WARP (MASQUE)",
		ConfigJSON: json.RawMessage(`{"type":"vless","server":"h.example.com","server_port":443}`),
	}}

	opts, _, choices := DetourOptionsWithNodes(m, &configtypes.ProxySource{}, none)
	want := detourNodeMarker + "WARP hop"
	if !contains(opts, want) {
		t.Fatalf("источник с ручным JSON обязан предлагаться как %q, получено %v", want, opts)
	}
	if choices[want].NodeTag != "🔥🎭 WARP (MASQUE)" {
		t.Errorf("NodeTag = %q", choices[want].NodeTag)
	}
}

func TestDetourOptionsWithNodes_ExcludesOwnURI(t *testing.T) {
	m := modelWithServerSource(t, "WARP hop", detourTestServerURI)
	src := &configtypes.ProxySource{Connections: []string{detourTestServerURI}}
	opts, _, _ := DetourOptionsWithNodes(m, src, none)
	if contains(opts, detourNodeMarker+"WARP hop") {
		t.Errorf("a source must not chain through itself, got %v", opts)
	}
}

func TestDetourOptionsWithNodes_SelectedByTag(t *testing.T) {
	m := modelWithServerSource(t, "WARP hop", detourTestServerURI)
	src := &configtypes.ProxySource{
		DetourNodeSourceID: "01SRV0000000000000000000",
		DetourNodeTag:      "WARP hop",
		DetourNodeLabel:    "stale label",
	}
	_, sel, choices := DetourOptionsWithNodes(m, src, none)
	if sel != detourNodeMarker+"WARP hop" {
		t.Errorf("selected = %q, want the live node option", sel)
	}
	if choices[sel].NodeTag != "WARP hop" {
		t.Errorf("selected choice tag mismatch: %q", choices[sel].NodeTag)
	}
}

func TestDetourOptionsWithNodes_DanglingTagKept(t *testing.T) {
	m := modelWithOutbounds(t, "proxy")
	src := &configtypes.ProxySource{DetourNodeTag: "gone-hop-tag", DetourNodeLabel: "gone hop"}
	opts, sel, choices := DetourOptionsWithNodes(m, src, none)
	want := detourNodeMarker + "gone hop"
	if sel != want || !contains(opts, want) {
		t.Errorf("dangling tag must stay visible as %q, got sel=%q opts=%v", want, sel, opts)
	}
	if choices[want].NodeTag != "gone-hop-tag" {
		t.Errorf("dangling choice must keep the stored tag")
	}
}
