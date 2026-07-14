# TASKS 082 — Native file picker

- [x] `platform.PickOpenFile` (file_dialog.go) + `ErrNativeDialogUnavailable`
- [x] darwin: osascript `choose file` (file_dialog_darwin.go) + AppleScript escaping
- [x] windows: PowerShell OpenFileDialog `-STA` (file_dialog_windows.go) + filter/quote
- [x] linux: zenity → kdialog → ErrNativeDialogUnavailable (file_dialog_linux.go)
- [x] stub: other GOOS → ErrNativeDialogUnavailable (file_dialog_stub.go)
- [x] UI: source_tab uses PickOpenFile, falls back to Fyne dialog on ErrNativeDialogUnavailable
- [x] Locale prompt en + ru (`wizard.source.pick_file_prompt`)
- [x] Tests: AppleScript escaping; cross-compile darwin/windows/linux
- [x] docs/ParserConfig.md + docs/release_notes/upcoming.md
- [x] `go build ./... && go test ./... && go vet ./...`
