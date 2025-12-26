package wizard

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizarddialogs "singbox-launcher/ui/wizard/dialogs"
	wizardstate "singbox-launcher/ui/wizard/state"
	wizardtabs "singbox-launcher/ui/wizard/tabs"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// ShowConfigWizard opens the configuration wizard window.
func ShowConfigWizard(parent fyne.Window, controller *core.AppController) {
	state := &wizardstate.WizardState{
		Controller:        controller,
		PreviewNeedsParse: true,
	}

	if templateData, err := wizardtemplate.LoadTemplateData(controller.FileService.ExecDir); err != nil {
		templateFileName := wizardtemplate.GetTemplateFileName()
		wizardstate.ErrorLog("ConfigWizard: failed to load %s from %s: %v", templateFileName, filepath.Join(controller.FileService.ExecDir, "bin", templateFileName), err)
		// Update config status in Core Dashboard (similar to UpdateConfigStatusFunc)
		if controller.UIService != nil && controller.UIService.UpdateConfigStatusFunc != nil {
			controller.UIService.UpdateConfigStatusFunc()
		}
		return
	} else {
		state.TemplateData = templateData
	}

	// Create new window for wizard
	wizardWindow := controller.UIService.Application.NewWindow("Config Wizard")
	wizardWindow.Resize(fyne.NewSize(620, 660))
	wizardWindow.CenterOnScreen()
	state.Window = wizardWindow

	// Create first tab
	tab1 := wizardtabs.CreateSourceTab(state)

	loadedConfig, err := wizardbusiness.LoadConfigFromFile(state)
	if err != nil {
		wizardstate.ErrorLog("ConfigWizard: Failed to load config: %v", err)
		dialog.ShowError(fmt.Errorf("Failed to load existing config: %w", err), wizardWindow)
	}
	if !loadedConfig {
		// If we didn't load from template or config.json - show error
		if state.TemplateData == nil || state.TemplateData.ParserConfig == "" {
			templateFileName := wizardtemplate.GetTemplateFileName()
			dialog.ShowError(fmt.Errorf("No config found and template file (bin/%s) is missing or invalid.\nPlease create %s or ensure config.json exists.", templateFileName, templateFileName), wizardWindow)
			wizardWindow.Close()
			return
		}
	}

	// Initialize template state
	state.InitializeTemplateState()

	// Create container with tabs (only one for now)
	tab1Item := container.NewTabItem("Sources & ParserConfig", tab1)
	tabs := container.NewAppTabs(tab1Item)
	var rulesTabItem *container.TabItem
	var previewTabItem *container.TabItem
	var currentTabIndex int = 0
	// Use ShowAddRuleDialog from wizard/dialogs directly
	showAddRuleDialogWrapper := wizarddialogs.ShowAddRuleDialog
	if templateTab := wizardtabs.CreateRulesTab(state, showAddRuleDialogWrapper); templateTab != nil {
		rulesTabItem = container.NewTabItem("Rules", templateTab)
		previewTabItem = container.NewTabItem("Preview", wizardtabs.CreatePreviewTab(state))
		tabs.Append(rulesTabItem)
		tabs.Append(previewTabItem)
	}

	// Create navigation buttons
	state.CloseButton = widget.NewButton("Close", func() {
		wizardWindow.Close()
	})

	// Close window via X
	wizardWindow.SetCloseIntercept(func() {
		wizardWindow.Close()
	})
	state.CloseButton.Importance = widget.HighImportance

	state.PrevButton = widget.NewButton("Prev", func() {
		if currentTabIndex > 0 {
			currentTabIndex--
			tabs.SelectTab(tabs.Items[currentTabIndex])
		}
	})
	state.PrevButton.Importance = widget.HighImportance

	state.NextButton = widget.NewButton("Next", func() {
		if currentTabIndex < len(tabs.Items)-1 {
			currentTabIndex++
			tabs.SelectTab(tabs.Items[currentTabIndex])
		}
	})
	state.NextButton.Importance = widget.HighImportance

	state.SaveButton = widget.NewButton("Save", func() {
		if strings.TrimSpace(state.ParserConfigEntry.Text) == "" {
			dialog.ShowError(fmt.Errorf("ParserConfig is empty"), state.Window)
			return
		}
		if strings.TrimSpace(state.SourceURLEntry.Text) == "" {
			dialog.ShowError(fmt.Errorf("VLESS URL is empty"), state.Window)
			return
		}
		if state.SaveInProgress {
			dialog.ShowInformation("Saving", "Save operation already in progress... Please wait.", state.Window)
			return
		}
		if state.AutoParseInProgress {
			dialog.ShowInformation("Parsing", "Parsing in progress... Please wait.", state.Window)
			return
		}

		// Start async save with progress indication
		state.SetSaveState("", 0.0) // Show progress bar
		go func() {
			defer wizardstate.SafeFyneDo(state.Window, func() {
				state.SetSaveState("Save", -1) // Hide progress, show button
			})

			// Step 0: Check and wait for parsing if needed (0-40%)
			if state.PreviewNeedsParse || state.AutoParseInProgress {
				wizardstate.SafeFyneDo(state.Window, func() {
					state.SaveProgress.SetValue(0.05)
				})

				// If parsing hasn't started yet, start it
				if !state.AutoParseInProgress {
					state.AutoParseInProgress = true
					go wizardbusiness.ParseAndPreview(state)
				}

				// Wait for parsing to complete (check every 100ms)
				maxWaitTime := 60 * time.Second // Maximum wait time
				startTime := time.Now()
				iterations := 0
				for state.AutoParseInProgress {
					if time.Since(startTime) > maxWaitTime {
						wizardstate.SafeFyneDo(state.Window, func() {
							dialog.ShowError(fmt.Errorf("Parsing timeout: operation took too long"), state.Window)
						})
						return
					}
					time.Sleep(100 * time.Millisecond)
					iterations++
					// Update progress smoothly (0.05 - 0.40)
					// Show that process is running
					progressRange := 0.35
					baseProgress := 0.05
					// Smooth forward movement with cyclic effect
					cycleProgress := float64(iterations%40) / 40.0
					currentProgress := baseProgress + cycleProgress*progressRange
					wizardstate.SafeFyneDo(state.Window, func() {
						state.SaveProgress.SetValue(currentProgress)
					})
				}
				wizardstate.SafeFyneDo(state.Window, func() {
					state.SaveProgress.SetValue(0.4)
				})
			}

			// Step 1: Build config (40-80%)
			wizardstate.SafeFyneDo(state.Window, func() {
				state.SaveProgress.SetValue(0.4)
			})
			text, err := wizardbusiness.BuildTemplateConfig(state, false)
			if err != nil {
				wizardstate.SafeFyneDo(state.Window, func() {
					dialog.ShowError(err, state.Window)
				})
				return
			}
			wizardstate.SafeFyneDo(state.Window, func() {
				state.SaveProgress.SetValue(0.8)
			})

			// Step 2: Save file (80-95%)
			path, err := state.SaveConfigWithBackup(text)
			if err != nil {
				wizardstate.SafeFyneDo(state.Window, func() {
					dialog.ShowError(err, state.Window)
				})
				return
			}
			wizardstate.SafeFyneDo(state.Window, func() {
				state.SaveProgress.SetValue(0.95)
			})

			// Step 3: Completion (95-100%)
			time.Sleep(100 * time.Millisecond)
			wizardstate.SafeFyneDo(state.Window, func() {
				state.SaveProgress.SetValue(1.0)
			})
			// Small delay so user sees progress
			time.Sleep(200 * time.Millisecond)

			// Successfully saved
			wizardstate.SafeFyneDo(state.Window, func() {
				dialog.ShowInformation("Config Saved", fmt.Sprintf("Config written to %s", path), state.Window)
				state.Window.Close()
			})
		}()
	})
	state.SaveButton.Importance = widget.HighImportance

	// Create ProgressBar for Save button
	state.SaveProgress = widget.NewProgressBar()
	state.SaveProgress.Hide()
	state.SaveProgress.SetValue(0)

	// Set fixed size via placeholder (same as button)
	saveButtonWidth := state.SaveButton.MinSize().Width
	saveButtonHeight := state.SaveButton.MinSize().Height

	// Create placeholder to preserve size
	state.SavePlaceholder = canvas.NewRectangle(color.Transparent)
	state.SavePlaceholder.SetMinSize(fyne.NewSize(saveButtonWidth, saveButtonHeight))
	state.SavePlaceholder.Show()

	// Save tabs reference in state
	state.Tabs = tabs

	// Create container with stack for Save button (placeholder, button, progress)
	saveButtonStack := container.NewStack(
		state.SavePlaceholder,
		state.SaveButton,
		state.SaveProgress,
	)

	// Function to update buttons based on tab
	updateNavigationButtons := func() {
		totalTabs := len(tabs.Items)

		var buttonsContent fyne.CanvasObject
		if currentTabIndex == totalTabs-1 {
			// Last tab (Preview): Close on left, Prev and Save on right
			buttonsContent = container.NewHBox(
				state.CloseButton,
				layout.NewSpacer(),
				state.PrevButton,
				saveButtonStack, // Use stack with ProgressBar
			)
		} else if currentTabIndex == 0 {
			// First tab: Close on left, Next on right (Prev hidden)
			buttonsContent = container.NewHBox(
				state.CloseButton,
				layout.NewSpacer(),
				state.NextButton,
			)
		} else {
			// Middle tabs: Close on left, Prev and Next on right
			buttonsContent = container.NewHBox(
				state.CloseButton,
				layout.NewSpacer(),
				state.PrevButton,
				state.NextButton,
			)
		}
		state.ButtonsContainer = buttonsContent
	}

	// Initialize button container
	updateNavigationButtons()

	// Update buttons when switching tabs
	tabs.OnChanged = func(item *container.TabItem) {
		// Update current tab index
		for i, tabItem := range tabs.Items {
			if tabItem == item {
				currentTabIndex = i
				break
			}
		}
		if item == previewTabItem {
			// Trigger async parsing (if needed)
			go func() {
				wizardbusiness.TriggerParseForPreview(state)
			}()
			// Check if preview needs recalculation due to changes on Rules tab
			if state.TemplatePreviewNeedsUpdate {
				go func() {
					wizardbusiness.UpdateTemplatePreviewAsync(state)
				}()
			}
		}
		updateNavigationButtons()
		// Update Border container with new buttons
		content := container.NewBorder(
			nil,                    // top
			state.ButtonsContainer, // bottom
			nil,                    // left
			nil,                    // right
			tabs,                   // center
		)
		wizardWindow.SetContent(content)
	}

	// Preview is generated only via "Show Preview" button

	content := container.NewBorder(
		nil,                    // top
		state.ButtonsContainer, // bottom
		nil,                    // left
		nil,                    // right
		tabs,                   // center
	)

	wizardWindow.SetContent(content)
	wizardWindow.Show()
}
