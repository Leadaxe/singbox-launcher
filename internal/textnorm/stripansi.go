package textnorm

import (
	"regexp"
	"strings"
)

// CSI / escape sequences (ECMA-48) as emitted by many CLIs (e.g. sing-box colored logs).
var reANSI = regexp.MustCompile(`\x1b(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~])`)

// Orphan SGR fragments when U+001B was dropped (e.g. some UI layers strip ESC).
var reOrphanSGR = regexp.MustCompile(`\[[0-9;]*m`)

// StripANSI removes terminal color and cursor escape codes from s for plain-text UI.
func StripANSI(s string) string {
	if s == "" {
		return s
	}
	s = reANSI.ReplaceAllString(s, "")
	s = reOrphanSGR.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}
