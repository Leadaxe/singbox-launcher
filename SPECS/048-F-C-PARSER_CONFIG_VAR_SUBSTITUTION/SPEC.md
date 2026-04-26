# SPEC 048 — PARSER_CONFIG_VAR_SUBSTITUTION

**Тип:** F (Feature) · **Статус:** C (Complete) · **Релизы:** v0.8.8.1 (initial hotfix), v0.8.8.3 (follow-up). Версия v0.8.8.2 была локально использована во время работы над follow-up'ом, но публикации не достигла — третий баг (неверный JSON-ключ) был найден ещё до создания тега, поэтому опубликован сразу v0.8.8.3.

**Ретроспективная спецификация** хотфикса бага с `@varname` плейсхолдерами в `parser_config.outbounds[].options`. Оба релиза вышли как точечные заплатки поверх v0.8.8 в течение одного дня. SPEC написан после факта — фиксирует **что** было сделано, **почему** именно так, и **где наступили на грабли**.

---

## 1. Контекст и предыстория

### 1.1. Корень проблемы — v0.8.7 (SPEC 040)

В v0.8.7 ([SPEC 040](../040-F-C-WIZARD_TEMPLATE_OPTION_TITLES/SPEC.md)) `auto-proxy-out` в `bin/wizard_template.json` получил три template-переменные:

```jsonc
{
  "tag": "auto-proxy-out",
  "type": "urltest",
  "options": {
    "url":      "@urltest_url",
    "interval": "@urltest_interval",
    "tolerance": "@urltest_tolerance"
  }
}
```

До v0.8.7 эти значения были захардкожены (`"5m"`, `100`, `https://cp.cloudflare.com/generate_204`). Идея SPEC 040 — вытащить их в template `vars`, чтобы пользователь мог менять через Settings tab.

### 1.2. Скрытый дефект SPEC 040

`auto-proxy-out` сидит в **`parser_config.outbounds[]`** (массив global selector'ов, генерируемых парсером), а **не** в `template.outbounds[]` (статические outbound'ы шаблона).

Подстановка `@varname` (`ui/wizard/template/SubstituteVarsInJSON`) применяется только к **`template.config.*`** секциям через `ApplyTemplateWithVars`. К `parser_config` она **не** применяется — parser-генератор (`core/config/outbound_generator.go::GenerateSelectorWithFilteredAddOutbounds`) копирует `outboundConfig.Options` дословно.

Итог: после v0.8.7 в финальном `config.json` оставались литералы `"@urltest_interval"`, `sing-box check` падал на `outbounds[1].interval: time: invalid duration "@urltest_interval"`, лаунчер не запускался.

### 1.3. Почему баг не всплыл сразу

- Существующие установки v0.8.6 и старше имели **state.json**, где `parser_config.outbounds` хранилось с **захардкоженными** значениями (от старого шаблона до SPEC 040). При загрузке wizard приоритезирует `state.json` над template, и значения сохранились — баг был невидим.
- На свежей установке (или после удаления state.json) `parser_config` копировался из нового шаблона с placeholders, и баг моментально проявлялся.
- В v0.8.8 (релиз после v0.8.7) баг тоже был, но всплыл массово только когда юзеры начали обновляться/переустанавливаться в открытую (Telegram-чат: TrueMagnetMan, Дмитрий и др.).

## 2. v0.8.8.1 — точечный hotfix

### 2.1. Решение

Пошёл путём **минимальной хирургии в parser-слое**, без затрагивания подстановочного механизма wizard-template (он работает корректно для своего scope — template body).

Новый файл `core/config/varsubst.go`:

```go
type VarSubstituter func(name string) (interface{}, bool)

func SubstituteParserConfigPlaceholders(pc *ParserConfig, subst VarSubstituter)
func BuildVarSubstituterFromDisk(execDir string) VarSubstituter
func knownPlaceholderFallback(name string) (interface{}, bool)
```

`SubstituteParserConfigPlaceholders` обходит все `Options` под `parser_config.outbounds[]` и `parser_config.proxies[].outbounds[]`, для каждого string-значения, начинающегося с `@`, разрешает имя через переданный `VarSubstituter`. Резолюция в три уровня:

1. Пользовательский override из `state.json` (`settings_vars`).
2. `default_value` из `wizard_template.json` `vars[]`.
3. Hardcoded fallback (только для трёх известных URLTest-плейсхолдеров) — defense-in-depth, на случай отсутствующих/битых файлов.

Hook вживлён в **обоих путях** генерации:

- `core.ConfigService.GenerateOutboundsFromParserConfig` — wizard preview / Save.
- `core.ConfigService.UpdateConfigFromSubscriptions` — manual Update + auto-update cron.

Дополнительный safety net: сам `config.GenerateOutboundsFromParserConfig` дёргает `SubstituteParserConfigPlaceholders(pc, nil)` на входе — если caller не передал substituter, hardcoded fallback всё равно сработает для трёх известных placeholders.

### 2.2. Самоисцеление сломанных state.json

Существующие битые `state.json` (со значениями `@urltest_*` в parser_config) **не требуют миграции**. При первом Save в визарде:

1. `ParseAndPreview` загружает `model.ParserConfigJSON` (с placeholders) в struct.
2. Перед генерацией selector'ов вызывается `SubstituteParserConfigPlaceholders` — мутирует Options в-памяти.
3. `model.ParserConfig` теперь с разрешёнными значениями.
4. `SaveCurrentState` сериализует `model.ParserConfig` обратно в `state.json` — placeholders уезжают, остаются hardcoded значения.

Подтверждено end-to-end на живой установке (см. §5.2).

### 2.3. Покрытие тестами

`core/config/varsubst_test.go` — 17 тестов: nil-safety, hardcoded fallback, custom substituter, mixed формы options, сохранность не-string значений, type coercion (int для tolerance, bool для tun-флагов), missing-file resilience, end-to-end.

### 2.4. Что НЕ затронуто

- Подстановка `@varname` в `template.config.*` — продолжает работать через `ApplyTemplateWithVars`, не трогаем.
- Wizard UI — Settings tab сохраняет vars как раньше, формат state.json не меняется.

## 3. Три бага в самом hotfix v0.8.8.1

После публикации v0.8.8.1 при первом же тестировании выяснилось — `BuildVarSubstituterFromDisk` написан с тремя независимыми ошибками подряд:

| # | Что | Реальность |
|---|---|---|
| 1 | Путь к state.json: `bin/state.json` | На самом деле `bin/wizard_states/state.json` |
| 2 | Формат `settings_vars`: `map[string]string` | На самом деле массив `[{name, value}, ...]` |
| 3 | JSON-ключ блока: `"settings_vars"` / `"SettingsVars"` | На самом деле `"vars"` (см. `WizardStateFile.Vars` в `ui/wizard/models/wizard_state_file.go:72` с тегом `json:"vars"`) |

Все три бага вместе → `loadStateSettingsVars` молча возвращает пустую карту → override-ветка не срабатывает → юзерская кастомизация `urltest_interval = 1m` в Settings игнорируется, всегда подставляется template default `5m`.

Регрессии относительно v0.8.8 не было (там было сломано полностью), и hardcoded fallback покрывал главный симптом (sing-box не падал). Но override-механизм был мёртв.

### Корневая причина — процесс

Я **не сверился с источником истины** при написании каждой из трёх частей:

- Путь — захардкодил из головы, не grep'нул реальный usage.
- Формат — предположил `map`, не открыл реальный state.json.
- JSON-ключ — предположил `settings_vars`, не сверил с `WizardStateFile.Vars` JSON-tag.

И что хуже — **тесты воспроизводили те же ошибки**, что production-код. Я писал fixture'ы под свою же фантазию, тесты были зелёные, но валидировали фикцию.

## 4. v0.8.8.3 — два следующих прохода

> Локально работа велась под именем v0.8.8.2: ветка `fix-v0.8.8.2`, тег v0.8.8.2 был создан, CI запущен и отменён, тег удалён. Публиковать как v0.8.8.2 решили не публиковать после обнаружения третьего бага — чтобы не было путаницы между «v0.8.8.2 broken in main but never released» и «v0.8.8.2 published». Финальная публикация — v0.8.8.3.

### 4.1. Первый коммит (`2363baa`) — invariants + format

Чтобы баг `bin/state.json` vs `bin/wizard_states/state.json` не повторялся (нужно grep'ать три пакета, чтобы найти, где реально лежит файл) — **консолидируем path-инварианты**.

#### Константы → `internal/constants/constants.go`

```go
const (
    WizardTemplateFileName = "wizard_template.json"  // было в ui/wizard/template/loader.go
    WizardStateFileName    = "state.json"            // было в ui/wizard/models/wizard_state_file.go
    WizardStatesDirName    = "wizard_states"         // было в ui/wizard/business/state_store.go
)
```

#### Хелперы → `internal/platform/platform_common.go`

```go
func GetWizardTemplatePath(execDir string) string  // <execDir>/bin/wizard_template.json
func GetWizardStatesDir(execDir string) string     // <execDir>/bin/wizard_states/
func GetWizardStatePath(execDir string) string     // <execDir>/bin/wizard_states/state.json
```

**Контракт:** код, которому нужно найти эти файлы, ОБЯЗАН ходить через хелперы. Никаких больше `filepath.Join(execDir, "bin", "wizard_states", "state.json")` на каждом callsite.

#### Migration

Все callsite-ы переехали:

- `ui/wizard/template/loader.go::LoadTemplateData` — `platform.GetWizardTemplatePath`.
- `ui/wizard/business/state_store.go::NewStateStore` — `platform.GetWizardStatesDir`.
- `core/config/varsubst.go::BuildVarSubstituterFromDisk` — `platform.GetWizardTemplatePath` + `platform.GetWizardStatePath`. Сигнатура изменена с `(binDir)` на `(execDir)`.

Старые константы в ui-слое сохранены как re-exports канонических значений из `internal/constants` для back-compat:

```go
// ui/wizard/business/state_store.go
const WizardStatesDir = constants.WizardStatesDirName

// ui/wizard/models/wizard_state_file.go
const StateFileName = constants.WizardStateFileName

// ui/wizard/template/loader.go
const TemplateFileName = constants.WizardTemplateFileName
```

#### Format fix

`loadStateSettingsVars` переписан — `[]struct{Name, Value string}` вместо `map[string]string`.

Этот коммит исправил **баги #1 и #2**. **Баг #3 остался** (ключ всё ещё `settings_vars` в struct).

### 4.2. Второй коммит (`6fd8a22`) — реальный JSON-ключ

Юзер на ревью указал: ключ — `vars`, не `settings_vars`. Действительно, `WizardStateFile.Vars` имеет тег `json:"vars"`. Я проигнорировал источник истины ещё раз.

```go
var root struct {
    // "vars" is the on-disk key — see WizardStateFile.Vars in
    // ui/wizard/models/wizard_state_file.go (json:"vars"). No legacy
    // aliases: any other key would be a refactor mistake somewhere
    // upstream and deserves to fail loudly, not be silently absorbed.
    Vars []settingVarEntry `json:"vars"`
}
```

**Без legacy fallback** на `settings_vars`/`SettingsVars`: они никогда не существовали на диске; добавлять их = создавать вид исторического формата, которого не было, плюс маскировать будущие баги. Падение должно быть громким.

### 4.3. Защита от регрессии — реальная фикстура

`core/config/testdata/state-real.json` — захвачено из реального `~/Applications/.../wizard_states/state.json`. Минимальное содержимое: `version`, `id`, `vars`.

Новый тест `TestBuildVarSubstituterFromDisk_RealFixture`:

```go
func TestBuildVarSubstituterFromDisk_RealFixture(t *testing.T) {
    fixtureRaw, _ := os.ReadFile("testdata/state-real.json")
    execDir := newTestLayout(t)
    statePath := filepath.Join(execDir, "bin", "wizard_states", "state.json")
    os.WriteFile(statePath, fixtureRaw, 0644)
    // ... template + assertions
}
```

Если кто-то в будущем поменяет JSON-тег и сломает чтение — этот тест упадёт громко, потому что фикстура — реальная, а не выдуманная.

## 5. Архитектура hook'а

### 5.1. Псевдокод

```
GenerateOutboundsFromParserConfig(pc)
   │
   ├─ SubstituteParserConfigPlaceholders(pc, subst)   ← обходит все @varname
   │     └─ для каждого @name:
   │           ├─ subst(name)?                          ← BuildVarSubstituterFromDisk
   │           │     ├─ overrides[name]?                ←   loadStateSettingsVars (state.vars)
   │           │     ├─ defaults.values[name]?          ←   loadTemplateVarDefaults
   │           │     └─ return ok=false
   │           ├─ knownPlaceholderFallback(name)?        ← hardcoded urltest_*
   │           └─ leave literal + WARN log
   │
   └─ далее обычная генерация selector JSON-ов
```

### 5.2. Реальный тест — end-to-end

Симуляция сломанного состояния пострадавших юзеров (на live-инсталле):

1. Установлен v0.8.8.1.
2. В `bin/config.json` и `bin/wizard_states/state.json` через `sed` все `"interval": "5m"` / `"tolerance": 100` / `"url": "..."` заменены на `@urltest_*` плейсхолдеры (18× каждый файл).
3. Перезапуск лаунчера → клик Update в UI.
4. Результат:
   - `config.json` после генерации: 0 placeholders, `interval: "5m"`, `tolerance: 100`, `url: "https://cp.cloudflare.com/generate_204"` — корректно.
   - `state.json`: тоже 0 placeholders (само-исцеление через wizard-save flow).
   - `sing-box check` — exit 0, конфиг валиден.
   - Лаунчер запустился.

## 6. Открытые TODO / debt

1. **Hardcoded fallback на три URLTest-имени — не масштабируется.** Если в будущем в template добавится новый `@varname` в `parser_config.outbounds[]`, и кто-то очистит state.json + удалит wizard_template.json, новый placeholder останется как литерал. Решение для v0.9: пропустить через event-driven config-build (SPEC 045/047), где template — обязательная зависимость генерации.

2. **`tolerance` из state.json не coerce'ится в int.** `coerceVarValue` смотрит только в `intVars` (`tun_mtu`, `mixed_listen_port`, `proxy_in_listen_port`, `urltest_tolerance`). Если кастомный template объявит свою int-переменную, не из этого списка — она пойдёт строкой. На URLTest не влияет — `urltest_tolerance` в списке.

3. **`coerceVarValue` knows only `bool` from template.** Остальные `type` (`text`, `enum`, `text_list`) идут строкой. `text_list` в parser_config.outbounds.options не используется, но если когда-то — нужна логика.

## 7. Уроки

1. **Сверяться с источником истины перед коммитом.** `grep -rn 'state.json'` на 5 секунд показал бы реальный путь. Открыть `wizard_state_file.go` показало бы реальный JSON-tag. Я не сделал ни того, ни другого — три раза подряд.

2. **Тесты должны зеркалировать production-инвариант, а не свою же фантазию.** Тест `TestBuildVarSubstituterFromDisk_StateOverridesTemplate` в v0.8.8.1 был зелёный, потому что использовал тот же неправильный ключ `settings_vars`, что и production-код. **Реальная фикстура** (`testdata/state-real.json`) ловит такие ошибки сразу.

3. **Path-инварианты должны быть в одном месте.** До этого SPEC'а — три пакета держали разные части: `internal/constants/` имя файла шаблона, `ui/wizard/business/` имя поддиректории, `ui/wizard/models/` имя state-файла. Никто не «владел» полным путём `<execDir>/bin/wizard_states/state.json`. Теперь — `platform.GetWizardStatePath`, единственный санкционированный способ.

4. **Hotfix-у — hotfix-овый scope.** v0.8.8.1 пытался решить большую проблему (универсальный @varname substituter) минимальными средствами. Получилось коряво, потребовался v0.8.8.3 (через локальный v0.8.8.2) с двумя итерациями. Если бы это попало в SPEC 045 (state/config decoupling), фикс был бы естественной частью большего рефакторинга, без давления времени.

## 8. Связи

- **Источник бага:** [SPEC 040 — WIZARD_TEMPLATE_OPTION_TITLES](../040-F-C-WIZARD_TEMPLATE_OPTION_TITLES/SPEC.md). Там введены `@urltest_*` плейсхолдеры, но не предусмотрена их подстановка в `parser_config`.
- **Архитектурное решение:** [SPEC 045 — STATE_CONFIG_DECOUPLING](../045-F-N-STATE_CONFIG_DECOUPLING/SPEC.md). После split'а Wizard Save и Build Config, `BuildConfig` станет единственной точкой генерации и сможет применять подстановку в любых местах конфига, а не точечно в parser_config.
- **Релиз:** [docs/release_notes/0-8-8-1.md](../../docs/release_notes/0-8-8-1.md), [docs/release_notes/0-8-8-3.md](../../docs/release_notes/0-8-8-3.md).
- **Файлы:** `core/config/varsubst.go`, `core/config/varsubst_test.go`, `core/config/testdata/state-real.json`, `internal/constants/constants.go`, `internal/platform/platform_common.go`, `internal/platform/platform_common_test.go`, `core/config_service.go`, `ui/wizard/template/loader.go`, `ui/wizard/business/state_store.go`, `ui/wizard/models/wizard_state_file.go`.

## 9. Метрика

| Метрика | Значение |
|---|---|
| Опубликованных релизов на один баг | 2 (v0.8.8.1, v0.8.8.3); v0.8.8.2 — локальный, не достиг публикации |
| Коммитов | 3 (`5a78d9c` v0.8.8.1, `2363baa` v0.8.8.2 attempt, `6fd8a22` v0.8.8.3 fix) |
| Багов в самом hotfix | 3 (путь, формат, ключ) |
| Тестов добавлено | 22 (17 в v0.8.8.1 + 5 helper-тестов в v0.8.8.3 + переписанные fixtures + RealFixture-canary) |
| Изменено файлов | 11 |
| Время от обнаружения до v0.8.8.1 | ~2 часа |
| Время от обнаружения проблем v0.8.8.1 до v0.8.8.2 attempt | ~30 минут |
| Время от первого фикса v0.8.8.2 до фикса JSON-ключа (v0.8.8.3) | ~10 минут (на ревью пользователем) |
