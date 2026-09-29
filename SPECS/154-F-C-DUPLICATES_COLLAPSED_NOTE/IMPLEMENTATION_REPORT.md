# IMPLEMENTATION REPORT 154

Дата: 2026-09-29. Статус: реализовано, контракт 1.1.102.

## Что сделано

- **Реестр.** `duplicates_collapsed` (info, params `count`, `names`) в
  `contract/registry/warnings.json`; `VERSION` 1.1.102; строка changelog в
  `contract/README.md`; `TASKS_LXBOX.md` §99; `go generate ./contract/...`.
- **Парсер.** Общий шаг `markDuplicatesCollapsed` (`server_conn_key.go`):
  фильтр имён, кап 10 имён, `names` = имя выжившего, если называть некого.
  `sourceDedup` помнит выжившего по указателю и его исходный тег;
  `markSurvivors` зовётся в `bodyParseState.finish` и в `DedupParsedNodes`
  (превью = боевой разбор).
- **Xray-массив.** `xrayCollapse` (`xray_json_array.go`) копит имена
  узлов, выброшенных владением, по ключу сервера; члены групп своего
  элемента не считаются (`poolMemberServers`); метка ставится после
  `simplifySoloElementTags`.
- **State.** `sourceFactWarningCodes` (`core/state/node_enabled.go`):
  `ReplaceDerivedWarnings` бережёт код, если свежий набор его не принёс, и
  берёт свежий, если принёс (иначе `CarryCoreVerdict` терял бы свежий код у
  узла с вердиктом ядра); `hasDerivedWarnings` код производным не считает.
- **Корпус +2:** `body/uri_list/duplicates_collapsed`,
  `body/xray/duplicates_collapsed_owner`. Конверт корпуса несёт только
  код, `count`/`names` — норма записи реестра.
- **Тест:** `TestDuplicatesCollapsedNamedOnSurvivor` — список ссылок и
  Xray-массив с пулом.

## Проверка на живых подписках (29.09.2026, не в репозитории)

- 11 записей → 6 узлов, у пяти код с `count` 1 и именем пары
  («🇦🇹 Австрия» ← «🇩🇪 Германия» и т. д.); Xray-форма — то же.
- 108 записей → 5 узлов, `count` 25/21/19/20/18 (в сумме 103), по 10 имён
  и `…`; Xray-форма — то же, члены пула «Турбо» не в счёте.

## Проверки

`go build ./...`; `go test ./core/config/subscription -run 'Registry|TestDuplicatesCollapsed'`;
`go test ./core/config -run 'Registry|TestContractCorpus'` — зелёные.
UI не менялся: `info`-коды уже показываются (`nodewarn.InfoOnly`).

## Не сделано в лаунчере

Сторона LxBox — `TASKS_LXBOX.md` §99 (снять per-app `duplicate` из
`dropped[]`, поставить код на выживший).
