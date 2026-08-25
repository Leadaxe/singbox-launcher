// File preview_cache_chain_test.go — цепочка видна в превью Направления
// (issue #91).
//
// Сборка конфига кладёт источники-цепочки в общий пул узлов
// (config.ResolveChainSources из GenerateOutboundsFromParserConfig), а кэш
// превью раньше этого не делал: он загружал только подписки. Итог —
// regex-фильтр Направления ловил цепочку в config.json, но не показывал её
// ни в превью, ни во flag picker'е, и пользователь читал это как «фильтр
// цепочки не берёт».
package business

import (
	"testing"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

func TestRebuildPreviewCache_ChainBecomesNode(t *testing.T) {
	prev := config.ChainSupportProbe
	config.ChainSupportProbe = func() (bool, string) { return true, "" }
	defer func() { config.ChainSupportProbe = prev }()

	m := &wizardmodels.WizardModel{
		Sources: []corestate.Source{
			{
				ID: "01SRV", Type: corestate.SourceTypeServer, Enabled: true,
				NodeTag: "🇳🇱 Amsterdam",
				URI:     "vless://11111111-1111-1111-1111-111111111111@10.0.0.1:443?security=none#ams",
			},
			{
				ID: "01CHN", Type: corestate.SourceTypeChain, Enabled: true,
				NodeTag: "NL-chain",
				Chain:   &configtypes.SourceChain{Hops: []string{"🇳🇱 Amsterdam", "direct-out"}},
			},
		},
	}
	m.RefreshDerivedParserConfig()

	if _, err := RebuildPreviewCache(m); err != nil {
		t.Fatalf("RebuildPreviewCache: %v", err)
	}

	var chain *config.ParsedNode
	for _, n := range m.PreviewNodes {
		if n != nil && n.Tag == "NL-chain" {
			chain = n
		}
	}
	if chain == nil {
		var tags []string
		for _, n := range m.PreviewNodes {
			if n != nil {
				tags = append(tags, n.Tag)
			}
		}
		t.Fatalf("цепочки нет в превью — issue #91; узлы: %v", tags)
	}

	// Главное утверждение issue: regex Направления отбирает цепочку так же,
	// как обычный узел. Проверяется тем же вызовом, которым отбирает превью.
	filtered, _ := config.PreviewSelectorNodes(m.PreviewNodes, config.Direction{
		Tag:     "vpn-1",
		Filters: map[string]interface{}{"tag": "/chain/i"},
	})
	if len(filtered) != 1 || filtered[0].Tag != "NL-chain" {
		t.Fatalf("regex не отобрал цепочку: %v", filtered)
	}
}

// Ядро без with_lx_chain цепочку не материализует — превью обязано
// показывать тот же пул, что уедет в конфиг, а не обещать несуществующий
// узел.
func TestRebuildPreviewCache_ChainDegradedWhenCoreLacksSupport(t *testing.T) {
	prev := config.ChainSupportProbe
	config.ChainSupportProbe = func() (bool, string) { return false, "ядро собрано без with_lx_chain" }
	defer func() { config.ChainSupportProbe = prev }()

	m := &wizardmodels.WizardModel{
		Sources: []corestate.Source{
			{
				ID: "01SRV", Type: corestate.SourceTypeServer, Enabled: true,
				NodeTag: "🇳🇱 Amsterdam",
				URI:     "vless://11111111-1111-1111-1111-111111111111@10.0.0.1:443?security=none#ams",
			},
			{
				ID: "01CHN", Type: corestate.SourceTypeChain, Enabled: true,
				NodeTag: "NL-chain",
				Chain:   &configtypes.SourceChain{Hops: []string{"🇳🇱 Amsterdam", "direct-out"}},
			},
		},
	}
	m.RefreshDerivedParserConfig()

	if _, err := RebuildPreviewCache(m); err != nil {
		t.Fatalf("RebuildPreviewCache: %v", err)
	}
	for _, n := range m.PreviewNodes {
		if n != nil && n.Tag == "NL-chain" {
			t.Fatal("цепочка показана узлом, хотя ядро её не поддерживает")
		}
	}
}
