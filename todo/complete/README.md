# TODO complete — Реализованные фичи и отчёты

Папка содержит спецификации и отчёты по **уже реализованным** задачам. Актуальные (текущие) фичи перечислены в [../README.md](../README.md).

---

## Фичи (Spec Kit)

Завершённые фичи в формате Spec Kit (SPEC, PLAN, TASKS, IMPLEMENTATION_REPORT):

| Папка | Описание |
|-------|----------|
| **DIAGNOSTICS_LOG_VIEWER/** | Окно просмотра логов (Internal, Core, API) с вкладки Diagnostics; sink в debuglog и api, tail для Core, автообновление 5 с |
| **DOWNLOAD_FAILED_MANUAL/** | Единая подсистема: при ошибке загрузки (sing-box, wintun, wizard_template, SRS) — диалог с ссылкой и кнопкой «Open folder» |
| **PING_ERROR_TOOLTIP/** | Ошибка Ping: tooltip при наведении на кнопку "Error" вместо отдельного окна с ошибкой (Clash API) |
| **SRS_LOCAL_DOWNLOAD/** | Локальное скачивание SRS, устранение зависимости от raw.githubusercontent.com |
| **UNIFIED_CONFIG_TEMPLATE/** | Единый шаблон конфигурации (JSON schema, имплементация) |
| **WIZARD_STATE/** | Управление состоянием визарда (спека, схема, отчёт, код-ревью) |

---

## FEATURES_2025/ — отчёты и задачи (одиночные документы)

Отдельные документы 2025 года в папке **FEATURES_2025/** — отчёты о реализации, рефакторинги, задачи без полного Spec Kit:

| Документ | Кратко |
|----------|--------|
| **FEATURES_2025/2026-02-10-get-free-vpn-button.md** | Кнопка «Get free VPN» и связанный функционал |
| **FEATURES_2025/2026-02-15-ci-cd-workflow.md** | CI/CD workflow |
| **FEATURES_2025/GET_FREE_JSON_REFACTOR.md** | Рефакторинг Get Free JSON |
| **FEATURES_2025/LOGGING_CENTRALIZATION_REPORT.md** | Централизация логирования (debuglog) |
| **FEATURES_2025/MACOS_TUN_PRIVILEGED_LAUNCH_REPORT.md** | Запуск sing-box с привилегиями (TUN) на macOS |
| **FEATURES_2025/MIGRATION_V2_TO_V3.md** | Миграция v2 → v3 |
| **FEATURES_2025/OPTIMIZATION_RECOMMENDATIONS.md** | Рекомендации по оптимизации |
| **FEATURES_2025/REFACTOR_CONFIG_STRUCTURE.md** | Рефакторинг структуры конфига |
| **FEATURES_2025/REFACTOR_UI_WIZARD_STRUCTURE.md** | Рефакторинг структуры UI визарда |
| **FEATURES_2025/SINGLETON_CONTROLLER.md** | Singleton AppController |
| **FEATURES_2025/SOCKS5_UDP_STUN.md** | STUN-тест через системный SOCKS5 прокси (macOS) |
| **FEATURES_2025/UPDATE_CHECK_POPUP.md** | Проверка обновлений с попапом при запуске |
| **FEATURES_2025/improvements_implementation_report.md** | Отчёт по улучшениям |
| **FEATURES_2025/text_based_config_experiment.md** | Эксперимент с текстовым конфигом |
| **FEATURES_2025/wizard_refactoring.md** | Рефакторинг визарда |
