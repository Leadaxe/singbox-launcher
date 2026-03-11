## 019-F-N — Адаптация лаунчера под Windows 7

### 1. Контекст и проблема

- В CI уже есть отдельный job `build-win7` (Windows 7 x86, legacy), который собирает `singbox-launcher-win7-32.exe` и пакует его в `singbox-launcher-<version>-win7-32.zip` (`.github/workflows/ci.yml`, `artifacts-windows-win7-32`).
- В релизных заметках и SPECS описана общая схема CI/CD (`SPECS/001-F-C-FEATURES_2025/2026-02-15-ci-cd-workflow.md`, `docs/release_notes/0-8-1.md`, `.github/workflows/README.md`), но Win7 до недавнего времени фактически воспринимался как «отдельная сборка лаунчера» без полноценной интеграции с шаблоном визарда и выбором версии `sing-box`.
- В проект добавлены первые изменения под Win7:
  - фиксированная версия `sing-box` для Win7 (`core/core_downloader.go`, `Win7LegacyVersion` и выбор ассетов `windows-386-legacy-windows-7.zip`);
  - поддержка платформы `win7` в фильтрации шаблона визарда (`ui/wizard/template/loader.go`, `matchesPlatform` + описание в `docs/release_notes/upcoming.md`);
  - использование 32-битного ядра `sing-box` и 32-битного Wintun (архитектура 386) как для Win7 x86, так и для Win7 x64 (через WoW64).
- Требуется оформить и доработать эту работу как отдельную фичу: адаптация лаунчера под Windows 7, синхронизированная с существующим CI/CD и шаблоном визарда.

### 2. Цель задачи

Обеспечить предсказуемую и поддерживаемую работу лаунчера на Windows 7 (x86, legacy build) с учётом:

- фиксированной и проверенной версии ядра `sing-box` для Win7;
- корректной фильтрации и применения параметров/правил визарда под платформу Win7 (включая совместное применение секций `windows` и `win7` при сборке Win7 из CI);
- понятного поведения CI/CD (build, артефакты, release, install instructions) и отражения этого в документации.

### 3. Область охвата (scope)

В рамках этой задачи рассматриваем:

1. **Ядро sing-box для Win7**
   - Выбор версии и ассетов (GitHub / SourceForge) для Windows 7 / GOARCH=386.
   - Обоснование и фиксация legacy-версии (с учётом совместимости с Win7).
2. **Визард и шаблон конфигурации**
   - Поддержка платформы `win7` в `bin/wizard_template.json` и фильтрующей логике (`ui/wizard/template/loader.go`).
   - Поведение при сборке из CI (GOOS=windows, GOARCH=386; job `build-win7`).
3. **CI/CD под Win7**
   - Текущая схема: job `build-win7`, артефакты (`artifacts-windows-win7-32`), включение в release (`singbox-launcher-<version>-win7-32.zip` и install instructions).
   - Проверка, что Win7-сборка получает корректный шаблон визарда и умеет скачать/обновить нужный legacy core.
4. **Документация**
   - Обновление `docs/release_notes/upcoming.md` и/или отдельных документов в `docs/`/`SPECS/` с описанием поддержки Win7, ограничений и сценариев использования.

Вне scope:

- Расширение поддержки Win7 за пределы оговоренного legacy-режима (например, новые фичи ядра, не поддерживаемые в выбранной версии `sing-box`).
- Общий рефакторинг CI/CD или визарда, не относящийся к специфике Win7.

### 4. Требования и критерии приёмки

1. **Сборка и запуск Win7 лаунчера**
   - Job `build-win7` в `.github/workflows/ci.yml` по-прежнему успешно собирает `singbox-launcher-win7-32.exe` и публикует артефакт `singbox-launcher-<version>-win7-32.zip`.
   - В release job Win7-артефакт корректно включается в список артефактов и в install instructions для Windows 7 (legacy).

2. **Версия ядра sing-box на Win7**
   - При сборке лаунчера для Win7 (`GOOS=windows`, `GOARCH=386`) используется зафиксированная версия `sing-box`, описанная в `core/core_downloader.go` (константа `Win7LegacyVersion`).
   - Для Win7 используются корректные ассеты `sing-box` (архив `windows-386-legacy-windows-7.zip`), соответствующие выбранной версии.
   - Win7-режим предполагает запуск 32-битного `sing-box` и 32-битного Wintun как на Win7 x86, так и на Win7 x64.

3. **Работа визарда на Win7**
   - При запуске Win7-сборки лаунчера визард применяет секции шаблона с `"platforms": ["windows"]` и `"platforms": ["win7"]` (по аналогии с `darwin`/`darwin-tun`), как описано в `docs/release_notes/upcoming.md`.
   - Функция `matchesPlatform` в `ui/wizard/template/loader.go` корректно обрабатывает Win7 (GOOS=windows, GOARCH=386) без влияния на другие платформы.

4. **Документация**
   - В `docs/release_notes/upcoming.md` кратко описаны изменения по Win7 (ядро, визард, CI/CD).
   - При необходимости добавлена отдельная заметка (в `docs/` или в рамках этого SPEC) с описанием того, как устроена Win7-сборка в CI и какие ограничения есть у Win7-режима.

5. **Стабильность и обратная совместимость**
   - Изменения не ломают сборки для других платформ (Win64, macOS, Linux).
   - При отсутствии Win7-сборки (когда job `build-win7` пропущен) поведение CI/CD и релиза остаётся корректным (release создаётся при наличии хотя бы одного успешного build).

### 5. Связанные материалы

- CI/CD и Win7:
  - `.github/workflows/ci.yml` — jobs `build-win7`, `release`, артефакты `artifacts-windows-win7-32`, zip `singbox-launcher-<version>-win7-32.zip`.
  - `.github/workflows/README.md` — краткое описание job'ов `build-darwin`, `build-windows`, `build-win7`.
  - `SPECS/001-F-C-FEATURES_2025/2026-02-15-ci-cd-workflow.md` — детальное ТЗ по обновлённому workflow (включая `target`, раздельные build jobs и release).
- Визард и шаблоны:
  - `bin/wizard_template.json` — шаблон визарда, секции `params` и `selectable_rules` с фильтрацией по полю `platforms`.
  - `ui/wizard/template/loader.go` — логика загрузки шаблона, применения `platforms` (включая Win7).
- Ядро и релизные заметки:
  - `core/core_downloader.go` — загрузка `sing-box`, выбор ассетов и версия для Win7.
  - `docs/release_notes/0-8-1.md` и `docs/release_notes/upcoming.md` — релизные заметки, включая разделы про CI и Win7.

