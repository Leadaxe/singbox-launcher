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

- **Новый тип правил — "Processes"**: добавлен новый тип правил в Wizard — **Processes**. Особенности:
  - Селектор активных процессов в отдельном попапе (Refresh / +Add), поддерживает множественный выбор. ✅
  - В списке и в сохранении используются только имена процессов (без PID), элементы **дедуплицируются** и **сортируются по имени**. 🔧
  - Поле выбора процессов отображается примерно на 4 строки (как и другие многострочные поля) и показывает выбранные процессы с возможностью удаления по строкам.
  - Правило сохраняется под ключом `process_name` (константа `ProcessKey`) в структуре правила.

- **Новый тип правил — "Custom JSON"**: добавлен отдельный тип правила **Custom JSON**, который позволяет задавать произвольное правило в виде JSON. Особенности:
  - При выборе типа **Custom JSON** отображается многострочное поле для ввода/правки JSON; поле обязательно и должно содержать валидный JSON перед сохранением. ✅
  - При сохранении: JSON **обязательно должен быть объектом** (map). Этот объект используется как полное тело `Raw` правила (включая все ключи). `outbound` по-прежнему выставляется отдельно и не переопределяется пользовательским JSON.
  - Если введён **не‑объект** (массив, строка и т.д.) или некорректный JSON, показывается всплывающий диалог с ошибкой и сохранение отклоняется (ввод должен быть исправлен вручную). ⚠️
  - При редактировании: правило определяется как **Custom** если оно не содержит известных ключей (`ip_cidr`, `domain`, `domain_regex`, `process_name`) — в этом случае диалог откроется в режиме **Custom JSON** и подставит отформатированный JSON из `Raw`.

- **Рефакторинг: `internal/process`** — логика перечисления процессов вынесена в пакет `internal/process` (обёртка над `go-ps`), дубли ранее присутствовавших реализаций удалены, вызовы в `core` и UI обновлены для единого API (улучшена удобочитаемость и кросс-платформенность).

- **Валидация и UX**: кнопка **Add/Save** активна только при заполненном имени правила и наличии данных для выбранного типа (для `Processes` — хотя бы один выбранный процесс); при желании можно добавить явные подсказки о причине неактивной кнопки. 💡

- **Прочие правки**: удалены неиспользуемые импорты и исправлены сборочные ошибки после рефакторинга; сборка прошла успешно.

## Servers (Clash API)
- **Per-selector last proxy**: the app now remembers the last selected proxy _per selector group_ (instead of a single global value), and restores the previous proxy per group when available.
- **Selector → Active Outbound (live)**: added an info icon next to the selector dropdown that queries the Clash API and shows the current active outbound for each selector (format: `selector.tag → active_outbound`). Errors are shown inline per selector. Selector mapping icon ⇄
- **Cleanup**: removed unused config-based selector→outbounds helper and deprecated global last-proxy functions; introduced `SetLastSelectedProxyForGroup` / `GetLastSelectedProxyForGroup` for explicit per-group state.
- **UX**: the Servers tab status now displays the "Last used proxy" for the currently selected group.
