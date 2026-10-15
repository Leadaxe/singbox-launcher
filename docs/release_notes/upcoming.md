# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- **Homebrew on macOS:** `brew install --cask leadaxe/tap/singbox-launcher`. The Cask lives in [Leadaxe/homebrew-tap](https://github.com/Leadaxe/homebrew-tap) and follows stable releases on its own; the install script stays the recommended option.
- **macOS release ZIPs keep the executable bit.** The launcher binary was packed as `0644`, so an app extracted by hand did not start without `chmod +x`.
- **Template language: short `#if` form.** `{"#if": [cond, value, else]}` is now the canonical spelling; the long `{"#and": […], "#value": …, "#else": …}` form is still read indefinitely, and the stock template is rewritten to the short form with byte-identical output. A `#do` operation in a node loop can be conditional: `{"#if": [cond, {"#append": "@node"}]}` skips nodes where the condition is false. Contract 1.1.125, `TEMPLATE_LANG` §4.1 / §6.7.1, decision D-133.

### Technical / Internal
- The `#if` body is parsed in one place (`core/template/if_form.go`); map-spread, array-element, `EvalIfScalar`, `default_value`, the load validator and `#do` all go through it. Corpus: 28 new cases (`short_*`, `do_if_*`), paired with the long form.

## RU
### Основное
- **Homebrew на macOS:** `brew install --cask leadaxe/tap/singbox-launcher`. Cask живёт в [Leadaxe/homebrew-tap](https://github.com/Leadaxe/homebrew-tap) и сам следует за стабильными релизами; install-скрипт остаётся рекомендуемым способом.
- **В macOS-архивах релиза у бинаря есть бит исполнения.** Бинарь лаунчера упаковывался как `0644`, и распакованное вручную приложение не запускалось без `chmod +x`.
- **Язык шаблонов: короткая форма `#if`.** `{"#if": [cond, value, else]}` — теперь канон; длинная `{"#and": […], "#value": …, "#else": …}` читается бессрочно, штатный шаблон переписан на короткую форму с байт-в-байт тем же конфигом. Операция `#do` обхода узлов может быть условной: `{"#if": [cond, {"#append": "@node"}]}` пропускает узлы, где условие ложно. Контракт 1.1.125, `TEMPLATE_LANG` §4.1 / §6.7.1, решение D-133.

### Техническое / Внутреннее
- Тело `#if` разбирается в одном месте (`core/template/if_form.go`); map-spread, элемент массива, `EvalIfScalar`, `default_value`, валидатор загрузки и `#do` идут через него. Корпус: 28 новых кейсов (`short_*`, `do_if_*`), парных с длинной формой.
