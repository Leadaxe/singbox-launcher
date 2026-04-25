# PLAN 045 — STATE_CONFIG_DECOUPLING

**Статус:** stub. Детальный план пишется по итогам фаз 0–1 (LxBox deep-dive + карта текущей архитектуры лаунчера). См. `SPEC.md` раздел «План (высокоуровневый)».

## Ожидаемая структура PLAN.md после фаз 0–1

1. **Новый слой `state` (чистая модель).**
   - Структуры: какие поля, какая версия, миграции с v4 → v5.
   - Где живёт в коде (предположительно `core/state/` или `internal/state/`).
   - Кто читает / пишет.
2. **Build Config как функция.**
   - Сигнатура: `func BuildConfig(state *State, outboundsCache *OutboundsCache) (*Config, error)`.
   - Где вызывается: Update-кнопка, auto-update cron, initial boot (при отсутствии config.json).
   - Отделить от собственно sing-box reload (reload — следующий уровень).
3. **Кэш outbounds.**
   - Где хранится (файл `bin/outbounds.cache.json` или в `StateService` в памяти).
   - Когда инвалидируется (по каждой подписке, по источнику, глобально).
   - Совместимость со старым кодом, который лепит outbounds прямо в `config.json` при парсинге.
4. **Два dirty-маркера.**
   - `StateService.UpdateDirty()` — флаг для кнопки Update.
   - `StateService.RestartDirty()` — флаг для кнопки Restart.
   - Как вычисляются (diff state между save и последним build / последним reload).
   - Сброс флага на успешный build / restart.
5. **Переименование Wizard.**
   - Решение о новом имени (после фазы 0: перенять ли терминологию LxBox).
   - Площадь: i18n ключи, имена файлов, вкладка в UI, документация.
   - Отдельный коммит в конце имплементации.
6. **Порядок PR'ов / коммитов.**
   - Слой state / модель. Тесты.
   - Build Config как функция. Тесты.
   - Разводка два-маркера. UI.
   - Миграция `state.json` v4 → v5 + интеграционные тесты.
   - Rename Wizard → Configurator (или финальное имя).

## Требования к фазам 0–1 (input для PLAN)

**Фаза 0 — LxBox deep-dive** должна ответить:

- Как называются слои state / config-build / dispatcher у мобилки.
- Какие события типизированы, кто их публикует, кто слушает.
- Как мобилка решает проблему «нет подписок при первом build'е».
- Какие тесты прикрывают инвариант «state save ↛ config mutate».
- Какие имена использует UI мобилки для того, что на десктопе «Wizard».

**Фаза 1 — карта текущего лаунчера** должна выдать:

- Все callsites `config.json` write (grep `os.WriteFile.*config`, `atomicwrite.*config`).
- Все callsites `state.json` write (аналогично).
- Все callsites `UpdateConfigStatusFunc` (broadcast-шум, который уйдёт в event-driven).
- Граф: что триггерит Wizard Save, что триггерит парсер, что триггерит sing-box reload.

Оба артефакта — либо отдельные markdown-файлы в `docs/`, либо приложения к этому SPEC'у.
