// Package dialogs содержит диалоговые окна визарда конфигурации.
//
// Файл get_free_dialog.go содержит функцию ShowGetFreeVPNDialog, которая создает диалоговое окно
// для загрузки и применения конфигурации из get_free.json:
//   - Скачивание get_free.json с GitHub при необходимости
//   - Загрузка parser_config и selectable_rules
//   - Применение данных к модели визарда (как при загрузке состояния)
//   - Отображение информационных полей (text, link)
//
// Диалог работает аналогично загрузке состояния из state.json, но использует get_free.json
// как источник данных по умолчанию для быстрой настройки.
//
// Используется в:
//   - tabs/source_tab.go - вызывается при нажатии кнопки "Get free VPN!"
//
// Взаимодействует с:
//   - presenter - применяет данные к модели через LoadState-подобную логику
//   - models.WizardStateFile - использует ту же структуру для загрузки состояния
package dialogs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/ui/components"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
)

// GetFreeData представляет структуру данных из get_free.json.
type GetFreeData struct {
	GetFree struct {
		Text string `json:"text"`
		Link string `json:"link"`
	} `json:"get_free"`
	ParserConfig    json.RawMessage `json:"parser_config"`
	SelectableRules []struct {
		Label            string `json:"label"`
		Enabled          bool   `json:"enabled"`
		SelectedOutbound string `json:"selected_outbound"`
	} `json:"selectable_rules,omitempty"`
	CustomRules  []wizardmodels.PersistedCustomRule `json:"custom_rules,omitempty"`
	ConfigParams []wizardmodels.ConfigParam         `json:"config_params,omitempty"`
}

// downloadGetFreeJSON скачивает get_free.json с GitHub.
func downloadGetFreeJSON(presenter *wizardpresentation.WizardPresenter, force bool) error {
	ac := presenter.Controller()
	if ac == nil {
		return fmt.Errorf("controller not available")
	}

	binDir := filepath.Join(ac.FileService.ExecDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	targetPath := filepath.Join(binDir, "get_free.json")

	// Check if file already exists (skip if not forced)
	if !force {
		if _, err := os.Stat(targetPath); err == nil {
			debuglog.DebugLog("get_free.json already exists, skipping download")
			return nil
		}
	}

	// Download from GitHub (ветка main или develop в зависимости от версии приложения)
	downloadURL := fmt.Sprintf("https://raw.githubusercontent.com/Leadaxe/singbox-launcher/%s/bin/get_free.json", constants.GetMyBranch())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := core.CreateHTTPClient(30 * time.Second)
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "singbox-launcher/1.0")

	resp, err := client.Do(req)
	if err != nil {
		// Check if it's a network error and provide more details
		if core.IsNetworkError(err) {
			return fmt.Errorf("network error: %s. Please check your internet connection", core.GetNetworkErrorMessage(err))
		}
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: server returned status %d. Please try again later", resp.StatusCode)
	}

	// Read response body with size limit
	const maxFileSize = 1024 * 1024 // 1 MB limit
	limitedReader := io.LimitReader(resp.Body, maxFileSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if len(data) > maxFileSize {
		return fmt.Errorf("file too large (exceeds %d bytes)", maxFileSize)
	}

	if len(data) == 0 {
		return fmt.Errorf("downloaded file is empty")
	}

	// Validate JSON before writing
	var testData map[string]interface{}
	if err := json.Unmarshal(data, &testData); err != nil {
		return fmt.Errorf("downloaded file is not valid JSON: %w", err)
	}

	// Write to file
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	debuglog.InfoLog("Successfully downloaded get_free.json")
	return nil
}

// loadGetFreeJSON загружает get_free.json из локального файла или скачивает при необходимости.
func loadGetFreeJSON(presenter *wizardpresentation.WizardPresenter) (*GetFreeData, error) {
	ac := presenter.Controller()
	if ac == nil {
		return nil, fmt.Errorf("controller not available")
	}

	filePath := filepath.Join(ac.FileService.ExecDir, "bin", "get_free.json")

	// Try to read local file first
	data, err := os.ReadFile(filePath)
	if err != nil {
		// If file doesn't exist, try to download it
		if os.IsNotExist(err) {
			debuglog.DebugLog("get_free.json not found locally, downloading...")
			if downloadErr := downloadGetFreeJSON(presenter, false); downloadErr != nil {
				return nil, fmt.Errorf("failed to download get_free.json: %w", downloadErr)
			}
			// Try reading again after download
			data, err = os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to read get_free.json after download: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to read get_free.json: %w", err)
		}
	}

	// Try to parse JSON
	var getFreeData GetFreeData
	if err := json.Unmarshal(data, &getFreeData); err != nil {
		// If parsing fails, file might be corrupted - try to download again
		debuglog.WarnLog("get_free.json appears to be corrupted, attempting to re-download: %v", err)
		if downloadErr := downloadGetFreeJSON(presenter, true); downloadErr != nil {
			return nil, fmt.Errorf("failed to parse get_free.json and re-download failed: %w (original parse error: %v)", downloadErr, err)
		}
		// Try reading again after re-download
		data, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read get_free.json after re-download: %w", err)
		}
		// Try parsing again
		if err := json.Unmarshal(data, &getFreeData); err != nil {
			return nil, fmt.Errorf("failed to parse get_free.json after re-download: %w", err)
		}
	}

	// Validate that required fields are present
	if len(getFreeData.ParserConfig) == 0 {
		return nil, fmt.Errorf("get_free.json is missing parser_config field")
	}

	return &getFreeData, nil
}

// convertGetFreeDataToStateFile преобразует GetFreeData в WizardStateFile для использования LoadState.
func convertGetFreeDataToStateFile(getFreeData *GetFreeData) (*wizardmodels.WizardStateFile, error) {
	// Преобразуем selectable_rules в PersistedSelectableRuleState
	selectableRuleStates := make([]wizardmodels.PersistedSelectableRuleState, 0, len(getFreeData.SelectableRules))
	for _, rule := range getFreeData.SelectableRules {
		selectableRuleStates = append(selectableRuleStates, wizardmodels.PersistedSelectableRuleState{
			Label:            rule.Label,
			Enabled:          rule.Enabled,
			SelectedOutbound: rule.SelectedOutbound,
		})
		debuglog.DebugLog("convertGetFreeDataToStateFile: converted rule label=%s, enabled=%v, selected_outbound=%s", rule.Label, rule.Enabled, rule.SelectedOutbound)
	}

	// Используем фабрику для создания WizardStateFile
	// Логика работы с ParserConfig и проверки на nil инкапсулированы внутри NewWizardStateFile
	return wizardmodels.NewWizardStateFile(
		getFreeData.ParserConfig,
		getFreeData.ConfigParams,
		selectableRuleStates,
		getFreeData.CustomRules,
	)
}

// ShowGetFreeVPNDialog открывает диалог для загрузки и применения конфигурации из get_free.json.
func ShowGetFreeVPNDialog(presenter *wizardpresentation.WizardPresenter) {
	guiState := presenter.GUIState()
	if guiState.Window == nil {
		return
	}

	// Show loading indicator
	loadingDialog := dialog.NewInformation("Loading", "Downloading get_free.json...", guiState.Window)
	loadingDialog.Show()

	// Download and load get_free.json in background
	go func() {
		defer func() {
			if r := recover(); r != nil {
				debuglog.ErrorLog("Panic in ShowGetFreeVPNDialog goroutine: %v", r)
				fyne.Do(func() {
					loadingDialog.Hide()
					dialog.ShowError(fmt.Errorf("Произошла ошибка: %v", r), guiState.Window)
				})
			}
		}()

		// Download get_free.json if needed
		if err := downloadGetFreeJSON(presenter, false); err != nil {
			debuglog.ErrorLog("Failed to download get_free.json: %v", err)
			fyne.Do(func() {
				loadingDialog.Hide()
				dialog.ShowError(fmt.Errorf("Не удалось скачать get_free.json:\n\n%w\n\nПроверьте подключение к интернету и попробуйте снова.", err), guiState.Window)
			})
			return
		}

		// Load data
		getFreeData, err := loadGetFreeJSON(presenter)
		if err != nil {
			debuglog.ErrorLog("Failed to load get_free.json: %v", err)
			fyne.Do(func() {
				loadingDialog.Hide()
				dialog.ShowError(fmt.Errorf("Не удалось загрузить get_free.json:\n\n%w\n\nПроверьте файл или попробуйте скачать заново.", err), guiState.Window)
			})
			return
		}

		// Validate loaded data
		if getFreeData == nil {
			debuglog.ErrorLog("getFreeData is nil after loading")
			fyne.Do(func() {
				loadingDialog.Hide()
				dialog.ShowError(fmt.Errorf("Ошибка: данные не загружены"), guiState.Window)
			})
			return
		}

		// Используем значения только из get_free.json (без дефолтов)
		text := getFreeData.GetFree.Text
		linkStr := getFreeData.GetFree.Link

		// Convert to WizardStateFile and load state
		stateFile, err := convertGetFreeDataToStateFile(getFreeData)
		if err != nil {
			debuglog.ErrorLog("Failed to convert get_free.json to state: %v", err)
			fyne.Do(func() {
				loadingDialog.Hide()
				dialog.ShowError(fmt.Errorf("Не удалось обработать get_free.json:\n\n%w", err), guiState.Window)
			})
			return
		}

		fyne.Do(func() {
			// Hide loading dialog BEFORE creating new dialog
			loadingDialog.Hide()

			// Create dialog content
			thanks := widget.NewLabel(text)
			thanks.Wrapping = fyne.TextWrapWord
			linkURL, _ := url.Parse(linkStr)
			link := widget.NewHyperlink(linkStr, linkURL)

			var freeVPNDialog dialog.Dialog
			applyButton := widget.NewButton("Apply configuration", func() {
				// Load state using the same logic as LoadState
				if err := presenter.LoadState(stateFile); err != nil {
					debuglog.ErrorLog("Failed to load state from get_free.json: %v", err)
					dialog.ShowError(fmt.Errorf("Не удалось применить конфигурацию:\n\n%w", err), guiState.Window)
					return
				}

				debuglog.InfoLog("Successfully applied configuration from get_free.json")
				dialog.ShowInformation("Success", "Configuration from get_free.json has been applied successfully!", guiState.Window)

				// Close the dialog
				if freeVPNDialog != nil {
					freeVPNDialog.Hide()
				}
			})

			spacer := canvas.NewRectangle(color.Transparent)
			spacer.SetMinSize(fyne.NewSize(0, applyButton.MinSize().Height))
			mainContent := container.NewVBox(
				thanks,
				link,
				spacer,
				applyButton,
			)

			freeVPNDialog = components.NewCustom("Get free VPN", mainContent, nil, "Close", guiState.Window)
			freeVPNDialog.SetOnClosed(func() {
				// Dialog closed
			})
			// Resize dialog to make it visible
			freeVPNDialog.Resize(fyne.NewSize(400, 200))
			freeVPNDialog.Show()
		})
	}()
}
