# SPECS — спецификации и технические задания (Spec Kit)

Все задачи (фичи, баги, исследования) в одной папке. Имя папки задаёт **номер**, **тип**, **статус** и **название**.

## Имя папки: `NNN-T-S-NAME`

| Часть | Значение | Расшифровка |
|-------|----------|-------------|
| **NNN** | 001, 002, … | Сквозной номер |
| **T** (тип) | F | Feature — фича |
| | B | Bug — баг |
| | Q | Question — исследование |
| **S** (статус) | N | New — новый / в плане |
| | W | Wait — ожидание |
| | O | Open — в работе / reopen |
| | C | Complete — сделано |
| **NAME** | UPPER_SNAKE или kebab-case | Название задачи |

Примеры: `001-F-C-FEATURES_2025`, `011-B-C-launcher-freeze-after-sleep`, `013-F-C-LOCALIZATION`.

## Формат Spec Kit (внутри папки)

| Файл | Назначение |
|------|------------|
| **SPEC.md** | Что и зачем — проблема, требования, критерии приёмки |
| **PLAN.md** | Как строить — архитектура, изменения в файлах |
| **TASKS.md** | Чеклист задач по этапам |
| **IMPLEMENTATION_REPORT.md** | Отчёт после реализации — статус, изменения, дата |

## Корневой уровень SPECS

| Файл | Назначение |
|------|------------|
| **CONSTITUTION.md** | Неизменяемые принципы проекта — приоритеты, архитектура, запреты |
| **IMPLEMENTATION_PROMPT.md** | Промпт для реализации — философия разработки, DoD, ограничения (Git, консоль) |

## Workflow

1. Создать папку `SPECS/NNN-T-S-NAME/` (номер следующий по списку, тип F/B/Q, статус N для новой задачи).
2. Написать SPEC.md.
3. Написать PLAN.md, разбить на TASKS.md.
4. Реализовать по TASKS с учётом [IMPLEMENTATION_PROMPT.md](IMPLEMENTATION_PROMPT.md), заполнить IMPLEMENTATION_REPORT.md.
5. При завершении переименовать папку: заменить статус на **C** (Complete).

## Текущий список (кратко)

- **001–010** — завершённые фичи (F-C): FEATURES_2025, WIZARD_STATE, UNIFIED_CONFIG_TEMPLATE, SRS_LOCAL_DOWNLOAD, DOWNLOAD_FAILED_MANUAL, PING_ERROR_TOOLTIP, DIAGNOSTICS_LOG_VIEWER, OUTBOUNDS_CONFIGURATOR, WIREGUARD_URI, OUTBOUND_EDIT_PREVIEW_TAB
- **011–012** — баги (B): launcher-freeze-after-sleep (C), update-reload-clash-config (O)
- **013** — фича завершена (F-C): LOCALIZATION
- **016** — фича в работе/плане (F-N): SUBSCRIPTION_JSON_ARRAY
- **014** — фича закрыта без отдельной реализации (F-C): RULE_TYPE_SRS_URL (содержание перенесено в 018)
- **017** — фича завершена (F-C): RULE_TYPE_PROCESS_PATH_REGEX (Match by path)
- **018** — фича в плане (F-N): CUSTOM_RULE_SUBSYSTEM_REFACTOR (объединяющая: константы типов ips/urls/processes/raw, вкладка Raw, params в custom_rules, документация по state в docs/)
- **015** — исследование закрыто (Q-C): TELEMETRY

Подробное описание каждой задачи — в SPEC.md соответствующей папки.
