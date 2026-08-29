// File source_chain_save_test.go — карта ссылок между цепочками (SPEC 110).
//
// Прежние тесты applyProxyEditToSource удалены: предмет (полевой маппинг
// scratch-ProxySource → Source на Save) упразднён SPEC 117 — окно источника
// правит deep-copy state.Source, и Save записывает её целиком (контракт
// буфера покрыт source_node_tag_buffer_test.go).
package tabs

import (
	"testing"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Кто ссылается на цепочку по имени — карта для предупреждения о разрыве
// ссылок при переименовании.
func TestChainReferencedBy(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		{Node: wizardmodels.Node{Kind: wizardmodels.SourceKindChain, Tag: "inner",
			Hops: []wizardmodels.NodeLink{{Tag: "a"}, {Tag: "b"}}}},
		{Node: wizardmodels.Node{Kind: wizardmodels.SourceKindChain, Tag: "outer",
			Hops: []wizardmodels.NodeLink{{Tag: "inner"}, {Tag: "c"}}}},
		{Node: wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Tag: "srv"}},
	}}

	got := chainReferencedBy(m)

	if users := got["inner"]; len(users) != 1 || users[0] != "outer" {
		t.Errorf("на inner ссылается %v, ожидали [outer]", users)
	}
	if _, ok := got["c"]; !ok {
		t.Error("обычная позиция тоже должна попасть в карту")
	}
}
