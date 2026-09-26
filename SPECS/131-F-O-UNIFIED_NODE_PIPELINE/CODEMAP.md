# CODEMAP — 131-F-O-UNIFIED_NODE_PIPELINE (единый конвейер добавления узла)

Карта построена по `develop` на 17.09.2026 (HEAD `d0e5aca3`, дерево чистое).
Адреса `файл:строка` проверены `rg -n`/`sed -n` на этом коммите; правка файла
выше по тексту сдвигает всё, что ниже.

Цель кампании — один конвейер для ВСЕХ входов:

```
текст → парсер (тупой маппер в карту sing-box)
      → санитайзер (правила из contract/registry)
      → тупой эмиттер
      → тело в state + warnings[] (код + путь) в записи узла
      → ⚠ на строке узла в UI, причина в карточке, ссылка на якорь документации
```

Сегодня из этой цепочки собраны три звена — и собраны **по три раза каждое**:
правила «что делать с мусором» живут в URI-парсерах, в JSON-санитайзере и в
allowlist-эмиттере. Карта показывает, где именно.

**Главный вывод разведки.** Поле `configtypes.ParsedNode.Warnings` **уже
существует** и уже заполняется парсерами (30 мест `AddWarning`), словарь кодов
уже нормативен (`contract/registry/warnings.json`, 61 код), конверт корпуса уже
несёт `warnings[]` у узла. Но в `state.Node` поля `warnings` НЕТ, и обе функции
материализации тела молча его теряют. Конвейер разорван ровно в одном шве —
см. §5.1.

---

## 0. Три копии правил — что с чем расходится

| Копия | Где живёт | Что умеет | Чего не умеет |
|---|---|---|---|
| URI-парсеры | `core/config/subscription/node_parser_*.go`, `shareuri_*.go` | ставит коды через `AddWarning` | работает только на URI-входе |
| JSON-санитайзер | `core/config/subscription/singbox_sanitize.go:63` | правит готовую map | возвращает коды лишь для 2 деградаций из 8 |
| Allowlist-эмиттер | `core/config/outbound_generator.go:313`, `outbound_jsonbuilder.go:84`, `outbound_tls_emit.go:41` | режет неизвестное | кодов не ставит **вообще**, только `debuglog` |

Валидаторы значений REALITY уже общие у первых двух копий — это готовый
нуклеус будущего реестрового санитайзера:

| Валидатор | Адрес | Кто зовёт |
|---|---|---|
| `normalizeRealityShortID` | `core/config/subscription/node_parser_transport.go:782` | URI-путь и `singbox_sanitize.go:214` |
| `isValidRealityPublicKey` | `core/config/subscription/node_parser_transport.go:812` | URI-путь и `singbox_sanitize.go:205` |
| `NormalizeRealityKeyShare` | `core/config/subscription/node_parser_transport.go:842` | URI-путь и `singbox_sanitize.go:222` |
| `noteRealityFingerprint` | `core/config/subscription/node_parser_transport.go:1126` | `singbox_import.go:377` |

---

## 1. Точки входа: где текст становится записью узла

### 1.1 Две воронки материализации — и обе теряют warnings

Весь репозиторий сходится в две функции. Это хорошая новость кампании: шов,
куда вставлять `warnings`, всего один на каждую воронку.

| Воронка | Адрес | Обслуживает |
|---|---|---|
| `MaterializeServerNode(uri, configJSON)` | `core/config/migrate_materialize.go:341` | **все** одиночные входы UI + миграция + Regen |
| `MaterializeSubscriptionBody(subID, body, skip, capN)` | `core/config/fetch_materialize.go:51` | fetch подписки (все форматы) |

Обе объявлены единственными точками правды и снабжены комментарием, что вторая
реализация разъедется на первой же правке эмиттера
(`migrate_materialize.go:334-340`).

**Тип результата одиночной воронки — без поля для кодов:**

| Сущность | Адрес | Заметка |
|---|---|---|
| `type ServerNodeMaterial{Body, OriginKind, OriginRaw}` | `core/config/migrate_materialize.go:326-332` | **поля `Warnings` нет** — Ловушка Л1 |
| `materializeServerForMigration` | `core/config/migrate_materialize.go:174` | реальная работа |
| ветка `config_json` (в обход эмиттера!) | `:175-191` | см. Ловушку Л2 |
| ветка wg-quick `[Interface]` | `:198-200` → `materializeWGConfBlock` `:230` | |
| ветка share-URI | `:202-222` | `ParseNode` `:205`, `emitMigrationBody` `:212` |
| `emitMigrationBody` | `core/config/migrate_materialize.go:261` | endpoint vs outbound, затем `stripTagAndDetour` |
| `stripTagAndDetour` | `core/config/migrate_materialize.go:283` | снимает `tag`/`detour`, порядок ключей сохраняет |

**Тип результата подписочной воронки — warnings есть, но не про узел:**

| Сущность | Адрес | Заметка |
|---|---|---|
| `type SubscriptionFetchMaterial{Nodes, …, Warnings []string}` | `core/config/fetch_materialize.go:31`, `:41` | `Warnings` — **per-body**, плоские строки, не коды |
| `canonicalNodeFromEntry(subID, e)` | `core/config/migrate_materialize.go:125` | **здесь теряются `e.Node.Warnings`** — Ловушка Л1 |
| ветка группы | `:131-155` | `state.Node{Kind: Auto}` |
| ветка сервера | `:156-166` | `state.Node{Kind: Server, Body: bodyJSON}` — кодов не переносит |
| `unsupportedNodeFromRecord` | `core/config/fetch_materialize.go:149` | отбракованная запись → `kind=unsupported` + `Reason` |

### 1.2 Вход: диалог Add (вставка текста)

| Слой | Функция | Адрес |
|---|---|---|
| Форма Add server | `(*addServerForm).refreshJSON` | `ui/configurator/dialogs/add_server_dialog.go:469` |
| Строка результата разбора (превью) | `(*addServerForm).previewJSON` | `ui/configurator/dialogs/add_server_dialog.go:481` |
| Разбор входа (ошибки глотаются) | `parseAddServerInput` | `ui/configurator/dialogs/add_server_dialog.go:547` |
| Вариант Tailscale | `previewTailscaleDocument` / `tailscaleDocument` | `add_server_dialog.go:528`; `add_server_tailscale.go:158` |
| Запись узлов в state (URI-ветка) | `AddSources` | `ui/configurator/business/source_input.go:133` (материализация), `:138-145` (`state.Node{…}`) |
| Запись узлов в state (JSON-ветка) | тот же | `source_input.go:158` (материализация), `:162-173` (`state.Node{…}` + `Sections`) |
| Отчёт после добавления | `reportAddSourcesResult` | `ui/configurator/tabs/source_add_report.go:25` |

Цепочка: `refreshJSON` → `parseAddServerInput` → `AddSources` →
`config.MaterializeServerNode` → `materializeServerForMigration` →
`ParseNode`/`NodeFromManualConfigJSON` → `emitMigrationBody` →
`GenerateNodeJSONBare` → `stripTagAndDetour` → `state.Node.Body`.

### 1.3 Вход: fetch подписки

| Слой | Функция | Адрес |
|---|---|---|
| HTTP-забор | `FetchSubscriptionWithMeta` | `core/config/subscription/fetcher.go:266` |
| Декод (base64 и пр.) | `DecodeSubscriptionContent` | `core/config/subscription/decoder.go:21` |
| Классификация формата | `ClassifySubscriptionBody` | `core/config/subscription/body_classify.go:90` |
| Разбор тела | `ParseSubscriptionBody` | `core/config/subscription/parse_body.go:146` |
| Импорт sing-box JSON | `ParseSingboxBody` | `core/config/subscription/singbox_import.go:68` |
| Материализация в узлы | `MaterializeSubscriptionBody` | `core/config/fetch_materialize.go:51` |
| Запись в state + статус | `config_service_subscriptions.go` | `core/config_service_subscriptions.go:164`, warnings `:201-203`, `:274-276` |

**Форматы тела** (`BodyKind`, `body_classify.go:14-41`) — порядок проверок
зафиксирован SPEC 094 A1 и значим:

| Формат | Константа | Заметка |
|---|---|---|
| список share-URI | `BodyKindURIList` | дефолт: всё нераспознанное |
| base64 | снимается декодером до классификации | `decoder.go:21` |
| Xray-массив | `BodyKindXrayArray` | проверяется раньше sing-box-массивов |
| одиночный sing-box outbound | `BodyKindSingboxOutbound` | `type` раньше `outbounds` — иначе селектор станет конфигом |
| массив outbound'ов | `BodyKindSingboxOutboundArray` | |
| целый sing-box конфиг | `BodyKindSingboxConfig` | |
| массив конфигов | `BodyKindSingboxConfigArray` | |
| wg-quick INI | `BodyKindWGConf` | `looksLikeWGConf` `:131` |
| Amnezia `vpn://` | `BodyKindVPNLink` | проверяется ПЕРВОЙ (`:` не в base64-алфавите) |
| **Clash YAML** | **не существует** | код `clash_yaml_unsupported` объявлен в реестре, `go: null` — Ловушка Л16 |

### 1.4 Вход: Regen / пересборка из origin.Raw

| Слой | Функция | Адрес |
|---|---|---|
| Regen из сохранённого origin | `regenServerBodyFromRaw` | `ui/configurator/tabs/source_body_edit.go:129` (→ `:133`) |
| Regen из явного текста | `regenServerBodyFromRawText` | `ui/configurator/tabs/source_body_edit.go:147`; материализация `:159` (JSON) / `:161` (URI) |
| Правка тела руками | тот же файл | `source_body_edit.go:84` |
| Достройка тела при вставке записи | `source_record_paste.go` | `ui/configurator/business/source_record_paste.go:150` |
| Вставка целого JSON источников | `sources_json.go` | `ui/configurator/business/sources_json.go:184` |
| Превью-окно правки узла | `preview_node_edit_window.go` | `ui/configurator/tabs/preview_node_edit_window.go:37` |

Regen читает `origin.Raw` и **заново разбирает** — значит warnings обязаны
пересчитываться, а не переноситься (Ловушка Л5).

### 1.5 Прочие входы

| Вход | Адрес | Как попадает тело |
|---|---|---|
| Ручной одиночный JSON | `NodeFromManualConfigJSON` `core/config/subscription/manual_config.go:34` | только валидирует `type`; **санитайзер не зовёт** |
| Эмиссия «как есть» | `ParsedNode.EmitRaw` `core/config/configtypes/types.go:726`; `generateRawNodeJSON` `core/config/outbound_generator.go:1684` | обходит per-scheme switch |
| Готовое тело канона | `ParsedNode.EmitBody` `core/config/configtypes/types.go:738`; `generateCanonicalBodyJSON` `outbound_generator.go:1728` | обходит и `EmitRaw`, и switch |
| Цепочка (тело настроек) | `ui/configurator/tabs/source_chain_tab.go:891` (`ChainBody`); эмиссия `core/config/chain_nodes.go:276` (`EmitRaw: true`) | позиции — отдельно, в `Hops` |
| Tailscale-форма | `tailscaleDocument` `ui/configurator/dialogs/add_server_tailscale.go:158` | собирает JSON вручную → JSON-ветка Add |
| Миграция state v6→v7 | `core/state/migration_v6_to_v7.go:272` (server), `:245` (chain), `:289` (подписка); хук `core/config/migrate_materialize.go:29` (`init`) | зовёт ту же воронку |
| Masque-фикс контракта 0.8.0 | `core/state/masque_uri_migration.go:101` | `node.Body = res.Body` через ту же воронку |
| **Релеи BYPASS (Xray `dialerProxy`)** | `relayNodesFromEntry` `core/config/relay_materialize.go:129` | `Service: true`, тело собирается отдельно от воронки |
| **AWG-форма обфускации** | `applyAWGSettings` `ui/configurator/tabs/source_awg_edit.go:239`, запись `:273`; сброс `:289`, запись `:309` | **правит ключи в готовом теле**, не пересобирает — Ловушка Л23 |
| Вставка записи хранения | `normalizePastedNode` `ui/configurator/business/source_record_paste.go:139`, материализация `:155` | из `origin.Raw` |
| Перенос/копия узла | `cloneCanonicalNodeForMove` `ui/configurator/business/node_move.go:779`, `:786` | глубокая копия тела, без разбора |
| Заливка подписки в папку | `FillFolderFromSubscription` `ui/configurator/business/folder_fill_subscription.go:77` | берёт готовые `sub.Nodes`, второго разбора нет |

**Импорт бэкапа — два формата, разное поведение:**

| Формат | Адрес | Что делает с телом |
|---|---|---|
| 1.0 (`lx_backup: 2`) | `decode10Source` `core/backup/import10.go:116` | копирует `body` **как есть**, без разбора и материализации |
| 0.x, сервер с `config_json` | `importServer` `core/backup/legacy_read_0x.go:373` | `config_json` → `Body` напрямую + `Origin{Kind: json}` `:374` |
| 0.x, сервер только с `uri` | `core/backup/legacy_read_0x.go:379-384` | **`Body` остаётся пустым**, ставится только `Origin`; вид (`uri`/`wg_ini`) определяется формой текста |
| 0.x, цепочка | `importChainBody` `core/backup/legacy_read_0x.go:415` | |
| 0.x, подписка | `importSubscription` `core/backup/legacy_read_0x.go:213` | `nodes[]` не едут; отметки ждут первого fetch `:262` |
| Слияние в state | `applyDecoded` `core/backup/import.go:376`; `mergeSources` `:437` | |
| REST-вход | `handleBackupImport` `core/debugapi/backup_endpoints.go:254`, роут `:429` | **единственный пишущий узлы эндпоинт** |

**Не точки входа** (проверено): share из контекстного меню
(`core/config/outbound_share.go` — только чтение, `GetOutboundMapByTag` `:41`);
`NodeLink` (адресация, не тело); все `/state/*` — только `GET`
(`core/debugapi/server.go:245`, `:258`, `:262`); `LoadNodesFromSourceEx`
(`core/config/subscription/source_loader.go:246`) — мёртвый legacy-путь, зовётся
лишь из `core/integration_test.go`.

---

## 2. Парсеры и правила, зашитые в них

### 2.1 Диспетчер и точки входа по схемам

| Сущность | Адрес |
|---|---|
| `ParseNode(uri, skipFilters)` — общий вход | `core/config/subscription/node_parser_core.go:86` |
| `vpn://` (Amnezia) — раньше всех | `:90-92` → `parseAmneziaVPNLink` `node_parser_amnezia.go:50` |
| MASQUE / WARP | `:94-96` → `node_parser_masque.go` |
| wg-quick текст → URI | `ConvertWGConfText` `wgconf_text.go:64`; блоки `WGConfBlocksOf` `:151` |
| AmneziaWG 3.x | `core/config/subscription/awg3.go` |
| Xray-объект → sing-box map | `core/config/subscription/xray_outbound_convert.go` |
| Xray-массив конфигов | `core/config/subscription/xray_json_array.go` |
| Xray hysteria | `core/config/subscription/xray_hysteria.go:115` (зовёт санитайзер) |
| Xray протоколы | `core/config/subscription/xray_protocols.go:338` (зовёт санитайзер) |

Пер-схемные файлы: `node_parser_{anytls,http,hysteria,hysteria2,masque,naive,
ss,ssh,tuic,vmess,wireguard,amnezia,transport}.go`, обратное направление —
`shareuri_{anytls,http,hysteria,hysteria2,masque,naive,socks,ss,ssh,trojan,
tuic,vless,vmess,wireguard}.go`.

### 2.1.1 Точка разбора по каждой схеме

| Схема | Функция разбора | Адрес |
|---|---|---|
| диспетчер | `ParseNode`; switch схем `:121-331`; allowlist `IsDirectLink` `:19-43` | `node_parser_core.go:86` |
| vless | ветка `:151` → `buildOutbound` vless `:663-729` | `node_parser_core.go:660` |
| vmess | `parseVMessDecoded`; JSON `:176`; legacy `:92` | `node_parser_vmess.go:43` |
| trojan | ветка `:154` → `buildOutbound` trojan `:857-866` | `node_parser_core.go:154` |
| ss | SIP002 `:161-184`; legacy blob `:185-217`; outbound `:867-873` | `node_parser_core.go:157` |
| hysteria2 | `buildHysteria2Outbound`; TLS `:91` | `node_parser_hysteria2.go:20` |
| hysteria v1 | `buildHysteriaOutbound`; TLS `:125` | `node_parser_hysteria.go:26` |
| tuic | `buildTuicOutbound`; TLS `:102` | `node_parser_tuic.go:56` |
| anytls | `buildAnyTLSOutbound`; TLS `:51` | `node_parser_anytls.go:21` |
| naive | `buildNaiveOutbound`; headers `:81` | `node_parser_naive.go:122` |
| socks/socks5 | `buildOutbound` socks `:883-890` | `node_parser_core.go:280` |
| http | `parseHTTPProxyURI` — **своя точка входа**, минует общую ветку | `node_parser_http.go:42` |
| ssh | `buildSSHOutbound` | `node_parser_ssh.go:16` |
| masque | `parseMasqueURI` — диспатч ДО гейта длины | `node_parser_masque.go:28` |
| wireguard/awg | `parseWireGuardURI`; поля AWG2 `applyAWGFields` `:446` | `node_parser_wireguard.go:18` |
| awg3 | `applyAWG3Fields` `:216`; валидация-убийца `validateAWG3` `:309`; детект `hasAWG3Params` `:116` | `awg3.go:216` |
| `awg://base64(.conf)` | `parseWGConfBase64Link` → `wgConfToURI` → снова `parseWireGuardURI` `:261` | `wgconf_text.go:221` |
| `.conf` текстом | `ConvertWGConfText` `:64`; тело `WGConfBodyToConvertedBlocks` `:190`; блоки `ExtractWGConfBlocks` `:26` | `wgconf_text.go:64` |
| `vpn://` Amnezia | `parseAmneziaVPNLink`; много узлов `ParseAmneziaVPNLinkAll` `:252` | `node_parser_amnezia.go:50` |

### 2.2 Правила, зашитые в парсерах (сводная)

| Правило | Адрес | Что делает | Код |
|---|---|---|---|
| insecure-алиасы | `node_parser_transport.go:37-45` | `insecure`/`allowInsecure`/`allowinsecure`, значения `1/true/yes` | — (`tls_insecure` объявлен, **не ставится**) |
| insecure у TUIC | `node_parser_tuic.go` (`tuicQueryFlagTrue`) | + `allow_insecure` | — |
| insecure у MASQUE | `node_parser_masque.go` | + `skip_cert_verify` | — |
| uTLS fp — allowlist | `node_parser_transport.go:50-56` (`singboxUTLSFingerprints`), ставится `:889` | вне списка → поле снимается | `utls_fp_unknown` |
| REALITY sid | `normalizeRealityShortID` `:782`, ставится `:908` | нечётная длина/не-hex → снимается | `reality_short_id_invalid` |
| REALITY pbk | `isValidRealityPublicKey` `:812` | мусор → вся reality снимается до plain TLS | — (`reality_pbk_invalid` объявлен, **не ставится**) |
| REALITY fp вне chrome | `realityFingerprintRisky` / `noteRealityFingerprint` `:1126-1146`, также `:914`, `:1144` | отпечаток уходит как есть, узел предупреждает (D-119) | `reality_fp_not_chrome` |
| REALITY key_share | `NormalizeRealityKeyShare` `:842`, ставится `:925` | вне `hybrid`/`classical` → поле снимается | `reality_key_share_invalid` |
| ws early data `?ed=N` | `node_parser_core.go:652`, `:788`; `node_parser_transport.go:476` | хвост пути → `max_early_data` + `early_data_header_name` | `ws_early_data_converted` |
| ws ed у httpupgrade | `xray_outbound_convert.go:341-344` | хвост срезается, `ed` **отбрасывается** | — |
| flow vision vs transport | эмиттер `outbound_generator.go:663-671`; санитайзер `singbox_sanitize.go:244-270` | vision только на голом TLS | — (`vision_with_transport` объявлен, **не ставится**) |
| flow allowlist | `outbound_generator.go:667-671`; `singbox_sanitize.go:256-262` | только `""` и `xtls-rprx-vision` | — (`flow_deprecated` объявлен, **не ставится**) |
| packet_encoding | `node_parser_core.go:703`; `singbox_sanitize.go:271` | allowlist `""`/`xudp`/`packetaddr` | `packet_encoding_unknown` |
| hysteria2 порты/`mport` | `hysteria2_ports.go:17`, `:42`, `:67`, `:78`, `:143`, `:167`, `:175` | диапазоны → `server_ports[]` | — |
| hysteria2 obfs | `node_parser_hysteria2.go:43`, `:46` | тип вне allowlist; пароль пуст | `obfs_unknown`, `obfs_password_missing` |
| ss method | `node_parser_ss.go` (`isValidShadowsocksMethod`) | вне списка → **нода дропается** | `ss_method_invalid` (error) |
| ssh user | `node_parser_ssh.go:22` | пустой → дефолт | `ssh_user_default` |
| naive padding | `node_parser_core.go:435` | игнорируется | `naive_padding_ignored` |
| naive extra-headers | `node_parser_naive.go:142` | битая пара пропускается | `naive_extra_headers_invalid` |
| tuic congestion / udp relay | `node_parser_tuic.go:73`, `:83` | allowlist | `tuic_congestion_invalid`, `tuic_udp_relay_mode_invalid` |
| anytls min_idle | `node_parser_anytls.go:40` | невалидное → дефолт | `anytls_min_idle_invalid` |
| anytls reality | `node_parser_anytls.go:96`, `:104` | key_share / fp | `reality_key_share_invalid`, `reality_fp_not_chrome` |
| masque vhttp | `node_parser_masque.go:188` | вне `{h3,h2,auto}` → `h3` | `masque_vhttp_invalid` |
| AWG заголовки / MTU | `node_parser_wireguard.go:286`; `awg3.go` | MTU клампится до 1280 | `awg_header_invalid`, `awg_headers_overlap`, `awg3_*` |
| Amnezia контейнер | `node_parser_amnezia.go:87` | профиль с несколькими контейнерами | `amnezia_container_choice` |
| Tailscale из подписки | `parse_body.go:434` | | `tailscale_from_subscription` |
| Xray неизвестный транспорт | `xray_outbound_convert.go:363` (`default: return nil`) | транспорт теряется **молча** | — (реестр называет это РАЗРЫВом) |

### 2.3 Правила-«убийцы» — узел выбрасывается, а не деградирует

Отдельно от таблицы выше: здесь `ParsedNode` не рождается вовсе, и вешать код
не на что (PARSING_PRINCIPLES §4, severity `error`).

| Правило | Адрес |
|---|---|
| Порт вне 1..65535 — **не дефолтится**, узел дропается | `node_parser_core.go:396-403` |
| Длина URI > `MaxURILength = 8192` | `node_parser_core.go:47`, проверка `:100` |
| Фрагмент невалидный UTF-8 (непочинимый) | `node_parser_core.go:443-456` |
| ss: метод вне allowlist | `node_parser_core.go:176-179`, `:200-203`; таблица `node_parser_ss.go:6-21` |
| ss: нет метода или пароля | `node_parser_core.go:386-393` |
| vmess JSON: нет add/port/id | `node_parser_vmess.go:184-210` |
| WG: ключ не base64 ровно в 32 байта | `normalizeWGKey` `node_parser_wireguard.go:326-344` |
| AWG2: перекрытие h1–h4 | `awgHeaderOverlap` `node_parser_wireguard.go:527-560`, вызов `:230-232` |
| AWG3: битый `header_protection_key` | `normalizeAWG3HeaderKey` `awg3.go:172-204`, гейт `:309-318` |
| AWG3: при ключе заголовка s1–s4 < 12 | `awg3.go:321-327` |
| Xray: неподдержанный протокол — **типизированная** ошибка («не поддержан» ≠ «битая подписка») | `xray_protocols.go:63-79` |

### 2.4 Правила-дефолты, меняющие смысл

| Правило | Адрес | Заметка |
|---|---|---|
| **`xtls-rprx-vision-udp443` → порт принудительно 443** | `node_parser_core.go:668-676`; Xray `xray_outbound_convert.go:165-172` | + `packet_encoding=xudp` |
| hysteria v1: полоса по умолчанию 100 Мбит | `node_parser_hysteria.go:67-70`, `:80`, `hysteriaBandwidthOrDefault` `:82` | без неё ядро валит ВЕСЬ конфиг |
| **MTU AWG клампится до 1280** | `awgMaxMTU` `node_parser_wireguard.go:401-408`; применение `:146-158` | явный меньший — уважается, больший — срезается |
| Детект AWG для клампа | `hasAWGParams` `node_parser_wireguard.go:423-432` (AWG3 через `:431`) | **устаревший комментарий** `awg3.go:112-115` утверждает обратное |
| Чистый WireGuard MTU не эмитит вовсе | `node_parser_wireguard.go:137-141`, `:218-220` | дефолт ядра 1408 |
| masque: MTU 1280 по умолчанию, **без клампа** | `node_parser_masque.go:110-115` | |
| WG: `allowedips` по умолчанию `0.0.0.0/0,::/0` | `node_parser_wireguard.go:99-111`; `.conf` `node_parser_amnezia.go:523-527` | |
| WG: порт по умолчанию 51820 | `node_parser_wireguard.go:83-88` | |
| Эвристика «порт без TLS» (80/8080/8880/2052/2082/2086/2095) | `plaintextVLESSPorts` `node_parser_transport.go:152-165`, применение `:932-934` | на экспорт **не** применяется (`shareuri_vless.go:10-26`) |
| `security=none` → ключа `tls` нет вовсе | vless `node_parser_transport.go:875-877`; trojan `:974-990` | `enabled:false` роняет ядро в SIGSEGV |
| REALITY требует uTLS; пустой/`random` fp → `chrome` | `EnforceRealityFingerprint` `node_parser_transport.go:1060-1103` | |
| anytls: пустой fp → `random` | `node_parser_anytls.go:72-78` | паритет D-009 |
| xhttp: `uplink_data_placement: header` без mode → `packet-up` | `xhttpGuardUplinkPlacement` `node_parser_transport.go:443-467` | `xhttp_mode_forced_packet_up` / `xhttp_param_reset` |
| xhttp: плоский query бьёт `extra` ровно для mode/path/host | `xhttpBuildTransport` `node_parser_transport.go:363-416`, трио `:376-386` | |
| hysteria2: восстановление multi-port authority | `hysteria2RecoverMultiPortAuthority` `hysteria2_ports.go:78-138`, хук `node_parser_core.go:342-347` | |
| Ярлык никогда не берётся из userinfo | `node_parser_core.go:458-472` | защита от утечки секрета |
| Пустой тег → `scheme-server-port` | `generateDefaultTag` `node_parser_core.go:577`, вызов `:480-486` | |

### 2.5 Реестр кодов и причин

Коды-константы: `core/config/subscription/parse_warnings.go:22-129` (27 штук).
Правило пакета (`:15-20`): код ставится там, где узел в области видимости;
нормализаторы остаются чистыми и отдают предикат «деградировало бы» —
`realityShortIDWouldDegrade` `:139`, `utlsFingerprintWouldDegrade` `:149`.

Причины отбраковки: `core/config/subscription/parse_failure_reasons.go` —
накопитель с дедупом и шапкой `MaxParseFailureReasons = 3` `:28`, обрезка **на
вставке** `:55-59` (иначе 500 одинаковых причин копились бы), флаг `truncated`
`:39`, хук `RecordParseFailures` `:96`. Тексты причин Xray-ветки —
`xray_protocols.go:31-38`.

---

## 3. Санитайзер `singbox_sanitize.go`

419 строк. Правит map на месте, `tag` — только для логов.

| Функция | Адрес | Что делает | Возвращает код |
|---|---|---|---|
| `SanitizeSingboxOutboundMap(ob, tag) []string` | `:63` | диспетчер, вызывает все ниже | агрегат |
| `sanitizeSingboxMasqueLegacy` | `:94` | стрипает плоские `network`/`sni`/`skip_cert_verify` | **нет** |
| `sanitizeSingboxTLS` | `:115` | uTLS allowlist, REALITY, снятие uTLS/REALITY на QUIC | прокидывает из reality |
| `sanitizeSingboxUTLS` | `:160` | fp вне allowlist | **нет** |
| `sanitizeSingboxReality` | `:186` | pbk `:205`, short_id `:214`, key_share `:222` | только `reality_key_share_invalid` `:230` |
| `sanitizeSingboxFlow` | `:244` | vision-allowlist + гашение при транспорте | **нет** |
| `sanitizeSingboxPacketEncoding` | `:271` | allowlist | `packet_encoding_unknown` (через bool) |
| `sanitizeSingboxHysteriaBandwidth` | `:298` | полоса | **нет** |
| `sanitizeSingboxHysteriaObfs` | `:343` | obfs v1 | **нет** |
| `sanitizeSingboxHysteria2Obfs` | `:372` | obfs v2 | **нет** |

Служебное: `singboxServiceTypes` `:23`, `singboxGroupTypes` `:28`,
`quicOutboundTypes` `:48`, `IsSingboxServiceType` `:33`, `IsSingboxGroupType`
`:38`, `mapString` `:406`.

**Из 10 санитайзов коды возвращают ДВА.** Остальные восемь деградаций уходят в
`debuglog` и не доходят ни до UI, ни до LxBox — это и есть половина работы
кампании.

### 3.1 Кто зовёт санитайзер

| Call site | Адрес | Вешает ли коды на узел |
|---|---|---|
| Импорт sing-box JSON | `core/config/subscription/singbox_import.go:355` | да, `:373-375` |
| Xray-протоколы | `core/config/subscription/xray_protocols.go:338` | да, `:350-352` |
| Xray hysteria | `core/config/subscription/xray_hysteria.go:115` | да, `:126-128` |

### 3.2 Кто НЕ зовёт, хотя мог бы

| Путь | Адрес | Последствие |
|---|---|---|
| `NodeFromManualConfigJSON` | `manual_config.go:34` | ручной JSON едет в тело **без единой проверки** |
| ветка `config_json` воронки | `migrate_materialize.go:175-191` | `stripTagAndDetour(req.ConfigJSON)` — вход = выход |
| `generateRawNodeJSON` (`EmitRaw`) | `outbound_generator.go:1684` | эмиссия без санитайза |
| `generateCanonicalBodyJSON` (`EmitBody`) | `outbound_generator.go:1728` | тело из state как есть |
| URI-парсеры | `node_parser_*.go` | имеют собственные копии тех же правил |

---

## 4. Эмиттер

### 4.1 Точки входа и три обхода switch

| Сущность | Адрес | Заметка |
|---|---|---|
| `GenerateNodeJSON(node)` | `core/config/outbound_generator.go:296` | + обёртка config.json |
| `wrapOutboundForConfig` | `:716` | таб, комментарий-имя, хвостовая запятая |
| `GenerateNodeJSONBare(node)` | `:313` | голый объект |
| **обход 1:** `EmitBody` непусто | `:324-326` → `generateCanonicalBodyJSON` `:1728` | приоритетнее всего |
| **обход 2:** `Scheme == SchemeGroup` | `:328-330` → `generateGroupNodeJSON` `:1624` | |
| **обход 3:** `EmitRaw` | `:335-337` → `generateRawNodeJSON` `:1684` | ручной JSON и цепочки |
| Endpoint-ветка | `GenerateEndpointJSON` `:1039`, `Bare` `:1053`, `generateEndpointJSONBare` `:1057` | wireguard/tailscale |
| Обе сразу | `EmitNodeJSONs` `:1088` | |
| Сборка конфига | `GenerateOutboundsFromParserConfig` `:1142` | гейты ядра `:1195-1213` |

### 4.2 Per-scheme switch — это цепочка `else if`, а не `switch`

`core/config/outbound_generator.go:346-638`. Схема без своей ветки молча
получает только общие поля (`tag`, `type`, `server`, `server_port`):

| Схема | Адрес ветки |
|---|---|
| `ss` (тип) | `:346` |
| `socks`/`socks5` (тип) | `:348` |
| `vless`/`vmess` | `:367` (vmess `:370`) |
| `trojan` | `:378` |
| `hysteria2` | `:380` |
| `hysteria` | `:432` |
| `ss` (поля) | `:468` |
| `socks` (поля) | `:486` |
| `naive` | `:504` |
| `http` | `:536` |
| `tuic` | `:568` |
| `masque` | `:594` |
| `anytls` | `:612` |
| `ssh` | `:623` |
| `vless` post-switch (`packet_encoding`, `encryption`) | `:675-687` |

Общий хвост: flow `:642-672`, транспорт `:689`, TLS `:692`, detour `:696-704`.

### 4.3 Аллоулисты внутри эмиттера

| Слой | Адрес | Чем режет |
|---|---|---|
| TLS | `emitOutboundTLSJSON` `core/config/outbound_tls_emit.go:41` | allowlist по `OutboundTLSOptions` ядра |
| alpn (толерантно) | `outbound_tls_emit.go:91` | `tolerantStringSlice` |
| Транспорт | `appendOutboundTransportParts` `core/config/outbound_jsonbuilder.go:84` | поле за полем, фиксированный порядок |
| ws early data | `outbound_jsonbuilder.go:104-108` | `max_early_data` + `early_data_header_name` |
| XHTTP v2 | `outbound_jsonbuilder.go:125-150` | `xhttpV2StringKeys`, `xhttpIntKeys` |
| xmux (объект) | `:152-172` | значения — как есть |
| headers | `:179` | `map[string]string` и `map[string]interface{}` |
| `transportInt` | `:212` | толерантное целое |
| Балансировщик | `sanitizeBalancerOptions` `outbound_generator.go:1002` | |

**Где теряются неизвестные поля:** везде, где список ключей перечислен
руками. Ключ, которого нет в списке ветки §4.2 или в аллоулисте §4.3, в
`config.json` не попадает и **никак не отмечается**.

### 4.4 Толерантные ассерты (и где их всё ещё нет)

| Хелпер | Адрес | Зачем |
|---|---|---|
| `tolerantInt` | `core/config/outbound_generator.go:2011` | `int` / `int64` / `float64` / `json.Number` |
| `tolerantStringSlice` | `core/config/outbound_generator.go:2035` | `[]string` / `[]interface{}`, элемент-число тоже |
| `obfsIntField` | `core/config/outbound_generator.go:1999` | обёртка над `tolerantInt` |
| `transportInt` | `core/config/outbound_jsonbuilder.go:212` | то же для транспорта |

Толерантно читаются: `server_ports` `:390`, `:443`; `up_mbps`/`down_mbps`
`:399`, `:403`, `:455`, `:458`; obfs `:423`; alpn `outbound_tls_emit.go:91`.

**Жёсткие ассерты остались** в ветках §4.2 — например ssh `.([]string)`
`:630`, anytls `.(int)` `:620`, большинство `.(string)`. Карта JSON несёт
`float64`/`[]interface{}` — Ловушка Л8.

### 4.5 Гейты по версии ядра — все

| Проба | Адрес объявления | Установка | Гранулярность |
|---|---|---|---|
| `NaiveSupportProbe` | `outbound_generator.go:259` | `core/controller.go:278` | узел выбрасывается `:1195` |
| `ChainSupportProbe` | `core/config/chain_generator.go:33` | `core/controller.go:279` | узел |
| `TailscaleSupportProbe` | `outbound_generator.go:265` | `core/controller.go:282` | узел `:1202` |
| `AWG3SupportProbe` | `outbound_generator.go:270` | `core/controller.go:285` | узел `:1209` |
| `RealityKeyShareSupportProbe` | `outbound_generator.go:280` | `core/controller.go:288` | **поле**, читает эмиттер через `coreSupportsRealityKeyShare` `:284` |

`nil`-хук = «ядро умеет» во всех пяти (деградировать по догадке нельзя).

---

## 5. Запись узла в state

Текущая версия схемы — **v8** (SPEC 127 уже влит).

| Сущность | Адрес |
|---|---|
| `SchemaVersion = SchemaVersionV8` | `core/state/state.go:43` |
| `SchemaVersionV8 = 8` | `core/state/disk_v8.go:31-32` |
| `SchemaNameV8 = "sources_v8"` | `core/state/disk_v8.go:34-36` |
| `SchemaMajor = SchemaVersionV8` | `core/state/schema_gate.go:28` |
| Миграция v7→v8 | `core/state/migration_v7_to_v8.go` |
| Роутер версий | `core/state/load_router.go:54` |

### 5.1 `type Node` — и отсутствующее поле

`core/state/sources_v7.go:199-270`:

| Поле | Адрес | Заметка |
|---|---|---|
| `Kind SourceKind` | `:200` | `server`/`chain`/`auto`/`folder`/`unsupported` |
| `Tag string` | `:205` | сырой тег = идентичность (SPEC 112) |
| `Enabled bool` | `:206` | |
| `Origin *Origin` | `:207` | |
| `Body json.RawMessage` | `:221` | готовый outbound без `tag`/`detour` |
| `Detour *NodeLink` | `:225` | |
| `Hops []NodeLink` | `:227` | chain only |
| `Group *AutoGroup` | `:229` | auto only |
| `Service bool` | `:247` | приехал служебной частью чужой записи |
| `Reason string` | `:252` | **unsupported only**, английский текст парсера |
| `Sections *NodeSections` | `:262` | server only |
| **`Warnings`** | **нет** | ← цель кампании |

`Origin` — `core/state/sources_v7.go:48-57`: `Kind` (`uri`/`wg_ini`/`json`,
константы `:59-63`), `Raw` (байт в байт), `SubURL`.

**Нужен ли бамп версии state под `warnings`?** Нет. Поле добавляется как
`json:"warnings,omitempty"`, `additionalProperties` у бэкапа открыт (контракт
0.11.0, принцип П3), а роутер `load_router.go` ловит `meta >= 8` в `parseV8`.
Старый файл просто не несёт ключа — это пустой список, а не потеря. Бампа
требовало бы переименование или смена смысла существующего ключа, чего здесь
нет. Ср. `Reason` `:252`, добавленный так же.

### 5.2 Warnings подписки сегодня

| Сущность | Адрес |
|---|---|
| `type FetchWarning{Kind, Tag, Message, Count}` | `core/state/sources_v7.go:409-419` |
| `SubUpdateStatus.Warnings []FetchWarning` | `core/state/sources_v7.go:435` |
| Наполнение (parse) | `core/config_service_subscriptions.go:201-203`, `:274-276` |
| Наполнение (merge) | `core/config_service_subscriptions.go:279` |
| Перенос прошлых | `core/config_service_subscriptions.go:251` |

Это **строки**, а не коды, и они **per-subscription**, а не per-node: поле
`Tag` есть, но заполняется не всегда. Форматирование для UI —
`fetchWarningTexts` `ui/configurator/tabs/source_meta_format.go:347`.
Единственное место показа — вкладка Overview окна источника,
`ui/configurator/tabs/source_edit_overview.go:212-218`. На строке источника и в
drill-down они **не видны** — это и есть дыра, которую закрывает ⚠.

---

## 6. UI

### 6.1 Где рисуются строки узлов

| Поверхность | Функция | Адрес |
|---|---|---|
| Вкладка Servers — шаблон строки | `createItem` в `CreateProxyListPanel` | `ui/clash_api_tab.go:669`; имя `:680-694`; задержка `:710-723`; сборка `:729-739` |
| Вкладка Servers — данные | `updateItem` | `ui/clash_api_tab.go:743`; имя `:774`; подпись `:781`; фон `:794-801` |
| Вкладка Servers — сам список | `widget.NewList` | `ui/clash_api_tab.go:882-886` |
| **Строка узла (единственный шаблон)** | `newSourceNodeRow(spec)` | `ui/configurator/tabs/source_node_row.go:91`; спец `:55` |
| Её вызовы (drill-down) | `folderDrillNodeRow` | `ui/configurator/tabs/source_folder_drilldown.go:581`, вызовы `:642`, `:645`, рендер `:880-889` |
| Шапка контейнера источника | IIFE в `CreateSourcesTab` | `ui/configurator/tabs/source_tab.go:647`; ярлык `:659-758`; кнопки `:951-985`; `titleRow` `:989`; подписи `:995-1053` |
| Список Preview в окне источника | inline `widget.NewList` | `ui/configurator/tabs/source_edit_window.go:1718`; createItem `:1720-1760`; updateItem `:1761-1855` |
| Модель строк Preview | `buildPreviewRows` | `ui/configurator/tabs/preview_rows.go:63` |
| Тексты строк Preview | `previewRowTitle` / `previewRowSubtitle` / `previewRowToolTip` | `ui/configurator/tabs/preview_row_view.go:29`, `:60`, `:87` |
| Карточка узла (Info) | `showNodeInfoWindow` | `ui/servers_node_info.go:38`; `infoRow` `:426`; `sectionHeader` `:403`; `memberRow` `:444` |
| Окно Core runtime | `coreRuntimeNodeRow` / `…WithStatus` | `ui/core_runtime_window.go:239`, `:246` |
| Подпись узла на Servers | `serversNodeSubtitle` | `ui/servers_node_subtitle.go:226` |

### 6.2 Уже существующие ⚠ — переиспользовать, не заводить новый

| Маркер | Адрес |
|---|---|
| **Константа `previewUnsupportedMark = "⚠"`** | `ui/configurator/tabs/preview_row_view.go:22` |
| `⚠ <reason>` в подписи строки Preview | `preview_row_view.go:61`; пустая группа `:73` |
| ⚠-подпись цветом `theme.ColorNameWarning` в шаблоне строки узла | `ui/configurator/tabs/source_node_row.go:148-152`; флаг `SubtitleWarn` `:61` |
| То же в списке Preview | `source_edit_window.go:1824` |
| `⚠ N node error(s)` в шапке drill-down | `source_folder_drilldown.go:478-484` |
| `⚠ Excluded from the config: %s` (detour fail-closed) | `source_tab.go:1011-1020`; причина `:766` (`config.ExcludedSourceReason`) |
| `⚠ No nodes from this source: %s` | `source_tab.go:1021-1029`; причина `:785` |
| `⚠ %d node(s) dropped: %s` | `source_tab.go:1030-1039`; данные `:777` |
| `⚠ ` + эмиссионные деградации источника | `source_tab.go:1045-1052`; данные `:800` (`config.EmitWarningsForSource`) |
| `⚠` у висячего члена группы в Info | `ui/servers_node_info.go:452` |
| Реестры-источники этих причин | `core/config/excluded_sources_registry.go:103`; `core/config/build_report.go:314`, `:339`, `:361` |

Порядок в шапке источника нормирован: исключение → parse-failed → dropped
(взаимоисключающие), затем эмиссионные warnings отдельными строками.
`Wrapping = fyne.TextWrapWord` обязателен (`source_tab.go:1012` — ловушка
min-width у Fyne).

**Для строки узла работы по вёрстке нет:** шаблон уже принимает
`sourceNodeRowSpec.Subtitle` + `SubtitleWarn: true` (`source_node_row.go:57-61`).

### 6.3 Открытие ссылки во внешнем браузере

`fyne.CurrentApp().OpenURL` в проекте **не используется**. Свой хелпер:

| Хелпер | Адрес |
|---|---|
| `platform.OpenURL(url) error` | `internal/platform/platform_darwin.go:29`; `platform_linux.go:32`; `platform_windows.go:33` |
| Гейт схем (`http`/`https`/`tg`) | `urlsafe.IsSafeAnnounceURL` `internal/urlsafe/url.go:28` |
| **Готовый образец кнопки-иконки со ссылкой** | `supportLinkButton` `ui/configurator/tabs/source_support_link.go:27`; применения `source_tab.go:961`, `source_folder_drilldown.go:504`, `source_preview_header.go:58` |
| Кнопка ссылки в диалоге ошибки | `buildURLAffordance` `ui/configurator/dialogs/source_error_dialog.go:213` |
| `widget.NewHyperlink` | `internal/dialogs/dialogs.go:124`; `ui/help_tab.go:85`, `:94`; `ui/core_dashboard_tab.go:525`, `:1047`, `:1154` |

Для ссылки «причина → якорь документации» копировать
`source_support_link.go:27-57`.

### 6.4 Диалог ошибок источника

| Сущность | Адрес |
|---|---|
| `ShowSourceErrorDialog` | `ui/configurator/dialogs/source_error_dialog.go:71` |
| Тело | `buildSourceErrorBody` `:105` |
| Кнопка-иконка ⚠/📢 на строке источника | `ui/configurator/tabs/source_tab.go:915-945` |
| Блок «почему узлы отбракованы» | `previewParseReasonsBlock` `ui/configurator/tabs/source_edit_window.go:69` |

---

## 7. Локализация

| Сущность | Адрес |
|---|---|
| Пакет (натуральные ключи, SPEC 111: **английский текст на месте вызова И ЕСТЬ ключ**) | `internal/locale/locale.go:1-7` |
| `T(key)` | `internal/locale/locale.go:119` |
| `TN(form, key)` — омонимы | `:126` |
| `Tf(key, args…)` = `fmt.Sprintf(T(key), args…)` | `:138` |
| `TfN` / `Plural` / `PluralN` | `:143`, `:150`, `:155` |
| Цепочка языков (`lang` → `en` → сам ключ) | `:168`; init `:49-54` |
| Формы записи каталога | `internal/locale/entry.go:7-24`; `Entry` `:21`; `Value` `:27`; `UnmarshalJSON` `:38` |
| Загрузка каталогов с диска | `LoadExternalLocales` `:255`; каталог `:384` |
| Файл ключей | `bin/locale/ru.json` (~1535 записей; ведётся только `ru`) |
| Карта позиций аргументов для чекера | `tools/l10n/display_positions.json` |
| Пометки отказа | `// l10n-exempt:`, `// l10n-key` |

**Пример ключа с подстановкой.** Вызов — `ui/configurator/tabs/source_tab.go:1016`:

```go
warn := widget.NewLabel(locale.Tf("⚠ Excluded from the config: %s", exclusionReason))
```

Запись в `bin/locale/ru.json`:

```json
"⚠ Excluded from the config: %s": { "value": "⚠ Исключён из конфига: %s" }
```

Два плейсхолдера (перевод переставляет предложение, порядок `%d`/`%s` держит) —
`source_tab.go:1034`:

```json
"⚠ %d node(s) dropped: %s": { "value": "⚠ снято узлов: %d — %s" }
```

Переводы новых ключей — в той же ветке (memory `translations-delegated`).

---

## 8. Документация и контракт

### 8.1 `docs/Protocols*`

| Файл | Строк | Язык |
|---|---|---|
| `docs/Protocols.md` | 762 | EN |
| `docs/Protocols.ru.md` | 734 | RU |

Оба — **рукописные**, маркеров генерации нет; держатся в EN/RU-локстепе
(заголовки на одних строках 1/12/127/141/192). Разделов `##` всего пять:
шапка `:1`, «Содержание» `:12`, «Документы и исходный код парсера» `:127`,
«Share URI из outbound» `:141`, «Форматы URI» `:192`. Таблица 14 схем —
`docs/Protocols.md:24-40`.

**Готовый якорь для ссылки из карточки узла** — раздел
`### Degradation codes on a node`, `docs/Protocols.md:85`
(`#degradation-codes-on-a-node`, в оглавлении `:17`). Он уже описывает
`ParsedNode.Warnings`, `AddWarning`, реестр и раскол info/warning vs error, и
уже ссылается на `registry_sync_test.go`. Русский близнец — тот же раздел в
`docs/Protocols.ru.md`.

### 8.2 `contract/registry/`

Десять файлов + `protocols/` из 16 схем.

| Файл | Корневые ключи |
|---|---|
| `protocols/<scheme>.json` | `v, scheme, kind, singbox_type, aliases, sources, extension, uri, emit, degrade, refs` |
| `tls.json` | `v, note, tls` |
| `transports.json` | `v, note, transports` |
| `warnings.json` | `v, warnings` |
| `allowlists.json` | `v, allowlists` |
| `limits.json` | `v, limits` |
| `backup_warnings.json`, `containers.json`, `presets.json`, `vars.json` | прочее |

Фрагмент `contract/registry/protocols/vless.json:1-25`:

```json
{
  "v": 1,
  "scheme": "vless",
  "kind": "outbound",
  "singbox_type": "vless",
  "aliases": [],
  "sources": ["uri", "singbox", "xray"],
  "extension": null,
  "uri": {
    "userinfo": { "meaning": "uuid", "note": "…" },
    "query": {
      "flow": {
        "type": "enum",
        "values": ["", "xtls-rprx-vision", "xtls-rprx-vision-udp443"],
        "allowlist": null,
        "default": "",
        "aliases": [],
        "note": "Ядро принимает ровно '' и 'xtls-rprx-vision'. …"
      }
    }
  }
}
```

Дескриптор параметра един для `protocols/*.json`, `tls.json`, `transports.json`:
`{type, values[], allowlist, default, aliases[], ext?, note}`. `allowlist`
называет запись в `allowlists.json`; `ext: "mobile"` — поле только LxBox;
`refs: {go[], dart[]}` — якоря `файл:строка` в обе кодовые базы; `degrade[]` —
правила деградации текстом.

`tls.json` — общая TLS-подсхема (`security`, `sni`, `alpn`, `insecure`, `fp`,
блок `reality` с `key_share`/`pbk`/`sid`), переиспользуемая всеми TLS-схемами.
`transports.json` — по имени транспорта → `{singbox_type, params{…}}`
(`ws` с `path`/`host`/`ed`/`eh`, `grpc`, `http`, `httpupgrade`, `quic`, `xhttp`).

`warnings.json` — **61 код** (`warning` 31, `error` 16, `info` 14), у каждого
ровно пять ключей:

```json
"transport_unsupported": {
  "severity": "warning",
  "params": ["transport", "fallback"],
  "desc": "Транспорт не поддержан ядром/билдом … РАЗРЫВ: Go в Xray-конверте молча возвращает nil …",
  "dart": "UnsupportedTransportWarning",
  "go": "core/config/subscription/xray_outbound_convert.go:337"
}
```

`params[]` — имена плейсхолдеров для локализации (локализация на стороне
приложений, PARSING_PRINCIPLES §6). Поле `go` уже несёт якорь `файл:строка` — это готовая
основа для ссылки «код → место в коде», параллельной ссылке на документацию.

`allowlists.json` — 7 наборов (`utls_fingerprints` 15 значений, `ss_methods`,
`tuic_congestion`, `tuic_udp_relay_mode`, `hysteria2_obfs`, `packet_encoding`,
`vless_flow`); `note` каждого называет символы обеих сторон для сверки.

`limits.json` — 9 лимитов в форме `{value, current:{go,dart}, note}`, то есть
канон и фактическое расхождение сторон данными, а не комментарием. Значимое
для кампании: `max_uri_length` канон 65536, **go 8192**;
`max_nodes_per_subscription` 3000; `max_detour_chain` 8.

### 8.3 `contract/docs/PARSING_PRINCIPLES.md`

70 строк, 7 разделов. Конверт — §1 `:5-16`:

```
Форма — `schema/node.schema.json`: `{v, nodes[], dropped[], meta?}`. Узел:
`{kind, scheme, label, entry, chain?, warnings?}`.
```

Warnings — §6 `:59-62`: коды из реестра, **в порядке появления при разборе**,
локализация на приложениях.

Деградация — §4 `:41-51`: битое **значение** → чинится/выкидывается с
warning-кодом, нода живёт; битая **нода** → в `dropped[{ref, reason}]`,
подписка живёт; значение, которое ядро отвергнет fatal'ом на весь конфиг, в
`entry` попадать **не имеет права**.

Сравнение — §7 `:66-70`: по значению после канонизации, не по байтам;
per-app override только со ссылкой на задокументированное различие.

### 8.4 `contract/README.md` и `VERSION`

`contract/VERSION` = **`1.0.4`**. Лог версий — двухколоночная таблица,
новые строки снизу, `contract/README.md:54-58` шапка, строки `0.1.0` `:58` →
`1.0.4` `:89`. Строка консистентно заканчивается перечнем добавленных кейсов
корпуса и номером встречной секции `TASKS_LXBOX`. Политика semver — в таблице
`Состав` `:13` (major — правила канонизации, minor — новые конструкции,
patch — уточнения).

**Устаревший адрес:** `contract/README.md:50` шлёт за раннерами лаунчера в
`core/config/subscription/contract_test.go` — такого файла нет (см. §8.7).

### 8.5 `contract/TASKS_LXBOX.md`

1717 строк. Формат секции — `## <N>. <заголовок> (приоритет <1-3>)`,
подсекции `### <N>.<M>`. Образец `:1601-1607`:

```
## 23. `tls.reality.key_share` (ядро lx.4, SPEC 089) — норма тела и вопрос про share-URI (приоритет 2)
```

**Последняя секция — `## 23`** (`:1601`), с подсекцией `### 23.1 Ответ
владельца 17.09.2026` `:1659`. Новая задача кампании — `## 24`.

### 8.6 `go:generate` и CI

**`go:generate` в репозитории нет ни одной** (проверено по `*.go`, `*.md`,
`*.yml`, `*.sh`). Реестр, документы и корпус ведутся руками, согласованность
держат тесты, а не генерация.

| Workflow | Джобы |
|---|---|
| `.github/workflows/ci.yml` | `meta` `:86`, `test` `:144`, `build-darwin` `:274`, `build-windows` `:353`, `build-win7` `:414`, `release` `:560` |
| `.github/workflows/golangci-lint.yml` | `golangci-lint` `:12` |
| `.github/workflows/claude.yml` | `claude` `:45` |

**Отдельной джобы «контракт против кода» нет**: `contract`/`registry`/`corpus`
не встречаются в workflow'ах ни разу. Сверка идёт только транзитом через
`go test ./...` (`ci.yml:204`, `:212`, `:220` → `build/test_linux.sh:23`,`:36`).

### 8.7 Корпус и его раннеры

| Каталог | Файлов | Формат кейса |
|---|---|---|
| `uri/<scheme>/` | 636 | `<case>.uri` + `<case>.expected.json` |
| `body/<container>/` | 44 | `<case>.body` + `<case>.expected.json` |
| `template/<group>/` | 315 | `<case>.template.json` + `<case>.expected.json` |
| `backup/` | 77 | `<case>.backup.json` (+ `<case>.pre.backup.json`) + expected |
| `direction/` | 57 | `<case>.direction.json` + expected |
| `dns/` | 15 | `<case>.dns.json` + expected |

`uri/` — 14 схем, `body/` — 6 контейнеров (`vpn`, `wgconf`, `base64`,
`uri_list`, `singbox`, `xray`). Конвенции — `contract/corpus/README.md:22-34`:
ведущие строки `# ` = описание и происхождение, затем полезная нагрузка (ровно
одна строка для `.uri`). Битые/деградационные кейсы — полноправная часть
корпуса. Per-app override — `<case>.expected.launcher.json`.

**`expected.json` уже несёт `warnings[]` у узла** — например
`contract/corpus/uri/vless/allowinsecure_lowercase_zero.expected.json`:

```json
{ "nodes": [ { "entry": {…}, "kind": "outbound", "label": "t",
              "scheme": "vless", "warnings": ["reality_fp_not_chrome"] } ],
  "v": 1 }
```

Раннеры (в `core/config/`, а не в `subscription/`):

| Корпус | Файл | Тест | Обход |
|---|---|---|---|
| `uri/` | `core/config/contract_test.go` | `TestContractCorpusURI` `:62` | `filepath.Walk`, суффикс `.uri` `:63`, `:69`, `:73`; имя кейса `:86` |
| `uri/` обратно | `core/config/contract_emit_test.go` | `TestContractCorpusEmitRoundTrip` `:129` | там же |
| `body/` | `core/config/contract_body_test.go` | `TestContractCorpusBody` `:181` | `filepath.Walk`, `.body` `:182`, `:188`, `:192` |
| `template/` | `core/template/contract_template_test.go` | `:182`, `:287` | `filepath.Walk` |
| `direction/` | `core/config/contract_direction_test.go` | `TestContractCorpusDirection` `:153` | плоский `os.ReadDir` |
| `dns/` | `core/build/contract_dns_test.go` | `TestContractCorpusDNS` `:49` | плоский |
| `backup/` | `core/backup/corpus_test.go:37` | | плоский, отсеивает `.pre.` `:373-379` |

Общие константы: `contractCorpusRelPath = "../../contract/corpus"`
`core/config/contract_test.go:30`; выбор ожидания с override `:54-59`.

### 8.8 Сторожевые тесты реестра

`core/config/subscription/registry_sync_test.go`,
`registryRelPath = "../../../contract/registry"` `:24`:

| Тест | Адрес | Что проверяет |
|---|---|---|
| `TestRegistrySyncUTLSFingerprints` | `:96` | список Go ↔ `utls_fingerprints`, **в обе стороны** |
| `TestRegistrySyncHysteria2Obfs` | `:106` | ↔ `hysteria2_obfs` |
| `TestRegistrySyncTuicCongestion` | `:118` | ↔ `tuic_congestion` |
| `TestRegistryAllowlistsRejectOutsiders` | `:130` | значение вне списка действительно отвергается |
| `TestRegistrySyncWarningCodesDeclared` | `:191` | каждая Go-константа есть в `warnings.json` |
| **`TestRegistryWarningCodesAreActuallySet`** | `:204` | код с severity ≠ `error` обязан где-то ставиться на узел |

Механика: `diffSets` `:52` — дрейф ошибка в любую сторону; `goWarningConstants`
`:170` **парсит `parse_warnings.go` текстом** регуляркой
`(Warn\w+)\s*=\s*"([^"]+)"` и падает, если констант ноль; обход `:204` идёт по
`{".", "../../../ui", "../../../core"}`, пропуская `parse_warnings.go`.
Загрузчики `t.Skipf`-ают при отсутствии `contract/` (`:41`, `:157`).

**Этот тест — рычаг кампании и мина одновременно** (Ловушка Л12).

---

## 9. Ограничения сборки (Win7 / go1.20)

| Сущность | Адрес |
|---|---|
| Джоба | `.github/workflows/ci.yml:414` `build-win7`, «Build Windows 7 (x86, legacy)» |
| Тулчейн | `ci.yml:431-435` — `go-version: '1.20.14'` |
| Понижение `go.mod` до 1.20 | `ci.yml:443-447` (PowerShell-регексп) |
| Восстановление директивы в `go.win7.mod` | `ci.yml:463-471` |
| Сборка | `ci.yml:542-548` — `go build -modfile=go.win7.mod -tags desktop …` |
| Модфайл | `go.win7.mod` — на диске `go 1.21`, CI правит на `1.20` |
| Пины несовместимых зависимостей | `go.win7.mod:57` (`x/sys` v0.25.0), `:67` (`grpc` v1.64.1), `:69` (`protobuf` v1.34.2) |

**Запрет для нового кода:** весь модуль собирается тулчейном go1.20 — нельзя
`slices`, `maps`, `min`, `max`, `clear`, дженерик-хелперы стандартной
библиотеки 1.21+, `http.Request.PathValue`, `errors.Join`-новшества 1.21+.
Перед релизом — греп + локальная проверка с `go.win7.mod`.

---

## 10. Ловушки

**Л1. Warnings теряются в обеих воронках — это и есть разрыв конвейера.**
`canonicalNodeFromEntry` (`core/config/migrate_materialize.go:156-166`) строит
`state.Node` и не копирует `e.Node.Warnings`; `ServerNodeMaterial`
(`:326-332`) вообще не имеет такого поля. Парсеры код поставили, до state он не
доехал. Чинить обязательно **оба** шва: правка одного даст узлы с ⚠ из
подписки и без ⚠ из Add — худший вид расхождения.

**Л2. Ручной `config_json` обходит вообще всё.**
`materializeServerForMigration:175-191` на непустом `ConfigJSON` делает
`stripTagAndDetour(req.ConfigJSON)` и возвращает: ни санитайзера, ни эмиттера.
`NodeFromManualConfigJSON` (`manual_config.go:34`) проверяет только наличие
`type`. Вставленный мусор едет в `state.Node.Body` дословно и валит
`sing-box check` на всём конфиге. Новый конвейер обязан прогнать и эту ветку —
но тогда изменится тело уже сохранённых узлов (см. Л3).

**Л3. Санитайзер на ранее «сыром» пути перепишет существующие тела.**
Как только `config_json`/`EmitRaw` начнут санитизироваться, тела узлов в
`state.json` изменятся у пользователей на апгрейде. Это меняет и байты, и
эталоны. Решать явно: санитизировать только на входе (новые и Regen) либо
разово мигрировать с отчётом.

**Л4. `EmitBody` обходит per-scheme switch и санитайзер.**
`GenerateNodeJSONBare:324-326` при непустом `EmitBody` отдаёт тело как есть
(через `generateCanonicalBodyJSON:1728`). Это by design (W4: тело — ровно то,
что эмиттер написал при материализации), но означает, что **правка эмиттера не
доходит до уже материализованных узлов**. Новые правила применяются только на
парсе/Regen, а не на сборке.

**Л5. Regen пересчитывает warnings, а не переносит.**
`regenServerBodyFromRawText` (`source_body_edit.go:147`) заново гонит
`origin.Raw` через воронку. Значит после Regen набор кодов может отличаться от
исходного (версия ядра сменилась, allowlist расширился). Warnings обязаны
писаться заново, полным замещением, а не append — иначе накопятся дубли и
протухшие коды.

**Л6. Per-scheme switch — цепочка `else if`, и новая схема молча урезается.**
`outbound_generator.go:346-638`. Схема без своей ветки получает только
`tag`/`type`/`server`/`server_port` — так уже ломались masque/anytls/ssh
(memory `emitter-parser-pairing`). «Тупой эмиттер» обязан перестать быть
switch'ем, иначе следующая схема повторит историю.

**Л7. Аллоулисты эмиттера молча режут неизвестные ключи.**
`appendOutboundTransportParts` (`outbound_jsonbuilder.go:84`),
`emitOutboundTLSJSON` (`outbound_tls_emit.go:41`), `xhttpV2StringKeys`,
`xhttpIntKeys`. Ключ вне списка исчезает без кода и без лога. Именно так
теряются поля новых версий ядра — и именно это должен ловить `warnings[]`.

**Л8. JSON-карта несёт `float64`/`[]interface{}` — жёсткие ассерты роняют поля.**
`tolerantInt` `:2011` и `tolerantStringSlice` `:2035` заведены как заплатки
(`up_mbps`, `server_ports`), но большинство веток §4.2 всё ещё делают
`.(string)`, `.(int)` (anytls `:620`), `.([]string)` (ssh `:630`). Тело из
`state.Body` после round-trip через JSON — всегда `float64`. Тупой эмиттер
обязан читать типы толерантно **везде**, иначе hysteria v1 снова не стартует
(memory `json-map-type-assert-trap`).

**Л9. Санитайзер возвращает коды лишь для 2 деградаций из 10.**
`SanitizeSingboxOutboundMap:63-81`. `pbk`-мусор (`:205`), fp вне allowlist
(`:160`), flow (`:244`), masque legacy (`:94`), hysteria obfs/полоса
(`:298`,`:343`,`:372`) молча уходят в `debuglog`. При переносе правил в реестр
каждая обязана обзавестись кодом — иначе `warnings[]` соврёт полнотой.

**Л10. Четыре кода объявлены в реестре, но не ставятся нигде в Go.**
`reality_pbk_invalid` (warning), `tls_insecure` (info), `flow_deprecated`
(info), `vision_with_transport` (info) — `grep` по `core/` пуст. Сегодня
`TestRegistryWarningCodesAreActuallySet` их **пропускает**, потому что ищет имя
Go-константы, а констант под них нет. Заведёте константу — тест немедленно
потребует и `AddWarning`.

**Л11. `severity: error` ≠ warning на узле.**
Правило PARSING_PRINCIPLES §4 и теста `:204`: код, который нигде не ставится, обязан быть
`error`, то есть описывать **отброшенный** узел, где `ParsedNode` не
существует. Не вешайте error-коды на выживший узел и наоборот — тест поймает,
но уже после того, как разъедется семантика с LxBox.

**Л12. Сторожевой тест парсит `parse_warnings.go` регуляркой.**
`goWarningConstants` `registry_sync_test.go:170`:
`(Warn\w+)\s*=\s*"([^"]+)"`. Перенос констант в другой файл, генерация их из
реестра или смена формы объявления (например `const Warn… = warnCode("…")`)
обрушит тест с `t.Fatal` «констант ноль». Если конвейер начнёт брать коды из
реестра — тест надо переписывать в одном коммите с переносом.

**Л13. Реестр — данные, а не контракт исполнения: CI его не сторожит.**
В workflow'ах нет ни `contract`, ни `registry`, ни `corpus` (§8.6). Дрейф
падает как обычный unit-тест внутри `go test ./...` и ничем не отличается от
любого другого красного теста. Ни одна джоба не сверяет `contract/` с копией
LxBox.

**Л14. Порядок кодов нормирован, дубли запрещены.**
PARSING_PRINCIPLES §6: «в порядке появления при разборе». `AddWarning`
(`configtypes/types.go:769-779`) дедуплицирует, но порядок = порядок вызовов.
Сортировка кодов при записи в state сломает сверку с корпусом; сбор кодов из
разных слоёв (парсер → санитайзер → эмиттер) обязан сохранять
последовательность слоёв.

**Л15. Warnings подписки и warnings узла — разные сущности, не сливать.**
`SubUpdateStatus.Warnings []FetchWarning` (`sources_v7.go:435`) — строки,
per-body, про fetch (skip-счётчики, потерянные члены групп). Новое поле узла —
коды, per-node, про значения. У них разные адресаты и разные жизненные циклы
(первое переносится между фетчами `config_service_subscriptions.go:251`, второе
пересчитывается). Один тип на оба смысла даст ровно ту же болезнь, что
`Detour` у folder/server.

**Л16. Clash YAML объявлен в реестре и не существует в Go.**
`clash_yaml_unsupported` (severity `warning`, `"go": null`) обещает детект
Clash-тела по `proxies` и внятную подсказку вместо generic-ошибки. В
`ClassifySubscriptionBody` (`body_classify.go:90`) такой ветки нет — тело
уезжает в построчный разбор и даёт ноль узлов без объяснения. Код body-level
(на подписку, не на узел) — если кампания вводит `warnings` только на узле,
ему по-прежнему негде жить.

**Л17. Гейты ядра — два разных класса, не унифицировать бездумно.**
`Naive`/`Chain`/`Tailscale`/`AWG3` выбрасывают **узел**
(`outbound_generator.go:1195-1213`); `RealityKeyShare` снимает **поле** и
читается внутри эмиттера (`:284`). Полевой гейт обязан оставлять узел живым и
ставить код; узловой — отправлять запись в `dropped`/`unsupported`. `nil`-хук
везде означает «ядро умеет».

**Л18. Лимит длины URI у Go втрое меньше канона.**
`limits.json`: `max_uri_length` канон 65536, `current.go` **8192**. Код
`uri_too_long` — `severity: error`. Длинная, но валидная ссылка (Amnezia,
MASQUE с ключами) отбрасывается у лаунчера и проходит у LxBox — расхождение
сторон на входе, до всякого санитайза.

**Л19. Fyne: ⚠-строка без `Wrapping` раздувает окно.**
`source_tab.go:1012` несёт явный комментарий об этом. Длинный `Label` без
`fyne.TextWrapWord` даёт single-line min-width, которая переопределяет
`Resize` (memory `fyne-label-minwidth-trap`). Причина деградации — текст
произвольной длины из реестра, то есть ровно тот случай.

**Л20. `Reason` у `unsupported` — английский текст, а не код.**
`sources_v7.go:252` и комментарий там же: текст парсера намеренно не
переводится, потому что он же едет в диагностику fetch. Новое поле `warnings`
— коды, которые переводятся на стороне UI. Не свести их в одно поле и не
начать переводить `Reason` «заодно».

**Л21. Один `expected.json` на обе стороны.**
Кейс корпуса нормативен для лаунчера и LxBox сразу. Добавление кода в
`warnings[]` существующего кейса — **изменение контракта**: либо встречная
правка LxBox (секция `## 24` в `TASKS_LXBOX.md`), либо обоснованный
per-app override `<case>.expected.launcher.json` со ссылкой на
задокументированное различие. Бесхозный override — ошибка линтера корпуса
(PARSING_PRINCIPLES §7).

**Л22. Win7 собирает весь модуль тулчейном go1.20.**
Никаких `slices`/`maps`/`min`/`max`/`clear`. Реестровый санитайзер — ровно тот
код, где рука тянется к `slices.Contains` и `maps.Keys`.

**Л23. AWG-форма правит тело ключами, минуя воронку.**
`applyAWGSettings` (`ui/configurator/tabs/source_awg_edit.go:239`) делает
`json.Unmarshal(node.Body)`, правит `jc/jmin/jmax/ip/id/ib` и пишет обратно
(`:273`; сброс `:289`→`:309`). Ни парсера, ни санитайзера, ни эмиттера. Значит
и warnings после этой правки не пересчитываются — набор кодов на узле
разъедется с его же телом. Либо форма идёт через конвейер, либо явно
инвалидирует коды затронутых правил.

**Л24. Бэкап 0.x кладёт узел с пустым телом.**
`legacy_read_0x.go:379-384`: запись только с `uri` получает `Origin` и **пустой
`Body`** — узел становится собираемым лишь после первого Regen или сборки.
Конвейер, который вычисляет `warnings` в момент материализации, для такого узла
не отработает вовсе: кодов не будет, пока пользователь не нажмёт Regen. А
формат 1.0 (`import10.go:116`) копирует чужое `body` как есть — вместе с
`warnings`, если они там будут. Решить, кто владеет кодами при импорте: писатель
или читатель.

**Л25. Релеи рождаются вне воронки.**
`relayNodesFromEntry` (`core/config/relay_materialize.go:129`) собирает
служебные узлы (`Service: true`) из `sockopt.dialerProxy` Xray своим кодом. Это
третье место рождения `Body` помимо двух воронок §1.1 — при переводе на единый
конвейер его легко не заметить, и у релеев warnings не появятся.

**Л26. `MaxParseFailureReasons = 3` обрезает на вставке.**
`parse_failure_reasons.go:28`, `:55-59`. Если коды узлов начнут ехать тем же
накопителем, подписка на 500 узлов покажет три причины и флаг `truncated`.
Warnings узла — **на узле**, а не в общем накопителе источника; это разные
ёмкости и разные адресаты (ср. Л15).

**Л27. Отдельные точки входа минуют общий switch схем.**
`parseHTTPProxyURI` (`node_parser_http.go:42`), `parseMasqueURI`
(`node_parser_masque.go:28`, диспатч **до** гейта длины `node_parser_core.go:100`),
`parseAmneziaVPNLink` (`:90`), `parseWGConfBase64Link` (`:108-112`). Правило,
добавленное в `buildOutbound`, до них не доедет. `node_parser_http.go:115-126`
уже показывает симптом: переиспользует парсер заголовков naive и **намеренно
не** ставит `naive_extra_headers_invalid`.

**Л28. Устаревший комментарий про MTU AWG3.**
`awg3.go:112-115` утверждает, что AWG3 держит MTU сервера и кламп AWG2 к нему
не применяется. Фактически `hasAWG3Params` скармливается `hasAWGParams`
(`node_parser_wireguard.go:431`), и AWG3 **клампится** до 1280 — как и решил
владелец 05.09.2026 (`node_parser_wireguard.go:142-145`). Не поверьте
комментарию при переносе правила в реестр.

---

## 11. Что переиспользовать, а не изобретать

| Нужное кампании | Уже есть | Адрес |
|---|---|---|
| Словарь кодов | `warnings.json`, 61 код, с `go:`-якорями | `contract/registry/warnings.json` |
| Поле кодов на узле разбора | `ParsedNode.Warnings` + `AddWarning` | `core/config/configtypes/types.go:765`, `:769` |
| Аллоулисты значений | `allowlists.json` + сторожевые тесты | `contract/registry/allowlists.json`; `registry_sync_test.go:96-190` |
| Общие валидаторы REALITY | 3 функции, уже общие у двух копий | `node_parser_transport.go:782`, `:812`, `:842` |
| Глиф ⚠ | `previewUnsupportedMark` | `ui/configurator/tabs/preview_row_view.go:22` |
| ⚠-подпись на строке узла | `SubtitleWarn` в шаблоне | `ui/configurator/tabs/source_node_row.go:61`, `:148-152` |
| Открытие ссылки + гейт схем | `platform.OpenURL` + `urlsafe` | `internal/platform/platform_darwin.go:29`; `internal/urlsafe/url.go:28` |
| Образец кнопки-ссылки | `supportLinkButton` | `ui/configurator/tabs/source_support_link.go:27` |
| Якорь документации | «Degradation codes on a node» | `docs/Protocols.md:85` |
| Конверт с `warnings[]` | корпус + раннеры | `contract/corpus/uri/**`; `core/config/contract_test.go:62` |
| Единые воронки материализации | две функции, объявленные точками правды | `migrate_materialize.go:341`; `fetch_materialize.go:51` |
