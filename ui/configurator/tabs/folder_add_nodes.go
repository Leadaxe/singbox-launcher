// File folder_add_nodes.go — «Add nodes…» в окне ПАПКИ (SPEC 116 этап 3, W6;
// цель 2, сценарий С1).
//
// # Почему кнопка живёт на вкладке Preview
//
// Там же, где список узлов папки: наполнение — это операция над составом, и
// её место рядом с составом, а не в Settings (там свойства контейнера: имя,
// тег-политика, свёртка, detour). У подписки кнопки нет вовсе: её состав
// принадлежит провайдеру (features/sources.md §«Свобода и несвобода узлов»),
// и добавленный руками узел исчез бы на первом же fetch'е.
//
// # Почему пункты меню те же, что у ⋮ Sources
//
// Пути наполнения перечислены в features/sources.md §«Наполнение папки» п.1:
// вставка URI/wg-INI/JSON, импорт файлов (мультивыбор), Add WARP, форма
// сервера. Ровно эти пункты уже есть в ⋮ списка Sources (source_tab.go:271) и
// зовут те же диалоги — второй набор диалогов разъехался бы с первым.
// Отличается только адрес назначения: там корень, здесь `Source.Nodes` папки
// (business.AppendNodesToFolder, W6).
//
// # Почему мутация немедленная
//
// Тот же довод, что у операций над узлом (см. шапку preview_node_ops.go):
// узел приезжает в ПАПКУ, а окно правит value-snapshot одного источника.
// Буферизация оставила бы Cancel возможность откатить добавление уже после
// того, как пользователь увидел узел в списке. Поэтому — прямая мутация
// модели + applySourceMutation + reloadScratch, как у Move/Copy.
package tabs

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizarddialogs "singbox-launcher/ui/configurator/dialogs"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	folderAddNodesPasteHintText = "Subscription URLs, direct links (vless://, wireguard://, …), [Interface]/[Peer] config text or a sing-box outbound JSON. One per line."
	// Отдельный текст, а не общий «no valid URLs to add»: в папке подписка
	// отвергается не потому, что строка непонятна, а потому что контейнер в
	// контейнер не кладут. Общая формулировка отправила бы человека искать
	// опечатку в правильной ссылке.
	folderAddNodesSubscriptionText = "a subscription URL cannot be added to a folder — add it in Sources, then fill the folder from it"
)

// folderAddNodes — контекст кнопки «Add nodes…» для одного открытого окна
// папки.
//
// Папка адресуется ULID'ом, а не индексом: диалоги асинхронны (нативная
// файловая панель, форма WARP), и к моменту возврата список источников мог
// поехать — например, фоновым удалением другой строки. Индекс окна для этого
// не годится, а ULID — единственная идентификация папки (SPEC 118).
type folderAddNodes struct {
	ops      *previewNodeOps
	folderID string
	// win — окно папки: владелец диалогов и адресат уведомлений.
	win fyne.Window
}

// newFolderAddNodes собирает контекст, если этому источнику наполнение
// вообще положено. nil означает «кнопки в шапке нет».
func newFolderAddNodes(ops *previewNodeOps, win fyne.Window) *folderAddNodes {
	if ops == nil || win == nil || !ops.nodeOpsAllowed() {
		return nil
	}
	m := ops.presenter.Model()
	if m == nil || ops.sourceIndex < 0 || ops.sourceIndex >= len(m.Sources) {
		return nil
	}
	id := strings.TrimSpace(m.Sources[ops.sourceIndex].ID)
	if id == "" {
		return nil
	}
	return &folderAddNodes{ops: ops, folderID: id, win: win}
}

// button — «Add nodes…» с тем же ⋮-паттерном, что у шапки Sources: одна
// кнопка, меню под ней. Новых глифов не заводим — DocumentCreateIcon уже
// означает «создать запись» в этом окне.
func (f *folderAddNodes) button() fyne.CanvasObject {
	var btn *widget.Button
	btn = widget.NewButtonWithIcon(locale.T("Add nodes…"), theme.DocumentCreateIcon(), func() {
		menu := fyne.NewMenu("",
			fyne.NewMenuItem(locale.T("Paste links or JSON…"), f.showPasteDialog),
			fyne.NewMenuItem(locale.T("Add from file"), f.addFromFiles),
			fyne.NewMenuItem(locale.T("Add WARP"), f.addWarp),
			fyne.NewMenuItem(locale.T("Add server"), f.addServer),
			// SPEC 116 W7 — четвёртый путь наполнения: заливка из уже
			// добавленной подписки. Последним и отделён: остальные три кладут
			// то, что человек принёс с собой, а этот — то, что уже есть в
			// «Источниках» (folder_fill_from_sub.go).
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem(locale.T("Fill from subscription…"), f.showFillFromSubscriptionDialog),
		)
		// Раскрытие — тем же приёмом, что у ⋮ в шапке Sources
		// (source_tab.go:301): меню встаёт ПОД кнопкой, по её MinSize.
		pop := widget.NewPopUpMenu(menu, f.win.Canvas())
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(btn)
		pop.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+btn.MinSize().Height))
	})
	btn.Importance = widget.MediumImportance
	return btn
}

// showPasteDialog — вставка текста: тот же вход, что у поля Add на вкладке
// Sources, только результат едет в папку.
func (f *folderAddNodes) showPasteDialog() {
	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapOff
	entry.SetMinRowsVisible(6)

	// Ловушка fyne-label-minwidth-trap: подсказка обязана переноситься, иначе
	// одна длинная строка задаёт диалогу min-width во весь экран.
	hint := widget.NewLabel(locale.T(folderAddNodesPasteHintText))
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance

	content := container.NewBorder(hint, nil, nil, nil, entry)

	d := dialog.NewCustomConfirm(
		locale.T("Add nodes to folder"), locale.T("Add"), locale.T("Cancel"), content,
		func(ok bool) {
			if !ok {
				return
			}
			f.applyInput(entry.Text, "")
		}, f.win)
	d.Resize(fyne.NewSize(640, 380))
	d.Show()
}

// addFromFiles — импорт МУЛЬТИВЫБОРОМ (features/sources.md §«Наполнение
// папки» п.1). Каждый файл разбирается своим вызовом ядра: имя файла — это
// имя для узлов ЭТОГО файла, и одним куском текста их было бы уже не
// различить.
//
// Fallback на внутридиалог Fyne — как у «Add from file» в Sources: там, где
// нативной панели нет (Linux без zenity/kdialog), выбор остаётся одиночным.
func (f *folderAddNodes) addFromFiles() {
	paths, ok, err := platform.PickOpenFiles(
		locale.T("Select config files (.conf / .vpn / .txt / .json)"),
		[]string{"conf", "vpn", "txt", "json"})
	if err == platform.ErrNativeDialogUnavailable {
		f.fyneFileOpen()
		return
	}
	if err != nil {
		dialog.ShowError(err, f.win)
		return
	}
	if !ok || len(paths) == 0 {
		return // cancelled
	}
	f.applyFiles(paths)
}

// fyneFileOpen — внутридиалог Fyne, одиночный выбор.
func (f *folderAddNodes) fyneFileOpen() {
	fd := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, f.win)
			return
		}
		if rc == nil {
			return // cancelled
		}
		defer func() { _ = rc.Close() }()
		text, rerr := wizardbusiness.ReadSourceFileText(rc)
		if rerr != nil {
			dialog.ShowError(rerr, f.win)
			return
		}
		if text == "" {
			return
		}
		// Имя из URI диалога: у него своё представление пути, а правило
		// «тег из имени файла» одно на оба входа.
		f.applyInput(text, wizardbusiness.TagFromFileName(rc.URI().Name()))
	}, f.win)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".conf", ".vpn", ".txt", ".json"}))
	fd.Show()
}

// applyFiles читает файлы и добавляет их узлы одной операцией.
//
// Один applySourceMutation на весь мультивыбор, а не по разу на файл:
// ревизия модели монотонна и пересобирает производные, и десять её подъёмов
// подряд означали бы десять пересборок пула кандидатов на одно
// пользовательское действие.
//
// Ошибка одного файла не отменяет остальные: человек выбрал десять конфигов,
// и «один из них не разобрался» — не повод потерять девять. Отвергнутые
// перечисляются в конце одним сообщением.
func (f *folderAddNodes) applyFiles(paths []string) {
	m := f.ops.presenter.Model()
	if m == nil {
		return
	}
	var total wizardbusiness.FolderFillResult
	var failed []string
	for _, path := range paths {
		name := wizardbusiness.TagFromFileName(path)
		fh, oerr := os.Open(path)
		if oerr != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", name, oerr))
			continue
		}
		text, rerr := wizardbusiness.ReadSourceFileText(fh)
		_ = fh.Close()
		if rerr != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", name, rerr))
			continue
		}
		if text == "" {
			continue
		}
		res, aerr := wizardbusiness.AppendNodesToFolder(m, f.folderID, text, name)
		if aerr != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", name, f.humanError(aerr)))
			continue
		}
		total.Added += res.Added
		total.SkippedSubscriptions += res.SkippedSubscriptions
	}
	f.finish(total, failed)
}

// applyInput — один текст → узлы папки.
func (f *folderAddNodes) applyInput(text, defaultTag string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	m := f.ops.presenter.Model()
	if m == nil {
		return
	}
	res, err := wizardbusiness.AppendNodesToFolder(m, f.folderID, text, defaultTag)
	if err != nil {
		dialog.ShowError(f.humanError(err), f.win)
		return
	}
	f.finish(res, nil)
}

// humanError переводит машинную формулировку ядра в текст локали.
//
// Единственный случай, который стоит перевода, — подписка в папке: остальные
// ошибки ядра («no valid URLs to add», «битый JSON») пользователь читает
// как есть, они уже так показываются на вкладке Sources.
//
// Сравнение сентинелом, а не подстрокой сообщения: правка формулировки в
// business не должна тихо отключать перевод.
func (f *folderAddNodes) humanError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, wizardbusiness.ErrSubscriptionInFolder) {
		return fmt.Errorf("%s", locale.T(folderAddNodesSubscriptionText))
	}
	return err
}

// addWarp — генератор Cloudflare WARP: тот же диалог, что в Sources, только
// выданный wireguard://-URI едет в папку.
func (f *folderAddNodes) addWarp() {
	wizarddialogs.ShowAddWarpDialog(f.ops.presenter, f.win, func(uri string) {
		f.applyInput(uri, "")
	})
}

// addServer — ручная форма источника. У неё два исхода, и оба обязаны
// доехать до папки одним путём: текстовый вход идёт ядром как есть, а
// вручную отредактированный JSON — тем же ядром (carveSingboxJSON узнаёт
// одиночный outbound и кладёт его тело побайтово).
//
// `res.Label` становится ИМЕНЕМ безымянного узла: у формы одно поле тега на
// весь ввод, ровно как имя файла у импорта.
func (f *folderAddNodes) addServer() {
	wizarddialogs.ShowAddServerDialog(f.ops.presenter, f.win, func(res wizarddialogs.AddServerResult) {
		text := strings.TrimSpace(res.Text)
		if len(res.ConfigJSON) > 0 {
			text = string(res.ConfigJSON)
		}
		f.applyInput(text, strings.TrimSpace(res.Label))
	})
}

// finish — общий хвост: побочки мутации, перечитывание рабочей копии окна и
// отчёт пользователю.
//
// Порядок обязателен и тот же, что у операций над узлом: сначала побочки,
// потом reloadScratch (иначе Save запишет снимок, снятый ДО добавления, и
// молча выбросит новые узлы), и только затем модальные сообщения.
// Отброшенная подписочная строка попадает в тот же список отвергнутого, что и
// нечитаемый файл: смешанный вход не роняет операцию, но и не проглатывает
// строку молча.
func (f *folderAddNodes) finish(res wizardbusiness.FolderFillResult, failed []string) {
	if res.Added > 0 {
		applySourceMutation(f.ops.presenter, f.ops.guiState)
		f.ops.afterModelMutation()
	}
	if res.SkippedSubscriptions > 0 {
		failed = append(failed, locale.T(folderAddNodesSubscriptionText))
	}
	if len(failed) > 0 {
		body := widget.NewLabel(strings.Join(failed, "\n"))
		body.Wrapping = fyne.TextWrapWord
		d := dialog.NewCustom(
			locale.T("Some files were not added"), locale.T("OK"),
			container.NewVScroll(body), f.win)
		d.Resize(fyne.NewSize(560, 320))
		d.Show()
		return
	}
	if res.Added == 0 {
		// Нажатие без результата — тоже результат, о нём говорят.
		dialogs.ShowAutoHideInfo(fyne.CurrentApp(), f.win,
			locale.T("Nothing added"), locale.T(addNothingAddedText))
		return
	}
	dialogs.ShowAutoHideInfo(fyne.CurrentApp(), f.win,
		locale.T("Nodes added"),
		locale.Tf("%d node(s) added to the folder.", res.Added))
}

// folderAddNodesHeader — шапка списка узлов: счётчик слева, кнопка справа.
//
// Для не-папки возвращает счётчик как был: строка состояния существует у
// всех видов источника, а кнопка — только у папки.
func folderAddNodesHeader(status fyne.CanvasObject, add *folderAddNodes) fyne.CanvasObject {
	if add == nil {
		return status
	}
	return container.NewBorder(nil, nil, nil, add.button(), status)
}
