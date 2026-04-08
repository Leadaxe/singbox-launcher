## TASKS — 019-F-C-WIN7_ADAPTATION

### A. Анализ и фиксация текущего состояния

- [x] Изучить CI/CD для Win7:
  - `.github/workflows/ci.yml` — job `build-win7`, release job и упаковка `singbox-launcher-<version>-win7-32.zip`.
  - `.github/workflows/README.md` и `SPECS/001-F-C-FEATURES_2025/2026-02-15-ci-cd-workflow.md` — общая схема, параметр `target`, артефакты.
- [x] Зафиксировать текущие изменения по Win7:
  - `core/core_downloader.go` — `Win7LegacyVersion`, выбор legacy-ассетов **386** (актуальное имя архива — в коде).
  - `ui/wizard/template/loader.go` — `matchesPlatform` по **GOOS**; без метки **`win7`** в JSON.
  - `ui/wizard/template/vars_default.go` / **`vars_resolve.go`** — **`default_value`**-объект (например **`tun_stack`**) для **windows/386**.
  - `docs/release_notes/upcoming.md` — актуальное описание Win7 и шаблона.
- [x] Создать Spec Kit для задачи: `SPECS/019-F-C-WIN7_ADAPTATION` (`SPEC.md`, `PLAN.md`, `TASKS.md`).

### B. Ядро sing-box для Win7

- [ ] Уточнить и задокументировать выбранную версию `Win7LegacyVersion`:
  - источник версии (релиз `sing-box`), причины выбора;
  - ограничения и поддержку функций относительно современных версий.
- [ ] Проверить логику downloader'а для Win7:
  - при `GOOS=windows` и `GOARCH=386` всегда используется `Win7LegacyVersion`;
  - для Win7 выбирается корректный ассет `windows-amd64-legacy-windows-7.zip`;
  - другие платформы не затронуты.
- [ ] При необходимости добавить/уточнить комментарии в `core/core_downloader.go` по Win7-режиму.

### C. Визард и шаблон под Win7

- [x] `matchesPlatform` в `loader.go`: Win7 (**windows/386**) матчит **`"windows"`** в шаблоне; отдельная **`win7`** в JSON не используется.
- [x] `bin/wizard_template.json`: один TUN-блок для **`windows`/`linux`**, без дублирующего **`params`** под **`win7`**.
- [x] `vars_resolve` + **`VarDefaultValue`**: платформенный **`default_value`**, тесты **`TestVarDefaultValueForPlatform_*`**, **`TestResolveTemplateVars_tunStackPerPlatformDefault`**.

### D. CI/CD для Win7

- [x] Проверить условия запуска job `build-win7`:
  - теги `v*`;
  - `workflow_dispatch` с `run_mode=build|prerelease` и `target` (пусто или содержит `Win7`).
- [x] Убедиться, что артефакт `artifacts-windows-win7-32` содержит `singbox-launcher-win7-32.exe` и корректно подтягивается в release:
  - zip `singbox-launcher-<version>-win7-32.zip` создаётся;
  - Win7-zip включён в список release-артефактов и install instructions.
- [x] При необходимости скорректировать `.github/workflows/README.md`/документацию, чтобы отразить текущее поведение Win7-сборки.

### E. Документация и релизные заметки

- [x] Актуализировать `docs/release_notes/upcoming.md` по Win7:
  - ядро `sing-box` (legacy-версия и ассеты);
  - поведение визарда на Win7-сборке (секции **`windows`**, **`tun_stack`** в **`vars_resolve`**);
  - особенности CI/CD и артефактов для Win7.
- [ ] При необходимости добавить краткое описание Win7-режима в основную документацию (`docs/`/README), с указанием ограничений.

### F. Проверка и завершение

- [x] Запустить локально проверки:
  - `go build ./...`
  - `go test ./...`
  - `go vet ./...`
- [x] Убедиться, что изменения не ломают сборки для Win64/macOS/Linux.
- [x] Обновить `IMPLEMENTATION_REPORT.md` для задачи 019-F-C-WIN7_ADAPTATION.

