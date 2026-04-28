// Package dialogs содержит диалоговые окна визарда конфигурации.
//
// Файл save_state_dialog.go содержит функцию ShowSaveStateDialog, которая создает диалоговое окно
// для сохранения состояния визарда под новым ID:
//   - Поле ввода ID (обязательное) с валидацией
//   - Поле ввода комментария (необязательное)
//   - Предупреждение, если ID уже существует
//   - Buttons: "Save", "Cancel"
//
// Диалог используется в двух сценариях:
//   - При нажатии кнопки "Save As"
//   - При нажатии кнопки "Save", если state.json уже существует (для сохранения предыдущего состояния)
//
// Используется в:
//   - presentation/presenter_state.go - при сохранении состояния
package dialogs

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	internaldialogs "singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// SaveStateResult представляет результат диалога сохранения состояния.
type SaveStateResult struct {
	Action  string // "save", "cancel"
	ID      string
	Comment string
}

// ShowSaveStateDialog показывает диалог сохранения состояния.
// Возвращает результат через callback.
func ShowSaveStateDialog(presenter *wizardpresentation.WizardPresenter, onResult func(SaveStateResult)) {
	guiState := presenter.GUIState()
	if guiState.Window == nil {
		onResult(SaveStateResult{Action: "cancel"})
		return
	}

	// Input fields
	idEntry := widget.NewEntry()
	idEntry.SetPlaceHolder(locale.T("wizard.save_state.placeholder_id"))

	commentEntry := widget.NewMultiLineEntry()
	commentEntry.SetPlaceHolder(locale.T("wizard.save_state.placeholder_comment"))
	commentEntry.Wrapping = fyne.TextWrapWord

	// Предупреждение о существующем ID
	warningLabel := widget.NewLabel("")
	warningLabel.Hide()

	// ID validation function
	validateID := func() (string, error) {
		id := idEntry.Text
		if id == "" {
			return "", fmt.Errorf("%s", locale.T("wizard.save_state.error_empty"))
		}
		if err := wizardmodels.ValidateStateID(id); err != nil {
			return "", err
		}
		return id, nil
	}

	// Функция проверки существования ID
	checkIDExists := func(id string) bool {
		stateStore := presenter.GetStateStore()
		return stateStore.StateExists(id)
	}

	// Обновление предупреждения при изменении ID
	idEntry.OnChanged = func(text string) {
		id, err := validateID()
		if err != nil {
			warningLabel.Hide()
			return
		}
		if checkIDExists(id) {
			warningLabel.SetText(locale.T("wizard.save_state.warning_exists"))
			warningLabel.Show()
		} else {
			warningLabel.Hide()
		}
	}

	// Buttons
	var dialogWindow dialog.Dialog
	saveButton := widget.NewButton(locale.T("wizard.save_state.button_save"), func() {
		id, err := validateID()
		if err != nil {
			dialog.ShowError(err, guiState.Window)
			return
		}

		comment := commentEntry.Text
		if dialogWindow != nil {
			dialogWindow.Hide()
		}
		onResult(SaveStateResult{
			Action:  "save",
			ID:      id,
			Comment: comment,
		})
	})
	saveButton.Importance = widget.HighImportance

	// Fields container
	fieldsContainer := container.NewVBox(
		widget.NewLabel(locale.T("wizard.save_state.label_id")),
		idEntry,
		widget.NewLabel(locale.T("wizard.save_state.label_comment")),
		container.NewScroll(commentEntry),
		warningLabel,
	)

	// Buttons container (без cancelButton - он будет через dismissText)
	buttonsContainer := container.NewHBox(
		layout.NewSpacer(),
		saveButton,
	)

	// Сохраняем оригинальный обработчик клавиатуры до создания диалога
	originalOnTypedKey := guiState.Window.Canvas().OnTypedKey()

	// Create dialog with simplified API (cancelButton через dismissText, ESC обрабатывается автоматически)
	dialogWindow = internaldialogs.NewCustom(locale.T("wizard.save_state.title"), fieldsContainer, buttonsContainer, locale.T("wizard.save_state.button_cancel"), guiState.Window)
	dialogWindow.Resize(fyne.NewSize(400, 300))

	// Обработчик для cancelButton через dismissText и ESC
	// internaldialogs.NewCustom уже устанавливает обработчик для восстановления клавиатуры,
	// поэтому мы перезаписываем его, но сохраняем логику восстановления
	dialogWindow.SetOnClosed(func() {
		// Восстанавливаем оригинальный обработчик клавиатуры
		if originalOnTypedKey != nil {
			guiState.Window.Canvas().SetOnTypedKey(originalOnTypedKey)
		} else {
			guiState.Window.Canvas().SetOnTypedKey(nil)
		}
		// Вызываем callback для cancel (если диалог закрыт через Cancel или ESC)
		onResult(SaveStateResult{Action: "cancel"})
	})
	dialogWindow.Resize(fyne.NewSize(400, 300))
	dialogWindow.Show()

	// Focus on ID field
	idEntry.FocusGained()
}
