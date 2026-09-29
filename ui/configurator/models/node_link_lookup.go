// File node_link_lookup.go — поиск узла источников по ссылке NodeLink.
package models

import (
	corestate "singbox-launcher/core/state"
)

// FindNodeByLink — узел источников по ссылке {FolderID, Tag}.
//
// Корневой узел адресуется тем же именем, что его знает конфиг
// (NodeTagOrLabel) — так же, как его собирает пересев.
func FindNodeByLink(m *WizardModel, link corestate.NodeLink) *corestate.Node {
	if m == nil || link.Tag == "" {
		return nil
	}
	for i := range m.Sources {
		src := &m.Sources[i]
		if link.FolderID == "" {
			if src.Kind == corestate.SourceKindServer && src.NodeTagOrLabel() == link.Tag {
				return &src.Node
			}
			continue
		}
		if src.ID != link.FolderID {
			continue
		}
		for j := range src.Nodes {
			if src.Nodes[j].Tag == link.Tag {
				return &src.Nodes[j]
			}
		}
	}
	return nil
}
