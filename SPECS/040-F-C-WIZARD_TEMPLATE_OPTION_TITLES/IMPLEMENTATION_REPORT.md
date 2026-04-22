# Реализация: 040 — WIZARD_TEMPLATE_OPTION_TITLES

**Коммиты:** `45730a7` (WizardOption infra), `cfc0634` (URLTest vars). Ветка `night-work`, 2026-04-22.
**Спека написана ретроспективно.**

## Что сделано

### `45730a7` — WizardOption infra

- `TemplateVar.UnmarshalJSON` принимает 3 формы JSON-options:
  - `["5m", "30m"]` — legacy, `OptionTitles == nil`.
  - `[{"title":"5m (default)","value":"5m"}, ...]` — `OptionTitles` заполнен.
  - Mixed (строки среди объектов) — каждый элемент парсится отдельно.
- `OptionTitle(i)` — helper, fallback value.
- Unit-тесты на 4 формы.
- `settings_tab.go` рендерер маппит title↔value через два closure (`titleForValue`, `valueForTitle`) — сейчас используется и в enum, и в text.

### `cfc0634` — URLTest template vars

- В `wizard_template.json` три vars между `auto_detect_interface` и `log_level`:
  - `urltest_url` — `type: "text"`, object-options (4 probe-URL).
  - `urltest_interval` — `type: "text"`, object-options (5 интервалов).
  - `urltest_tolerance` — `type: "text"`, object-options (4 tolerance).
- `auto-proxy-out` использует `@urltest_*` placeholders.
- `isIntCastVar` hoisted + `urltest_tolerance` для integer-cast.

## Что **не** сделано — требует доработки (§4 SPEC)

Выбран неправильный widget для `type: "text"` + object-options: сейчас рендерится `widget.NewSelectEntry` (combo), в котором можно допечатать «ваолрваопорвао» прямо к выбранному пресету — маппинг title→value ломается.

### Нужно доделать

1. **Рендерер**: разделить по `type` (см. SPEC §4.1 и §2.3). `enum` → Select с title'ами, `text` + plain options → SelectEntry, `text` + object-options → запрещено.
2. **Validator**: `template_validate.go` отклоняет `text` + любую пару `title != value`.
3. **Шаблон**: `urltest_interval` и `urltest_tolerance` → `type: "enum"` (с object-options). `urltest_url` → `type: "text"` + plain strings.
4. **Тесты**: unit на `buildSettingsVarRow` (5 сценариев) + unit на validator.
5. **Документация**: `docs/CREATE_WIZARD_TEMPLATE[_RU].md` — описать правило и три формы options.

## Проверка на текущей реализации

- `go test ./ui/wizard/template/` — зелёные (JSON unit-тесты).
- Live-тест `go run /tmp/test_tmpl.go` — template парсится корректно (`options=4/5/4`, `titles=4/5/4`).
- End-to-end в UI: после копии `bin/wizard_template.json` в установленное приложение — URLTest-vars в Settings-табе визарда видны с подписями «Cloudflare (HTTPS, default)», «5m (default)», «100 ms (default)».
- **Баг-симптом**: в том же UI можно допечатать произвольный текст прямо в поле «Cloudflare (HTTPS, default)» и он сохранится. Именно это лечит TODO.
