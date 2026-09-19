# ТЗ 5 · Мелочи CI

**Цель одной фразой:** поднять `codecov-action` до v5, пригвоздить раннер к
конкретной версии Ubuntu вместо плавающего `ubuntu-latest` и встроить
греп-стража, который ловит запрещённые для легаси-сборки Win7 конструкции
раньше, чем на них упадёт сборка тулчейном go1.20.

---

## 0. Контекст для того, кто репозиторий видит впервые

Приложение — десктопный лаунчер VPN-ядра `sing-box` на Go + Fyne. Собирается
под macOS, Windows 64 и **Windows 7 32-бит**.

Про Win7 важно: у него **отдельный модульный файл** `go.win7.mod` и
**отдельный тулчейн Go 1.20.14**, потому что новые Go на Win7 не работают.
Общий код собирается обоими тулчейнами, а значит **не имеет права**
пользоваться тем, чего в Go 1.20 ещё не было:

- builtin `min` / `max` (появились в 1.21);
- пакеты `slices` и `maps` (1.21);
- `http.Request.PathValue` (1.22);
- `range` по целому числу (1.22);
- builtin `clear` (1.21).

Норма записана в `AGENTS.md:107`. Обходные пути в репозитории уже есть —
например `slicesContains` в `ui/clash_api_tab_helpers.go:42` с комментарием,
прямо объясняющим, почему не взят пакет `slices`.

**Беда:** Win7-джоба идёт **только на тег или на ручной запуск с целью Win7**.
Значит, нарушение нормы может прожить в `develop` неделями и всплыть ровно
в момент выпуска релиза — в худшую из возможных минут.

---

## 1. Границы

### Входит

Только `.github/workflows/` и, при необходимости, вспомогательный скрипт
стража.

1. `codecov/codecov-action` — с `@v4` на `@v5`.
2. `ubuntu-latest` → пин конкретной версии (`ubuntu-24.04`).
3. Новый шаг-страж запрещённых для go1.20 конструкций.

### НЕ трогать

- **`.github/workflows/claude.yml`** — запрещено, даже если там тот же
  `ubuntu-latest` (он есть, строка 48). Обойти стороной.
- `macos-latest` и `windows-latest` — в объём **не входят**. Пин раннера
  заказан только для Ubuntu; менять раннеры сборок без спроса нельзя, от них
  зависят подписи артефактов и версии SDK.
- Версии Go, состав джоб, триггеры, логика релиза.
- `go.mod`, `go.win7.mod`.
- Любой Go-код. Страж только **находит**; если он что-то найдёт на текущем
  дереве — это отдельная задача, см. §3 п. 5.

---

## 2. Разведка (проверено, ссылки точные)

### 2.1 Файлы

| Файл | Строк | Трогаем? |
|------|-------|----------|
| `.github/workflows/ci.yml` | 871 | **да** |
| `.github/workflows/contract.yml` | 70 | **да** (один раннер) |
| `.github/workflows/golangci-lint.yml` | 69 | **да** (матрица) |
| `.github/workflows/claude.yml` | 78 | **НЕТ** |
| `.github/workflows/README.md` | 101 | да, если поведение стало заметным |

Режимы запуска описаны в `.github/workflows/README.md`. Коротко: push/PR в
`main` → тесты; тег `v*` → сборка и релиз; ручной запуск с
`run_mode` ∈ {`tests`, `build`, `prerelease`} и необязательным
`target` ∈ {`macOS`, `Win64`, `Win7`}.

### 2.2 codecov

| Где | Сейчас |
|-----|--------|
| `.github/workflows/ci.yml:244` | `uses: codecov/codecov-action@v4` |

Единственное вхождение во всём репозитории. Строка `:248` рядом —
`name: codecov-umbrella`, это параметр, не действие.

**Что учесть при переходе на v5:** пятая версия перешла на новый загрузчик
(Codecov CLI) и переименовала часть входов относительно v4. Перед правкой
сверить фактический набор `with:` в шаге со списком входов v5 в README
действия; молча поменять цифру и оставить старые ключи — получить
предупреждения или тихую потерю отчёта.

### 2.3 Раннеры

| Файл:строка | Значение | В объёме? |
|-------------|----------|-----------|
| `ci.yml:91` | `ubuntu-latest` (джоба `meta`) | **да** |
| `ci.yml:152` | `${{ matrix.os }}` (джоба `test`), матрица — `:156` `[ubuntu-latest, macos-latest, windows-latest]` | **да, только элемент ubuntu** |
| `ci.yml:281` | `macos-latest` (`build-darwin`) | нет |
| `ci.yml:360` | `windows-latest` (`build-windows`) | нет |
| `ci.yml:421` | `windows-latest` (`build-win7`) | нет |
| `ci.yml:563` | `ubuntu-latest` (`release`) | **да** |
| `contract.yml:39` | `ubuntu-latest` | **да** |
| `golangci-lint.yml:27` | `${{ matrix.os }}`, матрица — `:17` `ubuntu-latest`, `:20` `macos-latest`, `:23` `windows-latest` | **да, только элемент ubuntu** |
| `claude.yml:48` | `ubuntu-latest` | **НЕТ, не трогать** |

Итого к правке: `ci.yml:91`, `ci.yml:156` (один элемент), `ci.yml:563`,
`contract.yml:39`, `golangci-lint.yml:17`. Пять мест.

### 2.4 Версии действий (для контекста, менять только codecov)

| Действие | Версия | Где |
|----------|--------|-----|
| `actions/checkout` | `@v6` | `ci.yml:99,160,285,364,427,577`; `contract.yml:44`; `golangci-lint.yml:30` |
| `actions/setup-go` | `@v6` | `ci.yml:165,290,369,432`; `contract.yml:47`; `golangci-lint.yml:45` |
| `actions/upload-artifact` | `@v6` | `ci.yml:338,347,408,553` |
| `actions/download-artifact` | `@v6` | `ci.yml:605` |
| `msys2/setup-msys2` | `@v2` | `ci.yml:475` |
| `softprops/action-gh-release` | `@v2` | `ci.yml:796` |
| `golangci/golangci-lint-action` | `@v8` (линтер пин `v2.12.2`, `:65`) | `golangci-lint.yml:61` |
| **`codecov/codecov-action`** | **`@v4`** | **`ci.yml:244`** |

### 2.5 Win7-джоба — где именно врезать стража

Джоба `build-win7`, `.github/workflows/ci.yml:414`. Условие запуска — `:416–420`
(тег `v*` **или** ручной запуск `build`/`prerelease` с целью `Win7`).
Раннер — `:421` `windows-latest`.

Последовательность шагов:

| Строка | Шаг |
|--------|-----|
| 431–435 | `Set up Go (legacy)` — `actions/setup-go@v6`, `go-version: '1.20.14'` |
| 457 | `go mod download "-modfile=go.win7.mod"` |
| 459–460 | `Populate Win7 go.sum entries` — `go get "-modfile=go.win7.mod" ./...` |
| **467–471** | `Restore Go 1.20 directive in go.win7.mod` — pwsh, возвращает директиву `go 1.20` |
| 474 | `Setup MSYS2 (MINGW32)` |
| 485–548 | `Build Windows 7 x86 (Fyne +rsrc@fixed)` — MSYS2, `GOOS=windows GOARCH=386 CGO_ENABLED=1`, `go build -modfile=go.win7.mod -tags desktop … .` (`:542–548`) |

**Рекомендуемая точка врезки: после шага на строке 471, перед шагом на 474.**
Рабочий каталог — корень репозитория (checkout по умолчанию, у джобы
`working-directory` не задан).

**Но есть вопрос посерьёзнее места.** Джоба `build-win7` идёт только на теге
и ручном запуске — то есть страж там сработает **уже на выпуске**, когда
чинить поздно. Заказчик просил «где встроить» — ответ по букве выше, но
**правильнее поставить страж ещё и в джобу, которая ходит часто**:
`test` (`ci.yml:144`, идёт на PR и на ручном `run_mode=tests`) либо
`golangci-lint`. Это **изменение объёма** — вынести владельцу решением, а
по умолчанию сделать по букве и пометить в отчёте.

**Что попадает в Win7-бинарь** (важно для стража, чтобы не ловить ложное):

- собирается корневой main-пакет под `windows/386` с тегом `desktop`
  тулчейном **1.20.14**;
- **не** собираются файлы с тегом `//go:build darwin` (весь стек демона и
  gRPC, ~24 файла);
- **не** собираются файлы за версионными тегами: `core/debugapi/pathparam_go122.go`
  (`//go:build go1.22`) и `internal/debuglog/crash_go123.go` (`//go:build !go1.23`
  у пары) — вместо них берутся легаси-близнецы
  `core/debugapi/pathparam_legacy.go` и `internal/debuglog/crash_legacy.go`;
- при этом `go get -modfile=go.win7.mod ./...` (`:460`) **резолвит все**
  пакеты модуля независимо от тегов — это про зависимости, не про компиляцию.

### 2.6 Стиль шагов-стражей в репозитории

Своего греп-стража по исходникам **нет ни одного**. Образцы house style —
два шага, на которые надо равняться формой (`set -euo pipefail`, понятное
сообщение, явный `exit 1`, зелёная строка на успехе):

```250:272:.github/workflows/ci.yml
      - name: Check go mod tidy is clean (Ubuntu)
        if: matrix.os == 'ubuntu-latest'
        shell: bash
        run: |
          set -euo pipefail

          echo "Running go mod tidy check..."
          go mod tidy

          if ! git diff --exit-code; then
            echo ""
            echo "❌ go.mod / go.sum are not tidy"
            echo ""
            echo "Please run locally:"
            echo "  go mod tidy"
            echo "  git commit -am \"go mod tidy\""
            echo ""
            echo "Diff:"
            git diff
            exit 1
          fi

          echo "✅ go.mod and go.sum are tidy"
```

```57:67:.github/workflows/contract.yml
      - name: Generated docs are in sync with the registry
        run: |
          if ! git diff --exit-code contract/docs/generated; then
            echo "::error::contract/docs/generated устарел. Запустите 'go generate ./contract/...' и закоммитьте результат."
            exit 1
          fi
```

Есть и третий образец — проверка инструментом, а не грепом
(`ci.yml:235–240`, `go run ./tools/l10n/l10n_check --strict`). Если страж
выйдет сложнее десятка строк YAML, **лучше повторить этот приём**: положить
скрипт в репозиторий и звать его одной строкой из workflow.

### 2.7 Состояние дерева — страж сегодня **зелёный**

Проверено по не-тестовым `.go`:

| Конструкция | Найдено |
|-------------|---------|
| `import "slices"` / `"maps"` | **ноль** |
| builtin `min(` / `max(` | **ноль** (только в комментариях: `core/services/file_service.go:278`, `ui/configurator/outbounds_configurator/configurator.go:256`; и локальные хелперы в тестах — `core/template/gate_migration_test.go:68,93`, `internal/platform/proclist_unix_test.go:21,34`) |
| `range` по целому | **ноль** |
| builtin `clear(` | **ноль** (`ui/clash_api_tab.go:138` — это `p.clear()`, вызов метода) |
| `PathValue` | только `core/debugapi/pathparam_go122.go:13`, файл за тегом `//go:build go1.22` — **в Win7-сборку не входит** |

Последняя строка — главная причина, почему страж нельзя писать наивным
грепом: `PathValue` в репозитории есть **законно**.

---

## 3. Ловушки

1. **`claude.yml` не трогать.** Соблазн «заодно пройтись `sed`-ом по всем
   `ubuntu-latest`» ломает это правило. Править адресно.
2. **Ложное срабатывание на легально исключённом коде.** Страж обязан
   пропускать: (а) `*_test.go`; (б) файлы за тегами `//go:build darwin` и
   `//go:build go1.22` / `go1.23`; (в) комментарии. Простое
   `rg 'min\('` завалит сборку на комментарии в
   `core/services/file_service.go:278`.
3. **`min`/`max` — слова общего употребления.** `p.max(`, `math.Max(`,
   `r.min` — это не builtin. Ловить **вызов builtin** (идентификатор не
   после точки), а не подстроку.
4. **`clear(` то же самое.** `p.clear()` в `ui/clash_api_tab.go:138` —
   метод, легален.
5. **`slices`/`maps` ловить по строке `import`, а не по имени пакета.**
   `maps` встречается в прозе комментариев.
6. **Страж без зубов бесполезен.** Обязательно проверить его **обратно**:
   временно вписать в общий файл `x := min(1, 2)`, убедиться, что джоба
   **красная**, откатить. Без этой проверки страж — украшение.
7. **Точка врезки решает, когда сигнал приедет.** См. §2.5: в `build-win7`
   он приедет на релизе. Это по букве задания, но отметить в отчёте.
8. **codecov v5 — не просто цифра.** См. §2.2: сверить набор входов.
9. **Win7-джобу локально не запустить и проверять локально нельзя.**
   Сборка/`vet` с `go.win7.mod` локально **запрещены**. Проверка — только CI.
10. **Матрица в `golangci-lint.yml`** задана списком `include` (строки
    17–24): раннер и версия Go идут парой на каждый элемент. Править только
    элемент ubuntu (`:17`), не сломав отступы `cgo`/`go` под ним.

---

## 4. Шаги

1. `ci.yml:244` — `codecov/codecov-action@v4` → `@v5`, сверить входы шага со
   списком v5.
2. Пять мест из §2.3 — `ubuntu-latest` → `ubuntu-24.04`:
   `ci.yml:91`, элемент матрицы `ci.yml:156`, `ci.yml:563`,
   `contract.yml:39`, `golangci-lint.yml:17`.
   **Внимание:** если где-то рядом есть условие
   `if: matrix.os == 'ubuntu-latest'` (такое есть, например `ci.yml:251`),
   его значение надо поменять **вместе** с матрицей — иначе шаг тихо
   перестанет выполняться. Пройти `rg "ubuntu-latest" .github/workflows/`
   после правки и убедиться, что осталось только `claude.yml`.
3. Написать стража. Шаблон — шаг `bash` в стиле §2.6; если логики больше
   десятка строк, вынести в скрипт (например `build/win7_guard.sh`) и звать
   одной строкой, как это сделано с `tools/l10n`.
   Шаблон сообщения об отказе:

   ```
   ❌ Win7 (go1.20) forbidden construct: <что> at <file:line>
      The Win7 build uses the Go 1.20 toolchain (go.win7.mod).
      Allowed alternatives: a local helper (see ui/clash_api_tab_helpers.go:42),
      or a build-tagged twin (see core/debugapi/pathparam_legacy.go).
   ```

4. Врезать шаг в `build-win7` после строки 471, перед шагом на 474.
5. **Проверить обратно:** временной правкой убедиться, что страж краснеет,
   и откатить правку. Без этого пункт не считается сделанным.
6. Если страж нашёл настоящее нарушение на текущем дереве — **не чинить
   код**, записать в отчёт отдельной строкой.
7. Обновить `.github/workflows/README.md`, если появился шаг, о котором
   стоит знать.

---

## 5. Как проверить

Локально workflow не исполняется. Что можно:

```
go build ./...
rg -n "ubuntu-latest" .github/workflows/
```

Вторая команда после правок обязана показывать **только** `claude.yml:48`.

Валидность YAML — любым локальным парсером (`python3 -c "import yaml,sys;
yaml.safe_load(open('.github/workflows/ci.yml'))"`).

Настоящая проверка — **в CI**:

```
gh workflow run ci.yml --ref develop -f run_mode=tests
```

и отдельно, чтобы поднять саму Win7-джобу со стражем:

```
gh workflow run ci.yml --ref develop -f run_mode=build -f target=Win7
```

Упавшее разбирать: `gh run view <ID> --log-failed`.

**Запрещено локально:** `go test ./...`, `go vet ./...` по репозиторию,
Win7-сборка и `vet` с `go.win7.mod`.

**Запрещено вообще:** запускать `sing-box run`, убивать процессы sing-box
(`pkill`, `killall`) — на машине живой VPN. Тегов не ставить, релиз не
выпускать (`run_mode=prerelease` и `run_mode=build` без явного разрешения
владельца **не запускать**; для проверки стража хватит цели `Win7`, но
согласовать).

---

## 6. Как коммитить

Ветка **только `develop`**. Запрещены `git checkout`/`switch`, `stash`,
`reset`, `rebase`, `clean`, `git add -A`, `git commit -a`. Рабочая копия
разделяемая — чужие незакоммиченные правки не трогать.

Явным списком путей:

```
git commit -m "ci: codecov v5, пин раннера ubuntu-24.04, страж легаси-сборки Win7" -- \
  .github/workflows/ci.yml \
  .github/workflows/contract.yml \
  .github/workflows/golangci-lint.yml \
  .github/workflows/README.md \
  build/win7_guard.sh \
  docs/release_notes/upcoming.md
git push origin develop
```

(Путь скрипта — только если он появился.) В
`docs/release_notes/upcoming.md` — по пункту в `### Technical / Internal` и
`### Техническое / Внутреннее`.

---

## 7. Формат отчёта (≤ 8 строк)

```
1. codecov: v4 → v5; входы шага сверены — да/нет, что поменялось
2. Раннер: мест правлено <N>; условий `if: matrix.os == …` поправлено <M>
3. Страж: где врезан, каким приёмом (inline yaml / скрипт), что ловит
4. Страж проверен обратно (намеренная поломка краснеет): да/нет
5. rg ubuntu-latest .github/workflows/ → остался только claude.yml: да/нет
6. CI: run_mode=tests <ID> — <результат>; Win7 <ID или «не запускал»>
7. Настоящих нарушений go1.20 на дереве: <список file:line или «нет»>
8. Требует решения владельца: <страж в частой джобе; прочее — или «нет»>
```
