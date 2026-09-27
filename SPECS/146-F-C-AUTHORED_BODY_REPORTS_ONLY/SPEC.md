# SPEC 146 — Авторское тело: реестр сообщает, не правит; источник узла — тело узла

Статус: **C** (решение владельца 2026-09-27, реализовано 2026-09-27).
Тип: Feature. Контракт: бамп `contract/VERSION` → **1.1.87**, параграф **§84** в `contract/TASKS_LXBOX.md`.

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
`TestAuthoredDetourYield`, `TestPipelineAWGMTUExceptionSurvivesStateReload`.

## 5. Не сделано

- Массив тел во вкладке JSON не принимается (как и до SPEC 146): ввод
  ограничен телом и документом узла.
- Правка тела узла из ссылки или INI происхождение не меняет, поэтому такое
  тело авторским не становится (в LxBox Edit JSON пишет в источник голое тело).
- Тест «шаги сборки не пишут в карту тела мимо точки решения» по исходникам не
  заведён.
