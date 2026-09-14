package business

import (
	"strings"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SourceDisplayName — как источник зовут пользователю: подпись, за ней тег
// узла, за ним URL/URI. Тот же порядок, что у диагностики сборки
// (config.sourceDisplayName), чтобы имя в окне и имя в логе совпадали.
func SourceDisplayName(s wizardmodels.Source) string {
	// SPEC 116 W4: у КОНТЕЙНЕРА имя канонически живёт в Name (Label —
	// отображаемое имя узловых kind'ов, sources_v7.go:181). Порядок здесь
	// обязан совпадать с corestate.displayName(), иначе папка со старевшим
	// Label звалась бы в диалогах одним именем, а в списке — другим.
	if s.Kind == wizardmodels.SourceKindFolder || s.Kind == wizardmodels.SourceKindSubscription {
		if v := strings.TrimSpace(s.Name); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(s.Label); v != "" {
		return v
	}
	if v := strings.TrimSpace(s.Name); v != "" {
		return v
	}
	if v := strings.TrimSpace(s.Tag); v != "" {
		return v
	}
	if v := strings.TrimSpace(s.URL); v != "" {
		return v
	}
	if s.Origin != nil {
		if v := strings.TrimSpace(s.Origin.Raw); v != "" {
			return v
		}
	}
	return s.ID
}
