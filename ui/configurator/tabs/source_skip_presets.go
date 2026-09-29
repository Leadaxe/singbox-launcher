package tabs

import (
	"encoding/json"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	wizardmodels "singbox-launcher/ui/configurator/models"

	"singbox-launcher/internal/locale"
)

// skipPresetsText / skipPresetsHint — переключатель поля записи skip_presets
// (LxBox §578, контракт 1.1.86).
const (
	skipPresetsText = "Skip presets"
	skipPresetsHint = "Presets will not add routing or DNS rules for this node."
)

// skipPresetsBlock — переключатель «Skip presets» формы узла-сервера.
//
// Виден у своего сервера и члена папки (у узла подписки записи нет), когда в
// шаблоне есть пресет с for_each, чей node_type равен типу тела узла. nil —
// показывать нечего. Переключатель правит рабочую копию узла; в модель она
// уходит по Save окна целиком.
func skipPresetsBlock(m *wizardmodels.WizardModel, link wizardmodels.NodeLink, node *wizardmodels.Node) fyne.CanvasObject {
	if m == nil || m.TemplateData == nil || node == nil || node.Kind != wizardmodels.SourceKindServer {
		return nil
	}
	if fid := strings.TrimSpace(link.FolderID); fid != "" {
		for i := range m.Sources {
			if m.Sources[i].ID == fid && m.Sources[i].Kind == wizardmodels.SourceKindSubscription {
				return nil
			}
		}
	}
	var body struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(node.Body, &body); err != nil || body.Type == "" {
		return nil
	}
	served := false
	for i := range m.TemplateData.Presets {
		if fe := m.TemplateData.Presets[i].ForEach; fe != nil && fe.NodeType == body.Type {
			served = true
			break
		}
	}
	if !served {
		return nil
	}
	check := widget.NewCheck(locale.T(skipPresetsText), func(on bool) {
		node.SkipPresets = on
	})
	check.SetChecked(node.SkipPresets)
	hint := widget.NewLabel(locale.T(skipPresetsHint))
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance
	return container.NewVBox(check, hint)
}
