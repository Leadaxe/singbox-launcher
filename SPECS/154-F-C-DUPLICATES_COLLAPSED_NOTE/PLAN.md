# PLAN 154

1. **Реестр.** `contract/registry/warnings.json` — запись `duplicates_collapsed`
   рядом с `group_member_missing`; `contract/VERSION` → 1.1.102; строка
   changelog в `contract/README.md`; `contract/TASKS_LXBOX.md` §99;
   `go generate ./contract/...`.
2. **Парсер.** `core/config/subscription/parse_warnings.go` — константа
   `WarnDuplicatesCollapsed`. Общий шаг `markDuplicatesCollapsed(node, ownName,
   names)` в `server_conn_key.go` (фильтр имён, кап 10, params).
3. **Дедуп тела.** `sourceDedup` помнит выжившего по указателю и имена
   схлопнутых; `markSurvivors()` зовётся в `bodyParseState.finish` и в
   `DedupParsedNodes` (превью ≡ боевой разбор).
4. **Xray-массив.** `filterByServerOwner` копит имена выброшенных узлов по
   ключу сервера, кроме членов групп своего элемента; после
   `simplifySoloElementTags` имена вешаются на выживших.
5. **State.** `core/state/node_enabled.go` — второй список «сохраняемых»
   кодов (`sourceFactWarningCodes`): `ReplaceDerivedWarnings` бережёт их,
   если свежий набор их не принёс, и берёт свежие, если принёс;
   `hasDerivedWarnings` их производными не считает. Константа кода
   дублируется строкой, как `core_rejected`.
6. **Проверки.** `go build ./...`; раннер корпуса тела по имени
   (`TestContractCorpusBody`) и страж реестра (`TestRegistryWarningCodes*`);
   один интеграционный тест на обе формы подписки.
7. **Доки.** `docs/release_notes/upcoming.md`, `SPECS/README.md`.
