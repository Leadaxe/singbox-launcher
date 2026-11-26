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
	"errors"
	"fmt"
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
	// SPEC 187: a ".json" reference is a source-format rule-set. Its tag keeps
	// the extension as a suffix ("<name>-<hash8>.json"), so the file name and
	// the format are derivable from the tag alone, everywhere.
	ext := ""
	if hasJSONExt(path) {
		ext = jsonExt
		path = path[:len(path)-len(jsonExt)]
	}
	filename := strings.TrimSuffix(path, srsExt)
	if filename == "" {
		filename = "srs"
	}
	sum := sha256.Sum256([]byte(urlStr))
	return filename + "-" + hex.EncodeToString(sum[:4]) + ext
}

const (
	srsExt  = ".srs"
	jsonExt = ".json"

	// FormatBinary / FormatSource — sing-box rule_set "format" values.
	FormatBinary = "binary"
	FormatSource = "source"
)

func hasJSONExt(s string) bool {
	return len(s) >= len(jsonExt) && strings.EqualFold(s[len(s)-len(jsonExt):], jsonExt)
}

// FileName is the on-disk name of the rule-set copy for a tag: "<tag>.srs",
// or the tag itself when it already carries ".json" (SPEC 187). The only place
// the name is formed — downloader, refresh loop, build, deploy and GC share it.
func FileName(tag string) string {
	if hasJSONExt(tag) {
		return tag
	}
	return tag + srsExt
}

// TagFromFileName reverses FileName (orphan GC walks the directory).
func TagFromFileName(name string) string {
	if hasJSONExt(name) {
		return name
	}
	return strings.TrimSuffix(name, srsExt)
}

// Format is the rule-set format of a tag or file name: source for ".json",
// binary for everything else.
func Format(tagOrName string) string {
	if hasJSONExt(tagOrName) {
		return FormatSource
	}
	return FormatBinary
}

// FormatFromRef is the rule-set format of a reference (URL or file://) by the
// extension of its path.
func FormatFromRef(ref string) string { return Format(TagFromURL(ref)) }

// IsFileRef reports whether a rule-set reference is a local file (file://).
func IsFileRef(ref string) bool {
	return len(ref) >= 7 && strings.EqualFold(ref[:7], "file://")
}

// hasDrivePrefix reports "X:" at the start of s followed by a slash.
func hasDrivePrefix(s string) bool {
	if len(s) < 3 || s[1] != ':' || (s[2] != '/' && s[2] != '\\') {
		return false
	}
	c := s[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// IsAbsPath reports whether s looks like an absolute filesystem path of any
// platform ("/…", "C:\…", "C:/…", "\\server\…") — a pasted path the rule
// dialog turns into a file:// reference.
func IsAbsPath(s string) bool {
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, `\\`) || hasDrivePrefix(s)
}

// ErrNetworkPath — a Windows network path (\\server\share) or a file:// URL
// with a host: not supported as a rule-set source.
var ErrNetworkPath = errors.New("network paths are not supported")

// FileRefFromPath turns an absolute path into a file:// reference (RFC 8089):
//
//	/Users/a/my rules/x.json → file:///Users/a/my%20rules/x.json
//	C:\Rules\x.srs           → file:///C:/Rules/x.srs
//
// Pure string logic (no runtime.GOOS): a state written on one OS stays
// readable on another.
func FileRefFromPath(p string) (string, error) {
	if strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//") {
		return "", ErrNetworkPath
	}
	if hasDrivePrefix(p) {
		p = "/" + strings.ReplaceAll(p, `\`, "/")
	}
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("not an absolute path: %s", p)
	}
	return (&url.URL{Scheme: "file", Path: p}).String(), nil
}

// PathFromFileRef reverses FileRefFromPath. The result uses forward slashes
// (Windows accepts them); the caller may pass it through filepath.FromSlash.
func PathFromFileRef(ref string) (string, error) {
	if !IsFileRef(ref) {
		return "", fmt.Errorf("not a file reference: %s", ref)
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
		return "", ErrNetworkPath
	}
	p := u.Path
	if p == "" {
		return "", fmt.Errorf("empty path in %s", ref)
	}
	if len(p) > 1 && p[0] == '/' && hasDrivePrefix(p[1:]) {
		p = p[1:]
	}
	return p, nil
}
