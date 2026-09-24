# SPEC 140 · План реализации

Норма — SPEC.md. Здесь: где живёт каждый этап, что от чего зависит, как проверяется.

Предпосылка: SPEC 139 в `develop` (манифест `asInvoker`, CLI-вход автозапуска, раскладка для
повышенного процесса — SPEC §8). Ветка работы — отдельный worktree от `develop`. Этапы 2–4
от 139 не зависят и могут идти раньше, выпуск — только вместе с 139 или после.

## Архитектурные решения

| Решение | Где | Почему |
|---|---|---|
| Inno Setup 6, per-machine, `x64os`, без `portable.txt` | §3.1 | один UAC, данные в `%LOCALAPPDATA%` по 135, wintun — драйвер |
| Лаунчер закрывается сам по событию `Local\SingboxLauncher.Quit`, мьютекс для обнаружения | §4 | RM и `taskkill` снимают ядро аварийно, прокси остаётся |
| Автозапуск и его снятие — флагом лаунчера (139), не записью установщика в `HKCU` | §3.3, §6 | одно место для значения `Run`; `HKCU` админ-установщика бывает чужим |
| Перед `-purge-data` удалить `{app}\bin\wizard_states` | §6 п. 5 | иначе повышенная очистка выберет Legacy и пощадит настоящие данные |
| Один скрипт подготовки `win64-full` для zip и установщика | §7 | состав zip и установщика не расходится |
| Числовая версия из `git describe --long --match "v[0-9]*"`, отказ при неразборе | §7 | `VersionInfoVersion` требует `X.Y.Z.N`, в `build` версия `dev.*` |
| Шаг службы за `#ifdef DaemonService` | §3.3, §6 п. 3 | включается после 141 без переписывания скрипта |

## Этапы

| # | Что | Файлы | Зависит от | Проверка |
|---|---|---|---|---|
| 1 | Лаунчер: мьютексы `Local\`/`Global\SingboxLauncher.Instance`, событие `Quit` → `GracefulExit` на UI-потоке. Создаются только в GUI-режиме, после веток `-paths`, `-purge-data`, `-gl-probe*` и флага автозапуска 139 | `internal/platform/instance_windows.go` (новый), `internal/platform/instance_other.go` (новый, заглушка), `main.go` | — | `go build ./...`, `go run ./tools/win7guard` |
| 2 | GL-гейт: в непишущемся AppDir не предлагать установку Mesa, а назвать задачу установщика; ключи `locale.T` сразу в `ru.json` | `internal/platform/glprobe_windows.go`, `bin/locale/ru.json` | — | `go build ./...`, показ владельцу |
| 3 | Скрипт подготовки: блок `ci.yml:698-737` в `stage_win64_full.sh <exe> <version> <out>` без маркера; `release` зовёт его и сам кладёт `portable.txt` | `build/installer/stage_win64_full.sh` (новый), `.github/workflows/ci.yml` | — | CI `build`: `unzip -l` `win64-full` до и после совпадает |
| 4 | `.iss`: `[Setup]`, `[Languages]` en/ru, `[CustomMessages]`, `[Tasks]`, `[Files]`, `[InstallDelete]`, `[UninstallDelete]`, `[Icons]`, `[Run]`; `[Code]`: `CloseLauncher`, `IsUpgrade`, `UpdateReadyMemo`, шаги удаления §6 | `build/installer/singbox-launcher.iss` (новый) | 1 (имена мьютекса/события), 3 | ISCC в CI без предупреждений `UsedUserAreas` |
| 5 | CI: job `build-windows-installer` (§7); `release` — `needs`, условие, перенос setup в корень, `checksums.txt`, `files:`, тело релиза (Windows: установщик первым, без совета про Program Files для zip) | `.github/workflows/ci.yml`, `.github/workflows/README.md` | 3, 4 | `gh workflow run ci.yml --ref <ветка> -f run_mode=build -f skip_tests=true -f target=Win64` |
| 6 | Доки (ниже) | см. «Документация» | 4, 5 | — |
| 7 | Ручная приёмка §11 на Windows 10 и 11 | — | 1–5, 139 | владелец |
| 8 | После 141: сборка с `/DDaemonService`, шаг 3 удаления, сценарий 11 | `.iss`, `ci.yml` | 141 | владелец |

Порядок: 1 ∥ 2 ∥ 3 → 4 → 5 → 6 → 7; 8 отдельно, после 141.

## Документация

| Файл | Что |
|---|---|
| `docs/BUILD_WINDOWS.md` / `.ru.md` | раздел «Installer»: Inno Setup 6, подготовка каталога скриптом, вызов `ISCC /D…` с числовой версией |
| `README.md` / `README.ru.md`, раздел Installation → Windows | установщик первым вариантом, zip — portable, распаковывать в папку с правом записи |
| `docs/RELEASE_PROCESS.md` / `.ru.md` | список артефактов (+ `win64-setup.exe`) в §1.4 и чеклисте §4 |
| `.github/workflows/README.md` | job `build-windows-installer`, артефакт `artifacts-windows-installer` |
| `docs/RDP_OPENGL.md` / `.ru.md` | задача Mesa в установщике, поведение гейта в Program Files |
| `docs/ARCHITECTURE.md` / `.ru.md` | абзац о поставке рядом с раскладкой 135 (§7a): установщик = AppDir в Program Files, данные System |
| `docs/release_notes/upcoming.md` | EN Highlights / RU Основное: установщик, данные, удаление, SmartScreen |
| `SPECS/README.md` | строка 140 |

## Политика проверок

Локально только `go build ./...` (этапы 1–2) и `go run ./tools/win7guard`. Новый Go-код попадает
в сборку Win7 (go1.20): без `min`/`max`, `slices`, `maps`. Новых Go-тестов нет: логика
установщика — Pascal Script в `.iss`, правка лаунчера — склейка процесса и UI. Установщик
собирается только в CI (режим `build`, цель `Win64`). Публикацию проверяет пререлиз, его
запускает владелец. Полный прогон — `gh workflow run ci.yml --ref develop -f run_mode=tests`
после слияния.

## Границы

Не делаем: winget, scoop, choco, MSI; подпись (заготовка в `.iss` закомментирована); ARM64, Win7;
самообновление; Job Object для ядра; замену `CheckIfLauncherAlreadyRunningUtil` на мьютекс.
