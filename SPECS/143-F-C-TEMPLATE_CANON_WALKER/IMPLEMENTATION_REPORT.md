# SPEC 143 — единый канонический обходчик шаблона — отчёт о реализации

Дата закрытия: 26.09.2026. Ветка `develop`, последний кодовый коммит — `f7773859`.

## Цель

Одна реализация языка шаблонов на desktop: `@var` / `#if` во всей сборке —
главный конфиг, `on_change.set`, тела пресетов, шаблонные DNS-серверы —
подставляет канонический обходчик `core/template/substitute_canon.go` по
правилам `contract/docs/TEMPLATE_LANG.md`. Предупреждения шаблона доходят до
«Итога» отчёта сборки, а не только до лога. Локальные копии правил
(`isSpliceable`, `isIntCastVar`, `intCastVarNames`, `knownWalkerDivergences`,
`dropUnsetServerPlaceholders`) и второй (legacy / Strict) обходчик удалены.

## Волны

| Волна | Коммит(ы) | Контракт | Что сделано |
|-------|-----------|----------|-------------|
| 0 | `a442b643` | 1.1.68 | Норматив: TEMPLATE_LANG §2.1 (`options` при любом типе кроме `bool`, `options_open`), §2.2 (`enum` = `text` + закрытые `options`, каст только по `type`, C5 закрыт), §3 (без коллапса `["@name"]`), §4.4 (сплайс ветки-массива на один уровень, вложение `[[...]]`), §7.2 (`@runtime.*` в значениях; mobile — объявлено/null → Dropped), §9 (C5, N5). Корпус +8 кейсов, `vars.json` без пометок C5, DECISIONS D-123…D-127, TASKS_LXBOX §64. |
| 1 | `842c048a` | — | Канон: сплайс любого массива в ветке условного элемента (`isSpliceable` снят), `TemplateWarning{Code, Params}` с дедупом по (код, параметры), `SubstituteVarsInJSONCanonWarnings`; `SubstituteVarsInJSONCanon` — обёртка с кодами для корпуса; Go-тест параметров `TestCanonTemplateWarningParamsAndSplice`. |
| 0+1 хвосты | `1556af23` | — | После слияния волн 0 и 1: `int` с объектными `options` не сводится к `enum` (кейс `types/int_with_options_stays_int` зелёный, N5 закрыт для `int`), страж паритета учёл два новых расхождения legacy до его снятия волной 2, Т23 SPEC уточнён (`runtime.*` на mobile). |
| 2 | `984b6246`, `bf0d2540` | 1.1.69 | Главный конфиг и `on_change.set` на канон: `ApplyTemplateWithVarsForWarnings` / `GetEffectiveConfigForWarnings` (старые сигнатуры — обёртки), `EvalIfScalar` на канон. Валидатор пускает `@runtime.platform/arch/target` в значениях `config`; неизвестное поле `@runtime.*` и bare-форма в предикатах — отказ. `build.Result.TemplateWarnings`, вид `BuildReportTemplateDegraded` (`template_degraded`) первым в «Итоге», `core.FeedBuildReportFromTemplate` в `core/rebuild.go` и `ui/configurator/business/create_config.go`. Страж корпуса через боевой путь, `knownWalkerDivergences` удалён. Golden real-v088 — адрес TUN массивом (R3). Поле `go` у четырёх кодов `template_*` в `warnings.json`. Хвосты (`bf0d2540`): строка «Итога» без дубля субъекта, `on_change.set` не пишет плейсхолдер необъявленного имени, `params[].value` отвергает неизвестное поле `@runtime.*` как и `config`. |
| 3 | `06776e64`, `1c08927f`, `5cdf76dd` | 1.1.70 | Сначала эталоны `config.json` штатного шаблона от develop (8 наборов × 4 таргета, все пресеты включены; `TestWizardTemplateConfigUnchanged`, `core/build`). Затем тела пресетов (`substitutePresetBody` видит все `td.Vars` + переменные пресета; пустая глобаль даёт Dropped ключа) и шаблонные DNS-серверы (`substituteTemplateDNSServer`) на канон. Гейты после Dropped в одном месте `core/build/preset_expand.go`: правило без `outbound`/`action`, DNS-правило без `server`/`action`, `rule_set` без источника, DNS-сервер адресного типа без `server`, правило без условий → новый код `template_fragment_dropped {owner, kind, reason}`. Предупреждения пресетов и DNS — в `Result.TemplateWarnings`. Удалены `SubstituteVarsInJSON`, `SubstituteVarsInJSONStrict`, `UnresolvedVarError`, legacy-обходчик, `dropUnsetServerPlaceholders`. Эталон `resolve_strategy_unset` (`5cdf76dd`). |
| 4 | `9b40e9a4`, `1470e45f` | — | `TemplateVar.OptionsOpen`; `enum` у переменных шаблона на загрузке сводится к `text` с закрытыми `options`; объектные `options` тип не меняют. Валидатор отвергает `options` у `bool`. `isIntCastVar` и `intCastVarNames` удалены: общее `template.IsIntVarType` + `template.CastIntValue` (clamp [0, 65535], не-число строкой) для канона и для подстановки `parser_config` (`core/config/varsubst.go`) по типу из шаблона. Шаблон: `tun_mtu`, `proxy_in_listen_port`, `urltest_tolerance` → `int`, `urltest_url` → `options_open`. Settings: список по `options`, комбобокс при `options_open`, `int` проверяется `numberEntryValidator` (и в поле, и в комбобоксе). Golden real-v088 объявляет те же три переменные `int`. `1470e45f`: `options_open` без `options` игнорируется (толерантность §1), а не отвергается. |
| попутно | `f7773859` | — | Пресетный DNS-сервер, снятый своим `#enable`, больше не попадает в конфиг: гейт считается на переменных вместе с глобалями шаблона, как у тел пресета; `InactiveReason` «enable=имя»; эталоны `globals_off` без `russian:yandex_doh/yandex_dot`. |
| 5 | этот коммит | 1.1.71 | Доки: `docs/ARCHITECTURE.md` (один обходчик, канал предупреждений в отчёт), `ARCHITECTURE_PACKAGES(.ru)`, `DATA_FLOW(.ru)`, `TEMPLATE_REFERENCE(.ru)`, `WIZARD_TEMPLATE(.ru)` (типы, `options`/`options_open`, без списка числовых имён), `TEMPLATE_LANG.md` (разрыв C4 описан закрытым), `upcoming.md`, `RELEASE_NOTES.md`, TASKS_LXBOX §67, папка → `143-F-C-…`. |

## Отклонения от PLAN

- Поле `go` у кодов шаблона в `warnings.json` переписано в волне 2, а не 0 —
  как PLAN и допускал (страж `TestRegistryWarningCodesAreActuallySet`).
- Выпавший фрагмент получил собственный код `template_fragment_dropped`
  (контракт 1.1.70) вместо `ExpandWarning` с существующим кодом: подходящего
  кода в реестре не было. Добавлен пятый гейт — правило без условий.
- Строка `template_degraded` в «Итоге» рендерится текстом `Reason` без
  субъекта (`case BuildReportTemplateDegraded: return e.Reason`), а не
  `Subject` из `name`/`key`: имя уже есть в тексте кода, иначе дубль.
- Сведение `enum` → `text` сделано в `vars_resolve.go` только для переменных
  шаблона, не в `canonicalVarType`: у переменных пресетов `enum` пока живёт
  (см. «Открыто»).
- `wantsIntCast` не переписан, а удалён: вместо него `template.IsIntVarType` +
  `CastIntValue`, общие для канона, `varsubst.go` и Settings.
- Числовая валидация `int` в Settings (была в подзадаче 143.1) сделана в
  волне 4; в 143.1 остался только UI мультивыбора.
- Между волнами 2 и 3 добавлен отдельный коммит эталонов (`06776e64`), чтобы
  волна 3 сверялась с develop побайтно.

## Решения по развилкам (закрыты в ходе реализации)

- Ошибка в возврате Canon-функций оставлена (`…Warnings` возвращают `error`
  на невалидный JSON входа).
- Предупреждения сортируются детерминированно (`SortTemplateWarnings`: по
  коду, затем по параметрам) — порядок в «Итоге» стабилен между сборками.
- Строка «Итога» — без субъекта, см. выше.
- `on_change.set` не пишет плейсхолдер `"@name"` в состояние, если ветка
  ссылается на незаявленную переменную: значение цели не меняется, код — в лог.
- Валидатор отвергает неизвестное поле `@runtime.*` и в `config`, и в
  `params[].value`.
- `options_open` без `options` игнорируется (толерантность §1), не ошибка
  загрузки.
- Новый код назван `template_fragment_dropped`, параметры `{owner, kind, reason}`.
- Эталон `resolve_strategy_unset`: правило `resolve` без `strategy` остаётся
  (SPEC §6 п.5), а не выпадает.

## Открыто для владельца (не блокирует)

- Пустая строка в главном конфиге остаётся `""`, а не Dropped: Dropped даёт
  только объявленная переменная без значения; для текстовой переменной с
  пустой строкой поведение прежнее.
- Предупреждения `core/config/varsubst.go` (подстановка `parser_config`) и
  `on_change.set` идут только в лог: при правке в UI сборки нет, а
  `parser_config` — отдельный движок вне канала отчёта.
- `enum` у переменных ПРЕСЕТОВ не сведён к `text` (тип читается как раньше).
- Пользовательские шаблоны, где MTU / порт объявлены `type: text`, теперь
  подставляют строку (списка зашитых имён больше нет) — отмечено в
  `upcoming.md` и `RELEASE_NOTES.md`.
- Golden real-v088 получили `type: int` у трёх переменных, чтобы эталон
  отражал штатный шаблон.
- Подзадача 143.1 — UI мультивыбора для `text_list` + `options` (сейчас
  многострочное поле).

## Проверки

- CI волн 0–4 зелёный: `ci.yml` (`run_mode=tests`) и `contract.yml`.
- Golden `TestWizardTemplateConfigUnchanged` (`core/build`) страхует волны 3–4
  и попутный фикс: `config.json` штатного шаблона меняется только там, где
  это решение задачи (эталоны обновлены осознанно в `5cdf76dd`, `f7773859`).
- Волна 5 локально: `go build ./...`; `TestRegistryWarningCodesAreActuallySet`
  (`core/config/subscription`) и `TestContractCorpusTemplate` (`core/template`)
  по имени — зелёные; `l10n_check --strict` — 0 ошибок.
  Полный прогон CI после слияния волны 5 — `gh workflow run ci.yml --ref develop -f run_mode=tests`.
