# SPEC 140 · Задачи по этапам

Отмечать по факту коммита. Номера этапов — PLAN.md.

## Этап 0 · Предпосылки
- [x] SPEC.md, PLAN.md, TASKS.md.
- [ ] Решения владельца по спорным местам: закрытие через событие `Quit` (правка лаунчера в 140), служба снимается при любом ответе, задача автозапуска только при первой установке, задача Mesa, правка GL-гейта в 140.
- [ ] SPEC 139 в `develop`: `asInvoker`, CLI-вход автозапуска (имя флага сверить с §3.3/§6), раскладка для повышенного процесса (§8).

## Этап 1 · Лаунчер: мьютекс и событие Quit
- [ ] `internal/platform/instance_windows.go`: `Local\` и `Global\SingboxLauncher.Instance` (отказ Global — не ошибка), событие `Local\SingboxLauncher.Quit`, ожидание в горутине.
- [ ] `internal/platform/instance_other.go`: заглушка.
- [ ] `main.go`: создание только в GUI-режиме, после `-paths`, `-purge-data`, `-gl-probe*` и флага автозапуска; по событию — `GracefulExit` через `fyne.Do`, без `RequestRestartAfterExit`.
- [ ] `go build ./...`, `go run ./tools/win7guard`.

## Этап 2 · GL-гейт в непишущемся AppDir
- [ ] `glprobe_windows.go`: при `!paths.ProbeWritable(App)` установка Mesa не предлагается, текст называет задачу установщика.
- [ ] Новые ключи `locale.T` с переводом в `bin/locale/ru.json`.
- [ ] `go build ./...`, показ владельцу.

## Этап 3 · Скрипт подготовки win64-full
- [ ] `build/installer/stage_win64_full.sh <exe> <version> <out>`: логика `ci.yml:698-737` без изменений, без `portable.txt`.
- [ ] `release` зовёт скрипт и докладывает `portable.txt` перед zip.
- [ ] На Windows-раннере проверить `unzip` в Git Bash (запасной путь — `7z x`).
- [ ] CI `build`: `unzip -l` `win64-full.zip` совпадает с прежним.

## Этап 4 · Скрипт Inno
- [ ] `build/installer/singbox-launcher.iss`: `[Setup]` по §3.2 (`AppId` из SPEC, `x64os`, `MinVersion=10.0`, `CloseApplications=force`, логи установки и удаления).
- [ ] `[Languages]` english/russian, `[CustomMessages]` на обоих языках.
- [ ] `[Tasks]`: `desktopicon`, `autostart` (`Check: not IsUpgrade`), `mesa`, `daemonservice` за `#ifdef DaemonService`.
- [ ] `[Files]`, `[InstallDelete]` (`portable.txt`; DLL Mesa при `not mesa`), `[UninstallDelete]` (`*.dll.off`), `[Icons]`.
- [ ] `[Run]`: автозапуск `runasoriginaluser`; служба (за `#ifdef`); «Launch after install» `postinstall nowait skipifsilent runasoriginaluser`.
- [ ] `[Code]` `CloseLauncher`: `CheckForMutexes` → `SetEvent` → опрос 20 с → Retry/Cancel/Ignore, `taskkill /F /T` по Ignore и в тихом режиме; вызов из `PrepareToInstall` и `InitializeUninstall`.
- [ ] `[Code]` `UpdateReadyMemo`: строка о данных portable-копии при `{app}\bin\wizard_states\state.json`.
- [ ] `[Code]` удаление §6: вопрос (по умолчанию «Нет»), служба по `ImagePath` (за `#ifdef`), `-autostart=off`, `DelTree(wizard_states)` → `-purge-data -yes` через `ExecAndCaptureOutput`, код ≠ 0 → сообщение, `usPostUninstall` при «Да» → `DelTree` `{app}\bin` и `{app}\logs`, `RemoveDir({app})` если пуст (не весь каталог).

## Этап 5 · CI
- [ ] Job `build-windows-installer`: условие как у `build-windows`, `needs: [meta, build-windows]`, checkout `fetch-depth: 0`, артефакт exe, скрипт подготовки, числовая версия (отказ при неразборе), ISCC или `choco install innosetup`, артефакт `artifacts-windows-installer`.
- [ ] `release`: `needs` и условие (`success`/`skipped`), setup в корень `release-artifacts`, `checksums.txt`, `files:`.
- [ ] Тело релиза: раздел Windows — установщик первым, zip — portable, без совета распаковывать в Program Files.
- [ ] `.github/workflows/README.md`: job и артефакт.
- [ ] Прогон `run_mode=build`, `target=Win64` зелёный, setup.exe в артефактах, `VersionInfoVersion` в свойствах файла.

## Этап 6 · Документация
- [ ] `docs/BUILD_WINDOWS.md` / `.ru.md` — раздел Installer.
- [ ] `README.md` / `README.ru.md` — Installation → Windows.
- [ ] `docs/RELEASE_PROCESS.md` / `.ru.md` — список артефактов и чеклист.
- [ ] `docs/RDP_OPENGL.md` / `.ru.md` — задача Mesa.
- [ ] `docs/ARCHITECTURE.md` / `.ru.md` — поставка рядом с раскладкой 135.
- [ ] `docs/release_notes/upcoming.md` — EN Highlights, RU Основное.
- [ ] `SPECS/README.md` — строка 140.

## Этап 7 · Приёмка (владелец, Windows 10 и 11)
- [ ] Сценарии SPEC §11, пп. 1–10 и 12.
- [ ] Пререлиз с установщиком и строкой в `checksums.txt` (запускает владелец).

## Этап 8 · После SPEC 141
- [ ] Сборка с `/DDaemonService`: задача службы, шаг 3 удаления, сценарий §11 п. 11.
- [ ] Сверить с 141: команды, путь копии вне `{app}`, `--service=uninstall` без `--purge`.

## Закрытие
- [ ] CI `run_mode=tests` после слияния.
- [ ] Папка → `140-F-C-WINDOWS_INSTALLER`.
