// File generate_tab.go — вкладка «Generate»: собрать config.json в файл,
// посмотреть его и перенести настройки между машинами.
//
// Три действия над готовым состоянием, а не режим работы: сборка конфига,
// просмотр результата и импорт/экспорт бэкапа. Поэтому они собраны на
// последней вкладке, за настройками, а не разбросаны по ним.
//
// Превью показывается ПО КНОПКЕ, а не постоянно: сборка конфига на большой
// подписке занимает секунды, и делать её на каждое открытие вкладки значило
// бы платить этим за информацию, которая нужна изредка. Прежняя вкладка
// Preview именно так и работала — пересчитывалась на каждое переключение.
package tabs

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// configFileName — имя по умолчанию для выгружаемого конфига.
const configFileName = "config.json"

// CreateGenerateTab строит вкладку.
func CreateGenerateTab(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState) fyne.CanvasObject {
	win := guiState.Window

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord

	previewEntry := widget.NewMultiLineEntry()
	previewEntry.Wrapping = fyne.TextWrapOff
	// Поле только для чтения: править собранный конфиг здесь бессмысленно —
	// он пересобирается из состояния на каждой сборке.
	previewEntry.OnChanged = func(string) {}

	previewBox := container.NewStack(
		canvas.NewRectangle(color.Transparent),
		previewEntry,
	)
	previewScroll := container.NewVScroll(previewBox)
	previewScroll.SetMinSize(adaptiveScrollSize(guiState, 0.45, 320))
	// Спрятано до первого показа: пустая рамка в пол-экрана выглядит как
	// сломанная вкладка.
	previewScroll.Hide()

	// build собирает конфиг, синхронизируя модель с формой. Возвращает
	// текст либо ошибку, уже показанную пользователю.
	build := func() (string, bool) {
		presenter.MergeGUIToModel()
		text, err := wizardbusiness.BuildPreviewConfig(presenter.Model())
		if err != nil {
			debuglog.ErrorLog("generate: сборка конфига: %v", err)
			status.SetText(locale.Tf("wizard.generate.status_error", err))
			return "", false
		}
		return text, true
	}

	var showBtn *widget.Button
	showBtn = widget.NewButton(locale.T("wizard.generate.button_show"), func() {
		if previewScroll.Visible() {
			previewScroll.Hide()
			showBtn.SetText(locale.T("wizard.generate.button_show"))
			return
		}
		status.SetText(locale.T("wizard.generate.status_building"))
		text, ok := build()
		if !ok {
			return
		}
		previewEntry.SetText(text)
		previewScroll.Show()
		showBtn.SetText(locale.T("wizard.generate.button_hide"))
		status.SetText(locale.Tf("wizard.generate.status_built", len(text)))
	})

	saveBtn := widget.NewButton(locale.T("wizard.generate.button_save"), func() {
		status.SetText(locale.T("wizard.generate.status_building"))
		text, ok := build()
		if !ok {
			return
		}
		path, picked, err := platform.PickSaveFile(
			locale.T("wizard.generate.save_prompt"), configFileName)
		if err != nil && err != platform.ErrNativeDialogUnavailable {
			debuglog.WarnLog("generate: диалог сохранения: %v", err)
		}
		switch {
		case err == platform.ErrNativeDialogUnavailable:
			// Нативного диалога нет — кладём рядом с исполняемым файлом и
			// говорим куда: молча ничего не делать хуже.
			path = filepath.Join(defaultBackupDir(), configFileName)
		case !picked:
			status.SetText("")
			return // отмена пользователя
		}
		if !strings.EqualFold(filepath.Ext(path), ".json") {
			path += ".json"
		}
		if err := writeTextFile(path, text); err != nil {
			debuglog.ErrorLog("generate: запись %s: %v", path, err)
			dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.generate.error_write"), err), win)
			status.SetText(locale.Tf("wizard.generate.status_error", err))
			return
		}
		status.SetText(locale.Tf("wizard.generate.status_saved", path))
	})

	hint := widget.NewLabel(locale.T("wizard.generate.hint"))
	hint.Wrapping = fyne.TextWrapWord

	head := container.NewVBox(
		hint,
		container.NewHBox(saveBtn, showBtn),
		status,
	)

	body := container.NewVBox(
		head,
		previewScroll,
		backupSection(presenter, win),
	)
	return container.NewVScroll(body)
}
