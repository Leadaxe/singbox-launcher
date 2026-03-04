# Отчёт о реализации: Локальное скачивание SRS

## Статус

- [x] Реализовано
- [x] Протестировано (go test ./ui/wizard/business/... — проходит)
- [ ] Готово к релизу (требуется ручная проверка UI)

## Выполненные задачи

1. **Удалён go-any-way-githubusercontent** — из `wizard_template.json`, `get_free.json`
2. **Удалены download_detour, update_interval** — из rule_set в шаблоне для всех SRS-правил
3. **Директория bin/rule-sets/** — создаётся при инициализации через `platform.EnsureDirectories`
4. **Сервис DownloadSRS** — `core/services/srs_downloader.go`: скачивание по HTTP, таймаут 60 с, атомарная запись
5. **MergeRouteSection** — подстановка `type: "local"`, `path` вместо remote для SRS с raw.githubusercontent.com
6. **Кнопка SRS** — "⬇ SRS" / "🔄 SRS" / "✔️ SRS" после иконки `?` для правил с SRS; надписи вынесены в константы; при наведении показывается tooltip с оригинальным URL из wizard_template.json
7. **Чекбокс** — при клике по правилу без SRS запускается скачивание (как при клике на кнопку); при успехе правило включается
8. **При открытии визарда** — без всплывашек; правила с SRS не скачанными — чекбоксы сняты
9. **Обновлены source_tab, ParserConfig.md** — удалены упоминания go-any-way-githubusercontent

## Изменённые файлы

| Файл | Изменения |
|------|-----------|
| `bin/wizard_template.json` | Удалён outbound go-any-way-githubusercontent, удалены download_detour/update_interval из rule_set |
| `bin/get_free.json` | Удалён outbound go-any-way-githubusercontent |
| `internal/constants/constants.go` | Константа RuleSetsDirName |
| `internal/platform/platform_common.go` | GetRuleSetsDir(), EnsureDirectories — добавлена bin/rule-sets/ |
| `core/services/srs_downloader.go` | **Новый** — DownloadSRS, GetRemoteSRSEntries, AllSRSDownloaded, RuleSRSPath, SRSFileExists |
| `ui/wizard/models/wizard_model.go` | Поле ExecDir |
| `ui/wizard/wizard.go` | model.ExecDir = ac.FileService.ExecDir |
| `ui/wizard/business/create_config.go` | MergeRouteSection(execDir), convertRuleSetToLocalIfNeeded, services.RuleSRSPath |
| `ui/wizard/business/generator_test.go` | MergeRouteSection(..., "") |
| `ui/wizard/presentation/gui_state.go` | RuleWidget.SRSButton |
| `ui/wizard/presentation/presenter_methods.go` | InitializeTemplateState — services.AllSRSDownloaded для enabled |
| `ui/wizard/presentation/presenter_state.go` | restoreSelectableRuleStates — services.AllSRSDownloaded для enabled |
| `ui/components/tooltip_wrapper.go` | **Новый** — ToolTipWrapper (overlay + desktop.Hoverable) для tooltip при наведении |
| `ui/wizard/tabs/rules_tab.go` | Кнопка SRS, константы srsBtn*, tooltip с URL из шаблона, логика checkbox (клик → скачивание), без попапов при открытии |
| `ui/wizard/tabs/source_tab.go` | Удалена подсказка go-any-way-githubusercontent |
| `docs/ParserConfig.md` | Удалены упоминания go-any-way-githubusercontent |

## Ключевые фрагменты кода

### services/srs_downloader.go
```go
func DownloadSRS(ctx context.Context, url string, destPath string) error
func AllSRSDownloaded(execDir string, ruleSets []json.RawMessage) bool
func RuleSRSPath(execDir string, tag string) string
```

### create_config.go
```go
func convertRuleSetToLocalIfNeeded(rs json.RawMessage, execDir string) interface{}
// type: remote + raw.githubusercontent.com → type: local, path: services.RuleSRSPath(execDir, tag)
```

### Кнопка SRS (rules_tab.go)
Константы: `srsBtnDownload`, `srsBtnLoading`, `srsBtnDone` — надписи управляются в одном месте.
- **⬇ SRS** — SRS не скачаны, клик запускает скачивание
- **🔄 SRS** — идёт скачивание, кнопка неактивна
- **✔️ SRS** — скачано, клик — перезагрузка

**Tooltip:** при наведении показывается оригинальный URL из wizard_template.json. Для нескольких SRS — URL через перевод строки. Реализовано через `components.ToolTipWrapper` (overlay + desktop.Hoverable + widget.PopUp).

### Клик по чекбоксу
При попытке включить правило без SRS — вызывается `srsButton.OnTapped()`, запускается скачивание (без попапа).

## UI — все сообщения на английском

- При клике по чекбоксу правила без SRS — запускается скачивание (попап не показывается)
- Ошибка скачивания: "Failed to download SRS: %v. Check your internet connection or use a VPN."

## Команды для проверки

```bash
go build ./...
go test ./...
go vet ./...
```

*Примечание:* на Windows в некоторых средах `go build` может падать из-за OpenGL (fyne/gl) — это среда, не код.

## Риски и ограничения

1. **GitHub заблокирован** — пользователь в РФ не сможет скачать SRS без VPN. Решение: использовать VPN вне приложения или получить rule-sets от другого пользователя.
2. **Размер SRS** — файлы могут занимать несколько мегабайт.
3. **Права на запись** — директория `bin/rule-sets/` должна быть доступна для записи.

## Assumptions

1. **Контекст отмены** — при закрытии визарда во время скачивания горутина продолжает работу (context.Background). Файл будет записан при успехе.
2. **Путь path** — используется абсолютный путь `{ExecDir}/bin/rule-sets/{tag}.srs` для совместимости с sing-box.

## Дата завершения

2026-02-18
