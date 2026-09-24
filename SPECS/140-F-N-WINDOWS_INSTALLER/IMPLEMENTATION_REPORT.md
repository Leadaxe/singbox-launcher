# SPEC 140 · Отчёт о реализации

Дата: 24.09.2026. Ветка `spec-140-installer` от `develop` (`879f276e`). Этапы 1–6
PLAN.md; этап 7 (ручная приёмка) — за владельцем, этап 8 — после SPEC 141.
Выпуск — только вместе с SPEC 139 или после (SPEC §8).

## Что сделано

**Лаунчер (этапы 1–2).**
- `internal/platform/instance_windows.go` — `RegisterInstance(onQuit)`: мьютексы
  `Local\SingboxLauncher.Instance` и `Global\SingboxLauncher.Instance` (отказ
  Global — строка INFO, не ошибка), событие `Local\SingboxLauncher.Quit`,
  горутина `WaitForSingleObject` → `onQuit`. `instance_other.go` — заглушка.
- `main.go` — регистрация после строки «launcher … started» (логи уже открыты),
  то есть после `-paths`, `-purge-data`, `-gl-probe*` и — после слияния с 139 —
  после её флагов. По событию — `fyne.Do(controller.GracefulExit)`: UI-поток, без
  `RequestRestartAfterExit`. До запуска цикла событий вызов встаёт в очередь.
- `internal/platform/glprobe_windows.go` — в ветке отказа железа, перед D3:
  `!paths.ProbeWritable(App)` → одно сообщение с названием задачи установщика
  (`installerMesaTask`) и ссылкой на RDP_OPENGL, установка не предлагается.
  Кнопка Mesa в Диагностике в непишущемся AppDir и так недоступна; ветка D1
  (Mesa стоит, железо есть) в Program Files не достигается — переименовать
  opengl32.dll нельзя, проба видит Mesa (`SawMesa`).

**Скрипт подготовки (этап 3).** `build/installer/stage_win64_full.sh <exe>
<version> <out>` — блок бывшего `ci.yml:698-737` без изменений логики, без
`portable.txt`; падает явно, если нет `unzip`. Job `release` зовёт его и сам
докладывает маркер перед zip. На `windows-latest` `unzip` в Git Bash есть
(прогон ниже).

**Установщик (этап 4).** `build/installer/singbox-launcher.iss` по §3.2–§3.3,
§4–§6: `[Setup]` (AppId из спеки, `x64os`, `MinVersion=10.0`,
`CloseApplications=force`, логи), `[Languages]` en/ru, `[CustomMessages]` на
двух языках, задачи `desktopicon` / `autostart` (`Check: not IsUpgrade`) /
`mesa` / `daemonservice` (`#ifdef DaemonService`), `[InstallDelete]`,
`[UninstallDelete]`, `[Icons]`, `[Run]`; `[Code]`: `CloseLauncher` (из
`PrepareToInstall` и `InitializeUninstall`), `UpdateReadyMemo`, шаги удаления
§6 в `usUninstall` / `usPostUninstall`, служба по `ImagePath` за `#ifdef`.

**CI (этап 5).** Job `build-windows-installer` (§7) и правки `release`: `needs`,
условие, подъём setup.exe в корень, строка в `checksums.txt`, `files:`, раздел
Windows в теле релиза (установщик — вариант 1, zip — portable в папку с правом
записи, совета про Program Files нет).

**Доки (этап 6).** README / README.ru (Installation → Windows),
BUILD_WINDOWS(.ru) (раздел про установщик, локальная сборка ISCC),
RELEASE_PROCESS(.ru) (6 артефактов, чеклист), `.github/workflows/README.md`,
RDP_OPENGL(.ru) (задача Mesa), ARCHITECTURE(.ru) §7a.6, `upcoming.md`
(EN/RU), строка 140 в `SPECS/README.md`.

## Отступления от спеки (явно)

1. **Явные DACL у мьютексов и события** (в спеке не оговорено). С DACL по
   умолчанию объект доступен только создателю и SYSTEM: `CheckForMutexes`
   повышенного установщика под другой учётной записью администратора (и для
   `Global\` из чужого сеанса) мьютекс бы не увидел, а `OpenEvent` не открыл бы
   событие. Мьютекс: полный доступ SYSTEM / Administrators / владельцу,
   `SYNCHRONIZE` всем (только обнаружение). Событие: SYSTEM / Administrators /
   владелец; `Local\` и так виден только своему сеансу.
2. **Событие с ручным сбросом** — один `SetEvent` закрывает все экземпляры
   сеанса, а не один. Цена: пока жив зависший экземпляр, держащий событие
   поднятым, новый лаунчер того же сеанса тоже выйдет сразу; установщик в этой
   ветке всё равно завершает зависший (Ignore) или отменяется.
3. **Ключей `locale.T` нет.** Диалоги GL-гейта только английские (SPEC 125
   §2.3: гейт работает до `locale.SetLang`), новая строка — такая же. Подпись
   задачи в гейте — константа `installerMesaTask`, совпадает с английским
   `TaskMesa` в `.iss` (перекрёстные комментарии в обоих местах).
4. **Mesa в `[Files]` — три строки** (спека показывала одну), парой к трём
   строкам `[InstallDelete]` (список `mesaDLLs`).
5. **Имена.** `AppName=singbox-launcher` как в спеке; ярлыки,
   `UninstallDisplayName` и `AppVerName` — «Singbox Launcher» (заголовок окна).
6. **Retry / Cancel / Ignore** — `TaskDialogMsgBox` с `MB_ABORTRETRYIGNORE` и
   своими подписями; «Отменить установку/удаление» = кнопка Abort. Ожидание
   20 с — опрос `Sleep(250)`, страница Preparing на это время не отвечает
   (обычно лаунчер закрывается за 1–3 с).
7. **Вопрос «Удалить данные?»** задаётся в `usUninstall`, то есть после
   стандартного подтверждения удаления; `CloseLauncher` — в
   `InitializeUninstall`, как в спеке (до подтверждения).
8. **`%n` в `[CustomMessages]`** дополнительно превращается в перевод строки в
   `[Code]` (`CustomText`), чтобы не зависеть от того, делает ли это
   компилятор для `CustomMessage()`.
9. **Лишний шаг CI** «Compile-check DaemonService variant»: ветку
   `#ifdef DaemonService` до SPEC 141 иначе никто не компилирует. Собирается на
   пустых файлах-заглушках в `RUNNER_TEMP`, результат выбрасывается (~2 с).
10. **Проверка числовой версии** дополнительно отвергает компонент > 65535
    (ограничение `VERSIONINFO`), а не только неразбор.
11. **`checksums.txt`** считается с `-maxdepth 1` (архивы и установщик лежат в
    корне `release-artifacts`).
12. **Мьютекс регистрируется не на старте процесса**, а после `NewAppController`
    (то есть после миграции SPEC 135 §3.4) — чтобы строки `instance:` попали в
    открытый лог. Лаунчер, пойманный установщиком в этом промежутке (обычно
    доли секунды, при миграции дольше), мьютексом не виден; его закрывает
    страховка Restart Manager (`CloseApplications=force`).
13. **Запуск из `{app}` с правами только внутри Program Files** (правка по
    ревью). `-autostart=off`, `-purge-data -yes` при удалении и будущая
    установка службы выполняются, только если `{app}` лежит внутри
    `{commonpf64}`; иначе шаг пропускается с записью в лог (в `D:\Apps\…`
    обычный пользователь мог бы подменить exe). Записи с `runasoriginaluser` не
    затронуты.
14. **Остатки `{app}\bin` и `{app}\logs`** при «Да» удаляются, только если
    папка программы называется `singbox-launcher` (правка по ревью): вручную
    введённая общая папка не теряет чужие `bin` и `logs`.
15. **Отмена в `CloseLauncher`** сбрасывает событие Quit (`ResetEvent`):
    поднятым оно закрыло бы следующий лаунчер сеанса сразу после старта
    (правка по ревью; снимает цену из п. 2 для ветки Cancel).
16. **Inno Setup 6.4+**, а не 6.3+: `ExecAndCaptureOutput` появилась в 6.4.0.
    Проверка — `#if VER < EncodeVer(6,4,0)` в `.iss` и версия `ISCC.exe` в
    шаге CI (образ несёт новее, ставить ничего не нужно).
17. **Задача автозапуска** называется как чекбокс SPEC 139: «Start with
    Windows» / «Запускать вместе с Windows» (спека: «Launch at startup»).

## Проверки

- Локально: `go build ./...`; `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet
  ./internal/...` и то же для `GOARCH=386 ./internal/platform/`;
  `go run ./tools/win7guard`, `tools/paths_guard --strict`,
  `tools/l10n/hardcoded_check --strict`; `bash -n` и прогон
  `stage_win64_full.sh` на macOS с exe-заглушкой (состав совпал с прежним
  блоком: exe, `bin/{sing-box.exe, libcronet.dll, wintun.dll,
  wizard_template.json, wizard_template.version}`, `mesa3d/{opengl32,
  libgallium_wgl, dxil}.dll`). `go vet ./core/...` под Windows без cgo не
  собирается (fyne/gl) — только CI.
- CI (ветка `spec-140-installer`):
  - 36021777914 (`build`, `Win64`, без тестов) — красный: ISCC, `case … else` в
    Pascal Script принимает один оператор; исправлено в `2b792eb8`.
  - 36023011274 (`build`, `Win64`, без тестов) — зелёный; setup 51,7 МБ,
    `FileVersion 2.1.0.25`.
  - 36024385151 (`build`, все цели, с тестами) — зелёный: тесты трёх ОС, macOS,
    Win64, Win7, установщик (51,7 МБ, `FileVersion 2.1.0.26`, `ProductVersion` =
    версия meta), вариант `/DDaemonService` компилируется; предупреждений ISCC нет.
  - 36024405504 (`tests`) — зелёный.
- Не проверено CI в режиме `build`: состав `win64-full.zip` «до/после» —
  job `release` идёт только на теге и пререлизе. Проверит пререлиз владельца.

## Что осталось

- Слияние с SPEC 139: флаги `-autostart=on|off` и `-handoff` приходят оттуда.
  До 139 exe с `requireAdministrator` не стартует из `runasoriginaluser`
  (ошибка 740): не работают задача автозапуска и «Launch after install», а
  `-autostart=off` при удалении выходит с кодом 2 (неизвестный флаг) — это
  безвредно и видно в логе удаления.
- Ручная приёмка SPEC §11, пп. 1–10 и 12, на Windows 10 и 11 (этап 7).
- Пререлиз с установщиком и строкой в `checksums.txt` — запускает владелец.
- Этап 8 после SPEC 141: сборка с `/DDaemonService`, сценарий §11 п. 11.
