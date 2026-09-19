# ТЗ 6 · Предрелизная разведка v2.0.0

**Дата:** 2026-09-19 · **ветка:** `develop` · **последний stable:** `v1.6.3` · **целевой релиз:** `v2.0.0` (версию в коде не меняли, тег не ставили).

Разведка только чтением/grep/локальным `go run ./tools/win7guard`. Код приложения не трогали.

---

## 1. Где зашита версия приложения и как попадает в бинарь

### 1.1. Литерал `1.6.3` (grep без `.git`, `SPECS/`, `docs/release_notes/1-*`)

| Файл:строка | Контекст |
|-------------|----------|
| `CHANGELOG.md:13` | Историческая секция `## v1.6.3` |
| `RELEASE_NOTES.md:11,15,683` | Индекс релизов, ссылка на `docs/release_notes/1-6-3.md` |
| `contract/registry/tls.json:509` | Комментарий в реестре («…до 1.6.3») |

**Вывод:** номер **текущей** версии приложения нигде не захардкожен как `1.6.3`. На `develop` сейчас `git describe --tags --exclude='*-prerelease'` → `v1.6.3-161-gb80a4abf`.

### 1.2. Источник истины `AppVersion`

| Файл:строка | Роль |
|-------------|------|
| `internal/constants/constants.go:160-172` | Переменная `AppVersion = "v-local-test"` — **source-default** для `go run .`; в релизных сборках перезаписывается через `-ldflags` |
| `internal/constants/constants.go:158` | `RequiredCoreVersion = "1.14.1-lx.8"` — **отдельная** константа (версия ядра sing-box-lx, не версия лаунчера) |

### 1.3. Инъекция в бинарь (`-ldflags -X singbox-launcher/internal/constants.AppVersion=…`)

| Файл:строка | Откуда берётся `$VERSION` |
|-------------|---------------------------|
| `build/build_linux.sh:124-137` | `git describe --tags --always --dirty --exclude='*-prerelease'` (fallback `0.4.1`) |
| `build/build_darwin.sh:184-208` | `$APP_VERSION` или тот же `git describe`, fallback `unnamed-dev` |
| `build/build_windows.bat:119-212` | `$APP_VERSION` или `git describe`, fallback `unnamed-dev` |
| `.github/workflows/ci.yml:103-142` | **meta:** stable tag из `GITHUB_REF`; prerelease → `git describe …`-prerelease; build → `dev.<branch>.<sha>` |
| `.github/workflows/ci.yml:331,342,416,520` | `APP_VERSION: ${{ needs.meta.outputs.version }}` → скрипты сборки |
| `.github/workflows/ci.yml:573-575` | Win7: inline `go build … -ldflags "-X …AppVersion=$APP_VERSION …"` |

Параллельно во всех путях инжектится `RequiredTemplateRef=$(git rev-parse HEAD)` — см. те же строки ldflags.

### 1.4. Платформенные манифесты / ресурсы (не ldflags)

| Файл:строка | Что там |
|-------------|---------|
| `build/build_darwin.sh:345-348` | `Info.plist`: `CFBundleShortVersionString` и `CFBundleVersion` = `$VERSION` (тот же git describe / APP_VERSION) |
| `app.manifest:3` | `assemblyIdentity version="1.0.0.0"` — **фиксированная** версия UAC-манифеста, не версия приложения |
| `build/build_windows.bat:153-160`, `.github/workflows/ci.yml:554` | `rsrc … -manifest app.manifest -o rsrc.syso` — иконка + UAC; **FileVersion/ProductVersion в .rc/.syso нет** (grep по репо пуст) |
| `build/Dockerfile.linux:28` | `go build -ldflags="-s -w"` **без** AppVersion — dev-образ, не релизный путь |

### 1.5. Что менять при выпуске v2.0.0

1. Поставить annotated tag `v2.0.0` на merge-commit в `main` — CI/meta и ldflags подхватят тег автоматически.
2. Source-default `AppVersion` в `constants.go` **не** трогать до post-flight (§1.5 `RELEASE_PROCESS.md`).
3. Отдельно проверить/обновить `RequiredCoreVersion` при смене пина ядра (§5.1 `RELEASE_PROCESS.md`).

---

## 2. Автообновление: сравнение версий

### 2.1. Код

| Файл:строка | Функция |
|-------------|---------|
| `core/core_version.go:64-91` | `GetLatestLauncherVersion()` — GitHub API `/releases/latest`, поле `tag_name` |
| `core/core_version.go:138-190` | `getLatestVersionFromURLWithPrefix()` — парсит JSON, сохраняет префикс `v` |
| `core/core_version.go:192-256` | **`CompareVersions(v1,v2)`** — ядро алгоритма |
| `core/core_version.go:258-289` | `ShowUpdatePopupIfAvailable()` — `CompareVersions(current, latest) >= 0` → обновление не нужно |
| `core/core_version.go:108-136` | `CheckLauncherVersionOnStartup()` — фоновый fetch + кеш |
| `ui/help_tab.go:42-61` | Help-вкладка: тот же `CompareVersions` для статуса |
| `core/template_migration.go:78` | Сравнение `LastTemplateLauncherVersion` vs `AppVersion` |
| `core/core_capabilities.go:297` | Сравнение версии ядра (не лаунчера) |

### 2.2. Алгоритм `CompareVersions` (`core/core_version.go:194-256`)

1. Снять префикс `v`.
2. `extractBaseVersion`: база = часть **до первого `-`** (`2.0.0-1-gSHA` → base `2.0.0`, suffix есть).
3. `compareBaseVersions`: разбить base на `.`, сравнить компоненты **как int** (не строкой).
4. Если base равны: версия **с** суффиксом считается **новее** версии без суффикса (`2.0.0-1-gabc` > `2.0.0`).

**Не semver:** суффиксы `-rc.1`, `-beta` не ранжируются между собой; git describe / `-prerelease` — единственный ожидаемый формат.

### 2.3. Доказательство для перехода 1.6.3 → 2.0.0 и пререлизов

**Юнит-теста `CompareVersions` нет** (grep `TestCompare` / `CompareVersions` в `*_test.go` — пусто).

Локальный прогон `/tmp/cvtest.go` (2026-09-19):

| v1 | v2 | результат | ожидание |
|----|-----|-----------|----------|
| `1.6.3` | `2.0.0` | -1 | ✓ (update available) |
| `2.0.0` | `1.6.3` | 1 | ✓ |
| `2.0.0` | `2.0.0` | 0 | ✓ |
| `1.6.3` | `1.6.3-5-gabc1234` | -1 | ✓ (dev/newer on same base) |
| `2.0.0-1-gSHA-prerelease` | `2.0.0` | 1 | ✓ |
| `2.0.0` | `2.0.0-1-gSHA-prerelease` | -1 | ✓ |
| `2.0.0-1-gSHA-prerelease` | `1.6.3` | 1 | ✓ |
| `1.6.3` | `2.0.0-1-gSHA-prerelease` | -1 | ✓ |

**Строкового бага «2.x < 1.x» нет** — сравнение покомponentно-числовое.

**Замечание (не блокер для 2.0.0):** `/releases/latest` отдаёт только **stable** релизы. Пока последний stable — `v1.6.3`, пререлиз `v2.0.0-…-prerelease` в popup не появится; после stable `v2.0.0` — появится корректно.

**Потенциальный дефект (не чинили):** если когда-нибудь появятся semver-пререлизы вида `2.0.0-rc.1` vs stable `2.0.0`, текущая логика посчитает rc **новее** stable (suffix > no suffix). Минимальная правка — отдельная ветка для `-rc`/`-beta` или библиотека semver; сейчас теги проекта этот формат не используют.

---

## 3. Легаси Win7 (go1.20): страж `tools/win7guard`

**Коммит стража:** `b3300560` · **CI:** `.github/workflows/ci.yml:195-200` (job `test`), `:495-499` (job `build-win7`).

### 3.1. Локальный прогон (2026-09-19)

```bash
go run ./tools/win7guard
# ✅ win7guard: no go1.21+ constructs in the Win7 build set (599 files scanned)
```

### 3.2. Go-файлы, изменённые после `v1.6.3`

```bash
git diff v1.6.3..HEAD --name-only -- '*.go' | wc -l
# 235
```

**Нарушений go1.20 в коде, добавленном после v1.6.3: нет.** Страж смотрит AST набора пакетов для `windows/386` с ReleaseTags ≤ go1.20; наивный grep по `slices`/`maps` в diff даёт ложные срабатывания (комментарии, JSON-теги `maps_to`, локальные переменные `maps`).

Если страж когда-нибудь упадёт — смотреть вывод `go run ./tools/win7guard` (file:line + конструкция); обходы задокументированы в `.github/workflows/README.md:53-57`.

---

## 4. Процедура релиза (чек-лист)

Источник: `docs/RELEASE_PROCESS.md`, `.github/workflows/README.md`.

### Stable `v2.0.0`

- [ ] CI зелёный на `develop`: `gh workflow run ci.yml --ref develop -f run_mode=tests`, затем `-f run_mode=build` (локально `go test ./...` / Win7 **не** гонять)
- [ ] `develop` — потомок последнего stable (`git describe --tags --exclude='*-prerelease'`)
- [ ] `RequiredCoreVersion` соответствует протестированному sing-box-lx (§5.1)
- [ ] `bin/wizard_template.json` на `develop` — финальное состояние
- [ ] `git mv docs/release_notes/upcoming.md docs/release_notes/2-0-0.md`, вычистить, создать пустой `upcoming.md` по шаблону
- [ ] Обновить `RELEASE_NOTES.md`
- [ ] Коммит `docs(release): v2.0.0 notes` → push `develop`
- [ ] `git checkout main && git pull --ff-only && git merge --no-ff develop`
- [ ] Push `main`, **отдельно** `git tag -a v2.0.0 && git push origin v2.0.0` (не `--tags` вместе с branch push)
- [ ] `gh run watch` — 5 zip + checksums, release published
- [ ] **Post-flight:** merge `main` → `develop`, bump `RequiredTemplateRef` source-default (§1.5)
- [ ] `gh release view v2.0.0` → `isLatest:true, isPrerelease:false`

### Prerelease (если нужен до stable)

- [ ] CI green на `develop`
- [ ] SLUG локально: `VER="$(git describe --tags --always --exclude='*-prerelease')-prerelease"` → `docs/release_notes/${VER#v}.md` с `-` вместо `.`
- [ ] Route A: достаточно актуального `upcoming.md` (CI fallback для prerelease)
- [ ] `gh workflow run ci.yml --ref develop -f run_mode=prerelease`
- [ ] CI сам создаёт tag `vX.Y.Z-N-gSHA-prerelease` и GitHub Release `prerelease=true`

---

## 5. `docs/release_notes/upcoming.md`

| Проверка | Статус |
|----------|--------|
| Секция `## EN` | ✓ есть (`:9`) |
| Секция `## RU` | ✓ есть (`:60`) |
| 2.0.0 как начало экосистемы с LxBox (EN) | ✓ `:5`, `:11` — «one contract with LxBox» |
| 2.0.0 как начало экосистемы с LxBox (RU) | ✓ `:5`, `:62` — «один контракт с LxBox» |
| Подразделы Highlights/Fixes/Technical + Основное/Исправления/Техническое | ✓ заполнены |

**Дописывать не потребовалось** — черновик уже подаёт 2.0.0 как единую экосистему с LxBox на обоих языках.

---

## 6. Риски перед тегом v2.0.0 (кратко)

1. **Версия** — меняется тегом + ldflags; literal в коде не нужен.
2. **Автообновление** — `CompareVersions` корректен для 1.6.3→2.0.0; до stable-тега GitHub `/latest` останется на 1.6.3.
3. **Win7** — нарушений go1.20 в diff с v1.6.3 нет; страж в CI на каждом PR (`test` job).
4. **Release notes** — `upcoming.md` готов к переносу в `2-0-0.md`.
