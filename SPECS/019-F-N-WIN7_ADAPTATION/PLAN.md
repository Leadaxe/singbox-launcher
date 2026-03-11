## План реализации — 019-F-N-WIN7_ADAPTATION

### 1. Анализ текущего состояния Win7

1.1. Изучить:
- `.github/workflows/ci.yml` — job `build-win7`, блоки с упаковкой `singbox-launcher-<version>-win7-32.zip` и раздел install instructions для Win7.
- `.github/workflows/README.md` и `SPECS/001-F-C-FEATURES_2025/2026-02-15-ci-cd-workflow.md` — описание новой схемы CI/CD, параметра `target` и артефактов `artifacts-windows-win7-32`.
- `core/core_downloader.go` — логика выбора версии `sing-box` и ассетов для Win7.
- `ui/wizard/template/loader.go` и `bin/wizard_template.json` — фильтрация по `platforms`, поведение для `windows`/`win7`.

1.2. Зафиксировать в этом SPEC/PLAN, какие части уже реализованы (ядро, визард, CI) и какие ещё остаются задачами.

### 2. Ядро sing-box для Win7

2.1. **Версия и ассеты**
- Уточнить и зафиксировать в коде и документации, какая версия `sing-box` используется как `Win7LegacyVersion` и откуда берутся архивы (`windows-amd64-legacy-windows-7.zip`).
- Проверить, что для Win7 (GOOS=windows, GOARCH=386) downloader всегда использует именно legacy-ассет и не пытается скачать несовместимые билды.

2.2. **Безопасность и обратная совместимость**
- Убедиться, что логика Win7 не затрагивает остальные платформы (Win64, macOS, Linux) и не меняет их поведение.
- При необходимости добавить/уточнить комментарии в коде по Win7-режиму.

### 3. Визард и шаблон под Win7

3.1. **Фильтрация платформ**
- Проверить и при необходимости доработать `matchesPlatform` в `ui/wizard/template/loader.go`, чтобы:
  - при `GOOS=windows`, `GOARCH=386` платформа `win7` обрабатывалась вместе с `windows`;
  - для остальных платформ поведение не менялось.

3.2. **Шаблон `wizard_template.json`**
- Проверить, что в `bin/wizard_template.json` корректно размечены секции `params` и `selectable_rules` с `platforms`:
  - общие секции для `["windows"]`;
  - специфические секции для `["win7"]` (если нужны отдельные отличия).
- При необходимости добавить/скорректировать секции `platforms` так, чтобы Win7-сборка получала нужные опции без влияния на Win64.

3.3. **Тесты / проверки**
- Добавить/обновить юнит-тесты для логики фильтрации по платформам (если такие тесты уже есть рядом с визардом).
- Минимальные сценарии:
  - Win64 (GOOS=windows, amd64) — только `windows`;
  - Win7 (GOOS=windows, 386) — `windows` + `win7`;
  - macOS (darwin, enableTunForDarwin) — `darwin` + `darwin-tun`.

### 4. CI/CD под Win7

4.1. **Job `build-win7`**
- Проверить условия запуска job:
  - теги `v*` (stable);
  - `workflow_dispatch` с `run_mode=build|prerelease` и `target` пустым или содержащим `Win7`.
- Убедиться, что:
  - используется Go 1.20.x для совместимости с Win7;
  - правильно настраиваются `GOOS=windows`, `GOARCH=386`, `CGO_ENABLED=1`.

4.2. **Артефакты и release**
- Проверить, что:
  - артефакт `artifacts-windows-win7-32` содержит `singbox-launcher-win7-32.exe`;
  - release job упаковывает Win7 exe в `singbox-launcher-<version>-win7-32.zip`;
  - Win7-zip добавлен в список релизных артефактов и описан в install instructions (шаги для пользователя).

4.3. **Документация по CI/CD**
- При необходимости расширить `.github/workflows/README.md` или отдельный документ в `SPECS/001-F-C-FEATURES_2025` ссылкой на Win7-режим и его ограничения.

### 5. Документация и release notes

5.1. Обновить `docs/release_notes/upcoming.md`:
- Явно описать:
  - наличие отдельной Win7-сборки лаунчера (`singbox-launcher-<version>-win7-32.zip`);
  - использование legacy-версии `sing-box` для Win7;
  - особенности визарда на Win7 (совместное применение `windows`/`win7`).

5.2. При необходимости добавить краткое описание Win7-режима в основную документацию (`docs/`, README_RU/README_EN).

### 6. Проверка и Definition of Done

- `go build ./...`, `go test ./...`, `go vet ./...` — проходят локально.
- CI jobs (как минимум build-win7) успешно выполняются на GitHub Actions.
- Поведение лаунчера на Win7 и Win64 не конфликтует: Win7 использует legacy-режим, Win64 — обычный.
- Визард на Win7 корректно применяет нужные секции шаблона.
- Релизные заметки и документация отражают текущее состояние поддержки Win7.

