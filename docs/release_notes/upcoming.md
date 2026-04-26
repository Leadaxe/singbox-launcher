# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

---

## EN

### Highlights

- **Pinned sing-box and template** — each launcher build now ships pinned to a specific sing-box version (`RequiredCoreVersion`) and a specific commit of `wizard_template.json` (`RequiredTemplateRef`). The Core Dashboard no longer polls GitHub for newer sing-box releases or pushes "Update to vX.Y.Z" nudges; you upgrade the core by upgrading the launcher. On launcher upgrade, the locally cached template is invalidated and a fresh "Download Template" prompt appears so the format always matches the launcher version. See [SPEC 046](../../SPECS/046-F-N-PINNED_CORE_AND_TEMPLATE/SPEC.md).

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

- **Pinned-ядро и шаблон** — каждая сборка лаунчера теперь привязана к конкретной версии sing-box (`RequiredCoreVersion`) и конкретному коммиту `wizard_template.json` (`RequiredTemplateRef`). Core Dashboard больше не опрашивает GitHub в поисках свежих релизов sing-box и не показывает «Update to vX.Y.Z» — обновляется ядро только вместе с лаунчером. При апгрейде лаунчера локальный шаблон инвалидируется, появляется кнопка «Download Template» — чтобы формат всегда соответствовал версии лаунчера. Подробности — [SPEC 046](../../SPECS/046-F-N-PINNED_CORE_AND_TEMPLATE/SPEC.md).

### Техническое / Внутреннее

- `RequiredTemplateRef` инжектится на этапе сборки через `-ldflags` из `git rev-parse HEAD`; CI-сборки всегда pin'ятся на свой коммит. Локальный `go run .` берёт source-default, который maintainer бампит на `origin/main` после каждого релиза (RELEASE_PROCESS §1.5).
- Удалены: `GetLatestCoreVersion`, `CheckVersionInBackground`, `ShouldCheckVersion`, core-side `GetCachedVersion`/`SetCachedVersion`, `CheckForUpdates`, `CoreVersionInfo`, `FallbackVersion`. Self-update самого лаунчера не затронут.
- Новая функция `core.InvalidateTemplateIfStale(execDir)`, вызывается в `main.go` до построения UI.
- Новое поле `Settings.LastTemplateLauncherVersion` в `bin/settings.json`, записывается после успешного «Download Template».

### Миграция

- После апгрейда существующий `bin/wizard_template.json` будет удалён один раз при первом запуске; UI попросит перекачать. Один клик, никаких других действий.
- Если у вас был локально-модифицированный шаблон — забэкапьте его до апгрейда: для не-dev сборок инвалидация безусловная.
