// SPEC 054 — preview_nodes generation tests, focus on Xray JSON array format.
package core

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestExtractPreviewNodes_LineBased_Base64Decoded(t *testing.T) {
	// Typical base64-decoded body: one URI per line.
	body := []byte(`vless://uuid@srv1.example.com:443?type=tcp#node-1
vless://uuid@srv2.example.com:443?type=tcp#node-2
vless://uuid@srv3.example.com:443?type=tcp#node-3
# this is a comment line
vmess://uuid@srv4.example.com:443#node-4`)

	preview := extractPreviewNodes(body, 50)
	if len(preview) != 4 {
		t.Errorf("expected 4 entries, got %d: %+v", len(preview), preview)
	}
	if !strings.Contains(preview[0], "node-1") {
		t.Errorf("unexpected first entry: %q", preview[0])
	}
	// Verify each entry is reasonably small.
	for i, p := range preview {
		if len(p) > 500 {
			t.Errorf("entry %d too large: %d bytes", i, len(p))
		}
	}

	count := countURIs(body)
	if count != 4 {
		t.Errorf("countURIs: expected 4, got %d", count)
	}
}

func TestExtractPreviewNodes_LineBased_TruncationLimit(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("vless://uuid@srv.example.com:443#node-")
		sb.WriteString("xxxx")
		sb.WriteString("\n")
	}
	preview := extractPreviewNodes([]byte(sb.String()), 50)
	if len(preview) != 50 {
		t.Errorf("expected 50 (limit), got %d", len(preview))
	}
}

// === SPEC 054 fix tests: Xray JSON array body ===

func TestExtractXrayJSONPreviewNodes_Smoke(t *testing.T) {
	body, err := os.ReadFile("config/subscription/testdata/xray_provider_anon.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	preview, total := extractXrayJSONPreviewNodes(body, 50)
	if total == 0 {
		t.Fatalf("expected non-zero total nodes")
	}
	if len(preview) == 0 {
		t.Fatalf("expected non-empty preview")
	}
	// SPEC 054 acceptance: each preview entry < 1 KB.
	for i, p := range preview {
		if len(p) >= 1024 {
			t.Errorf("preview[%d] too large: %d bytes (limit 1024) — content: %q",
				i, len(p), p)
		}
	}
	// URI-like format expected: <scheme>://<server>:<port>#<tag>
	for i, p := range preview {
		if !strings.Contains(p, "://") {
			t.Errorf("preview[%d] not URI-like: %q", i, p)
		}
		if !strings.Contains(p, "#") {
			t.Errorf("preview[%d] missing tag (#...): %q", i, p)
		}
	}
}

func TestExtractXrayJSONPreviewNodes_SyntheticArray(t *testing.T) {
	// Synthetic 100-node JSON array — verifies truncation + count.
	arr := make([]map[string]interface{}, 100)
	for i := range arr {
		arr[i] = map[string]interface{}{
			"outbounds": []map[string]interface{}{{
				"protocol": "vless",
				"tag":      "synthetic-node",
				"settings": map[string]interface{}{
					"vnext": []map[string]interface{}{{
						"address": "srv.example.com",
						"port":    443,
						"users":   []map[string]interface{}{{"id": "00000000-0000-0000-0000-000000000000"}},
					}},
				},
				"streamSettings": map[string]interface{}{"network": "tcp"},
			}},
		}
	}
	body, _ := json.Marshal(arr)

	preview, total := extractXrayJSONPreviewNodes(body, 50)
	if total != 100 {
		t.Errorf("expected total=100, got %d", total)
	}
	if len(preview) != 50 {
		t.Errorf("expected preview limited to 50, got %d", len(preview))
	}
	for i, p := range preview {
		if len(p) >= 1024 {
			t.Errorf("preview[%d] too large: %d bytes", i, len(p))
		}
	}
}

func TestExtractXrayJSONPreviewNodes_BloatedBodyNotBloatedPreview(t *testing.T) {
	// Simulate SPEC 054 bug condition: large JSON body (~50KB synthetic).
	// Before fix: extractPreviewNodes would return [body_as_single_string].
	// After fix: extractXrayJSONPreviewNodes returns small URI strings.
	bigComment := strings.Repeat("x", 1000)
	arr := make([]map[string]interface{}, 5)
	for i := range arr {
		arr[i] = map[string]interface{}{
			"outbounds": []map[string]interface{}{{
				"protocol": "vless",
				"tag":      "n-" + bigComment[:50],
				"settings": map[string]interface{}{
					"vnext": []map[string]interface{}{{
						"address": "srv.example.com", "port": 443,
						"users": []map[string]interface{}{{"id": "00000000-0000-0000-0000-000000000000"}},
					}},
				},
				"streamSettings": map[string]interface{}{"network": "tcp"},
				// embedded comment-junk to bloat the body
				"comment_junk": bigComment,
			}},
		}
	}
	body, _ := json.Marshal(arr)
	if len(body) < 5000 {
		t.Fatalf("test fixture too small: %d", len(body))
	}

	preview, total := extractXrayJSONPreviewNodes(body, 50)
	if total != 5 {
		t.Errorf("expected total=5, got %d", total)
	}
	totalPreviewBytes := 0
	for _, p := range preview {
		totalPreviewBytes += len(p)
	}
	// SPEC 054 acceptance: total preview << body size.
	if totalPreviewBytes >= len(body)/2 {
		t.Errorf("preview total %d bytes is too close to body %d bytes — bloat not fixed",
			totalPreviewBytes, len(body))
	}
}

// === Regression: non-JSON body falls through to line-based path ===

func TestPreviewDispatcher_LineBasedFallthrough(t *testing.T) {
	// Body that doesn't start with `[` should NOT be treated as JSON array.
	body := []byte("vless://uuid@srv:443#a\nvless://uuid@srv:443#b\n")
	// Simulate refreshOneSubscriptionSource dispatch:
	xrayParsed := false
	bodyStr := string(body)
	if len(bodyStr) > 0 && bodyStr[0] == '[' {
		xrayParsed = true
	}
	if xrayParsed {
		t.Errorf("non-JSON body should not be parsed as Xray")
	}
	preview := extractPreviewNodes(body, 50)
	if len(preview) != 2 {
		t.Errorf("line-based path: expected 2, got %d", len(preview))
	}
}
