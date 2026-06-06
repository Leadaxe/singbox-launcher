# SPEC 065-B-N — CLEANUP_GHOST_WINTUN_ADAPTERS

**Status:** New (N)
**Type:** Bug-fix (B) — каждый запуск VPN на **Windows 7** оставляет ghost-инстанс TUN-адаптера в реестре. После N стартов имена в системе разрастаются до `singbox-tun0`, `tun1`, `tun2`, ..., в Device Manager (Show hidden devices → Network adapters) копится мусор, netcfg тормозит.
**Depends on:** ничего. Чисто win-platform-layer фикс на уровне `core/platform/`.
**Не меняет:** sing-box config (имя адаптера остаётся `singbox-tun0`), общая логика старта/стопа VPN, не-Windows платформы (полный no-op на macOS/Linux), Windows 8/10/11 (детектится версия — no-op).

---

## Проблема

На **Windows 7** каждое graceful-shutdown sing-box оставляет phantom-device в реестре. Цепочка:

1. sing-box стартует TUN через WinTun: `WintunCreateAdapter("singbox-tun0", "WinTun", &guid)` → SetupAPI создаёт PnP device-node в `HKLM\SYSTEM\CurrentControlSet\Enum\ROOT\NET\NNNN`. NetConnectionID `"singbox-tun0"` регистрируется в `HKLM\SYSTEM\CurrentControlSet\Control\Network\{4d36e972-...}\{NetCfgInstanceId}\Connection\Name`.
2. На VPN-stop sing-box вызывает `WintunCloseAdapter` → `SetupDiCallClassInstaller(DIF_REMOVE)` → device-node помечается `DN_REMOVED`.
3. **На Win8/10/11** PnP Manager после `DIF_REMOVE` физически удаляет node + чистит NetConnectionID. Имя `singbox-tun0` освобождается.
4. **На Win7** umpnpmgr.dll старее — только устанавливает `CM_PROB_PHANTOM` (problem code 24), оставляя node и NetConnectionID занятыми. Адаптер невидим в Network Connections, **виден** в Device Manager → View → Show hidden devices как серый.
5. **Следующий старт** sing-box — WinTun видит занятое имя → инкрементит до `singbox-tun1`. И так каждый раз.

**Свидетельство в продакшене** (Telegram-чат, юзер на Win7):
```
06.06.2026 00:56 — "Сейчас заработало, но каждый раз создаётся новая сеть с новым номером"
06.06.2026 01:03 — "Я перезагрузил компьютер, но опять создаётся новая сеть с номером 5 уже"
```

Это **не баг sing-box** и **не баг WinTun** — оба корректно вызывают официальный API, который на Win7 не доделан до конца. Известная проблема Windows 7 PnP-subsystem'а.

### Что **не** работает как митигация

- ❌ Fixed adapter name — WinTun всегда создаёт fresh, нет API "reuse".
- ❌ `WintunDeleteDriver` на каждом stop — выгружает драйвер целиком, next start требует пере-инсталла + админ-промпт. Слишком инвазивно.
- ❌ User-facing batch — мы уже сделали `scripts/cleanup-singbox-tun*.bat`, но это требует ручного действия пользователя. Не решение, а workaround.

### Что **работает**

- ✅ После `DIF_REMOVE` (sing-box exit'нул) дозвать `SetupDiCallClassInstaller(DIF_REMOVE)` повторно с того же device-info handle снаружи — это **физически** удаляет phantom node, NetConnectionID освобождается.

---

## Целевая модель

### Когда триггерим

**Триггер:** finalization части `ProcessService.Stop()` — после того как launcher отправил kill-signal и sing-box процесс завершился (либо graceful, либо force-kill через watchdog).

**Не триггерим на:**
- VPN **start** — пользователь не должен ждать cleanup перед поднятием туннеля. Кроме того, на start активный адаптер может быть в transitional state, ложно-srabatывает фильтр.
- launcher **startup** — не наша зона ответственности при холодном старте чужого процесса не трогать.
- launcher **shutdown** — только если VPN был активен в момент закрытия (это и так покроется stop-хуком).
- **Невладельческие** адаптеры — только имена с префиксом `singbox-tun` (не `wg-*`, не AmneziaWG, не другие WinTun-клиенты).

### Что делаем

После сигнала о смерти sing-box (≥500ms задержка чтобы PnP Manager обработал `DIF_REMOVE`):

```go
// core/platform/wintun_cleanup_windows.go
func CleanupGhostSingboxTunAdapters() (removed int, err error)
```

1. **Гард по OS-версии** — `RtlGetVersion()` возвращает major.minor:
   - Не Windows → return 0, nil (build tag `_windows.go` уже это покрывает, но явно подтвердим в `runtime.GOOS`)
   - Не 6.1 (Win7) → return 0, nil
   - 6.1 (Win7 + Win7 SP1) → продолжаем
2. **Enum Net-class devices**: `SetupDiGetClassDevs(&GUID_DEVCLASS_NET, NULL, NULL, 0)` — флаг 0 (БЕЗ `DIGCF_PRESENT`), чтобы phantom тоже попадали.
3. **Для каждого**:
   - Читаем FriendlyName (`SPDRP_FRIENDLYNAME`) → если не starts-with `singbox-tun` → skip.
   - Читаем Service (`SPDRP_SERVICE`) → если не `Wintun` → skip. Двойная защита от случайного попадания.
   - Читаем DevNode-status через `CM_Get_DevNode_Status(&status, &problem, devInst, 0)`:
     - Если `problem != CM_PROB_PHANTOM (24)` → skip (адаптер живой).
     - Если `status & DN_STARTED (0x00000008)` → skip (драйвер активен — кто-то им пользуется, может быть параллельный launcher).
   - Только если **обе** проверки прошли — это phantom. Вызываем `SetupDiCallClassInstaller(DIF_REMOVE, h, &devInfo)`.
4. **Кап** на одну итерацию: максимум 32 удалений (заведомо больше реалистичного накопления, защита от рантайм-петли при кривом enum).
5. **Логи**:
   - Заголовок: `"ghost-tun cleanup: scanning (Win7 detected)"`
   - Per-skip: `"ghost-tun cleanup: skip name=%q reason=%q"` на DebugLog
   - Per-remove: `"ghost-tun cleanup: removed instance=%q name=%q"` на InfoLog
   - Финал: `"ghost-tun cleanup: done, removed=%d skipped=%d"` на InfoLog
   - Любая SetupAPI ошибка → WarnLog, не блокирует следующие итерации.

### Где вшиваем в код

```go
// core/process_service.go::Stop()
//
// После того как sing-box подтверждённо завершился (watchdog отработал),
// запускаем cleanup в отдельной goroutine с задержкой.
//
// Важно: НЕ блокируем UI Stop button — пользователь видит "Stopped" сразу,
// cleanup идёт фоном.
go func() {
    if runtime.GOOS != "windows" {
        return
    }
    time.Sleep(500 * time.Millisecond) // PnP Manager обрабатывает DIF_REMOVE
    removed, err := platform.CleanupGhostSingboxTunAdapters()
    if err != nil {
        debuglog.WarnLog("ghost-tun cleanup failed: %v", err)
        return
    }
    if removed > 0 {
        debuglog.InfoLog("ghost-tun cleanup: removed %d phantom adapter(s)", removed)
    }
}()
```

Insertion point — в самом конце `Stop()`, после того как `RunningState.Set(false)` и `StoppedByUser` сброшены, чтобы UI уже знал что VPN остановлен.

---

## Safety inventory (почему «не агрессивно»)

| Гард | Что отсекает |
|---|---|
| `runtime.GOOS != "windows"` | macOS / Linux — early return до загрузки `setupapi.dll` |
| RtlGetVersion ≠ 6.1 | Win8/10/11 — там нет проблемы, лезть в SetupAPI бессмысленно |
| `!strings.HasPrefix(name, "singbox-tun")` | Чужие WinTun-клиенты (Cloudflare WARP, AmneziaWG, wireguard.exe) |
| `service != "Wintun"` | Не-WinTun адаптер случайно назвался `singbox-tun*` (маловероятно, но защита) |
| `problem != CM_PROB_PHANTOM` | Активный или просто заглушенный (не phantom) — не трогаем |
| `status & DN_STARTED` | Драйвер активен — кто-то реально использует |
| `removed < 32` capper | Защита от runaway-цикла при кривом enum-ответе |
| Errors → WarnLog only, не fatal | Любой сбой в SetupAPI **не блокирует** стоп VPN |

Если ни один phantom не найден — функция честно возвращает `0, nil` и в логе только `done, removed=0`. **Никаких UI-уведомлений** на ноль-removed случай (типичный сценарий на Win10/11 + первый стоп на Win7).

---

## Out of scope

- **Win10/11 cleanup** — там нет проблемы, не лезем.
- **Cleanup на startup VPN** — пользователь явно отверг, и race-условия сложнее (см. §Целевая модель).
- **Удаление WinTun-драйвера** (`WintunDeleteDriver`) — слишком инвазивно, требует переустановки на next start.
- **Cleanup при crash launcher'а** — если launcher упал, hook не сработает. Митигация: следующий graceful stop почистит и накопленные с прошлой сессии (фильтр по `CM_PROB_PHANTOM` ловит и старые). Для catastrophic-case есть `scripts/cleanup-singbox-tun*.bat`.
- **Renaming** active adapter обратно в `singbox-tun0` после cleanup'а ghost'ов — sing-box при следующем старте уже **сам** создаст `tun0` (имя свободно). Не нужна отдельная логика.
- **UI feedback** ("очищено N адаптеров") — это служебная операция, не интересно юзеру. Только в логах.
- **Diagnostics tab кнопка** "Reset WinTun driver" — отдельная фича, не часть этого SPEC'а.

---

## Implementation phases

### Phase 1 — SetupAPI bindings
Файл: `core/platform/wintun_cleanup_windows.go` (build tag `//go:build windows`).
- LazyProcs для `setupapi.dll`, `cfgmgr32.dll`, `ntdll.dll` (для RtlGetVersion).
- Структуры `SP_DEVINFO_DATA`, `OSVERSIONINFOEX`.
- Константы: `DIGCF_*`, `SPDRP_FRIENDLYNAME=0x0C`, `SPDRP_SERVICE=0x04`, `DIF_REMOVE=5`, `CM_PROB_PHANTOM=24`, `DN_STARTED=0x00000008`.
- Helper `isWindows7() bool`.
- Helper `getRegistryPropertyW(h, devInfo, prop) string`.
- Helper `getDevNodeStatus(devInst) (status, problem uint32, ok bool)`.

### Phase 2 — Public API
- `CleanupGhostSingboxTunAdapters() (removed int, err error)`.
- Stub-файл `core/platform/wintun_cleanup_other.go` (build tag `//go:build !windows`) с no-op.

### Phase 3 — Integration
- `core/process_service.go::Stop()` — добавить goroutine в самом конце (см. §Целевая модель).
- Убедиться что вызов не из под `ac.CmdMutex` (Stop его освобождает раньше).

### Phase 4 — Tests
- **Unit-test** для `isWindows7()` — мок `RtlGetVersion` через interface. На самом ходовом Mac/Linux build'е тест check'нет fallback-путь "не Windows → false".
- **Manual QA matrix**:
  - macOS: VPN start/stop — функция должна no-op. Проверить логи на отсутствие шума.
  - Win10 VM: start/stop × 3 — функция должна no-op (версия != 6.1). Проверить логи.
  - Win7 VM (если есть) или через beta-тестера: start/stop × 5 — на 2-м и далее в логе должно появиться `removed=N>0`. После 5 циклов в Device Manager (Show hidden) ghost-адаптеров не должно копиться — максимум 1 (текущий).

### Phase 5 — Documentation
- Release notes: `docs/release_notes/upcoming.md` — секция "Bug fixes": "Windows 7: cleanup phantom TUN adapters left by sing-box on VPN stop (SPEC 065)".
- `scripts/cleanup-singbox-tun*.bat` оставляем как safety-net для уже-накопившихся ghost'ов у тех кто обновится с старой версии. Добавляем комментарий в .bat что начиная с N версии cleanup автоматический.

### Phase 6 — Build + commit
- `./build/build_windows.sh` (если есть; иначе через cross-compile).
- `go vet ./...`, `go test ./...`.
- Commit с conventional message: `fix(platform/wintun): auto-cleanup phantom TUN adapters on Windows 7 (SPEC 065)`.

---

## Test plan (acceptance)

### Локально (Mac/Linux)
- `go build ./...` проходит.
- `go vet ./...` зелёный.
- `go test ./core/platform/...` зелёный (включая non-Windows fallback).

### Win10 VM (QA)
- Start VPN → connection живая.
- Stop VPN → в `singbox-launcher.log`:
  ```
  ghost-tun cleanup: scanning (skipped — not Windows 7)
  ```
  ИЛИ функция вообще не зовётся (если решим guard'ить раньше). **Acceptable:** функция вызвана, но рано вышла.
- Повторить 5 раз — никаких adapter leaks, никаких unexpected error в логе.

### Win7 VM или beta-юзер (final QA)
- Start VPN → подключение живое.
- Stop VPN → в логе: `ghost-tun cleanup: scanning (Win7 detected)` + `done, removed=N` (N=1 на старте, N=0 при идеальном случае).
- Open Device Manager → View → Show hidden devices → Network adapters → **только один** `singbox-tun0` (активный или удалённый последним).
- Повторить 5 циклов start/stop — счётчик не растёт.

### Регрессия
- Все остальные сценарии (старт без VPN, force-kill через task manager, fault recovery, переключение пресетов) — не должны быть затронуты. Cleanup — это **postscript** к Stop, не часть основного flow.

---

## Прогноз эффорта

| Phase | LOC | Время |
|---|---|---|
| 1 — SetupAPI bindings | ~180 | 0.5 day |
| 2 — Public API + stub | ~40 | 0.1 day |
| 3 — Integration in Stop() | ~20 | 0.1 day |
| 4 — Tests + manual QA setup | ~80 unit + manual | 0.3 day |
| 5 — Docs | ~10 | 0.1 day |
| 6 — Build, vet, commit | — | 0.1 day |
| **Итого** | ~330 LOC | **~1.2 day** |

Win7 VM не обязательна — финальная QA через beta-сборку у того же Telegram-юзера (он уже отзывчивый, готов проверять). На Win10 VM проверяем что не-Win7 ветка not aggressive.

---

## Risk register

| Риск | Вероятность | Митигация |
|---|---|---|
| `CM_Get_DevNode_Status` под нагрузкой возвращает stale state | low | гард `status & DN_STARTED` страхует |
| Cleanup триггерится на VPN-stop из watchdog'а параллельно с user-инициированным stop | low | `removed=0` идемпотентно, гонка безопасна |
| Антивирус блокирует SetupAPI вызовы из Go-process'а | very low | `setupapi.dll` — стандартный системный API, не флагается |
| На Win7 без SP1 RtlGetVersion возвращает 6.1 но другая build | низкая | гард по major.minor достаточно, build не важен |
| Goroutine в Stop утечёт если cleanup-вызов залип | low | `time.Sleep` ограничен 500ms, SetupAPI вызовы синхронны, max 32 итераций |
| User отменил VPN start (Cancel в админ-промпте), Stop вызван без актуального start | none | функция всё равно идемпотентна — ничего не найдёт, return 0 |

---

## Open questions

1. **Триггерить также на launcher graceful shutdown?** SPEC сейчас триггерит только на VPN-stop. Если launcher закрыт с активным VPN — `Stop()` всё равно вызовется в shutdown sequence, так что покрыто. Если закрыт когда VPN не запущен — нечего чистить. → **Не нужно отдельного хука.**
2. **Что если sing-box запущен extern'но (не нашим launcher'ом) и есть ghost'ы от него?** Текущий префикс-фильтр `singbox-tun` поймает и их. → **Это feature, не bug.** Чистим всё что подходит под фильтр.
3. **Триггерить на periodic-таймере (например раз в час) в фоне?** Перебор. Накопление 5 ghost'ов до того как мешать — это 5 циклов start/stop. На каждом мы чистим. Periodic не нужен.
