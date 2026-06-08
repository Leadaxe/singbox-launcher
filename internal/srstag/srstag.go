// Package srstag derives the content-addressed local tag/filename for a
// downloaded SRS rule-set. It is the single source of truth shared by the build
// pipeline (core/build) and the configurator UI (ui/configurator/dialogs) so the
// downloader, the build resolver and the UI always agree on the on-disk
// bin/rule-sets/<tag>.srs key. Divergence between copies = silent cache-miss
// (the rule resolves to a tag whose file was saved under a different name and is
// dropped as "remote .srs not cached"), so the algorithm lives here, once.
package srstag

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
)

// TagFromURL returns "<filename-without-.srs>-<hash8>", where hash8 is the first
// 8 hex chars of SHA-256(url) (~32 bits, collision ~1/4e9).
//
//	https://example.com/path/blocklist.srs → "blocklist-a3f5c2d1"
//	https://other.com/path/blocklist.srs   → "blocklist-7e9b1f04"
//	(repeat of the first URL)               → "blocklist-a3f5c2d1" (dedup)
//
// The same URL always yields the same tag (stable across runs); different URLs
// that share a filename get different tags (no collision). A URL with no
// filename (no slash / no extension) falls back to "srs" so the tag never
// starts with "-". An unparseable URL returns "" — the caller must skip the
// entry.
func TagFromURL(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}
	path := u.Path
	if path == "" {
		path = urlStr
	}
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	filename := strings.TrimSuffix(path, ".srs")
	if filename == "" {
		filename = "srs"
	}
	sum := sha256.Sum256([]byte(urlStr))
	return filename + "-" + hex.EncodeToString(sum[:4])
}
