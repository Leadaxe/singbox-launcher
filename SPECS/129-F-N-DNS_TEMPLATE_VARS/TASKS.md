# SPEC 129 · TASKS — волна лаунчера

Одна ветка (`feat/spec129-record-vars`), одна волна по SPEC §8 и §12. Отметки
ставятся по факту; адреса нового кода — `CODEMAP.md`.

## Слои

- [x] **Шаблон.** `NormalizeDNSOptions` без склейки: объявления по тегу
      (`TemplateData.DNSServerVars`), тело держит локальные `@name`;
      `loader.go` не дописывает их в `root.Vars`; валидация объявлений каждого
      сервера и Н11 (плейсхолдер тела объявлен сервером или шаблоном).
- [x] **Состояние.** `DNSServer.Vars`; копии (`CloneDNSServer`,
      `NodeSections.Clone`), поштучные сборщики (`migration_v7_to_v8.go`,
      `node_sections_convert.go`, legacy-вход, UI-синк); инвариант в заголовке
      `dns_options.go`.
- [x] **Нормализация.** `core/state/record_vars.go`: объявления
      (`RecordVarDecls`), перенос корневых `dns_<tag>_<var>` (Н8, самый длинный
      тег по объявленным парам), Н2–Н4 для `dns.servers[kind=template].vars` и
      `rules[kind=preset].vars`, снятие записи-сироты; мост
      `template.RecordVarDeclsFor`.
- [x] **Сборка.** Подстановка тела шаблонного сервера по объявлениям сервера и
      `vars` записи (локальные поверх глобальных, «не задано» → ключ выпадает);
      `buildContextFromState` — перенос в памяти; вторая линия fail-closed
      (Н10): сервер с висячим detour выпадает, правило → `reject`, `final` →
      заглушка, резолверы (`dns`, `route`, `outbounds`, `endpoints`) → замена;
      секция `dns` собирается первой; раннер корпуса DNS на `state[].vars`.
- [x] **UI.** Модель `DNSTemplateVars`; загрузка (перенос ДО фильтра сирот),
      сохранение, превью; окно сервера на `vars` записи (хранилище строки —
      параметром `buildSettingsVarRow`, выбор и сброс умолчания снимают ключ,
      построение строки модель не пачкает); подпись строки списка — по записи;
      пресеты: строка правила и диалог снимают умолчание; пункт 7
      `root_name_refs.go`.
- [x] **Бэкап.** Опции с объявлениями (UI, debug API, корпус); декодеры 1.0 и
      0.x (Н8 в файле, `DNSRef.Vars`, `vars` у чужого вида —
      `backup_unknown_field`); слияние §5.2 (наложение по именам,
      `backup_dns_entry_skipped`); Н9 для записей шаблона и `user`; Н2/Н4 для
      пресетов; экспорт нормализует копию; пять имён из `portable_vars.go`;
      `Warning.Record`; тексты UI и `bin/locale/ru.json`.
- [x] **Debug API.** `PATCH /state/dns` (вид `vars`, нормализация),
      `PATCH /state/rules` (нормализация), экспорт/импорт с объявлениями, поля
      `record`/`reason` в предупреждениях.
- [x] **Контракт.** `backup.schema.json` (`dnsServer.vars` обе стороны),
      `registry/vars.json` (пять имён — терпимая форма чтения),
      `registry/backup_warnings.json` (параметры), `BACKUP.md`,
      `TEMPLATE_LANG.md` N8, `ONE_NAMESPACE.md`, `NODE_LINK.md` §6, DECISIONS
      (новый D, номер сверить перед слиянием), `VERSION` 1.0.2 + строка README,
      корпус (`v10_dns_template_vars` + фикстура, `v10_lxbox_fields`,
      `non_portable_var_skipped`, DNS `nested_vars_substituted`, README
      корпусов), `TASKS_LXBOX.md ## 18`.
- [x] **Заметки.** `docs/release_notes/1-6-0.md` EN/RU, `CHANGELOG.md`.
- [x] **Проверки.** Тесты §12.3 (падают на старом коде); стенд livecmp на
      копии живых данных: config байт-в-байт, круги импорта (в копию и в
      пустое); гейт `gofmt -l .`, `go build`, `go vet`, `go test -count=1 ./...`;
      go1.20-греп.

## Ловушки сверх SPEC §12.1

| # | Ловушка | Что делать |
|---|---|---|
| Т1 | Порядок секций берётся из шаблона: `route`/`outbounds` могут идти раньше `dns` | итог второй линии считать ДО обхода секций, а не по ходу |
| Т2 | `repairDanglingDNSRefs` общая для слияния пресетов и второй линии | fail-closed только для тегов, выпавших второй линией: у конфигов без линии 2 ни байта не меняется |
| Т3 | Перенос корневых имён по одному: запись, получившая первое имя, выглядела бы «со своими vars» для второго | «у записи непустой vars» — снимок ДО переноса |
| Т4 | `checkWarningReasons` сверяет множество причин | в ожиданиях — различные причины, без повторов |
| Т5 | Предупреждение `backup_var_skipped` без `reason` валит сверку причин | причина у всех, включая `not_portable` |
| Т6 | `SetSelected` зовёт `OnChanged` при построении строки | хранилище записи помечает модель изменённой только при реальной смене карты |

## Итог волны (15.09.2026)

- **Стенд на копии живых данных** (`scratchpad/livecmp`, `harness_v3`): config.json
  ветки байт-в-байт с develop `d74b2d76` — на нетронутой копии и на варианте с
  `dns_google_udp_dns_ip = 8.8.4.4`; develop совпадает с эталоном `out_new2`.
  Экспорт: запись `google_udp` несёт `outbound` (и `dns_ip` у варианта), корневых
  `dns_google_*` нет, `out` пресета `russian`, равный умолчанию, снят. Круг в копию
  — состояние изменилось ровно в четырёх местах (vars записи, vars пресета, два
  корневых имени), config тот же, повторный экспорт = файл. Круг в пустое —
  маршрут через proxy-out и адрес 8.8.4.4 на месте, повторный экспорт = файл.
  Файл develop (форма D-117) в пустое — маршрут переносится терпимым чтением.
  `sing-box check` lx.39 — все собранные конфиги приняты.
- **Эталон v6mig** (`ETALON_V6MIG=1`) и `TestGoldenScenarios` — без пересчёта.
- **go1.20** — греп диффа чист; `core/state`, `core/template`, `core/build`,
  `core/backup`, `core/debugapi` собираются тулчейном go1.20.14
  (`-modfile=go.win7.mod`, windows/386).
