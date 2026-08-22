// File files_tab.go — вкладка «Files»: собрать config.json в файл,
// посмотреть его и перенести настройки между машинами.
//
// Три действия над готовым состоянием, а не режим работы: всё это уходит в
// файлы и приходит из них. Поэтому собраны на последней вкладке, за
// настройками, а не разбросаны по ним.
//
// Конфиг показывается ПО КНОПКЕ и в ОТДЕЛЬНОМ окне. По кнопке — потому что
// сборка на большой подписке занимает секунды, и делать её на каждое
// открытие вкладки значило бы платить этим за информацию, которая нужна
// изредка (прежняя вкладка Preview именно так и работала). В отдельном
// окне — потому что конфиг читают целиком, а внутри вкладки ему достаётся
// половина высоты, и та отнимается у остального содержимого.
package tabs

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// CreateFilesTab строит вкладку.
func CreateFilesTab(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState) fyne.CanvasObject {
	win := guiState.Window

	// Статус нужен только показу конфига: результат сохранения показывает
	// сам SaveConfig — прогресс на кнопке внизу и диалог по завершении.
	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Alignment = fyne.TextAlignCenter

	// build собирает конфиг, синхронизируя модель с формой. Возвращает
	// текст либо ошибку, уже показанную пользователю.
	build := func() (string, bool) {
		presenter.MergeGUIToModel()
		text, err := wizardbusiness.BuildPreviewConfig(presenter.Model())
		if err != nil {
			debuglog.ErrorLog("files: сборка конфига: %v", err)
			status.SetText(locale.Tf("wizard.generate.status_error", err))
			return "", false
		}
		return text, true
	}

	showBtn := widget.NewButton(locale.T("wizard.generate.button_show"), func() {
		status.SetText(locale.T("wizard.generate.status_building"))
		text, ok := build()
		if !ok {
			return
		}
		status.SetText(locale.Tf("wizard.generate.status_built", len(text)))
		showConfigWindow(text)
	})

	// Та же операция, что у кнопки Save справа внизу: записать state.json и
	// пересобрать рабочий config.json. Не «выгрузить копию куда-то» —
	// диалог «куда сохранить» здесь означал бы вторую, побочную копию, а
	// пользователю нужно применить настройки.
	saveBtn := widget.NewButton(locale.T("wizard.files.button_save"), func() {
		presenter.SaveConfig()
	})
	saveBtn.Importance = widget.HighImportance

	hint := widget.NewLabel(locale.T("wizard.generate.hint"))
	hint.Wrapping = fyne.TextWrapWord

	// Кнопки по центру: спейсеры по краям, а не HBox слева — сборка конфига
	// главное действие вкладки, и прижимать его к левому краю рядом с
	// подписью значит прятать.
	buttons := container.NewHBox(
		layout.NewSpacer(),
		saveBtn,
		showBtn,
		layout.NewSpacer(),
	)

	body := container.NewVBox(
		hint,
		buttons,
		status,
		backupSection(presenter, win),
	)
	return container.NewVScroll(body)
}

// showConfigWindow открывает собранный конфиг в собственном окне.
//
// Application.NewWindow, а не модальный диалог: диалог живёт внутри канвы
// родителя и не может быть выше её, а конфиг — это тысячи строк, которые
// читают, прокручивают и копируют. Своё окно пользователь разворачивает и
// двигает, оставляя визард открытым рядом.
func showConfigWindow(text string) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	w := app.NewWindow(locale.T("wizard.generate.window_title"))

	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapOff
	entry.SetText(text)
	// Только для чтения: править собранный конфиг здесь бессмысленно — он
	// пересобирается из состояния при каждой сборке. Но выделение и
	// копирование остаются, ради них поле и взято вместо Label.
	entry.OnChanged = func(string) {}

	closeBtn := widget.NewButton(locale.T("wizard.outbound.button_cancel"), func() { w.Close() })
	w.SetContent(container.NewBorder(
		nil,
		container.NewHBox(layout.NewSpacer(), closeBtn),
		nil, nil,
		container.NewVScroll(entry),
	))
	w.Resize(fyne.NewSize(820, 640))
	w.CenterOnScreen()
	w.Show()
}
