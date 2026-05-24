# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights

- **Template updates now reach you automatically.** Previously when a new launcher version shipped with an updated outbound (new comment, tweaked options, extra group), your existing config kept the stale copy. Now outbound entries in `state.json` are thin references — body lives in the template — so template changes flow through to your config on the next launch without any manual reset. Your custom edits on top still persist as field-level diff.
- **«Restore missing» now revives outbounds correctly.** Deleting `auto-proxy-out` and bringing it back via Restore now properly inherits all active preset filters (e.g. the «no Russian proxies» patch from the Russian domains preset). Previously you had to toggle the preset off/on to re-apply the filter.
- **Edit outbound — URLTest fields are now editable.** For automated (urltest) outbounds — `auto-proxy-out` and similar — Edit dialog now exposes Interval / Tolerance / URL as dropdowns. Pick a preset value or select `@urltest_url` (etc.) to inherit the value you configured in Settings tab.
- **`wizard.*` field removed.** The legacy `wizard: {required: 1}` wrapper in templates is replaced by a top-level `required: true` field. Existing templates with `wizard.required` continue to work via fallback, but new templates should use the top-level form.

### Technical / Internal

- **SPEC 058 STATE_AS_TEMPLATE_DIFF.** Outbound entries in `state.connections.outbounds[]` split into two classes:
  - **Direct** (`ref` absent) — self-contained body, full user ownership.
  - **Referenced** (`ref: "#TEMPLATE#"` or `ref: "<preset_id>"`) — thin shape (only `tag` + `ref` + `updates`), body resolved live from `template.parser_config.outbounds[]` or `template.presets[].outbounds[]` at render/build time.
- USER edits on referenced entries become field-level `OutboundFieldDiff` stored in `updates[]` with `ref: "#USER#"` — always last in the stack, replace-not-append on each Save.
- One-shot migration on first load converts legacy SPEC 057 state (direct entries with snapshotted body) → referenced shape + USER patch with diff against `template + active preset patches` (not raw template — preset edits stay correctly attributed). Backup: `state.json.pre-058.bak`, lossless rollback.
- Sentinel constants `RefTemplate = "#TEMPLATE#"`, `RefUser = "#USER#"` in `core/config/configtypes`. State loader validates positional rules (entry-level vs updates-level).
- New helpers: `core/build/migrate_outbounds_spec058.go`, `core/build/outbound_diff.go`. Resolver expansion in `core/build/resolve_outbounds.go` (3-way classify: direct/template/preset).
- UI: collectRows shows ✏ badge on referenced entries with USER patch. Reset button clears USER patch (not body replace). Edit dialog: form populated via `wizardbusiness.ResolveMergedOutbound` (same pipeline as Preview/build emit). Settings ↔ JSON tab sync handles thin shape correctly.
- **SPEC 060 STATE_NAMESPACE_COLLAPSE.** `core/state/v5/` and `core/state/v6/` subpackages collapsed into unified `core/state/`. `State.RulesV6` → `State.Rules` (~50+ callsites updated). Dual write path (`useV6` gate, `marshalDiskV6` vs `marshalDisk`) removed — Save always writes canonical v6 shape. Wire format on disk unchanged; v5 files still read correctly via `parseV5Legacy`. Legacy DNS shape exposed as `state.LegacyDNSOptionsV5` for UI back-compat. `parseV6` renamed to `parseCurrent`. ~26 import callsites in `core/` and `ui/` updated.

## RU
### Основное

- **Обновления template приходят к тебе автоматически.** Раньше при выходе новой версии launcher'а с обновлённым outbound'ом (новый комментарий, доработанные options, новая группа) твоя сохранённая конфигурация продолжала жить со старой копией. Теперь outbound entry в `state.json` — это тонкая ссылка, body живёт в template'е, и template-обновления приходят к тебе на следующем запуске без manual reset. Твои собственные правки остаются как field-level diff.
- **«Restore missing» теперь правильно восстанавливает outbounds.** Удалил `auto-proxy-out` → восстановил через Restore — теперь сразу подтягивает все активные preset-фильтры (например, фильтр «не использовать российские прокси» от Russian domains preset). Раньше приходилось выключать и обратно включать preset чтобы фильтр применился.
- **Edit outbound — поля URLTest стали редактируемыми.** Для urltest-outbound'ов (`auto-proxy-out` и похожие) в Edit диалоге появился блок «URLTest options» с тремя dropdown'ами: Interval / Tolerance / URL. Можно выбрать preset значение или `@urltest_url` (и т.п.) чтобы наследовать значение из Settings tab.
- **Поле `wizard.*` убрано.** Legacy обёртка `wizard: {required: 1}` в template'ах заменена на top-level `required: true`. Старые template'ы с `wizard.required` ещё работают через fallback, но в новых — используем top-level форму.

### Техническое / Внутреннее

- **SPEC 058 STATE_AS_TEMPLATE_DIFF.** Outbound entries в `state.connections.outbounds[]` поделились на два класса:
  - **Прямые** (`ref` отсутствует) — self-contained body, полное юзерское владение.
  - **Ссылочные** (`ref: "#TEMPLATE#"` или `ref: "<preset_id>"`) — thin shape (только `tag` + `ref` + `updates`), body резолвится live из `template.parser_config.outbounds[]` или `template.presets[].outbounds[]` на render/build time.
- USER правки на ссылочные entries становятся field-level `OutboundFieldDiff` в `updates[]` с `ref: "#USER#"` — всегда последний в стеке, replace-not-append при каждом Save.
- Однопроходная migration на первом load конвертирует legacy SPEC 057 state (прямые entries с snapshot'нутым body) → ссылочный shape + USER patch с diff против `template + active preset patches` (не raw template — preset правки корректно атрибутируются). Backup: `state.json.pre-058.bak`, lossless rollback.
- Sentinel константы `RefTemplate = "#TEMPLATE#"`, `RefUser = "#USER#"` в `core/config/configtypes`. State loader валидирует positional rules (entry-level vs updates-level).
- Новые helpers: `core/build/migrate_outbounds_spec058.go`, `core/build/outbound_diff.go`. Resolver расширен в `core/build/resolve_outbounds.go` (3-way classify: direct/template/preset).
- UI: collectRows показывает ✏ badge для ссылочных entries с USER patch'ем. Reset кнопка чистит USER patch (не replace body). Edit диалог: форма заполняется через `wizardbusiness.ResolveMergedOutbound` (тот же pipeline что Preview/build emit). Settings ↔ JSON tab sync корректно обрабатывает thin shape.
- **SPEC 060 STATE_NAMESPACE_COLLAPSE.** Подпакеты `core/state/v5/` и `core/state/v6/` свёрнуты в единый `core/state/`. `State.RulesV6` → `State.Rules` (~50+ callsite'ов). Dual write path (`useV6` gate, `marshalDiskV6` vs `marshalDisk`) удалён — Save всегда пишет canonical v6 shape. Wire format на диске не меняется; v5 файлы по-прежнему читаются через `parseV5Legacy`. Legacy DNS shape остался доступен как `state.LegacyDNSOptionsV5` (для UI backward-compat). `parseV6` → `parseCurrent`. ~26 import-callsite'ов в `core/` и `ui/` обновлены.
