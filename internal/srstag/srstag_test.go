package srstag

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func hash8(u string) string {
	sum := sha256.Sum256([]byte(u))
	return hex.EncodeToString(sum[:4])
}

func TestTagFromURL(t *testing.T) {
	const a = "https://example.com/path/blocklist.srs"
	const b = "https://other.com/path/blocklist.srs"

	cases := []struct {
		name string
		url  string
		want string
	}{
		{"basic", a, "blocklist-" + hash8(a)},
		{"same filename different host → different tag", b, "blocklist-" + hash8(b)},
		{"no extension", "https://h/x/geosite", "geosite-" + hash8("https://h/x/geosite")},
		{"host only → host basename", "https://host.only", "host.only-" + hash8("https://host.only")},
		{"trailing slash → fallback srs", "https://h/dir/", "srs-" + hash8("https://h/dir/")},
		{"unparseable → empty", "://bad url with spaces\x7f", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TagFromURL(tc.url); got != tc.want {
				t.Errorf("TagFromURL(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestTagFromURL_Stable(t *testing.T) {
	const u = "https://cdn.example.org/rules/ads.srs"
	if TagFromURL(u) != TagFromURL(u) {
		t.Fatal("TagFromURL not stable for the same URL")
	}
}

func TestTagFromURL_NoLeadingDash(t *testing.T) {
	for _, u := range []string{"https://host.only", "https://h/dir/", ".srs"} {
		if tag := TagFromURL(u); strings.HasPrefix(tag, "-") {
			t.Errorf("tag for %q starts with '-': %q", u, tag)
		}
	}
}
