# Задачи: 040 — WIZARD_TEMPLATE_OPTION_TITLES

## Этап 1 — парсер + subst (done)

- [x] `TemplateVar.UnmarshalJSON` — три формы (plain, object, mixed).
- [x] `OptionTitles []string` + `OptionTitle(i int) string` helper.
- [x] `isIntCastVar()` hoisted, `urltest_tolerance` добавлен.
- [x] Unit-тесты на парсинг (4 формы + empty-title fallback).

## Этап 2 — URLTest vars в template (partial)

- [x] Три vars добавлены в `bin/wizard_template.json` между `auto_detect_interface` и `log_level`.
- [x] Шаблон `auto-proxy-out` использует `@urltest_url / @urltest_interval / @urltest_tolerance`.
- [ ] **TODO:** сменить типы: `url` остаётся `text`+plain, `interval` и `tolerance` → `enum`+object-form.

## Этап 3 — UI рендерер (needs redesign)

- [x] `text` + options рендерится как `SelectEntry` (combo).
- [ ] **TODO:** разделение по `type`, см. SPEC §4.1 — `text`+options → SelectEntry (только plain); `enum`+options → Select (title'ы).

## Этап 4 — Validator (TODO)

- [ ] **TODO:** в `template_validate.go` — правило «text + options с title != value = error».
- [ ] **TODO:** unit-test validator'а.

## Этап 5 — Документация

- [ ] **TODO:** docs/CREATE_WIZARD_TEMPLATE.md — обновить секцию про `options` + новую форму.
- [ ] **TODO:** docs/CREATE_WIZARD_TEMPLATE_RU.md — то же.
- [ ] **TODO:** release-notes upcoming.md — упомянуть правило и URLTest vars.
