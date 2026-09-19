// File draft_disabler.go — адаптер выключателя страховки поверх ЧЕРНОВИКА
// Конфигуратора (SPEC 132 волна 6, хвост 1.1).
//
// Save кладёт в state поверхностную копию слайса источников
// (presenter_state.go). Выключение в сохранённом состоянии затрётся;
// поэтому цикл на Final и remote-Save пишет в model.Sources через
// FindNodeByLink. Commit — no-op: черновик закрепит Save.
package presentation

import (
	"strings"

	"singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

type draftDisabler struct {
	model *wizardmodels.WizardModel
}

func newDraftDisabler(m *wizardmodels.WizardModel) *draftDisabler {
	return &draftDisabler{model: m}
}

func (d *draftDisabler) Disable(link state.NodeLink, reason string) bool {
	if d == nil || d.model == nil {
		return false
	}
	node := wizardmodels.FindNodeByLink(d.model, link)
	if node == nil {
		return false
	}
	return node.SetCoreRejected(reason)
}

func (d *draftDisabler) Commit() error { return nil }

func (d *draftDisabler) Label(link state.NodeLink) string {
	if d == nil || d.model == nil {
		return ""
	}
	for i := range d.model.Sources {
		src := &d.model.Sources[i]
		if link.FolderID == "" {
			if src.Kind == state.SourceKindServer && src.NodeTagOrLabel() == link.Tag {
				return draftSourceLabel(src)
			}
			continue
		}
		if src.ID == link.FolderID {
			return draftSourceLabel(src)
		}
	}
	return ""
}

func draftSourceLabel(src *state.Source) string {
	if src == nil {
		return ""
	}
	if s := strings.TrimSpace(src.Label); s != "" {
		return s
	}
	return strings.TrimSpace(src.Tag)
}
