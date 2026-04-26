# TASKS 046 — PINNED_CORE_AND_TEMPLATE

Чеклист по слоям из PLAN.md. Каждый слой — отдельный коммит, после каждого `go build ./... && go test ./... && go vet ./...` должны быть зелёные.

---

## Слой 1 — константы и CI-инжекция

- [ ] `internal/constants/constants.go`: добавить `const RequiredCoreVersion = "1.13.11"` (актуальный QA-версии sing-box на момент имплементации).
- [ ] `internal/constants/constants.go`: добавить `var RequiredTemplateRef = "<актуальный SHA>"` — перед коммитом сделать `git fetch origin main && git rev-parse origin/main`, подставить полный 40-символьный SHA. Перезатирается ldflags в CI.
- [ ] `internal/constants/constants_test.go`: minimal sanity-test (формат версии).
- [ ] `build/build_darwin.sh`: объявить `TEMPLATE_REF=$(git rev-parse HEAD)`, добавить `-X ...RequiredTemplateRef=$TEMPLATE_REF` во все `go build`.
- [ ] `build/build_linux.sh`: то же.
- [ ] `build/build_windows.bat`: то же (cmd-синтаксис).
- [ ] Smoke-тест: `bash build/build_darwin.sh` → запустить бинарник с `--debug` и убедиться, что `debuglog` фиксирует ref'у в URL шаблона.
- [ ] Коммит: `chore(constants): introduce RequiredCoreVersion and RequiredTemplateRef ldflags var`.

## Слой 2 — pin core, удаление latest-логики

- [ ] `core/core_downloader.go::DownloadCore`: пустой `version` → подставить `RequiredCoreVersion`.
- [ ] `core/core_version.go`: удалить `GetLatestCoreVersion`, `getLatestVersionFromURL` (для core), `CheckVersionInBackground` для core, `ShouldCheckVersion`, `FallbackVersion`.
- [ ] `core/core_version.go`: оставить только launcher-вариант проверки.
- [ ] `core/services/state_service.go`: убрать поля `VersionCheckCache`, `VersionCheckCacheTime`, `VersionCheckMutex`, `VersionCheckInProgress` (для core; launcher-эквиваленты остаются).
- [ ] `ui/core_dashboard_tab.go::updateVersionInfoAsync`: новая логика кнопки (download / reinstall / hidden).
- [ ] `ui/core_dashboard_tab.go::startAutoUpdate`: удалить целиком (вместе с полем `stopAutoUpdate`).
- [ ] `ui/core_dashboard_tab.go::handleDownload`: `DownloadCore(ctx, "", ...)` (без manual version).
- [ ] `bin/locale/ru.json` / `internal/locale/en.json`: ключ `core.button_reinstall_version` (опционально — можно переиспользовать `button_download_version`).
- [ ] Тест на `DownloadCore` с пустой `version`.
- [ ] Ручной smoke: запустить с installed=Required → нет кнопки; запустить без бинарника → есть «Download v$Required»; запустить с installed≠Required (искусственно подменить файл) → есть «Reinstall v$Required», без HighImportance.
- [ ] Коммит: `feat(core): pin sing-box version, drop latest-update UI nudge`.

## Слой 3 — pin template ref + invalidation

- [ ] `ui/wizard/template/loader.go::GetTemplateURL`: использовать `RequiredTemplateRef`, fallback на `GetMyBranch()` для пустого ref / dev-сборки.
- [ ] `ui/wizard/template/loader_test.go`: тест на pinned ref vs dev-fallback.
- [ ] `internal/locale/settings.go::Settings`: добавить `LastTemplateLauncherVersion` поле.
- [ ] `internal/locale/settings.go`: добавить helper `MarkTemplateInstalled(binDir, appVersion)`.
- [ ] `core/template_migration.go` (new): `InvalidateTemplateIfStale(execDir)`.
- [ ] `core/template_migration_test.go` (new): покрыть 6 сценариев из PLAN §3.7.
- [ ] `main.go` или `core/controller.go::Init`: вызвать `InvalidateTemplateIfStale(execDir)` после `EnsureDirectories`.
- [ ] `ui/core_dashboard_tab.go::downloadConfigTemplate`: после успешной записи — `MarkTemplateInstalled`.
- [ ] Smoke-сценарий «миграция»:
  1. Подложить `bin/wizard_template.json` от старой версии.
  2. Прописать `last_template_launcher_version: v0.8.6` в `bin/settings.json`.
  3. Запустить лаунчер с `AppVersion=v0.8.7`.
  4. Убедиться: файл удалён, UI показывает «Download Template», после клика — `LastTemplateLauncherVersion = v0.8.7`.
- [ ] Коммит: `feat(template): pin to commit ref, invalidate on launcher upgrade`.

## Слой 4 — release process docs

- [ ] `docs/RELEASE_PROCESS.md`:
  - [ ] Новая секция §5 «Pinned dependencies» с описанием модели.
  - [ ] §1.1 — два новых чеклист-пункта (см. SPEC §4.1).
  - [ ] §1.5 — после `git push origin develop` (merge main → develop) бампить source-default `RequiredTemplateRef` на новый main HEAD, отдельным коммитом (см. SPEC §4.4).
  - [ ] §2.1 — пункт про core bump для pre-release.
  - [ ] §4 — обновить чеклист закрытия (см. SPEC §4.3).
- [ ] `docs/ARCHITECTURE.md`: абзац про invalidation pipeline в разделе про startup.
- [ ] `docs/release_notes/upcoming.md`: запись EN + RU + migration note.
- [ ] Коммит: `docs(release-process): document pinned dependencies` (либо собрать всю доку в один коммит).

## Финализация

- [ ] `go build ./...`, `go test ./...`, `go vet ./...` зелёные на всех платформах.
- [ ] `golangci-lint run` на изменённых пакетах.
- [ ] Запустить ручной end-to-end на macOS (как минимум): свежая установка → шаблон скачан с pinned commit; обновление с подменённым settings.json → invalidation сработала.
- [ ] `IMPLEMENTATION_REPORT.md`: что сделано, какие отклонения от PLAN, дата.
- [ ] Переименовать папку `SPECS/046-F-N-PINNED_CORE_AND_TEMPLATE` → `SPECS/046-F-C-PINNED_CORE_AND_TEMPLATE`.
- [ ] Обновить `SPECS/README.md` — добавить строку `046` в текущий список.
