package tabs

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 113-E + SPEC 117: правки окна источника БУФЕРИЗУЮТСЯ до Save.
//
// Рабочий буфер окна — deep-copy state.Source (cloneSource). Тег — единственная
// идентичность узла (SPEC 112), и его смена обязана идти в паре со сбросом
// ссылок; сброс делается только на Save. Cancel закрывает окно без следов:
// ни одно поле копии не должно разделять память с записью модели (риск Р4).

func serverSourceWithTag(tag string) wizardmodels.Source {
	return wizardmodels.Source{
		ID:      "01SRV0000000000000000000",
		Node:    wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Enabled: true},
		Label:   "WARP hop",
		NodeTag: tag,
		URI:     "vless://uuid@host:443",
	}
}

// Cancel: буфер правили, Save не жали — модель обязана остаться прежней.
func TestNodeTagEditIsBufferedUntilSave(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{serverSourceWithTag("hop")}}
	clone := cloneSource(&m.Sources[0])

	// Ровно то, что делает nodeTagEntry.OnChanged: пишет в буфер, и только.
	clone.NodeTag = "hop-renamed"

	if got := m.Sources[0].NodeTagOrLabel(); got != "hop" {
		t.Fatalf("тег в модели = %q — правка утекла мимо Save", got)
	}
}

// Save: буфер доезжает до модели одной записью (m.Sources[i] = копия).
func TestNodeTagEditReachesModelOnSave(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{serverSourceWithTag("hop")}}
	clone := cloneSource(&m.Sources[0])
	clone.NodeTag = "hop-renamed"

	m.Sources[0] = clone

	if got := m.Sources[0].NodeTagOrLabel(); got != "hop-renamed" {
		t.Fatalf("тег в модели = %q, ожидался hop-renamed", got)
	}
}

// Очистка поля — тоже правка: пустой NodeTag в копии доезжает до модели тем
// же присваиванием (отдельный applyClearedNodeTag больше не нужен), а
// NodeTagOrLabel откатывается на подпись — прежнее поведение.
func TestClearedNodeTagReachesModelOnSave(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{serverSourceWithTag("hop")}}
	clone := cloneSource(&m.Sources[0])
	clone.NodeTag = ""

	m.Sources[0] = clone

	if m.Sources[0].NodeTag != "" {
		t.Fatalf("тег = %q, очистка поля не доехала", m.Sources[0].NodeTag)
	}
	if got := m.Sources[0].NodeTagOrLabel(); got != "WARP hop" {
		t.Errorf("NodeTagOrLabel = %q, ожидался откат на подпись", got)
	}
}

// Р4 «Cancel без следов»: копия не должна разделять ссылочные поля с моделью —
// иначе правка формы утечёт в модель до Save и переживёт Cancel.
func TestCloneSourceIsDeeplyIndependent(t *testing.T) {
	orig := wizardmodels.Source{
		ID:        "01SUB0000000000000000000",
		Node:      wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
		Label:     "Proton NL",
		URL:       "https://example.com/sub",
		Skip:      []map[string]string{{"scheme": "ss"}},
		TagPolicy: &wizardmodels.TagSpec{Prefix: "NL-"},
		Outbounds: []configtypes.Direction{
			{Tag: "AL:select", AddOutbounds: []string{"a", "b"}},
		},
		Fold:          &configtypes.SourceFold{Mode: "select", Auto: &configtypes.DirectionAuto{Interval: "5m"}},
		Meta:          &wizardmodels.SubscriptionMeta{ProfileTitle: "Proton", PreviewNodes: []string{"n1"}},
		DisabledNodes: map[string]int64{"🇩🇪 DE": 42},
	}
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{orig}}
	clone := cloneSource(&m.Sources[0])

	// Мутации буфера — всё, что делает форма между открытием и Cancel.
	setNodeEnabled(&clone, "🇳🇱 NL", false)
	delete(clone.DisabledNodes, "🇩🇪 DE")
	clone.TagPolicy.Prefix = "XX-"
	clone.Fold.Mode = "auto"
	clone.Fold.Auto.Interval = "1m"
	clone.Outbounds[0].AddOutbounds[0] = "hacked"
	clone.Skip[0]["scheme"] = "vless"
	clone.Meta.ProfileTitle = "hacked"
	clone.Meta.PreviewNodes[0] = "hacked"

	src := &m.Sources[0]
	if _, off := src.DisabledNodes["🇳🇱 NL"]; off {
		t.Error("отметка узла утекла в модель до Save")
	}
	if _, kept := src.DisabledNodes["🇩🇪 DE"]; !kept {
		t.Error("снятие отметки утекло в модель до Save")
	}
	if src.TagPolicy.Prefix != "NL-" {
		t.Errorf("Tag.Prefix утёк: %q", src.TagPolicy.Prefix)
	}
	if src.Fold.Mode != "select" || src.Fold.Auto.Interval != "5m" {
		t.Errorf("Fold утёк: %+v", src.Fold)
	}
	if src.Outbounds[0].AddOutbounds[0] != "a" {
		t.Errorf("Outbounds утекли: %v", src.Outbounds[0].AddOutbounds)
	}
	if src.Skip[0]["scheme"] != "ss" {
		t.Errorf("Skip утёк: %v", src.Skip)
	}
	if src.Meta.ProfileTitle != "Proton" || src.Meta.PreviewNodes[0] != "n1" {
		t.Errorf("Meta утекла: %+v", src.Meta)
	}
}

// Цепочка: правка позиций в буфере не трогает модель (форма цепочки живёт на
// копии Chain — Load/Collect не должны дотягиваться до записи модели).
func TestCloneSourceChainIndependent(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{{
		ID:      "01CHN0000000000000000000",
		Node:    wizardmodels.Node{Kind: wizardmodels.SourceKindChain, Enabled: true},
		NodeTag: "chain-1",
		Chain:   &configtypes.SourceChain{Hops: []string{"a", "b"}, Strip: map[string]bool{"x": true}},
	}}}
	clone := cloneSource(&m.Sources[0])

	clone.Chain.Hops[0] = "hacked"
	clone.Chain.Strip["x"] = false
	clone.Chain = &configtypes.SourceChain{Hops: []string{"c"}}

	src := &m.Sources[0]
	if src.Chain.Hops[0] != "a" || len(src.Chain.Hops) != 2 {
		t.Errorf("позиции утекли: %v", src.Chain.Hops)
	}
	if !src.Chain.Strip["x"] {
		t.Errorf("Strip утёк: %v", src.Chain.Strip)
	}
}
