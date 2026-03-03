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

- **OUTBOUNDS_CONFIGURATOR/** — кнопка «Config Outbounds» у ParserConfig в визарде; окно со списком outbounds (глобальные + по источникам), Edit/Delete/Add и диалог настройки (scope, type, filters, default, addOutbounds)
- **DOWNLOAD_FAILED_MANUAL/** — единая подсистема: при ошибке загрузки (sing-box, wintun, wizard_template, SRS) показ диалога с ссылкой и кнопкой «Open folder»
- **RULE_TYPE_SRS_URL/** — тип пользовательского правила «SRS (URL)»: вставка своей ссылки на SRS в диалоге Add Rule
- **TELEMETRY/** — система статистики (opt-in, allowlist, relay)

## Сделанное (todo/complete/)

- **DIAGNOSTICS_LOG_VIEWER/** — окно просмотра логов (Internal, Core, API) с вкладки Diagnostics; sink в debuglog и api, tail для Core, автообновление 5 с
- **SRS_LOCAL_DOWNLOAD/** — локальное скачивание SRS, устранение зависимости от raw.githubusercontent.com
- **FEATURES_2025/** — лог завершённых фич 2025 года (агрегирующий список без переноса файлов):
  - `complete/2026-02-10-get-free-vpn-button.md`
  - `complete/2026-02-15-ci-cd-workflow.md`
  - `complete/GET_FREE_JSON_REFACTOR.md`
  - `complete/improvements_implementation_report.md`
  - `complete/LOGGING_CENTRALIZATION_REPORT.md`
  - `complete/MACOS_TUN_PRIVILEGED_LAUNCH_REPORT.md`
  - `complete/MIGRATION_V2_TO_V3.md`
  - `complete/OPTIMIZATION_RECOMMENDATIONS.md`
  - `complete/REFACTOR_CONFIG_STRUCTURE.md`
  - `complete/REFACTOR_UI_WIZARD_STRUCTURE.md`
  - `complete/SINGLETON_CONTROLLER.md`
  - `complete/SOCKS5_UDP_STUN.md`
  - `complete/text_based_config_experiment.md`
  - `complete/UPDATE_CHECK_POPUP.md`
  - `complete/wizard_refactoring.md`

## Workflow

1. Создать папку `todo/{FEATURE_NAME}/`
2. Написать SPEC.md (что и зачем)
3. Написать PLAN.md (архитектура)
4. Разбить на TASKS.md
5. Реализовать по TASKS с учётом [IMPLEMENTATION_PROMPT.md](IMPLEMENTATION_PROMPT.md) и заполнить IMPLEMENTATION_REPORT.md
