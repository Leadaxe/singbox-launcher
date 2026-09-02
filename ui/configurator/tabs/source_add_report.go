// File source_add_report.go — отчёт кнопки Add на вкладке Sources.
//
// Нажатие Add не бывает молчаливым: любой исход — добавлено, всё уже есть,
// не разобралось — называется пользователю. Ошибки разбора показывает
// вызывающий диалогом (там текст ядра), здесь — успешные и пустые исходы.
package tabs

import (
	"strings"

	"fyne.io/fyne/v2"

	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// addNothingAddedText — вход разобрался, но положить оказалось нечего.
const addNothingAddedText = "The input was recognized, but nothing was added." // l10n-key

// reportAddSourcesResult — короткое сообщение об итоге корневого Add.
//
// Дубликаты называются отдельно: «добавлено 0» без причины читается как
// поломка, а «уже в списке» — как ответ.
func reportAddSourcesResult(win fyne.Window, res wizardbusiness.AddSourcesResult) {
	if win == nil {
		return
	}
	var parts []string
	if res.Subscriptions > 0 {
		parts = append(parts, locale.Tf("%d subscription(s)", res.Subscriptions))
	}
	if res.Folders > 0 {
		parts = append(parts, locale.Tf("%d folder(s)", res.Folders))
	}
	if res.Nodes > 0 {
		parts = append(parts, locale.Tf("%d node(s)", res.Nodes))
	}
	if len(parts) == 0 {
		msg := locale.T(addNothingAddedText)
		if res.Duplicates > 0 {
			msg = locale.Tf("%d item(s) are already in the list.", res.Duplicates)
		}
		dialogs.ShowAutoHideInfo(fyne.CurrentApp(), win, locale.T("Nothing added"), msg)
		return
	}
	msg := locale.Tf("Added: %s.", strings.Join(parts, ", "))
	if res.Duplicates > 0 {
		msg += " " + locale.Tf("%d item(s) skipped — already in the list.", res.Duplicates)
	}
	dialogs.ShowAutoHideInfo(fyne.CurrentApp(), win, locale.T("Sources added"), msg)
}
