# SPEC 147 — Кэш DNS: размер, устаревшие ответы, хранение между запусками

Статус: **C** (решение владельца 2026-09-27, реализовано 2026-09-27).
Тип: Feature. Контракт: бамп `contract/VERSION` → **1.1.93**, параграф **§90**
в `contract/TASKS_LXBOX.md`, норма — `contract/docs/TEMPLATE_LANG.md` §6.8.

Источник решения — спека LxBox §580
(`LxBox/docs/spec/tasks/580-dns-cache-settings.md`). Здесь — только лаунчер.

## 1. Опись (до изменений)

- **Переменные DNS в шаблоне.** `dns_strategy`, `dns_final`,
  `dns_default_domain_resolver` объявлены в `vars[]` `bin/wizard_template.json`
  с `wizard_ui: "fix"` (на вкладке Settings не показываются, только вкладка
  DNS) и подставляются маркерами `@` в `config.dns` / `config.route`.
  Значения лежат в `state.json → vars`; во время сессии визарда их держат
  зеркальные поля модели (`model.DNSStrategy` и др.), которые перед сборкой
  переписывают `SettingsVars` (`business/dns_settings_vars.go`,
  `build.ApplyDNSScalarsToVars`).
- **Вкладка DNS.** `ui/configurator/tabs/dns_tab.go` (`CreateDNSTab`): список
  серверов, строка Strategy, правила, строка Final + Default domain resolver.
  Перечитывание из модели — `presenter_sync.go` `refreshDNSSelectsFromModel`.
- **Очистка кэша DNS.** В лаунчере её нет (ни кнопки, ни вызова ядра, ни
  удаления записей из `cache.db`). По решению задачи не добавлялась.
- **`cache_file`.** Секция задаётся шаблоном (`enabled`, `path: cache.db`,
  `store_fakeip`); код сборки своей секции не строит, демон
  (`core/daemon_manager.go`) только делает `path` абсолютным и остальные поля
  сохраняет.

## 2. Решение

| Переменная | Тип | По умолчанию | Допустимо | Поле конфига |
|---|---|---|---|---|
| `dns_cache_capacity` | `int` | `16384` | 1024..65536 | `dns.cache_capacity` |
| `dns_optimistic` | `bool` | `true` | | `dns.optimistic` |
| `dns_store_cache` | `bool` | `true` | | `experimental.cache_file.store_dns` |

- Шаблон: три переменные после `dns_final`, `wizard_ui: "fix"`; поля
  добавлены в `config.dns` и `config.experimental.cache_file` (путь не
  менялся).
- Зеркальных полей модели у новых переменных нет: вкладка DNS пишет прямо в
  `model.SettingsVars` и помечает конфиг изменённым.
- Сборка (`core/build/dns_cache_vars.go`, вызов в `BuildConfig`): сохранённый
  `dns_cache_capacity` вне границ или не число снимается из vars сборки —
  подставляется значение по умолчанию шаблона, в `Validation.Warnings`
  предупреждение. Карта вызывающего не меняется.
- Существующие пользователи: переменных в состоянии нет — действуют значения
  по умолчанию (`ResolveTemplateVars`), переноса нет.
- Бэкап: три имени переносимы (`core/backup/portable_vars.go`,
  `registry/vars.json`).

## 3. Интерфейс (вкладка DNS)

`ui/configurator/tabs/dns_cache_settings.go`, под строкой Strategy:

| Настройка | Вид | Пояснение |
|---|---|---|
| DNS cache size | поле ввода | Number of cached answers. |
| Serve stale answers | переключатель | Answer from cache at once and refresh in the background. |
| Keep DNS cache after restart | переключатель | |

Поле размера с валидатором: ввод вне 1024..65536 не сохраняется, поле
показывает «From 1024 to 65536». Строка переменной, которой нет в шаблоне
(свой шаблон), скрыта. Перечитывание — `GUIState.RefreshDNSCacheSettings`
из `refreshDNSSelectsFromModel`. Переводы — `bin/locale/ru.json`.

## 4. Проверка

- `core/build/dns_cache_vars_test.go`: `TestDNSCacheSettings_DefaultsInConfig`,
  `TestDNSCacheSettings_ChangedValuesInConfig`,
  `TestDNSCacheSettings_CapacityOutOfBoundsNotInConfig`.
- Эталоны `core/build/testdata/wizard_template_config/*` перегенерированы:
  разница — только три новых поля.
- Корпус: `contract/corpus/template/subst/dns_cache_defaults`,
  `dns_cache_changed`.

## 5. Открытое

- Верхняя граница 65536 выше отсечки int-подстановки (TEMPLATE_LANG §2.2,
  65535): введённое 65536 уйдёт в конфиг как 65535. Нужно слово владельца.
- Рост `cache.db` от записей DNS и тест вкладки DNS (видимость, пометка
  изменения) не делались.
