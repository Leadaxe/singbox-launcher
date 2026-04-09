// Package urlredact removes user credentials embedded in URL-like substrings (for safe error messages).
package urlredact

import "regexp"

// credentialsInURLPattern matches scheme://user:password@ in common URL forms.
var credentialsInURLPattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+\-.]*://)([^:@/\s]+):([^@/\s]+)@`)

// RedactURLUserinfo replaces password segments in matched URLs with ***.
func RedactURLUserinfo(message string) string {
	return credentialsInURLPattern.ReplaceAllString(message, "${1}${2}:***@")
}
