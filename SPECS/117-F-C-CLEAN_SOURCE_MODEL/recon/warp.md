# Recon: WARP — State.WarpAccounts ↔ источники/узлы (аудит против SPEC 117 DRAFT)

Дата: 2026-08-29. Ветка develop. Модель: SPECS/117-F-N-CLEAN_SOURCE_MODEL/DRAFT.md.

## 1. Что есть в коде

### 1.1 Хранилище: State.WarpAccounts

- `core/state/state.go:143` — `WarpAccounts *WarpAccountsSection`, комментарий: кеш выданных
  Cloudflare-регистраций; повторный «Add WARP» переиспользует запись; галочка «создать новые
  ключи» сбрасывает (фактически перезаписывает) запись.
- `core/state/disk_v6.go:58` — на диске корневой ключ `"warp_accounts,omitempty"` в `diskStateV6`.
- `core/state/disk_v6.go:73-78` — `WarpAccountsSection{ WG *WarpWGAccount; Masque *WarpMasqueAccount }`
  — **ровно по одному слоту** на транспорт (синглтоны). WG и MASQUE раздельно из-за разных
  типов ключей (X25519 vs ECDSA P-256).
- `core/state/disk_v6.go:83-95` — `WarpWGAccount`: private_key, peer_public, client_v4/v6,
  client_id, device_id, token, account_id, license, warp_plus, created_at.
- `core/state/disk_v6.go:101-111` — `WarpMasqueAccount`: private_key_der, server_pub_der,
  client_v4/v6, server, port, device_id, token, created_at.
  Комментарий (99-100): network/sni/таймауты сюда НЕ входят — «это параметры ноды, а не
  регистрации» (H2 и H3 строятся из одной записи).
- Save/Load: `core/state/save.go:121`, `core/state/load_v6.go:37,63` — секция едет как есть.
- В wizard-модель зеркалится целиком: `ui/configurator/models/wizard_model.go:85-88`,
  load: `presentation/presenter_state_helpers.go:39`, save-back: `presentation/presenter_state.go:107`.

### 1.2 Регистрация: пакет core/warp

- `core/warp/account.go:33-47` — рантайм-`Account` = регистрация + параметры узла
  (Endpoint, AWG map, License). Ключ генерится на устройстве, приватный ключ никуда не
  уходит (комментарий 1-9).
- `core/warp/cache.go:21-62` — WGToCache/WGFromCache: в кеш **сознательно не попадают**
  Endpoint и AWG («параметры ноды: пресет обфускации, выбор эндпоинта, кубик — их задаёт UI
  при каждой сборке»). Аналогично MasqueToCache/FromCache (64-102): network/SNI/таймауты
  проставляет вызывающий.
- `core/warp/account.go:74-122` — `ToWireguardURI(includeReserved)`: собирает
  `wireguard://<privkey>@host:port?publickey=&address=&allowedips=&mtu=1280[&reserved=b,b,b][AWG-поля]#<DisplayTag>`.
  Т.е. **весь материал аккаунта, включая приватный ключ, зашивается в URI**.
- `core/warp/masque.go:70-115` — `ToMasqueURI()`:
  `masque://<privDER>@server:port?publickey=&address=&profile=cloudflare&vhttp=&mtu=[&sni=...]#🔥🎭 WARP (MASQUE)`.
- Теги фиксированные: `account.go:58-67` DisplayTag = "🔥☁️ WARP" / "🔥⛈️ WARP (AWG)" (+"+"),
  `masque.go:41` = константа "🔥🎭 WARP (MASQUE)".

### 1.3 «Add WARP»: как регистрация становится источником

Цепочка:
1. `ui/configurator/tabs/source_tab.go:183-185` — кнопка вызывает
   `ShowAddWarpDialog(presenter, applyAddedSources)`.
2. `ui/configurator/dialogs/warp_dialog.go:322-356` (`runWarpRegistration`) и 358-385
   (`runMasqueRegistration`):
   - `cachedWG`/`cachedMasque` (389-410) — читают `presenter.Model().WarpAccounts`;
     промах или галочка «Create new keys» → `warp.Client.Register` в Cloudflare →
     `storeWG`/`storeMasque` (414-441) кладут снимок в модель + `MarkAsChanged()`
     (на диск уедет с обычным Save визарда).
   - Попадание в кеш → `ApplyNodeOptions` накатывает НА рантайм-объект узловые параметры
     из формы (endpoint/AWG либо vhttp/sni) — регистрация одна, узлы разные.
   - Затем `ToWireguardURI`/`ToMasqueURI` → готовый URI → `onURI`.
3. `source_tab.go:93-110` `applyAddedSources` → `wizardbusiness.AppendURLsToSources`.
4. `ui/configurator/business/sources.go:101-123` — URI классифицируется как direct link и
   становится **обычным** `corestate.Source{Type: server, URI: <uri>, NodeTag: <#fragment>}`
   (фрагмент = DisplayTag с эмодзи). Дедуп — по точному совпадению URI (61-72, 102).

Итог: WARP-узел в состоянии — **неотличим от вручную вставленной wireguard://masque:// ссылки**.
Никакого поля «это warp», никакого указателя на аккаунт. Дальше его парсят штатные
`node_parser_wireguard.go` / `node_parser_masque.go` на каждой сборке.

### 1.4 Связь узел ↔ аккаунт

Связи НЕТ ни в одном направлении:
- В `Source` (`core/state/connections.go:76-183`) нет ни warp-полей, ни ссылки на регистрацию.
- В `WarpAccountsSection` нет списка порождённых узлов.
- Связь односторонняя и одноразовая: аккаунт → URI **в момент создания**. После этого узел
  самодостаточен (приватный ключ, peer public, адреса, reserved, AWG-параметры — всё в URI).

Следствия (текущее поведение):
- **Удаление узла** ничего не делает с кешем: grep по `WarpAccounts.* = nil` — ноль вхождений
  вне диалога; сброс возможен только перезаписью через галочку «Create new keys»
  (`warp_dialog.go:337,364,390,402`). Device-запись в Cloudflare тоже никогда не удаляется
  (нет вызова DELETE в `core/warp/client.go`). Это осознанно: кеш и существует чтобы
  переиспользовать регистрацию и «не плодить device-записи» (`disk_v6.go:63-67`).
- **«Create new keys»** перезаписывает слот, но старые узлы продолжают жить со старым ключом
  из своего URI — работают, пока Cloudflare держит старую device-запись; модель этого никак
  не отслеживает.
- **Перенос узла** (между папками — когда появятся, между машинами копипастой URI) работает:
  узел самодостаточен. Перенос **аккаунта** — только через бэкап: `core/backup/export.go:104-107,
  174-198` (`warp[]` с дискриминатором type=wg/masque), импорт `core/backup/import.go:565-593`.
  Комментарий export.go:104-106 фиксирует мотив: без переноса регистрации «Add WARP» на новой
  машине плодил лишние device-записи.

### 1.5 Где живут секреты

Приватный ключ WARP лежит в **трёх** местах state-мира: (1) внутри `Source.URI`;
(2) в `warp_accounts` (плюс Token — bearer к аккаунту CF); (3) в бэкапе `warp[]`.
`disk_v6.go:81-82` прямо говорит: URI уже несёт секрет, секция «не добавляет новый класс
данных на диск». Совпадает с памятью «Секреты в state — by design».

## 2. Проекция на модель DRAFT

Как WARP-узел ложится на новую модель БЕЗ изменений семантики:
`Server{ tag, origin: {kind: uri, raw: <wireguard://…| masque://…>, subUrl: null}, body: parse(raw) }`,
`warp_accounts` остаётся отдельной корневой секцией (DRAFT так и перечисляет корень:
`sources[] / … / warp_accounts / meta` — строка 114 DRAFT). Материализация body (решение
«узлы подписки материализуются») закрывает сегодняшний перепарс URI на каждой сборке.

## 3. Конфликты и пробелы модели

### К1. `origin.kind = warp` не имеет носителя в коде и не определён по существу
DRAFT:15 — `OriginKind kind; // uri | wgIni | json | warp — чем парсить raw`. Но:
- Сегодня продукт «Add WARP» — обычный share-URI, парсуемый веткой `kind=uri`
  (`node_parser_core.go:31,95`). Отдельного «warp-формата raw» не существует нигде в коде.
- Если `kind=warp` задуман как «raw = снимок регистрации», то негде хранить УЗЛОВЫЕ
  параметры (endpoint, jc/jmin/jmax, masquerade id/ip/ib, vhttp, sni, idle/keep-alive) —
  код принципиально держит их ВНЕ регистрации (`cache.go:13-15,43-44,85-86`), они
  зашиваются в URI при создании. Regen (В1: `parse(raw)` пересоздаёт body) для такого raw
  не определён — парсера нет, и из одной регистрации законно происходят РАЗНЫЕ узлы
  (H2/H3, разные пресеты) — «raw аккаунта» не детерминирует body.
- Вариант «kind=warp, raw = сгенерированный URI» = чистая диагностическая метка при
  парсере uri; тогда надо явно сказать, что parse(kind=warp) == parse(kind=uri), иначе
  каждый switch по kind получит мёртвую/дублирующую ветку (ловушка «эмиттер и парсер
  ходят парой»).
Ответ на вопрос ТЗ «достаточно ли origin.kind=warp + body»: body — да, достаточно
(узел самодостаточен уже сегодня); отдельный kind — не обоснован кодом, uri покрывает.

### К2. Фиксированные теги WARP ломают инвариант уникальности корневого неймспейса
DRAFT:122-123 — тег узла «обязан быть уникален в корневом неймспейсе». Генератор ставит
константные DisplayTag (`masque.go:41`, `account.go:58-67`): добавление H2 и H3 подряд
(заявленный сценарий, ради него кеш и сделан) даёт два Source с одинаковым NodeTag
«🔥🎭 WARP (MASQUE)» и разными URI — дедуп по URI (`sources.go:102`) их пропускает оба.
Сегодня это «работает» (тег не единственный ключ), в новой модели это коллизия, а
merge-правила В2 «по (subUrl, tag)» такие узлы посчитали бы одним. Нужна уникализация
тега на Add (суффикс) или смена DisplayTag на параметризованный (в т.ч. h2/h3).

### К3. Судьба аккаунта при удалении узла — пробел, унаследованный моделью
Модель не вводит связи узел↔аккаунт (и код её не имеет). Значит: удаление последнего
WARP-узла оставляет в state приватный ключ + bearer-token навсегда; «удалить регистрацию»
как операции не существует (ни в UI, ни в client.go). Для DRAFT это стоит зафиксировать
явно как решение («кеш живёт независимо от узлов, чистится только перезаписью»), иначе
при рефакторинге секцию легко посчитать сиротой и «почистить» — что сломает мотив кеша
(не плодить device-записи).

### К4. Однослотность warp_accounts против «переноса между машинами»
Импорт бэкапа (`import.go:583,592`) слепо перезаписывает слот. Сценарий: на машине B своя
WARP-регистрация + свои узлы; импорт бэкапа с машины A затирает регистрацию B, при этом
узлы B (с ключами B в URI) остаются. Следующий «Add WARP» на B пойдёт от аккаунта A.
Ничего не падает (узлы самодостаточны), но инвариант «узлы и кеш из одной регистрации»
молча рвётся. Модель наследует это как есть — упомянуть ценой, не чинить.

### К5. Мелочь: комментарий Source.DetourTag «Not applied to WireGuard nodes»
(`connections.go:146`) — WARP-узлы суть wireguard/masque; в новой модели `Node.detour`
объявлен у любого узла. При переносе на NodeLink надо либо сохранить исключение явно,
либо снять его — сейчас оно живёт одной строкой комментария.

## 4. Ответы на вопросы ТЗ (сводно)

- **origin.kind=warp + body достаточно?** Body — да. kind=warp — лишний либо чисто
  диагностический (см. К1); функционально хватает kind=uri.
- **Где живёт приватный ключ/reserved?** В URI узла (и будущем origin.raw/body), плюс
  копия регистрации в warp_accounts, плюс бэкап warp[]. Reserved выводится из client_id
  (`account.go:52-54`), в URI попадает по галочке (`warp_dialog.go:252`).
- **Перенос узла между папками/машинами?** Безопасен — узел самодостаточен; аккаунт
  переносится отдельно бэкапом (К4 — затирание слота).
- **Удаление узла?** Кеш и CF-device живут дальше; никакого GC (К3).
