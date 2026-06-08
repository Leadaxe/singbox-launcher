# SPEC 065-B-N — CLEANUP_GHOST_WINTUN_ADAPTERS

**Status:** Implemented (revised post-v0.9.9 — итеративные доработки по обратной связи Win7-бета-юзера).
**Type:** Bug-fix (B). Каждый запуск VPN на Windows оставляет два класса осиротевших артефактов в системе:
  1. **PnP device-node** (phantom WinTun-адаптер) — проблема Windows 7 only.
  2. **NLA profile / signature** (запись Network Location Awareness) — проблема всех версий Windows.
**Depends on:** ничего. Чисто win-platform-layer фикс на уровне `internal/platform/`.
**Не меняет:** sing-box config, общая логика старта/стопа VPN, не-Windows платформы (полный no-op).

---

## Этапы доработки

После релиза v0.9.9 (initial-имплементация) спека итеративно дорабатывалась по обратной связи Win7-бета-юзера. Все этапы в ветке `develop`, ещё НЕ зарелизены.

| Этап | Коммит | Изменение |
|---|---|---|
| Initial (v0.9.9) | — | Phantom device-node cleanup на Win7. Фильтр: `FriendlyName starts-with "singbox-tun"` + `Service == "Wintun"` + `CM_PROB_PHANTOM` + `!DN_STARTED`. |
| Manifest fix | `9d61229` | Embed manifest в Win7 build (без него requireAdministrator из родительского процесса не наследовался под UAC-disabled). |
| NetConnectionID switch | `34a1bb0` | Имя адаптера читалось из FriendlyName ("Wintun Userspace Tunnel"), фильтр не срабатывал. Переключение на NetConnectionID через registry lookup. |
| Aggressive mode + drop name-check | `e3eaebe` | Введён mode-flag, дефолтный aggressive: `Service == "Wintun"` + `!DN_STARTED` без name-prefix. На Win7 WinTun не пробрасывает launcher-set имя ни в одно из читаемых SetupAPI properties (NetConnectionID локализован). Phantom-only mode сохраняет name-check как best-effort. |
| NLA cleanup (текущий этап) | `42a8985` + правки в этом документе | Добавлена `cleanupNLAProfiles`. Чистится на **всех** Windows версиях, не только Win7 — накопление NLA-кэша универсально, но видно для пользователя только на Win7. |

---

## Проблема — два независимых вектора накопления

### Вектор 1 — phantom device-node (Win7 only)

Цепочка:

1. sing-box стартует TUN через WinTun: `WintunCreateAdapter("singbox-tunK", "WinTun", &guid)` → SetupAPI создаёт PnP device-node в `HKLM\SYSTEM\CurrentControlSet\Enum\ROOT\NET\NNNN`. K = 0,1,2,... (sing-box инкрементит индекс если имя занято).
2. На VPN-stop sing-box вызывает `WintunCloseAdapter` → `SetupDiCallClassInstaller(DIF_REMOVE)` → device-node помечается `DN_REMOVED`.
3. **На Win8/10/11** PnP Manager после `DIF_REMOVE` физически удаляет node + чистит NetConnectionID. Имя `singbox-tunK` освобождается.
4. **На Win7** umpnpmgr.dll старее — node остаётся "висеть": CM_PROB_PHANTOM ставится не всегда (особенно при `taskkill` без graceful), `Wintun` service остаётся ассоциированным, NetConnectionID занят.
5. **Следующий старт** sing-box — индекс инкрементится. И так каждый раз.

Это **не баг sing-box** и **не баг WinTun** — оба вызывают официальный API, который на Win7 не доделан до конца.

### Вектор 2 — NLA profile / signature (все Windows версии)

Windows Network Location Awareness кэширует профиль для каждой увиденной сети и **никогда не удаляет** их автоматически. Структура реестра (стабильна с Vista):

```
HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\NetworkList\
  Profiles\{GUID}/
    ProfileName  = "singbox-tunK", "singbox-tunK  2", ..., "singbox-tunK  N"
    Description  = "singbox-tunK"          ← launcher-set имя адаптера (наш match key)
    Category     = REG_DWORD               ← 0=Public, 1=Private, 2=Domain
    NameType, DateCreated, DateLastConnected, ...
  Signatures\Unmanaged\<hash>/
    ProfileGuid  = "{GUID}"                ← back-ref в Profiles
    Description, FirstNetwork, ...
  Signatures\Managed\<hash>/               ← AD-domain профили, мы НЕ трогаем
```

Когда sing-box создаёт новый WinTun-адаптер с именем `singbox-tunK`, NLA-сервис при первом сетевом event'е заводит новую запись `Profiles\{GUID}` с `ProfileName = "singbox-tunK"`. Если `ProfileName` уже занят в кэше — NLA дедуплицирует через суффикс `"  2"`, `"  3"`, ..., и сохраняет в `Description` точное имя адаптера без суффикса.

После уничтожения адаптера запись остаётся. Пользователь видит:
- **Win7** — диалог "Choose Network Location" с растущим суффиксом `"singbox-tun0  16"`, `"...17"`, `"...18"` на каждом старте (визуальный спам).
- **Win8+** — диалог убран; имена видны только в Settings → "Управление известными сетями" / Network and Sharing Center. Функционального вреда нет, но реестр разрастается монотонно.

---

## Целевая модель

### Архитектура: одна public-функция, две внутренние фазы

```go
// internal/platform/wintun_cleanup_windows.go

func CleanupGhostSingboxTunAdapters(mode GhostTunCleanupMode) (removed int, err error) {
    // Phase A: NLA cleanup. Все Windows версии, безусловно.
    pRemoved, sRemoved := cleanupNLAProfiles()
    if pRemoved > 0 || sRemoved > 0 {
        debuglog.WarnLog("ghost-tun cleanup: NLA done profiles=%d signatures=%d", ...)
    }

    // Phase B: Device-node cleanup. Win7 only.
    if !isWindows7() {
        debuglog.DebugLog("ghost-tun cleanup: device-cleanup skipped (not Win7)")
        return 0, nil
    }
    // ... SetupAPI enum + filter + DIF_REMOVE ...
}
```

**Порядок (NLA → device) намеренный**: callsite гарантирует, что sing-box не работает (см. ниже § Когда триггерим). Любой `singbox-tun*` профиль в реестре по определению stale, поэтому NLA-чистку можно делать в первую очередь, без зависимости от устранения device-node. На Win8+ выйдем сразу после Phase A.

### Когда триггерим

| Callsite | Файл / точка | Sync? | Гард |
|---|---|---|---|
| `runGhostTunCleanup(true)` | `process_service.go::Restart` (pre-Start) | inline | sing-box подтверждённо мёртв |
| `runGhostTunCleanup(false)` | `process_service.go::Stop` (post-kill) | goroutine, delay 500ms | sing-box подтверждённо мёртв (watchdog отработал) |
| `CleanupStaleTunAtStart` | App startup | goroutine, no delay | `!isSingBoxProcessRunning() && !RunningState.IsRunning()` |

Во всех точках — sing-box не работает. Это инвариант, на который опираются обе фазы.

**Не триггерим на:**
- VPN start без предварительного Restart — пользователь не должен ждать cleanup перед поднятием туннеля.
- launcher shutdown — Stop-хук уже покрывает.
- Невладельческие адаптеры/профили — фильтрация по префиксу `singbox-tun`.

### Phase A — NLA cleanup (детально)

```go
func cleanupNLAProfiles() (profilesRemoved, signaturesRemoved int) {
    defer func() { recover() }() // безусловная страховка от паники

    // Step 1: enum Profiles, collect matched GUIDs, delete profile keys.
    profKey := registry.OpenKey(HKLM, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\NetworkList\Profiles`, ...)
    for _, name := range profKey.ReadSubKeyNames() {
        desc := sub.GetStringValue("Description")
        if !strings.HasPrefix(desc, "singbox-tun") {
            continue
        }
        registry.DeleteKey(profKey, name)
        matchedGUIDs[name] = true
    }

    // Step 2: enum Signatures\Unmanaged, delete entries with ProfileGuid in matchedGUIDs.
    sigKey := registry.OpenKey(HKLM, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\NetworkList\Signatures\Unmanaged`, ...)
    for _, name := range sigKey.ReadSubKeyNames() {
        guid := sub.GetStringValue("ProfileGuid")
        if matchedGUIDs[guid] {
            registry.DeleteKey(sigKey, name)
        }
    }
}
```

**Match policy:** `strings.HasPrefix(desc, "singbox-tun")` — тот же `adapterNamePrefix`, который используется в Phase B. Префикс уникален к нам (никакое чужое ПО не использует имя `singbox-tun*`), поэтому ложноположительных совпадений быть не может.

**Collect-then-delete** (а не delete-while-iterating): `RegEnumKey` после удаления текущего ключа имеет undefined ordering — собираем имена сначала, удаляем после.

**Cross-version safety:**
- Путь `\NetworkList\Profiles` и value-поля `Description` / `ProfileGuid` — стабильная Microsoft-схема с Vista. На Win7/8.1/10/11 идентична.
- Подтверждается тем, что PowerShell-командлеты `Get-NetConnectionProfile` / `Remove-NetConnectionProfile` (Win8+) работают поверх этих же ключей, как и UI "Управление известными сетями" Win10/11.
- Если на будущей Windows-версии Microsoft реструктурирует ключ — наш строгий `HasPrefix` просто ничего не найдёт, вернётся (0, 0). Fail-safe.

**Защиты:**
- `defer recover()` — безусловный wrapper, если registry-операция запаникует (не должна, но страховка).
- Все ошибки (`OpenKey`, `ReadSubKeyNames`, `DeleteKey`) → `WarnLog`, НЕ пропагируются. Любой сбой → пропускаем запись, продолжаем со следующей.
- `Signatures\Managed` (AD-domain) не трогаем вообще.

### Phase B — Device-node cleanup (Win7 only)

Существующая логика без изменений по сути, актуальная версия фильтра:

| Гард | aggressive (default) | phantom-only |
|---|---|---|
| `Service == "Wintun"` | ✅ | ✅ |
| `!DN_STARTED` | ✅ | ✅ |
| `CM_PROB_PHANTOM == problem` | — | ✅ |
| `FriendlyName starts-with "singbox-tun"` | — | ✅ (best-effort) |

**Почему drop name-prefix в aggressive:** на Win7 WinTun не пробрасывает launcher-set имя в FriendlyName (там фиксированное "Wintun Userspace Tunnel") и NetConnectionID (там локализованное "Подключение по локальной сети N"). Нет SetupAPI property, в котором лежит "singbox-tun*" — все варианты проверены через `.reg` экспорты у beta-юзера.

`Service == "Wintun"` + `!DN_STARTED` достаточно: Ethernet/Wi-Fi/RAS адаптеры имеют другой service, активные WinTun-адаптеры других клиентов (WireGuard, AmneziaWG) имеют DN_STARTED.

**Phantom-only mode** сохранён для будущего использования (если найдём callsite, где безопаснее консервативно).

### Где вшиваем в код

См. `core/process_service.go`:
- `runGhostTunCleanup(sync)` — общий wrapper с panic-recover.
- `triggerGhostTunCleanup()` — async из Stop.
- `runGhostTunCleanup(true)` — sync из Restart pre-Start.
- `CleanupStaleTunAtStart()` — async на startup launcher'а.

---

## Safety inventory

### Phase A — NLA (все Windows)

| Гард | Что отсекает |
|---|---|
| `runtime.GOOS != "windows"` | macOS / Linux — early return на уровне build-тегов (файл `wintun_cleanup_other.go`) |
| `!strings.HasPrefix(desc, "singbox-tun")` | Чужие сети (Ethernet, Wi-Fi, корпоративные VPN, другие WinTun-клиенты) |
| `Signatures\Managed` не трогается | AD-domain профили (managed by GPO) |
| callsite-инвариант: sing-box не работает | Гарантирует что любой `singbox-tun*` профиль — stale, активного не удалим |
| `defer recover()` | Паника registry-API → log, return (0, 0) |
| Errors → WarnLog, не пропагируются | Любой сбой → пропуск записи, не падает функция |
| `registry.DeleteKey` fail на ключе с детьми | Логируется + skip; safe degradation, если Microsoft в будущем добавит nested subkeys |

### Phase B — Device-node (Win7)

| Гард | Что отсекает |
|---|---|
| `runtime.GOOS != "windows"` | macOS / Linux |
| `!isWindows7()` | Win8/10/11 — там нет проблемы phantom device-node |
| `Service != "Wintun"` | Не-WinTun адаптеры (Ethernet, RAS, etc.) |
| `DN_STARTED` set | Активный адаптер (свой текущий или чужой WinTun-клиент) |
| `removed < 32` cap | Защита от runaway-цикла |
| Errors → WarnLog only, не fatal | Любой сбой в SetupAPI не блокирует стоп VPN |
| `ERROR_ACCESS_DENIED` на DIF_REMOVE | Залогируется с указанием run-as-admin, не падает |

Если ни Phase A, ни Phase B ничего не нашли — функция честно возвращает `0, nil`, лог: `done scanned=N removed=0 skipped=0` (NLA-строка отсутствует когда `pRemoved=sRemoved=0`).

---

## Out of scope

- **Renaming** active adapter обратно в `singbox-tun0` после cleanup'а — sing-box при следующем старте сам найдёт свободный индекс.
- **UI feedback** ("очищено N адаптеров / профилей") — служебная операция, не интересно юзеру. Только в логах.
- **Удаление WinTun-драйвера** (`WintunDeleteDriver`) — слишком инвазивно, требует переустановки на next start.
- **Periodic-таймер cleanup в фоне** — перебор. Накопление пары stale-записей до того как мешать — это пара циклов start/stop. На каждом мы чистим.
- **Cleanup `Signatures\Managed`** — AD/GPO-managed профили могут перетираться доменом. Не наша зона ответственности.
- **Дополнительные адаптерные prop-properties** — не нужны: на Phase B `Service == "Wintun"` + `!DN_STARTED` достаточно, на Phase A `Description` достаточно.

---

## Implementation phases (фактические)

### Phase 1 — SetupAPI bindings (initial)
Файл: `internal/platform/wintun_cleanup_windows.go` (build tag `//go:build windows`).
LazyProcs для `setupapi.dll`, `cfgmgr32.dll`, `ntdll.dll`. Структуры `SP_DEVINFO_DATA`, `OSVERSIONINFOEX`. Helper'ы `isWindows7()`, `getNetConnectionID()` (registry lookup для Win10/11 dev-машин).

### Phase 2 — Public API (initial)
`CleanupGhostSingboxTunAdapters(mode GhostTunCleanupMode) (removed int, err error)`. Stub `wintun_cleanup_other.go` для не-Windows.

### Phase 3 — Integration (initial)
`process_service.go::Stop`, `Restart`, `CleanupStaleTunAtStart`. Sync/async по contexture.

### Phase 4 — Manifest fix (post-release итерация по фидбэку Win7-юзера)
`.github/workflows/ci.yml` — embed `embed_manifest_windows.go` в Win7 build, без него requireAdministrator не наследовался под UAC-disabled.

### Phase 5 — Aggressive mode (та же серия итераций)
- Mode flag `GhostTunCleanupAggressive` / `GhostTunCleanupPhantomOnly`.
- Aggressive фильтр: `Service == "Wintun"` + `!DN_STARTED`, name-prefix check дропнут (см. § Phase B).
- Все три callsite переключены на aggressive.

### Phase 6 — NLA cleanup (текущий этап, этот SPEC)
- Новая функция `cleanupNLAProfiles()` в том же файле.
- Хук в `CleanupGhostSingboxTunAdapters` как **Phase A** (до Win7-гейта).
- `defer recover()` страховка.
- Match по `Description`-префиксу `adapterNamePrefix`.

### Phase 7 — Documentation + commit
- `docs/release_notes/upcoming.md` — секции "Win7" + "Windows (все версии)".
- Cross-compile `GOOS=windows GOARCH=386` и `GOARCH=amd64` clean.
- Conventional commits под префиксом `fix(win7):` для Phase B-изменений, `fix(windows):` для NLA.

---

## Test plan (acceptance)

### Локально (Mac/Linux)
- `go build ./...` проходит.
- `go vet ./...` зелёный.
- Cross-compile Win7-32 (`GOOS=windows GOARCH=386`) и Win64 (`GOOS=windows GOARCH=amd64`) clean.

### Win10/11 VM (QA)
- Start/Stop VPN × 3.
- В логе при каждом Stop:
  ```
  ghost-tun cleanup: NLA done profiles=N signatures=N    ← может появиться (если накопилось)
  ghost-tun cleanup: device-cleanup skipped (os=..., not Win7)
  ```
- Открыть Settings → Network → "Управление известными сетями" — `singbox-tun*` записи не должны накапливаться.
- В `regedit` под HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\NetworkList\Profiles — нет subkey с `Description = "singbox-tun*"`.

### Win7 VM или beta-юзер (final QA)
- Start VPN → подключение живое.
- Stop VPN → в логе:
  ```
  ghost-tun cleanup: NLA done profiles=N signatures=N
  ghost-tun cleanup: scanning aggressive (os=6.1.7601, Win7)
  ghost-tun cleanup: removing name="..." service="Wintun"
  ...
  ghost-tun cleanup: done scanned=K removed=K skipped=0
  ```
- В диалоге "Choose Network Location" больше не появляются дубликаты `singbox-tun0  N`.
- В Device Manager → Show hidden devices → Network adapters — только активный адаптер или ничего после Stop.
- Повторить 5 циклов — счётчики не растут.

### Регрессия
- macOS/Linux — `runtime.GOOS` гард + build-теги работают, никаких побочных эффектов.
- На Win10/11 NLA-cleanup безопасен: чужие WiFi/Ethernet профили не задеты (их `Description` не имеет префикса `singbox-tun`).

---

## Risk register

| Риск | Вероятность | Митигация |
|---|---|---|
| NLA-сервис держит профиль открытым в момент DeleteKey | low | `ERROR_SHARING_VIOLATION` → WarnLog + skip, следующий run догонит |
| Microsoft реструктурирует `\NetworkList\Profiles` в future Windows | very low (схема стабильна с Vista) | Строгий `HasPrefix` ничего не найдёт → return (0, 0). Fail-safe. |
| Корпоративная GPO забирает write-доступ к `\NetworkList` | low | `OpenKey` → ошибка → WarnLog + return (0, 0). VPN работает, реестр не чистится. |
| Localized Description на не-English Windows | none | `Description` всегда содержит launcher-set имя адаптера, не локализуется |
| Race: NLA создаёт новый профиль одновременно с нашим DeleteKey | low | Идемпотентно: на следующем Stop догоним |
| Пользователь сам назвал свою сеть `"singbox-tun0"` | none (близко к 0) | Снесём. Никаких реальных пользователей этим не задеваем — префикс зарезервирован за нами |
| Phase B-фильтр случайно зацепит активный WinTun-клиент (WireGuard) | very low | `DN_STARTED` страхует — активный клиент не пройдёт |

---

## Open questions — закрыты

1. **NLA cleanup только на Win7?** — Решено: на всех версиях. Накопление универсально, безопасность одинаковая. На Win8+ невидимо для пользователя, но реестр не разрастается монотонно.
2. **Триггерить на periodic-таймере?** — Не нужно. Stop/Restart/StartCleanup покрывают все реалистичные сценарии.
3. **Почему `Description`, а не `ProfileName`?** — `ProfileName` содержит NLA-суффикс дедупликации (`"singbox-tun0  16"`), `Description` всегда точное launcher-set имя (`"singbox-tun0"`). Префикс-match по `Description` устойчив к суффиксам.
4. **Почему не валидируем по существующему адаптеру?** — Cleanup как раз и нужен когда адаптера уже нет. Запись в NLA по определению "сирота".
