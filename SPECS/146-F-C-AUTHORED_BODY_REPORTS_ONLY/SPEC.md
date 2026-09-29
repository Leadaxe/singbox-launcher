# SPEC 146 — Авторское тело: реестр сообщает, не правит; источник узла — тело узла

Статус: **C** (решение владельца 2026-09-27, реализовано 2026-09-27).
Коммиты: `d1163968` (контракт 1.1.87), `9d340a06` (контракт 1.1.88, хвосты), `870a4224` (контракт 1.1.89, §6).
Тип: Feature. Контракт: бамп `contract/VERSION` → **1.1.87**, параграф **§84** в `contract/TASKS_LXBOX.md`; хвосты — **1.1.88**, **§85** (раздел 6).

Источник решения — спеки LxBox §576
(`LxBox/docs/spec/tasks/576-node-source-is-bare-body.md`) и §577
(`LxBox/docs/spec/tasks/577-authored-json-registry-reports-only.md`). Норма
обеих сторон — `contract/docs/PARSING_PRINCIPLES.md` §10, §11. Здесь — только
то, что касается лаунчера.

## 1. Проблема

Узел, написанный руками в форме ядра, обещан «как есть», но санитайзер,
починки сборки и уступка `detour` переписывали его по правилам реестра.
Исключение по входу (`except_sources`, потолок MTU AmneziaWG) действовало и на
sing-box JSON подписок.

## 2. Решение

- Тело авторское: свой сервер в корне или член папки, не группа, `origin.kind
  = json` и `origin.raw` — голое тело узла (`state.Node.Authored`,
  `state.IsBareNodeBody`). Признак едет в сборку (`CanonicalNode.Authored` →
  `ParsedNode.Authored`) и в пересчёт кодов (`SanitizeBodyRequest.Authored`).
- Одна точка решения — `nodeflow.Decide(scheme, authored, warning)`: обычное
  тело правится всегда; авторское — только жёстким правилом
  (`registry.RuleCoreRejects` по атрибуту `core_rejects`, узел без `type`);
  мягкое даёт код с `Applied=false`.
- Вход `singbox` — только авторское тело; sing-box JSON подписки получает
  вход `other` (`configtypes.NodeSourceOther`).
- Старые записи с документом или массивом в `origin.raw` сводятся к телу узла
  при загрузке состояния и при импорте бэкапа
  (`state.NormalizeBareBodyOrigins`). Правка во вкладке JSON у JSON-узла
  пишет в `origin.raw` только тело узла.
- Показ: при `applied: false` карточка узла берёт заголовок, причину и
  решения из реестра, а вместо «что произошло» — общую строку
  (`internal/nodewarn`). Debug API отдаёт `applied` в `/state/full`. Строка
  отчёта сборки неприменённой уступки `detour` помечена `(not applied)`, лог
  починок — тоже.

## 3. Реализация

| Что | Где |
|---|---|
| точка решения, авторский результат санитайзера, починки | `core/config/nodeflow/authored.go` |
| тихий дефолт с `core_rejects` | `core/config/nodeflow/sanitize.go` (`Result.HardPaths`) |
| признак `core_rejects`, `RuleCoreRejects` | `core/config/registry/registry.go` |
| материализация авторского тела | `core/config/node_materialize.go` |
| починки на сборке, уступка `detour` | `core/config/node_build_gate.go`, `core/config/outbound_generator.go`, `core/config/nodelink_resolve.go` |
| признак записи, вход `other` | `core/config/configtypes/types.go`, `core/state/sources_v7.go`, `core/config/subscription/*.go` |
| авторское тело и свод источника | `core/state/authored.go`, `core/state/load_router.go`, `core/backup/import.go` |
| показ | `internal/nodewarn/nodewarn.go`, `ui/configurator/tabs/source_body_edit.go` |

Шаги сборки, меняющие тело узла, и их отношение к точке решения:
санитайзер при материализации — через `AuthoredResult`; починки
(`Repairs`) в `repairBodyForBuild` и `generateRawNodeJSON` — через
`RepairsFor`; уступка `detour` в `yieldBodyToDetour`, `yieldToBuildDetour`
(и цепочки через него) — через `Decide`; полевой гейт ядра
(`gateBodyForCore`) — жёсткий всегда; `applyTailscaleStateDirectory`,
глобальные TLS (`core/build/tls_transforms.go`) и лечение `domain_resolver`
(`core/build`) — не правила реестра, норма их не касается.

## 4. Проверка

`TestContractCorpusAuthored`, `TestContractCorpusBody`, `TestBackupCorpus`,
`TestAuthoredDetourYield`, `TestPipelineAWGMTUExceptionSurvivesStateReload`;
хвосты — `TestAuthoredEditStepsGoThroughDecide`, `TestContractCorpusNodeEdit`,
`TestApplyServerBodyJSON`, `TestApplyBodyLeavesNodeDereferenced`.

## 5. Хвосты (контракт 1.1.88, TASKS_LXBOX §85)

- **Правка JSON своего узла из ссылки или INI.** Свой сервер в корне и член
  папки: `applyServerBodyJSON(node, text, ownContainer)` материализует тело
  авторским и заменяет `origin` на `{kind: json, raw: голое тело}`; ссылка
  или INI не хранятся. Контейнер определяет окно источника (`ownContainer`
  в `showSourceEditWindowAt`). Перед заменой — подтверждение
  (`ownEditDropsOrigin`). Узел подписки: происхождение сохраняется, тело не
  авторское (`MaterializeEditedBody`), как раньше.
- **Массив тел во вкладке JSON.** `config.IsNodeBodyArray`,
  `config.ParseNodeBodyArray`: в источник первый элемент. Остаток массива и
  документа — одно сообщение `NodeInputDroppedText` (оно же в форме «Add
  server»); прежний текст с перечнем частей документа снят.
- **Тест по исходникам.** `TestAuthoredEditStepsGoThroughDecide`
  (`core/config/authored_decision_point_test.go`): каждый шаг описи из §3
  доходит по графу вызовов пакета до `nodeflow.Decide` /
  `AuthoredResult` / `RepairsFor` и не содержит `m[k] = v` и `delete(...)`.
- **Коды авторского узла при разборе.** Разбор авторского тела уже гонит
  санитайзер (`materializeAuthoredBody` → `AuthoredResult`, пересчёт при
  загрузке — `sanitizeStoredNodeBody`): мягкие коды с `applied: false`, тело
  не меняется. Закреплено кейсами `authored/soft_unknown_key_nested_transport_kept`
  и `authored/hard_flow_invalid_removed`. Попутно `vless.flow` `on_invalid`
  получил `core_rejects`: sing-vmess `vless.NewClient` отвечает «unsupported
  flow», конфиг не стартует.
- **Корпус `node_edit/`** (6 кейсов, раннер `TestContractCorpusNodeEdit` в
  `ui/configurator/tabs`): контейнер, прежний источник, ввод вкладки JSON →
  источник после правки, `authored`, `rest_not_kept`, коды.

## 7. Пробел `core_rejects` (контракт 1.1.91, TASKS_LXBOX §88)

Правила, которые по прозе реестра роняют старт всего конфига, но признака не
имели, сверены с sing-box-lx: 29 признаков поставлены (`vless.encryption`,
`shadowsocks.method`, ключи и порт WireGuard, `header_protection_key`,
`id`/`ip`/`ib`, REALITY `public_key`/`short_id`/`key_share`, xhttp
placements и `x_padding_method`, `tuic.uuid`, `naive.quic_congestion_control`,
`masque.profile`/ключи, `hysteria.obfs`, `server_ports` обоих hysteria,
`tailscale.advertise_routes`). Перечень с функциями ядра — TASKS_LXBOX §88.
Мягкими остались `server` и `peers[].address` (ошибка только при соединении)
и `on_core_unsupported`. Пароль shadowsocks 2022 — правила в реестре нет.
`registry.RuleCoreRejects` понимает путь с индексом в скобках. Корпус —
26 кейсов `authored/hard_*`.

## 6. Не сделано

Расхождений нет. Документ с несколькими узлами во вкладке JSON закрыт в
контракте 1.1.89: правка узла берёт первый узел, не служебный и не группа
(`config.ParseNodeDocumentFirstNode`); импорт своих узлов по-прежнему создаёт
запись на каждый узел (`config.ParseNodeDocument` для документа с одним узлом).
