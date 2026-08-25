// File source_chain_save_test.go — позиции цепочки доезжают от формы до
// состояния (SPEC 110).
//
// applyProxyEditToSource разбирает scratch по типу источника, и у цепочки
// нет ни Source, ни Connections, ни ConfigJSON — то есть без своей ветки
// она не попадала НИ В ОДНУ, и Save молча терял все позиции. Пользователь
// добавлял хопы, жал Save и получал пустую цепочку.
package tabs

import (
	"testing"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

func TestApplyProxyEdit_ChainHopsSurviveSave(t *testing.T) {
	scratch := &config.ProxySource{
		TagMask: "chain-1",
		Chain: &configtypes.SourceChain{
			Hops:        []string{"vpn-1", "🇳🇱 Amsterdam"},
			IdleTimeout: "10m",
		},
	}
	src := &wizardmodels.Source{ID: "01ABC", Type: wizardmodels.SourceTypeChain}

	applyProxyEditToSource(scratch, src)

	if src.Chain == nil {
		t.Fatal("Chain не перенесён — позиции потеряны на сохранении")
	}
	if len(src.Chain.Hops) != 2 || src.Chain.Hops[0] != "vpn-1" {
		t.Errorf("позиции = %v, ожидали [vpn-1 🇳🇱 Amsterdam]", src.Chain.Hops)
	}
	if src.Chain.IdleTimeout != "10m" {
		t.Errorf("idle_timeout = %q", src.Chain.IdleTimeout)
	}
	if src.Type != wizardmodels.SourceTypeChain {
		t.Errorf("тип = %q, ожидали chain", src.Type)
	}
	// TagMask несёт ТЕГ узла цепочки — он и едет в NodeTag; подпись
	// (Label) при этом не трогается.
	if src.NodeTag != "chain-1" {
		t.Errorf("node_tag = %q, ожидали chain-1", src.NodeTag)
	}
	// Поля чужих типов не должны появиться: цепочка не подписка и не сервер.
	if src.URL != "" || src.URI != "" {
		t.Errorf("URL=%q URI=%q — у цепочки их не бывает", src.URL, src.URI)
	}
}

// Подписка и сервер не должны пострадать: ветка цепочки стоит первой и
// обязана срабатывать только на своих.
func TestApplyProxyEdit_OtherTypesUnaffected(t *testing.T) {
	sub := &wizardmodels.Source{}
	applyProxyEditToSource(&config.ProxySource{Source: "https://example.com/sub"}, sub)
	if sub.Type != wizardmodels.SourceTypeSubscription || sub.URL != "https://example.com/sub" {
		t.Errorf("подписка сломана: type=%q url=%q", sub.Type, sub.URL)
	}

	srv := &wizardmodels.Source{}
	applyProxyEditToSource(&config.ProxySource{Connections: []string{"vless://x@h:443"}}, srv)
	if srv.Type != wizardmodels.SourceTypeServer || srv.URI != "vless://x@h:443" {
		t.Errorf("сервер сломан: type=%q uri=%q", srv.Type, srv.URI)
	}
}

// Тег цепочки правится в форме. В ProxySource он живёт в TagMask, а в
// состоянии — в NodeTag; без переноса правка терялась бы так же молча,
// как раньше терялись позиции.
func TestApplyProxyEdit_ChainRenameSurvives(t *testing.T) {
	scratch := &config.ProxySource{
		TagMask: "через-Германию",
		Chain:   &configtypes.SourceChain{Hops: []string{"a", "b"}},
	}
	src := &wizardmodels.Source{
		ID: "01ABC", Type: wizardmodels.SourceTypeChain, NodeTag: "chain-1",
	}

	applyProxyEditToSource(scratch, src)

	if src.NodeTag != "через-Германию" {
		t.Errorf("тег = %q, ожидали «через-Германию»", src.NodeTag)
	}
}

// Подпись — не тег: правка тега в форме не должна её трогать, иначе
// разделение ролей существует только на бумаге.
func TestApplyProxyEdit_ChainRetagKeepsLabel(t *testing.T) {
	src := &wizardmodels.Source{
		Type: wizardmodels.SourceTypeChain, NodeTag: "chain-1", Label: "Моя Германия",
	}

	applyProxyEditToSource(&config.ProxySource{
		TagMask: "chain-2",
		Chain:   &configtypes.SourceChain{Hops: []string{"a", "b"}},
	}, src)

	if src.NodeTag != "chain-2" {
		t.Errorf("тег = %q, ожидали chain-2", src.NodeTag)
	}
	if src.Label != "Моя Германия" {
		t.Errorf("подпись = %q — правка тега её затёрла", src.Label)
	}
}

// Кто ссылается на цепочку по имени — карта для предупреждения о разрыве
// ссылок при переименовании.
func TestChainReferencedBy(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		{Type: wizardmodels.SourceTypeChain, NodeTag: "inner",
			Chain: &configtypes.SourceChain{Hops: []string{"a", "b"}}},
		{Type: wizardmodels.SourceTypeChain, NodeTag: "outer",
			Chain: &configtypes.SourceChain{Hops: []string{"inner", "c"}}},
		{Type: wizardmodels.SourceTypeServer, NodeTag: "srv"},
	}}

	got := chainReferencedBy(m)

	if users := got["inner"]; len(users) != 1 || users[0] != "outer" {
		t.Errorf("на inner ссылается %v, ожидали [outer]", users)
	}
	if _, ok := got["c"]; !ok {
		t.Error("обычная позиция тоже должна попасть в карту")
	}
}

// TestApplyProxyEdit_EmptyChainKeepsRename — цепочка с нулём позиций: scratch
// несёт Chain == nil (Collect пустой формы), но источник ТИПА chain обязан
// принять правку — иначе Save no-op и переименование только что созданной
// цепочки молча откатывается (сценарий «создал, вписал имя, Save»).
func TestApplyProxyEdit_EmptyChainKeepsRename(t *testing.T) {
	src := &wizardmodels.Source{
		Type:    wizardmodels.SourceTypeChain,
		NodeTag: "chain-1",
	}
	ps := &config.ProxySource{
		TagMask: "my-route",
		Chain:   nil, // пустая форма
	}
	applyProxyEditToSource(ps, src)
	if src.NodeTag != "my-route" {
		t.Fatalf("переименование пустой цепочки потеряно: %q", src.NodeTag)
	}
	if src.Type != wizardmodels.SourceTypeChain || src.Chain == nil {
		t.Fatalf("тип источника разъехался: %+v", src)
	}
}
