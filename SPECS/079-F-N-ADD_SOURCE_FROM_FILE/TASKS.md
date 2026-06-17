# TASKS 079 — Add source from file

- [x] `business.ReadSourceFileText(io.Reader)` — чтение с лимитом 1 МБ + trim (source_file.go)
- [x] UI: вынести apply-логику Add в `applyAddedSources(text)`; кнопка «Add from file» (dialog.NewFileOpen, фильтр .conf/.vpn/.txt) → ReadSourceFileText → applyAddedSources
- [x] Кнопка в header вкладки Sources; импорт `fyne/storage`
- [x] Локали en + ru (`wizard.source.button_add_from_file`)
- [x] Тест `ReadSourceFileText` (trim/conf/nil/over-cap/at-cap)
- [x] docs/ParserConfig.md (раздел Add from file) + docs/release_notes/upcoming.md
- [x] `go build ./... && go test ./... && go vet ./...`
