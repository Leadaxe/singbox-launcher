# Release Notes

See [docs/release_notes/0-7-0.md](docs/release_notes/0-7-0.md) for detailed release notes 

## Validate FREE VPN feature
With sincere thanks to [igareck](https://github.com/igareck) for the project [vpn-configs-for-russia](https://github.com/igareck/vpn-configs-for-russia),  
whose ideas and contributions provided valuable inspiration for this release.

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


## Fixes
- Fix: Config Wizard now properly removes deleted subscription URLs and direct links when editing input in the wizard. Previously removed lines could remain in the generated `ParserConfig`; this has been fixed to respect full and partial deletions and preserve existing settings only for matching entries.

# Unreleased (2026-01-12)

## UI
- **Wizard single-instance**: окно конфигурационного визарда теперь может быть открыто только в одном экземпляре — повторный вызов фокусирует существующее окно вместо создания нового.
- **Click-redirect overlay**: добавлен невидимый оверлей поверх основного окна, который при клике переводит фокус на открытый визард, чтобы предотвратить одновременное взаимодействие с основным окном.
- **Open from tray**: при выборе "Open" в трее главное окно всегда восстанавливается; если визард открыт — он отображается поверх главного окна и получает фокус.
- **Рефакторинг**: логика оверлея вынесена в `ui/components/ClickRedirect`, инициализация — в `ui/wizard_overlay.go`, а общее поведение интегрировано через `UIService` (метод `ShowMainWindowOrFocusWizard`).
- **Wizard**: automatic URL checking on input change with a 2s debounce; the manual **Check** button was removed to prevent excessive calls and improve UX.

- **Chat GPT button**: added a "Chat GPT" quick action in Sources tab that builds a URL-encoded prompt using the ParserConfig and the ParserConfig docs link and opens ChatGPT (also copies prompt to clipboard when the web interface cannot accept prompts directly).
