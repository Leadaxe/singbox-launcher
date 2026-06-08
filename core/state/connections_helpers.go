// File connections_helpers.go — shared helpers for legacy <-> canonical
// connection model conversion.
package state

import (
	"fmt"
	"net/url"
	"strings"
)

// buildTagSpecFromLegacy возвращает *TagSpec (или nil если все три поля пустые).
func buildTagSpecFromLegacy(prefix, postfix, mask string) *TagSpec {
	if prefix == "" && postfix == "" && mask == "" {
		return nil
	}
	return &TagSpec{
		Prefix:  prefix,
		Postfix: postfix,
		Mask:    mask,
	}
}

// serverLabelFromLegacy зеркалит v5.serverLabel: tag_prefix + fragment +
// tag_postfix (или fallback "server-N"). Используется когда legacy ProxySource
// (с connections[]) попадает в state без существующего Source-матча по URI.
func serverLabelFromLegacy(uri string, oneBasedIndex int, tagPrefix, tagPostfix string) string {
	frag := extractURIFragment(uri)
	if frag == "" {
		frag = ""
	}
	if frag == "" {
		// fallback: server-N
		base := ""
		if !strings.Contains(tagPrefix, "{$") {
			base += tagPrefix
		}
		base += sprintfServerN(oneBasedIndex)
		if !strings.Contains(tagPostfix, "{$") {
			base += tagPostfix
		}
		return base
	}
	out := frag
	if !strings.Contains(tagPrefix, "{$") {
		out = tagPrefix + out
	}
	if !strings.Contains(tagPostfix, "{$") {
		out = out + tagPostfix
	}
	return out
}

// extractURIFragment — `vless://...#name` → "name" (percent-decoded).
func extractURIFragment(s string) string {
	hashAt := strings.Index(s, "#")
	if hashAt < 0 {
		return ""
	}
	frag := s[hashAt+1:]
	if frag == "" {
		return ""
	}
	if dec, err := url.QueryUnescape(frag); err == nil {
		return dec
	}
	return frag
}

// sprintfServerN — fmt.Sprintf("server-%d", n).
func sprintfServerN(n int) string { return fmt.Sprintf("server-%d", n) }
