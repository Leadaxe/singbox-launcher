## TASKS — 019-F-N-WIN7_ADAPTATION

### A. Анализ и фиксация текущего состояния

- [x] Изучить CI/CD для Win7:
  - `.github/workflows/ci.yml` — job `build-win7`, release job и упаковка `singbox-launcher-<version>-win7-32.zip`.
  - `.github/workflows/README.md` и `SPECS/001-F-C-FEATURES_2025/2026-02-15-ci-cd-workflow.md` — общая схема, параметр `target`, артефакты.
- [x] Зафиксировать текущие изменения по Win7:
  - `core/core_downloader.go` — `Win7LegacyVersion`, выбор ассетов `windows-amd64-legacy-windows-7.zip`.
  - `ui/wizard/template/loader.go` — поддержка `win7` в `matchesPlatform`.
  - `docs/release_notes/upcoming.md` — пункт про визард и платформу `win7`.
- [x] Создать Spec Kit для задачи: `SPECS/019-F-N-WIN7_ADAPTATION` (`SPEC.md`, `PLAN.md`, `TASKS.md`).

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

- [ ] Проверить `matchesPlatform` в `ui/wizard/template/loader.go`:
  - Win64 (amd64) — поведение без изменений;
  - Win7 (386) — матч `windows` + `win7`;
  - macOS darwin/darwin-tun — поведение без изменений.
- [ ] Аудит `bin/wizard_template.json`:
  - убедиться, что секции `params` и `selectable_rules` с `platforms` для Win7 размечены корректно;
  - при необходимости добавить/скорректировать секции `"platforms": ["win7"]` и общие `"platforms": ["windows"]`.
- [ ] (Опционально) Добавить/обновить тесты для платформенной фильтрации (если в пакете визарда есть тесты).

### D. CI/CD для Win7

- [ ] Проверить условия запуска job `build-win7`:
  - теги `v*`;
  - `workflow_dispatch` с `run_mode=build|prerelease` и `target` (пусто или содержит `Win7`).
- [ ] Убедиться, что артефакт `artifacts-windows-win7-32` содержит `singbox-launcher-win7-32.exe` и корректно подтягивается в release:
  - zip `singbox-launcher-<version>-win7-32.zip` создаётся;
  - Win7-zip включён в список release-артефактов и install instructions.
- [ ] При необходимости скорректировать `.github/workflows/README.md`/документацию, чтобы отразить текущее поведение Win7-сборки.

### E. Документация и релизные заметки

- [ ] Актуализировать `docs/release_notes/upcoming.md` по Win7:
  - ядро `sing-box` (legacy-версия и ассеты);
  - поведение визарда и платформ `windows`/`win7`;
  - особенности CI/CD и артефактов для Win7.
- [ ] При необходимости добавить краткое описание Win7-режима в основную документацию (`docs/`/README), с указанием ограничений.

### F. Проверка и завершение

- [ ] Запустить локально проверки:
  - `go build ./...`
  - `go test ./...`
  - `go vet ./...`
- [ ] Убедиться, что изменения не ломают сборки для Win64/macOS/Linux.
- [ ] Обновить `IMPLEMENTATION_REPORT.md` для задачи 019-F-N-WIN7_ADAPTATION.

