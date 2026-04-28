# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

---

## EN

### Highlights

- **Connections redesign — `state.json` v5 + per-source meta** — the `state.json` layout has been redesigned: top-level `meta` (version, timestamps, comment) and `connections` (sources, outbounds, defaults) replace the legacy nested `parser_config` block. Each subscription source now persists its own metadata (`profile_title`, `subscription-userinfo` quota, expire date, `support-url`, last-fetched timestamp, last-status, error count, first-50 preview nodes) so the wizard can show quota progress, expiry badges, and per-source status without an extra refresh. Raw subscription bodies are cached at `bin/subscriptions/<id>.raw` (atomic .tmp + Rename) — Rebuild now parses `.raw` files directly without a network round-trip; `bin/outbounds.cache.json` is no longer used and is removed on first run after upgrade. New per-row **Refresh** button in the wizard triggers a fetch + meta + raw-cache update for a single subscription without disturbing others. Failed fetches keep the old `.raw` intact (per-source resilience). Old v2-v4 state files migrate automatically on first load. See [SPEC 052](../../SPECS/052-F-C-CONNECTIONS_REDESIGN/SPEC.md).
- **Pinned sing-box and template** — each launcher build now ships pinned to a specific sing-box version (`RequiredCoreVersion`) and a specific commit of `wizard_template.json` (`RequiredTemplateRef`). The Core Dashboard no longer polls GitHub for newer sing-box releases or pushes "Update to vX.Y.Z" nudges; you upgrade the core by upgrading the launcher. On launcher upgrade, the locally cached template is invalidated and a fresh "Download Template" prompt appears so the format always matches the launcher version. See [SPEC 046](../../SPECS/046-F-N-PINNED_CORE_AND_TEMPLATE/SPEC.md).
- **Auto-ping soft cap on huge subscriptions** — auto-ping after VPN connect (and after system resume) is now skipped when the proxy list exceeds **150** nodes. With CIDR-derived subscriptions of several hundred nodes, the timer-driven ping-all opened so many simultaneous TCP+TLS handshakes through TUN that game logins and other in-flight traffic could freeze for over a minute. The manual «Test» button on Servers, Cmd/Ctrl+P, and the `/action/ping-all` debug-API endpoint are unaffected — only the automatic timer path is gated. The threshold is tunable via `bin/settings.json` → `auto_ping_after_connect_max_proxies` (positive int; 0/absent = use the 150 default). See [SPEC 039 §1.3](../../SPECS/039-F-C-SETTINGS_TAB_PREFERENCES/SPEC.md).
- **`GET /debug/snapshot`** — new debug-API endpoint returning the four wizard-pipeline files (`wizard_template.json`, `wizard_states/state.json`, `outbounds.cache.json`, `config.json`) as one JSON object. Files that are missing or contain invalid JSON are reported via `missing[]` / `errors{}`; the rest of the snapshot is still produced. **No redaction**: bearer-auth on the endpoint already gates access, masking secrets from the legitimate caller would be theatre. Useful for golden-data capture, bug-repros, and future MCP wrappers. See [SPEC 038 SUB_SPEC_SNAPSHOT.md](../../SPECS/038-F-C-DEBUG_API/SUB_SPEC_SNAPSHOT.md).
- **State / Config decoupling** — Wizard "Save" now writes only `state.json` (the declarative source of truth); the actual sing-box `config.json` is rebuilt automatically when you press Update (subscriptions changed) or Restart (template/DNS/rules changed). Two independent dirty markers replace the previous single `*` indicator: Update button shows `*` when sources need re-fetching, Restart button shows `*🔄` when the running sing-box is using a stale config. New file `bin/outbounds.cache.json` snapshots the last successful subscription parse so Restart-only changes don't re-fetch the network. Internally the build pipeline is a clean leaf-package `core/build.BuildConfig(BuildContext) → Result` with byte-equal golden-test parity vs the legacy v0.8.8 path. See [SPEC 045](../../SPECS/045-F-N-STATE_CONFIG_DECOUPLING/SPEC.md). The old wizard package is now `ui/configurator/`; user-facing label is unchanged.

### Technical / Internal

- `RequiredTemplateRef` is injected at build time via `-ldflags` from `git rev-parse HEAD`; CI builds always pin to their own commit. Local `go run .` builds inherit a source-default that the maintainer bumps to `origin/main` after each release (RELEASE_PROCESS §1.5).
- Removed: `GetLatestCoreVersion`, `CheckVersionInBackground`, `ShouldCheckVersion`, core-side `GetCachedVersion`/`SetCachedVersion`, `CheckForUpdates`, `CoreVersionInfo`, `FallbackVersion`. Launcher self-update path is unchanged.
- New `core.InvalidateTemplateIfStale(execDir)` called from `main.go` before the UI starts.
- New `Settings.LastTemplateLauncherVersion` field in `bin/settings.json`, written after a successful Download Template.

### Migration notes

- After upgrading, your existing `bin/wizard_template.json` will be removed once on first launch and the UI will prompt to re-download. One click; no further action.
- If you ran a custom-modified template locally, back it up before upgrading — the invalidation is unconditional for non-dev builds.

---

## RU

### Основное

- **Connections redesign — `state.json` v5 + per-source meta** — формат `state.json` переделан: top-level `meta` (version, timestamps, comment) и `connections` (sources, outbounds, defaults) вместо старой вложенной обёртки `parser_config`. Каждая подписка теперь хранит собственные метаданные: `profile_title`, квота из `subscription-userinfo`, expire date, `support-url`, время последнего fetch, статус, счётчик ошибок, превью первых 50 нод — визард показывает progress-bar квоты, badge с днями до expire, индикатор статуса (●ok / ●err) без отдельного refresh. Raw-тела подписок кэшируются в `bin/subscriptions/<id>.raw` (atomic .tmp + Rename) — Rebuild теперь парсит `.raw` напрямую без network call'а; `bin/outbounds.cache.json` больше не используется и удаляется one-shot при первом запуске. Кнопка **Refresh** per-row в визарде делает fetch + meta + raw-cache одной подписки, не трогая остальные. Failed fetch сохраняет старый `.raw` (per-source resilience). Старые v2-v4 файлы автоматически мигрируются при первой загрузке. Подробности — [SPEC 052](../../SPECS/052-F-C-CONNECTIONS_REDESIGN/SPEC.md).
- **Pinned-ядро и шаблон** — каждая сборка лаунчера теперь привязана к конкретной версии sing-box (`RequiredCoreVersion`) и конкретному коммиту `wizard_template.json` (`RequiredTemplateRef`). Core Dashboard больше не опрашивает GitHub в поисках свежих релизов sing-box и не показывает «Update to vX.Y.Z» — обновляется ядро только вместе с лаунчером. При апгрейде лаунчера локальный шаблон инвалидируется, появляется кнопка «Download Template» — чтобы формат всегда соответствовал версии лаунчера. Подробности — [SPEC 046](../../SPECS/046-F-N-PINNED_CORE_AND_TEMPLATE/SPEC.md).
- **Soft-cap для авто-пинга на больших подписках** — авто-пинг через 5 секунд после connect'а (и после wake) теперь пропускается, если в списке нод больше **150**. На подписках от CIDR-парсеров с сотнями нод одновременный ping-all открывал столько TCP+TLS-хэндшейков через TUN, что игровые клиенты могли подвисать на минуту-две на экране логина. Ручная «Test» в Servers, Cmd/Ctrl+P и `/action/ping-all` debug-API — без cap'а, через них пингуется всё. Порог настраивается в `bin/settings.json` → `auto_ping_after_connect_max_proxies` (положительный int; `0` / отсутствует = дефолт 150). Подробности — [SPEC 039 §1.3](../../SPECS/039-F-C-SETTINGS_TAB_PREFERENCES/SPEC.md).
- **`GET /debug/snapshot`** — новый эндпоинт debug-API: возвращает JSON-объект с четырьмя файлами wizard-pipeline'а (`wizard_template.json`, `wizard_states/state.json`, `outbounds.cache.json`, `config.json`) разом. Отсутствующие или битые-как-JSON файлы попадают в `missing[]` / `errors{}` соответственно, не валя весь snapshot. **Без редакции**: bearer-токен на endpoint'е и есть trust-boundary, маскировать секреты от своего же запроса — security theatre. Удобно для захвата golden-данных, bug-репродов, будущих MCP-обёрток. Подробности — [SPEC 038 SUB_SPEC_SNAPSHOT.md](../../SPECS/038-F-C-DEBUG_API/SUB_SPEC_SNAPSHOT.md).
- **Разделение State / Config** — кнопка «Save» в визарде теперь пишет только `state.json` (декларативный source-of-truth); реальный `config.json` для sing-box пересобирается автоматически при нажатии Update (источники изменились) или Restart (шаблон/DNS/rules изменились). Один общий маркер `*` заменён на два независимых: Update показывает `*` когда нужны свежие подписки, Restart показывает `*🔄` когда работающий sing-box использует устаревший config. Новый файл `bin/outbounds.cache.json` хранит snapshot последнего успешного парса подписок, чтобы Restart-only изменения не дёргали сеть. Внутри build-пайплайн вынесен в leaf-пакет `core/build.BuildConfig(BuildContext) → Result` с byte-equal golden-тестом против legacy v0.8.8. Подробности — [SPEC 045](../../SPECS/045-F-N-STATE_CONFIG_DECOUPLING/SPEC.md). Wizard-пакет переименован в `ui/configurator/`; пользовательское название не изменилось.

### Техническое / Внутреннее

- `RequiredTemplateRef` инжектится на этапе сборки через `-ldflags` из `git rev-parse HEAD`; CI-сборки всегда pin'ятся на свой коммит. Локальный `go run .` берёт source-default, который maintainer бампит на `origin/main` после каждого релиза (RELEASE_PROCESS §1.5).
- Удалены: `GetLatestCoreVersion`, `CheckVersionInBackground`, `ShouldCheckVersion`, core-side `GetCachedVersion`/`SetCachedVersion`, `CheckForUpdates`, `CoreVersionInfo`, `FallbackVersion`. Self-update самого лаунчера не затронут.
- Новая функция `core.InvalidateTemplateIfStale(execDir)`, вызывается в `main.go` до построения UI.
- Новое поле `Settings.LastTemplateLauncherVersion` в `bin/settings.json`, записывается после успешного «Download Template».

### Миграция

- После апгрейда существующий `bin/wizard_template.json` будет удалён один раз при первом запуске; UI попросит перекачать. Один клик, никаких других действий.
- Если у вас был локально-модифицированный шаблон — забэкапьте его до апгрейда: для не-dev сборок инвалидация безусловная.
