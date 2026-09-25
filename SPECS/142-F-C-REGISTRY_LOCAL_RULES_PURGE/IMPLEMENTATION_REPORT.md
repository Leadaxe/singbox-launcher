# SPEC 142 — вычистка рукописных правил об узлах вне реестра — отчёт о реализации

Дата закрытия: 25.09.2026. HEAD кампании: `48bf6c7f` (ветка `develop`).

## Цель

Курс владельца (25.09.2026): любое правило о полях узла — допустимость,
значения, связи между полями, дефолты, деградации при негодных данных —
живёт только данными в `contract/registry/**`; в Go остаётся общий движок
(`core/config/linkmap`, `core/config/nodeflow`) без единого имени схемы или
протокола. Рукописные копии таких правил — по одной на схему, часто
расходящиеся между входами (ссылка / JSON-тело / Xray-объект / форма) —
переносились в примитивы реестра (с кодом предупреждения, если узел
меняется) и удалялись вместе с тестами, проверявшими копию.

Находки аудита — [AUDIT.md](AUDIT.md), группы A–D. Ход по волнам и решения —
[PLAN.md](PLAN.md) (таблица «Статус»).

## Волны

| Волна | Коммит(ы) | Контракт | Что снято / закрыто |
|-------|-----------|----------|----------------------|
| 1 | `4f98a370` | без бампа (1.1.56 на тот момент) | A1–A11: таблица sing-box type → схема, таблица endpoint-схем, список префиксов прямой ссылки, рукописная сборка socks-хопа Xray, предпроверки `tls`/адреса импорта sing-box, мёртвые хелперы uTLS/WS Early Data/Vision/MASQUE. Данные реестра не менялись. |
| 2 | `9547f74c` | 1.1.57 | B1 + C1: `SanitizeSingboxOutboundMap`/`singbox_sanitize.go` сняты целиком. Новый примитив `on_invalid: unwrap` (hysteria v1 `obfs`-объект → строка-пароль, `obfs_object_flattened`/`obfs_password_missing`); masque legacy-ключи → `body.skipped`, снимаются `unknown_key`. |
| 3 | `4bb75e8e` | 1.1.58 | B2, B3, B5, B6, B8 (транспорт), B9, B10; B7 частично. Данные: `chain.strip` каталог, wireguard `ip=quic requires id`, `id` LDH-паттерн, tailscale `exit_node_allow_lan_access requires exit_node`. Registry API: `Field.AllowedForScheme`, `FieldAllowed`, `FieldStrings`, `FieldsWithBuildTag`. Маска перенесена в реестр: masque получил глобальную TLS-фрагментацию. |
| 4 | `18fb2f11` | 1.1.59 | C2 (+ C6 попутно): атрибут поля `role` (`credential`/`private_key`), линтер `checkFieldRoles`. Сняты `singboxCredentialFromMap`, `canonicalCredential`, `credentialFromBody`, `Result.HadUserInfo`, таблица `shareURISecretCarrierFields`. |
| 5 | `38fba8f0` | 1.1.60 | C3 + хвосты волны 3 (`clearRangedKeepalive`, уровень AWG). Новые атрибуты `on_core_unsupported`, `range_form`, `levels`/`level`/`level_mark`. Движок: `nodeflow.NodeCoreRefusal`. Сняты три пробы (naive/tailscale/AWG3) с кэшами, `subscription/awg3.go` целиком, шесть полей `Skipped*` результата. |
| 6 | `47e163af` | 1.1.61 | C4 + B4: REALITY↔uTLS одним правилом реестра. Новые примитивы `requires[].set`, `coerce_when`, `on_hop_required {unstrip}`. Сняты `EnforceRealityFingerprint`, `HealRealityFingerprints` и шаг сборки, `NodeUsesReality`/`ChainStripsUTLS`/`ChainRealityConflict`. |
| 7 | `a7377d1f` | 1.1.63 | C5, C7–C10 + хвосты волн 1 и 3 (`vpn://`-литерал, `jmin ≤ jmax`, запрет дефолтного маршрута, `format: cidr` голого адреса) + пробел движка (CANON §6.2: снятое правилом поле недоступно последующим связям/условиям). Новые примитивы тела: `relations.kind: ordered` + `action: drop`, `item_forbidden`, normalize `cidr_masked`, `exit_capable_when`. Новые примитивы маппера: `context.<путь>`, `ref.<as>.<путь>` + `deref`, `substitute`, `when.type_of`. Сняты `applyXrayFreedomFragment`+хелперы, `amneziaPrepareConf`+хелперы, `parseTailscalePrefixList`, проверка jmin≤jmax формы AWG. |
| 8 | `8fa95fc3`, `697894fd`, `687da4be` | 1.1.62 | D1–D4: ссылки реестра на код приведены к правде (`refs.go` 141→53 записи, ~335 строк `impl`/`go`/`note` переведены на актуальное место исполнения). Новый линтер `TestRegistryCodeRefsResolve` (`core/config/registry_refs_test.go`) — Contract CI job проверяет его на любой `.go`-диф. Данные реестра не менялись. |
| 10a | `30a0bd06` | без бампа (1.1.64) | Коды `warnings.json`, жившие в логе/тексте, доезжают до пользователя: detour-коды импорта на узле, `group_empty` в отбраковке/`update_status`, `max_nodes_exceeded`, `source_detour_missing` (одна запись на пару источник→цель), `chain_*` в отчёте; `Code/Params` у `ChainDegradation`, `EmissionWarning`, `FetchWarning`, `BuildReportEntry`; отчёт переводит по коду. Реестр не менялся. |
| 10b | `122ea5f1` | 1.1.65 | Хвосты кодов с правкой реестра: поле `go` у 13 кодов 10a — на место постановки; `ssh_user_default` данными (`ssh.json` `user.default_when`, root явно, identity = тег не задета); `body_dialect_unrecognized` снят вместе с мёртвой константой; `direction_filter_matched_nothing` зарегистрирован; `detour_with_listen_port` — живой дефект исправлен общим правилом: после проставления detour сборка спрашивает `registry.Registry.YieldsTo` и снимает уступающие поля с кодом связи (`wireguard.json` `listen_port.conflicts` → код `detour_with_listen_port`), detour не трогается; новые коды сборки `source_detour_self`, `source_detour_cycle` в `EmissionWarning.Code`. Корпус +1 (`body/singbox/ssh_user_default`). |
| 11a | `3782fbe0` | 1.1.66 | Решение владельца 25.09: `warnings[]` у узла-группы (`kind=auto`). `backup.schema.json` «server и auto», BACKUP.md §2 строка `warnings[]`, CANON §6 абзац про группу; импорт бэкапа кладёт записи группы как есть (`core/backup/import10.go:decode10Source`). `group_member_missing` — на узле-группе: `subscription/singbox_groups.go:singboxGroupToNode`, `xray_json_array.go:resolveGroupMembers`, `parse_body.go:bodyParseState.finish` (код уровня тела 5d2cf47b снят, текст в сводке остался), общий `markGroupMemberMissing` складывает число; в состояние — `migrate_materialize.go:canonicalNodeFromEntry` (Auto-ветка). UI: подстрока группы в списке — «[N] … — ⚠ <заголовок кода>» (`preview_row_view.go:previewRowSubtitle`). Корпус +1 (`body/singbox/group_member_missing`). |
| 12 | `f83f23a5` | 1.1.67 | Решение владельца 25.09: `core_rejected` при переносе бэкапом — экспорт как есть, импорт запись снимает, `enabled` из файла, узел не включается (код `core/backup/import10.go` уже так делал — отставал текст с 1.1.10): описание `warnings` в обоих `$defs` `backup.schema.json`, BACKUP.md §2, CANON §9.4. Выпавший на сборке член Auto-группы (`resolveCanonicalGroup`) шёл в отчёт текстом без кода — новый код `group_member_dropped` (warning, params `tag`, `member`; `group_member_missing` не подошёл: он про разбор и живёт на группе с `count`). Фразы групп перенесены в файл использования, ключи с переводом в `ru.json`. `TestEmitE4_AutoFiltersDisabledMembers` проверяет код и params. TASKS_LXBOX §63 |

Контракт прошёл путь 1.1.56 → 1.1.63 (семь бампов; волна 1 и волна 8 — без
изменения данных, только структура/ссылки). Подробности каждой волны и
факты ядра, на которые опирались решения, — `contract/TASKS_LXBOX.md` §53–59.

## Новые примитивы реестра (сводно)

**Тело протокола / поле:**
- `on_invalid: unwrap {key, else_code}` — объект-обёртка соседнего диалекта → значение члена, иначе код (волна 2).
- `role: credential | private_key` — учётные данные узла и признак приватного ключа по роли, не по имени схемы (волна 4).
- `on_core_unsupported {drop_node, code}`, `range_form {min_core, build_tag, level, on_core_unsupported}`, `levels`/`level`/`level_mark` — узловой гейт возможностей ядра (волна 5).
- `requires[].set` — недостающий требуемый сосед материализуется с кодом вместо снятия поля (волна 6).
- `coerce_when` — условная замена уже годного значения (волна 6).
- `on_hop_required {action: unstrip}` у ключа каталога `strip` цепочки (волна 6).
- `relations.kind: "ordered"` + `action: "drop"` — упорядоченность пары/диапазона полей, снятие всех участников при нарушении (волна 7).
- `item_forbidden {values, code}` у списочного поля (волна 7).
- normalize `cidr_masked` — голый адрес получает маску хоста, биты за префиксом обнуляются (волна 7).
- `exit_capable_when` у тела протокола — годность узла быть выходом Направления (волна 7).

**Маппер:**
- источник `context.<путь>` — значение от вызывающего распаковщика контейнера (волна 7).
- `deref {key, as}` + источник `ref.<as>.<путь>` — ссылка на соседа по документу (волна 7).
- `substitute {sep, join, tokens}` — подстановка плейсхолдеров в составном значении (волна 7).
- оператор `when.type_of` (волна 7).

**Движок / норма санитайзера:**
- `nodeflow.NodeCoreRefusal` — общий узловой гейт ядра (волна 5).
- CANON §6.2: поле, снятое по ходу обхода правилом значения или связью, для последующих связей и условий отсутствует, как снятое запретом схемы (волна 7).
- `nodeflow.Repairs` — правила-починки (`requires…set`, `coerce_when`) для тел, которые сборка не прогоняет через санитайзер: замороженные тела состояния, ручной `config_json` (волна 6).
- `TestRegistryCodeRefsResolve` — линтер истинности ссылок реестра на код (волна 8).

## Изменения поведения для пользователя

Сведены в `docs/release_notes/upcoming.md` (разделы EN/RU «Highlights»/
«Основное» и «Technical/Internal» / «Техническое») по каждой волне; текст там
финальный и в отчёт не дублируется построчно. Кратко, самое заметное:

- REALITY без uTLS больше не теряется (вместо снятия — включение uTLS с
  `reality_utls_enabled`); отпечаток `random` под REALITY закрепляется как
  `chrome` (`reality_fp_random_pinned`) — у ядра нет гибридного обмена
  ключами на `edge`/`ios`.
- AmneziaWG `jmin > jmax` больше не роняет весь конфиг ядра — оба поля
  снимаются с `fields_order_invalid`, узел живёт.
- Tailscale `advertise_routes`: голый адрес получает маску хоста, дефолтный
  маршрут (`0.0.0.0/0`, `::/0`) снимается с `tailscale_default_route_advertised`.
- Фрагментация ClientHello для Xray-хопа с `freedom`+`fragment` теперь
  ставится самому хопу, а не владельцу цепочки (раньше уходила впустую —
  внутрь туннеля релея).
- `vpn://`-ссылки распознаются без учёта регистра.
- Глобальная анти-DPI фрагментация TLS применяется и к MASQUE (h2-нога;
  h3 игнорирует).
- Учётные данные узла (`role: credential`/`private_key`) читаются одинаково
  на всех входах — ссылка и JSON больше не расходятся (пароль ss вместо
  шифра, имя пользователя ssh/naive/socks/http, приватный ключ
  WireGuard/MASQUE больше не попадает в UUID-слот).
- Узловые гейты ядра (naive/tailscale/AWG3) — один и тот же список
  отброшенных узлов и кодов, теперь с заголовком из реестра в отчёте сборки.

## Проверки

- CI зелёный на `48bf6c7f`: `ci.yml` run [36160442194](https://github.com/Leadaxe/singbox-launcher/actions/runs/36160442194),
  `contract.yml` run [36160434018](https://github.com/Leadaxe/singbox-launcher/actions/runs/36160434018).
- Локально по каждой волне — только `go build ./...` и новый тест волны по
  имени (`-run`); полный прогон и `go vet` — в CI, по правилу проекта
  (`SPECS/CONSTITUTION.md` §8.1, `AGENTS.md` §3).
- Раннер корпуса контракта запускался по имени при правках реестра/корпуса
  каждой волны; корпус вырос на +7 кейсов совокупно (волна 2 +2, волна 7 +4,
  плюс единичные точечные кейсы по ходу).

## Хвосты после кампании

Открытые несоответствия и отложенные решения, зафиксированные в ходе волн.
Ничего из перечисленного не блокирует закрытие кампании — либо это осознанно
отложенное решение владельца, либо мёртвый код без влияния на поведение,
либо задокументированное ограничение нормы, которое требует отдельной волны
для снятия (неподвижная точка обхода — за рамками SPEC 142).

### Коды `warnings.json`, которые Go не ставил (волна 10a)

Волна 10a (без изменения реестра) довела коды до пользователя:

- `detour_cycle_broken`, `detour_target_missing`, `detour_to_group`,
  `detour_chain_too_deep` — на узле импорта sing-box
  (`subscription/detour_chain.go:singboxChainInfo.attachChain`), ⚠ в строке узла.
- `group_empty` — код отбраковки (`dropped[].code`) у пустой sing-box-группы
  (`singbox_groups.go:singboxGroupToNode` → `singbox_import.go`) и у группы,
  опустевшей в `parse_body.go:bodyParseState.finish`; у синтезированной
  Xray-группы исходника нет — код уровня тела (`FetchWarning.Code`,
  `xray_json_array.go:resolveGroupMembers` → `ParseSubscriptionBody`); на
  сборке — `EmissionWarning.Code` (`nodelink_resolve.go:ApplyCanonicalNodeLinks`).
- `max_nodes_exceeded` — `FetchWarning.Code/Params` (`parse_body.go:finish`,
  `ParsedBody.WarningCodes` → `config_service_subscriptions.go:parseFetchWarnings`).
- `source_detour_missing` — одна запись отчёта на пару источник→цель с числом
  узлов (`nodelink_resolve.go:ApplyCanonicalNodeLinks`, `resolveCanonicalDetour`).
- `chain_unsupported_by_core`, `chain_invalid`, `chain_hop_missing`,
  `chain_nested_position` — `ChainDegradation.Code/Params`
  (`chain_nodes.go:buildChainNode`); `chain_strip_utls_on_reality` —
  предупреждение эмиссии (`ResolveChainSources`); `chain_cycle_through_direction`
  — `chain_cycle.go:chainCycleWarnings`. Тест корпуса Направлений читает
  `ChainDegradation.Code` вместо разбора текста.
- Отчёт «Итога» переводит любую запись с кодом текстом реестра
  (`final_report_model.go:registryCodeText`).

Не сделано в 10a:

- `group_member_missing` на узле-группе: `backup.schema.json` описывает
  `warnings` узла как «server only», у `kind=auto` слота нет — нужна правка
  контракта (10b); текст потери членов остаётся в `update_status`.
  **Закрыто волной 11a (контракт 1.1.66):** код на узле-группе, `warnings`
  у `kind=auto` разрешены и переживают бэкап. Потеря члена у Auto-группы
  канона на сборке (`nodelink_resolve.go:resolveCanonicalGroup`) осталась
  текстом отчёта — это сборка, не разбор.
- Мёртвый `LoadNodesFromSource(Ex)`/`ProcessProxySource`/`rebindImportedGroupNodes`
  не снят: на него опираются семь тестовых файлов (`singbox_import_e2e_test`,
  `dedup_test`, `identity_stamp_test`, `dedup_group_rebind_test`,
  `uniquify_collision_test`, `xray_ownership_test`, `manual_config_emit_test`)
  и `integration_test` — снятие = перевод этих тестов на `ParseSubscriptionBody`,
  отдельная задача.
  **Закрыто волной 11b:** сняты `LoadNodesFromSource(Ex)`, `SourceLoadResult`,
  `applyURINodeTags`/`applyTagsToSingboxNode`/`applyTagsToXrayNode`,
  `rebindImportedGroupNodes`, хук `RecordParseFailures`, обёртка
  `FetchSubscription` и `ConfigService.ProcessProxySource`. Тесты разбора
  (дедуп, идентичность, уникализация, группы) переведены на
  `ParseSubscriptionBody`; проверки с префиксом и глобальной уникализацией —
  на `MaterializeSubscriptionBody` → `ToProxySourceV4` → сборку
  (`canonical_emit_test.go`, `TestBodyEmit_*`); ручной config_json — на
  `MaterializeServerNode` → сборку; `integration_test` — на
  `MaterializeSubscriptionBody`.
- Поля `go` в `warnings.json` у кодов выше ещё описывают прежнее «кодом не
  ставится» — обновит волна 10b вместе с бампом.
  **Закрыто волной 10b (контракт 1.1.65).**
- `naive_unavailable`, `awg3_core_unsupported`, `tailscale_core_unsupported`
  доставлены с волны 5 (отчёт `core_unsupported` с заголовком реестра).
- `template_*` (4 кода), `ssh_user_default`, `body_dialect_unrecognized`,
  `detour_with_listen_port`, `direction_filter_matched_nothing` — решения
  владельца / 10b.
  **Волна 10b (контракт 1.1.65):** `ssh_user_default` — данными
  (`ssh.json` `body.fields.user.default_when`); `body_dialect_unrecognized`
  снят (события нет); `direction_filter_matched_nothing` зарегистрирован;
  `detour_with_listen_port` — сборка после проставления detour перепроверяет
  связи `conflicts {with: detour}` реестра по готовому телу
  (`registry.Registry.YieldsTo`, `nodelink_resolve.go:yieldToBuildDetour`),
  уступает `listen_port` с кодом. Самоссылка и кольцо detour на сборке —
  новые коды `source_detour_self`/`source_detour_cycle`. Остались:
  `template_*` (миграция сборки на канонический обход — решение владельца),
  `group_member_missing`/`warnings` у `kind=auto` (решение владельца —
  **закрыто волной 11a, контракт 1.1.66**),
  `EmptyDirections`/`DetourCycles` в отчёт сборки не выводятся (код
  `direction_filter_matched_nothing` ставит только раннер корпуса).
  Проверка связей после managed-поля сделана в одном месте проставления —
  `ApplyCanonicalNodeLinks`; detour звена импортированной цепочки
  (`EmitNodeJSONs`, копия тела на эмиссии) её не проходит: endpoint-схемы
  туда не попадают (ранний выход), другим схемам связей с detour в реестре
  нет.

### vless Xray-вход: `encryption: None` расходится со входом-ссылкой

У vless нет `value_map_case: sensitive` на `encryption`, поэтому Xray-вход со
значением `None` (с заглавной буквы, как отдаёт часть панелей) молча
снимается общим правилом enum, а тот же случай на входе-ссылке роняет узел
кодом `vless_encryption_invalid`. Входы расходятся по итоговому поведению
(снятие vs явный отказ) — не описано ни одним из правил, принятых в рамках
кампании; кандидат на отдельную находку.

**Закрыто волной 9 (контракт 1.1.64).** Норма — как у ядра: `protocol/vless/outbound.go` сличает `none` точно (`Encryption != "none"` включает слой). Запись `mappers.xray.params.encryption` получила `normalize: trim` + `value_map_case: sensitive`: `None` у Xray-входа отбраковывает узел `vless_encryption_invalid`, как ссылка и тело sing-box. Корпус `body/xray/vless_encryption_none_wrong_case_rejected`.

### Мёртвый код

- `core/config/subscription/hysteria2_ports.go` — используется только своим
  тестом (`hysteria2_ports_test.go`), в рабочем пути не вызывается.
- `percentEncodeUserinfoSpaces` (`core/config/subscription/node_parser_core.go:116`) —
  используется только тестом (`userinfo_space_test.go`).
- Комментарий в `node_parser_core.go` (в районе строки 316–325, на который
  ссылается `node_parser_engine.go:191`) описывает функции, снятые волнами
  кампании; ссылка требует сверки/чистки.

**Закрыто волной 9 (f82877c6).** `hysteria2_ports.go` удалён с тестом (правило — `normalize: port_range_spec` и `$multiport` реестра hysteria2/hysteria); `percentEncodeUserinfoSpaces` и его тест сняты; ссылка `node_parser_engine.go` на несуществующие строки `node_parser_core.go:316-325` заменена на `contract/docs/IDENTITY.md §4a-C`, устаревший комментарий о «прежнем пути» в `ParseNode` поправлен.

### `ParsedNode.UUID`

В рабочем коде не читается (skip-фильтры и `getNodeValue` ключа `uuid` поле
не используют) — зафиксировано ещё в волне 4 как неточность исходного
аудита C2. Поле продолжают читать только тесты (~70 мест) и копирование
значения в звенья цепочки. Кандидат на снятие — отдельным решением, не
входит в объём SPEC 142.

**Закрыто волной 9 (f82877c6).** Поле снято с `ParsedNode` и `ParsedJump` (перенос в звенья цепочки — `SyncJumpFromChain`, `AdoptLegacyJump`, `normalizeChainHop`/`chainHopsOf` — эмиссией не читался), пять записей на входах и ставший мёртвым `registry.Registry.Credential` сняты; роль `credential` в реестре остаётся (контракт, линтер ролей). Тесты: утверждения о поле сняты, где они проверяли учётные данные по смыслу — переведены на тело (`Outbound["uuid"|"password"|"username"]`), ключи подписи в тестах дедупа — через тело.

### Гейт `build_tag` тела не объявлен у wireguard/quic/masque/chain

`on_core_unsupported`/`range_form` (волна 5) заведены не у всех тел, которые
физически зависят от тега сборки ядра — у wireguard, quic, masque, chain
такого объявления нет. Нужна отдельная сверка строки `Tags:` реальных сборок
ядра прежде, чем заводить данные. `ChainSupportProbe` цепочек остаётся
отдельным путём (не общим `NodeCoreRefusal`) — тоже сознательно вне объёма.

### Ограничение нормы §6.2 (волна 7)

Новая норма CANON §6.2 («снятое поле недоступно последующим связям/условиям»)
не даёт неподвижной точки обхода: поле, стоящее РАНЬШЕ по `body.order`, всё
ещё видит соседа, который будет снят ПОЗЖЕ в этом же проходе (вперёд-смотрящие
связи не пересматриваются). Пример из волны 7: `i1`-конфликты `id`/`ip`/`ib`
судятся по исходной карте, не по карте после снятия. Требует либо
многопроходного обхода, либо явного порядка полей в реестре — за рамками
кампании.

### Отложенная задача владельца: правила TLS masque

Решение владельца зафиксировано, но не реализовано в рамках SPEC 142: у TLS
masque снимать `alpn`/`ech`/`reality`/`kernel_tx`/`kernel_rx` всегда, с
info-кодом; `fragment`/`record_fragment` снимать только при `vhttp: h3`
(на h2 фрагментация уже применяется реестром, волна 3); `server_name`,
`disable_sni`, `insecure` — оставлять как есть. Кандидат на отдельную задачу.

**Закрыто волной 9 (контракт 1.1.64).** `tls.json`: `alpn`, `ech`, `kernel_tx`, `kernel_rx` — `forbidden_for: masque` с кодом `masque_tls_field_ignored` (новый, info), у `reality` код для masque сменён на него же; `fragment`, `record_fragment` (и плоские синонимы masque) — `conflicts {with: vhttp, when: {vhttp: h3}}`, код `masque_tls_fragment_h3` (новый, info). Пустой `vhttp` = auto ядра — фрагментация остаётся (вход ссылки материализует h3). Сборка: `core/build/tls_transforms.go` спрашивает `registry.Registry.FieldAllowedOn` по телу узла. Серая зона (utls, certificate, min_version, cipher_suites) не тронута. Корпус `body/singbox/masque_tls_owner_rules`.

## Переименование

Папка задачи переименована `142-F-O-…` → `142-F-C-…` (Complete) этим же
коммитом закрытия — кампания реализована по всем восьми волнам, CI и
contract зелёные на `48bf6c7f`, хвосты выше не блокируют закрытие и
зафиксированы для будущих отдельных задач.
