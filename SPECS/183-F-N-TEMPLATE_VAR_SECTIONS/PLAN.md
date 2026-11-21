# PLAN 183

## 1. Загрузка (`core/template`)

- `vars_resolve.go`: `TemplateSection {ID, Name, Description, Chapter string}`; у `TemplateVar` — поле `Section string` с `json:"-"` (id раздела, пусто у плоской формы).
- Новый файл `var_sections.go`: единственная функция разбора контейнера — `DecodeVarContainer(raw []byte) (vars []TemplateVar, sections []TemplateSection, err error)`. Читает корневые `sections` и `vars`; обе формы сразу — ошибка; из `sections` разворачивает плоский список, проставляя `Section`; проверяет `id` (непустой, kebab-case, уникальный), непустой `name`, отсутствие `separator` внутри раздела; `chapter` по умолчанию `core`.
- `loader.go`: корневая структура перестаёт читать `vars` сама и зовёт `DecodeVarContainer`; `TemplateData` получает `Sections []TemplateSection`.
- `template_validate.go`: `validateVarsSeparator` остаётся для плоской формы; тексты ошибок раздела — в `DecodeVarContainer`.
- `core/config/varsubst.go:171`: свой разбор корневого `vars` заменить вызовом `DecodeVarContainer`.
- Тестовые структуры с `json:"vars"`, читающие поставляемый шаблон (`target_test.go`, `tun_address_test.go`, `contract_template_test.go`, `auto_route_test.go`, `gate_migration_test.go`), перевести на `DecodeVarContainer`. Фрагменты корпуса остаются плоскими и читаются той же функцией.

Остальной код (`substitute.go`, `target.go`, `dns_server_form.go`, `vars_resolve.go`, сборка) работает с плоским `Vars` и не меняется. Win7: без `slices`/`maps`/builtin `min`/`max`.

## 2. Settings (`ui/configurator/tabs/settings_tab.go`)

- В цикле отрисовки (сейчас ветка `vd.Separator` → `settingsSeparatorBlock()`): на смене `vd.Section` заголовок откладывается и добавляется перед первой реально нарисованной строкой раздела — после проверок `settingsVarVisible` и `VarConditionIsTargetOnly`. Раздел без нарисованных строк не оставляет ничего.
- `settingsSectionHeader(sec)`: `settingsSeparatorBlock()` (перед первым разделом линии нет), заголовок жирным, описание — `widget.Label` с `Wrapping = fyne.TextWrapWord` и `LowImportance`. Метка без переноса раздувает минимальную ширину окна — описание только с переносом.
- Тексты разделов мимо `locale.T` (приходят из шаблона). Новых ключей локали нет.
- Плоская форма (`Section == ""`): прежний путь с разделителями.

## 3. Шаблон и документы

- `bin/wizard_template.json`: переложить в `sections[]` по таблице SPEC §2.3, убрать разделители, укоротить четыре подписи Sniff guard; затем `go run ./tools/tplfmt -w bin/wizard_template.json` (формат общий, сам `tplfmt` не меняется). Перед перекладкой проверить, что перенос `cert_store`, `log_level`, `mtu_align`, `route_final` не ломает порядок зависимостей умолчаний (`default_value` с `#if` на другие переменные).
- `docs/WIZARD_TEMPLATE.md`: §1 (корневая таблица), §3 — раздел про `sections[]`, §3.4 — разделитель только в плоской форме, §8.1 — раскладка раздела, §9 — новые ошибки загрузки.
- `docs/TROUBLESHOOTING.md`, `.ru.md`: раздел про detour — новые названия строк.
- `docs/ARCHITECTURE.md` — если упоминает плоский `vars[]`; `docs/release_notes/upcoming.md` (EN + RU).

## 4. Контракт

- `contract/docs/TEMPLATE_LANG.md`: §1 — строка «объявления переменных»; §2 — новый подраздел «Контейнер объявлений» (норма SPEC §1); §7 — `separator` как расширение desktop плоской формы; §8.1 — снять (переехало в ядро), `chapter` описать как необязательное поле ядра; §9 — строка C9; таблица решений — D-151.
- `contract/corpus/template/grammar/`: `sections_container` (шаблон в форме `sections`, результат подстановки тот же, что у плоского), `sections_and_vars_rejected` (`"load": "reject"`), `sections_separator_rejected` (`"load": "reject"`; у LxBox разделителя нет — проверить, что кейс для них осмыслен, иначе оставить в Go-тесте лаунчера).
- `contract/VERSION`, changelog в `contract/README.md`, `contract/TASKS_LXBOX.md` §176: форма у LxBox уже такая; от них — отказ загрузки при обеих формах, прогон новых кейсов, подтверждение списка общих `id`.
- `registry/**` не меняется.

## 5. Проверки

- `go build ./...`.
- По имени: раннер корпуса шаблона (grammar), эталонные тесты конфига из шаблона, `TestWizardTemplateFormatted`.
- Новых тестов, кроме кейсов корпуса, нет. UI показывается владельцу.
- Полный прогон — CI: `gh workflow run ci.yml --ref develop -f run_mode=tests`.

## Риски

- **Порядок умолчаний.** Перенос переменной в другой раздел меняет её место в однопроходном разрешении. Страхуют эталонные тесты конфига.
- **Второй читатель формы.** Любой код, читающий корневой `vars` из сырого JSON мимо `DecodeVarContainer`, на новом шаблоне молча получит пустой список. Перед закрытием — греп по `json:"vars"` и `"vars"` во всём репозитории.
- **Длинное описание раздела** без переноса раздувает окно — см. п. 2.
