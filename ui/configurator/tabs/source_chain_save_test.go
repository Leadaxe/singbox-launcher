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
	// Имя цепочки становится тегом её узла — берётся из TagMask, как у
	// server-источника.
	if src.Label != "chain-1" {
		t.Errorf("label = %q, ожидали chain-1", src.Label)
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
