package template

import (
	"sort"
	"strings"
)

// EvalIf reports whether an if/if_or guard is satisfied for the given vars:
// every name in ifList must resolve to "true" (case-insensitive), AND ifOrList
// is empty OR at least one of its names is "true". A leading "@" on a name is
// stripped before lookup (SPEC 067 canonical form).
//
// This is the single source of truth for preset / rule / outbound if/if_or
// activation, shared by the build pipeline (core/build) and the configurator UI
// (ui/configurator) so server-side emit and the UI preview never diverge.
func EvalIf(ifList, ifOrList []string, varsMap map[string]string) bool {
	ok, _ := EvalIfWithReason(ifList, ifOrList, varsMap)
	return ok
}

// EvalIfWithReason is EvalIf plus a human-readable reason when the guard fails
// ("if=<name>" or "if_or=<sorted,names>"); the reason is "" when it passes.
// The if_or names in the reason are sorted (a COPY — the caller's slice is never
// mutated) for deterministic, test-stable output.
func EvalIfWithReason(ifList, ifOrList []string, varsMap map[string]string) (bool, string) {
	for _, name := range ifList {
		if !strings.EqualFold(varsMap[strings.TrimPrefix(name, "@")], "true") {
			return false, "if=" + name
		}
	}
	if len(ifOrList) > 0 {
		anyTrue := false
		for _, name := range ifOrList {
			if strings.EqualFold(varsMap[strings.TrimPrefix(name, "@")], "true") {
				anyTrue = true
				break
			}
		}
		if !anyTrue {
			sorted := append([]string(nil), ifOrList...)
			sort.Strings(sorted)
			return false, "if_or=" + strings.Join(sorted, ",")
		}
	}
	return true, ""
}
