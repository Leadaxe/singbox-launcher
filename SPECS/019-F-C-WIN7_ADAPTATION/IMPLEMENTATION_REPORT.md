## IMPLEMENTATION REPORT — 019-F-C-WIN7_ADAPTATION

- **Status:** Completed
- **Date completed:** 2026-03-11

### 1. Summary

Адаптация лаунчера под Windows 7 (x86, legacy): зафиксированы версия `sing-box` (Win7LegacyVersion), шаблон визарда для Win7-сборки через те же секции **`windows`**, что и для Win64 (без отдельной метки **`win7`** в JSON; дефолт **`tun_stack`** — **`vars_resolve`**), восстановлена и стабилизирована сборка в CI (job `build-win7`), артефакт и release.

### 2. Implemented changes

**Спецификация и план:**
- Создана задача `SPECS/019-F-C-WIN7_ADAPTATION` (SPEC.md, PLAN.md, TASKS.md). Зафиксированы изменения: `core/core_downloader.go` (Win7LegacyVersion, ассеты), `loader.go` / `vars_resolve.go` и шаблон (актуально — см. SPEC и **`docs/CREATE_WIZARD_TEMPLATE.md`**).

**CI/CD под Win7 (основной объём работ):**
- Введён отдельный модуль для Win7: `go.win7.mod` с `go 1.20` и `golang.org/x/sys v0.25.0` (совместимость с Go 1.20; основной go.mod остаётся на Go 1.25 и x/sys v0.42).
- Job `build-win7` переведён на использование только Win7-модуля:
  - `go mod download "-modfile=go.win7.mod"` (кавычки для PowerShell);
  - удаление старого `go.win7.sum` перед загрузкой;
  - `go get "-modfile=go.win7.mod" ./...` для заполнения go.sum;
  - `go build -modfile=go.win7.mod ...` в шаге сборки в MSYS2;
  - `GOFLAGS: -mod=mod` на уровне job и в шаге сборки (MSYS2).
- Файл `go.win7.sum` не хранится в репозитории — генерируется на раннере при каждом запуске.
- Исправления по ходу: экшен setup-go в Win7 переведён на v6; для PowerShell аргумент `-modfile=go.win7.mod` передаётся в кавычках.
- Обновлены экшены артефактов в ci.yml: `actions/upload-artifact@v6`, `actions/download-artifact@v6` (Node 24); удалён глобальный `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24`.

**Результат CI:**
- Job `build-win7` успешно собирает `singbox-launcher-win7-32.exe`, артефакт `artifacts-windows-win7-32` загружается.
- Release job формирует `singbox-launcher-<version>-win7-32.zip` и включает его в список артефактов и install instructions.
- Ручной prerelease при коммите с изменённым `ci.yml` может не пушить тег из-за политики GitHub (workflows permission); сборки и артефакты при этом создаются корректно.

**Документация:**
- В `docs/release_notes/upcoming.md` добавлен пункт про Win7 CI (go.win7.mod, стабильная сборка и артефакт в release).

### 3. Tests & Checks

- [x] CI job `build-win7` успешно выполняется на GitHub Actions.
- [x] Артефакт `artifacts-windows-win7-32` создаётся и подтягивается в release.
- [x] Сборки Win64, macOS не затронуты.
- Локальные проверки `go build ./...`, `go test ./...`, `go vet ./...` выполняются по основному go.mod (GUI-пакеты исключены из тестов по CONSTITUTION).

### 4. Risks / Limitations

- Legacy-версия sing-box для Win7 может не поддерживать часть новых возможностей; это отражено в документации.
- Поддержка Win7 — режим совместимости. Ручной prerelease с коммита, меняющего workflow, не пушит тег без расширения прав (решение: не расширять права, при необходимости создавать тег вручную).

### 5. Notes

- Задача закрыта; папка переименована в `019-F-C-WIN7_ADAPTATION` (C = completed).
- Опционально на будущее: явная документация версии Win7LegacyVersion и источника ассетов; дополнительные тесты фильтрации платформ в визарде.
