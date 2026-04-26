# Протокол выпуска релизов и пререлизов

Документ описывает, как выпустить **stable-релиз** (`vX.Y.Z`) и **пререлиз** (`vX.Y.Z-N-gSHA-prerelease`). Canonical-source для процедуры: если что-то в других документах противоречит — править здесь, остальное приводить в соответствие.

Смежные документы:
- **`.github/workflows/README.md`** — механика CI: run-modes, генерация версий, job'ы, команды `gh workflow run`.
- **`AGENTS.md`** — общий scope агента, обязанности при закрытии задачи.
- **`docs/release_notes/`** — per-version release notes, источник тела релиза для CI.

---

## 0. Что меняет CI, что делаете вы

CI (`ci.yml`, job `release`) формирует тело релиза из двух частей:

1. **Шапка** — инлайн-шаблон в `body:` (Downloads, инструкции по платформам, Checksums). Шаблонится по `${{ needs.meta.outputs.version }}`. Вас это не касается.
2. **Release notes** — читаются из **`docs/release_notes/<slug>.md`** (шаг `Read release notes`). Где `<slug>` = `VERSION` без ведущего `v`, с `-` вместо `.`.

| VERSION                               | SLUG                                  | файл                                                         |
|---------------------------------------|---------------------------------------|--------------------------------------------------------------|
| `v0.8.7`                              | `0-8-7`                               | `docs/release_notes/0-8-7.md`                                |
| `v0.8.7-1-g50c7352-prerelease`        | `0-8-7-1-g50c7352-prerelease`         | `docs/release_notes/0-8-7-1-g50c7352-prerelease.md`          |

**Файл обязателен.** Если его нет — CI падает на шаге `Read release notes` ещё до создания релиза, с инструкцией в логе. Это by design: per-version файл — единственный источник тела релиза, чтобы исключить протечку контента из агрегированных index-файлов.

Для пререлизов CI дополнительно префиксит тело баннером `> ⚠️ **Pre-release build** off \`$VERSION\` — for testing only, ...` — руками писать не надо.

---

## 1. Stable-релиз — `vX.Y.Z`

### 1.1. Pre-flight

1. На `develop` всё зелёное (`go build ./... && go test ./... && go vet ./...`). Линтер (`golangci-lint run`) — как минимум на изменённых пакетах.
2. `develop` — прямой потомок последнего stable-тега. Проверка:
   ```bash
   git fetch --tags
   git describe --tags --exclude='*-prerelease'
   # Должно быть что-то вроде v0.8.7-N-gSHA; если далеко отошло — подумайте, нужен ли промежуточный пререлиз
   ```
3. `docs/release_notes/upcoming.md` накопил пункты по всем фичам/багам, вошедшим в релиз. Формат — как в существующих файлах: `Highlights (EN)` + `Основное (RU)`, пункты со ссылками на SPEC'и.
4. **Pinned dependencies** (см. §5):
   - **`internal/constants.RequiredCoreVersion`** — соответствует версии sing-box, на которой проводилось финальное QA релиза. Если за время разработки в SagerNet вышло что-то критичное (CVE, breaking-fix) и решили бампнуть — отдельный коммит `chore(core): pin sing-box vX.Y.Z` **до** тега, плюс пометка в release-notes.
   - **`internal/constants.RequiredTemplateRef`** — source-default. Вручную не редактировать; CI-сборки инжектят SHA через `-ldflags`. Source-default бампится в §1.5 (post-flight).
   - **`bin/wizard_template.json`** на `develop` отражает финальное состояние шаблона. Никаких полузатёртых правок от чужого PR.

### 1.2. Перенос upcoming.md → per-version

1. `git mv docs/release_notes/upcoming.md docs/release_notes/X-Y-Z.md` (для v0.8.8 → `0-8-8.md`).
2. Причесать: убрать черновые TODO, добить недостающее, структурировать по подсекциям (`Resilience & observability`, `Security`, `Fixed`, `Template defaults`, `Migration notes` и т.п. — см. `0-8-7.md` как образец).
3. **Создать новый пустой `upcoming.md`** из шаблона:
   ```markdown
   # Upcoming release — черновик

   Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

   ## EN
   ### Highlights
   -

   ### Technical / Internal
   -

   ## RU
   ### Основное
   -

   ### Техническое / Внутреннее
   -
   ```
4. Обновить `RELEASE_NOTES.md` (index репо): добавить строку в таблицу «Последний релиз / Latest release» и, опционально, короткую «Выжимку (RU) / Highlights (EN)» вверху.
5. Один коммит в `develop`: `docs(release): v0.8.8 notes`.

### 1.3. Мердж в main и тег

CI запускается **только на push тега**, и тег нужен **отдельной командой** (см. `.github/workflows/README.md` §⚠️):

```bash
git checkout main
git pull --ff-only
git merge --no-ff develop -m "Merge branch 'develop' into main"
git push origin main

# Теперь отдельно — тег
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin vX.Y.Z
```

**Не** делать `git push origin main --tags` — GitHub в этом случае отправит только событие по ветке, и запустятся одни тесты, без build/release.

> ⚠️ После этого шага тег сидит на merge-коммите в `main`, который **не** является предком `develop`. Пока не выполнен §1.5 (merge `main` → `develop` обратно) — `develop` отстаёт от тега, `git describe` на develop возвращает старый тег, следующий пререлиз получит кривое имя. **§1.5 — обязательный, не забыть.**

### 1.4. Проверка CI

```bash
gh run list --workflow=ci.yml --limit 3
gh run watch <RUN_ID> --exit-status
```

На финише ожидаем:
- 4 артефакта: `macos.zip`, `macos-catalina.zip`, `win64.zip`, `win7-32.zip` + `checksums.txt`.
- Release опубликован (`isDraft=false`, `isPrerelease=false`).
- Тело содержит Downloads + Checksums + вашу `X-Y-Z.md` без посторонних блоков.

### 1.5. Post-flight: вернуть main в develop

После релиза merge-коммит в `main`, на котором сидит тег, **не** является предком `develop`. Если это не починить, следующая работа на develop будет идти «не от тега», `git describe` на develop будет возвращать старый тег, и имя следующего пререлиза станет кривым.

```bash
git checkout develop
git fetch origin
git merge --no-ff origin/main -m "chore: merge main (vX.Y.Z tag) back into develop"
git push origin develop
# Проверка: git describe на develop теперь показывает vX.Y.Z-N-gSHA
```

Если с момента релиза в `develop` уже успели уехать коммиты и есть желание линейной истории — можно `git reset --hard vX.Y.Z && git cherry-pick <коммиты>` + `--force-with-lease`, но это разрушающая операция и делается осознанно.

**Затем — бампнуть source-default `RequiredTemplateRef`** в `internal/constants/constants.go` на свежий `origin/main` HEAD. Это нужно для `go run .` и нестандартных билдов без CI-инжекции; CI-сборкам безразлично.

```bash
NEW_REF="$(git rev-parse origin/main)"
# Открыть internal/constants/constants.go и заменить значение RequiredTemplateRef на $NEW_REF.
git commit -am "chore(constants): bump RequiredTemplateRef source-default"
git push origin develop
```

См. §5 «Pinned dependencies» — почему source-default нужен и как он отличается от ldflags-инжекции.

### 1.6. Verify

- `gh release view vX.Y.Z --json isLatest,isPrerelease,isDraft` → `{isLatest:true, isPrerelease:false, isDraft:false}`.
- Один из артефактов действительно запускается локально.
- Скрипт установки macOS работает: `curl ... install-macos.sh | bash -s -- vX.Y.Z`.

---

## 2. Пререлиз — `vX.Y.Z-N-gSHA-prerelease`

Используется, когда хочется собрать билды поверх `develop` для ручного тестирования новой фичи, но ещё не готовы к stable.

### 2.1. Pre-flight

1. На `develop` всё зелёное.
2. `develop` — потомок последнего stable-тега (иначе CI сгенерит кривой describe).
3. **Если решили бампнуть `RequiredCoreVersion`** для пре-релиза (например, тестируете обновление под новый sing-box) — отдельный коммит **до** запуска workflow, чтобы CI собрал бинарник с новой константой. `RequiredTemplateRef` руками не трогаем — CI инжектит SHA текущего HEAD автоматически.
3. Вычислить SLUG **локально**, ровно как это сделает CI:
   ```bash
   git fetch --tags
   VER="$(git describe --tags --always --exclude='*-prerelease')-prerelease"
   SLUG="${VER#v}"; SLUG="${SLUG//./-}"
   echo "docs/release_notes/${SLUG}.md"
   # Например: docs/release_notes/0-8-7-1-g50c7352-prerelease.md
   ```

### 2.2. Создать файл release notes

**Это обязательный шаг.** Без файла CI упадёт на `Read release notes`.

1. Создать `docs/release_notes/<SLUG>.md` (путь из 2.1).
2. Содержимое: что нового **именно в этом пререлизе поверх последнего stable** — обычно 1–3 пункта. Формат:
   ```markdown
   ## Highlights (EN)

   - **<feature>** — one-line summary. See [SPEC NNN](../../SPECS/NNN-.../SPEC.md).

   Everything from **vX.Y.Z** is included — see the [vX.Y.Z release notes](https://github.com/Leadaxe/singbox-launcher/releases/tag/vX.Y.Z).

   ## Основное (RU)

   - **<фича>** — одно предложение. См. [SPEC NNN](../../SPECS/NNN-.../SPEC.md).

   Всё из **vX.Y.Z** уже внутри — см. [release notes vX.Y.Z](https://github.com/Leadaxe/singbox-launcher/releases/tag/vX.Y.Z).
   ```
3. Коммит на `develop`: `docs(release): notes for <SLUG>`. Запушить.

> Баннер `⚠️ Pre-release build` добавляет **CI** автоматически — в файл его писать не надо.

### 2.3. Запустить workflow

```bash
gh workflow run ci.yml --ref develop -f run_mode=prerelease -f skip_tests=false
# Если уверены в тестах и хочется быстрее:
gh workflow run ci.yml --ref develop -f run_mode=prerelease -f skip_tests=true
# Только определённые платформы:
gh workflow run ci.yml --ref develop -f run_mode=prerelease -f skip_tests=true -f "target=macOS Win64"
```

### 2.4. Дождаться сборки

```bash
# ID последнего запущенного run:
RUN_ID="$(gh run list --workflow=ci.yml --limit 1 --json databaseId -q '.[0].databaseId')"
gh run watch "$RUN_ID" --exit-status
```

При успехе CI сам:
- создаст аннотированный тег `vX.Y.Z-N-gSHA-prerelease`;
- создаст GitHub Release с `prerelease=true`;
- нальёт в тело: Downloads + Checksums + баннер `⚠️ Pre-release build` + содержимое вашего `<SLUG>.md`;
- приложит артефакты + `checksums.txt`.

> Раньше релиз создавался в **draft** и нужно было вручную снимать draft + править body. С новым CI это не требуется: `isDraft=false` из коробки, body уже чистый.

### 2.5. Verify

```bash
gh release view "<TAG>" --json isDraft,isPrerelease,name,url
# Ожидаем: {"isDraft":false, "isPrerelease":true, ...}
```

---

## 3. Траблшутинг

### CI падает на «Release notes file not found»

Это ровно то, что мы документируем — файл `docs/release_notes/<slug>.md` обязателен. Создайте его (см. §1.2 или §2.2), запушьте в `develop`, перезапустите workflow / перепушьте тег.

### `git describe` на develop возвращает старый тег

Значит после прошлого релиза `main` не был слит обратно в `develop`. Либо делайте §1.5, либо `git merge --no-ff origin/main` прямо сейчас одним коммитом. Пока не починено, пререлиз сгенерирует имя от старого тега.

### Выпустили релиз, а в теле посторонние блоки чужих версий

Такого не должно быть: CI читает только `docs/release_notes/<slug>.md`. Если увидели — проверить:
1. В логах шага `Read release notes` есть строка `✓ Using release notes from: ...` с ожидаемым путём.
2. Сам файл `docs/release_notes/<slug>.md` не содержит посторонних блоков.
3. Горячий фикс для уже опубликованного релиза: `gh release edit <tag> --notes-file <clean-body>.md`.

### Запушили `main` и тег одной командой, build не стартовал

См. `.github/workflows/README.md` §⚠️ — GitHub в этом случае шлёт только событие по ветке. Перепушьте тег отдельно:
```bash
git push origin vX.Y.Z
```
Workflow стартует заново.

### Тег уже существует, нужно перевыпустить

Удалять тег и релиз — последняя мера. Если действительно нужно:
```bash
gh release delete vX.Y.Z --yes
git push --delete origin vX.Y.Z
git tag -d vX.Y.Z
# Потом повторить §1.3
```
Людей, которые уже скачали предыдущий артефакт, это не затронет, но crc/checksums другим пользователям не совпадут.

---

## 4. Чеклист для агента (копируй в ответ пользователю)

### Stable vX.Y.Z
- [ ] `develop` зелёная, descendant от прошлого stable-тега.
- [ ] `RequiredCoreVersion` соответствует протестированной sing-box версии (см. §1.1, §5).
- [ ] `bin/wizard_template.json` в `develop` отражает финальное состояние шаблона.
- [ ] `upcoming.md` → `docs/release_notes/X-Y-Z.md`, причёсан.
- [ ] Новый пустой `upcoming.md` создан.
- [ ] `RELEASE_NOTES.md` index обновлён.
- [ ] Коммит `docs(release): vX.Y.Z notes` запушен.
- [ ] `main` ← merge `develop`, запушен; тег `vX.Y.Z` запушен **отдельной командой**.
- [ ] `gh run watch` зелёный, 5 артефактов в релизе.
- [ ] **`main` слит обратно в `develop`** (§1.5) — без этого шага develop «не от тега».
- [ ] **Source-default `RequiredTemplateRef` забампен** на новый `origin/main` HEAD (§1.5, §5).
- [ ] `git describe` на develop показывает `vX.Y.Z-0-...` или `vX.Y.Z`.
- [ ] `gh release view vX.Y.Z` → `isLatest:true`.

### Prerelease
- [ ] `develop` зелёная, descendant от прошлого stable-тега.
- [ ] Если бампали `RequiredCoreVersion` — коммит запушен **до** workflow (§2.1, §5).
- [ ] SLUG посчитан локально (`git describe ... + '-prerelease'`).
- [ ] `docs/release_notes/<SLUG>.md` создан, содержит 1–3 пункта о новом в этом пререлизе.
- [ ] Коммит запушен в `develop`.
- [ ] `gh workflow run ci.yml --ref develop -f run_mode=prerelease` запущен.
- [ ] `gh run watch` зелёный.
- [ ] `gh release view <TAG>` → `isDraft:false, isPrerelease:true`.

---

## 5. Pinned dependencies (`RequiredCoreVersion`, `RequiredTemplateRef`)

Каждая версия лаунчера тестируется на конкретной паре (ядро sing-box, шаблон wizard'а). Чтобы пользователь получал именно эту пару — а не «свежак с GitHub», который мог разойтись по формату — обе зависимости pin'ятся в коде. См. [SPEC 046](../SPECS/046-F-N-PINNED_CORE_AND_TEMPLATE/SPEC.md).

### 5.1 `RequiredCoreVersion` (manual)

- **Где:** константа в `internal/constants/constants.go`.
- **Что значит:** версия sing-box, которую `DownloadCore` поставит при «Download / Reinstall» из Core Dashboard. UI больше не предлагает «Update to latest».
- **Кто меняет:** maintainer релиза. Бамп — отдельным коммитом перед тегом stable, или перед `gh workflow run` для пререлиза.
- **Когда менять:** только если для этого релиза действительно тестировали другую версию ядра (CVE, breaking-fix в SagerNet, новая фича sing-box). По умолчанию — не трогаем.
- **Как менять:**
  ```bash
  # editor: internal/constants/constants.go → RequiredCoreVersion = "<new-version>"
  git commit -am "chore(core): pin sing-box vX.Y.Z"
  git push origin develop
  ```
  Релизные ноты должны содержать пункт про бамп (если он есть).

### 5.2 `RequiredTemplateRef` (CI-injected, source-default)

- **Где:** `internal/constants/constants.go`, переменная (не константа) с дефолтным значением.
- **Что значит:** SHA коммита, из которого `wizard_template.json` тянется через `https://raw.githubusercontent.com/Leadaxe/singbox-launcher/<SHA>/bin/wizard_template.json`. Привязывает шаблон к точному состоянию репо на момент сборки бинарника.
- **CI-сборки:** `build/build_*.sh|bat` инжектят актуальный SHA через `-ldflags="-X singbox-launcher/internal/constants.RequiredTemplateRef=$(git rev-parse HEAD)"`. Source-default здесь не используется.
- **`go run .` и нестандартные билды без ldflags:** используется source-default. Чтобы он не «протухал», бампится в §1.5 после каждого релиза:
  ```bash
  NEW_REF="$(git rev-parse origin/main)"
  # editor: internal/constants/constants.go → RequiredTemplateRef = "<NEW_REF>"
  git commit -am "chore(constants): bump RequiredTemplateRef source-default"
  git push origin develop
  ```
- **Когда **не** менять:** до релиза, по ходу разработки фичи — source-default оставляем нетронутым. `go run .` всё это время тянет шаблон последнего релиза, а не HEAD develop'а — это by design (так локальные сборки соответствуют тому, что собирали пользователи).

### 5.3 Инвалидация шаблона на стороне пользователя

При апгрейде лаунчера `core.InvalidateTemplateIfStale` (вызывается в `main.go`) сравнивает `Settings.LastTemplateLauncherVersion` (записан после последнего успешного «Download Template») с `constants.AppVersion`. Если меньше — `bin/wizard_template.json` удаляется, UI показывает синюю «Download Template». После успешного скачивания UI пишет в `bin/settings.json` свежее `last_template_launcher_version`.

Dev-сборки (AppVersion = `v-local-test`, `unnamed-dev`, `*-dirty`) пропускают инвалидацию — иначе локальная разработка ломается на каждом запуске.

Подробности и тесты — [`core/template_migration.go`](../core/template_migration.go).
