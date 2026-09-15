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
	"singbox-launcher/core/build"
	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	settingsBackupExportDoneText    = "Saved to:\n%s\n\nThe file stores passwords and keys as plain text — keep it somewhere safe."
	settingsBackupHintText          = "Export settings to a file to move them between this launcher and LxBox on your phone. Subscriptions, servers, rules, DNS and portable variables are carried over."
	settingsBackupSummaryCountsText = "Subscriptions: %d\nServers: %d\nRules: %d\nVariables: %d"

	// Что импорт сделает с тем, что уже настроено. Раньше здесь стояло
	// «Importing replaces the current sources and rules» — с переходом на
	// слияние (D-095) это была бы прямая неправда о самом страшном исходе:
	// пользователь отказывался бы от импорта, боясь потерять свои источники.
	// Про правила сказано отдельно и честно — они единственные замещаются
	// целиком; DNS сливается, и обещать его замену тоже было бы неправдой.
	settingsBackupImportMergeNoteText = "Settings are merged, not replaced: subscriptions match by address, servers by what they connect to, and anything of yours that is not in the file stays. Routing rules are the exception — they are replaced by the file."

	// Сводка файла 1.0: там одна секция источников (sources[]), и разложить
	// её обратно на «подписки и серверы» ради старой строки значило бы
	// научить UI форме файла — ровно тому, от чего избавляет union File.
	settingsBackupSummaryCounts10Text = "Sources: %d\nRules: %d\nVariables: %d"
)

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

// handleBackupExport пишет файл и показывает отчёт.
//
// Формат не спрашивается: писатель у лаунчера один — 1.0 (D-110), релизы
// лаунчера и LxBox выходят синхронно, и выбирать пользователю не из чего.
func handleBackupExport(presenter *wizardpresentation.WizardPresenter, win fyne.Window) {
	st := presenter.CreateStateFromModel("", "")
	if st == nil {
		dialog.ShowError(fmt.Errorf("%s", locale.T("Cannot read the current state")), win)
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

	exportWarns, err := backup.ExportFile(path, st, backupExportOptions(presenter, st))
	if err != nil {
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

// backupExportOptions — шапка файла и то, что писатель берёт у шаблона.
//
// Направления — телом ПОСЛЕ слияния (build.ResolveDirections): ссылочная
// запись в состоянии тонкая, и без тела шаблона или пресета proxy-out уехал
// бы в файл одним тегом — без отбора узлов и без включений. Тег блокировки —
// тот же, что у галки формы Направления.
func backupExportOptions(presenter *wizardpresentation.WizardPresenter, st *corestate.State) backup.ExportOptions {
	var td *wizardtemplate.TemplateData
	if model := presenter.Model(); model != nil {
		td = model.TemplateData
	}
	values := make(map[string]string, len(st.Vars))
	for _, v := range st.Vars {
		values[v.Name] = v.Value
	}
	return backup.ExportOptions{
		AppVersion: constants.AppVersion,
		Platform:   runtime.GOOS,
		Directions: build.ResolveDirections(st.Directions, td, build.TargetSpecFromState(st)),
		BlockTag:   td.DirectionBlockTag(),
		// SPEC 129: записи шаблонных DNS-серверов и пресетов едут без
		// умолчаний и необъявленных имён (писатель — тоже писатель, Н4).
		RecordVars: wizardtemplate.RecordVarDeclsFor(td, values, build.TargetSpecFromState(st)),
	}
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

	// Импорт меняет состояние — спрашиваем ДО, а не после: слияние
	// оставляет своё, но правила и DNS замещает, и знать об этом надо
	// заранее.
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

func applyBackup(presenter *wizardpresentation.WizardPresenter, win fyne.Window, b *backup.File, parseWarns []backup.Warning) {
	// Новая машина: своего state.json у визарда ещё нет и правок не было —
	// модель тогда сид шаблона, и файл сливается в пустое состояние, как у
	// POST /backup/import на свежей установке (ImportBackupFile).
	fresh := !presenter.GetStateStore().StateExists("") && !presenter.HasUnsavedChanges()
	res, stage, err := presenter.ImportBackupFile(b, fresh)
	if err != nil {
		switch stage {
		case wizardpresentation.BackupImportStageRead:
			dialog.ShowError(fmt.Errorf("%s", locale.T("Cannot read the current state")), win)
		case wizardpresentation.BackupImportStageLoad:
			dialog.ShowError(fmt.Errorf("%s: %w", locale.T("Failed to restore state"), err), win)
		default:
			dialog.ShowError(fmt.Errorf("%s: %w", locale.T("Import failed"), err), win)
		}
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
//
// Файл может быть любого читаемого формата, и UI про это знать не обязан:
// шапку и счётчики отдаёт сам File. Разная строка счётчиков — не косметика:
// у 0.x секции источников две (подписки и серверы порознь), у 1.0 одна, и
// назвать «Серверов: 0» там, где их пять внутри sources[], значило бы соврать
// пользователю ДО того, как он нажал «Импорт».
func backupSummary(b *backup.File, warns []backup.Warning) string {
	var sb strings.Builder
	by, at := b.ExportedByOf()
	fmt.Fprintf(&sb, locale.T("From %s %s, exported %s"), by.App, by.Version, at)
	sb.WriteString("\n\n")
	if b.Legacy != nil {
		fmt.Fprintf(&sb, locale.T(settingsBackupSummaryCountsText),
			len(b.Legacy.Subscriptions), len(b.Legacy.Servers),
			len(b.Legacy.Rules), len(b.Legacy.Vars))
	} else {
		sources, rules, vars := b.Counts()
		fmt.Fprintf(&sb, locale.T(settingsBackupSummaryCounts10Text), sources, rules, vars)
	}
	if len(warns) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(warnLines(warns, backupSummaryWarnLimit))
	}
	sb.WriteString("\n\n")
	sb.WriteString(locale.T(settingsBackupImportMergeNoteText))
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
		// SPEC 129 Н9: у DNS-сервера цель — канал запроса, и выключается
		// сервер, а не правило.
		if w.Kind == "dns_server" {
			return fmt.Sprintf(locale.T("%s — this DNS server's channel does not exist here, the server is imported turned off"), w.Detail)
		}
		return fmt.Sprintf(locale.T("%s — target does not exist here, the rule is imported turned off"), w.Detail)
	case backup.WarnBackupFinalDropped:
		return fmt.Sprintf(locale.T("%s — default route target does not exist here, left unchanged"), w.Detail)
	case backup.WarnBackupUnknownPreset:
		return fmt.Sprintf(locale.T("%s — unknown preset, the rule is imported turned off"), w.Detail)
	case backup.WarnBackupVarSkipped:
		return varSkippedWarnText(w)
	case backup.WarnBackupDNSEntrySkipped:
		return fmt.Sprintf(locale.T("%s — this template has no such DNS server, the entry is not imported"),
			strings.TrimPrefix(w.Detail, "template:"))
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
	case backup.WarnBackupDirectionExists:
		return fmt.Sprintf(locale.T("%s — a Direction with this tag already exists here, the incoming one is skipped"), w.Detail)
	case backup.WarnBackupTagMaskDropped:
		return fmt.Sprintf(locale.T("%s — the tag mask is gone, only prefix and postfix apply now"), w.Detail)
	case backup.WarnBackupLocalDirectionDropped:
		return fmt.Sprintf(locale.T("%s — per-source Directions are gone, this one is not imported (create a global Direction with a filter instead)"), w.Detail)
	case backup.WarnBackupReplaceTagDerived:
		// Detail несёт оба имени («тег → дериватив»): пользователь обязан
		// увидеть, под каким именем группа окажется на приёмнике.
		return fmt.Sprintf(locale.T("%s — the shared format has no field for the replacement tag: after import the group will carry the derived name, and rules aimed at the old one arrive turned off"), w.Detail)
	case backup.WarnBackupSourceIdentityDropped:
		return fmt.Sprintf(locale.T("%s — these subscription settings are not part of the shared format and did not go into the file; the provider may return a different set of nodes on the other machine"), w.Detail)
	case backup.WarnBackupSourceFlagDropped:
		return fmt.Sprintf(locale.T("%s — the \"exclude from the global list\" flag is gone; its nodes stay in the candidate pool (fold the source into a group for the previous behaviour)"), w.Detail)
	case backup.WarnBackupLabelDropped:
		return fmt.Sprintf(locale.T("%s — label dropped, a node is named by its tag"), w.Detail)
	case backup.WarnBackupLocalOnlyDropped:
		// Экспорт: опции Направления, которые не теги Направлений, в общий
		// формат не едут (NODE_LINK.md §8). Detail — «Направление: опции».
		return fmt.Sprintf(locale.T("%s — these Direction options are settings of this machine (folder replacements, service tags, nodes) and did not go into the file; a node joins a Direction through its filter"), w.Detail)
	case backup.WarnBackupDirectionIncludeDropped:
		return fmt.Sprintf(locale.T("%s — these Direction options are not Directions or known names here, they were left out; a node joins a Direction through its filter"), w.Detail)
	case backup.WarnBackupGroupDegraded:
		// У лаунчера причина одна — группа по правилу отбора без состава;
		// чужой reason (LxBox упрощает selector) показывать нечем, и сырой
		// код лучше, чем неверное объяснение.
		if w.Reason == backup.GroupDegradedMembersRule {
			return fmt.Sprintf(locale.T("%s — this group picks its members by a rule, which the launcher does not support; it was not imported, and links to it will not resolve"), w.Detail)
		}
		return w.Code + ": " + w.Detail + " (" + w.Reason + ")"
	case backup.WarnBackupSectionRecordDropped:
		// Detail несёт «тег узла: вид записи»: без тега пользователю негде
		// искать, что именно потеряло часть своей связки. Причина у кода не
		// одна (норма B3), и общая фраза про «вид записи» на правиле с
		// rule_set просто врала бы — вид там как раз законный.
		if w.Reason == backup.SectionDropRuleSet {
			return fmt.Sprintf(locale.T("%s — a node rule referenced a rule set; node sections neither declare nor reference rule sets, so the whole rule was dropped (cutting just the reference would have turned it into a match-all)"), w.Detail)
		}
		return fmt.Sprintf(locale.T("%s — a node carries an entry of this kind, which node sections do not allow; the entry was dropped, the rest of the node came through"), w.Detail)
	default:
		// Сюда попадать не должно: каждый код обязан иметь свою фразу выше.
		// Сырой код остаётся последним рубежом, чтобы новое предупреждение
		// не пропало молча, если фразу забыли.
		return w.Code + ": " + w.Detail
	}
}

// varSkippedWarnText — непримененная переменная (SPEC 129 §5.6): причина и
// носитель различают четыре разных разговора с пользователем.
func varSkippedWarnText(w backup.Warning) string {
	record := w.Record
	if i := strings.Index(record, ":"); i >= 0 {
		record = record[i+1:]
	}
	switch w.Reason {
	case corestate.RecordVarUndeclared:
		if w.Kind == "preset" {
			return fmt.Sprintf(locale.T("%s: parameter \"%s\" is not declared by this template's preset, skipped"), record, w.Detail)
		}
		return fmt.Sprintf(locale.T("%s: parameter \"%s\" is not declared by this template's DNS server, skipped"), record, w.Detail)
	case corestate.RecordVarSuperseded:
		return fmt.Sprintf(locale.T("%s — the file also sets this DNS server's parameters in its entry; the entry wins, this older form is skipped"), w.Detail)
	case corestate.RecordVarNoRecord:
		return fmt.Sprintf(locale.T("%s — the file has no entry for this DNS server, the value is skipped"), w.Detail)
	default:
		return fmt.Sprintf(locale.T("%s — this setting means something else on this machine, skipped"), w.Detail)
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
