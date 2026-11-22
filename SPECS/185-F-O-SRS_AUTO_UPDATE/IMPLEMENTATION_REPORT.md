# IMPLEMENTATION REPORT 185

Дата: 22.11.2026. Статус: реализовано, ждёт показа владельцу (UI и проверка
перечитывания набора на живом ядре). Контракт 1.1.177.

## Что сделано

### Этап 1 — режим local
- `core/services/srs_downloader.go`: `RefreshSRS` (условный GET по mtime, 304
  продлевает свежесть), `SRSFileModTime`, проверка скачанного до замены
  (`validateSRSFile`: сигнатура `SRS` либо JSON) — действует и для кнопки srs;
  `NormalizeSRSURL` экспортирован.
- `core/build/rule_set_policy.go` (новый): `ResolveRuleSetPolicy` (правило →
  переменная → умолчание), `PresetMergeContext.RuleSetFetches()`,
  `DropUncachedSrsPaths`, `remoteRuleSetBody`.
- `core/srs_refresh.go` (новый): обход цикла (`runScheduledSRSRefresh`), не
  больше 2 загрузок сразу, пауза 1 ч после сбоя, повтор через 15 с и по
  VPN-событию; докачка отсутствующих в сборке (`ensureMissingRuleSets`, 20 с);
  `configReferencesMissingRuleSet`; `RefreshMachineRuleSets`.
- `core/auto_update.go`: вызовы на старте, по часовому тику и по VPN-событию.
- `core/rebuild.go`: докачка перед сборкой; pre-start принудительно собирает
  конфиг, если `config.json` ссылается на отсутствующий файл.
- `core/config_service_context.go`: правило `kind: srs` без файла выпадает.
- `core/services/lxd_remote_deploy.go`, `core/config_service.go`:
  `PreDeployFunc` — обновление наборов машины перед Deploy.

### Этап 2 — режим remote и настройки
- `core/state`: поля записи правила `srs_mode`, `update_interval_hours`,
  `download_detour`; `RuleSetOptions`; клон; провоз через `CustomRule` и модель
  визарда (`RuleState.SrsOptions`, `PresetRefState.SrsOptions`).
- `core/build/resolve_route.go`: `ResolveRouteChecked` — эмиссия `type: remote`
  для пресета и `kind: srs`, недоступный `download_detour` снимает набор и
  правило; `PresetMergeContext.FinalOutboundTags`.
- `ui/configurator/business/root_name_refs.go`: 8-й вид ссылки при
  переименовании Направления.
- UI: `dialogs/rule_set_options.go` (блок «Rule-set updates») в окне правила SRS
  и пресета; дата загрузки в подсказке кнопки srs; у remote кнопки и гейта нет.
- `bin/wizard_template.json`: раздел `rule-sets` с тремя переменными;
  `bin/locale/ru.json`.
- Контракт 1.1.177: `VERSION`, changelog `README.md`, `BACKUP.md`,
  `backup.schema.json`, `registry/vars.json`, корпус `v10_rule_set_options`
  (+ `.expected.lxbox.json`), `TASKS_LXBOX.md` §180, D-154;
  `core/backup/file_keys_10.go` — поле общее.

## Отступления от хода обсуждения
- Обновление подписки по часовому циклу конфиг не пересобирает, а помечает
  устаревшим. Наборы поступают так же: появившийся (ранее отсутствовавший)
  файл помечает конфиг устаревшим; обновление существующего файла — ничего
  (ядро перечитывает само).
- Имя поля режима — `srs_mode` (а не `mode`): запись правила плоская, общее
  слово там двусмысленно; совпадает с именем переменной.
- Цикл автообновления работает, только если автообновление подписок включено
  (тот же гейт).

## Известные ограничения
- Пресеты с `for_each_node*`: их наборы в план обновления не попадают, пока
  конфиг не собран (узлы известны только сборке) — кнопка srs работает.
- Ручной Deploy без пересборки заливает уже собранный конфиг; набор, который
  до этого отсутствовал, попадёт в конфиг машины при следующей её пересборке.
- `RELEASE_NOTES.md` — выжимка пишется при выпуске релиза.

## Проверки
- `go build ./...` — ок.
- `go test ./core/services -run TestRefreshSRS`, `./core/build -run
  TestRuleSetRemoteEmission` — ок.
- Раннеры контракта по имени: `TestBackupCorpus`,
  `TestLxBoxSideFieldsLeaveStateUnchanged`, `TestExport10EntityKeysAreDeclared`,
  `TestPortableVarsMatchRegistry`, `TestRoundTrip10ByteIdentical`,
  `TestContractCorpusTemplate`, `TestWizardTemplateFormatted` — ок.
- `tools/l10n` (`l10n_check`, `hardcoded_check` `--strict`) — ок.
- На HEAD до этой работы падают `TestWizardTemplateConfigUnchanged` и
  `TestBundledTemplate_GatewayIncludeInterface` (коммит 34370ff6, issue #160) —
  к SPEC 185 не относятся: на базе 2658201c с правками 185 оба проходят.

## Осталось владельцу
- Посмотреть UI: Settings → Rule-sets; окно правила SRS и пресета — блок
  «Rule-set updates»; подсказка кнопки srs.
- На живом ядре подменить `.srs` и убедиться, что ядро перечитало набор
  (fswatch при замене через rename).
