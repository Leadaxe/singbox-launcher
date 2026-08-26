// File preview_dedup_test.go — превью обязано совпадать с боевым разбором.
package business

import (
	"fmt"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 112-B часть A, пункт 4: счётчик узлов на строке источника выведен из
// кэша превью, а тот разбирается тем же LoadNodesFromSource, что и сборка.
// Расхождение читается пользователем как потерянные записи (память
// lazy-cache-vs-lost-state), поэтому дедуп обязан быть виден и здесь.
//
// Форма подписки — darkline: 32 байт-одинаковых ss:// плюс несколько
// самостоятельных узлов. До SPEC 112-B строка показывала все 39.
func TestPreviewNodeCountsSeeDedup(t *testing.T) {
	const dup = "ss://YWVzLTI1Ni1nY206c2VjcmV0cGFzcw@DARK-BOT:443"
	lines := make([]string, 0, 39)
	for i := 0; i < 32; i++ {
		lines = append(lines, fmt.Sprintf("%s#копия %d", dup, i))
	}
	for i := 0; i < 7; i++ {
		lines = append(lines,
			fmt.Sprintf("ss://YWVzLTI1Ni1nY206c2VjcmV0cGFzcw@host-%d:443#узел %d", i, i))
	}

	const url = "https://example.invalid/darkline"
	prev := subscription.LookupCachedBody
	subscription.LookupCachedBody = func(requested string) ([]byte, bool) {
		if requested == url {
			return []byte(strings.Join(lines, "\n")), true
		}
		return nil, false
	}
	t.Cleanup(func() { subscription.LookupCachedBody = prev })

	m := &wizardmodels.WizardModel{
		Sources: []corestate.Source{{
			ID:      "01DARK",
			Type:    corestate.SourceTypeSubscription,
			Enabled: true,
			URL:     url,
		}},
	}
	m.RefreshDerivedParserConfig()

	if _, err := RebuildPreviewCache(m); err != nil {
		t.Fatalf("RebuildPreviewCache: %v", err)
	}
	if !EnsureSourceNodeCounts(m) {
		t.Fatal("счёт не выполнен")
	}

	c := m.SourceNodeCounts[0]
	if c.Total != 8 {
		tags := make([]string, 0, len(m.PreviewNodesBySource[0]))
		for _, n := range m.PreviewNodesBySource[0] {
			tags = append(tags, n.Tag)
		}
		t.Fatalf("счётчик показал %d узлов, ожидалось 8 (32 копии → 1 плюс 7 своих); теги: %v",
			c.Total, tags)
	}
}
