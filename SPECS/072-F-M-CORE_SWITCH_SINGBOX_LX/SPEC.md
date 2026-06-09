# SPEC 072-F-M — ПЕРЕВОД ЯДРА НА ФОРК SING-BOX-LX (XHTTP + AmneziaWG)

## Цель

Лаунчер качает и пиннит ядро `sing-box` с upstream-релизов **SagerNet/sing-box** (сейчас `1.13.12`). Чтобы в визарде/подписках стали доступны два клиентских фич-набора — **XHTTP** (Xray-совместимый транспорт для VLESS/VMess/Trojan) и **AmneziaWG 2.0** (DPI-обфускация WireGuard-эндпоинтов), — нужно переключить источник ядра на форк **`github.com/Leadaxe/sing-box-lx`** (ветка `lx`, релиз `v1.13.13-lx.1+`). Обе фичи собраны в бинарь форка под build-тегами (`with_xhttp`, `with_awg`); конфиги с этими полями upstream-ядро **отвергает на этапе load** — значит без смены ядра SPEC 071 (XHTTP в парсере) и 073 (AmneziaWG в парсере) физически не запустятся.

Эта SPEC — **чисто инфраструктурная**: меняет источник скачивания, имя ассета, формат pinned-версии, парсинг суффикса `-lx.1`, тексты Core Dashboard, install-скрипты и вводит проверку контрольной суммы. UI-маппинги транспортов/эндпоинтов и парсер-логика — **вне объёма** (это 071/073). Критический узел — отсутствие у форка `windows-386` ассета, на который сейчас завязана Win7-сборка лаунчера; решение по нему принимает мейнтейнер (см. § Риски).

## Контекст

Текущее состояние (по коду):

* `internal/constants/constants.go:61` — `const RequiredCoreVersion = "1.13.12"`. Pinned вручную (дисциплина SPEC 046), source-of-truth здесь.
* `internal/constants/constants_test.go:17` — `TestRequiredCoreVersion_SemVerShape` валидирует regex `^\d+\.\d+\.\d+$`. **Отвергнет** `1.13.13-lx.1`.
* `core/core_downloader.go:139` — `getReleaseInfoFromGitHub()` хардкодит `https://api.github.com/repos/SagerNet/sing-box/releases/tags/v<version>`.
* `core/core_downloader.go:26` — `const Win7LegacyVersion = "1.13.12"`; применяется безусловно при `GOOS=windows && GOARCH=386` (`DownloadCore()` строки 59-61).
* `core/core_downloader.go:197-241` — `buildSourceForgeAssets()` строит имена `sing-box-<version>-<os>-<arch>.{tar.gz,zip}` (для 386 — `sing-box-<version>-windows-386-legacy-windows-7.zip`). Зеркало `sourceforge.net/projects/sing-box.mirror/` — нет fork-артефактов.
* `core/core_downloader.go:245-280` — `SingboxAssetSuffix()` возвращает платформенный суффикс имени ассета (`windows-amd64.zip`, `darwin-arm64.tar.gz`, `windows-386-legacy-windows-7.zip`).
* `core/core_downloader.go:283-296` — `findPlatformAsset()` — `strings.Contains(asset.Name, suffix)`. Имя ассета форка `sing-box-1.13.13-lx.1-darwin-amd64.tar.gz` содержит суффикс `darwin-amd64.tar.gz` → substring-матч **уцелеет**.
* `core/core_downloader.go:298-337` — `downloadFile()`: GitHub → `ghproxy.com` → SourceForge. Контрольная сумма **не проверяется** (комментарий `// Size is unknown in advance`, строка 237).
* `core/core_version.go:43` — `GetInstalledCoreVersion()` парсит `sing-box version\s+(\S+)` — берёт ровно то, что печатает бинарь.
* `ui/core_dashboard_tab_status.go:247` — `case installedVersion != required:` — **строгое строковое равенство**. Если бинарь форка печатает `1.13.13` (а pin = `1.13.13-lx.1`) или наоборот, кнопка «Reinstall» будет висеть всегда.
* `ui/core_dashboard_tab.go:452` — hint `sing-box-<RequiredCoreVersion>-<suffix>`; строки 458-461 — гиперссылка `constants.SingboxReleasesURL` = `https://github.com/SagerNet/sing-box/releases` (constants.go:53).
* Build-скрипты (`build/build_darwin.sh`, `build_linux.sh`, `build_windows.bat`) ядро **не пиннят** — лаунчер качает его в runtime; `build_windows.bat:97` дефолтит `GOARCH=amd64`.

Возможности форка (релиз `v1.13.13-lx.1`):

* Ассеты: `{linux,darwin,windows}×{amd64,arm64}` + файл `SHA256SUMS`. **НЕТ `windows-386`.**
* Имя ассета: `sing-box-1.13.13-lx.1-<os>-<arch>.{tar.gz|zip}` (версия с суффиксом `-lx.1` внутри имени).
* `LX_TAGS` включает `with_xhttp,with_awg` (плюс штатные `with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api`).
* Конфиги с XHTTP/AWG-полями без соответствующих тегов — **отвергаются при load**, не деградируют молча.

## Объём / Вне объёма

**В объёме:**

* Параметризация источника релиза ядра на `Leadaxe/sing-box-lx` (GitHub API URL, releases-URL для UI-хинтов).
* Bump `RequiredCoreVersion` → `1.13.13-lx.1`; ослабление regex в `constants_test.go` под суффикс `-lx.N`.
* Корректный парсинг/сравнение версии с суффиксом `-lx.1` (что печатает бинарь vs что pinned).
* Имя ассета форка в `buildSourceForgeAssets()` / `SingboxAssetSuffix()` / `findPlatformAsset()`.
* Проверка `SHA256SUMS` скачанного архива (новая функциональность, защита от подмены на зеркале).
* Тексты Core Dashboard (releases-URL, manual-hint имя файла).
* Решение и реализация по **windows-386 (Win7)**: один из вариантов A–D ниже, выбор фиксируется мейнтейнером.
* Install/build-скрипты — ровно настолько, насколько они касаются версии/арки ядра (фактически: документировать, что 386 больше не качается с форка; обновить комментарии).
* `extractArchive`/`extractZip`/`extractTarGz` — проверить, что матч бинаря внутри архива (`sing-box`/`sing-box.exe`) работает для форка (вероятно без правок).

**Вне объёма:**

* Маппинг XHTTP-транспорта в парсере/билдере (`node_parser_transport.go`, `outbound_jsonbuilder.go`) — **SPEC 071**.
* Парсинг AmneziaWG URI и эмиссия `jc/jmin/jmax/s1–s4/h1–h4/i1–i5` (`node_parser_wireguard.go`, `shareuri_wireguard.go`) — **SPEC 073**.
* UI-формы для XHTTP/AWG-полей, шаблонные preset-паттерны (`bin/wizard_template.json`) для новых протоколов.
* Любые изменения семантики `CompareVersions()` для launcher-версии (`AppVersion`) — она остаётся `X.Y.Z[-suffix]`, отдельный трек.
* Самостоятельная сборка `windows-386` из исходников форка как часть этой SPEC (если выбран вариант D — это отдельная задача в § Риски).

## Входные данные

Файлы и функции к правке (точные пути):

| Файл | Сущность | Что меняется |
|---|---|---|
| `internal/constants/constants.go:61` | `RequiredCoreVersion` | `"1.13.12"` → `"1.13.13-lx.1"` |
| `internal/constants/constants.go:53` | `SingboxReleasesURL` | → `https://github.com/Leadaxe/sing-box-lx/releases` (или dual-source хинт) |
| `internal/constants/constants.go` | **новое** `SingboxCoreRepo` | `"Leadaxe/sing-box-lx"` — единый source для API URL |
| `internal/constants/constants_test.go:17-22` | `TestRequiredCoreVersion_SemVerShape` | regex `^\d+\.\d+\.\d+$` → `^\d+\.\d+\.\d+(-lx\.\d+)?$` |
| `core/core_downloader.go:26` | `Win7LegacyVersion` | зависит от варианта A–D (см. Фаза 4) |
| `core/core_downloader.go:139` | `getReleaseInfoFromGitHub()` | URL через `constants.SingboxCoreRepo` |
| `core/core_downloader.go:197-241` | `buildSourceForgeAssets()` | имя ассета форка / отключить SourceForge-ветку для форка |
| `core/core_downloader.go:245-280` | `SingboxAssetSuffix()` | без правок для amd64/arm64; 386-ветка по варианту |
| `core/core_downloader.go` | **новое** проверка SHA256 | скачать `SHA256SUMS`, сверить хэш архива до extract |
| `core/core_version.go:43` | `GetInstalledCoreVersion()` regex | проверить, что `\S+` ловит `1.13.13-lx.1` целиком |
| `ui/core_dashboard_tab_status.go:247` | `installedVersion != required` | привести в соответствие с тем, что печатает бинарь |
| `ui/core_dashboard_tab.go:452,458-461` | manual-hint + releases-link | имя файла форка, URL форка |
| `build/build_windows.bat:97` | дефолт `GOARCH` | комментарий/документация про 386-gap |

Формы конфигов (целевые, эмитятся 071/073 — здесь только для контекста «что должно загрузиться без отказа на новом ядре»):

```jsonc
// XHTTP transport (SPEC 071 эмитит; ядро форка обязано принять):
{"type":"xhttp","host":"example.com","path":"/xhttp",
 "mode":"auto|packet-up|stream-up|stream-one",
 "headers":{"User-Agent":"..."},"x_padding_bytes":"100-1000","no_grpc_header":false}
```

```jsonc
// AmneziaWG 2.0 endpoint (SPEC 073 эмитит; поля promoted в корень endpoint):
{"type":"wireguard","address":["10.0.0.2/32"],"private_key":"...","mtu":1408,
 "jc":10,"jmin":50,"jmax":100,"s1":20,"s2":20,"s3":60,"s4":60,
 "h1":1234567890,"h2":1234567891,"h3":1234567892,"h4":1234567893,
 "i1":"<b 0x000100002112a442><r 12>","i2":"...","i3":"<r 24>","i4":"","i5":"",
 "peers":[{"address":"server.example.com","port":51821,"public_key":"...",
           "pre_shared_key":"...","allowed_ips":["0.0.0.0/0","::/0"],
           "persistent_keepalive_interval":25}]}
```

```jsonc
// ReleaseInfo от GitHub API форка:
{"tag_name":"v1.13.13-lx.1",
 "assets":[{"name":"sing-box-1.13.13-lx.1-darwin-amd64.tar.gz",
            "browser_download_url":"https://github.com/Leadaxe/sing-box-lx/releases/download/v1.13.13-lx.1/...",
            "size":0}]}
```

## Фазы

Порядок: сперва строковый контракт версии (иначе тесты красные), потом источник, потом checksum, потом Win7-развилка, потом UI/доки/тесты.

### Фаза 1 — Версия-контракт: pin + regex + парсинг суффикса

Deliverables:

1. `constants.go:61` → `const RequiredCoreVersion = "1.13.13-lx.1"`.
2. `constants_test.go:18` → regex `^\d+\.\d+\.\d+(-lx\.\d+)?$` (принять `-lx.N`, по-прежнему отвергнуть branch-имена/мусор). Комментарий обновить: core-версия теперь fork-tagged.
3. **Решить и зафиксировать строковый контракт** между `RequiredCoreVersion` и выводом `sing-box version` форка. Два под-варианта (мейнтейнер выбирает по тому, что печатает реальный бинарь):
   * (1a) Бинарь печатает `1.13.13-lx.1` → строгое равенство `installedVersion != required` (`core_dashboard_tab_status.go:247`) работает as-is. **Предпочтительно** — попросить мейнтейнера форка вшить полный tag в `version`.
   * (1b) Бинарь печатает `1.13.13` (upstream base без суффикса) → строгое равенство **сломается** (вечная «Reinstall»). Тогда ввести `core.CoreVersionMatches(installed, required string) bool`, сравнивающую только базовую `X.Y.Z` (re-use `extractBaseVersion` из `core_version.go:217`), и заменить `!=` на `!CoreVersionMatches(...)`.
4. Проверить, что regex `sing-box version\s+(\S+)` в `core_version.go:43` ловит `1.13.13-lx.1` целиком (точка/дефис в `\S+` — да; добавить unit-тест на фикстуру вывода).

Verification: `go test ./internal/constants/... ./core/...` зелёный; добавлен тест `TestGetInstalledCoreVersion_ParsesLxSuffix` с фикстурой `sing-box version 1.13.13-lx.1\n...`. Юнит-тест на выбранный контракт (1a или 1b): `installed="1.13.13-lx.1"` против `required="1.13.13-lx.1"` → match.

### Фаза 2 — Источник релиза: форк вместо SagerNet

Deliverables:

1. `constants.go` — новый `const SingboxCoreRepo = "Leadaxe/sing-box-lx"`.
2. `core_downloader.go:139` — URL через `fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/v%s", constants.SingboxCoreRepo, version)`.
3. `constants.go:53` `SingboxReleasesURL` → `https://github.com/Leadaxe/sing-box-lx/releases`.
4. `buildSourceForgeAssets()` (`core_downloader.go:197-241`): у форка **нет** SourceForge-зеркала. Решение: SourceForge-fallback для форка **отключить** (вернуть пустой список ассетов / лог-warn «no fork mirror»), оставив только GitHub + ghproxy цепочку (`downloadFile()` строки 308-335 — ветка `strings.Contains(url, "github.com")` → SourceForge URL должна быть закрыта для fork-репо, чтобы не дёргать несуществующее зеркало).
5. `findPlatformAsset()` (строка 290): подтвердить substring-матч на имени `sing-box-1.13.13-lx.1-darwin-amd64.tar.gz` против суффикса `darwin-amd64.tar.gz` — работает; добавить тест-фикстуру с fork-именами.

Verification: тест `TestGetReleaseInfoFromGitHub_UsesForkRepo` (мок HTTP, проверить, что запрошенный URL содержит `Leadaxe/sing-box-lx`); `TestFindPlatformAsset_ForkAssetNames` (fork-имена ассетов матчатся для darwin/linux/windows amd64+arm64).

### Фаза 3 — Проверка контрольной суммы (SHA256SUMS) — ❌ СНЯТА (решение мейнтейнера)

> **Descoped 2026-06-09.** Реализовано, затем удалено по решению мейнтейнера: архив и
> `SHA256SUMS` берутся из одного и того же GitHub-релиза по HTTPS, поэтому само-сверка
> не создаёт отдельной границы доверия (компрометирующий релиз правит и архив, и сумму),
> а целостность канала уже обеспечивает TLS. SourceForge-фоллбэк для форк-пути отключён
> (`coreReleaseIsLegacy()`), так что сценария «недоверенное зеркало + доверенная сумма»
> на форк-пути нет. Функции `fetchSHA256SUMS`/`parseSHA256SUMS`/`verifyChecksum`, шаг 4b
> в `DownloadCore` и связанные тесты удалены. Описание ниже оставлено как исторический
> контекст того, что обсуждалось.

Deliverables (не реализовано):

1. Новый ассет `SHA256SUMS` есть в каждом fork-релизе. После скачивания архива (`DownloadCore` шаг 4, строки 94-100), **до** extract (шаг 5): скачать `SHA256SUMS` (найти в `release.Assets` по имени, либо собрать URL рядом с архивом), распарсить строки `<sha256>␣␣<filename>`, найти строку для `asset.Name`, посчитать SHA256 локального файла, сравнить.
2. Несовпадение → `DownloadProgress{Status:"error", Error: ...}` с понятным сообщением, архив удаляется (temp dir и так чистится `defer`, строки 88-92).
3. Если `SHA256SUMS` недоступен (старый релиз / сетевой сбой) — мягкая деградация: warn-log + продолжить (не блокировать установку), чтобы не превратить опциональную защиту в единую точку отказа. Это явное решение, задокументировать.
4. Новая функция `verifyChecksum(archivePath, assetName string, sums map[string]string) error` + `fetchSHA256SUMS(ctx, release) (map[string]string, error)`.

Verification: `TestVerifyChecksum_Match` / `_Mismatch` на синтетическом файле + известном хэше; `TestFetchSHA256SUMS_Parse` на фикстуре формата GNU coreutils. Ручная: установка ядра проходит, в логах — `checksum OK`.

### Фаза 4 — windows-386 (Win7) gap — РАЗВИЛКА МЕЙНТЕЙНЕРА

Форк не публикует `windows-386`. Текущий код безусловно качает `Win7LegacyVersion=1.13.12` для `windows/386` (`core_downloader.go:59-61`). Этот pin указывает на **SagerNet** upstream — а Фаза 2 переключила API-URL на форк, у которого 386 нет. Без явного решения Win7-сборка сломается (404 на fork-релизе). Мейнтейнер выбирает **ровно один** вариант; реализация — под него:

* **Вариант A — Drop Win7 entirely.** Убрать `windows/386` из build-матрицы лаунчера, удалить `Win7LegacyVersion` и 386-ветки в `SingboxAssetSuffix()`/`buildSourceForgeAssets()`/`DownloadCore`. Breaking для Win7-юзеров. Чистейший код. XHTTP/AWG для Win7 невозможны в любом случае (фич-бинаря нет).
* **Вариант B — Dual-source: Win7 остаётся на SagerNet 1.13.12.** `windows/386` продолжает качать `Win7LegacyVersion=1.13.12` **с SagerNet** (отдельный repo-const `SingboxLegacyRepo="SagerNet/sing-box"`), все остальные платформы — форк. Win7 не получает XHTTP/AWG (ожидаемо), но и не ломается. Минимально-инвазивно, **рекомендуемый дефолт**. `getReleaseInfoFromGitHub` параметризуется repo (legacy vs fork) по `GOARCH==386`.
* **Вариант C — Попросить мейнтейнера форка добавить `windows-386` в ветку `lx`.** Откладывает миграцию до выхода ассета; затем `Win7LegacyVersion` поднимается до `1.13.13-lx.1` и Win7 уходит на форк целиком. Зависит от внешней стороны.
* **Вариант D — Собрать `windows-386` форка как third-party артефакт** и положить на собственное зеркало; добавить fork-mirror в fallback-цепочку. Больше всего работы и поддержки.

Deliverables: реализация выбранного варианта + ADR-абзац в этой SPEC (зафиксировать выбор и причину) + запись в release notes.

Verification: для A — build-матрица не содержит 386, `go build` под остальные таргеты ок. Для B — мок-тест `TestDownloadCore_Win7UsesLegacyRepo` (при `GOARCH=386` URL содержит `SagerNet/sing-box` и версию `1.13.12`; иначе — форк). Для C/D — интеграционная проверка реального скачивания 386.

### Фаза 5 — UI Core Dashboard тексты

Deliverables:

1. `core_dashboard_tab.go:452` — hint имя файла: для amd64/arm64 будет `sing-box-1.13.13-lx.1-<suffix>` автоматически (берёт `RequiredCoreVersion`). Проверить визуально, что суффикс `-lx.1` читается нормально в строке вида `sing-box-1.13.13-lx.1-windows-amd64.zip`.
2. `core_dashboard_tab.go:458-461` — releases-link уже через `constants.SingboxReleasesURL` (Фаза 2 п.3 обновил).
3. Локали `internal/locale/*.json`: ключи `core.button_download_version` / `core.button_reinstall_version` принимают версию аргументом (`Tf`) — текст не хардкодит число, правок строк не требует; но **проверить**, что «vX» + `1.13.13-lx.1` рендерится как `v1.13.13-lx.1` без двойного «v».
4. Для Win7-варианта B: рассмотреть отдельный tooltip «Win7 использует legacy-ядро 1.13.12 без XHTTP/AWG» (опционально, по решению мейнтейнера).

Verification: ручной прогон Core Dashboard на свежей установке (кнопка «Download v1.13.13-lx.1»), после установки — кнопка скрыта (контракт Фазы 1 п.3 удовлетворён). На несовпадающей версии — «Reinstall v1.13.13-lx.1».

### Фаза 6 — Install / build-скрипты + документация

Deliverables:

1. Build-скрипты ядро не пиннят (download runtime) — менять логику не нужно. Обновить комментарии в `build/build_windows.bat` про 386-gap согласно выбранному варианту.
2. Документация (по дисциплине house-style — обновить при закрытии):
   * `docs/RELEASE_PROCESS.md` §5.1 — источник ядра теперь `Leadaxe/sing-box-lx`; процедура bump `RequiredCoreVersion` на `-lx.N`; чеклист: при новом `-lx.N` поднять константу вручную (auto-discovery нет, by design SPEC 046).
   * `docs/ARCHITECTURE.md` — у `core/core_downloader.go` отметить fork-source + checksum-проверку.
   * `docs/release_notes/upcoming.md` (EN/RU) — переход на форк, доступность XHTTP/AWG, **явный** пункт про Win7 (по варианту).
3. SPEC-cross-ref: эта SPEC — prerequisite для 071 (XHTTP parser) и 073 (AmneziaWG parser).

Verification: `grep -rn "SagerNet" core/ internal/ ui/` — не должно остаться боевых ссылок на SagerNet, кроме (если вариант B) явной legacy-Win7-ветки и историч. комментариев.

### Фаза 7 — Тесты (acceptance) + ручная установка

Deliverables (юнит):

* `constants_test.go` — regex принимает `1.13.13-lx.1`, отвергает `1.13.13-lx` (без числа), `lx.1`, `1.13`, branch-имена.
* `core_downloader_test.go` — fork-repo URL, fork-asset matching (все 6 платформ), checksum match/mismatch/missing, Win7-ветка (по варианту).
* `core_version_test.go` — парсинг `-lx.1` из `sing-box version`; выбранный контракт сравнения (1a/1b).
* Существующие тесты download/version не ломаются.

Ручная матрица:

| Платформа | Ожидание |
|---|---|
| darwin/arm64 | качает `sing-box-1.13.13-lx.1-darwin-arm64.tar.gz`, checksum OK, версия совпала, кнопка скрыта |
| windows/amd64 | качает `...-windows-amd64.zip`, ставит `sing-box.exe`, версия совпала |
| linux/amd64 | качает `...-linux-amd64.tar.gz`, версия совпала |
| windows/386 (Win7) | по варианту: A — таргета нет; B — качает SagerNet 1.13.12, ставится, без XHTTP/AWG; C/D — fork 386 |
| Конфиг с XHTTP/AWG-полями | загружается ядром без отказа (sanity: `sing-box check` на эмиченном 071/073 конфиге) |

Verification: `./build/build_darwin.sh -i arm64` → переустановка ядра через Core Dashboard → ручная матрица. `go test ./...` зелёный.

## Риски и открытые вопросы

* **КРИТИЧНО — windows-386 (Win7) gap.** Реальное решение мейнтейнера (Фаза 4, варианты A–D). Рекомендация: **B** (Win7 остаётся на SagerNet 1.13.12 dual-source) как наименее ломающее; XHTTP/AWG для 32-бит Win7 недоступны в любом случае. Зафиксировать выбор ADR-абзацем перед реализацией.
* **Что печатает `sing-box version` форка** (Фаза 1 п.3) — нужно проверить на реальном бинаре `v1.13.13-lx.1`. Если печатает голую `1.13.13` без суффикса, строгое равенство в `core_dashboard_tab_status.go:247` даст вечную «Reinstall» → обязателен под-вариант 1b (`CoreVersionMatches` по базе). Запросить у мейнтейнера форка вшивание полного tag в `version` (под-вариант 1a, чище).
* **Готовность фич форка.** Research: XHTTP помечен «O» (live-test против Xray pending; расхождение в размещении padding — `X-Padding` header vs `Referer` query, варьируется по версии Xray); `mode=auto` алиасится в `stream-one` (нет реальной negotiation). AWG — «C» (live-validated). Подтвердить production-readiness XHTTP до релиза; возможно, релизить лаунчер с AWG, но XHTTP пометить experimental.
* **AWG CPS-параметры `i1–i5` config-only, не negotiated** — рассинхрон client/server рвёт соединение. Это предупреждение для UI — но UI вне объёма этой SPEC; зафиксировать как требование к 073.
* **SHA256SUMS мягкая деградация** (Фаза 3 п.3) — если зеркало `ghproxy` не отдаёт `SHA256SUMS`, проверка пропускается с warn. Альтернатива — жёсткий fail; решение мейнтейнера (дефолт — мягкая деградация, HTTPS+GitHub authenticity остаются базовой защитой как сейчас).
* **Авто-bump на новые `-lx.N`.** By design (SPEC 046) auto-discovery нет — каждый новый `v1.13.13-lx.2` требует ручного bump `RequiredCoreVersion`. Внести в release-чеклист `docs/RELEASE_PROCESS.md`.
* **SPEC 046 template-invalidation** — смена `RequiredCoreVersion` сама по себе шаблон не инвалидирует (инвалидация завязана на `AppVersion` vs `LastTemplateLauncherVersion`, не на core-версию); если 071/073 добавят шаблонные preset-паттерны под XHTTP/AWG, инвалидация поедет через свой bump `AppVersion` в тех SPEC. Здесь — без изменений template-логики.
* **`extractVersionAndFileName()` (core_downloader.go:480)** парсит версию из URL-пути по префиксу `v` — для fork-URL `.../download/v1.13.13-lx.1/...` вернёт `1.13.13-lx.1`. Используется только в SourceForge-fallback, который для форка отключаем (Фаза 2 п.4) — низкий риск, но добавить тест-фикстуру с fork-URL.

## Принцип очерёдности

* **072 — фундамент, ставится первой.** XHTTP (071) и AmneziaWG (073) добавляют в парсер/билдер поля, которые **только ядро форка** принимает без отказа на load. Пока ядро = SagerNet `1.13.12`, конфиги 071/073 будут отвергнуты на старте sing-box → их e2e/ручная верификация физически невозможна.
* **Зависимость:** 071 → 072, 073 → 072. 071 и 073 между собой независимы (разные протоколы, разные файлы парсера: `node_parser_transport.go` vs `node_parser_wireguard.go`) — могут идти параллельно после landing 072.
* **072 самодостаточна** — не зависит ни от 071, ни от 073. После её закрытия лаунчер качает fork-ядро, в котором фичи уже встроены под build-тегами; визард их ещё не эмитит (это сделают 071/073), но загрузка вручную написанного XHTTP/AWG-конфига через Raw-JSON уже работает.
* **Внутри 072 строгий порядок фаз:** Фаза 1 (версия-контракт) перед всем остальным — иначе `go test ./internal/constants/...` красный из-за regex и весь branch в красном. Фаза 4 (Win7) — после Фазы 2, т.к. именно переключение API-URL на форк обнажает 386-gap.
* **Release-gate:** 072 релизится только с bump `AppVersion` (semver) — иначе у пользователей на той же версии лаунчера ядро не переустановится автоматически (Download-кнопка появится, но pinned-сравнение сработает лишь при наличии нового бинаря). Bump `RequiredCoreVersion` + `AppVersion` — в одном релизе.
