// Package wizardsync содержит чистую логику синхронизации визарда GUI→модель без импорта Fyne,
// чтобы юнит-тесты не тянули GL/CGO. Риск: затереть модель пустыми виджетами до завершения первого
// applyWizardWidgetsFromModel или до того, как Select отразит загруженный state.
package wizardsync

import (
	"slices"
	"strings"
)

// GuiTextAwaitingProgrammaticFill: пустой Entry/strategy до WizardWidgetsReady при непустой модели —
// это «ещё не SetText/SetSelected», а не очистка пользователем.
func GuiTextAwaitingProgrammaticFill(ready bool, widgetText, modelText string) bool {
	return !ready && strings.TrimSpace(widgetText) == "" && strings.TrimSpace(modelText) != ""
}

// FinalOutboundSelectReadLooksStale: пустой выбор Final outbound не должен затирать модель,
// пока виджет не отражает state. До ready — любой пустой Selected при непустой модели.
// После ready — только если Options пусты или тег модели уже в списке (иначе пустой выбор — валидное состояние UI).
func FinalOutboundSelectReadLooksStale(ready bool, widgetSelected, modelOutbound string, opts []string) bool {
	ws := strings.TrimSpace(widgetSelected)
	mo := strings.TrimSpace(modelOutbound)
	if ws != "" || mo == "" {
		return false
	}
	if !ready {
		return true
	}
	return len(opts) == 0 || slices.Contains(opts, mo)
}
