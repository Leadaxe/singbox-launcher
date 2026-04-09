# Отчёт: DNS scalars → `vars` (`dns_*`)

**ТЗ:** [SUB_SPEC_DNS_TAB_VARS.md](./SUB_SPEC_DNS_TAB_VARS.md)

## Сделано

- **`bin/wizard_template.json`:** четыре скрытые переменные **`dns_*`**, в **`config.dns` / `config.route`** — литералы **`@dns_*`**; в **`dns_options`** шаблона только **`servers`** и **`rules`**.
- **Код:** миграция старых полей **`dns_options`** → **`SettingsVars`**, загрузка снимка только **servers/rules**, **`ApplyDNSVarsFromSettingsToModel`**, **`SyncDNSModelToSettingsVars`**, **`BuildTemplateConfig`** / **`SyncGUIToModel`** / **`refreshDNSSelectsFromModel`** синхронизируют модель с **`vars`**; **`dns_independent_cache`** в шаблоне — **`type: bool`** (подстановка общей веткой **`substitute`**); **`fillDNSAuxiliaryIfEmpty`** не дублирует поля, если объявлены **`dns_*`**.
- **Документация:** **`docs/WIZARD_STATE.md`**, **`docs/release_notes/upcoming.md`**, статус в **SUB_SPEC**.
- **Тесты:** **`dns_state_test`**, **`dns_settings_vars_test`**, правка **`wizard_dns_test`** / **`wizard_integration_test`**.

## Проверки

- `go test ./ui/wizard/models ./ui/wizard/business ./ui/wizard/template`
- `.\build\build_windows.bat` — успешная сборка

## Граничные случаи

- **`default_domain_resolver_unset`:** флаг модели + удаление **`dns_default_domain_resolver`** из **`vars`**; **`MergeRouteSection`** по-прежнему опускает ключ в **`route`**.
- Шаблон **без** объявлений **`dns_*`:** прежняя логика **`fill`** / **`pick*`** из **`dns_options`** и скелета.
- Идемпотентная миграция: не перезаписывает уже существующие ключи **`vars`**.
