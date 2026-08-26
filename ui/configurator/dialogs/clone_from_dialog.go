package dialogs

// clone_from_dialog.go — диалог «Clone from…»: перенос настроек с другой
// машины на текущую.
//
// Зачем отдельно от Read и от LX Backup. Read листает снапшоты ТОЛЬКО своей
// машины (store'ы машин изолированы by design), поэтому на свежезаведённой
// машине ему нечего показать. LX Backup умеет переносить, но через файл на
// диске — оправданно между лаунчером и телефоном, избыточно между двумя
// машинами одного лаунчера: оба состояния лежат рядом в bin/wizard_states/.
//
// Диалог из двух шагов, а не одного: сперва «откуда», потом «что приедет и
// что останется своим». Второй шаг обязателен, потому что клон ЗАМЕНЯЕТ
// состояние машины целиком — цена ошибки высокая, а список цифр («12
// источников, 30 правил») сразу показывает, ту ли машину выбрали.

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/debuglog"
	internaldialogs "singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// CloneFromResult — результат выбора источника.
type CloneFromResult struct {
	Action string // "clone", "cancel"
	Source wizardbusiness.CloneSource
}

// ShowCloneFromDialog показывает список машин-доноров.
//
// sources уже отфильтрован (без текущей машины) и размечен HasState.
func ShowCloneFromDialog(
	presenter *wizardpresentation.WizardPresenter,
	sources []wizardbusiness.CloneSource,
	onResult func(CloneFromResult),
) {
	guiState := presenter.GUIState()
	if guiState.Window == nil {
		onResult(CloneFromResult{Action: "cancel"})
		return
	}

	// Ни одного другого профиля — говорим об этом прямо. Пустой список в
	// окне выглядел бы как «не загрузилось».
	if len(sources) == 0 {
		dialog.ShowInformation(
			locale.T("Clone from"),
			locale.T("There are no other machines to clone from. Add a machine on the Remote tab, or configure the local one first."),
			guiState.Window)
		onResult(CloneFromResult{Action: "cancel"})
		return
	}

	selected := -1
	// Первым выделяем первый источник, У КОТОРОГО ЕСТЬ состояние: выделять
	// заведомо пустую строку значило бы предлагать заведомо неработающее
	// действие.
	for i, s := range sources {
		if s.HasState {
			selected = i
			break
		}
	}

	list := widget.NewList(
		func() int { return len(sources) },
		func() fyne.CanvasObject {
			return container.NewHBox(widget.NewLabel(""), layout.NewSpacer(), widget.NewLabel(""))
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= widget.ListItemID(len(sources)) {
				return
			}
			row := obj.(*fyne.Container)
			name := row.Objects[0].(*widget.Label)
			hint := row.Objects[2].(*widget.Label)

			src := sources[id]
			name.SetText(src.Name)
			name.TextStyle = fyne.TextStyle{Bold: src.HasState}

			// Правая колонка отвечает на «можно ли отсюда клонировать» и,
			// если можно, «что это за машина».
			switch {
			case !src.HasState:
				hint.SetText(locale.T("not configured yet"))
			case src.Platform != "":
				hint.SetText(src.Platform)
			default:
				hint.SetText("")
			}
			name.Refresh()
			hint.Refresh()
		},
	)
	if selected >= 0 {
		list.Select(widget.ListItemID(selected))
	}
	list.OnSelected = func(id widget.ListItemID) { selected = int(id) }

	var dialogWindow dialog.Dialog

	cloneButton := widget.NewButton(locale.T("Clone"), func() {
		if selected < 0 || selected >= len(sources) {
			return
		}
		src := sources[selected]
		// Источник без состояния — не ошибка пользователя, а пустая машина.
		// Объясняем, вместо того чтобы молча ничего не сделать.
		if !src.HasState {
			dialog.ShowInformation(
				locale.T("Clone from"),
				locale.Tf("%s has no saved configuration yet — there is nothing to clone from it.", src.Name),
				guiState.Window)
			return
		}
		if dialogWindow != nil {
			dialogWindow.Hide()
		}
		onResult(CloneFromResult{Action: "clone", Source: src})
	})
	cloneButton.Importance = widget.HighImportance

	hint := widget.NewLabel(locale.T("Pick the machine whose settings you want to copy here."))
	hint.Wrapping = fyne.TextWrapWord

	scrollList := container.NewScroll(list)
	scrollList.SetMinSize(fyne.NewSize(340, 160))

	content := container.NewBorder(hint, nil, nil, nil, scrollList)
	buttons := container.NewHBox(layout.NewSpacer(), cloneButton)

	dialogWindow = internaldialogs.NewCustom(
		locale.T("Clone from"), content, buttons, locale.T("Cancel"), guiState.Window)
	dialogWindow.Resize(fyne.NewSize(400, 300))
	dialogWindow.SetOnClosed(func() {
		onResult(CloneFromResult{Action: "cancel"})
	})
	dialogWindow.Show()
}

// ShowClonePreviewDialog — второй шаг: что приедет и что останется своим.
//
// Отдельным экраном, а не строчкой в первом: замена состояния целиком
// заслуживает подтверждения, которое видно, а сводка отвечает на «ту ли
// машину я выбрал» лучше, чем её имя.
func ShowClonePreviewDialog(
	win fyne.Window,
	src wizardbusiness.CloneSource,
	summary wizardbusiness.CloneSummary,
	onConfirm func(bool),
) {
	var sb strings.Builder

	sb.WriteString(locale.Tf("Copying settings from %s.", src.Name))
	sb.WriteString("\n\n")
	sb.WriteString(locale.T("Will be copied:"))
	sb.WriteString("\n")
	sb.WriteString(locale.Tf("  Subscriptions: %d", summary.Subscriptions))
	sb.WriteString("\n")
	sb.WriteString(locale.Tf("  Servers: %d", summary.Servers))
	sb.WriteString("\n")
	sb.WriteString(locale.Tf("  Chains: %d", summary.Chains))
	sb.WriteString("\n")
	sb.WriteString(locale.Tf("  Directions: %d", summary.Directions))
	sb.WriteString("\n")
	sb.WriteString(locale.Tf("  Rules: %d", summary.Rules))
	sb.WriteString("\n")
	sb.WriteString(locale.Tf("  Variables: %d", summary.Vars))

	// Что НЕ переносится — не мелким шрифтом внизу, а такой же строкой:
	// пользователь должен уйти отсюда, зная, что интерфейсы шлюза и
	// исходящий интерфейс у этой машины остались свои, а не «почему-то не
	// применились».
	if len(summary.SkippedVars) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(locale.T("Stays machine-specific (not copied):"))
		sb.WriteString("\n  ")
		labels := make([]string, 0, len(summary.SkippedVars))
		for _, name := range summary.SkippedVars {
			labels = append(labels, locale.T(name))
		}
		sb.WriteString(strings.Join(labels, ", "))
	}

	sb.WriteString("\n\n")
	sb.WriteString(locale.T("This replaces the current settings of this machine. The current state is saved as a snapshot first — you can get it back with Read."))

	body := widget.NewLabel(sb.String())
	body.Wrapping = fyne.TextWrapWord

	scroll := container.NewScroll(body)
	scroll.SetMinSize(fyne.NewSize(380, 260))

	dialog.ShowCustomConfirm(
		locale.T("Clone from"),
		locale.T("Clone"),
		locale.T("Cancel"),
		scroll,
		func(ok bool) {
			if !ok {
				debuglog.DebugLog("clone: preview cancelled for %q", src.Name)
			}
			onConfirm(ok)
		}, win)
}
