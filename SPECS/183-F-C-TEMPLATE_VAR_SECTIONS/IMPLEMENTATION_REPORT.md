# IMPLEMENTATION REPORT 183

**Итог:** переменные шаблона объявляются в корневом `sections[]` в форме LxBox (контракт 1.1.174, D-151); вкладка Settings рисует заголовки разделов; плоский `vars[]` читается как раньше. 21.11.2026.

## Что сделано

- `core/template/var_sections.go` (новый): `TemplateSection`, `DecodeVarContainerRaw` и `DecodeVarContainer` — единственное место, знающее форму контейнера. Проверки: обе формы сразу, `id` (непустой, kebab-case, уникальный), непустой `name`, наличие `vars`, запрет `separator` внутри раздела; `chapter` по умолчанию `core`. `TemplateVar.Section` — id раздела.
- `core/template/loader.go`: объявления берутся через `DecodeVarContainer`; `TemplateData.Sections`.
- `core/template/template_validate.go`: адрес переменной в тексте ошибки — `sections[<id>].vars[<name>]`, у плоской формы прежний `vars[N]`. Запрет ссылки вперёд в `default_value.#if` (SPEC 097) работает по развёрнутому списку без правок.
- `core/config/varsubst.go`: свой разбор корневого `vars` заменён на `DecodeVarContainerRaw`. Других читателей корневого `vars` в коде нет (греп по `json:"vars"` и `["vars"]`).
- `ui/configurator/tabs/settings_tab.go`: `settingsSectionHeader` — линия (кроме первого раздела), заголовок жирным, пояснение приглушённым текстом с переносом; заголовок добавляется перед первой нарисованной строкой раздела, пустой раздел не рисуется. Плоская форма — прежний путь с разделителями. Новых ключей локали нет.
- `bin/wizard_template.json`: десять разделов по таблице SPEC §2.3, разделителей нет; подписи четырёх строк Sniff guard — «TLS method», «QUIC method», «Only nodes with a masked SNI», «Remember relays across restarts». Набор переменных, типы и умолчания прежние (48 объявлений).
- Контракт 1.1.174: `TEMPLATE_LANG.md` §1, §2.3 (новый), §7.5, §8.1, D-151; `VERSION`; changelog `README.md`; `TASKS_LXBOX.md` §176; корпус `template/grammar` +2 (`sections_container`, `sections_and_vars_rejected`), строка в `corpus/template/README.md`.
- Тесты, читавшие поставляемый шаблон своей структурой с `json:"vars"` (`core/template`: target, tun_address, auto_route, tun_route_exclude, gate_migration, раннер корпуса; `core/build`: golden, эталон конфига), переведены на `DecodeVarContainer`. Раннер корпуса считает отказ разбора контейнера вердиктом рубежа загрузки.
- Документация: `docs/WIZARD_TEMPLATE.md` и `.ru.md` (§1, §3, §3.2, §3.4, новый §3.5, §8.1, §9), `docs/TROUBLESHOOTING.md` и `.ru.md` (названия строк Sniff guard), `docs/ARCHITECTURE.md`, `docs/release_notes/upcoming.md`.

## Проверено

- `go build ./...`.
- По имени: `TestWizardTemplateConfigUnchanged` — эталоны конфига совпали без правки эталонов; `TestContractCorpusTemplate`, `TestWalkerParityAgainstCorpus`, `TestContractCorpusDeps`; `TestWizardTemplateFormatted`; тесты `core/template`, читающие поставляемый шаблон; golden-тесты `core/build`; тесты `varsubst`.
- Разово, без теста в репозитории: плоский шаблон до перекладки грузится (55 записей с разделителями, разделов нет); тексты шести ошибок загрузки раздела.

## Отступления от SPEC и PLAN

- Номер нового подраздела нормы — §2.3 (в постановке номер не назывался).
- Кейса корпуса `sections_separator_rejected` нет: разделитель — расширение лаунчера, LxBox записи без `name` пропускает; в `TASKS_LXBOX` §176 это оговорено.
- Описания разделов: `sniff-guard`, `network`, `general`, `tun`, `dns`, `quic` — текст LxBox; у `dpi-bypass` взято только первое предложение (остальное у LxBox — про переопределение на сервере и папке), у `auto-proxy` — «URLTest tuning for the auto groups» (значок группы у LxBox свой); `proxy-in` и `clash-api` — свои.

## Доработки по показу владельцу (21.11.2026)

- Раздел рисуется островом: строки в рамке `fynewidget.NewFramedPanel`, шапка над ним; пояснение — мелким шрифтом обычным цветом (приглушённый читался как погашенная строка). Линии между разделами убраны.
- Колонка под полосу прокрутки справа на всю вкладку (`components.NewScrollGutter`) — полоса не ложится на кнопки сброса.
- Многострочное поле стоит под названием на всю ширину, кнопка сброса — на строке названия (`settingsMultilineRow`).
- Колесо над полем ввода двигает вкладку: `components.WheelThrough` кладёт на строку невидимый слой-Scrollable (у каждого `widget.Entry` своя внутренняя прокрутка, она забирала событие). Многострочное поле забирает колесо, только когда в нём строк больше, чем видно (`settingsMultilineRows` = 3).
- Попутно, вне SPEC, по словам владельца: окно DNS Rules Preview — на `fynewidget.JSONView`; на вкладке DNS кнопки списка правил перенесены в шапку, правка правил общим JSON убрана.

## Проверено дополнительно

- CI `ci.yml` (`run_mode=tests`), macOS / Linux / Windows — зелёный: прогон 44943147520 (f41b2ee6) и 44944518306 (df335d63).
- Settings принят владельцем на живой сборке.

## Не проверено

- Сторона LxBox: отказ загрузки при обеих формах и два кейса корпуса.
