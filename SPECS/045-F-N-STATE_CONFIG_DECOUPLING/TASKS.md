# TASKS 045 — STATE_CONFIG_DECOUPLING

**Статус:** stub. Детальные задачи пишутся по итогам фаз 0–1. Ниже — крупноблочный backlog.

## Фаза 0 — LxBox deep-dive

- [ ] Прочитать структуру `/Users/macbook/projects/LxBox/` (app-слой, state-слой, config-слой, dispatcher).
- [ ] Найти точки, эквивалентные Wizard Save vs Build Config.
- [ ] Зафиксировать типизированные события мобилки (если есть) и их издателей/подписчиков.
- [ ] Зафиксировать терминологию UI мобилки для того, что десктоп зовёт Wizard.
- [ ] Составить заметку `docs/LXBOX_ARCHITECTURE_NOTES.md` (или приложение к SPEC).

## Фаза 1 — карта текущего лаунчера

- [ ] Перечислить все callsites записи `config.json` (grep + ручная верификация).
- [ ] Перечислить все callsites записи `state.json`.
- [ ] Перечислить все подписчики `UpdateConfigStatusFunc` и что каждый из них реально обновляет.
- [ ] Нарисовать поток: Wizard Save → что мутируется → какие UI-части реагируют.
- [ ] Зафиксировать: кто и когда сейчас инициирует sing-box reload.
- [ ] Составить заметку `docs/CURRENT_ARCH_MAP.md` (или приложение к SPEC).

## Фаза 2 — детальный PLAN.md

- [ ] На базе фаз 0 и 1 переписать PLAN.md: формат state v5, контракт BuildConfig, кэш outbounds, два маркера, порядок коммитов.
- [ ] Принять решение о переименовании Wizard (да/нет, в что).
- [ ] Обсудить с пользователем — accept/reject/adjust.

## Фаза 3 — имплементация

Детальные задачи появятся после фазы 2. Ожидаемые крупные блоки:

- [ ] Новый пакет `core/state` (или `internal/state`) — чистая модель.
- [ ] `BuildConfig` как изолированная функция с тестами.
- [ ] Кэш outbounds (файл или in-memory) + миграция со старого формата.
- [ ] Разводка dirty-маркера на два независимых флага в `StateService`.
- [ ] UI: два сигнала (Update `*` и Restart маркер), i18n-строки, tooltip'ы.
- [ ] Миграция `state.json` v4 → v5, интеграционный тест на загрузку старых файлов.
- [ ] (Опционально) rename Wizard → Configurator: файлы, i18n, docs.

## Фаза 4 — release

- [ ] Запись в `docs/release_notes/upcoming.md` (EN + RU): `State / Config decoupling`, миграционные заметки.
- [ ] Обновить `docs/ARCHITECTURE.md` — новый слой state.
- [ ] Переименовать папку `SPECS/045-F-N-STATE_CONFIG_DECOUPLING` → `SPECS/045-F-C-STATE_CONFIG_DECOUPLING` по принятии.
- [ ] IMPLEMENTATION_REPORT.md со сводкой фактических решений и отклонений от PLAN.
