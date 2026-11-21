# TASKS 183

- [x] T1 `core/template/var_sections.go` (новый): `TemplateSection`, `DecodeVarContainer` — обе формы, отказ при обеих сразу, проверки `id` / `name` / `separator` в разделе; `TemplateVar.Section`
- [x] T2 `core/template/loader.go`: загрузка через `DecodeVarContainer`, `TemplateData.Sections`
- [x] T2a `core/template/template_validate.go`: контекст ошибок переменной из раздела — `sections[<id>].vars[<name>]`; запрет ссылки вперёд в `default_value.#if` (SPEC 097) работает по развёрнутому списку без правок
- [x] T3 `core/config/varsubst.go`: чтение объявлений через `DecodeVarContainer`; греп остальных читателей корневого `vars`
- [x] T4 Тесты `core/template`, читающие поставляемый шаблон своей структурой, — на `DecodeVarContainer`
- [x] T5 `ui/configurator/tabs/settings_tab.go`: заголовок и описание раздела, пустой раздел не рисуется, плоская форма как раньше
- [x] T6 `bin/wizard_template.json`: `sections[]` по таблице SPEC §2.3, без разделителей, подписи Sniff guard без префикса; `tplfmt -w`
- [x] T7 Контракт: `TEMPLATE_LANG.md` §1, §2, §7, §8.1, §9, D-151; `VERSION`; changelog `README.md`; `TASKS_LXBOX.md` §176
- [x] T8 Корпус `template/grammar/sections_container`, `sections_and_vars_rejected`; раннер по имени
- [x] T9 `docs/WIZARD_TEMPLATE.md` §1, §3, §3.2 (текст устарел: `#if` в умолчании может ссылаться на переменные, объявленные выше, ссылка вперёд — ошибка загрузки), §3.4, §8.1, §9; `docs/TROUBLESHOOTING.md` + `.ru.md` (названия строк Sniff guard); `docs/ARCHITECTURE.md` при необходимости; `docs/release_notes/upcoming.md`
- [x] T10 `go build ./...`; по имени — корпус grammar, эталоны конфига шаблона, `TestWizardTemplateFormatted`
- [ ] T11 Показ Settings владельцу: раскладка и названия разделов; CI по `ci.yml`
