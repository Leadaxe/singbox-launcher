package tabs

// Список каналов роутинга и их редактирование (SPEC 104).
//
// Канал — именованная точка выбора, на которую ссылаются правила. Тег канала
// (`vpn-N`) неизменяем и пользователю не показывается как редактируемое поле:
// на него ссылаются правила, и переименование не должно ломать ссылки.
// Пользователь задаёт отображаемое имя и правила отбора узлов.

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// buildChannelsSection — секция «Каналы» на вкладке Outbounds.
func buildChannelsSection(presenter *wizardpresentation.WizardPresenter, win fyne.Window) fyne.CanvasObject {
	list := container.NewVBox()

	var refresh func()
	refresh = func() {
		model := presenter.Model()
		list.RemoveAll()

		if len(model.Channels) == 0 {
			hint := widget.NewLabel(locale.T("wizard.channels.empty"))
			hint.Wrapping = fyne.TextWrapWord
			list.Add(hint)
		}

		for i := range model.Channels {
			idx := i
			ch := model.Channels[idx]
			list.Add(buildChannelRow(presenter, win, ch, idx, refresh))
		}

		addBtn := widget.NewButton(locale.T("wizard.channels.add"), func() {
			model := presenter.Model()
			tag := corestate.NextChannelTag(model.Channels)
			if tag == "" {
				dialog.ShowInformation(
					locale.T("wizard.channels.limit_title"),
					fmt.Sprintf(locale.T("wizard.channels.limit_body"), corestate.MaxChannels),
					win)
				return
			}
			model.Channels = append(model.Channels, wizardmodels.Channel{
				Tag:                       tag,
				Label:                     defaultChannelLabel(tag),
				Enabled:                   true,
				InterruptExistConnections: true,
			})
			presenter.MarkAsChanged()
			refresh()
		})
		list.Add(addBtn)
		list.Refresh()
	}
	refresh()

	title := widget.NewLabelWithStyle(
		locale.T("wizard.channels.title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	hint := widget.NewLabel(locale.T("wizard.channels.hint"))
	hint.Wrapping = fyne.TextWrapWord

	return container.NewVBox(title, hint, list)
}

// buildChannelRow — одна строка списка: тумблер, имя, кнопки.
func buildChannelRow(
	presenter *wizardpresentation.WizardPresenter,
	win fyne.Window,
	ch wizardmodels.Channel,
	idx int,
	refresh func(),
) fyne.CanvasObject {
	toggle := widget.NewCheck("", func(v bool) {
		model := presenter.Model()
		if idx >= len(model.Channels) {
			return
		}
		model.Channels[idx].Enabled = v
		presenter.MarkAsChanged()
	})
	toggle.SetChecked(ch.Enabled)

	name := widget.NewLabel(ch.DisplayLabel())
	detail := widget.NewLabel(channelSummary(ch))
	detail.Importance = widget.LowImportance

	editBtn := widget.NewButton(locale.T("wizard.channels.edit"), func() {
		showChannelEditDialog(presenter, win, idx, refresh)
	})
	delBtn := widget.NewButton(locale.T("wizard.channels.delete"), func() {
		dialog.ShowConfirm(
			locale.T("wizard.channels.delete_title"),
			fmt.Sprintf(locale.T("wizard.channels.delete_body"), ch.DisplayLabel()),
			func(ok bool) {
				if !ok {
					return
				}
				model := presenter.Model()
				if idx >= len(model.Channels) {
					return
				}
				model.Channels = append(model.Channels[:idx], model.Channels[idx+1:]...)
				presenter.MarkAsChanged()
				refresh()
			}, win)
	})

	return container.NewBorder(
		nil, nil,
		container.NewHBox(toggle, name),
		container.NewHBox(editBtn, delBtn),
		detail,
	)
}

// channelSummary — краткое описание канала для строки списка.
func channelSummary(ch wizardmodels.Channel) string {
	parts := make([]string, 0, 4)
	if ch.NodeFilter != "" {
		verb := locale.T("wizard.channels.summary_filter")
		if ch.NodeFilterInvert {
			verb = locale.T("wizard.channels.summary_filter_invert")
		}
		parts = append(parts, verb+" "+ch.NodeFilter)
	} else {
		parts = append(parts, locale.T("wizard.channels.summary_all_nodes"))
	}
	if ch.Auto != nil {
		parts = append(parts, locale.T("wizard.channels.summary_auto"))
	}
	if ch.IncludeDirect {
		parts = append(parts, locale.T("wizard.channels.summary_direct"))
	}
	if ch.IncludeBlock {
		parts = append(parts, locale.T("wizard.channels.summary_block"))
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " · "
		}
		out += p
	}
	return out
}

// showChannelEditDialog — форма канала.
func showChannelEditDialog(
	presenter *wizardpresentation.WizardPresenter,
	win fyne.Window,
	idx int,
	refresh func(),
) {
	model := presenter.Model()
	if idx >= len(model.Channels) {
		return
	}
	ch := model.Channels[idx]

	labelEntry := widget.NewEntry()
	labelEntry.SetText(ch.Label)
	labelEntry.SetPlaceHolder(ch.Tag)

	filterEntry := widget.NewEntry()
	filterEntry.SetText(ch.NodeFilter)
	filterEntry.SetPlaceHolder(locale.T("wizard.channels.filter_placeholder"))

	invertCheck := widget.NewCheck(locale.T("wizard.channels.filter_invert"), nil)
	invertCheck.SetChecked(ch.NodeFilterInvert)

	defaultEntry := widget.NewEntry()
	defaultEntry.SetText(ch.DefaultFilter)
	defaultEntry.SetPlaceHolder(locale.T("wizard.channels.default_placeholder"))

	directCheck := widget.NewCheck(locale.T("wizard.channels.include_direct"), nil)
	directCheck.SetChecked(ch.IncludeDirect)

	blockCheck := widget.NewCheck(locale.T("wizard.channels.include_block"), nil)
	blockCheck.SetChecked(ch.IncludeBlock)

	autoCheck := widget.NewCheck(locale.T("wizard.channels.auto"), nil)
	autoCheck.SetChecked(ch.Auto != nil)

	autoURL := widget.NewEntry()
	autoInterval := widget.NewEntry()
	autoTolerance := widget.NewEntry()
	if ch.Auto != nil {
		autoURL.SetText(ch.Auto.URL)
		autoInterval.SetText(ch.Auto.Interval)
		if ch.Auto.Tolerance > 0 {
			autoTolerance.SetText(strconv.Itoa(ch.Auto.Tolerance))
		}
	}
	autoURL.SetPlaceHolder(locale.T("wizard.channels.auto_url_placeholder"))
	autoInterval.SetPlaceHolder("5m")
	autoTolerance.SetPlaceHolder("50")

	form := widget.NewForm(
		widget.NewFormItem(locale.T("wizard.channels.field_label"), labelEntry),
		widget.NewFormItem(locale.T("wizard.channels.field_filter"), filterEntry),
		widget.NewFormItem("", invertCheck),
		widget.NewFormItem(locale.T("wizard.channels.field_default"), defaultEntry),
		widget.NewFormItem("", directCheck),
		widget.NewFormItem("", blockCheck),
		widget.NewFormItem("", autoCheck),
		widget.NewFormItem(locale.T("wizard.channels.field_auto_url"), autoURL),
		widget.NewFormItem(locale.T("wizard.channels.field_auto_interval"), autoInterval),
		widget.NewFormItem(locale.T("wizard.channels.field_auto_tolerance"), autoTolerance),
	)

	d := dialog.NewCustomConfirm(
		locale.T("wizard.channels.edit_title"),
		locale.T("wizard.channels.save"),
		locale.T("dialog.button_cancel"),
		form,
		func(ok bool) {
			if !ok {
				return
			}
			model := presenter.Model()
			if idx >= len(model.Channels) {
				return
			}
			c := &model.Channels[idx]
			c.Label = labelEntry.Text
			c.NodeFilter = filterEntry.Text
			c.NodeFilterInvert = invertCheck.Checked
			c.DefaultFilter = defaultEntry.Text
			c.IncludeDirect = directCheck.Checked
			c.IncludeBlock = blockCheck.Checked

			if autoCheck.Checked {
				auto := &corestate.ChannelAuto{
					URL:                       autoURL.Text,
					Interval:                  autoInterval.Text,
					InterruptExistConnections: true,
				}
				if n, err := strconv.Atoi(autoTolerance.Text); err == nil && n > 0 {
					auto.Tolerance = n
				}
				c.Auto = auto
			} else {
				// nil, а не пустая структура: nil означает «автовыбора нет»,
				// и парная группа не эмитится вовсе.
				c.Auto = nil
			}

			presenter.MarkAsChanged()
			refresh()
		}, win)
	d.Resize(fyne.NewSize(520, 480))
	d.Show()
}

// defaultChannelLabel — имя нового канала по его тегу.
func defaultChannelLabel(tag string) string {
	return fmt.Sprintf(locale.T("wizard.channels.default_label"), tag)
}
