package subscription

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	config "singbox-launcher/core/config/configtypes"
)

// --- isValidNaiveHeaderName ----------------------------------------------

func TestIsValidNaiveHeaderName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty rejected", "", false},
		{"plain ASCII", "X-Username", true},
		{"plain upper", "CONTENT-TYPE", true},
		{"digits allowed", "X-Version-2", true},
		{"underscore allowed", "X_Secret", true},
		{"pipe allowed", "X|Weird", true},
		{"space rejected", "X Forwarded", false},
		{"colon rejected (reserved as separator)", "X:Y", false},
		{"tab rejected", "X\tY", false},
		{"newline rejected", "X\nY", false},
		{"NUL rejected", "X\x00Y", false},
		{"non-ASCII rejected", "Привет", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidNaiveHeaderName(tt.input); got != tt.want {
				t.Errorf("isValidNaiveHeaderName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// --- parseNaiveExtraHeaders ----------------------------------------------

func TestParseNaiveExtraHeaders(t *testing.T) {
	t.Run("nil on empty", func(t *testing.T) {
		if got := parseNaiveExtraHeaders(""); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("single pair", func(t *testing.T) {
		got := parseNaiveExtraHeaders("X-Username: user")
		want := map[string]string{"X-Username": "user"}
		if !mapsEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("two pairs with CRLF", func(t *testing.T) {
		got := parseNaiveExtraHeaders("X-A: 1\r\nX-B: 2")
		want := map[string]string{"X-A": "1", "X-B": "2"}
		if !mapsEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("value with colons preserved", func(t *testing.T) {
		got := parseNaiveExtraHeaders("X-Time: 12:34:56")
		want := map[string]string{"X-Time": "12:34:56"}
		if !mapsEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("leading/trailing spaces trimmed", func(t *testing.T) {
		got := parseNaiveExtraHeaders("  X-A :  1  ")
		want := map[string]string{"X-A": "1"}
		if !mapsEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("invalid name skipped, valid kept", func(t *testing.T) {
		got := parseNaiveExtraHeaders("X A: bad\r\nX-B: good")
		want := map[string]string{"X-B": "good"}
		if !mapsEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("no separator skipped", func(t *testing.T) {
		got := parseNaiveExtraHeaders("no-colon-here\r\nX-Good: v")
		want := map[string]string{"X-Good": "v"}
		if !mapsEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("all invalid → nil", func(t *testing.T) {
		got := parseNaiveExtraHeaders("no-colon\r\nalso-no-colon")
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("UTF-8 in value allowed", func(t *testing.T) {
		got := parseNaiveExtraHeaders("X-Note: Привет")
		want := map[string]string{"X-Note": "Привет"}
		if !mapsEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

// --- ParseNode for naive URIs --------------------------------------------

func TestParseNode_Naive_Canonical(t *testing.T) {
	// The four examples from the DuckSoft URI spec.
	cases := []struct {
		name     string
		uri      string
		wantUser string
		wantPass string
		wantHost string
		wantPort int
		wantQUIC bool
		wantLabel string
	}{
		{
			name:      "https with user:pass#label",
			uri:       "naive+https://what:happened@test.someone.cf?padding=false#Naive!",
			wantUser:  "what",
			wantPass:  "happened",
			wantHost:  "test.someone.cf",
			wantPort:  443,
			wantQUIC:  false,
			wantLabel: "Naive!",
		},
		{
			name:      "https anonymous with padding=true",
			uri:       "naive+https://some.public.rs?padding=true#Public-01",
			wantHost:  "some.public.rs",
			wantPort:  443,
			wantLabel: "Public-01",
		},
		{
			name:     "quic with user:pass",
			uri:      "naive+quic://manhole:114514@quic.test.me",
			wantUser: "manhole",
			wantPass: "114514",
			wantHost: "quic.test.me",
			wantPort: 443,
			wantQUIC: true,
		},
		{
			name:     "https with encoded extra-headers",
			uri:      "naive+https://some.what?extra-headers=X-Username%3Auser%0D%0AX-Password%3Apassword",
			wantHost: "some.what",
			wantPort: 443,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node, err := ParseNode(tc.uri, nil)
			if err != nil {
				t.Fatalf("ParseNode(%q) error: %v", tc.uri, err)
			}
			if node.Scheme != "naive" {
				t.Errorf("Scheme = %q, want %q", node.Scheme, "naive")
			}
			if node.Server != tc.wantHost {
				t.Errorf("Server = %q, want %q", node.Server, tc.wantHost)
			}
			if node.Port != tc.wantPort {
				t.Errorf("Port = %d, want %d", node.Port, tc.wantPort)
			}
			if node.UUID != tc.wantUser {
				t.Errorf("UUID (username) = %q, want %q", node.UUID, tc.wantUser)
			}
			if got := node.Query.Get("password"); got != tc.wantPass {
				t.Errorf("password = %q, want %q", got, tc.wantPass)
			}
			gotQUIC := node.Query.Get("quic") == "true"
			if gotQUIC != tc.wantQUIC {
				t.Errorf("quic = %v, want %v", gotQUIC, tc.wantQUIC)
			}
			if node.Label != tc.wantLabel {
				t.Errorf("Label = %q, want %q", node.Label, tc.wantLabel)
			}
			if node.Query.Has("padding") {
				t.Errorf("padding must be stripped from Query; got %q", node.Query.Get("padding"))
			}
		})
	}
}

func TestParseNode_Naive_DefaultPort(t *testing.T) {
	node, err := ParseNode("naive+https://host.tld", nil)
	if err != nil {
		t.Fatalf("ParseNode error: %v", err)
	}
	if node.Port != 443 {
		t.Errorf("Port = %d, want 443 (default)", node.Port)
	}
}

func TestParseNode_Naive_CustomPort(t *testing.T) {
	node, err := ParseNode("naive+https://a:b@host.tld:10443", nil)
	if err != nil {
		t.Fatalf("ParseNode error: %v", err)
	}
	if node.Port != 10443 {
		t.Errorf("Port = %d, want 10443", node.Port)
	}
}

func TestParseNode_Naive_PasswordOnly(t *testing.T) {
	// `naive+https://secret@host` — per spec, password alone goes in the user slot.
	node, err := ParseNode("naive+https://secret@host.tld", nil)
	if err != nil {
		t.Fatalf("ParseNode error: %v", err)
	}
	if node.UUID != "secret" {
		t.Errorf("UUID = %q, want %q", node.UUID, "secret")
	}
	if got := node.Query.Get("password"); got != "" {
		t.Errorf("password should be empty for user-only URI, got %q", got)
	}
}

func TestParseNode_Naive_Anonymous(t *testing.T) {
	node, err := ParseNode("naive+https://host.tld", nil)
	if err != nil {
		t.Fatalf("ParseNode error: %v", err)
	}
	if node.UUID != "" {
		t.Errorf("UUID must be empty for anonymous URI, got %q", node.UUID)
	}
	if got := node.Query.Get("password"); got != "" {
		t.Errorf("password must be empty, got %q", got)
	}
}

func TestParseNode_Naive_FragmentUTF8(t *testing.T) {
	// %E2%9C%85 = ✅, %20 = space
	node, err := ParseNode("naive+https://u:p@host.tld:443/?padding=false#%E2%9C%85%20JP-01", nil)
	if err != nil {
		t.Fatalf("ParseNode error: %v", err)
	}
	want := "\u2705 JP-01"
	if node.Label != want {
		t.Errorf("Label = %q, want %q", node.Label, want)
	}
}

func TestParseNode_Naive_InvalidSchemeRejected(t *testing.T) {
	// `naive://` without the `+https` / `+quic` suffix — explicitly NOT in spec.
	_, err := ParseNode("naive://host.tld", nil)
	if err == nil {
		t.Errorf("ParseNode(\"naive://...\") should fail — not a valid naive URI")
	}
}

func TestParseNode_Naive_MaxURILength(t *testing.T) {
	// ParseNode has a blanket 8 KB guard.
	huge := "naive+https://host.tld/?x=" + strings.Repeat("A", MaxURILength+10)
	_, err := ParseNode(huge, nil)
	if err == nil {
		t.Error("oversized URI should be rejected")
	}
}

// --- buildOutbound / JSON shape ------------------------------------------

func TestBuildOutbound_Naive_HTTPS(t *testing.T) {
	node, err := ParseNode("naive+https://user:pass@example.com:443#My%20Naive", nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	node.Tag = "naive-out"
	out := buildOutbound(node)

	assertEq(t, out["type"], "naive")
	assertEq(t, out["tag"], "naive-out")
	assertEq(t, out["server"], "example.com")
	assertEq(t, out["server_port"], 443)
	assertEq(t, out["username"], "user")
	assertEq(t, out["password"], "pass")

	if _, hasQUIC := out["quic"]; hasQUIC {
		t.Errorf("quic must be absent for naive+https, got %v", out["quic"])
	}

	tls, ok := out["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("tls block missing or wrong type: %T", out["tls"])
	}
	assertEq(t, tls["enabled"], true)
	assertEq(t, tls["server_name"], "example.com")

	// naive outbound TLS supports only server_name / certificate / certificate_path / ech.
	// We deliberately do NOT emit alpn/utls/reality/min_version.
	for _, forbidden := range []string{"alpn", "utls", "reality", "min_version"} {
		if _, bad := tls[forbidden]; bad {
			t.Errorf("TLS block must not contain %q (sing-box naive doesn't support it)", forbidden)
		}
	}
}

func TestBuildOutbound_Naive_QUIC(t *testing.T) {
	node, err := ParseNode("naive+quic://u:p@quic.example.com:443", nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	node.Tag = "naive-quic"
	out := buildOutbound(node)

	assertEq(t, out["quic"], true)
	assertEq(t, out["quic_congestion_control"], "bbr")
}

func TestBuildOutbound_Naive_WithExtraHeaders(t *testing.T) {
	uri := "naive+https://u:p@host.tld/?extra-headers=X-Username%3Auser%0D%0AX-Password%3Apassword"
	node, err := ParseNode(uri, nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	node.Tag = "naive-hdr"
	out := buildOutbound(node)

	hdrs, ok := out["extra_headers"].(map[string]interface{})
	if !ok {
		t.Fatalf("extra_headers missing or wrong type: %T", out["extra_headers"])
	}
	assertEq(t, fmt.Sprint(hdrs["X-Username"]), "user")
	assertEq(t, fmt.Sprint(hdrs["X-Password"]), "password")
}

func TestBuildOutbound_Naive_Anonymous(t *testing.T) {
	node, err := ParseNode("naive+https://host.tld", nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	node.Tag = "anon"
	out := buildOutbound(node)
	if _, ok := out["username"]; ok {
		t.Errorf("username must be absent for anonymous URI")
	}
	if _, ok := out["password"]; ok {
		t.Errorf("password must be absent for anonymous URI")
	}
}

// --- Round-trip (URI → ParseNode → buildOutbound → ShareURI) -------------

func TestShareURIRoundtrip_Naive_HTTPS(t *testing.T) {
	input := "naive+https://user:pass@example.com:443#My%20Tag"
	roundtrip := mustNaiveRoundtrip(t, input)

	// Fragment encoding may differ (+ vs %20 for space, underlying url.URL
	// normalization), so compare parsed components, not raw strings.
	u, err := url.Parse(roundtrip)
	if err != nil {
		t.Fatalf("round-trip result not parseable: %v", err)
	}
	if u.Scheme != "naive+https" {
		t.Errorf("scheme = %q, want naive+https", u.Scheme)
	}
	if u.Host != "example.com:443" {
		t.Errorf("host = %q", u.Host)
	}
	if u.User == nil || u.User.Username() != "user" {
		t.Errorf("username = %v, want user", u.User)
	}
	if pw, ok := u.User.Password(); !ok || pw != "pass" {
		t.Errorf("password = %v, ok=%v, want pass", pw, ok)
	}
	frag, _ := url.PathUnescape(u.Fragment)
	if frag != "My Tag" {
		t.Errorf("fragment = %q, want %q", frag, "My Tag")
	}
}

func TestShareURIRoundtrip_Naive_QUIC(t *testing.T) {
	input := "naive+quic://secret@quic.example.com"
	roundtrip := mustNaiveRoundtrip(t, input)
	u, err := url.Parse(roundtrip)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if u.Scheme != "naive+quic" {
		t.Errorf("scheme = %q, want naive+quic", u.Scheme)
	}
	if u.User == nil || u.User.Username() != "secret" {
		t.Errorf("user = %v, want secret (password-only spec)", u.User)
	}
}

func TestShareURIRoundtrip_Naive_ExtraHeaders(t *testing.T) {
	input := "naive+https://u:p@host.tld/?extra-headers=X-A%3A1%0D%0AX-B%3A2"
	roundtrip := mustNaiveRoundtrip(t, input)

	u, err := url.Parse(roundtrip)
	if err != nil {
		t.Fatalf("%v", err)
	}
	got := u.Query().Get("extra-headers")
	// Keys sorted lexicographically on the way out, values preserved.
	// Original had leading/trailing trim-eligible spaces; after parse→emit they're tight.
	want := "X-A: 1\r\nX-B: 2"
	if got != want {
		t.Errorf("extra-headers = %q, want %q", got, want)
	}
}

func TestShareURIFromNaive_EmptyServer(t *testing.T) {
	_, err := ShareURIFromOutbound(map[string]interface{}{
		"type": "naive",
	})
	if err == nil {
		t.Error("expected ErrShareURINotSupported for outbound missing server")
	}
}

func TestShareURIFromNaive_PaddingDroppedOnRoundtrip(t *testing.T) {
	// padding=true is intentionally NOT round-tripped (no sing-box equivalent).
	roundtrip := mustNaiveRoundtrip(t, "naive+https://u:p@host.tld/?padding=true")
	u, err := url.Parse(roundtrip)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if u.Query().Has("padding") {
		t.Errorf("padding should not survive round-trip, got %q", u.RawQuery)
	}
}

// --- helpers --------------------------------------------------------------

// mustNaiveRoundtrip parses `input`, builds the outbound map, and re-encodes
// via ShareURIFromOutbound. Fails the test on any error along the way. If the
// parsed node has an empty Tag (URI had no fragment), falls back to "t" so
// the share-URI encoder doesn't produce an empty fragment.
func mustNaiveRoundtrip(t *testing.T, input string) string {
	t.Helper()
	node, err := ParseNode(input, nil)
	if err != nil {
		t.Fatalf("ParseNode(%q): %v", input, err)
	}
	if node.Tag == "" {
		node.Tag = "t"
	}
	out := buildOutbound(node)
	got, err := ShareURIFromOutbound(out)
	if err != nil {
		t.Fatalf("ShareURIFromOutbound: %v", err)
	}
	return got
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func assertEq(t *testing.T, got, want interface{}) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v (%T), want %v (%T)", got, got, want, want)
	}
}

// ensure unused-import avoidance when config import is expected but unused in some tests
var _ = config.ParsedNode{}
