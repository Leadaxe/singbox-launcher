//go:build !darwin

package tabs

import (
	"fyne.io/fyne/v2/widget"

	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
	wizardtemplate "singbox-launcher/core/template"
)

// maybeTunOffDarwin — только macOS; на других ОС ничего не делает.
func maybeTunOffDarwin(_ *wizardpresentation.WizardPresenter, _ *wizardmodels.WizardModel, _ *wizardtemplate.TemplateData, _ string, _ *widget.Check) bool {
	return false
}
