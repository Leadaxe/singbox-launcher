# TASKS 187

## Этап 1 — примитивы и загрузчик
- [x] `internal/srstag`: `FormatFromRef`, `FileName`, `IsFileRef`, `FileRefFromPath`, `PathFromFileRef`; `.json` вне тега
- [x] `srs_downloader.go`: ветка `file://`, проверка содержимого по формату, имена копий по ref, чистка сирот для `.json`
- [x] `rule_set_policy.go`, `srs_refresh.go`: `file://` каждый проход, всегда local, пропавший исходник — предупреждение
- [x] Тест `TestRefreshSRSFileRef`

## Этап 2 — сборка
- [x] `resolve_route.go`, `preset_merge.go`, `route_merge.go`: `format` и имя файла по ref (local, remote-режим, ресурс-стор)
- [x] `go build ./...`

## Этап 3 — UI
- [x] `add_rule_dialog.go`: подписи, «Add file…», голый путь → `file://`, проверка при сохранении
- [x] `rules_tab.go`: кнопка `upd`, подсказка о пропавшем исходнике
- [x] `rule_set_options.go`: Mode недоступен для `file://`
- [x] `bin/locale/ru.json`

## Этап 4 — контракт и доки
- [x] `contract/**`: VERSION 1.1.178, changelog, BACKUP.md, схема, код предупреждения, кейс корпуса, параграф TASKS_LXBOX
- [x] Раннер корпуса бэкапа по имени
- [x] `docs/ARCHITECTURE.md`, `docs/WIZARD_STATE.md`, `upcoming.md`
- [x] IMPLEMENTATION_REPORT.md
- [ ] Показ владельцу (диалог, кнопка, подхват правки файла ядром)
- [ ] CI (`gh workflow run ci.yml --ref develop -f run_mode=tests`) — после коммита и пуша
