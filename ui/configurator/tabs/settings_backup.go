package tabs

// Экспорт и импорт LX Backup — секция вкладки «Файлы» (SPEC 103, фаза 4).
//
// Сперва жила под прокруткой Settings, но была прибита к её низу через
// Border и забирала свою высоту целиком (114–133px, тем больше, чем уже
// окно: подсказка переносится), а прокрутке настроек доставался остаток —
// нижние строки настроек обрезались. Переехала на «Файлы» к остальным
// действиям над готовым состоянием: собрать конфиг, посмотреть его,
// перенести настройки на другую машину.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/backup"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	settingsBackupExportDoneText    = "Saved to:\n%s\n\nThe file stores passwords and keys as plain text — keep it somewhere safe."
	settingsBackupHintText          = "Export settings to a file to move them between this launcher and LxBox on your phone. Subscriptions, servers, rules, DNS and portable variables are carried over."
	settingsBackupSummaryCountsText = "Subscriptions: %d\nServers: %d\nRules: %d\nVariables: %d"
)

// knownPresetIDs — id пресетов текущего шаблона. Пустой список означает
// «шаблон не загружен» — тогда ссылки на пресеты не режутся: выключить всё
// подряд хуже, чем импортировать как есть.
func knownPresetIDs(presenter *wizardpresentation.WizardPresenter) []string {
	model := presenter.Model()
	if model == nil || model.TemplateData == nil {
		return nil
	}
	out := make([]string, 0, len(model.TemplateData.Presets))
	for _, p := range model.TemplateData.Presets {
		if p.ID != "" {
			out = append(out, p.ID)
		}
	}
	return out
}

// backupSection — блок «Экспорт» / «Импорт» с пояснением.
func backupSection(presenter *wizardpresentation.WizardPresenter, win fyne.Window) fyne.CanvasObject {
	exportBtn := widget.NewButton(locale.T("Export…"), func() {
		handleBackupExport(presenter, win)
	})
	importBtn := widget.NewButton(locale.T("Import…"), func() {
		handleBackupImport(presenter, win)
	})

	title := widget.NewLabelWithStyle(
		locale.T("Backup"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	hint := widget.NewLabel(locale.T(settingsBackupHintText))
	hint.Wrapping = fyne.TextWrapWord

	return container.NewVBox(
		settingsSeparatorBlock(),
		title,
		hint,
		container.NewHBox(exportBtn, importBtn),
	)
}

// handleBackupExport собирает текущее состояние и пишет файл.
func handleBackupExport(presenter *wizardpresentation.WizardPresenter, win fyne.Window) {
	st := presenter.CreateStateFromModel("", "")
	if st == nil {
		dialog.ShowError(fmt.Errorf("%s", locale.T("Cannot read the current state")), win)
		return
	}

	b, exportWarns, err := backup.Export(st, backup.ExportOptions{
		AppVersion: constants.AppVersion,
		Platform:   runtime.GOOS,
	})
	if err != nil {
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("Export failed"), err), win)
		return
	}

	suggested := backup.SuggestFileName(time.Now().Format("2006-01-02"))
	path, ok, err := platform.PickSaveFile(locale.T("Save LX Backup"), suggested)
	if err != nil || !ok {
		if err != nil && err != platform.ErrNativeDialogUnavailable {
			debuglog.WarnLog("backup export: save dialog: %v", err)
		}
		if err == platform.ErrNativeDialogUnavailable {
			// Нативного диалога нет — кладём рядом с исполняемым файлом и
			// говорим куда: молча ничего не делать хуже.
			path = filepath.Join(defaultBackupDir(), suggested)
		} else {
			return // отмена пользователя
		}
	}
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		path += ".json"
	}

	if err := backup.WriteFile(path, b); err != nil {
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("Export failed"), err), win)
		return
	}

	// Секреты в файле лежат открытым текстом (BACKUP.md §5) — пользователь
	// должен знать об этом ДО того, как отправит файл куда-нибудь.
	//
	// Предупреждения экспорта — тем же окном, что у импорта, и БЕЗ обрезки
	// (SPEC 116 W9, критерий A9): то, что состояние несёт, а формат выразить
	// не умеет (папки, провайдерские группы), обязано быть названо целиком.
	// Прежняя модалка резала список на десяти строках и отсылала за
	// продолжением в отчёт импорта, которого при экспорте не существует.
	showExportReport(win, path, exportWarns)
}

// handleBackupImport читает файл, показывает, что приедет, и применяет
// только после подтверждения.
func handleBackupImport(presenter *wizardpresentation.WizardPresenter, win fyne.Window) {
	path, ok, err := platform.PickOpenFile(locale.T("Open LX Backup"), []string{"json"})
	if err != nil || !ok {
		if err == platform.ErrNativeDialogUnavailable {
			// Linux без zenity/kdialog: молча вернуться значило бы «кнопка
			// не работает и не говорит почему». Экспорт в этом случае пишет
			// в домашний каталог; у импорта запасного пути нет — говорим,
			// чего не хватает.
			dialog.ShowError(fmt.Errorf("%s", locale.T("Native file dialog is unavailable. Install zenity or kdialog and try again.")), win)
			return
		}
		if err != nil {
			debuglog.WarnLog("backup import: open dialog: %v", err)
		}
		return
	}

	b, parseWarns, err := backup.ReadFile(path)
	if err != nil {
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("Import failed"), err), win)
		return
	}

	// Импорт заменяет состояние целиком — спрашиваем ДО, а не после.
	summary := backupSummary(b, parseWarns)
	dialog.ShowCustomConfirm(
		locale.T("Import backup"),
		locale.T("Import"),
		locale.T("Cancel"),
		widget.NewLabel(summary),
		func(confirmed bool) {
			if !confirmed {
				return
			}
			applyBackup(presenter, win, b, parseWarns)
		}, win)
}

func applyBackup(presenter *wizardpresentation.WizardPresenter, win fyne.Window, b *backup.Backup, parseWarns []backup.Warning) {
	st := presenter.CreateStateFromModel("", "")
	if st == nil {
		dialog.ShowError(fmt.Errorf("%s", locale.T("Cannot read the current state")), win)
		return
	}

	res, err := backup.Import(st, b, backup.ImportOptions{
		// Известные цели берём из модели: правило, ссылающееся в никуда,
		// приедет выключенным, а не уронит конфиг ядра.
		KnownOutbounds: wizardbusiness.GetAvailableOutbounds(presenter.Model()),
		KnownPresets:   knownPresetIDs(presenter),
	})
	if err != nil {
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("Import failed"), err), win)
		return
	}

	if err := presenter.LoadState(st); err != nil {
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("Failed to restore state"), err), win)
		return
	}
	presenter.SyncModelToGUI()
	presenter.MarkAsChanged()

	// SPEC 115: импорт — единственная правка модели, которую делают, СТОЯ на
	// вкладке «Итог» (блок импорта живёт на ней). MarkAsChanged выше уже
	// стёр отчёт, но экран и кнопка Save остались от прошлой сборки: без
	// пересборки пользователь смотрел бы на отчёт чужого состояния и мог бы
	// его сохранить. Остальные вкладки такой проводки не требуют — на «Итог»
	// с них попадают входом, а вход и есть сборка.
	if guiState := presenter.GUIState(); guiState != nil && guiState.RunFinalBuild != nil {
		guiState.RunFinalBuild()
	}

	all := append(append([]backup.Warning(nil), parseWarns...), res.Warnings...)
	// Отчёт — не «ок, понял»: потери надо прочитать целиком, поэтому список
	// уезжает в своё окно без обрезки (settings_backup_report_window.go).
	// Вызов идёт с UI-потока (обработчик подтверждения), fyne.Do не нужен.
	showImportReport(win, res, all)
}

// backupSummary — что лежит в файле, до применения.
func backupSummary(b *backup.Backup, warns []backup.Warning) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, locale.T("From %s %s, exported %s"),
		b.ExportedBy.App, b.ExportedBy.Version, b.ExportedAt)
	sb.WriteString("\n\n")
	fmt.Fprintf(&sb, locale.T(settingsBackupSummaryCountsText),
		len(b.Subscriptions), len(b.Servers), len(b.Rules), len(b.Vars))
	if len(warns) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(warnLines(warns, backupSummaryWarnLimit))
	}
	sb.WriteString("\n\n")
	sb.WriteString(locale.T("Importing replaces the current sources and rules."))
	return sb.String()
}

// backupSummaryWarnLimit — сколько потерь показываем в модалке подтверждения.
//
// Модалка отвечает на «стоит ли вообще импортировать», а не «что именно
// потеряется»: полный список живёт в окне отчёта после импорта, куда обрезка и
// отсылает. Ограничение здесь не косметика — высокий модальный попап в Fyne
// раздувает окно ([[fyne-label-minwidth-trap]]).
const backupSummaryWarnLimit = 20

// warnLines превращает коды в читаемые строки. Код без пояснения ничего не
// говорит пользователю — он не читал реестр.
//
// limit <= 0 означает «без обрезки».
func warnLines(warns []backup.Warning, limit int) string {
	var sb strings.Builder
	sb.WriteString(locale.T("Not applied as-is:"))
	shown := 0
	for _, w := range warns {
		if limit > 0 && shown >= limit {
			fmt.Fprintf(&sb, "\n… +%d", len(warns)-shown)
			sb.WriteString("\n")
			sb.WriteString(locale.T(settingsBackupReportMoreText))
			break
		}
		sb.WriteString("\n• ")
		sb.WriteString(warnText(w))
		shown++
	}
	return sb.String()
}

func warnText(w backup.Warning) string {
	switch w.Code {
	case backup.WarnBackupUnknownOutbound:
		return fmt.Sprintf(locale.T("%s — target does not exist here, the rule is imported turned off"), w.Detail)
	case backup.WarnBackupFinalDropped:
		return fmt.Sprintf(locale.T("%s — default route target does not exist here, left unchanged"), w.Detail)
	case backup.WarnBackupUnknownPreset:
		return fmt.Sprintf(locale.T("%s — unknown preset, the rule is imported turned off"), w.Detail)
	case backup.WarnBackupVarSkipped:
		return fmt.Sprintf(locale.T("%s — this setting means something else on this machine, skipped"), w.Detail)
	case backup.WarnBackupUnknownField:
		return fmt.Sprintf(locale.T("%s — not supported here, skipped"), w.Detail)
	case backup.WarnBackupFieldTypeMismatch:
		// Ключ знакомый, а тип чужой: сказать «не поддержано» было бы
		// неправдой — поле есть, разошлась его форма.
		return fmt.Sprintf(locale.T("%s — this field means something different here, its value is skipped"), w.Detail)
	case backup.WarnBackupExtensionsDropped:
		// Один warning на файл: extensions — упразднённый карман, а не
		// лишний ключ, и перечислять его внутренности значило бы утопить
		// пользователя в списке вместо объяснения.
		return fmt.Sprintf(locale.T("this backup was made by an older version and carries an \"extensions\" section (%s); the shared fields were imported, the rest is dropped"), w.Detail)
	case backup.WarnBackupSourceKindUnsupported:
		return unsupportedSourceWarnText(w)
	case backup.WarnBackupChainExists:
		return fmt.Sprintf(locale.T("%s — a chain with this name already exists here, the incoming one is skipped"), w.Detail)
	default:
		return w.Code + ": " + w.Detail
	}
}

// unsupportedSourceWarnText — запись, которой в файле не будет вовсе
// (SPEC 116 W9, §O1=А).
//
// Три вещи обязаны прозвучать в одной строке: ЧТО потеряно (папка или
// провайдерская группа — код у них общий, а слова разные), КАК её зовут и
// СКОЛЬКО узлов уехало вместе с ней. Без числа «папка не поддержана»
// читается как мелкая оговорка формата, хотя на деле это решение не
// восстанавливаться из этого файла.
//
// Ноль узлов — не «нет данных», а пустая папка: числа тогда нет, потому что
// терять нечего кроме самой папки.
func unsupportedSourceWarnText(w backup.Warning) string {
	name := w.Detail
	if name == "" {
		name = locale.T("unnamed")
	}
	switch w.Kind {
	case string(corestate.SourceKindFolder):
		if w.Nodes > 0 {
			return fmt.Sprintf(
				locale.T("folder \"%s\" and its %d node(s) did not make it into the file: this backup format cannot carry folders yet"),
				name, w.Nodes)
		}
		return fmt.Sprintf(
			locale.T("folder \"%s\" did not make it into the file: this backup format cannot carry folders yet"),
			name)
	case string(corestate.SourceKindAuto):
		return fmt.Sprintf(
			locale.T("group \"%s\" did not make it into the file: this backup format cannot carry provider groups yet"),
			name)
	default:
		// Новый вид источника без своей ветки: сказать «не поехало» честнее,
		// чем показать машинный код. Ветка обязана появиться вместе с видом.
		return fmt.Sprintf(
			locale.T("%s — this backup format cannot carry it, the record did not make it into the file"),
			name)
	}
}

// defaultBackupDir — куда класть файл, когда нативного диалога нет.
func defaultBackupDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "."
}
