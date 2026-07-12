# 090-F-N — Общий язык пресетов с LxBox (+ импорт-конвертер)

**Тип:** Feature · **Статус:** N (спека; конвертер — низкоценный, отложен) · **Effort:** M
**Дата:** 2026-07-12 · **Ядро:** не участвует (чистый Go-маппинг форматов).

> Design + adversarial verify (Opus). Вердикт: `needs-fixes`; `spec-only`.
> **Ключевой вывод верификатора:** конвертация 8 bundled LxBox-пресетов **избыточна** — лаунчер
> уже несёт 15 hand-authored пресетов, покрывающих (и богаче) все 8 LxBox. Реальная ценность —
> **импорт произвольного пользовательского LxBox-пресета**, а не re-derive готовых.

## 1. Что уже общее (не трогать)

`#if`-конструкт LxBox — **буквальный порт SPEC 067 десктопного лаунчера** (те же and/or/value/else,
предикаты `#in/#notIn/#matches/#notEmpty/#isEmpty/#not`, режимы map-spread/array-element).
TEMPLATE.md прямо пишет: «Дизайн заимствован у десктопного лаунчера (SPEC 067), подмножество v1».
`@`-плейсхолдеры, плоские `dns_servers`, inline/remote `rule_set` — тоже общие. **Язык выражений
общий, его НЕ трогаем.**

## 2. Расхождения (только обёртка пресета)

| Аспект | LxBox | launcher |
|--------|-------|----------|
| Контейнер пресетов | `selectable_rules[]` | `presets[]` |
| ID | `preset_id` | `id` |
| UI-мета | `ui{label,description,default,locked,pinned}` | плоские `label`/`description`/`default_enabled` |
| DNS-правила | `dns_rules[]` (массив) | `dns_rule` (singular map) |
| var-семантика | `wizard_ui`, magic `dns_server`/`outbound`/`dns_enable` | `type`, `select`, magic те же |
| Прочее | `on_change`, `ref`-vars, `@vpn_mode`-инварианты | нет (частично SPEC 067) |

## 3. Deliverable (честный)

1. **Документ shared-формата** (`core/presetconv/doc.go` + этот SPEC): канонический preset-envelope
   как superset обоих; таблица маппинга ключей; список lossy-полей; что общее.
2. **Импорт-конвертер** `core/presetconv` (LxBox map → `template.Preset`) — **ценность в импорте
   произвольного пользовательского пресета**, НЕ в re-derive 8 bundled. Прогоняется ДО
   `LoadPresets`, его выход валидируется существующим `LoadPresets(raw, globalVarsNames)`.
   - `convert.go` — `FromLxBoxTemplate(raw) ([]template.Preset, []Warning, error)`.
   - `convert_var.go` — var-маппинг (`default_value`→`Default`; `wizard_ui` — lossy note).
   - `if_convert.go` — LxBox инлайновый `enabled:"@var"` на rule_set → launcher `if:["@var"]`.
   - `warning.go` — `Warning{PresetID,Field,Message,Action:"drop"|"lossy"|"note"}`.

## 4. Правки от adversarial verify

1. **Reframe**: ценность — импорт-путь, не 8 bundled. Для паритета — сравнивать выход конвертера
   с **существующими нативными пресетами лаунчера** как golden (доказать эквивалентность), а не
   «добавлять недостающее».
2. **LoadPresets — 2 аргумента**: `LoadPresets(raw, globalVarsNames map[string]bool)`. Round-trip
   тест обязан передавать реальные template-globals и проверять, что имена var не коллидируют.
3. **traffic-processing непереносим**: ссылается на `@vpn_mode` + эмитит `inbounds[]` — у лаунчера
   нет `@vpn_mode`-глобали, inbounds на уровне template. Помечать `note/lossy`, не конвертировать.
   `unknown-traffic` `package_name_regex`/`invert` — тоже отметить как non-portable.
4. **`dns_rules`-plural** у LxBox (ru-direct: force_ipv4-гейт) → launcher `dns_rule` singular
   вынуждает lossy (первый терминальный, force_ipv4 теряется). WARNING + рекомендация сделать
   launcher superset'ом (`Preset.DNSRules []map` — связано с [085.1](../085-F-O-FAKEIP/PLAN_085.1.md)).

## 5. Что launcher должен добавить в формат (чтобы стать superset)

- `Preset.DNSRules []map` (plural) — для точного переноса ru-direct/fakeip (см. 085.1).
- Опц.: `ui.locked`/`ui.pinned` (сейчас `default_enabled` + normalize_pinned).
- `on_change` / `ref`-vars уже частично в SPEC 067.

## 6. DoD

- [ ] doc общего формата + таблица маппинга (реализуемо сразу — низкий риск).
- [ ] (низкий приоритет) `core/presetconv` конвертер + golden-паритет с нативными пресетами +
      round-trip через `LoadPresets(raw, globals)`.
- [ ] UI «Import LxBox preset» (`ui/configurator/import_lxbox.go`) — отдельная сессия.

## 7. Почему конвертер отложен

Верификатор показал: конвертер даёт ценность только на импорте произвольного пользовательского
LxBox-пресета, а 8 bundled лаунчер уже имеет нативно и богаче. Плюс точный перенос требует
`dns_rules`-plural (085.1, тот же SPEC 062 ordering-риск). Поэтому в этой сессии — только
документ shared-формата; конвертер — по запросу, когда появится реальный сценарий импорта.
