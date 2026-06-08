package subscription

import (
	"encoding/base64"
	"fmt"
)

// encoding_utils.go consolidates the base64 padding-fallback decoders that
// previously lived in node_parser.go (decodeBase64WithPadding) and
// decoder.go (tryDecodeBase64). Both tried the same four variants in the
// same order (URL-safe no-pad, Std no-pad, URL-safe padded, Std padded) and
// returned the raw decoded bytes; this helper is the single source of truth.
//
// NOTE: meta.go's tryBase64Decode is deliberately NOT routed here. It differs
// observably: it tries a different variant set/order (Std, RawStd, URL,
// RawURL), rejects results that are not valid UTF-8 or that contain control
// characters, trims surrounding whitespace, and returns a string ("" on
// failure) rather than raw bytes. Merging it would change which inputs decode
// and what is returned, so it is left intact.

// DecodeBase64Multi decodes s by trying base64 variants in the canonical
// subscription order and returns the first successful result:
//  1. URL-safe base64 without padding (most common in subscriptions)
//  2. standard base64 without padding
//  3. URL-safe base64 with padding
//  4. standard base64 with padding
//
// It also returns a human-readable description of the variant that succeeded.
// If every variant fails, it returns a non-nil error.
func DecodeBase64Multi(s string) ([]byte, string, error) {
	if decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(s); err == nil {
		return decoded, "URL-safe base64", nil
	}
	if decoded, err := base64.StdEncoding.WithPadding(base64.NoPadding).DecodeString(s); err == nil {
		return decoded, "Standard base64", nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(s); err == nil {
		return decoded, "URL-safe base64 (with padding)", nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil {
		return decoded, "Standard base64 (with padding)", nil
	}
	return nil, "", fmt.Errorf("failed to decode base64")
}
