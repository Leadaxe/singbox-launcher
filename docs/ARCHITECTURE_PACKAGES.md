# Per-package inventory

> Companion to **[ARCHITECTURE.md](ARCHITECTURE.md)** (§8). One-line responsibility
> per package + key files with one-line purposes, grouped by layer L0–L7. Reflects
> the **current** post-SPEC-070 file layout (the monolith splits are landed). Test
> files (`*_test.go`) are omitted from the per-file lists.

Legend for layer headers matches ARCHITECTURE.md §2.

---

## L0 — platform (`internal/platform`)

**Responsibility:** OS abstraction behind one interface — power sleep/wake, HWID
device info, process enumeration, WinTun ghost-adapter cleanup, canonical filesystem
path getters. Platform-tagged files (`*_darwin.go`/`*_linux.go`/`*_windows.go`/
`*_stub.go`); no upward imports.

| File | Purpose |
|------|---------|
| `platform_common.go` | Shared path getters (config.json, wizard_template.json, `bin/wizard_states`, rule-sets, subscriptions) — single source of truth for filesystem layout. |
| `platform_darwin.go` / `platform_linux.go` / `platform_windows.go` | Per-OS path roots, tray/Dock specifics, console-ctrl send (Windows). |
| `power_darwin.go` / `power_linux.go` / `power_windows.go` / `power_stub.go` | Sleep/wake event subscription (IOKit / logind DBus / WM_POWERBROADCAST) + `IsSleeping`/`PowerContext` state; stub = no-op fallback. |
| `device_info_darwin.go` / `_linux.go` / `_windows.go` | HWID extraction (sw_vers / `/etc/os-release` / wmic), cached per process. |
| `proclist.go` + `proclist_darwin.go` / `_linux.go` / `_windows.go` | Cross-platform process enumeration interface + `ProcessEntry` (Traffic Profiler picker). |
| `wintun_cleanup.go` / `wintun_cleanup_other.go` | Shared ghost-adapter mode constants + `ghostTunDecision` pure predicate (platform-agnostic, unit-tested). |
| `wintun_cleanup_windows_device.go` | SPEC 065: SetupAPI device enumeration + `DIF_REMOVE` of phantom adapters. |
| `wintun_cleanup_windows_nla_profiles.go` / `_nla_sigs.go` | NLA network-profile / signature cleanup. |
| `wintun_cleanup_windows_syscall.go` | Lazy DLL bindings + GUID constants shared by the cleanup files. |
| `fs_unix.go` / `fs_windows.go` | Atomic-write / fsync filesystem helpers per OS. |
| `dock_handler.go` / `dock_handler_stub.go` | macOS Dock hide; stub elsewhere. |
| `privileged_darwin.go` / `privileged_stub.go` | macOS privileged escalation (TUN cache/log removal); stub elsewhere. |
| `singbox_exec_path.go` / `singbox_exec_path_linux.go` | Resolve the sing-box executable path (Linux may use `PATH`). |

---

## L1 — shared-internal (leaf utilities)

Each package is self-contained and dependency-free (or depends only on `debuglog`/`constants`).

| Package | Responsibility | Key files |
|---------|----------------|-----------|
| `internal/constants` | App-wide constants (file names, pinned core/template refs, UA strings, limits). | `constants.go` |
| `internal/debuglog` | Leveled logging (Off/Error/Warn/Info/Verbose/Trace), optional in-memory sink for the diagnostics log viewer, timing helpers. | `debuglog.go`, `close.go` |
| `internal/locale` | i18n: English embedded, external/remote JSON per language, `T`/`Tf` lookup with English fallback. | `locale.go`, `settings.go` |
| `internal/traffic` | Decoupled Traffic Profiler (stdlib only): Clash poller + log tailer join, rolling buffer, session recording, per-process attribution. | `profiler.go`, `session.go`, `types.go`, `clash_connections.go`, `logtail.go`, `parser.go`, `http_client.go`, `singleton.go`, `inode_unix.go`/`inode_windows.go` |
| `internal/outboundutil` | Single source of truth for `reject`/`drop` literal → rule `action`/`method` mapping (shared by core build + UI). | `outbound.go` |
| `internal/srstag` | Content-addressed local SRS filename generation (`name-<hash8>`) for dedup. | `srstag.go` |
| `internal/urlsafe` | URL-scheme allowlist for clickable affordances (http/https/tg allowed; javascript/file/data blocked). | `url.go` |
| `internal/urlredact` | Redact userinfo / sensitive query from URLs before logging. | `urlredact.go` |
| `internal/textnorm` | Normalize UTF-8 / display symbols in proxy tags (e.g. `❯ → >`), strip ANSI. | `proxy_display.go`, `stripansi.go` |
| `internal/ctxutil` | Sleep-aware context helper. | `sleep.go` |
| `internal/process` | Thin process-list wrapper used by runtime checks. | `process.go` |
| `internal/wizardsync` | Fyne-free predicates for GUI→model merge (`GuiTextAwaitingProgrammaticFill`, `FinalOutboundSelectReadLooksStale`) — unit-testable without CGO/GL. | `guards.go` |
| `internal/dialogs` | Shared dialog primitives independent of `ui` (custom dialog, download-failed dialog, auto-hide info). | `dialogs.go` |

> Note: `internal/dialogs` and `internal/fynewidget` both depend on Fyne. `dialogs`
> is grouped here as a leaf utility (no internal cross-deps); `fynewidget` is grouped
> at L7 as a widget-tier package. Both are reused across the UI layers.

---

## L2 — core-domain (state + build + config + template)

Pure domain. No Fyne, no `AppController`.

### `core/state` — state schema, load/save, migration

**Responsibility:** Canonical v6 `State` (Connections / Rules / DNS / Vars), v2–v6
load + forward-migration, atomic save, legacy↔canonical adapters, ULID/identity,
per-source raw-body cache.

| File | Purpose |
|------|---------|
| `state.go` | Root `State` struct (identity + legacy `ParserConfig` view + canonical `Connections`/`Rules`/`DNS`) + accessor helpers. |
| `save.go` | Memory→disk: `syncConnectionsFromLegacy`, `marshalDisk` (v6 layout), atomic fsync+rename, SPEC 058 backup. |
| `load_router.go` | `Load`/`Parse`: schema detection (top-level vs `meta.version`), routes to v6/v5/v2-v4 parsers. |
| `load_v6.go` | `parseCurrent` (v6 canonical) + `legacyDevDNSToOptions` fallback + `legacyCustomRulesFromV6` (legacy-view derivation). |
| `load_v5.go` | `parseV5Legacy` + `deriveV6FromLegacy` (BUG1 backfill for headless paths). |
| `load_v2_v3_v4.go` | `parseLegacyAndMigrate` (v2/v3/v4 → v5 → canonical). |
| `load_normalize.go` | Shared `normalizeAfterLoad` (`syncLegacyFromConnections` + nil-slice normalize + outbound-ref sanitize) called by all parse paths. |
| `sync_to_connections.go` | `syncConnectionsFromLegacy` (Save path: ParserConfig→Connections, match by URL/URI, preserve ID/Meta, ULID new sources). |
| `sync_to_legacy.go` | `syncLegacyFromConnections` (Load path: fills ParserConfig from Connections for backward-compat UI). |
| `connections.go` | v5 canonical schema: `ConnectionsSection`, `Source` (subscription/server), `SubscriptionMeta`. |
| `connections_helpers.go` | Hoisted shared `buildTagSpec` / server-label / URI-fragment helpers (deduped from adapter + migration). |
| `rule_types.go` | v6 `Rule` (kind = preset/inline/srs) + bodies + `DecodeBody` validation. |
| `rule_identity.go` | `StableRuleID` pure function (preset→Ref, inline/srs→sanitized name); identity is computed, not stored. |
| `dns_options.go` | v6 flat DNS schema (kind discriminator template/preset/user) + custom Marshal/Unmarshal. |
| `sync_dns.go` | `SyncDNSOptionsWithActivePresets` (idempotent add/remove preset DNS entries on Rule toggle). |
| `migration_v5_to_v6.go` | v5→v6 helpers (`migrateCustomRule` kind heuristic, `migrateDNS`). |
| `legacy_migration.go` | v2/v3/v4→v5 migration (`migrateV4ToV5`, `migrateLegacySources`). |
| `legacy_types.go` / `legacy_v4.go` | Legacy in-memory + on-disk v4 types (read-only shims). |
| `diff.go` | Change detection: `CacheStale` (parser impact) / `ConfigStale` (template impact). |
| `ulid.go` | Monotonic Crockford-base32 ULID generator. |
| `raw_cache.go` | `WriteRawBody` (atomic) / `ReadRawBody` / `DeleteOrphans` for `bin/subscriptions/<id>.raw`. |
| `provider_announce.go` | Parsed announcement headers (SPEC 061): HWID-binding status, max-devices. |
| `adapter_source.go` | `Source → ProxySource` converter for legacy parser code. |
| `disk_v6.go` | On-disk v6 schema constants + private `diskStateV6` struct. |

### `core/snapshot`

**Responsibility:** Read-only aggregation of template + state + cache + config into one JSON blob for diagnostics / bug reports.
- `snapshot.go` — `Snapshot.Build`: iterate file specs, handle missing/invalid JSON, report `Files`/`Missing`/`Errors`.

### `core/build` — config-to-JSON pipeline (pure)

**Responsibility:** `BuildConfig` orchestrates per-section rendering via pure section
handlers + the `ResolveDNS`/`ResolveRoute`/`ExpandPreset` resolvers.

| File | Purpose |
|------|---------|
| `build.go` | `BuildConfig` orchestrator: validate template, `GetEffectiveConfig`, dispatch per section, concat final JSON (pure). |
| `sections.go` | `BuildOutboundsSection`/`BuildEndpointsSection` — render outbounds/endpoints with parser markers + preview truncation. |
| `format.go` | Indentation + JSON formatting helpers (`Indent`, `FormatSectionJSON`). |
| `dns_merge.go` | `MergeDNSSection` — overlay DNS servers/rules/final/strategy onto template DNS; strip wizard-only fields. |
| `route_merge.go` | `MergeRouteSection` — append enabled custom-rules + SRS rule_sets; remote→local rule_set conversion. |
| `preset_merge.go` | `MergePresetsIntoRoute`/`MergePresetsIntoDNS` (SPEC 053) — second pass appending preset-bundled fragments via the resolvers; dedup by tag. |
| `preset_expand.go` | `ExpandPreset` — substitute `@vars`, eval `if`/`if_or`, prefix tags, clean dangling rule_set refs → `PresetFragments`. (Unified `evalIf` in SPEC 070 Stage 3b.) |
| `preset_outbounds.go` | Preset outbound expansion (add/update modes, SPEC 055/057). |
| `resolve_dns.go` | `ResolveDNS` pure resolver: unify template/preset/user DNS with Active/Enabled/Locked metadata. |
| `resolve_route.go` | `ResolveRoute` pure resolver: unify preset/inline/srs routing from `state.Rules`. |
| `resolve_outbounds.go` | Per-entry outbound resolver (`Ref`/`Updates` merge, SPEC 057/058). |
| `rules_pipeline.go` | Rule-order pipeline helpers shared by route resolution. |
| `sync_outbounds.go` | `SyncOutboundsWithActivePresets` (adopt-on-first-sync; preset-bound outbound lifecycle). |
| `migrate_outbounds_spec058.go` | `MigrateOutboundsToReferencedShape` one-shot v5→v6 outbound migration. |
| `outbound_diff.go` | `OutboundFieldDiff` helper (USER patch computation for Edit dialog). |
| `parsed_cache.go` | In-memory parsed-node cache used by build. |
| `secrets.go` | Materialize/redact secret fields during build. |

### `core/config` — subscription→outbounds generation + config readers

**Responsibility:** Orchestrate the subscription fetch→parse→outbounds-JSON pipeline (three-pass validity + selector filtering); read live `config.json`.

| File | Purpose |
|------|---------|
| `outbound_generator.go` | `GenerateOutboundsFromParserConfig` orchestrator + `GenerateNodeJSON`/`GenerateEndpointJSON`/`GenerateSelectorWithFilteredAddOutbounds` (thinned after SPEC 070 splits). |
| `outbound_validity.go` | Three-pass algorithm: `buildOutboundsInfo` → `computeOutboundValidity` (topological sort, cycle-detect) → `generateSelectorJSONs`. |
| `outbound_jsonbuilder.go` | `JSONBuilder{parts}` with insertion-order-safe `AppendField` (replaces `fmt.Sprintf`+`strings.Join`). |
| `outbound_filter.go` | Node filtering for selectors (`filterNodesForSelector`, `FilterNodesExcludeFromGlobal`, expose synthetic node, preview helpers). |
| `outbound_share.go` | Share-URI lookup from a written `config.json` (`GetOutboundMapByTag`, `ShareProxyURIForOutboundTag`). |
| `config_loader.go` | Read `config.json` (JSONC-aware): selector groups, TUN interface name, `experimental.cache_file`. |
| `varsubst.go` | `SubstituteParserConfigPlaceholders` — resolve `@name` placeholders in outbound options (template defaults + state override). |
| `models.go` | Type aliases → `configtypes` for backward-compat (`config.ParsedNode`, etc.). |

### `core/config/configtypes`

**Responsibility:** Shared parser types in a separate package to break a circular import (`subscription` imports `configtypes`, not `config`).
- `types.go` — `ParserConfig`, `ProxySource`, `OutboundConfig`, `ParsedNode`, `NormalizeParserConfig`.
- `matcher.go` — pattern/filter matching (`MatchesPattern`).

### `core/config/parser`

**Responsibility:** Extract/normalize the `ParserConfig` block from `config.json`.
- `factory.go` — `ExtractParserConfig`, `NormalizeParserConfig`, duplicate-tag stats.

### `core/config/subscription` — protocol parsers + fetch + encode

**Responsibility:** Per-protocol URI parsers + share-URI encoders (VLESS/VMess/Trojan/SS/Hysteria2/TUIC/SSH/SOCKS/Naive/WireGuard, plus Amnezia `vpn://`) and subscription transport (fetch + decode + metadata).

| File | Purpose |
|------|---------|
| `source_loader.go` | `LoadNodesFromSource` entry point: fetch → format-detect → parse → tag prefix/postfix/mask + skip-filter + dedup; `LookupCachedBody` offline hook. |
| `fetcher.go` | `FetchSubscriptionWithMeta` HTTP fetch (HWID/UA headers, 10 MB cap) + announce-header decode; deprecated `FetchSubscription` wrapper. |
| `meta.go` | Header + inline-`#comment` metadata parsing (Profile-Title, Subscription-Userinfo, update interval), provider-announce on empty body. |
| `decoder.go` | `DecodeSubscriptionContent` (base64 / Xray JSON array detection). |
| `node_parser_core.go` | `ParseNode` dispatcher + common helpers (`extractTagAndComment`, `generateDefaultTag`, `buildOutbound`, `IsDirectLink`). |
| `node_parser_transport.go` | VLESS/Trojan transport + TLS from URI query (`uriTransportFromQuery`, `vlessTLSFromNode`, `trojanTLSFromNode`, `queryGetFold`). |
| `node_parser_vmess.go` | VMess payload (JSON + legacy cleartext) + transports. |
| `node_parser_ss.go` | Shadowsocks (SIP002 + legacy). |
| `node_parser_ssh.go` | SSH. |
| `node_parser_hysteria2.go` | Hysteria2 (+ `hysteria2_ports.go` for mport ranges). |
| `node_parser_naive.go` | Naive. |
| `node_parser_tuic.go` | TUIC v5 (SPEC 074). |
| `node_parser_wireguard.go` | WireGuard + AmneziaWG 2.0 promoted fields (`applyAWGFields`, ranged `h1`–`h4` via `parseAWGHeaderRange`, overlap warning, AWG MTU clamp — SPEC 073/073.2). |
| `node_parser_amnezia.go` | Amnezia `vpn://` profile import: base64url + qCompress decode → WG/AWG container (`last_config`) → canonical `wireguard://` URI (SPEC 075). |
| `wgconf_text.go` | Pasted `[Interface]/[Peer]` conf text → `wireguard://` URIs (`ExtractWGConfBlocks` / `ConvertWGConfText`, SPEC 076). |
| `xray_json_array.go` / `xray_outbound_convert.go` | Xray JSON-array parsing: element → `ParsedNode` (+ jump hop), `remarks`→Label, slug tags; stream-settings→transport/TLS. |
| `share_uri.go` | `ShareURIFromOutbound` dispatcher (reverse of `ParseNode`). |
| `shareuri_vless.go` / `shareuri_vmess.go` / `shareuri_trojan.go` / `shareuri_ss.go` / `shareuri_socks.go` / `shareuri_hysteria2.go` / `shareuri_ssh.go` / `shareuri_tuic.go` / `shareuri_naive.go` / `shareuri_wireguard.go` | Per-protocol outbound→share-URI encoders. |
| `shareuri_helpers.go` | Shared encode helpers (`mapGet*`, `transportToQuery`, TLS-to-query, ALPN/insecure). |
| `utf8_utils.go` | Consolidated UTF-8 validate/repair (`FixUTF8*`, `HasControlChars`) — SPEC 070 dedup. |
| `encoding_utils.go` | Consolidated multi-variant base64 decode — SPEC 070 dedup. |

### `core/template` — template load + presets + expression language

**Responsibility:** Load `wizard_template.json`, apply per-platform params, extract presets (SPEC 053), `@var`/`#if` substitution (SPEC 067), validation.

| File | Purpose |
|------|---------|
| `loader.go` | `LoadTemplateData`: read + validate template, apply params by GOOS, extract presets, return `TemplateData`. |
| `template_validate.go` | `ValidateWizardTemplate` (uniqueness, refs, `#if` body, outer `@`-only). |
| `substitute.go` | `SubstituteVarsInJSON`: recursive `@var` substitution + `#if` walker (map-spread / array-element), runtime globals `@runtime.platform`/`@runtime.arch`. |
| `ifexpr.go` | `#if` predicate evaluation forms (equality, `#in`/`#matches`/`#not`, AND/OR short-circuit). |
| `vars_resolve.go` / `vars_default.go` | Var resolution (`VarAppliesOnGOOS`, `ParamBoolVarTrue`) + object `default_value` selection (GOOS/win7/default). |
| `preset_loader.go` / `preset_types.go` / `preset_outbounds.go` | Preset parsing + types (rules / dns / outbounds / vars). |
| `preset_lite.go` | Adapter implementing the `state.PresetLite` interface (the interface itself lives in `core/state` to break the import cycle) + `PresetLiteMap` for `state.SyncDNSOptionsWithActivePresets`. |
| `rule_utils.go` | Rule helpers (`HasOutbound`, `GetDefaultOutbound`, `CloneRuleRaw`). |

---

## L3 — services + lifecycle

### `core/services`

**Responsibility:** Stateful service implementations decoupled from `AppController`, Fyne-free.

| File | Purpose |
|------|---------|
| `file_service.go` | Paths (`ExecDir`, `ConfigPath`, `SingboxPath`, `WintunPath`), log open/close/rotation, backups. |
| `api_service.go` | Clash API integration + proxy-list / active-proxy state + last-ping-error storage. |
| `state_service.go` | Dirty markers (`MarkConfigStale`/`ClearCacheStale`), settings (auto-update enabled, cached launcher version); publishes `StateChanged`. |
| `srs_downloader.go` | Remote rule-set (.srs) HTTP fetch + group download for the Rules tab. |

### `core/uiservice`

**Responsibility:** UI state + callback container in a separate package so `core/services` can avoid importing Fyne.
- `ui_service.go` — singleton windows (wizard, settings), tray-menu state, icon resources, callback fields (`UpdateCoreStatusFunc`, `UpdateConfigStatusFunc`, `RefreshAPIFunc`, `ShowSubsResultFunc`, …, `FocusOpenChildWindows`); no callback *implementations*.

### `core/events`

**Responsibility:** Typed synchronous EventBus (SPEC 047) — see ARCHITECTURE.md §4.
- `events.go` — `EventKind` enum (3 kinds) + `Event`/`Handler`/`Cancel` + `Bus` interface.
- `payloads.go` — `StateChangedPayload`, `ConfigBuiltPayload`, `VpnStateChangedPayload`.
- `memory_bus.go` — `MemoryBus`: `RWMutex`-guarded handler map, panic-isolated sync `Publish`.

### `core` (app + process + config lifecycle)

**Responsibility:** App-lifecycle orchestration, process supervision, config update pipeline, downloaders. The DI wiring + EventBus owner.

| File | Purpose |
|------|---------|
| `controller.go` | `AppController` singleton: `NewAppController` (sole intended constructor) + `GetController`/`GetControllerOrPanic`; holds services + EventBus + UI callbacks; publishes `VpnStateChanged`; `GracefulExit`. (Still ~728 LOC; decomposition deferred — ADR-070-7.) |
| `process_service.go` | `ProcessService`: `Start`/`Stop`/`Monitor`, crash/restart state machine, privileged-script exit handling, TUN/phantom-adapter cleanup before Start (SPEC 065). |
| `config_service.go` | `ConfigService`: `RunParserProcess`, `UpdateConfigFromSubscriptions` (cache-refresh pipeline), `buildContextFromState`, per-source refresh. (Still ~1066 LOC; decomposition deferred.) |
| `rebuild.go` | `RebuildConfigIfDirty` — **sole `config.json` writer** (ADR-070-4); validate via `sing-box check`; publishes `ConfigBuilt`; `cleanupLegacyOutboundsCache`. |
| `rebuild_raw_cache.go` | `buildSnapshotFromRawCache` — rebuild from `.raw` bodies without network. |
| `auto_update.go` | SPEC 052 per-source event-driven auto-update: heartbeat loop, retry timers, subscribes `VpnStateChanged`. |
| `log_level.go` | Headless log-level apply (Load→mutate→Save). |
| `core_downloader.go` / `core_version.go` | sing-box download + version (pinned via `constants.RequiredCoreVersion`); launcher self-update check. |
| `wintun_downloader.go` | wintun.dll download (Windows). |
| `template_migration.go` | `InvalidateTemplateIfStale` — drop local template on launcher upgrade. |
| `tray_menu.go` | System-tray menu construction. |
| `network_utils.go` | Shared HTTP client + network-error classification + URL redaction. |
| `error_handler.go` | Unified error-to-UI surface. |
| `debugapi_wiring.go` | Wire the Debug API `ControllerFacade` to `AppController`. |
| `main.go` | Entry point: `NewAppController`, template-stale check, locale load, UI init, power-resume registration. |

---

## L4 — api / remote-control

### `api` — Clash API client

**Responsibility:** Outbound Clash API HTTP client (proxy list / switch / delay). Split per concern in SPEC 070.

| File | Purpose |
|------|---------|
| `clash_config.go` | `LoadClashAPIConfig` (base URL + token from config). |
| `clash_transport.go` | HTTP client lifecycle + `ResetClashHTTPTransport` (power resume). |
| `clash_log.go` | `api.log` sink/file + `writeLog` timestamping. |
| `clash_error.go` | `classifyRequestError`/`normalizeRequestError` (net-error → user message, sleep-aware). |
| `clash_proxy.go` | `GetProxiesInGroup` + `ProxyInfo` (`Name` raw vs `DisplayName` normalized, `DisplayOrName`). |
| `clash_switch.go` | `SwitchProxy` (PUT, path-escaped group + JSON name). |
| `clash_delay.go` | `GetDelay` + `TestAPIConnection` + ping-test URL/concurrency config. |
| `clash.go` | Remaining shared declarations / package wiring. |

### `core/debugapi` — inbound Debug HTTP API

**Responsibility:** Introspect/control the app via a `ControllerFacade` interface (no concrete upward import).

| File | Purpose |
|------|---------|
| `server.go` | HTTP server scaffold: Bearer auth, route table, facade to `AppController`. |
| `state_endpoints.go` | `/state/*` read + atomic-mutation endpoints (rules, DNS rules, log level). |
| `settings_endpoints.go` | `/settings/*` read/write. |
| `traffic_endpoints.go` | `/traffic/*` (status/live/sessions) wrapping `internal/traffic` (SPEC 059). |
| `snapshot.go` | `/snapshot` endpoint wrapping `core/snapshot.Build`. |

---

## L5 — ui-presentation (configurator MVP)

### `ui/configurator/models` — pure data

**Responsibility:** `WizardModel` (canonical Sources/GlobalOutbounds/Defaults) + rule/DNS slot ordering + preset-ref state + persisted state-file schema. No GUI deps.

| File | Purpose |
|------|---------|
| `wizard_model.go` | Central model: Sources (v5-canonical), GlobalOutbounds, Defaults; derived `ParserConfig`/JSON views; `TemplateData`, `RuleOrder`, `DNSRuleOrder`; outbound memo. |
| `wizard_state_file.go` | Persisted v6 state schema + migrations (`MigrateCustomRules`, `MigrateSelectableRuleStates`) + `ValidateStateID` + `NewWizardStateFile` factory. |
| `rule_slot.go` / `dns_rule_slot.go` | Rule / DNS-rule ordering containers (interleave preset + user rows per order). |
| `rule_state.go` / `rule_state_utils.go` | Routing-rule state + effective-outbound helpers. |
| `preset_ref_state.go` / `preset_ref_sync.go` | Preset-ref lifecycle (`SyncDNSByOrderToState`, `ReconcileDNSRuleOrder`, kind=preset vs user). |
| `dns_state.go` / `dns_user_rule.go` / `dns_vars.go` | DNS model state + user-rule typed struct (text↔JSON) + DNS vars. |
| `wizard_settings_migrate.go` | Migrate settings vars from legacy `ConfigParams`. |

### `ui/configurator/business` — pure logic (Fyne-free)

**Responsibility:** ParserConfig parsing, DNS/outbound schema, template interpolation, state lifecycle, validation — behind the `UIUpdater` interface; never imports Fyne or presentation.

| File | Purpose |
|------|---------|
| `parser.go` | `ParseAndPreview`, `ApplyURLToParserConfig` (classify lines, build ProxySource, restore tag prefix/postfix). |
| `create_config.go` | `BuildPreviewConfig`/`BuildTemplateConfig`, route-config-from-model, secret materialization (preview vs production). |
| `wizard_dns.go` | DNS public API (`ApplyWizardDNSTemplate`, `LoadPersistedWizardDNS`, `DNSTagLocked`, `DNSTagFromTemplate`). |
| `reconcilers.go` | DNS server reconciliation (extracted from `wizard_dns.go`, SPEC 070 Stage F). |
| `fillers.go` | DNS auxiliary fill / pick defaults (extracted, SPEC 070 Stage F). |
| `validators.go` | DNS validation (`ValidateDNSModel`) (extracted, SPEC 070 Stage F). |
| `dns_helpers.go` | Hoisted `parseTemplateDNSOptions`/`extractTemplateDNSTags` (kills the import-cycle dup). |
| `template_helpers.go` | Merged `effectiveTemplate` (was `effectiveWizardConfig` + `effectiveTemplateConfig`). |
| `dns_settings_vars.go` / `preset_bundled_dns.go` | DNS scalar vars (strategy/final/resolver) + preset-bundled DNS materialization. |
| `outbound.go` | `GetAvailableOutbounds` (memoized), `ResolveMergedOutbound`, default-tag enforcement. |
| `source_local_wizard.go` | Local `WIZARD:` group markers (auto/selector), expose flags, tag rename on prefix change. |
| `sources.go` | Source list helpers. |
| `validator.go` | Input validation (ParserConfig/URL/URI/outbounds/JSON size). |
| `loader.go` | `LoadConfigFromFile` (config.json preferred, template fallback) + `EnsureRequiredOutbounds`. |
| `state_store.go` | Thin wrapper over `core/state.Save`/`Load` (atomic). |
| `preview_cache.go` | Preview outbound cache + `SourceIndex` assignment. |
| `interfaces.go` / `ui_updater.go` | `UIUpdater` interface (the business↔presentation bridge). |
| `config_service.go` / `template_loader.go` / `file_service_adapter.go` | Adapters to `core` services (`ConfigService`, `TemplateLoader`, `FileService`). |

### `ui/configurator/presentation` — MVP presenter (orchestration)

**Responsibility:** Syncs GUI ↔ model via `WizardPresenter`; `fyne.Do` dispatch; save/load/async orchestration.

| File | Purpose |
|------|---------|
| `presenter.go` | `WizardPresenter` base + `SafeFyneDo` + child-window slots + DI for rules-tab recreation. |
| `gui_state.go` | Pure GUI state container (Fyne widgets only) + `ChildWindowsOverlay` + `RuleWidget`. |
| `presenter_sync.go` | Bidirectional model↔GUI sync (`SyncModelToGUI`/`SyncGUIToModel`/`MergeGUIToModel`) with stale-read guards. |
| `presenter_state.go` | `CreateStateFromModel`, `SaveCurrentState`/`SaveStateAs`, `LoadState` (9-step recovery), rule/DNS order sync. |
| `presenter_state_helpers.go` | Restore helpers extracted from `presenter_state.go` (SPEC 070 Stage F). |
| `presenter_save.go` | Save pipeline (state-only post-SPEC-045): validate → GUI→model → `state.Save` → publishes `StateChanged` → auto-rebuild → success dialog. |
| `presenter_methods.go` | `SetSaveState`, `RefreshOutboundOptions` (+ debounced ~300 ms), `InitializeTemplateState`. |
| `presenter_async.go` | `TriggerParseForPreview`, `UpdateTemplatePreviewAsync`. |
| `presenter_rules.go` | Rules-tab refresh (incl. recreate-after-LoadState via DI). |
| `presenter_ui_updater.go` | `UIUpdater` implementation (the bridge from business). |
| `preset_ref_helpers.go` | `extractTemplateDNSTags` (template DNS tag discrimination for `CreateStateFromModel`). |

### `ui/configurator/configurator.go` + `ui/configurator/utils`

- `configurator.go` — `ShowConfigWizard` entry: singleton check, model/GUI/presenter creation, template load, state.json→config.json fallback, window lifecycle.
- `utils/comparison.go`, `utils/constants.go`, `utils/truncate.go` — struct comparison, timeout/limit constants, text truncation.

---

## L6 — ui-views (tabs / dialogs / root)

### `ui` (root tabs + main views)

**Responsibility:** Root tab strip + main tabs (Core dashboard, Clash API, diagnostics, settings, help) + remote Clash API resolver + traffic bootstrap.

| File | Purpose |
|------|---------|
| `app.go` | Root tab container (Core / Servers / Diagnostics / Settings button-tab / Help); subscribes `VpnStateChanged`. |
| `core_dashboard_tab.go` | Core dashboard: sing-box status, version/download/wintun blocks, config status, state selector, subscription toast panel. |
| `core_dashboard_tab_helpers.go` / `_status.go` / `core_dashboard_subs_status.go` | Dashboard sub-builders + status updaters + subscription-status panel (SPEC 070 split). |
| `clash_api_tab.go` | Servers tab: proxy list with sort/filter/ping, active-proxy tracking, selector dropdown, share-URI export. (Still ~1266 LOC; decomposition deferred.) |
| `clash_api_tab_helpers.go` / `_render.go` | Servers-tab helpers + list rendering (SPEC 070 split). |
| `clash_remote.go` / `clash_remote_ui.go` | SPEC 064 remote Clash API endpoint resolver + dialog. |
| `diagnostics_tab.go` | STUN/DNS tests, sing-box panic kill, settings persistence. |
| `settings_tab.go` / `settings_window.go` | Settings UI (language, log level, …) in standalone window. |
| `help_tab.go` | Help tab. |
| `log_viewer_window.go` | In-app log viewer (reads the debuglog sink). |
| `dialogs.go` | Common dialogs (`ShowError`/`ShowInfo`/`ShowConfirm`/`ShowCustom`) with `fyne.Do`. |
| `traffic_bootstrap.go` / `traffic_verbose.go` | Wire/open the Traffic Profiler window + verbose toggle. |
| `wizard_overlay.go` | `wizardOverlayEnabled` constant + main-window click overlay flip. |

### `ui/configurator/tabs`

**Responsibility:** Wizard tab views (Sources / Outbounds / Rules / DNS / Settings / Preview) + source-edit window.

| File | Purpose |
|------|---------|
| `source_tab.go` | Sources tab: URL input, source list, preview-all window launcher. |
| `source_edit_window.go` + `source_edit_overview.go` / `_raw.go` / `_misc.go` | Per-source edit window (settings / preview / raw JSON; exclude/expose markers). |
| `source_meta_format.go` / `source_support_link.go` / `source_error_dialog.go` | Source metadata formatting + support/web-page link + error dialog. |
| `rules_tab.go` / `rules_unified_rows.go` | Routing rules list (add/edit/delete, SRS auto-download, per-rule outbound select). |
| `dns_tab.go` / `dns_unified_rules.go` / `dns_user_rules.go` / `dns_preset_bundled.go` | DNS servers + unified rules editor (preset + user). |
| `settings_tab.go` + `settings_tun_darwin.go` / `settings_tun_stub.go` | Template-vars settings; darwin TUN-off privileged cleanup. |
| `preset_ref_edit_dialog.go` / `preset_ref_convert.go` / `preset_ref_srs.go` | Preset-ref edit/convert/SRS handling. |
| `library_rules_dialog.go` | Template-preset library picker (Add selected → CustomRules). |
| `preview_tab.go` | Config preview tab. |
| `tight_vbox.go` / `tight_hbox.go` | Compact vbox/hbox layout helpers (`tight_hbox.go` packs row icons with a negative gap, `rowIconGap`). |
| `row_scaffold.go` | Shared row scaffolding for the reorderable Rules/DNS/Sources lists: `buildRowLeftLead` (↑/↓ + checkbox), `buildRowEditDelCluster` (edit/delete icons), `newRowLabelToggleTap` (toggle on label tap), `finalizeRow` (Border + HoverRow + tooltip-hover assembly). Unifies icon spacing across builders so it can't drift per-builder. |

### `ui/configurator/dialogs`

| File | Purpose |
|------|---------|
| `add_rule_dialog.go` | Add/edit routing-rule modal (form + raw JSON tabs, process picker, SRS URL list). (Still ~1146 LOC; decomposition deferred.) |
| `rule_dialog.go` / `rule_type_selection.go` | Rule-type picker + helpers. |
| `load_state_dialog.go` / `save_state_dialog.go` | Wizard state load/save dialogs. |
| `get_free_dialog.go` | Get-free-VPN config import dialog. |
| `srs_tag.go` | SRS tag helper for rule dialogs. |

### `ui/configurator/outbounds_configurator`

| File | Purpose |
|------|---------|
| `configurator.go` / `configurator_helpers.go` | Outbound list UI (add/edit/delete/reset, global vs source scope, preset-ref badge). |
| `edit_dialog.go` / `edit_dialog_helpers.go` | Outbound create/edit window (tag/type/comment, urltest options, scope selector, ref+preset merge). (Still ~975 LOC; decomposition deferred.) |
| `flag_picker.go` | Country-flag picker. |

### `ui/traffic`

| File | Purpose |
|------|---------|
| `window.go` | Traffic Profiler window coordinator (layout, live + per-process tabs, toolbar). |
| `live_view.go` | Packet list (pause/resume, sort, click-to-detail). |
| `per_process_view.go` | Process-grouped aggregation. |
| `event_detail.go` | Expanded event detail panel. |
| `process_picker.go` / `toolbar.go` | Process selection + toolbar filters/sorting. |

---

## L7 — ui-widgets / assets

### `internal/fynewidget`

**Responsibility:** Reusable, self-contained Fyne building blocks. No internal deps.

| File | Purpose |
|------|---------|
| `hover_row.go` | `HoverRow`: hover background + optional selection tint; `WireTooltipLabelHover`. |
| `check_with_content.go` | `CheckWithContent`: checkbox + content that toggles the check on tap. |
| `hover_forward.go` | `HoverForwardButton`/`Select`/`TTButton`: forward child hover to the parent row. |
| `tap_wrap.go` / `secondary_tap_wrap.go` | Tap / secondary-tap (context-menu) wrappers with modifier capture. |
| `tooltip.go` | `SetToolTipSafe` helper (consolidated tooltip type-assertion, SPEC 070 dedup). |
| `doc.go` | Package doc: embedding rules, hover-forwarding patterns. |

### `ui/icons`

**Responsibility:** Embedded SVG icon resources.
- `icons.go` — `bolt.svg` (Core tab), `telegram.svg`, `link.svg` as Fyne static resources.

### `ui/components`

**Responsibility:** Shared UI components.
- `scroll_gutter.go` — `NewScrollGutter` / `WrapInScrollWithGutter` (scrollbar spacing).
- `click_redirect.go` — `ClickRedirect` overlay forwarding clicks to the wizard window for focus elevation. **(Layering violation V1: imports `core`.)**
