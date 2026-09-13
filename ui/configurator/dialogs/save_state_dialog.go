// Package dialogs содержит диалоговые окна визарда конфигурации.
//
// Файл save_state_dialog.go содержит функцию ShowSaveStateDialog, которая создает диалоговое окно
// для сохранения состояния визарда под новым ID:
//   - Поле ввода ID (обязательное) с валидацией
//   - Поле ввода комментария (необязательное)
//   - Предупреждение, если ID уже существует
//   - Чекбокс «сделать текущим состоянием и пересобрать конфиг» (по опции)
//   - Buttons: "Save", "Cancel"
//
// Диалог используется в двух сценариях:
//   - При нажатии кнопки "Save As" — с чекбоксом «сделать текущим» (#122):
//     снимок без state.json оставлял пользователя без config.json, а визард
//     при следующем открытии показывал пустую форму. Если state.json ещё
//     нет, чекбокс включён и заблокирован — иного смысла у Save As нет.
//   - При нажатии кнопки "Read" с несохранёнными изменениями — только снимок,
//     чекбокса нет: следом в модель грузится другое состояние.
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
	// MakeCurrent — снимок должен стать текущим состоянием (state.json +
	// маркеры + пересборка конфига). Всегда false, если диалог открыт без
	// SaveStateDialogOptions.OfferMakeCurrent.
	MakeCurrent bool
}

// SaveStateDialogOptions — поведение диалога сохранения состояния.
type SaveStateDialogOptions struct {
	// OfferMakeCurrent — показать чекбокс «сделать текущим состоянием и
	// пересобрать конфиг». Включён по умолчанию; без state.json заблокирован
	// во включённом положении.
	OfferMakeCurrent bool
}

// ShowSaveStateDialog показывает диалог сохранения состояния.
// Возвращает результат через callback.
func ShowSaveStateDialog(presenter *wizardpresentation.WizardPresenter, opts SaveStateDialogOptions, onResult func(SaveStateResult)) {
	guiState := presenter.GUIState()
	if guiState.Window == nil {
		onResult(SaveStateResult{Action: "cancel"})
		return
	}

	// Input fields
	idEntry := widget.NewEntry()
	idEntry.SetPlaceHolder(locale.T("Enter state ID (a-z, A-Z, 0-9, -, _)"))

	commentEntry := widget.NewMultiLineEntry()
	commentEntry.SetPlaceHolder(locale.T("Comment (optional)"))
	commentEntry.Wrapping = fyne.TextWrapWord

	// Предупреждение о существующем ID
	warningLabel := widget.NewLabel("")
	warningLabel.Hide()

	// ID validation function
	validateID := func() (string, error) {
		id := idEntry.Text
		if id == "" {
			return "", fmt.Errorf("%s", locale.T("ID cannot be empty"))
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
			warningLabel.SetText(locale.T("State with this ID already exists. It will be overwritten."))
			warningLabel.Show()
		} else {
			warningLabel.Hide()
		}
	}

	// Чекбокс «сделать текущим». Без state.json выбор мнимый: снимок сам по
	// себе не даёт config.json (#122), поэтому чекбокс включён и заблокирован.
	var makeCurrentCheck *widget.Check
	if opts.OfferMakeCurrent {
		makeCurrentCheck = widget.NewCheck(locale.T("Make it the current state and rebuild config"), nil)
		makeCurrentCheck.SetChecked(true)
		if !presenter.GetStateStore().StateExists("") {
			makeCurrentCheck.Disable()
		}
	}

	// Buttons
	var dialogWindow dialog.Dialog
	saveButton := widget.NewButton(locale.T("Save"), func() {
		id, err := validateID()
		if err != nil {
			dialog.ShowError(err, guiState.Window)
			return
		}

		comment := commentEntry.Text
		makeCurrent := makeCurrentCheck != nil && makeCurrentCheck.Checked
		if dialogWindow != nil {
			dialogWindow.Hide()
		}
		onResult(SaveStateResult{
			Action:      "save",
			ID:          id,
			Comment:     comment,
			MakeCurrent: makeCurrent,
		})
	})
	saveButton.Importance = widget.HighImportance

	// Fields container
	fieldsContainer := container.NewVBox(
		widget.NewLabel(locale.T("State ID:")),
		idEntry,
		widget.NewLabel(locale.T("Comment:")),
		container.NewScroll(commentEntry),
		warningLabel,
	)
	if makeCurrentCheck != nil {
		fieldsContainer.Add(makeCurrentCheck)
	}

	// Buttons container (без cancelButton - он будет через dismissText)
	buttonsContainer := container.NewHBox(
		layout.NewSpacer(),
		saveButton,
	)

	// Сохраняем оригинальный обработчик клавиатуры до создания диалога
	originalOnTypedKey := guiState.Window.Canvas().OnTypedKey()

	// Create dialog with simplified API (cancelButton через dismissText, ESC обрабатывается автоматически)
	dialogWindow = internaldialogs.NewCustom(locale.T("Save State"), fieldsContainer, buttonsContainer, locale.T("Cancel"), guiState.Window)
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
