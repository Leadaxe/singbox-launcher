# PLAN 046 — PINNED_CORE_AND_TEMPLATE

**Статус:** черновик. Уточнить после ревью SPEC.md.

Реализация разбита на три слоя — константы и инжекция, runtime-поведение (downloader + template loader), миграция/инвалидация. Каждый слой — отдельный коммит, каждый коммит самодостаточен (билд зелёный после каждого).

---

## 1. Слой констант + CI-инжекция

**Цель:** появление `RequiredCoreVersion` / `RequiredTemplateRef` без изменения runtime-поведения. После этого коммита всё работает как раньше, но в `internal/constants` уже доступны новые символы.

### 1.1 Изменения в коде

- `internal/constants/constants.go`:
  - `+ const RequiredCoreVersion = "1.13.6"` (стартовое значение = текущий `FallbackVersion`).
  - `+ var RequiredTemplateRef = "<актуализировать на момент имплементации: git rev-parse origin/main>"` (CI перетирает через ldflags; для dev-сборок остаётся source-default).
  - Краткий package-comment, что это инвариант релиза.

### 1.2 Изменения в build-скриптах

- `build/build_darwin.sh:191,201,229,242` — добавить в `-ldflags`:
  ```
  -X singbox-launcher/internal/constants.RequiredTemplateRef=$TEMPLATE_REF
  ```
  Объявить `TEMPLATE_REF="$(git rev-parse HEAD)"` в одном месте перед первым go build.
- `build/build_linux.sh:94` — то же.
- `build/build_windows.bat:191+` — то же (cmd-синтаксис: `set "TEMPLATE_REF=...`).

### 1.3 Тесты

- Минимальный unit-тест в `internal/constants/constants_test.go`: проверить, что `RequiredCoreVersion` непустой и матчит регэксп `\d+\.\d+\.\d+`.

---

## 2. Слой downloader: pin core version

**Цель:** удалить latest-логику для core; UI больше не предлагает обновление.

### 2.1 `core/core_downloader.go`

- `DownloadCore(ctx, version, ch)`:
  - Если `version == ""` → `version = RequiredCoreVersion` (вместо текущего GitHub-API похода).
  - Win7-ветка остаётся как есть (`Win7LegacyVersion` имеет приоритет на `windows/386`).
- `getReleaseInfoFromSourceForge` — уже использует переданный `version`, ничего не меняем.
- Удалить из `getReleaseInfoFromSourceForge` поход в `GetLatestCoreVersion()` для пустого version (теперь невозможно — version всегда не пуст).

### 2.2 `core/core_version.go`

**Удалить:**
- `GetLatestCoreVersion`, `getLatestVersionFromURL`, `getLatestVersionFromURLWithPrefix` (для core; **launcher**-вариант остаётся — `GetLatestLauncherVersion`).
- `CheckVersionInBackground`, `ShouldCheckVersion` — для core. Все вызовы.
- `CheckForUpdates` для core — оставить только launcher-вариант (или переименовать в `CheckLauncherUpdates`).
- `GetCachedVersion` / `SetCachedVersion` для core — все call-site'ы. Свойства `VersionCheckCache*` в `StateService` уезжают.

**Оставить:**
- `GetInstalledCoreVersion` — отображение текущей версии.
- `CompareVersions` — пригодится для миграции шаблона (см. §4).
- `FallbackVersion` → стираем, заменён на `RequiredCoreVersion`.

### 2.3 `ui/core_dashboard_tab.go`

- `updateVersionInfoAsync(installed, binaryNotFound)`:
  - `binaryNotFound = true` → кнопка «Download v$RequiredCoreVersion».
  - `installed != RequiredCoreVersion` → кнопка «Reinstall v$RequiredCoreVersion» (нейтральная иконка, не HighImportance).
  - `installed == RequiredCoreVersion` → кнопка скрыта.
- Удалить `tab.startAutoUpdate()` (поллинг latest каждые 10 минут).
- `handleDownload` → `tab.controller.DownloadCore(ctx, "", progressChan)` (пустая version → подставится `RequiredCoreVersion`).

### 2.4 i18n

- `core.button_download_version` остаётся. Новый ключ `core.button_reinstall_version` для случая «версия отличается от Required».
- `bin/locale/ru.json`, `internal/locale/en.json` — обе локали.

### 2.5 Тесты

- Unit на `DownloadCore` с моком HTTP: при пустом `version` — скачивается `RequiredCoreVersion` (пробивается через mock URL).
- Snapshot на UI Core Dashboard? Не делаем — ручной smoke-test в чеклисте PR'а.

---

## 3. Слой template: pin commit ref + invalidation

**Цель:** template качается с pinned commit, на апгрейде локальный шаблон удаляется.

### 3.1 `ui/wizard/template/loader.go`

- `GetTemplateURL()`:
  ```go
  func GetTemplateURL() string {
      // RequiredTemplateRef непустой по построению:
      //  - CI-сборки: ldflags-инжекция git rev-parse HEAD
      //  - go run / локальные билды: source-default = последний коммит main
      return fmt.Sprintf("https://raw.githubusercontent.com/Leadaxe/singbox-launcher/%s/bin/%s",
          constants.RequiredTemplateRef, TemplateFileName)
  }
  ```
  `GetMyBranch()` больше не используется в loader'е (остаётся для `get_free_dialog.go`).
- Логирование на debuglog: `TemplateLoader: URL ref = <short-sha>` (источник для логов вычислять не нужно — ref всегда непустой; для отладки достаточно увидеть, какой именно коммит используется).

### 3.2 `internal/locale/settings.go`

- `Settings`:
  ```go
  LastTemplateLauncherVersion string `json:"last_template_launcher_version,omitempty"`
  ```
- Helper:
  ```go
  func MarkTemplateInstalled(binDir, appVersion string) error {
      s := LoadSettings(binDir)
      s.LastTemplateLauncherVersion = appVersion
      return SaveSettings(binDir, s)
  }
  ```

### 3.3 `core/template_migration.go` (новый файл)

```go
package core

import (
    "os"
    "path/filepath"
    "strings"

    "singbox-launcher/internal/constants"
    "singbox-launcher/internal/debuglog"
    "singbox-launcher/internal/locale"
    "singbox-launcher/internal/platform"
)

// InvalidateTemplateIfStale removes bin/wizard_template.json when it was last
// installed by an older launcher version. Called once on startup.
//
// Dev builds (AppVersion == "v-local-test*" or contains "unnamed-dev") are
// skipped to keep the inner-loop fast — devs swap templates manually.
func InvalidateTemplateIfStale(execDir string) error {
    if isDevBuild(constants.AppVersion) {
        return nil
    }
    binDir := platform.GetBinDir(execDir)
    s := locale.LoadSettings(binDir)
    last := s.LastTemplateLauncherVersion

    if last != "" && CompareVersions(stripV(last), stripV(constants.AppVersion)) >= 0 {
        return nil
    }
    templatePath := filepath.Join(binDir, "wizard_template.json")
    if _, err := os.Stat(templatePath); err != nil {
        return nil // нет файла — нечего инвалидировать
    }
    if err := os.Remove(templatePath); err != nil {
        return err
    }
    debuglog.InfoLog("template: invalidated (was installed by %q, current %q)", last, constants.AppVersion)
    return nil
}

func isDevBuild(v string) bool {
    return strings.HasPrefix(v, "v-local-test") || strings.Contains(v, "unnamed-dev")
}

func stripV(v string) string { return strings.TrimPrefix(v, "v") }
```

### 3.4 Где вызывать

В `main.go` или в `AppController.Init()` — после `EnsureDirectories`, до создания UI:

```go
if err := core.InvalidateTemplateIfStale(execDir); err != nil {
    debuglog.WarnLog("template invalidation failed: %v", err)
    // не fatal — UI всё равно покажет «Download Template»
}
```

### 3.5 Маркировка установки

В `ui/core_dashboard_tab.go::downloadConfigTemplate` после успешного `os.WriteFile(target, data, 0o644)`:

```go
if err := locale.MarkTemplateInstalled(filepath.Dir(target), constants.AppVersion); err != nil {
    debuglog.WarnLog("failed to mark template install version: %v", err)
}
```

### 3.6 UI-сообщение «формат изменился»

После инвалидации `bin/wizard_template.json` отсутствует. Текущее поведение Core Dashboard ([`ui/core_dashboard_tab.go:666-679`](../../ui/core_dashboard_tab.go)) уже показывает синюю «Download Template». Достаточно — никаких новых диалогов не вводим. (Можно опционально добавить tooltip «обновлено для новой версии лаунчера» — but that's polish; не входит в must-have).

### 3.7 Тесты

- `core/template_migration_test.go`:
  - `last == ""`, шаблон есть → удалён.
  - `last < AppVersion`, шаблон есть → удалён.
  - `last == AppVersion`, шаблон есть → не удалён.
  - `last > AppVersion` (downgrade) → не удалён, лог-warn.
  - `AppVersion == "v-local-test"` → пропуск.
  - Отсутствует шаблон → no-op без ошибки.
- `ui/wizard/template/loader_test.go`:
  - `RequiredTemplateRef = ""` → URL содержит `develop`/`main`.
  - `RequiredTemplateRef = "abc123"` → URL содержит `abc123`.

---

## 4. Документация и release process

### 4.1 `docs/RELEASE_PROCESS.md`

- Новая секция «§5 Pinned dependencies» — отдельный раздел про модель: что фиксируется, кто бампит, что инжектит CI.
- В §1.1 (stable pre-flight) — два новых чеклист-пункта (см. SPEC §4.1).
- В §2.1 (pre-release pre-flight) — пункт про core bump.
- В §4 (чеклист закрытия) — `RequiredCoreVersion` соответствует протестированной версии.

### 4.2 `docs/ARCHITECTURE.md`

- В разделе про startup — короткий абзац про invalidation pipeline.
- В разделе про `bin/` — пометка, что `wizard_template.json` теперь pinned-by-commit.

### 4.3 `docs/release_notes/upcoming.md`

- EN: «Pinned core/template — launcher now ships with fixed sing-box version and template ref; upgrade launcher to get newer core».
- RU: то же по-русски.
- Migration note: «existing installs will re-download the template on first launch after upgrade».

---

## 5. Порядок коммитов

1. `chore(constants): introduce RequiredCoreVersion and RequiredTemplateRef ldflags var` — слой 1.
2. `build(ci): inject RequiredTemplateRef from git rev-parse HEAD` — build-скрипты.
3. `feat(core): pin sing-box version, drop latest-update UI nudge` — слой 2 (downloader + UI).
4. `feat(template): pin to commit ref, invalidate on launcher upgrade` — слой 3.
5. `docs(release-process): document pinned dependencies` — обновление RELEASE_PROCESS.
6. `docs(release-notes): pinned core/template entry` — upcoming.md.

Каждый коммит — `go build && go test && go vet` зелёные.

---

## 6. Открытые вопросы

1. **Маркировать ли существующие установки при первом запуске нового launcher'а?** Вариант: если `LastTemplateLauncherVersion == ""` И `bin/wizard_template.json` существует И `AppVersion` — non-dev → принудительная инвалидация (как описано). Альтернатива: тихо пометить `LastTemplateLauncherVersion = AppVersion` и не трогать. Ответ: **инвалидируем** — формат шаблона мог уже разойтись за несколько релизов, а бесплатное прохождение «как будто всё ОК» опаснее, чем пара кликов «Download Template». Финализировано в SPEC §3.2.
2. **Нужен ли pinned ref для `get_free.json`?** Сейчас он тоже тянется по `GetMyBranch()` ([`ui/wizard/dialogs/get_free_dialog.go`](../../ui/wizard/dialogs/get_free_dialog.go)). Вынести в SPEC 046? Аргумент за: тот же класс проблем (формат может разойтись). Аргумент против: get_free — это готовые public-ноды, формат у них стабилен (мы его не меняем). **Решение:** не трогаем в этом SPEC; если появятся проблемы — отдельный спек.
3. **Формат `LastTemplateLauncherVersion` — `v0.8.7` или `0.8.7`?** В `AppVersion` сейчас всегда с `v` префиксом (от `git describe --tags`). В settings храним as-is, без нормализации. `CompareVersions` сам триммит `v` ([`core/core_version.go:476-477`](../../core/core_version.go)) — никаких хитростей.
