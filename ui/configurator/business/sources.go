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
	"strconv"
	"strings"
	"time"

	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
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

	// Вставленный sing-box JSON разбирается до построчного классификатора:
	// документ многострочный, и цикл по строкам не нашёл бы в нём ни ссылки.
	// isJSON отделяет «не JSON» от «битый JSON» — второе обязано дойти до
	// пользователя ошибкой, а не общим «no valid URLs to add».
	jsonNodes, isJSON, jsonErr := carveSingboxJSON(input)
	if jsonErr != nil {
		return jsonErr
	}

	var subs, conns []string
	if !isJSON {
		subs, conns = classifyInputLines(input, timing)
	}
	if len(subs) == 0 && len(conns) == 0 && len(jsonNodes) == 0 {
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
			ID:      corestate.MakeULID(),
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
			ID:      corestate.MakeULID(),
			Type:    corestate.SourceTypeServer,
			Enabled: true,
			Label:   label,
			URI:     uri,
		}
		model.Sources = append(model.Sources, newSrc)
		existingURIs[uri] = struct{}{}
		added++
	}

	// JSON-узлы: каждый outbound — отдельный Source(server) с ConfigJSON и
	// пустым URI. Дедупа по URI здесь нет — два одинаковых outbound'а это
	// осознанная вставка, а сравнивать документы побайтово смысла мало.
	for _, jn := range jsonNodes {
		label := jn.Label
		if label == "" {
			label = fmt.Sprintf("server-%d", startIndex+added)
		}
		model.Sources = append(model.Sources, corestate.Source{
			ID:         corestate.MakeULID(),
			Type:       corestate.SourceTypeServer,
			Enabled:    true,
			Label:      label,
			ConfigJSON: jn.ConfigJSON,
		})
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

// NextChainLabel — свободное имя для новой цепочки (SPEC 110).
//
// Имя цепочки становится тегом её узла, а тег обязан быть уникален: два
// одинаковых в конфиге ядро принимает, но выбор между ними становится
// неопределённым. Поэтому имя выдаётся автоматически, а не оставляется
// пустым.
func NextChainLabel(sources []corestate.Source) string {
	used := make(map[string]bool, len(sources))
	for _, src := range sources {
		if src.Label != "" {
			used[src.Label] = true
		}
	}
	for i := 1; ; i++ {
		name := "chain-" + strconv.Itoa(i)
		if !used[name] {
			return name
		}
	}
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

// Compile-time guard: импорт configtypes используется (легкая зависимость
// для других callsite'ов). Не удалять.
var _ configtypes.ProxySource
