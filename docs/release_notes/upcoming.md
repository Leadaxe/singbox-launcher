# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- **Comments in a node's JSON source are kept.** `//`, `#` and `/* */` in text pasted into the JSON tab or the Origin field stay in the source as typed; Regen and backup import understand them, and a commented node still counts as authored. When a document is trimmed to its node arrays on save, the trimmed remainder is stored without comments.
- **A source or node whose hop is unavailable is no longer dropped.** If the detour target is not in the built config — the Direction is turned off, the node was deleted, renamed or vanished from its subscription, a template group no longer exists — the hop is removed, the nodes stay and connect directly, and the source gets a ⚠ naming the target and the nodes (`detour_unavailable`, contract 1.1.171). Before, all such nodes were excluded. A detour onto the node itself and detour loops still exclude the nodes; a chain with a missing position still drops out whole.

### Technical / Internal
- Contract 1.1.172: comments (`//`, `#`, `/* */`) in a JSON node source are allowed and kept as typed; every reader strips them before parsing (`internal/jsonfmt.StripComments`), backup dedup compares the source without them.
- Contract 1.1.171: new warning code `detour_unavailable` (params `target`, `count`, `tags`), `source_detour_missing` removed; the build report kind `target_missing` is gone (a stripped hop is an `emit_degraded` entry with the code).

## RU
### Основное
- **Комментарии в JSON-исходнике узла сохраняются.** `//`, `#` и `/* */` в тексте, вставленном на вкладке JSON или в поле Origin, остаются в исходнике как набраны, Regen и импорт бэкапа их понимают; узел с комментариями по-прежнему считается авторским. Если документ при сохранении обрезается до узловых массивов, обрезанный остаток хранится без комментариев.
- **Источник или узел с недоступным хопом больше не выбрасывается.** Если цели detour нет в собранном конфиге — Направление выключено, узел удалён, переименован или пропал из подписки, группы шаблона больше нет, — хоп снимается, узлы остаются и подключаются напрямую, а у источника ⚠ с целью и узлами (`detour_unavailable`, контракт 1.1.171). Раньше такие узлы исключались все. detour узла на самого себя и кольца detour по-прежнему исключают узлы; цепочка с пропавшей позицией по-прежнему выпадает целиком.

### Техническое / Внутреннее
- Контракт 1.1.172: комментарии (`//`, `#`, `/* */`) в JSON-исходнике узла разрешены и хранятся как набраны; каждый читатель снимает их перед разбором (`internal/jsonfmt.StripComments`), дедуп импорта сравнивает исходник без них.
- Контракт 1.1.171: новый код предупреждения `detour_unavailable` (параметры `target`, `count`, `tags`), `source_detour_missing` снят; вид записи отчёта сборки `target_missing` убран (снятый хоп — запись `emit_degraded` с кодом).
