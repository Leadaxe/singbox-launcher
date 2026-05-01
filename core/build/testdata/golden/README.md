# Golden testdata for `core/build.BuildConfig`

Regression-protection harness для strangler-fig порта (SPEC 045 phase 5.3).

## Архитектурный контекст (важно перед добавлением сценариев)

В исходной кодовой базе **две разные сборки** config.json:

1. **«Initial build» — `ui/wizard/business.BuildTemplateConfig` (521 LOC).**
   Вход: `model.TemplateData` (parsed wizard_template) + `state.SettingsVars` + custom_rules + DNS.
   Выход: полный config.json с `/** @ParserConfig */` JSON-comment header, подставленными `@var`-плейсхолдерами, **сгенерированными** маркерами `/** @ParserSTART/@ParserEND */` и `/** @ParserSTART_E/@ParserEND_E */`, где между маркерами — outbounds/endpoints из `WizardModel.GeneratedOutbounds[]/GeneratedEndpoints[]`.
   Триггерится Wizard Save.

2. **«Update build» — `core/config.PopulateParserMarkers` + `WriteToConfig`.**
   Вход: уже существующий config.json (с маркерами от прошлого «initial build») + свежие outbounds из парсера подписок.
   Выход: тот же config.json с **обновлённым** содержимым между маркерами; всё остальное нетронуто.
   Триггерится кнопка Update / cron auto-update.

После SPEC 045 эти две сборки **сводятся в одну**: `core/build.BuildConfig(state, cache, template) → config`. Pure функция, единственный writer config.json.

## Какие сценарии у нас есть

### `marker-fill-*` (3 шт.)

Тестируют **поведение под-компонента `PopulateParserMarkers`** — substitute vars + заполнение существующих маркеров. Шаблоны искусственные: ВКЛЮЧАЮТ маркеры в `template.json`. Это **НЕ как реальный wizard_template** — это как уже-сгенерированный config.json в моменте между двумя parser-run'ами.

Полезность: pin'ят корректность substitute-vars и marker-replacement логики, которая останется частью BuildConfig после порта.

| Сценарий | Покрывает |
|---|---|
| `marker-fill-empty/` | Шаблон с пустыми маркерами + state без vars + пустой cache → маркеры остаются пусты, остальное as-is |
| `marker-fill-vars/` | Подстановка `@log_level` / `@tun_stack` в шаблон с маркерами |
| `marker-fill-cache/` | Заполнение existing маркеров outbounds + endpoints из cache |

Эти сценарии **зелёные** на текущем минимальном `BuildConfig` (по сути — обёртка над `PopulateParserMarkers + substituteVars`).

### `real-v088/`

Реальный input от установки v0.8.8.x с dev-машины (3 файла + сгенерированный пустой cache.json):
- `template.json` — реальный `wizard_template.json` (БЕЗ маркеров; `parser_config` секция + sing-box-конфиг с `@var` плейсхолдерами).
- `state.json` — реальный `bin/wizard_states/state.json` (vars, DNS, custom_rules).
- `cache.json` — пустой (`outbounds.cache.json` появится в установках после фазы 5.1 SPEC 045).
- `expected.config.json` — реальный `bin/config.json` после успешного Save+Update.

**Падает на текущем `BuildConfig`** — это и есть target для порта `BuildTemplateConfig`. Каждый порт-шаг закрывает часть расхождения с expected.

## Как добавить ещё один реальный сценарий

С dev-машины:
```bash
TOKEN=$(jq -r '.debug_api_token' /Applications/singbox-launcher.app/Contents/MacOS/bin/settings.json)
curl -sH "Authorization: Bearer $TOKEN" http://127.0.0.1:9263/debug/snapshot \
  > /tmp/snap.json
DST=core/build/testdata/golden/<scenario>
mkdir -p "$DST"
jq '.files.template' /tmp/snap.json > "$DST/template.json"
jq '.files.state'    /tmp/snap.json > "$DST/state.json"
jq '.files.cache'    /tmp/snap.json > "$DST/cache.json"
jq '.files.config'   /tmp/snap.json > "$DST/expected.config.json"
```

Если `cache` отсутствует (`null`):
```bash
echo '{"version":1,"state_id":""}' > "$DST/cache.json"
```

Затем `go test ./core/build -run TestGoldenScenarios -v`.

Ожидание: на старте порта **все** реальные сценарии красные. По мере того как BuildConfig обрастает функциональностью (effective config через `wizardtemplate.GetEffectiveConfig`, генерация `@ParserConfig` header, parser-marker emission, post-steps DNS/route/experimental с custom rules), сценарии один за другим становятся зелёными.

## Стратегия порта

`BuildTemplateConfig` декомпозируется на:

1. **`normalizeParserConfig`** (~20 LOC) — JSON-marshal с правильным форматом, заголовок `@ParserConfig`. Уже существует в `core/config`, использовать оттуда.
2. **`buildConfigSections`** — итерация top-level keys, форматирование секций.
3. **`buildOutboundsSection`** / **`buildEndpointsSection`** — emit маркеров + содержимое из cache.
4. **`buildDNSSection`** / **`buildRouteSection`** / **`MergeRouteSection`** — merge custom_rules в route.
5. **`wizardtemplate.GetEffectiveConfig`** — применение `params` (платформо-conditional), substitute vars с type-cast, `if`/`if_or` условия.
6. **`MaterializeClashSecretIfNeeded`** — генерация секрета (sticky в state.Vars).
7. **`SyncDNSModelToSettingsVars`** — копирование DNS-полей модели в state.Vars.

Каждый из 1–7 — отдельный sub-task в фазе 5.3 SPEC 045, отдельный коммит, golden-сценарии ловят регрессию.
