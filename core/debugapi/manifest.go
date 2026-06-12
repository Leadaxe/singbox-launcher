package debugapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

// Self-description constants and the docs-URL helper (SPEC 078). Shared by the
// GET / manifest, GET /help, and the settings "connection card" so every
// surface reports the same identity and points at the same documentation.
const (
	// APIDisplayName — human/agent-readable API name in the manifest and card.
	APIDisplayName = "singbox-launcher debug API"
	// APISpec — contract version; bump on breaking changes to the endpoint set.
	APISpec = "debugapi/v1"
	// APIAuthScheme — how to authenticate (shown so an agent knows the header).
	APIAuthScheme = "Authorization: Bearer <token>"
	// APIHint — one-liner telling an agent where to read more.
	APIHint = "Full reference at the docs URL. Live endpoint list at GET /help."
)

// releaseVersionRe matches a clean release tag like "v1.1.5" (no -N-gSHA / dirty
// suffix). Only those pin the docs link to a frozen tag; dev builds use main.
var releaseVersionRe = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// DocsURL returns the GitHub link to docs/API.md pinned to launcherVer when it
// is a release tag (vX.Y.Z), so an agent reads the doc matching this exact
// build. Dev / non-release versions (v-local-test, vX.Y.Z-N-gSHA, *-dirty)
// fall back to main.
func DocsURL(launcherVer string) string {
	ref := "main"
	if v := strings.TrimSpace(launcherVer); releaseVersionRe.MatchString(v) {
		ref = v
	}
	return "https://github.com/Leadaxe/singbox-launcher/blob/" + ref + "/docs/API.md"
}

// apiEndpoint is one row of the endpoint registry — the single source of truth
// for routing (routes()), the GET / manifest, and GET /help. Keeping the
// handler here means a path can never appear in the docs without being wired,
// or be wired without showing up in /help.
type apiEndpoint struct {
	Method  string
	Path    string
	Auth    bool
	Summary string
	handler http.HandlerFunc
}

// endpointView is the documentation projection of an apiEndpoint (no handler),
// emitted by GET / and GET /help.
type endpointView struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Summary string `json:"summary"`
	Auth    bool   `json:"auth"`
}

func endpointViews(eps []apiEndpoint) []endpointView {
	out := make([]endpointView, 0, len(eps))
	for _, e := range eps {
		out = append(out, endpointView{Method: e.Method, Path: e.Path, Summary: e.Summary, Auth: e.Auth})
	}
	return out
}

// ConnectionCardJSON builds the indented JSON "connection card" the settings
// screen copies to the clipboard (SPEC 078): everything an agent needs to
// connect — base URL, real token, versions, auth scheme and a version-pinned
// docs link. Unlike the GET / manifest (served behind auth, where the agent
// already has the token), the card carries base_url + token so the user can
// hand it over and the agent connects from scratch.
func ConnectionCardJSON(baseURL, token, launcherVer, coreVer string) (string, error) {
	card := map[string]any{
		"api":      APIDisplayName,
		"spec":     APISpec,
		"base_url": baseURL,
		"launcher": launcherVer,
		"core":     coreVer,
		"auth":     APIAuthScheme,
		"token":    token,
		"docs":     DocsURL(launcherVer),
		"hint":     APIHint,
	}
	b, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
