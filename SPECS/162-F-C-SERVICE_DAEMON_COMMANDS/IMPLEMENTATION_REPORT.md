# IMPLEMENTATION_REPORT 162

Статус: Complete. Дата закрытия: 2026-10-13. Ветка develop.

## Коммиты

| Коммит | Что |
|---|---|
| `af7e00b0` | Волна A: `InfoData` +5 полей, `lxdclient/commands.go` (+тест), суффикс версии только `-rc<K>`, `ServiceInitOpenRC`/`ParseInitSystem`/`CommandsRecipe` (+тест), ассет по `platform`, реестр переписывает платформу и init из паспорта, `DaemonAdminClient` |
| `c2caaea5` | Волна B+C: `ui/service_commands.go` (строка Daemon command, наблюдение прогона, автомат `update`), аккордеон Troubleshooting вместо вкладок, секция Suggested commands, паспорт в источниках, Edit/host window, ru.json, доки |
| `441f56f5` | Итог `update` сверяет состояние ядра внутри вернувшегося демона (`CoreState` у коммандера): «updated, but the core did not start» вместо зелёного done |
| `5ea7d11d` | Пин ядра 1.14.3-lx.17, SYNC_REV 411cc672 (proto без изменений) |

Ревью волны B (Opus): 7 находок (флаппинг polling↔reconnecting, устаревшая версия при незамеченном обрыве, `lost` после самоперезапуска, общий виджет Restart в двух секциях, Run активна во время POST, ошибка списка как успех, пустое имя в ответе) — все исправлены до коммита.

## Живой прогон на RouteRich (OpenWrt, procd, linux-arm64)

1. **12.10, test128 → lx.16.** Паспорт переписал запись «Home» с `amd64` на `arm64`, селектор init исчез, `echo` → `done · exit 0`, лог `platform=linux-arm64 init=procd client=launcher`. `update` отработал, exit 0, но новый демон стартовал раньше, чем старый отпустил TUN (`TUNSETIFF: device or resource busy`), ядро не поднялось, VPN лежал ~80 с до ручного `restart`. Лаунчер показал зелёный done. → Фикс `441f56f5` в лаунчере; в ядре — хотфикс lx.17 (повтор TUN на старте до 10 с, апдейтер ждёт `started`).
2. **13.10, lx.16 → lx.17** через апдейтер lx.16: обрыв, переподключение, «✔ Updated 1.14.3-lx.16 → 1.14.3-lx.17»; на роутере TUN поднялся без ошибки в ту же секунду. `/admin/commands` на lx.17 отдаёт 200.

## Отклонения от PLAN

- Строка машины в списке при рестарте краснеет честно (решение владельца), `expectRestart` не вводился.
- Отдельного `TestCompareCoreVersion` нет — случаи добавлены в `TestServiceRecipes/CompareCoreVersion`.
- `core.ParseInitSystem` недоступен из `core/services` (цикл импорта): там зеркальный список `knownInitSystem`.
- Платформа и init из паспорта пишутся одним `mutate`.
- `Service *bool` в паспорте заполняется только при непустом `init_system` (у старого ядра поле отсутствует и неотличимо от false).
- Опрос прогона — цепочка «через 1 с после ответа», не свободный тикер.
- Коммандер получил `CoreState()` сверх плана (инцидент с TUN).

## Проверки

`go build ./...`, `TestClientCommands`, `TestServiceRecipes`, `TestCommandsRecipe`, `TestServiceCoreVersionGate`, win7guard, `l10n_check --strict`, `hardcoded_check --strict`. CI `run_mode=tests` на `c2caaea5` — зелёный (run 38250950480); повторный прогон при закрытии.

## Хвосты

- Нет. SPEC 163 (запросы LxBox к реестру) — отдельная задача.
