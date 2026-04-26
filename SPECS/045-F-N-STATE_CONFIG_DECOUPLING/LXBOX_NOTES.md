# Фаза 0 — LxBox deep-dive

Отчёт по архитектуре мобильного клиента **LxBox** (`/Users/macbook/projects/LxBox/`), фаза 0 SPEC 045. Источник — структурный анализ кода (Flutter/Dart).

---

## 1. Стек и структура каталогов

**Flutter** (Dart), не Go. Архитектурные идеи переносимы, конкретные примитивы (ChangeNotifier, sealed classes) — нет.

```
/app/lib/
├── models/        — данные, valueobjects, validators
├── services/      — бизнес-логика (builder, parser, storage, migrations, app_log)
├── controllers/   — state management (ChangeNotifier-иерархия)
├── screens/       — UI экраны
└── vpn/           — нативный бридж к VPN
```

## 2. Слой state

**Точка персистенции:** `/app/lib/services/settings_storage.dart`. Пишет ортогональные куски:
- `vars` (Map<String,String>) — wizard-параметры (`tun_stack`, `clash_api_port`, `log_level`, DNS…).
- `server_lists` (List<ServerList>, v2) — подписки + локальные источники; миграция с v1 встроенная.
- `enabled_groups` (Set<String>) — какие preset-группы ON.
- `enabled_rules` (Set<String>) — активные selectable rules.
- `custom_rules` (List<CustomRule>) — пользовательские правила.
- `rule_outbounds` (Map<String,String>) — привязка правил к outbounds.

**State версионируется неявно** — нет поля `schema_version`, миграции lazy при первом чтении (`settings_storage.dart:240–292`, `migration/proxy_source_migration.dart:14–83`).

## 3. Слой config-build

**Единая точка сборки:** `/app/lib/services/builder/build_config.dart`, функция `buildConfig(lists, settings, template) → BuildResult`.

`BuildResult`:
- `configJson` — готовый JSON для sing-box
- `config` — parsed Map (для юнит-тестов)
- `validation` — fatal errors + warnings
- `emitWarnings` — текстовые предупреждения
- `generatedVars` — vars, сгенерированные процессом (clash_api port/secret при первом запуске)

**Шаги:** merge vars → deep-copy template + substitute → NodeSpec → SingboxEntry (outbound/endpoint) → preset-группы (`vpn-1/2/3 + auto`) → selectable+custom rules → post-steps (TLS-fragment, custom DNS) → validate.

**Кэш outbounds:** `ConfigCache` в `/app/lib/models/home_state.dart:32–71` — парсит `outbounds` из готового `config.json` один раз, держит lookup-таблицу `protoByTag`. Пересобирается только при смене `configRaw`.

## 4. Dispatcher / events

**Типизированных событий нет.** Использован ChangeNotifier-паттерн Flutter:

- `HomeController extends ChangeNotifier` → `notifyListeners()` на изменение `HomeState`.
- `SubscriptionController extends ChangeNotifier` → держит `configDirty` флаг.
- `AppLog.I (ChangeNotifier)` — глобальный кольцевой буфер логов. Читается UI для вкладки Debug.

Реактивность — broadcast через одну точку `_emit()` в каждом контроллере + `copyWith()` на state. **Проблема та же, что у десктопа** — coarse-grained. Мобилка её пока не решила; это отдельный TODO у них.

Явные «события» существуют только как `DebugEntry` (строки лога) с типизированными полями `DebugSource {app, core}` и `DebugLevel {debug, info, warning, error}` — это логирование, не event-bus.

## 5. Терминология UI (Wizard-аналог)

В мобилке **термин «Wizard» не используется**. Вместо этого — раздельные экраны по доменам:
- `HomeScreen` — Start/Stop, статус.
- `ConfigScreen` — редактор сырого `config.json`.
- `SettingsScreen`, `RoutingScreen`, `DnsSettingsScreen`, `AppSettingsScreen` — редакторы vars, rules, DNS, настроек приложения.
- `SubscriptionsScreen` / `SubscriptionsDetailScreen` — редакторы источников.

**Ключевой вывод:** мобилка **уже разделена** — Save в любом экране пишет **только state**, не пересобирает config. Пересборка — явный шаг (кнопка Update, auto-update по триггерам, `generateConfig()`).

Поскольку единого «Wizard»-экрана у мобилки нет, готового имени для десктопного переименования оттуда не взять. Решать на фазе 2.

## 6. Dirty / сигналы

**Два маркера.** Реализованы минимально, но семантически разведены:

1. **`SubscriptionController.configDirty`** (bool, `subscription_controller.dart:215`) — «state-источников изменился, config не пересобран».
   - `true` ← add/remove/edit source (`:683`), failure manual update (`:710`).
   - `false` ← successful `generateConfig()` (`:495`, `:515`), successful `updateAllFromSubscriptions()` (`:515`).

2. **`HomeState.configStaleSinceStart`** (bool, `home_state.dart:92`) — «config saved в момент, когда туннель уже работал».
   - Sticky в рамках одной сессии tunnel-up; сбрасывается на переходах tunnel up↔down.

UI-композиция в `HomeScreen:55–59`:
```dart
bool get _needsRestart {
  final state = _controller.state;
  if (!state.tunnelUp) return false;
  return state.configStaleSinceStart || _subController.configDirty;
}
```

Т.е. мобилка фактически различает две ситуации: **«config нужно перестроить»** и **«sing-box нужно перезапустить»**. Один маркер для Update, другой для Restart, как SPEC 045 и планирует.

## 7. Миграции state

Нет явной версии state-файла. Есть:
- Lazy-миграции при чтении: `SettingsStorage.getServerLists()` → детектит старый ключ `proxy_sources`, вызывает `migrateProxySources()`, перекладывает в новый ключ.
- `_absorbLegacyAppRules()` — конвертация `app_rules` → `custom_rules`.
- `markPresetsMigrated()` — one-shot флаг для миграции Presets v1 → v2.

**Урок для десктопа:** У нас уже есть `state.json v4` (SPEC 002), это лучше чем lazy-ключи. Оставляем явное версионирование.

## 8. Bootstrap

`HomeScreen.initState()` (`home_screen.dart:71–116`):
1. `SubscriptionController` → `AutoUpdater` → `HomeController` в правильном порядке зависимостей.
2. `_controller.init()` → `_loadSavedConfig()` из native VPN client.
3. `_initSubsAndAutoUpdate()` → sources из storage, старт `AutoUpdater`.
4. `UpdateChecker.hydrate` + async fetch через 5s.

**Порядок решения «state.json vs config.json» при старте:** читаются независимо. Если state новее или config отсутствует — будет переписан при первом `generateConfig()`, который auto-trigger'ится `AutoUpdater.maybeUpdateAll` по app-start триггеру.

Т.е. у мобилки **есть готовый паттерн**: «на старте всегда один раз проверить, не стоит ли пересобрать». Этот же подход можно перенести в десктоп — убрать текущую связку «Wizard Save всегда пишет config.json» и заменить на «Save пишет state, а Build либо триггерится пользователем, либо раз на старте».

## 9. Ключевые файлы (для самостоятельного чтения)

**State:**
- `settings_storage.dart:14–50` — `SettingsStorage`, getVar/setVar/getServerLists.
- `models/home_state.dart:73–212` — `HomeState`, `ConfigCache`, `copyWith`.
- `models/server_list.dart` — `ServerList` (sealed), `SubscriptionServers`, `UserServer`.

**Config-build:**
- `services/builder/build_config.dart:70–150` — `buildConfig`, `BuildResult`, `BuildSettings`.
- `services/builder/post_steps.dart` — TLS-fragment, custom DNS.
- `services/builder/server_list_build.dart` — `ServerList` → `SingboxEntry`.

**Controllers:**
- `controllers/home_controller.dart:58–90` — `init()`, `_loadSavedConfig()`.
- `controllers/home_controller.dart:330–377` — `saveParsedConfig()`, `saveConfigRaw()`.
- `controllers/subscription_controller.dart:215–505` — `configDirty`, `updateAllFromSubscriptions()`, `generateConfig()`.

**Events / logging:**
- `services/app_log.dart:1–57` — `AppLog` singleton.
- `models/debug_entry.dart` — `DebugEntry`, `DebugSource`, `DebugLevel`.

**Миграции:**
- `services/migration/proxy_source_migration.dart:14–83`.
- `services/settings_storage.dart:240–292` — absorbLegacyAppRules, markPresetsMigrated.

**UI:**
- `screens/config_screen.dart:111–119` — `ConfigScreen._save()` → `controller.saveConfigRaw()`.
- `screens/home_screen.dart:55–59` — `_needsRestart` формула.

## 10. Что переносимо, что нет

**✅ Переносимо (идеи):**
1. **Разделение state vs config-build** как раздельных операций — ключевая идея SPEC 045, сделано именно так.
2. **`BuildConfig(state, subCache) → BuildResult{configJson, validation, warnings, generatedVars}`** — чистая функция, один call-site записи `config.json`.
3. **ConfigCache после сборки** — один раз парсить outbounds в lookup, кэшировать.
4. **Lazy-миграции для «мелких» полей** (хотя у нас уже есть явная версия state.json, это удобно для ad-hoc полей settings.json).
5. **Два разных dirty-маркера** — для Update и для Restart, семантически независимы.

**❌ Не переносимо / нужна адаптация:**
1. **ChangeNotifier** — Flutter-специфично. В Go нужна своя реактивность: простые колбэки, каналы или pubsub.
2. **AppLog in-memory singleton** — ок для мобилки, десктопу нужна ротация файлов (у нас уже есть debuglog, не трогаем).
3. **Sealed classes** (Dart) — в Go заменяется на `interface` + дискриминированные union'ы через switch на тип или на поле-тэг.
4. **VPN connect/stop триггеры** — у мобилки это events от native-слоя. У десктопа другой бэкенд, триггеры надо организовывать вручную.
5. **Отсутствие явной версии state-файла у мобилки** — у нас это преимущество (SPEC 002 `state.json v4`), оставляем.
6. **Нет типизированных событий у мобилки** — нам стоит ввести их сразу при рефакторинге (будущий SPEC 047), не копируя coarse-grained broadcast.

---

**Главный вывод:** LxBox **уже живёт в той архитектуре**, которую мы хотим. Структура state-слоя, контракт `buildConfig()`, два маркера dirty/stale — всё можно портировать почти один-в-один, заменив Flutter-примитивы на Go-эквиваленты. Единственное, что не стоит копировать — coarse-grained реактивность через ChangeNotifier: у мобилки это технический долг, у нас есть шанс ввести event-bus правильно сразу.
