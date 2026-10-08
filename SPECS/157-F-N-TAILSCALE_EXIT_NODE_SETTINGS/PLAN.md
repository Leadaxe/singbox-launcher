# PLAN 157

## Файлы

| Файл | Изменение |
|------|-----------|
| `core/config/subscription/origin_kind.go` | новый: `OriginKindOfText`, `IsJSONOriginText` |
| `core/config/migrate_materialize.go` | JSON в `URI` → ветка config_json; `nodeBodyOfJSONOriginText` (документ/массив → первое тело) |
| `core/state/authored.go`, `core/state/load_router.go` | загрузка: `uri` с JSON в raw → `json`, файл пересохраняется |
| `core/backup/legacy_read_0x.go` | вид происхождения по `OriginKindOfText` |
| `ui/configurator/tabs/source_edit_window.go` | `setSourceOriginURI` по форме текста; блок Tailscale в ветке server; перечитывание после Regen и Apply JSON; перерисовка подписи поля после Regen |
| `ui/configurator/tabs/source_body_edit.go` | Regen ветвится по форме текста; JSON-документ/массив принимаются |
| `ui/configurator/tabs/source_tailscale_edit.go` | новый: блок роли (`tailscaleBlock`, `applyTailscaleRole`, `writeTailscaleBody`) |
| `bin/locale/ru.json` | переводы новых строк |
| `docs/release_notes/upcoming.md` | заметки |

## Что не трогаем

- Реестр (`contract/registry/protocols/tailscale.json`): правила
  `conflicts`/`requires` уже есть, форма на них опирается.
- Конструктор `add_server_tailscale.go` и вкладку Network (Save choice):
  поведение то же, общий путь записи — через тело узла.
- Тесты: UI-правка; серверная часть — единая точка материализации,
  полный прогон в CI.
