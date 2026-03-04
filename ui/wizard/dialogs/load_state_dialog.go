// Package dialogs содержит диалоговые окна визарда конфигурации.
//
// Файл load_state_dialog.go содержит функцию ShowLoadStateDialog, которая создает диалоговое окно
// для выбора сохранённого состояния визарда:
//   - Простой список имён файлов (высота ~5 строк с прокруткой)
//   - Выделение state.json как "текущее"
//   - Кнопки: "Load", "New", "Cancel"
//
// Диалог используется в двух сценариях:
//   - При открытии визарда, если существует state.json или есть другие сохранённые состояния
//   - При нажатии кнопки "Read" для загрузки другого состояния
//
// Используется в:
//   - wizard.go - при открытии визарда для выбора состояния
//   - presentation/presenter_state.go - при нажатии кнопки "Read"
package dialogs

import (
	"fmt"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/debuglog"
	internaldialogs "singbox-launcher/internal/dialogs"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
)

// LoadStateResult представляет результат выбора в диалоге загрузки состояния.
type LoadStateResult struct {
	Action     string // "load", "new", "cancel"
	SelectedID string // ID выбранного состояния (пусто для state.json)
}

// ShowLoadStateDialog показывает диалог выбора состояния.
// Возвращает результат через callback.
func ShowLoadStateDialog(presenter *wizardpresentation.WizardPresenter, onResult func(LoadStateResult)) {
	guiState := presenter.GUIState()
	if guiState.Window == nil {
		onResult(LoadStateResult{Action: "cancel"})
		return
	}

	// Загружаем только имена файлов (без чтения содержимого)
	stateStore := presenter.GetStateStore()
	states, err := stateStore.ListWizardStateNames()
	if err != nil {
		debuglog.ErrorLog("ShowLoadStateDialog: failed to list states: %v", err)
		dialog.ShowError(fmt.Errorf("Failed to load states list: %w", err), guiState.Window)
		onResult(LoadStateResult{Action: "cancel"})
		return
	}

	// Если состояний нет, вызываем callback с "new"
	if len(states) == 0 {
		onResult(LoadStateResult{Action: "new"})
		return
	}

	// Сортируем состояния: сначала state.json, затем по дате изменения (новые сверху)
	sort.Slice(states, func(i, j int) bool {
		if states[i].IsCurrent && !states[j].IsCurrent {
			return true
		}
		if !states[i].IsCurrent && states[j].IsCurrent {
			return false
		}
		return states[i].UpdatedAt.After(states[j].UpdatedAt)
	})

	// Создаём простой список имён файлов
	var selectedIndex widget.ListItemID = -1
	if len(states) > 0 && states[0].IsCurrent {
		selectedIndex = 0
	}

	// Подготавливаем список имён файлов
	fileNames := make([]string, len(states))
	for i, state := range states {
		if state.ID == "" {
			fileNames[i] = "state.json"
		} else {
			fileNames[i] = state.ID + ".json"
		}
		if state.IsCurrent {
			fileNames[i] += " (Current)"
		}
	}

	list := widget.NewList(
		func() int {
			return len(fileNames)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= 0 && id < widget.ListItemID(len(fileNames)) {
				obj.(*widget.Label).SetText(fileNames[id])
			}
		},
	)

	// Выделяем state.json по умолчанию
	if selectedIndex >= 0 && selectedIndex < widget.ListItemID(len(states)) {
		list.Select(selectedIndex)
	}

	// Declare dialog variable first so it can be used in button callbacks
	var dialogWindow dialog.Dialog

	// Функция для обновления списка состояний
	refreshStatesList := func() {
		stateStore := presenter.GetStateStore()
		newStates, err := stateStore.ListWizardStateNames()
		if err != nil {
			debuglog.ErrorLog("ShowLoadStateDialog: failed to refresh states list: %v", err)
			return
		}

		// Если состояний не осталось, закрываем диалог с действием "new"
		if len(newStates) == 0 {
			if dialogWindow != nil {
				dialogWindow.Hide()
			}
			onResult(LoadStateResult{Action: "new"})
			return
		}

		// Обновляем список
		states = newStates
		sort.Slice(states, func(i, j int) bool {
			if states[i].IsCurrent && !states[j].IsCurrent {
				return true
			}
			if !states[i].IsCurrent && states[j].IsCurrent {
				return false
			}
			return states[i].UpdatedAt.After(states[j].UpdatedAt)
		})

		// Обновляем имена файлов
		fileNames = make([]string, len(states))
		for i, state := range states {
			if state.ID == "" {
				fileNames[i] = "state.json"
			} else {
				fileNames[i] = state.ID + ".json"
			}
			if state.IsCurrent {
				fileNames[i] += " (Current)"
			}
		}

		// Сбрасываем выбор
		selectedIndex = -1
		if len(states) > 0 && states[0].IsCurrent {
			selectedIndex = 0
		}

		// Обновляем список
		list.Refresh()
		if selectedIndex >= 0 && selectedIndex < widget.ListItemID(len(states)) {
			list.Select(selectedIndex)
		}
	}

	// Отслеживаем выбранный элемент
	list.OnSelected = func(id widget.ListItemID) {
		selectedIndex = id
	}

	// Buttons
	loadButton := widget.NewButton("Load", func() {
		selectedID := ""
		if selectedIndex >= 0 && selectedIndex < widget.ListItemID(len(states)) {
			selectedID = states[selectedIndex].ID
		}
		if dialogWindow != nil {
			dialogWindow.Hide()
		}
		onResult(LoadStateResult{
			Action:     "load",
			SelectedID: selectedID,
		})
	})
	loadButton.Importance = widget.HighImportance

	newButton := widget.NewButton("New", func() {
		if dialogWindow != nil {
			dialogWindow.Hide()
		}
		onResult(LoadStateResult{Action: "new"})
	})

	// Кнопка удаления
	deleteButton := widget.NewButton("Delete", func() {
		if selectedIndex < 0 || selectedIndex >= widget.ListItemID(len(states)) {
			return
		}

		selectedState := states[selectedIndex]

		// Нельзя удалять state.json (текущее состояние)
		if selectedState.IsCurrent {
			dialog.ShowError(fmt.Errorf("Cannot delete current state (state.json)"), guiState.Window)
			return
		}

		// Подтверждение удаления
		dialog.ShowConfirm("Delete State", fmt.Sprintf("Delete state '%s'?", selectedState.ID+".json"), func(confirmed bool) {
			if !confirmed {
				return
			}

			// Удаляем состояние
			stateStore := presenter.GetStateStore()
			if err := stateStore.DeleteWizardState(selectedState.ID); err != nil {
				dialog.ShowError(fmt.Errorf("Failed to delete state: %w", err), guiState.Window)
				return
			}

			// Обновляем список состояний
			refreshStatesList()
		}, guiState.Window)
	})
	deleteButton.Importance = widget.MediumImportance

	// Контейнер с кнопками (без cancelButton - он будет через dismissText)
	buttonsContainer := container.NewHBox(
		layout.NewSpacer(),
		deleteButton,
		newButton,
		loadButton,
	)

	// Создаём прокручиваемый список с фиксированной высотой (примерно 5 строк)
	scrollList := container.NewScroll(list)
	scrollList.SetMinSize(fyne.NewSize(300, 150)) // Примерно 5 строк по 30px

	// Сохраняем оригинальный обработчик клавиатуры до создания диалога
	originalOnTypedKey := guiState.Window.Canvas().OnTypedKey()

	// Create dialog with simplified API (cancelButton через dismissText, ESC обрабатывается автоматически)
	dialogWindow = internaldialogs.NewCustom("Load State", scrollList, buttonsContainer, "Cancel", guiState.Window)
	dialogWindow.Resize(fyne.NewSize(300, 220))

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
		onResult(LoadStateResult{Action: "cancel"})
	})

	dialogWindow.Show()
}
