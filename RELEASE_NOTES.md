# Release Notes
See [docs/release_notes/0-7-0.md](docs/release_notes/0-7-0.md) for detailed release notes 

## Testing
- Validate FREE VPN feature (dialog, links insertion)

## CI / CD
- Updated CI/CD logic: added cross-platform Go build cache (GOCACHE and module cache), skipped `go mod tidy` in CI, and standardized action versions (`actions/checkout@v6`, `actions/setup-go@v6`) to speed up and stabilize builds.
- Added Windows-friendly cache paths for Go build and modules as per `actions/cache` examples.
- Updated golangci-lint in CI to v2.8.0; if CI reports config errors, run `golangci-lint migrate` to update `.golangci.yaml` or pin the action to a compatible version.

## Local caching
- Persist local Go build cache and module downloads between runs to speed up iterative development and CI previews. The repo now favors local cache directories for developer workflows; CI still uses actions/cache for reproducible caching across runners.

## macOS
- Added "Hide app from Dock" feature: user can toggle hiding the app from the Dock. When hidden, the app continues running in the tray; opening the app restores the Dock icon. Implementation uses a darwin-specific CGO helper with safe non-darwin stubs.
https://github.com/Leadaxe/singbox-launcher/pull/23 thnx https://github.com/MustDie-green

## Linting
- Fixed multiple `golangci-lint` issues across the codebase (typecheck/import errors, platform stubs, and formatting), improving CI lint pass reliability.



