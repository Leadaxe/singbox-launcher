# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- **A source or node whose hop is unavailable is no longer dropped.** If the detour target is not in the built config — the Direction is turned off, the node was deleted, renamed or vanished from its subscription, a template group no longer exists — the hop is removed, the nodes stay and connect directly, and the source gets a ⚠ naming the target and the nodes (`detour_unavailable`, contract 1.1.171). Before, all such nodes were excluded. A detour onto the node itself and detour loops still exclude the nodes; a chain with a missing position still drops out whole.

### Technical / Internal
- Contract 1.1.171: new warning code `detour_unavailable` (params `target`, `count`, `tags`), `source_detour_missing` removed; the build report kind `target_missing` is gone (a stripped hop is an `emit_degraded` entry with the code).

## RU
### Основное
- **Источник или узел с недоступным хопом больше не выбрасывается.** Если цели detour нет в собранном конфиге — Направление выключено, узел удалён, переименован или пропал из подписки, группы шаблона больше нет, — хоп снимается, узлы остаются и подключаются напрямую, а у источника ⚠ с целью и узлами (`detour_unavailable`, контракт 1.1.171). Раньше такие узлы исключались все. detour узла на самого себя и кольца detour по-прежнему исключают узлы; цепочка с пропавшей позицией по-прежнему выпадает целиком.

### Техническое / Внутреннее
- Контракт 1.1.171: новый код предупреждения `detour_unavailable` (параметры `target`, `count`, `tags`), `source_detour_missing` снят; вид записи отчёта сборки `target_missing` убран (снятый хоп — запись `emit_degraded` с кодом).
