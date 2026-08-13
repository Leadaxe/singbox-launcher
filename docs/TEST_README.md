# singbox-launcher tests

**🌐 Language**: English | [Русский](TEST_README.ru.md)

How the project's test suite is laid out and how to run it: roughly **1130 tests
across 197 files**. Listing them by name here is pointless — such a list goes stale
faster than the document does; what follows is where things live and how to run them.

## Running the tests

### Centralized scripts (recommended)

```bash
./build/test_linux.sh      # Linux
./build/test_darwin.sh     # macOS
build\test_windows.bat     # Windows
```

The scripts **exclude the GUI packages** (`/ui/`, `fyne.io`), which need OpenGL on a
headless runner; they set up the environment (CGO, PATH, GCC on Windows), write a
full log into `temp/<platform>/`, and print the package list before starting.

Parameters (the same for all three):

| Parameter | Effect |
|---|---|
| `nopause` / `silent` | don't wait for a keypress at the end |
| `short` | short tests only |
| `run TestName` | a single test by name |
| trailing argument | package path (defaults to `./...`) |

```bash
./build/test_darwin.sh nopause ./core/config/subscription
./build/test_darwin.sh nopause run TestParseNode_VLESS ./core/config/subscription
```

### Directly via `go test`

```bash
go test ./core/... ./internal/... ./api/...     # without the GUI packages
go test ./core/config/subscription -v -run TestParseNode_VLESS
```

The GUI packages (`ui/...`) require `CGO_ENABLED=1` and a C compiler; on a headless
machine they fail at OpenGL initialization — which is exactly why the scripts skip
them. Running them locally only makes sense on a machine with a graphical
environment.

## Where things live

| Area | Files | Tests | Coverage |
|---|---:|---:|---|
| `core/config/subscription` | 34 | ~240 | per-protocol URI parsers, share-URI round-trips, subscription-body classification, Xray JSON, sing-box JSON import, deduplication |
| `core/config` (all of it, `subscription` included) | 53 | ~335 | plus outbound generation, node identity, the group contract, per-scheme emission |
| `core/build` | 20 | ~141 | config assembly: DNS/route resolvers, preset expansion, outbound sync, SRS filenames |
| `core/state` | 13 | ~72 | the state schema, v2–v6 load and migration, atomic save, legacy↔canonical adapters |
| `core/services` | 9 | ~48 | the remote-machine registry, Deploy resources, the proxy-operation transport, the SRS downloader |
| `core` (root) | — | — | core engines (`CoreBackend`), downloaders, rebuild from the raw cache, metadata refresh, integration tests |
| `internal` | 23 | ~79 | leaf utilities: locales, the traffic profiler, platform helpers, `wizardsync` |
| `ui` | 40 | ~181 | **excluded from the scripts** — wizard business logic, models, machine-row formatting |
| `api` | 3 | 4 | the Clash API client |

The invariants the suite exists to protect:

- **Emitter and parser travel in pairs.** A new scheme without an emission branch
  silently truncates a node to `{tag,type,server,server_port}` — every scheme has an
  emission test.
- **Share-URI round-trips.** node → `config.json` → share URI → node must not lose
  fields; `ErrShareURINotSupported` covers what cannot be encoded into a single URI
  (selectors, WireGuard with several peers, `detour` chains).
- **The config passes `sing-box check`.** Where the core is available, the generated
  config is validated by the real core rather than only compared against a golden file.
- **Locale completeness.** `TestAllKeysPresent` fails when a key exists in one
  language and is missing in another.

## Requirements

- **Go 1.25** or newer (`go.mod`). The legacy Win7 build pins Go 1.20 separately via
  `go.win7.mod` — its dependencies resolve differently, see [BUILD_WINDOWS.md](BUILD_WINDOWS.md).
- Dependencies downloaded (`go mod download`).
- For the GUI packages — `CGO_ENABLED=1` and GCC (MinGW-w64 / TDM-GCC on Windows).
  The Windows script looks for GCC in the usual places (`C:\msys64\mingw64\bin`) and
  adds it to PATH itself.

## Notes

- Tests are isolated: temporary files and directories are created under `t.TempDir()`;
  the shared per-run temp directory is `temp/<platform>/`, wiped at the start of a run.
- The `-count=1` flag in the scripts guarantees runs are not served from the result cache.
- No network needed: the integration tests (`core/integration_test.go`) work on
  subscription samples embedded in the code, not on live URLs.
