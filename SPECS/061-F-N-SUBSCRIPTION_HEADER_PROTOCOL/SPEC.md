# SPEC 061-F-N — SUBSCRIPTION_HEADER_PROTOCOL

**Status:** New (N)
**Type:** Feature (F) — формализовать полный набор HTTP-заголовков подписки (response + request) которые наш launcher распознаёт и отправляет, в соответствии с де-факто стандартом, формирующимся в XTLS / Marzban / Remnawave / V2Board / Sub-Store ecosystem'е.
**Depends on:** SPEC 052 (subscription cache + per-source meta) — расширяет существующий `core/config/subscription` слой без рефакторинга.
**Не меняет:** wire format подписки (то что в теле — `vless://`/`vmess://`/Clash YAML/sing-box JSON остаётся как есть), state.json schema, build pipeline.

---

## Проблема

В `core/config/subscription/meta.go` мы распознаём 6 response-headers (`Subscription-Userinfo`, `Profile-Title`, `Profile-Update-Interval`, `Support-Url`, `Profile-Web-Page-Url`, `Content-Disposition`), и отправляем единственный кастомный `User-Agent: SubscriptionParserClient`. Этого недостаточно для современных панелей:

1. **HWID-binding панели (Remnawave / Marzneshin / Marzban-fork) блокируют unknown UA.** Реальный случай (NashVPN, `sub.towersflowerss.com`): провайдер отдаёт HTTP 200 + 0 байт + `announce` header'ы, потому что наш `SubscriptionParserClient` UA не распознаётся как легитимный клиент. Юзер видит «empty subscription body» вместо реальной причины.

2. **HWID-binding панели хотят слышать `X-Hwid`-семейство** в запросе чтобы зарегистрировать устройство и считать его одним из N разрешённых. Без этих заголовков мы пробиваемся как «не клиент» → 0 байт.

3. **Response-headers `Announce` / `Announce-Url` / `X-Hwid-*`** уже шлются провайдерами, но мы их игнорируем. Полезный сигнал теряется (юзер не видит «лимит устройств → @bot»).

4. **Inline `#header: value` парсер** покрывает только legacy V2Board-набор; новые ключи (`announce`, `x-hwid-*`) inline не распознаются. Часть провайдеров хостит подписку на статике (GitHub Pages, Gist) — HTTP-headers контролировать не могут, шлют через inline.

Источники канонического определения:
- **[XTLS Subscription Standards (discussion #4877)](https://github.com/XTLS/Xray-core/discussions/4877)** — community-draft, активно адаптируется панелями.
- **[Remnawave HWID Device Limit docs](https://docs.rw/docs/features/hwid-device-limit/)** — наиболее детальная спецификация HWID-family заголовков.
- **[Marzban subscription docs](https://gozargah.github.io/marzban/en/docs/subscription)** — пращур большинства current-gen панелей.
- **[v2rayN issue #3438](https://github.com/2dust/v2rayNG/issues/3438)** — внедрение `profile-update-interval`.
- **[sing-box clash API](https://sing-box.sagernet.org/configuration/experimental/clash-api/)** — Clash-совместимый формат.

---

## Response headers (server → клиент)

Все парсятся в `core/config/subscription/meta.go::ParseHeaders` и/или `ParseInlineComments`. Hint: HTTP-заголовки и inline-комменты (`#header: value` в первых N строках body) — **симметричный набор**: то, что бывает в response-headers, бывает и inline. Reason: статические хостинги (GitHub Pages, Gist, S3) не дают контроля над HTTP-headers, провайдеры эмитят через inline.

### 0. Базовые HTTP

| Header | Required? | Что значит для нас | Куда кладём |
|---|---|---|---|
| `Content-Type` | required | MIME ответа. Используем для эвристики «это JSON или text/uri-list». | implicit (HTTP stack) |
| `Content-Disposition` | required | `attachment; filename="my-profile.txt"` — suggested filename для UI / cache file naming. | `SubscriptionMeta.ContentDispositionFilename` |

### 1. Profile metadata (XTLS canonical)

| Header | Required? | Формат | `SubscriptionMeta` field |
|---|---|---|---|
| `Profile-Title` | preferred | Base64-encoded UTF-8 string ИЛИ plain UTF-8. Префикс `base64:` опционален; auto-detect через `decodeProfileTitle`. | `ProfileTitle` |
| `Profile-Update-Interval` | preferred | Целое число часов (например `24`). 0 / отрицательное / non-numeric → игнорим. | `ProfileUpdateIntervalHours` |
| `Profile-Web-Page-Url` | optional | URL «личного кабинета». UI рендерит как кликабельный link рядом с подпиской. | `ProfileWebPageURL` |
| `Support-Url` | optional | URL саппорта (часто Telegram/Discord/website). Кликабельно. | `SupportURL` |

### 2. Traffic / quota (XTLS canonical)

| Header | Required? | Формат | `UserInfo` field |
|---|---|---|---|
| `Subscription-Userinfo` | optional | `upload=BYTES; download=BYTES; total=BYTES; expire=UNIX_TIMESTAMP` (separator `;` или `,`, пробелы tolerated). | `UploadBytes`, `DownloadBytes`, `TotalBytes`, `ExpireUnix` |

UI uses это для прогресс-бара «80% траффика» + countdown «expires in 12 days». Pure-bytes (не human-readable строки!) — мы парсим через `strconv.ParseInt`. Маlformed → `ui = nil` (silently dropped, не блокирует remaining meta).

### 3. Announcement (XTLS optional + Remnawave extension)

| Header | Формат | Состав `ProviderAnnounce` |
|---|---|---|
| `Announce` | Base64 UTF-8 или plain text. Произвольное сообщение провайдера юзеру («истёк trial», «лимит устройств», «новости»). | `Message` (decoded) |
| `Announce-Url` | URL. Кликабельная ссылка к announce — обычно Telegram-бот или billing page. | `URL` |

**Семантика relative к body:**
- `Announce` + non-empty body → информационное сообщение (юзер должен прочитать, но подписка работает).
- `Announce` + **empty body** → блокирующий gate (подписка не отдаётся пока юзер не выполнит инструкции в announce). Маппим в `FetchAnnounceError` чтобы UI показал actionable dialog вместо плоской ошибки.

### 4. HWID-family (Remnawave / Marzneshin)

Четыре булевых флага. На сейчас провайдеры шлют их в разных комбинациях — самый широкий case: NashVPN шлёт только `X-Hwid-Limit` (legacy v2RayTun alias) + empty body + `Announce`. Modern Remnawave-installs шлют все 4.

| Header | Значение `true` | UI семантика |
|---|---|---|
| `X-Hwid-Active` | панель включила HWID-binding (на стороне сервера) | информационно — «провайдер считает устройства», показать badge в meta |
| `X-Hwid-Not-Supported` | binding включён, но клиент не прислал `X-Hwid` request-header | actionable — «launcher должен слать X-Hwid» (после внедрения §"Request headers" этот флаг исчезнет) |
| `X-Hwid-Max-Devices-Reached` | юзер исчерпал лимит устройств | actionable — показать «удалите старое устройство», link на `Announce-Url` / провайдерский панель |
| `X-Hwid-Limit` | legacy alias `X-Hwid-Max-Devices-Reached` (backwards-compat для v2RayTun) | прозрачно мапим в тот же UI-флаг что и `X-Hwid-Max-Devices-Reached` |

Truthy-значения: `true` / `1` / `yes` / `on` (case-insensitive). Всё остальное → `false`.

### 5. Inline `#header: value` parity

Любой из response-headers `§0-§3` может приходить как `#header-name: value` в первых строках body (до первой не-`#` строки). Сейчас в `ParseInlineComments` распознаются только §0-§2 (`subscription-userinfo`, `profile-title`, `profile-update-interval`, `support-url`, `profile-web-page-url`, `content-disposition`). Расширяется только на §3 (`announce`, `announce-url`).

**HWID-флаги (§4) inline НЕ парсятся** — у HWID-binding панелей серверная device-attribution логика, статические хостинги (где inline актуален) её по определению не имеют. Inline parsing HWID был бы dead code.

### 6. Merge precedence

`MergeMeta(headers, inline)` — HTTP-headers выигрывают, inline — fallback для пустых полей. Не меняется.

---

## Request headers (клиент → server)

Сейчас `fetcher.go::FetchSubscriptionWithMeta` шлёт только `User-Agent: SubscriptionParserClient`. Этого недостаточно — провайдеры с HWID-binding нас не распознают.

### 1. User-Agent

| Текущий | Новый |
|---|---|
| `SubscriptionParserClient` | `singbox-launcher/<version> (<os> <arch>)` |

Примеры:
- `singbox-launcher/0.9.8 (macOS arm64)`
- `singbox-launcher/0.9.8 (windows amd64)`
- `singbox-launcher/0.9.8 (linux amd64)`

Reason:
- Панели ведут whitelist клиентов; имя `SubscriptionParserClient` не выглядит как media-клиент и блокируется HWID-binding панелями (NashVPN: 0 байт response).
- Форма `product/version (platform)` — общепринятая (тот же `Mozilla/5.0 (...)`-шаблон), панели её распознают как known-good клиента.
- Честно идентифицируем себя — провайдер видит реальную версию + платформу, может делать targeted Min-Version checks (если когда-нибудь понадобится).
- OS/arch дублируются в `X-Device-OS` / `X-Ver-OS` (§2), но провайдеры, которые их не парсят, всё равно вытащат через UA — defence-in-depth.

**Реализация:** `core/config/configtypes/types.go` — `SubscriptionUserAgent` (const) заменяется на `BuildSubscriptionUserAgent() string` (func, читает `internal/constants.AppVersion`, `runtime.GOOS`, `runtime.GOARCH`). `GOOS` маппится в `macOS` / `windows` / `linux` (canonical case как у Remnawave docs). Прежний const убрать — нет callsite'ов которые должны оставить старое имя.

### 2. HWID identification (Remnawave / Marzneshin)

Отправляем при каждом fetch'е. Провайдеры без HWID-binding эти headers просто игнорируют (no-op). Provider'ы с binding'ом — регистрируют устройство и считают его одним из N разрешённых.

| Header | Источник значения | Пример |
|---|---|---|
| `X-Hwid` | стабильный UUIDv4, генерируется на первой инсталляции лаунчера и сохраняется в `bin/settings.json` под ключом `hwid` (через `internal/locale/settings.go::Settings.HWID`). Lazy: если ключ пустой при чтении — generate + persist. **Юзер-редактируемое** в Settings tab (для продвинутых: перенос HWID между установками, fake-rotation). ≤36 chars. | `7c9e6679-7425-40de-944b-e07fc1f90ae7` |
| `X-Device-OS` | `runtime.GOOS` mapping → `macOS` / `Windows` / `Linux` (capitalized как принято в Remnawave docs) | `macOS` |
| `X-Ver-OS` | `runtime.Version()` слишком технично; берём system release. Через platform-specific: macOS — `sw_vers -productVersion`, Windows — `cmd /c ver`, Linux — `/etc/os-release`. Cached at startup. | `15.2` |
| `X-Device-Model` | platform-specific: macOS — `sysctl hw.model`, Windows — `wmic computersystem get Model`, Linux — `/sys/devices/virtual/dmi/id/product_name`. Cached at startup; «unknown» на ошибке. **Hashed mode** (per setting): `sha256(model)[:16]` — провайдер получает стабильный opaque ID без leak'а конкретного hardware family. | full: `MacBookPro18,1`<br>hash: `a1b2c3d4e5f60718` |

**Privacy / opt-out:** все четыре header'а — пер-device idempotent identifiers. Не отправляем серверу никаких persistent identifiers с пользовательской привязкой (email, account ID). HWID UUID — random, не derived из system serial. Юзер может вайпнуть его удалив `hwid` ключ из `bin/settings.json` — лаунчер сгенерит новый при следующем старте.

Settings (Phase 4):
- `subscription_send_hwid: bool` (default `true`) — отключает отправку всех 4 заголовков (для юзеров, которые НЕ хотят регистрироваться). Опт-аут уровень nuclear: подписка работает только если провайдер не требует HWID-binding.
- `subscription_device_model_hashed: bool` (default `false`) — если `true`, в `X-Device-Model` уходит `sha256(model)[:16]` вместо `MacBookPro18,1`. Provider получает стабильный opaque ID (всё ещё считается одним device'ом в его counter'е), но конкретная hardware-модель не leak'ается. Middle ground между full и off.

### 3. Что НЕ отправляем

- **Никаких заголовков с user-controlled данными** (email, имя сабскрипшна, label). Все идентификаторы — машинные.
- **`Authorization`** — формат не определён в XTLS standard'е; провайдеры обычно зашивают токен в path/query вместо header'а.
- **Cookie-based session** — не используется ни одним современным панелем подписок.

---

## Изменения в коде

### Phase 1 — Response side

| Файл | Что |
|---|---|
| `core/config/subscription/meta.go` | Расширить `ProviderAnnounce` 4 HWID-флагами (`Active`, `NotSupported`, `MaxDevicesReached`, `Limit` уже есть). Распознавать `Announce` и `X-Hwid-*` в inline-варианте через `ParseInlineComments`. |
| `core/config/subscription/fetcher.go` | `FetchAnnounceError` уже есть (этот PR). Дополнить: если *любой* HWID-флаг присутствует но body не пустой — добавляем announce в `result.Meta.Provider` как warning, не как error (фетч успешен, но юзеру стоит знать). |
| `core/state/connections.go` | Добавить `SubscriptionMeta.ProviderAnnounce *ProviderAnnounce` field (optional, omit на serialize если nil/IsEmpty). |
| `core/config/subscription/meta_test.go` | Покрыть inline-варианты новых ключей + 4 HWID-флага matrix. |

### Phase 2 — Request side

| Файл | Что |
|---|---|
| `core/config/configtypes/types.go` | Заменить константу `SubscriptionUserAgent` на `BuildSubscriptionUserAgent() string` — формат `singbox-launcher/<v> (<os> <arch>)`. Reads `internal/constants.AppVersion` + `runtime.GOOS`/`runtime.GOARCH` (`GOOS` → canonical case: `macOS` / `windows` / `linux`). |
| `core/config/subscription/fetcher.go` | `FetchSubscriptionWithMeta` строит UA через builder + добавляет X-Hwid family. Опционально (settings.send_hwid=false → skip HWID-headers). |
| `internal/platform/device_info_{darwin,windows,linux}.go` | Новый файл per-OS — `DeviceOS()`, `DeviceOSVersion()`, `DeviceModel()`. Caching at startup; «unknown» fallback. |
| `internal/locale/settings.go` | Расширить `Settings` струк двумя полями: `HWID string \`json:"hwid,omitempty"\`` (lazy-generated UUIDv4 на первой инсталляции), `SubscriptionSendHWID *bool \`json:"subscription_send_hwid,omitempty"\`` (nil → true по дефолту; явный false выключает все 4 X-Hwid-* headers). Helper `Settings.EnsureHWID()` — generate+save если пусто. |

### Phase 3 — UI surfacing

| Файл | Что |
|---|---|
| `ui/configurator/...` | Если `SubscriptionMeta.ProviderAnnounce` не nil — показывать badge в Sources tab под source row: `📢 NashVPN: Вы достигли лимита устройств → @nash_vpn_bot`. URL кликабельный. |
| `ui/configurator/tabs/source_tab.go` | На fetch failure через `FetchAnnounceError` — диалог с decoded announce + actionable button «открыть Announce-Url». |

### Phase 4 — Settings (privacy opt-out + advanced editing)

| Файл | Что |
|---|---|
| `bin/settings.json` schema | Новые ключи: <br>• `hwid` (string UUIDv4, lazy-generated at first launch — никогда не пустой после первого Save). <br>• `subscription_send_hwid` (bool, default `true` — выключает все 4 X-Hwid headers если `false`). <br>• `subscription_device_model_hashed` (bool, default `false` — sha256-усечение `X-Device-Model` если `true`). <br>Все поля живут как top-level keys в `Settings` структ (см. Phase 2 row для `internal/locale/settings.go`). |
| `ui/settings_tab.go` (Settings tab) | Блок «Subscription identification»: <br>• Чекбокс **«Send device identification to providers (required for HWID-binding panels)»** — default checked. Tooltip явно перечисляет что отсылается: «UUIDv4 (random per install), OS family + version (e.g. macOS 15.2), exact device model (e.g. MacBookPro18,1). Required by Marzban / Remnawave / NashVPN-style panels for device counting. Unchecking returns subscription provider a less-fingerprinted request — may break HWID-binding panels.» <br>• Чекбокс **«Hash device model (privacy)»** — default unchecked, disabled когда send_hwid=false. Tooltip: «If checked, X-Device-Model header is sent as sha256(model)[:16] instead of MacBookPro18,1. Provider still counts you as one device but doesn't see the hardware family.» <br>• Поле **«Device ID (HWID)»** — отображает текущий UUID, editable. Validation: `uuid.Parse` валиден; невалид → toast + revert. Кнопка **«Regenerate»** — generate fresh UUIDv4 (для тех кто хочет «переустановить» себя на провайдере). Для advanced/power users: перенос HWID между установками (скопировать UUID со старого Mac → вставить в новый чтобы не съесть лишний device slot). |

---

## Контракт ошибок

- `FetchHTTPError` (status != 200) — **расширяется**: добавляется опциональное поле `Announce *ProviderAnnounce`. Если провайдер отдаёт 403/410/etc + announce headers (типа «region blocked → @bot») — мы их парсим и кладём сюда, UI рисует тот же actionable диалог что и для `FetchAnnounceError`. По дефолту nil — поведение прежнее.
- `FetchAnnounceError` (200 + empty body + announce headers) — добавлен в commit `65e8c02` (предшествует этой SPEC'е). Доступен через `errors.As` и helper `IsAnnounceError(err)`. Message формата:
  `<ProfileTitle>: <Message>  →  <URL>`
- Plain `"empty subscription body"` — fallback когда нет ни одного announce-header.

### Cache invalidation на UA change

После Phase 2 UA меняется → провайдеры начнут отвечать contentful body для тех ссылок, которые до этого возвращали 0 байт. Но `bin/subscriptions/<id>.raw` cache хранит stale 0-byte результаты — `RebuildConfigIfDirty` без force-update прочитает пустой кэш и эмитнет config без proxy nodes.

Два варианта:
1. **Не кэшировать 0-byte ответы** (defensive). В `state.WriteRawBody` (или sibling в `core/`) — skip write если `len(raw) == 0`. Действует постоянно, не требует one-shot миграции. **Выбираем этот вариант.**
2. ~~Bump `RawCacheSchemaVersion` чтобы инвалидировать всё~~ — overkill, выбрасывает валидные кэши других подписок.

Расширить тест в `core/refresh_meta_test.go`: empty body не должен оставить файл в `bin/subscriptions/`.

### Announce-Url scheme allowlist

Phase 3 UI рендерит `Announce-Url` как кликабельную кнопку. Provider может прислать `javascript:alert()` / `file:///etc/passwd` / `data:...` — для UX-safety валидируем перед показом:

- `http://` / `https://` — допустимы
- `tg://` — допустимы (Telegram deep links, в дикой природе встречаются у NashVPN-class провайдеров)
- Всё остальное — кнопка hidden, URL отображается plain-text без `fyne.OpenURL`-handler'а

Реализация: `internal/urlsafe.IsSafeAnnounceURL(string) bool` (новый helper), вызывается в UI dialog'е.

---

## Тестирование

- **Unit** — `meta_test.go` + `fetcher_meta_test.go` extend'ятся: matrix HWID-флагов, inline-варианты, malformed base64 не паникует, plain-text fallback.
- **Integration / golden** — httptest.Server'ы для трёх сценариев:
  - V2Board-like (header-set §0-§2 + 200 + body)
  - NashVPN-like (HWID-limit empty)
  - Remnawave-like (HWID-Active + HWID-Not-Supported + 200 + body) — наш `X-Hwid` в запросе после Phase 2 → провайдер не должен слать `X-Hwid-Not-Supported`
- **Manual smoke** — реальный NashVPN URL через тестовую учётку. После Phase 2 → подписка отдаётся. До Phase 2 → `FetchAnnounceError` с правильным сообщением.

---

## Migration / совместимость

- **Response-side parsing** — pure additive. Старые state.json без `ProviderAnnounce` поля грузятся как nil (omit). Provider'ы не шлющие новые headers — поведение прежнее.
- **Request-side UA смена** — может изменить ответ от агрессивно-фильтрующих провайдеров (как раз и было целью). Provider'ы, чувствительные к UA, начнут отвечать корректно. Не наблюдалось ни одного случая когда смена UA с `SubscriptionParserClient` на `singbox-launcher/<v> (<os> <arch>)` ломает рабочую подписку.
- **HWID send opt-out** — checkbox default `true`. Юзеры, которые не хотят, выключат. Нет пути сделать «не зарегистрироваться» на HWID-binding панели и при этом её использовать — провайдер требует identification.

---

## Resolved decisions (исторически были Open)

1. **`X-Hwid` стабильность через переустановку лаунчера.** Random UUIDv4 — финальное решение. Альтернатива — derive из system machine-id (`ioreg` на macOS / `/etc/machine-id` на Linux / registry на Windows) — отклонена:
   - При clean install на **новой** машине юзер ожидает **новый** device slot → random UUID правильно мапится.
   - Единственный pain point — re-install лаунчера на ту же машину → новый slot. Mitigation: Phase 4 даёт editable UUID в Settings tab — юзер вручную копирует со старой инсталляции (бэкап settings.json) и вставляет в новую. Не нужно прокидывать platform-specific machine-id reading.
   - Privacy bonus: не derive'им из системных идентификаторов = не leak'аем hardware fingerprint провайдеру (он видит только наш random UUID + явные `X-Device-OS`/`X-Device-Model`).

2. **UA strategy.** `singbox-launcher/<v> (<os> <arch>)` (Phase 2 §1) — финальное. Альтернативы (mimic `sing-box/<v>`, статический `SingBoxLauncher`) отклонены: первая нечестна и ломается если sing-box-mobile меняет fingerprint, вторая теряет version/OS info которая полезна для targeted Min-Version checks на стороне провайдера.

3. **Phase ordering.** Реализовано Phase 2 → Phase 1 → Phase 3 → Phase 4 (request side первая — иначе HWID-binding провайдеры остаются на 0-байт ответах и парсить нечего). Таблицы выше в порядке логической группировки, не порядка имплементации.

## Considered but rejected

- **`Profile-Sub-User`** (echo per-user identification из request) — отклонено, никто из реальных панелей в дикой природе не шлёт.
- **`X-Premium`, `X-Plan`** (V2Board fork-extensions) — отклонено, нет канонического определения, разные форки шлют разные значения. Если понадобится — отдельный SPEC.
- **`Update-Always` response header** (видели у NashVPN) — отклонено, дублирует семантику `Profile-Update-Interval: 1` (force update on every render). Игнорим без warning'а.
- **Inline parsing HWID-флагов** (§3-§4 inline parity) — отклонено, имеет смысл только когда body не пустой, а статические хостинги (где inline актуален) по определению не имеют HWID-binding (нет server-side counter'а).
- **Negotiation cycle** (первый fetch без X-Hwid → если провайдер шлёт `X-Hwid-Not-Supported` → retry с X-Hwid) — отклонено: двойной round-trip, юзер ждёт. Просто шлём X-Hwid всегда (no-op для non-HWID провайдеров).
- **Длинные multi-line announce inline** — отклонено, провайдер должен кодировать в base64 single-line (XTLS draft рекомендует).
- **Per-header opt-out** (выключать только `X-Device-Model` оставив `X-Hwid`) — отклонено, чрезмерная granularity. `subscription_send_hwid: false` = всё-или-ничего.

## Open questions

(Пусто на момент landing'а. Если возникнут в реализации — добавлять сюда + SPEC bump в `-R` revision.)
