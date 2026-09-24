# GitHub Actions — CI/CD (кратко)

Документ описывает новую логику CI для проекта **Sing-Box Launcher**: три режима запуска, унифицированная генерация версий и поведение релизов.

---

## 🔧 Политика CI — что и когда

- push в `main` / PR в `main` → **только тесты**
- push тега `v*` → **build + release (stable)**. Тег нужно пушить **отдельно** от ветки (см. ниже).
- ручной запуск `workflow_dispatch` → управляемо через `run_mode`:
  - `tests` — только тесты
  - `build` — сборка артефактов (без релиза)
  - `prerelease` — сборка + создание prerelease (аннотированный тег + релиз)

Параметры: `run_mode` (обязательный выбор), `skip_tests` (boolean), `target` (строка, необязательно).

**target** — какие сборки запускать (через пробел: `macOS`, `Win64`, `Win7`). Пусто = все три. `Win64` включает и установщик. Пример: `macOS Win64` — только macOS и Win64, без Win7.

---

## 🧩 Как генерируются версии

- На тегах `vX.Y.Z`:
  - version = `vX.Y.Z`
  - prerelease = `false`
  - tag = `vX.Y.Z`
- Ручной `prerelease`:
  - version = `git describe --tags --always --exclude='*-prerelease'` + `-prerelease` (например `v0.8.0-16-gc185054-prerelease`)
  - prerelease = `true`
  - создаётся аннотированный тег с этим именем и пушится (фильтр в локальных сборках: `--exclude='*-prerelease'`)
- Ручной `build` (без релиза):
  - version = `dev.<branch-sanitized>.<sha7>` (без `v.`)
  - prerelease = `false`
  - тег не создаётся

---

## 🚀 Job‑ы и артефакты

- Test job: запускается по push в main, PR, или вручную (run_mode=tests).
- Build job'ы (при теге `v*` или run_mode=build|prerelease):
  - **build-darwin** — macOS (универсальный .app + Catalina Intel-only); запускается, если `target` пусто или содержит `macOS`.
  - **build-windows** — Win64 (.exe); если `target` пусто или содержит `Win64`.
  - **build-win7** — Win7 x86; если `target` пусто или содержит `Win7`.
  - **build-windows-installer** — установщик Inno Setup `*-win64-setup.exe` (SPEC 140); после `build-windows`, то же условие (`Win64`). Набор готовит `build/installer/stage_win64_full.sh` (тот же, что у `win64-full.zip`, без `portable.txt`), `VersionInfoVersion` = `X.Y.Z.N` из `git describe --tags --long --match "v[0-9]*" --exclude "*-prerelease"` (не разобралось — job падает), ISCC из образа `windows-latest`, запасной путь — `choco install innosetup`. Отдельный шаг компилирует вариант `/DDaemonService` на пустых заглушках.
  На `macos-latest` два артефакта: универсальный и `*-macos-catalina.zip`.
- Release job: запускается после успешного выполнения хотя бы одного build для тегов (stable) или при ручном `run_mode=prerelease`; подтягивает только артефакты тех сборок, что реально запускались. Установщик поднимается в корень релиза и попадает в `checksums.txt`; в `build` он только артефакт на 30 дней.

Артефакты: `artifacts-darwin`, `artifacts-windows`, `artifacts-windows-installer`, `artifacts-macos-catalina`, `artifacts-windows-win7-32`.

---

## 🛡 Страж легаси-сборки Win7

Шаг **Win7 (go1.20) constructs guard** (`go run ./tools/win7guard`) стоит в двух джобах: в `test` (Ubuntu, то есть на каждом PR и при `run_mode=tests`) и в `build-win7` перед установкой MSYS2. Он красит джобу, если в коде, попадающем в Win7-сборку, появились конструкции Go 1.21+: импорт `slices`/`maps`, builtin `min`/`max`/`clear`, `range` по целому, `Request.PathValue`.

Набор файлов считается по build-тегам для `windows/386` с релизными тегами не выше `go1.20`, поэтому файлы за `//go:build darwin` и `//go:build go1.22` не проверяются — их в Win7-сборке нет. Разрешённые обходные пути: локальный хелпер (`ui/clash_api_tab_helpers.go`) или близнец за build-тегом (`core/debugapi/pathparam_legacy.go`). Запускается и локально из корня: `go run ./tools/win7guard`.

---

## 🧪 Примеры команд (cli)

### Стабильный релиз (тег)

Чтобы запустилась сборка и создание Release, тег должен уйти отдельным push. **Нельзя** пушить ветку и теги одной командой (`git push origin main --tags`) — в этом случае GitHub может создать только событие по ветке, и пойдут лишь тесты.

Правильная последовательность:

1. `git push origin main`
2. `git push origin vX.Y.Z`   (например `v0.8.4`)

### Пререлиз и build

- Пререлиз с тестами:
  gh workflow run ci.yml --ref develop -f run_mode=prerelease -f skip_tests=false
- Пререлиз без тестов:
  gh workflow run ci.yml --ref develop -f run_mode=prerelease -f skip_tests=true
- Ручной build:
  gh workflow run ci.yml --ref develop -f run_mode=build -f skip_tests=true
- Ручной build только Win7:
  gh workflow run ci.yml --ref develop -f run_mode=build -f skip_tests=true -f target=Win7
- Ручной build только macOS и Win64 (без Win7):
  gh workflow run ci.yml --ref develop -f run_mode=build -f skip_tests=true -f "target=macOS Win64"
- Тесты вручную:
  gh workflow run ci.yml --ref develop -f run_mode=tests

### 🔍 Запуск `golangci-lint`

- Вручную (через `workflow_dispatch`):
  gh workflow run golangci-lint.yml --ref develop
- Автоматически при PR: workflow настроен на срабатывание при событиях `opened`, `reopened`, `synchronize` на pull request — ничего дополнительно делать не нужно.

> Примечание: workflow выполняется по matrix (`ubuntu-24.04`, `macos-latest`, `windows-latest`) и использует Go 1.25; для локальной проверки можно запустить `golangci-lint` локально (`golangci-lint run`) после `go mod tidy`.

### 🤖 Dependabot

- Настройка: `.github/dependabot.yml` — обновления для `gomod` (еженедельно), лимит открытых PR — 10, метки `dependencies`, ревьювер `Leadaxe`.
- Для ручного контроля: используйте веб-интерфейс GitHub → Security / Dependabot или создавайте PR с обновлением `go.mod` вручную.

---

## ⚠️ Важные замечания

- **Теги для stable-релиза:** не использовать `git push origin main --tags`. Пушить сначала `main`, затем отдельно тег (`git push origin vX.Y.Z`), иначе CI запустится только по ветке и build/release не выполнятся.
- Для пуша тегов и создания релизов `GITHUB_TOKEN` должен иметь `contents: write` (в workflow уже выставлено).
- Мы создаём аннотированные теги для prerelease для удобства отладки (`git tag -a`).
- Проверка существования тега сейчас локальная; можно дополнительно `git fetch --tags` или `git ls-remote` для проверки remote.
- `build` — это не `release`. Если хотите автоматизировать публикацию при `build`, измените правила в `meta/release`.

