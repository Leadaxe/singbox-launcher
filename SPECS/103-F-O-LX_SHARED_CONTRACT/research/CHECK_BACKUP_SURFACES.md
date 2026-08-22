# Механизмы экспорта/импорта и бэкапа: singbox-launcher vs LxBox

## 1. Go-лаунчер: что можно экспортировать/импортировать сегодня

### Снапшоты wizard state — единственный «бэкап»-механизм
- Хранилище: `bin/wizard_states/state.json` (текущее) + `bin/wizard_states/<id>.json` (именованные). Для remote-машин — `bin/wizard_states/remote/<machineID>/` (`ui/configurator/business/state_store.go:56-61`); изоляция через пропуск поддиректорий в листинге (`state_store.go:174-176`).
- API: `StateStore.SaveWizardState/LoadWizardState/ListWizardStates/DeleteWizardState` — тонкий wrapper над `core/state.{Save,Load}` (`state_store.go:84-260`). Save атомарный: tmp + fsync + rename (`core/state/save.go:56-73`). Load с автомиграцией v2–v4→v5→v6.
- UI-пути: кнопка **Save As** → `ShowSaveStateDialog` (`ui/configurator/dialogs/save_state_dialog.go:42`, вызов `ui/configurator/configurator.go:685-696`); кнопка **Read** → `ShowLoadStateDialog` (`ui/configurator/dialogs/load_state_dialog.go:43`, вызов `configurator.go:612-682`; именованный снапшот при загрузке копируется в `state.json`, `configurator.go:660-665`).
- **Файлового экспорта/импорта state НЕТ**: ни save-в-выбранный-файл, ни импорта из файла — снапшоты живут только на диске рядом с бинарём. Слова «backup» в UI/core встречаются лишь про миграционные `.pre-058.bak` и бэкап бинаря ядра.

### Экспорт нод (не state)
- Кнопка на вкладке Servers: экспорт share-URI всех видимых (или мультивыбранных) нод в буфер обмена, по одной строке на ноду — `ui/clash_api_tab.go:1195-1257` (`config.BuildShareURILinesForOutboundTags`).
- Per-node: копирование share URI / jump URI (`ui/clash_api_tab_render.go:61-101`), копирование JSON ноды (`ui/servers_node_info.go:240`).
- «Импорт» нод — только через источники визарда (subscription URL / server URI / ручной `config_json`), не как восстановление бэкапа.

### Структура снапшота (v6, `core/state/disk_v6.go:52-59`)
```json
{
  "meta":        { version:6, schema:"presets_v1", comment, created_at, updated_at,
                   target, target_platform, target_arch },        // disk_v6.go:115-131
  "connections": { sources:[...], outbounds:[...], defaults:{reload,max_nodes} },  // connections.go:13-24
  "rules":       [ {kind:"preset|inline|srs", ref, enabled, body} ],               // rule_types.go:46-58
  "vars":        [ {name, value} ],                                // legacy_types.go:28-31
  "dns_options": { strategy?, final?, default_domain_resolver?,
                   servers:[{kind:"template|preset|user", tag|ref, enabled, ...body}],
                   rules:[{kind:"preset|user", ref, enabled, ...body}] },           // dns_options.go:75-135
  "warp_accounts": { wg:{...}, masque:{...} }                      // disk_v6.go:73-78, omitempty
}
```
`Source` (`core/state/connections.go:41-94`): `id, type(subscription|server), enabled, label, exclude_from_global, url, skip, tag{prefix,postfix,mask}, outbounds, expose_group_tags_to_global, update{interval_hours,auto_refresh}, max_nodes, meta(SubscriptionMeta, connections.go:128-162), uri, config_json, detour_tag, detour_node_hash/detour_node_label, disabled_nodes` (map identity-hash → unix time, `connections.go:93`). Секреты (ключи/пароли в URI, preview_nodes) лежат открыто — by design (локальная машина = trust boundary).

## 2. LxBox: что кладётся в бэкап и как импортируется

### Экспорт (`lib/screens/backup_screen.dart`, спека §040)
- 5 категорий-тумблеров: Server lists / Routing / App settings / VPN settings (default ON) + Debug config (default OFF) — `backup_screen.dart:37-43`.
- **Не весь `lxbox_settings.json`**: категорийный фильтр `filterStorageForExport` → `_filterStorageForImport` (`lib/services/backup_service.dart:436-494`) отбирает top-level ключи по категориям и режет `vars` на debug/не-debug.
- Способы: Save to file / Save to Downloads / Share sheet (`backup_screen.dart:96-170`). Имя: `lxbox-backup-v{ver}-{YYYYMMDD-HHMM}.json` (`backup_service.dart:421-432`).
- **Шифрования НЕТ** — plain JSON (pretty-printed, `backup_service.dart:266`). Сам `lxbox_settings.json` тоже plain JSON с атомарной записью tmp+rename и `.bak` (`lib/services/settings_storage/io.dart:195-215`); комментарий «зашифрованный файл» в `warp_account.dart:189` не соответствует коду.
- Wire-формат (`backup_service.dart:216-231`):
```json
{ "app":"lxbox", "kind":"backup", "created_at":"...", "source_app_version":"...",
  "storage": { ...отфильтрованные ключи lxbox_settings.json... },
  "vpn_settings": { auto_start, keep_on_exit, background_mode,
                    core_logs_enabled, allow_bypass, auto_redirect, memory_limit } }
```
- Категории → ключи: Routing = `_topLevelRoutingKeys` (`backup_service.dart:23-42`): `custom_rules, route_final, channels, channels_migrated, preset_ids_remapped, route_idle_suspend, route_idle_suspend_reachable, urltest_passive_check, enabled_groups, tun_apps, vpn_mode, excluded_nodes, dns_options`. App = `_topLevelAppKeys` (`backup_service.dart:46-56`): `ping_options, last_global_update, presets_migrated, warp_account, masque_account, interrupt_connections_on_switch, node_sort_mode, node_manual_order, profiler_retention_sec`. Debug = `vars.{debug_enabled, debug_token, debug_port}` (`backup_service.dart:60`).

### Импорт
- Пикер JSON → `parseImport` валидирует маркеры `app=lxbox`/`kind=backup` (`backup_service.dart:283-288`) → preview-диалог (`lib/screens/backup_screen/import_preview_dialog.dart`) с чекбоксами категорий и режимом **merge/replace** → `applyImport` (`backup_service.dart:319-404`): merge для `server_lists` = append-by-id; остальное через `SettingsStorage.replaceRaw(filtered, merge:)`.
- §159 default-deny allowlist применяется на входе `replaceRaw` (`settings_storage.dart:816-817`), отброшенные ключи возвращаются и показываются юзеру (`backup_screen.dart:246-249`). После импорта нужен рестарт (снэкбар «Restart now», `backup_screen.dart:250-267`).
- Тот же wire-формат у Debug API: `GET /backup/export`, `POST /backup/import` (`lib/services/debug/handlers/backup.dart:21-25`).

### Полный allowlist §159
`allowedTopLevelKeys` (`lib/services/settings_storage.dart:131-158`) — 24 ключа:
`vars, server_lists, custom_rules, dns_options, ping_options, route_final, route_idle_suspend, route_idle_suspend_reachable, urltest_passive_check, excluded_nodes` (deprecated §048), `enabled_groups` (deprecated §125), `channels, channels_migrated, tun_apps, vpn_mode, warp_account, masque_account, last_global_update, presets_migrated, preset_ids_remapped, interrupt_connections_on_switch, node_sort_mode, node_manual_order, profiler_retention_sec`.

`allowedVarKeys(templateVarNames)` = `_appFeatureFlagVars` ∪ имена vars из зашитого в APK template (`settings_storage.dart:217-218`). `_appFeatureFlagVars` (`settings_storage.dart:164-212`) — 34 ключа:
`auto_update_subs, auto_update_disabled_subs, auto_reload_on_change, auto_check_updates, last_update_check_at, last_known_version, dismissed_update_version, shown_crash_stamp, config_locked_for_debug, debug_enabled, debug_token, debug_port, wifi_history, auto_record_wifi_history, probe_ms_green, probe_ms_yellow, probe_ms_orange, auto_ping_on_start, automation_receive_enabled, automation_emit_lifecycle, automation_emit_state, automation_emit_subs, automation_emit_health, automation_explainer_shown_v1, subscription_user_agent, subscription_send_hwid, subscription_hwid, subscription_device_os, subscription_ver_os, subscription_device_model, haptic_enabled, notif_perm_prompted_v1, allow_rotation, app_language`.
Template-часть динамическая; известные config-vars из кода (`_configVarKeys`, `settings_storage.dart:104-118`): `auto_detect_interface, dns_default_domain_resolver, dns_final, dns_strategy, log_level, resolve_strategy, tun_address, tun_address6, tun_auto_route, tun_mtu, tun_name, tun_stack, tun_strict_route`.

## 3. WARP/MASQUE аккаунты: формат в обоих проектах

### WG-аккаунт
| Поле (смысл) | Лаунчер `warp_accounts.wg` (`disk_v6.go:83-95`) | LxBox `warp_account` (`warp_account.dart:191-205`) |
|---|---|---|
| приватный ключ X25519 | `private_key` | `priv_key` |
| публичный ключ пира | `peer_public` | `peer_pub` |
| адреса интерфейса | `client_v4`, `client_v6` | `client_v4`, `client_v6` |
| client_id (reserved) | `client_id` | `client_id` |
| device/account | `device_id`, `account_id` | `device_id`, `account_id` |
| bearer token | `token` | `token` |
| WARP+ | `license`, `warp_plus` | `license`, `warp_plus` |
| дата регистрации | `created_at` | `created_at` |
| endpoint пира | — (нет) | `endpoint` (default `engage.cloudflareclient.com:2408`) |
| AWG-обфускация | — (нет) | `awg` {jc/jmin/.../i1} nullable (§126) |

### MASQUE-аккаунт
| Поле | Лаунчер `warp_accounts.masque` (`disk_v6.go:101-111`) | LxBox `masque_account` (`masque_account.dart:121-136`) |
|---|---|---|
| ECDSA приватник (SEC1 DER b64) | `private_key_der` | `priv_key_der` |
| серверный pubkey (PKIX DER b64) | `server_pub_der` | `server_pub_der` |
| адреса | `client_v4`, `client_v6` | `client_v4`, `client_v6` (в LxBox CIDR) |
| endpoint | `server`, `port` | `server`, `port` |
| device/token/created | `device_id`, `token`, `created_at` | те же |
| параметры ноды | — (сознательно нет: sni/network/timeouts = свойство ноды, `disk_v6.go:97-100`) | `sni`, `idle_timeout`, `keep_alive` (а `network` удалён в §393 — тоже признан свойством ноды) |

**Вывод:** по смыслу записи совпадают 1:1 (обе — кеш регистрации Cloudflare, обе сознательно НЕ хранят HTTP-версию/транспорт). Расходятся: (а) имена двух ключей — `priv_key`/`private_key`, `peer_pub`/`peer_public`, `priv_key_der`/`private_key_der`; (б) вложенность — лаунчер группирует в `warp_accounts.{wg,masque}`, LxBox — два top-level ключа; (в) LxBox дополнительно несёт `endpoint`+`awg` (WG) и `sni/idle_timeout/keep_alive` (MASQUE). Единому формату нужен маппинг имён, но не смысловая конвертация.

## 4. Пересечение сущностей

| Сущность | state.json v6 (лаунчер) | lxbox_settings.json (LxBox) | Близость |
|---|---|---|---|
| Подписки | `connections.sources[]` type=subscription: url, tag{prefix,...}, update, max_nodes, meta (заголовки, userinfo, preview_nodes) — `connections.go:41-57` | `server_lists[]` type=subscription: url, tag_prefix, update_interval_hours, meta, identity-override, import_rules, on_update_action — `server_list.dart:150-254` | **Близко по ядру** (url/enabled/tag-prefix/interval/meta-заголовки — LxBox-совместимый контракт заголовков, `connections.go:126-127`); расходятся: LxBox — камelCase-модель c identity/import_rules, лаунчер — skip[]/outbounds[]/detour |
| Ручные серверы | `sources[]` type=server: `uri` или `config_json` | `server_lists[]` type=user (`rawBody`, `server_list.dart:370-373`) + папки `FolderServers`/`FolderMember{raw,enabled,detour}` (`server_list.dart:463-530`) | Близко: оба хранят сырой URI/фрагмент; у LxBox есть папки и per-member detour |
| Правила | `rules[]` {kind: preset\|inline\|srs, ref, enabled, body{vars \| name+match+outbound \| name+srs_url+outbound}} — `rule_types.go:46-86` | `custom_rules[]` {kind: inline\|srs\|preset\|json, id, name, enabled, outbound, domains/ipCidrs/ports/packages/wifi…, dns, resolve} — `custom_rule.dart:41,501,737,954,1063; toJson 621-645` | **Таксономия kind совпадает** (preset/inline/srs; LxBox добавил json); тела расходятся: лаунчер inline = сырой sing-box `match`-объект, LxBox — типизированные camelCase-поля + packages/wifi/dns-аспект |
| DNS | `dns_options.servers[]/rules[]` с kind template\|preset\|user, flat-сериализация — `dns_options.go:75-135`; скаляры в `vars` | `dns_options.{servers:[], rules:[]}` — свободные map'ы (`network.dart:99-221`), legacy `rules_json` | Форма похожа (servers+rules), но у LxBox нет kind-дискриминатора/ссылок на template |
| Vars | `vars[]` — массив `{name,value}` (`legacy_types.go:28-31`, `disk_v6.go:56`) | `vars` — объект `{name: value}` (`vars.dart:8-27`) | **Смысл одинаков** (подстановка @var в template), контейнер разный (array vs map); пересекающиеся имена: `dns_final`, `dns_strategy`, `tun_*`, `log_level` |
| Route final | `vars[]` name=`route_final` (canonical, `presenter_state.go:181-190`, `presenter_state_helpers.go:124-156`; template подставляет `@route_final`) | top-level `route_final` (string outbound, `network.dart:13-24`) | Смысл идентичен, расположение разное (var vs top-level) |
| Отключённые ноды | per-source `disabled_nodes`: map identity-hash → unix time (SPEC 094 D4, `connections.go:86-93`) | per-subscription `disabled_hashes`: map identity-hash → ISO8601 lastSeen (§283, `server_list.dart:164-171`, toJson `server_list.dart:242-244`); глобальный `excluded_nodes` удалён (§125-cleanup, `settings_storage.dart:423`) | **Очень близко** — оба keyed по identity-hash с TTL-очисткой; отличие только формат времени (unix vs ISO8601) и имя ключа |
| WARP | `warp_accounts.{wg,masque}` (`disk_v6.go:73-111`) | `warp_account` + `masque_account` (top-level) | Близко (см. §3): маппинг имён полей + вложенность |
| Нет аналога | `connections.outbounds[]` (global), `detour_*`, `meta.target*` | `channels`, `tun_apps`, `vpn_mode`, `ping_options`, `vpn_settings` (native), feature-flag vars | — |

**Ключевые наблюдения для единого формата бэкапа:** (1) у лаунчера нет ни файлового экспорта, ни маркеров формата — у LxBox уже есть конверт `{app, kind, created_at, source_app_version, storage}` и допуск-фильтр на импорте, его конверт можно взять за основу; (2) самые «готовые» к унификации сущности — disabled-ноды (hash-ключи совпадают), WARP/MASQUE (нужен только rename-маппинг) и route_final; (3) самые расходящиеся — подписки (модель нод/меты) и тела правил; (4) оба хранят секреты открытым текстом и оба пишут атомарно, шифрование бэкапа не реализовано нигде.