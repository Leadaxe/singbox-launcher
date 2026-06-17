// Package business — SPEC 079: read a config file's text for the Sources
// "Add from file" button. The content (a .conf [Interface]/[Peer], a .vpn
// vpn:// link, or a list of proxy URIs) is fed into the same AppendURLsToSources
// path as the Add field, so all parsing (SPEC 075/076 + URI parser) is reused.
package business

import (
	"fmt"
	"io"
	"strings"
)

// maxSourceFileBytes caps how much of a picked file is read. Generous for any
// real .conf/.vpn/URI-list, yet bounds a hostile/huge file. (vpn:// profiles
// have their own 512 KB cap inside the parser.)
const maxSourceFileBytes = 1 << 20 // 1 MB

// ReadSourceFileText reads a source file's text (trimmed) with a size cap.
// Returns an error if the file exceeds the cap or can't be read; the caller
// feeds the text into AppendURLsToSources, which decides what it is.
func ReadSourceFileText(r io.Reader) (string, error) {
	if r == nil {
		return "", fmt.Errorf("nil reader")
	}
	b, err := io.ReadAll(io.LimitReader(r, maxSourceFileBytes+1))
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	if len(b) > maxSourceFileBytes {
		return "", fmt.Errorf("file too large (over %d bytes)", maxSourceFileBytes)
	}
	return strings.TrimSpace(string(b)), nil
}
