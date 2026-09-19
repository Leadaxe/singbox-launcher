# REVIEW_SOURCE_KINDS — SPEC 134 pre-release review

**Scope:** document-level source classification and direct-path URI/JSON/ini
parsing (`core/config/linkmap/source.go`, `detect.go`, `select.go`,
`parse.go`, `exec.go`), `contract/registry/source_kinds.json`, and the
item-format/base64_rawurl parts of `core/config/nodeflow/sanitize.go`.

## Method

Read the target files end to end (source-kind selection/unwrap loop, `detect`
predicates, JSON/ini path lookups, URI lexer, percent-decode, query
dedup/ordering, `on_len_gt`, `uri_param_unknown`/`json_field_unknown`).
Reproduced hostile-input scenarios with a throwaway test
(`ClassifySource` on empty text, BOM, CRLF, 5MB strings, line-broken/no-padding
base64, base64-in-base64 beyond `max_unwrap_depth`, JSON with trailing
garbage, a mixed Xray/non-Xray/scalar array, `.conf` without `[Interface]`,
and malformed `vpn://`) — no panics, no unbounded recursion. Confirmed a
concurrency defect with `go test -race`, fixed it, and turned the repro into
a committed regression test.

## Confirmed defect (fixed)

**Data race on `namedValueMaps`/`namedValueMapsLoaded`** —
`core/config/linkmap/plan.go:722-741` (before fix).

`lookupNamedValueMap` (resolves `$ref` value-maps such as `tls.fp_dialect`,
called from `applyValueMap` in `exec.go` on every parsed URI/`.conf`) used a
check-then-act pattern on a bare `bool` flag, then wrote to a shared
`map[string]map[string]interface{}` with no lock:

```go
if !namedValueMapsLoaded {
    loadNamedValueMaps()
    namedValueMapsLoaded = true
}
```

Node parsing is not single-threaded in practice: e.g. `RebuildNodePool` runs
in a background goroutine from
`ui/configurator/tabs/source_edit_window.go:924` while foreground code parses
nodes too. Two goroutines racing through the first call cause concurrent
`map[string]interface{}` writes — `go test -race` reproduced it deterministically
(`fatal`-class data race, not just a flaky read). Silent corruption or a crash
were both possible depending on timing.

Fix: replaced the flag with `sync.Once` (same pattern already used for
`planCache` a few lines above), and made `loadNamedValueMaps` (re)initialize
the map itself. Added `TestNamedValueMapConcurrentAccess`
(`core/config/linkmap/linkmap_test.go`) that spins 20 goroutines through
`lookupNamedValueMap` — red under `-race` before the fix, green after.

## Left to the owner

Nothing else in scope. All other reviewed paths (detect predicates, JSON/ini
path lookups, `ParseQueryOrdered`/`queryValue` duplicate-key handling,
`percentUnescape`, `QueryNames`/`JSONKeys`/`INIKeys` ordering,
`applyOnLenGt`) were already hardened by prior DELTAS/QUIRKS fixes and use
comma-ok type assertions consistently; no reproducible defect was found there.

## Verification

```
go build ./...
go test -count=1 -run 'TestEngine|TestContract|TestNoSchemeNames|TestRegistry|TestSourceKinds' ./core/config/... ./contract/...
go test -count=1 -race -run TestNamedValueMapConcurrentAccess ./core/config/linkmap/...
```

All green.
