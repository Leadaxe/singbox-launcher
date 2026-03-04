# CI/CD workflow — что сделать

Состояние: в `ci.yml` восстановлена база из `ci-old.yml`, частично внесены правки. Ниже — что осталось довести до ума.

---

## 1. Параметр `target` — строка вместо choice

- **Сейчас:** `type: choice`, options `[ all, win7, win64 ]`, default `all`.
- **Нужно:** `type: string`, default `""`, description:  
  `"Targets to build (space-separated): macOS, Win64, Win7. Empty = all. Example: macOS Win64"`.
- Запускать только те сборки, которые перечислены в `target`; пустая строка = все три (macOS, Win64, Win7).

---

## 2. Разнести один job `build` на три job'а

- **Сейчас:** один job `build` с matrix (darwin + windows), условие по `target == 'all' || target == 'win64'`.
- **Нужно:**
  - **build-darwin** — только macOS (universal + Catalina). Запускать при теге `v*` или при `workflow_dispatch` (build/prerelease) и (`target` пусто или `target` содержит `macOS`). Артефакты: `artifacts-darwin`, `artifacts-macos-catalina`.
  - **build-windows** — только Win64. Запускать при теге `v*` или при dispatch и (`target` пусто или содержит `Win64`). Артефакт: `artifacts-windows`.
  - **build-win7** — оставить как отдельный job; изменить условие на: тег `v*` или (dispatch и (`target` пусто или содержит `Win7`)).

Имена артефактов в release не менять: по-прежнему искать `.app` / `.exe` по имени и собирать zip'ы с именами `singbox-launcher-${VERSION}-macos.zip`, `-win64.zip` и т.д.

---

## 3. Job `release`

- **Сейчас:** `needs: [ meta, build, build-win7 ]`, `if: needs.build.result == 'success' && ...`.
- **Нужно:**
  - `needs: [ meta, build-darwin, build-windows, build-win7 ]`.
  - Условие: `always()` и meta успешен, и каждый из трёх build'ов либо `success`, либо `skipped`, и хотя бы один build — `success`, и (тег или run_mode=prerelease).
- Список `files` и шаг «Create release packages» оставить как есть (без Win7-32 в assets, если в ci-old его нет; при необходимости добавить блок Win7-32 позже отдельным пунктом).

---

## 4. Версия/тег для пререлиза 

- В meta для prerelease использовать:
  - `DESCRIBE_RAW="$(git describe --tags --always --exclude='*-prerelease')"`
  - `DEV_TAG="${DESCRIBE_RAW}-prerelease"`
- Имена файлов в assets не менять: шаблон `singbox-launcher-${VERSION}-*.zip` остаётся, подставляется текущий `VERSION`/tag.

---

## 5. Форматы файлов и текст установки

- **Форматы файлов:** exe из build-win7 — `singbox-launcher-win7-32.exe` (так и оставить). В релизе не вводить новый формат zip вроде `singbox-launcher-${VERSION}-win7-32.zip`; оставить как в ci-old (macos, macos-catalina, win64, checksums).
- **Текст установки для пререлиза:** в body релиза для prerelease вынести комментарий за пределы копируемой команды и в блоке кода оставить одну команду с явной версией:  
  `curl -fsSL ... | bash -s -- <version>`.

