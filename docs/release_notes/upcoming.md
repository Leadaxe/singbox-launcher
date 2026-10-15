# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- **Homebrew on macOS:** `brew install --cask leadaxe/tap/singbox-launcher`. The Cask lives in [Leadaxe/homebrew-tap](https://github.com/Leadaxe/homebrew-tap) and follows stable releases on its own; the install script stays the recommended option.
- **macOS release ZIPs keep the executable bit.** The launcher binary was packed as `0644`, so an app extracted by hand did not start without `chmod +x`.
- **Template language: short `#if` form.** `{"#if": [cond, value, else]}` is now the canonical spelling; the long `{"#and": […], "#value": …, "#else": …}` form is still read indefinitely, and the stock template is rewritten to the short form with byte-identical output. A `#do` operation in a node loop can be conditional: `{"#if": [cond, {"#append": "@node"}]}` skips nodes where the condition is false. Contract 1.1.125, `TEMPLATE_LANG` §4.1 / §6.7.1, decision D-133.
- **DNS rule editor no longer drops `rule_set` references.** Opening "Edit DNS Rule" rebuilt the JSON from the form, whose picker held one value from a fixed list, so `"rule_set": ["user:custom", "user:dpi"]` came back as nothing (or as its first tag). The picker is now a multi-select, keeps tags that are not in the list (a disabled rule, a hand-written tag) and a server that is not enabled, and edits made in the JSON tab carry over to the form instead of being overwritten. Fields of the other rule type are cleared only when you switch the type yourself ([#140](https://github.com/Leadaxe/singbox-launcher/issues/140)).
- **Core builds for linux/386 and linux/riscv64.** "Download core" now finds them for a machine whose daemon has not reported its platform yet, `riscv64` is offered in the machine and Target architecture lists (Target also gains `mips`/`mipsle`), and a Linux machine on 386 or riscv64 without a passport defaults to the procd service recipe, as these builds target OpenWrt. UPX-packed copies (`*.upx.tar.gz`) are never picked by the table.

### Technical / Internal
- The `#if` body is parsed in one place (`core/template/if_form.go`); map-spread, array-element, `EvalIfScalar`, `default_value`, the load validator and `#do` all go through it. Corpus: 28 new cases (`short_*`, `do_if_*`), paired with the long form.

## RU
### Основное
- **Homebrew на macOS:** `brew install --cask leadaxe/tap/singbox-launcher`. Cask живёт в [Leadaxe/homebrew-tap](https://github.com/Leadaxe/homebrew-tap) и сам следует за стабильными релизами; install-скрипт остаётся рекомендуемым способом.
- **В macOS-архивах релиза у бинаря есть бит исполнения.** Бинарь лаунчера упаковывался как `0644`, и распакованное вручную приложение не запускалось без `chmod +x`.
- **Язык шаблонов: короткая форма `#if`.** `{"#if": [cond, value, else]}` — теперь канон; длинная `{"#and": […], "#value": …, "#else": …}` читается бессрочно, штатный шаблон переписан на короткую форму с байт-в-байт тем же конфигом. Операция `#do` обхода узлов может быть условной: `{"#if": [cond, {"#append": "@node"}]}` пропускает узлы, где условие ложно. Контракт 1.1.125, `TEMPLATE_LANG` §4.1 / §6.7.1, решение D-133.
- **Редактор DNS-правил больше не теряет ссылки `rule_set`.** «Edit DNS Rule» при открытии пересобирал JSON из формы, где выбор набора был одним значением из фиксированного списка, и `"rule_set": ["user:custom", "user:dpi"]` превращался в пустоту (или в первый тег). Теперь выбор множественный, теги не из списка (выключенное правило, ручной ввод) и невключённый сервер сохраняются, а правки на вкладке JSON переносятся в форму, а не затираются ею. Поля другого типа правила вычищаются только при ручной смене типа ([#140](https://github.com/Leadaxe/singbox-launcher/issues/140)).
- **Сборки ядра linux/386 и linux/riscv64.** «Download core» находит их и для машины, чей демон ещё не сообщил платформу; `riscv64` есть в выборе архитектуры машины и вкладки Target (там же добавлены `mips`/`mipsle`); Linux-машина на 386 или riscv64 без паспорта по умолчанию получает рецепт службы procd — эти сборки под OpenWrt. UPX-копии (`*.upx.tar.gz`) таблица не выбирает.

### Техническое / Внутреннее
- Тело `#if` разбирается в одном месте (`core/template/if_form.go`); map-spread, элемент массива, `EvalIfScalar`, `default_value`, валидатор загрузки и `#do` идут через него. Корпус: 28 новых кейсов (`short_*`, `do_if_*`), парных с длинной формой.
