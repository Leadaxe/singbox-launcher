// File folder_fill_from_sub.go — «Fill from subscription…» в окне ПАПКИ
// (SPEC 116 этап 3, W7; цель П4, сценарий С5).
//
// # Почему пункт того же меню, что «Add nodes…»
//
// Это четвёртый путь наполнения папки, и место у него там же, где у трёх
// первых: в шапке списка узлов, под одной кнопкой. Отдельная кнопка рядом
// разводила бы «добавить узлы» и «добавить узлы из подписки» как разные по
// смыслу действия, а они различаются только источником материала.
//
// # Почему нет отдельного «обновить папку»
//
// Повторная заливка той же подписки — тот же пункт меню (сценарий С5): merge
// идемпотентен по построению, и второй проход на неизменившемся материале не
// трогает ничего. Второй пункт «обновить» пришлось бы держать в
// синхронизации с первым, а пользователю — угадывать, чем они отличаются.
//
// # Подписка без узлов
//
// Материала нет, добывать его здесь нечем (тела подписок в модели нет, а
// второго разбора мы не заводим — см. шапку folder_fill_subscription.go).
// Поэтому заливка отказывается и предлагает обновление ТЕМ ЖЕ путём, что
// кнопка ↻ в списке источников — refreshOneSourceFromUI. Своего fetch'а тут
// нет: две точки обновления одной подписки разъехались бы диагностикой.
package tabs

import (
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	folderFillPickHintText = "Nodes of the chosen subscription are merged into this folder by raw tag: your enabled flags, detours and order survive, and a node that disappeared at the provider stays in the folder."
	// Отдельный текст, а не общая ошибка: пользователь выбрал существующую
	// подписку — сказать надо не «не вышло», а «её ещё ни разу не обновляли»
	// и предложить сделать это.
	folderFillNoNodesText = "Subscription %q has no nodes yet — it has never been fetched. Refresh it now?"
	folderFillNoSubsText  = "Add a subscription in Sources first, then fill the folder from it."
)

// showFillFromSubscriptionDialog — выбор подписки-донора.
//
// Ловушка fyne-label-minwidth-trap: подсказка обязана переноситься, иначе одна
// длинная строка задаёт диалогу min-width во весь экран.
func (f *folderAddNodes) showFillFromSubscriptionDialog() {
	m := f.ops.presenter.Model()
	if m == nil {
		return
	}
	choices := wizardbusiness.FolderFillSubscriptions(m)
	if len(choices) == 0 {
		dialog.ShowInformation(
			locale.T("No subscriptions yet"),
			locale.T(folderFillNoSubsText),
			f.win)
		return
	}

	labels := make([]string, 0, len(choices))
	seen := map[string]int{}
	for _, c := range choices {
		// Имена подписок не уникальны; ключ заливки — URL, поэтому именно он
		// различает две одноимённые. Совпал и он — дописываем номер, как в
		// селекте папок (preview_node_ops.folderTargets).
		label := c.Name
		if c.URL != "" && c.URL != c.Name {
			label = fmt.Sprintf("%s — %s", c.Name, c.URL)
		}
		if c.NodeCount == 0 {
			label = fmt.Sprintf("%s · %s", label, locale.T("not fetched yet"))
		} else {
			label = fmt.Sprintf("%s · %s", label, locale.Tf("%d node(s)", c.NodeCount))
		}
		seen[label]++
		if n := seen[label]; n > 1 {
			label = fmt.Sprintf("%s (%d)", label, n)
		}
		labels = append(labels, label)
	}

	hint := widget.NewLabel(locale.T(folderFillPickHintText))
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance

	sel := widget.NewSelect(labels, nil)
	sel.SetSelectedIndex(0)

	content := container.NewVBox(hint, sel)

	d := dialog.NewCustomConfirm(
		locale.T("Fill folder from subscription"), locale.T("Fill"), locale.T("Cancel"), content,
		func(ok bool) {
			if !ok {
				return
			}
			i := sel.SelectedIndex()
			if i < 0 || i >= len(choices) {
				return
			}
			f.applyFillFromSubscription(choices[i])
		}, f.win)
	d.Resize(fyne.NewSize(620, 260))
	d.Show()
}

// applyFillFromSubscription — заливка одной подписки.
//
// Мутация НЕМЕДЛЕННАЯ, как у остальных операций над составом (см. шапку
// preview_node_ops.go): заливка трогает узлы папки, а окно правит
// value-snapshot — буферизация дала бы Cancel откатить уже показанный
// результат. Отсюда обязательные applySourceMutation + afterModelMutation.
func (f *folderAddNodes) applyFillFromSubscription(choice wizardbusiness.FolderFillSubscriptionChoice) {
	m := f.ops.presenter.Model()
	if m == nil {
		return
	}
	res, err := wizardbusiness.FillFolderFromSubscription(m, f.folderID, choice.ID)
	if err != nil {
		if errors.Is(err, wizardbusiness.ErrSubscriptionNotFetched) {
			f.offerSubscriptionRefresh(choice)
			return
		}
		dialog.ShowError(err, f.win)
		return
	}

	if res.Changed {
		applySourceMutation(f.ops.presenter, f.ops.guiState)
		f.ops.afterModelMutation()
	}
	f.reportFillResult(res)
}

// offerSubscriptionRefresh — подписку ни разу не обновляли: заливать нечего.
//
// Обновление зовётся существующим путём списка источников
// (refreshOneSourceFromUI): он один на все точки, и второй fetch дал бы вторую
// диагностику и второй набор гонок с фоновым обновлением. Заливку после него
// пользователь повторяет сам — молча дозаливать по завершении горутины
// значило бы делать за него выбор, которого он не делал: обновление могло
// прийти совсем не тем, чего он ждал.
func (f *folderAddNodes) offerSubscriptionRefresh(choice wizardbusiness.FolderFillSubscriptionChoice) {
	dialog.ShowConfirm(
		locale.T("Subscription has not been fetched yet"),
		locale.Tf(folderFillNoNodesText, choice.Name),
		func(ok bool) {
			if !ok {
				return
			}
			refreshOneSourceFromUI(f.ops.presenter, f.ops.guiState, choice.ID)
		}, f.win)
}

// reportFillResult — исход заливки пользователю.
//
// Деградации merge (исчез у провайдера, занятый тег, prune члена группы)
// показываются списком тем же окном, что и отвергнутые файлы у «Add nodes…»:
// заливка их не роняет, но и не проглатывает. Тексты приходят из ядра как
// диагностика и через locale не идут — переводить их значило бы держать в
// locale зеркало формулировок merge.
func (f *folderAddNodes) reportFillResult(res wizardbusiness.FolderSubscriptionFillResult) {
	if len(res.Warnings) > 0 {
		body := widget.NewLabel(strings.Join(res.Warnings, "\n"))
		body.Wrapping = fyne.TextWrapWord
		d := dialog.NewCustom(
			locale.T("Filled with warnings"), locale.T("OK"),
			container.NewVScroll(body), f.win)
		d.Resize(fyne.NewSize(620, 340))
		d.Show()
		return
	}
	if !res.Changed {
		// Повторная заливка неизменившейся подписки — законный исход, и
		// молчание тут читалось бы как «кнопка не сработала».
		dialogs.ShowAutoHideInfo(fyne.CurrentApp(), f.win,
			locale.T("Folder filled"),
			locale.T("The folder is already up to date with this subscription."))
		return
	}
	dialogs.ShowAutoHideInfo(fyne.CurrentApp(), f.win,
		locale.T("Folder filled"),
		locale.Tf("Folder filled from %q.", res.SubName))
}
