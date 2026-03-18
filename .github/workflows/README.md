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

**target** — какие сборки запускать (через пробел: `macOS`, `Win64`, `Win7`). Пусто = все три. Пример: `macOS Win64` — только macOS и Win64, без Win7.

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
  На `macos-latest` два артефакта: универсальный и `*-macos-catalina.zip`.
- Release job: запускается после успешного выполнения хотя бы одного build для тегов (stable) или при ручном `run_mode=prerelease`; подтягивает только артефакты тех сборок, что реально запускались.

Артефакты: `artifacts-darwin`, `artifacts-windows`, `artifacts-macos-catalina`, `artifacts-windows-win7-32`.

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

> Примечание: workflow выполняется по matrix (`ubuntu-latest`, `macos-latest`, `windows-latest`) и использует Go 1.25; для локальной проверки можно запустить `golangci-lint` локально (`golangci-lint run`) после `go mod tidy`.

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

