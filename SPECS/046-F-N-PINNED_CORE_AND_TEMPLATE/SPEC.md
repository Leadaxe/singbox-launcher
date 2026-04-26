# SPEC 046 — PINNED_CORE_AND_TEMPLATE

**Тип:** F (Feature) · **Статус:** N (New, в планировании).

Закрепить за каждой версией лаунчера **(а)** конкретную версию `sing-box` и **(б)** конкретный коммит `wizard_template.json`. Удалить «плавающие» источники (latest-релиз GitHub, HEAD ветки) из путей загрузки. На апгрейде лаунчера принудительно инвалидировать локальный шаблон, если он был установлен предыдущей версией.

---

## 1. Контекст и проблема

Сейчас лаунчер тащит **актуальный** sing-box и **HEAD ветки** для шаблона:

### 1.1 sing-box core
- [`core/core_version.go:81`](../../core/core_version.go) `FallbackVersion = "1.13.6"` используется только когда GitHub API недоступен; иначе всегда подставляется `latest`.
- [`core/core_version.go:84`](../../core/core_version.go) `GetLatestCoreVersion()` лезет в `https://api.github.com/repos/SagerNet/sing-box/releases/latest`.
- [`ui/core_dashboard_tab.go:768-770`](../../ui/core_dashboard_tab.go) — кнопка превращается в синюю «Update to v1.x.y», как только `cached > installed`.
- [`ui/core_dashboard_tab.go:850-873`](../../ui/core_dashboard_tab.go) — `handleDownload` берёт `GetCachedVersion()` (= latest), а не вшитую версию.
- Прецедент правильного поведения уже есть: [`core/core_downloader.go:25`](../../core/core_downloader.go) `Win7LegacyVersion = "1.13.2"` — жёстко прибит и используется безусловно при `windows/386`. Эту схему обобщаем на все платформы.

**Проблема:** каждая версия лаунчера тестируется на **конкретной** версии ядра. Запуск с непротестированной версией sing-box даёт неопределённое поведение (синтаксис конфига расходится между минор-релизами sing-box, ломаются wizard-шаблоны, падают парсеры подписок). UI же активно подталкивает пользователя обновиться.

### 1.2 wizard_template.json
- [`ui/wizard/template/loader.go:142`](../../ui/wizard/template/loader.go) `GetTemplateURL` строит URL вида `https://raw.githubusercontent.com/Leadaxe/singbox-launcher/<branch>/bin/wizard_template.json`.
- [`internal/constants/constants.go:53-59`](../../internal/constants/constants.go) `GetMyBranch()` = `develop` для версий с `-` в имени (включая `v-local-test`), иначе `main`. Всегда HEAD ветки.
- [`ui/core_dashboard_tab.go:777-841`](../../ui/core_dashboard_tab.go) `downloadConfigTemplate` — кнопка на Core Dashboard, скачивает по этому URL.
- При апгрейде лаунчера никакой инвалидации локального `bin/wizard_template.json` нет. Шаблон, установленный полтора года назад, продолжает использоваться, пока что-то не упадёт.
- В [`internal/locale/settings.go`](../../internal/locale/settings.go) `Settings` — нет поля «какая версия лаунчера в последний раз скачивала шаблон».

**Проблема:** шаблон — формат с собственной семантикой (vars, params, selectable_rules, dns_options, …). Между версиями лаунчера семантика расходится: новые поля, переименованные ключи, изменённые валидации (см. историю SPEC 027/032/040). Старый шаблон с новым лаунчером (или наоборот) даёт «молчащую» поломку — wizard работает, но генерируется уже не тот конфиг, что тестировался на CI.

### 1.3 Пересечение с SPEC 045

SPEC 045 (`STATE_CONFIG_DECOUPLING`) переоформляет, **что** пишется в `config.json` и когда. Этот SPEC — про другое: **откуда** берутся ядро и шаблон. Ортогонально, можно делать раньше и независимо.

---

## 2. Желаемое поведение

### 2.1 sing-box core: фиксированная версия на launcher-build

| Сценарий                     | Поведение                                                                  |
|------------------------------|----------------------------------------------------------------------------|
| Свежая установка             | `DownloadCore` всегда ставит `RequiredCoreVersion`, сборка под текущий launcher |
| Запуск с уже установленным ядром той же версии | Не качает повторно                                       |
| Запуск с `installed != Required` | Кнопка Download/Reinstall видна; UI **не** называет это «обновлением» |
| GitHub недоступен            | SourceForge mirror как fallback (как сейчас); версия — та же `RequiredCoreVersion` |
| Win7 (windows/386)           | Существующая схема `Win7LegacyVersion` — частный случай; остаётся    |

**Удаляем:**
- проактивные latest-checks из UI (`updateVersionInfoAsync`, «Update to v1.x.y» декорация);
- `CheckVersionInBackground` retry-loop;
- `CheckForUpdates` для core (для launcher-версии — оставляем).

**Оставляем:**
- `DownloadCore` сам факт скачивания (ручная кнопка / первый запуск);
- mirrors-цепочку (GitHub → ghproxy → SourceForge);
- проверку installed-version через `sing-box version` для отображения статуса.

### 2.2 wizard_template.json: pinned commit ref

| Сценарий                                | Поведение                                                                |
|-----------------------------------------|--------------------------------------------------------------------------|
| Свежая установка, шаблона нет           | UI показывает «Download Template» (как сейчас); URL построен на `RequiredTemplateRef` (commit SHA) |
| Шаблон уже скачан этой же версией       | Используется как есть                                                    |
| Шаблон скачан **предыдущей** версией    | На старте удаляется (`bin/wizard_template.json`), UI показывает «Download Template» с пометкой «формат шаблона изменился» |
| Шаблон скачан, но `LastTemplateLauncherVersion` пустой (legacy миграция) | Удаляется один раз; в settings.json пишется текущая `AppVersion` после повторного скачивания |
| Dev-сборка / отсутствие ldflags-инжекции | `RequiredTemplateRef` = source-default (последний коммит main); pinned, но maintainer-controlled |

**Источник истины для `RequiredTemplateRef`:** commit SHA сборки. CI знает его, инжектит через `-ldflags` (см. §3.3).

### 2.3 Сводка контрактов

```go
// internal/constants/constants.go
const RequiredCoreVersion = "1.13.11"                                  // обновляется вручную в источнике
var   RequiredTemplateRef = "<actualize-at-implementation>"            // дефолт = последний коммит main на момент реализации; CI перетирает через ldflags
```

**Почему дефолт — реальный SHA, а не `""`:** без ldflags (локальный `go run .`, нестандартный билд) лаунчер всё равно получает рабочий pinned-ref, а не плавающий branch HEAD. Maintainer бампит этот дефолт при каждом merge `develop → main` (см. §4). Пустой ref как fallback не нужен.

> **На момент реализации** значение SHA нужно актуализировать: `git fetch origin main && git rev-parse origin/main` → подставить в источник. SHA, зафиксированный в этой SPEC при её написании (`abc07de…`), к моменту имплементации скорее всего уже устареет.

```go
// internal/locale/settings.go (Settings struct)
LastTemplateLauncherVersion string `json:"last_template_launcher_version,omitempty"`
```

---

## 3. Архитектура

### 3.1 Изменяемые точки

| Файл                                      | Что меняем                                                                |
|-------------------------------------------|---------------------------------------------------------------------------|
| `internal/constants/constants.go`         | + `RequiredCoreVersion` const, + `RequiredTemplateRef` var (для ldflags) |
| `core/core_downloader.go`                 | `DownloadCore`: если `version == ""`, использовать `RequiredCoreVersion` (вместо `getLatestCoreVersion`); удалить latest-логику в SourceForge fallback |
| `core/core_version.go`                    | удалить `CheckVersionInBackground` для core, `GetLatestCoreVersion`, `ShouldCheckVersion`, `GetCachedVersion` (для core); `GetInstalledCoreVersion` оставить; `CheckForUpdates` для core убрать; **launcher-version checks остаются** |
| `ui/core_dashboard_tab.go`                | `updateVersionInfoAsync` — кнопка показывается только если `installed != Required` или бинарника нет; удалить «Update to v1.x.y» с HighImportance; удалить `startAutoUpdate` для core (вкладка остаётся, но без бэкграунд-poll'а) |
| `ui/wizard/template/loader.go`            | `GetTemplateURL()` строит URL на `RequiredTemplateRef`; если ref пустой — используем `GetMyBranch()` (dev fallback) |
| `internal/locale/settings.go`             | + `LastTemplateLauncherVersion`; helper `MarkTemplateInstalled()` обновляет поле |
| `core/controller.go` (или новая функция в `core/`) | startup-check: если `LastTemplateLauncherVersion < AppVersion` и `bin/wizard_template.json` существует — удалить файл, лог `template invalidated by upgrade` |
| `ui/core_dashboard_tab.go` (`downloadConfigTemplate`) | после успешной записи — `MarkTemplateInstalled(AppVersion)` |
| `build/build_darwin.sh` / `build_linux.sh` / `build_windows.bat` | передать `-X ...RequiredTemplateRef=$(git rev-parse HEAD)` в ldflags |
| `.github/workflows/ci.yml`                | в job `build` — `git rev-parse HEAD` уже доступен; ничего не меняем, скрипты сами извлекут |
| `docs/RELEASE_PROCESS.md`                 | новая pre-flight задача: проверить/обновить `RequiredCoreVersion`         |

### 3.2 Логика инвалидации шаблона при апгрейде

В точке startup (после `EnsureDirectories`, до построения UI):

```go
// core/template_migration.go (новый файл)
func InvalidateTemplateIfStale(execDir string, appVersion string) error {
    binDir := platform.GetBinDir(execDir)
    s := locale.LoadSettings(binDir)
    last := s.LastTemplateLauncherVersion

    // Empty (legacy install) или меньше текущей AppVersion → инвалидировать.
    if last != "" && CompareVersions(last, appVersion) >= 0 {
        return nil // up-to-date или новее
    }

    templatePath := filepath.Join(binDir, "wizard_template.json")
    if _, err := os.Stat(templatePath); err != nil {
        // нет файла — invalidate not needed; просто пометим текущую версию
        // как «всё актуально», чтобы пустой last не триггерил следующий запуск
        return nil
    }
    if err := os.Remove(templatePath); err != nil {
        return fmt.Errorf("invalidate template: %w", err)
    }
    debuglog.InfoLog("template invalidated by launcher upgrade (was %q, now %q)", last, appVersion)
    return nil
}
```

**Когда писать `LastTemplateLauncherVersion`:** только после **успешного** `downloadConfigTemplate` — чтобы прерванная закачка не отметила миграцию как сделанную.

**Что делать с empty last:** для существующих (legacy) установок, у которых шаблон уже скачан, но `LastTemplateLauncherVersion` пустой — лечим консервативно: считаем шаблон протухшим один раз, удаляем, просим скачать. Альтернатива — игнорировать пустое поле — оставляет неопределённое окно совместимости. Принимаем разовый шум миграции в обмен на детерминированность.

### 3.3 CI / build-скрипты — инжекция RequiredTemplateRef

Текущая инжекция `AppVersion` ([`build/build_darwin.sh:191`](../../build/build_darwin.sh)):
```bash
GOARCH=arm64 go build -buildvcs=false \
  -ldflags="-s -w -X singbox-launcher/internal/constants.AppVersion=$VERSION" \
  -o "$TEMP_ARM64"
```

Расширяем:
```bash
TEMPLATE_REF="$(git rev-parse HEAD)"
GOARCH=arm64 go build -buildvcs=false \
  -ldflags="-s -w \
    -X singbox-launcher/internal/constants.AppVersion=$VERSION \
    -X singbox-launcher/internal/constants.RequiredTemplateRef=$TEMPLATE_REF" \
  -o "$TEMP_ARM64"
```

То же — `build_linux.sh:94`, `build_windows.bat:191+`.

**Почему commit SHA, а не `$VERSION`:**
- На stable (`v0.8.8`) и pre-release (`v0.8.8-1-gXXXXX-prerelease`) commit SHA одинаково однозначно адресует `bin/wizard_template.json` в репозитории через `raw.githubusercontent.com/<owner>/<repo>/<sha>/bin/wizard_template.json`.
- Tag-based URL (`raw.githubusercontent.com/.../tags/v0.8.8/bin/...`) работает для stable, но не для pre-release tags, которые создаёт CI **после** сборки (см. `RELEASE_PROCESS.md §2.4`). Commit SHA доступен **до** сборки.
- Для pre-release builds это даёт реальную проверяемость: «какой именно шаблон тестировался» = `git show <sha>:bin/wizard_template.json`.

**Local builds (`go run`, dev):** `RequiredTemplateRef` = source-default (последний коммит main, см. §2.3). Это реальный SHA, не плавающий — `go run .` тащит pinned шаблон, не HEAD ветки. Maintainer бампит дефолт при merge `develop → main` (отдельный коммит, см. §4.4).

### 3.4 RequiredCoreVersion — почему не инжектить из CI

Альтернативы рассматривал:
1. **Файл `bin/CORE_VERSION`, читать в скрипте, инжектить через ldflags.** Плюс: единый источник, нет рассинхрона. Минус: добавляется лишняя обвязка для константы, которую и так бампают редко.
2. **Парсить из `RELEASE_NOTES.md` / `docs/release_notes/X-Y-Z.md`.** Минус: фрагильно, не структурный источник.
3. **Захардкодить в коде, бамп — ручная задача в `RELEASE_PROCESS.md`.** Плюс: прозрачно, ревьюится в PR с релизными нотами. Минус: легко забыть → ловится §1.1 чеклиста релиз-процесса (см. §4.1).

Берём вариант **3**: `RequiredCoreVersion` — обычная константа в источнике. Бамп — отдельный коммит, ревью.

---

## 4. Обновление протокола выкатки релиза

### 4.1 Pre-flight для stable (`vX.Y.Z`) — новые пункты в `RELEASE_PROCESS.md §1.1`

- [ ] **`internal/constants.RequiredCoreVersion`** — проверить, что соответствует версии sing-box, на которой проводилось финальное тестирование релиза. Если за время разработки релиза в SagerNet вышло что-то критичное (CVE, breaking-fix) — бампим **до тега**, отдельным коммитом `chore(core): pin sing-box vX.Y.Z`. По дефолту версия не меняется.
- [ ] **`internal/constants.RequiredTemplateRef`** — для CI-сборок инжектится через ldflags автоматически. Source-default (для `go run` и нестандартных билдов) обновляется **в §1.5** после merge `main` ← `develop` обратно в `develop`. Убедиться, что `bin/wizard_template.json` в `develop` соответствует тому, что тестировалось.

### 4.2 Pre-flight для pre-release (`...-prerelease`) — новые пункты в §2.1

- [ ] Если бампали `RequiredCoreVersion` — отдельный коммит **до** запуска `gh workflow run ci.yml`, чтобы CI увидел новую константу.
- [ ] Шаблон — pinned автоматически на commit того же pre-release build'а, ничего руками не делаем.

### 4.3 Чеклист закрытия релиза — новый пункт в §4

```diff
 ### Stable vX.Y.Z
 - [ ] `develop` зелёная, descendant от прошлого stable-тега.
+- [ ] `RequiredCoreVersion` соответствует протестированной sing-box версии.
 - [ ] `upcoming.md` → `docs/release_notes/X-Y-Z.md`, причёсан.
 ...
```

### 4.4 Bump source-default `RequiredTemplateRef` после релиза

Добавляется шаг в `RELEASE_PROCESS.md §1.5` (post-flight: вернуть main в develop):

После `git push origin develop` с merge-коммитом из main — обновить дефолт `RequiredTemplateRef` на новый HEAD main:

```bash
NEW_REF="$(git rev-parse origin/main)"
# править internal/constants/constants.go: RequiredTemplateRef = "<NEW_REF>"
git commit -am "chore(constants): bump RequiredTemplateRef source-default to <short-sha>"
git push origin develop
```

Зачем: dev-сборки (`go run`, локальные билды без CI) тянут шаблон по этому дефолту. Если его не обновлять — на develop сидит протухший pin времён прошлого релиза. CI-сборкам это безразлично (там SHA инжектится из `git rev-parse HEAD`).

### 4.5 Documentation

- `docs/RELEASE_PROCESS.md` — новая секция «§5 Pinned dependencies» с подробным разъяснением что такое `RequiredCoreVersion` / `RequiredTemplateRef`, кто их меняет.
- `docs/ARCHITECTURE.md` — добавить упоминание про invalidation-pipeline (один абзац в разделе про startup).

---

## 5. Критерии приёмки

1. **Свежий install / fresh download.**
   `DownloadCore` без сетевого доступа к `api.github.com` (mock или офлайн-тест) корректно работает: качает `RequiredCoreVersion` через SourceForge mirror без обращения к latest-API. Проверка: unit-тест на `findPlatformAsset` + integration через `build/test_*.sh`.

2. **UI больше не «торопит» обновить core.**
   Запуск лаунчера с `installed == RequiredCoreVersion`: кнопка Download скрыта, нет «Update to v1.x.y», нет background-poll'а. Проверка: ручной (Core Dashboard tab snapshot до/после), grep на `GetLatestCoreVersion` после рефакторинга должен дать **ноль** call-site'ов в `ui/`.

3. **Шаблон pin'ится на commit.**
   `GetTemplateURL()` возвращает URL, содержащий ровно ту SHA, под которой собран бинарник: `https://raw.githubusercontent.com/Leadaxe/singbox-launcher/<sha>/bin/wizard_template.json`. Проверка: unit-тест с подменой `RequiredTemplateRef`.

4. **Инвалидация при апгрейде.**
   Сценарий: `LastTemplateLauncherVersion = "v0.8.7"`, `AppVersion = "v0.8.8"`, локальный `bin/wizard_template.json` есть → после старта файл удалён, лог содержит `template invalidated by launcher upgrade`. Проверка: unit-тест на `InvalidateTemplateIfStale`.

5. **Dev-experience не сломан.**
   `go run .` без ldflags запускается; `GetTemplateURL()` падает в `develop` HEAD; `DownloadCore` использует `RequiredCoreVersion` из исходника. Никто из контрибьюторов не должен возиться с локальной настройкой ldflags для запуска лаунчера.

6. **Release process отражает новые требования.**
   `docs/RELEASE_PROCESS.md` содержит явные шаги по `RequiredCoreVersion` (manual) и `RequiredTemplateRef` (CI auto). Чеклист §4 обновлён.

7. **Migration safety.**
   Существующие установки (где `LastTemplateLauncherVersion` пустой и шаблон лежит) корректно мигрируют один раз: шаблон удалён, UI показывает «Download Template», после клика — записывается `LastTemplateLauncherVersion = AppVersion`. Сценарий покрыт unit-тестом + ручной верификацией на macOS+Linux+Windows.

---

## 6. Риски

1. **Pre-release build с не-пушнутым commit'ом.**
   Локальная сборка (`build_darwin.sh` без `git push`) → инжектит SHA, которого нет на GitHub. Шаблон не скачается. **Митигация:** в build-скрипте после `git rev-parse HEAD` дополнительно `git fetch origin && git merge-base --is-ancestor HEAD origin/develop` с warn (не fail) если SHA не на remote. Pre-release CI всегда пушит — там проблемы нет.

2. **Шаблон в repo не успевает за RequiredCoreVersion.**
   Бампнули core до 1.14.0, но `wizard_template.json` ещё под 1.13.x → wizard может сгенерить конфиг, который sing-box 1.14 не принимает. **Митигация:** в pre-flight чеклисте бамп core и проверка/обновление шаблона — два пункта рядом, агент или релизный maintainer не пропустит.

3. **Сломанная миграция на dev-сборках.**
   `AppVersion = "v-local-test"` → `CompareVersions("v0.8.7", "v-local-test")` ведёт себя непредсказуемо. **Митигация:** в `InvalidateTemplateIfStale` явно проверять «AppVersion начинается с `v-local-test` или `unnamed-dev`» → пропускать инвалидацию, чтобы dev-цикл не ломал шаблон при каждом запуске.

4. **Регрессия в UX «я хочу новую версию sing-box».**
   Энтузиасты могут возмутиться «почему меня не апгрейдят на 1.14, оно уже вышло». **Митигация:** в release notes явно объяснить новую модель («launcher is pinned to a tested core version; upgrade the launcher to get a newer core»). UI-tooltip на binary-info — однажды.

5. **Большая площадь изменений в `core_dashboard_tab.go`.**
   Удаление update-логики затрагивает несколько помощников (`updateVersionInfoAsync`, `startAutoUpdate`, `CheckVersionInBackground` для core). Нужно аккуратно отделить от launcher-version-check (`CheckLauncherVersionOnStartup`, `ShowUpdatePopupIfAvailable`), который **остаётся**.

---

## 7. Не-цели

- Не меняем механику launcher self-update (там pin не нужен — launcher сам сам себя апгрейдит, это другая ось).
- Не меняем механику auto-update подписок ([`core/auto_update.go`](../../core/auto_update.go)) — это про конфиг, не про ядро/шаблон.
- Не вводим SBOM / signature verification для скачиваемых артефактов (отдельный SPEC при необходимости).
- Не делаем UI-возможность «принудительно скачать другую версию sing-box» — pinned значит pinned. Если пользователь сильно хочет — кладёт бинарник руками в `bin/` (документировано).

---

## 8. Связи

- **Параллельный SPEC:** [SPEC 045](../045-F-N-STATE_CONFIG_DECOUPLING/SPEC.md) — ортогонально, можно делать в любом порядке.
- **Зависит от:** [SPEC 019](../019-F-C-WIN7_ADAPTATION/) — Win7Legacy-схема, прецедент pinning для одной платформы; обобщаем.
- **Затрагивает:** `RELEASE_PROCESS.md` — обязательное обновление процедуры.
- **Из истории:** обсуждение с пользователем 2026-04-25 — два тезиса (core auto-update вреден, шаблон должен инвалидироваться при апгрейде) подтверждены кодом, оформлено в этот SPEC.
