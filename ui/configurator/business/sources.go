// Package business — SPEC 052 phase 8: Sources canonical, ParserConfig derived.
//
// AppendURLsToSources / ApplyURLsToSources заменяют старые
// AppendURLsToParserConfig / ApplyURLToParserConfig — мутируют canonical
// `model.Sources` напрямую и поднимают ревизию модели (`BumpRevision`,
// SPEC 117): производные результаты перечитываются от canonical.
package business

import (
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
//
// SPEC 116 W6: сам РАЗБОР текста живёт в `parseSourceInput`
// (source_input.go) — он же обслуживает наполнение папки. Здесь остался
// только адрес назначения: «каждый узел — отдельным Source в корень».
func AppendURLsToSources(ctx UIUpdater, input string) error {
	model := ctx.Model()
	updater := ctx
	timing := debuglog.StartTiming("appendURLsToSources")
	defer timing.EndWithDefer()

	parsed, err := parseSourceInput(input, len(model.Sources))
	if err != nil {
		return err
	}
	subs := parsed.Subscriptions

	// Build URL/URI lookup maps for de-dup.
	existingURLs := make(map[string]struct{}, len(model.Sources))
	existingURIs := make(map[string]struct{}, len(model.Sources))
	for _, src := range model.Sources {
		switch src.Kind {
		case corestate.SourceKindSubscription:
			if src.URL != "" {
				existingURLs[src.URL] = struct{}{}
			}
		case corestate.SourceKindServer:
			if src.Origin != nil && src.Origin.Kind == corestate.OriginKindURI && src.Origin.Raw != "" {
				existingURIs[src.Origin.Raw] = struct{}{}
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
			Node: corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: true},
			ID:   corestate.MakeULID(),
			URL:  subURL,
		}
		// tag_prefix derived from URL fragment (#abvpn → "abvpn:") иначе
		// generated `1:`, `2:` per index.
		prefix := tagPrefixFromSubscriptionFragment(subURL)
		if prefix == "" {
			prefix = GenerateTagPrefix(idx)
		}
		newSrc.TagPolicy = &corestate.TagPolicy{Prefix: prefix}
		model.Sources = append(model.Sources, newSrc)
		existingURLs[subURL] = struct{}{}
		added++
	}

	// Узлы (share-URI, wg-INI, вставленный sing-box JSON) — каждый отдельным
	// Source(server) в корень. Разбор уже сделан ядром: здесь только дедуп по
	// исходному URI и упаковка в Source.
	//
	// Дедупа для JSON-узлов нет (у них `URIOf == ""`): два одинаковых
	// outbound'а — осознанная вставка, а сравнивать документы побайтово
	// смысла мало.
	for i := range parsed.Nodes {
		uri := parsed.URIOf[i]
		if uri != "" {
			if _, ok := existingURIs[uri]; ok {
				continue
			}
			existingURIs[uri] = struct{}{}
		}
		model.Sources = append(model.Sources, corestate.Source{
			Node: parsed.Nodes[i],
			ID:   corestate.MakeULID(),
		})
		added++
	}

	if added == 0 {
		return nil
	}

	// Bump revision & refresh UI.
	model.BumpRevision()
	model.PreviewNeedsParse = true
	InvalidateNodePool(model)
	updater.RefreshOutboundsConfiguratorList()
	timing.LogTiming("append sources", time.Since(time.Now()))
	return nil
}

// NextChainLabel — свободный ТЕГ для новой цепочки (SPEC 110).
//
// Тег обязан быть уникален: два одинаковых в конфиге ядро принимает, но
// выбор между ними становится неопределённым. Поэтому он выдаётся
// автоматически, а не оставляется пустым.
//
// Занятость считается по тегам (NodeTagOrLabel), а не по подписям: подписи
// пользователь волен дублировать, и коллизия тегов от этого не зависит.
func NextChainLabel(sources []corestate.Source) string {
	used := make(map[string]bool, len(sources))
	for _, src := range sources {
		if tag := src.NodeTagOrLabel(); tag != "" {
			used[tag] = true
		}
	}
	for i := 1; ; i++ {
		name := "chain-" + strconv.Itoa(i)
		if !used[name] {
			return name
		}
	}
}

// NextFolderName — свободное ИМЯ для новой папки (SPEC 116 этап 3, W3).
//
// В отличие от цепочки (NextChainLabel) это НЕ тег: у папки тега нет, её
// имя — `Source.Name`, подпись контейнера. На него не ссылаются ни правила,
// ни фильтры Направлений (узлы папки адресуются своими сырыми тегами через
// пару «FolderID + тег»), поэтому уникальность здесь — удобство списка, а
// не требование конфига: две «Folder 1» рядом человек не различит.
//
// Занятость считается по именам КОНТЕЙНЕРОВ (папок и подписок): подписка
// со своим profile_title тоже стоит в этом же списке, и совпадение имён
// путало бы ровно так же.
func NextFolderName(sources []corestate.Source) string {
	used := make(map[string]bool, len(sources))
	for _, src := range sources {
		if name := strings.TrimSpace(src.Name); name != "" {
			used[name] = true
		}
	}
	for i := 1; ; i++ {
		name := "Folder " + strconv.Itoa(i)
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
