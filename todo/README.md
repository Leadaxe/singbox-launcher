# TODO — Спецификации и технические задания

Папка содержит спецификации фич в формате **Spec Kit** (spec-driven development).

## Формат Spec Kit

Каждая фича — отдельная папка `{FEATURE_NAME}/` с четырьмя документами:

| Файл | Назначение |
|------|------------|
| **SPEC.md** | Что и зачем — проблема, требования, критерии приёмки, структура данных |
| **PLAN.md** | Как строить — архитектура, компоненты, изменения в файлах |
| **TASKS.md** | Конкретные задачи — чеклист по этапам |
| **IMPLEMENTATION_REPORT.md** | Отчёт после реализации — статус, изменения, дата |

## Корневой уровень

| Файл | Назначение |
|------|------------|
| **constitution.md** | Неизменяемые принципы проекта — приоритеты, архитектура, ограничения, запреты |
| **IMPLEMENTATION_PROMPT.md** | Универсальный промпт для реализации — философия разработки, требования к коду, Definition of Done, ограничения (Git, консоль). Используется при реализации задач из SPEC/PLAN/TASKS |

## Текущие фичи

- **OUTBOUNDS_CONFIGURATOR/** — встроенный конфигуратор на вкладке Outbounds визарда: список outbounds (глобальные + по источникам), отдельные окна View (серверы источника) и Edit/Add outbound; единый overlay дочерних окон и один экземпляр View и Edit
- **DOWNLOAD_FAILED_MANUAL/** — единая подсистема: при ошибке загрузки (sing-box, wintun, wizard_template, SRS) показ диалога с ссылкой и кнопкой «Open folder»
- **RULE_TYPE_SRS_URL/** — тип пользовательского правила «SRS (URL)»: вставка своей ссылки на SRS в диалоге Add Rule
- **TELEMETRY/** — система статистики (opt-in, allowlist, relay)

## Сделанное (todo/complete/)

- **DIAGNOSTICS_LOG_VIEWER/** — окно просмотра логов (Internal, Core, API) с вкладки Diagnostics; sink в debuglog и api, tail для Core, автообновление 5 с
- **SRS_LOCAL_DOWNLOAD/** — локальное скачивание SRS, устранение зависимости от raw.githubusercontent.com
- **complete/FEATURES_2025/** — отчёты и спеки 2025 года (15 документов: Get free VPN, CI/CD, рефакторинги, логирование, TUN, миграция v2→v3 и др.). Подробнее — [complete/README.md](complete/README.md#features_2025--отчёты-и-задачи-одиночные-документы).

## Workflow

1. Создать папку `todo/{FEATURE_NAME}/`
2. Написать SPEC.md (что и зачем)
3. Написать PLAN.md (архитектура)
4. Разбить на TASKS.md
5. Реализовать по TASKS с учётом [IMPLEMENTATION_PROMPT.md](IMPLEMENTATION_PROMPT.md) и заполнить IMPLEMENTATION_REPORT.md
