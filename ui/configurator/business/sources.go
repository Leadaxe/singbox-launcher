// Package business — SPEC 052 phase 8: Sources canonical, ParserConfig derived.
//
// AppendURLsToSources / ApplyURLsToSources заменяют старые
// AppendURLsToParserConfig / ApplyURLToParserConfig — мутируют canonical
// `model.Sources` напрямую, потом вызывают `RefreshDerivedParserConfig`
// для синхронизации derived `ParserConfig`/`ParserConfigJSON`.
package business

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	corestate "singbox-launcher/core/state"
	v5 "singbox-launcher/core/state/v5"
	"singbox-launcher/internal/debuglog"
)

// AppendURLsToSources парсит multi-line input, классифицирует строки на
// subscription URL'ы и direct-link URI, и добавляет каждую как отдельный
// `corestate.Source` в `model.Sources` (subscription→Type=subscription;
// direct-link→Type=server, один Source per URI).
//
// Дубликаты по URL/URI пропускаются.
//
// SPEC 052 phase 8: каждый прямой линк — собственный Source(server) с
// `Label=fragment` (или fallback `server-N`). Это семантическое отличие
// от legacy, где все direct-links группировались в один ProxySource{
// Connections:[...]}; в v5 schema каждый сервер — индивидуальная сущность.
func AppendURLsToSources(ctx UIUpdater, input string) error {
	model := ctx.Model()
	updater := ctx
	timing := debuglog.StartTiming("appendURLsToSources")
	defer timing.EndWithDefer()

	if input == "" {
		return fmt.Errorf("input is empty")
	}
	subs, conns := classifyInputLines(input, timing)
	if len(subs) == 0 && len(conns) == 0 {
		return fmt.Errorf("no valid URLs to add")
	}

	// Build URL/URI lookup maps for de-dup.
	existingURLs := make(map[string]struct{}, len(model.Sources))
	existingURIs := make(map[string]struct{}, len(model.Sources))
	for _, src := range model.Sources {
		switch src.Type {
		case corestate.SourceTypeSubscription:
			if src.URL != "" {
				existingURLs[src.URL] = struct{}{}
			}
		case corestate.SourceTypeServer:
			if src.URI != "" {
				existingURIs[src.URI] = struct{}{}
			}
		}
	}

	startIndex := len(model.Sources) + 1
	added := 0

	for _, subURL := range subs {
		if _, ok := existingURLs[subURL]; ok {
			continue
		}
		idx := startIndex + added
		newSrc := corestate.Source{
			ID:      v5.MakeULID(),
			Type:    corestate.SourceTypeSubscription,
			Enabled: true,
			URL:     subURL,
		}
		// tag_prefix derived from URL fragment (#abvpn → "abvpn:") иначе
		// generated `1:`, `2:` per index.
		prefix := tagPrefixFromSubscriptionFragment(subURL)
		if prefix == "" {
			prefix = GenerateTagPrefix(idx)
		}
		newSrc.Tag = &corestate.TagSpec{Prefix: prefix}
		model.Sources = append(model.Sources, newSrc)
		existingURLs[subURL] = struct{}{}
		added++
	}

	for _, uri := range conns {
		if _, ok := existingURIs[uri]; ok {
			continue
		}
		label := extractURIFragment(uri)
		if label == "" {
			label = fmt.Sprintf("server-%d", startIndex+added)
		}
		newSrc := corestate.Source{
			ID:      v5.MakeULID(),
			Type:    corestate.SourceTypeServer,
			Enabled: true,
			Label:   label,
			URI:     uri,
		}
		model.Sources = append(model.Sources, newSrc)
		existingURIs[uri] = struct{}{}
		added++
	}

	if added == 0 {
		return nil
	}

	// Refresh derived caches & UI.
	model.RefreshDerivedParserConfig()
	model.PreviewNeedsParse = true
	InvalidatePreviewCache(model)
	updater.UpdateParserConfig(model.ParserConfigJSON)
	timing.LogTiming("append sources", time.Since(time.Now()))
	return nil
}

// extractURIFragment — `vless://...#name` → "name" (percent-decoded).
// Edge cases: empty fragment / no '#' → "".
func extractURIFragment(s string) string {
	hashAt := strings.Index(s, "#")
	if hashAt < 0 {
		return ""
	}
	frag := s[hashAt+1:]
	if frag == "" {
		return ""
	}
	if dec, err := url.QueryUnescape(frag); err == nil {
		return strings.TrimSpace(dec)
	}
	return strings.TrimSpace(frag)
}

// classifyInputLinesV2 — public wrapper над classifyInputLines для тестов.
// Возвращает subscriptions/connections в порядке появления в input.
func ClassifyInput(input string) (subscriptions []string, connections []string) {
	return classifyInputLinesV2(input)
}

// classifyInputLinesV2 — копия classifyInputLines без timing аргумента.
// Используется AppendURLsToSources для самосостоятельной классификации.
func classifyInputLinesV2(input string) (subscriptions []string, connections []string) {
	subscriptions = make([]string, 0)
	connections = make([]string, 0)
	for _, line := range strings.Split(input, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if subscription.IsSubscriptionURL(line) {
			subscriptions = append(subscriptions, line)
		} else if subscription.IsDirectLink(line) {
			connections = append(connections, line)
		}
	}
	return subscriptions, connections
}

// Compile-time guard: импорт configtypes используется (легкая зависимость
// для других callsite'ов). Не удалять.
var _ configtypes.ProxySource
