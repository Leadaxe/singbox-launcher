package core

import (
	"fmt"
	"os"
	"path/filepath"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
	v5 "singbox-launcher/core/state/v5"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// buildSnapshotFromRawCache — SPEC 052 phase 6: парсит `bin/subscriptions/*.raw`
// в память и строит ParsedCache, готовый для BuildConfig. **Без network call'ов.**
//
// Контракт:
//   - Для каждого enabled subscription Source в state.Connections.Sources
//     ищет matching `.raw` файл по ID;
//   - Server source'ы парсятся напрямую из URI (не нуждаются в .raw);
//   - Если хоть один enabled subscription без `.raw` — возвращает (nil, ErrRawCacheIncomplete);
//     caller делает auto-Update fallback.
func buildSnapshotFromRawCache(s *state.State, execDir string, subst config.VarSubstituter) (*build.ParsedCache, error) {
	if s == nil {
		return nil, fmt.Errorf("buildSnapshotFromRawCache: nil state")
	}
	subsDir := platform.GetSubscriptionsDir(execDir)

	// Проверяем completeness: для каждой enabled subscription есть .raw?
	missing := []string{}
	for _, src := range s.Connections.Sources {
		if src.Type != state.SourceTypeSubscription || !src.Enabled || src.URL == "" {
			continue
		}
		path := filepath.Join(subsDir, src.ID+".raw")
		if _, err := os.Stat(path); err != nil {
			missing = append(missing, src.URL)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: %d subscription(s) missing raw cache (e.g. %s)",
			ErrRawCacheIncomplete, len(missing), missing[0])
	}

	// URL → decoded body lookup для парсера.
	bodyByURL := buildBodyLookup(s, subsDir)

	prev := subscription.LookupCachedBody
	subscription.LookupCachedBody = func(url string) ([]byte, bool) {
		b, ok := bodyByURL[url]
		return b, ok
	}
	defer func() { subscription.LookupCachedBody = prev }()

	parserCfg := s.ParserConfig
	if subst != nil {
		config.SubstituteParserConfigPlaceholders(&parserCfg, subst)
	} else {
		// Caller не передал — берём дефолтный (template + state vars с диска).
		def := config.BuildVarSubstituterFromDisk(execDir)
		config.SubstituteParserConfigPlaceholders(&parserCfg, def)
	}

	tagCounts := make(map[string]int)
	loadNodesFunc := func(ps configtypes.ProxySource, tc map[string]int, pc func(float64, string), idx, total int) ([]*configtypes.ParsedNode, error) {
		return subscription.LoadNodesFromSource(ps, tc, pc, idx, total)
	}

	result, err := config.GenerateOutboundsFromParserConfig(&parserCfg, tagCounts, nil, loadNodesFunc)
	if err != nil {
		return nil, fmt.Errorf("generate outbounds from raw cache: %w", err)
	}

	subscription.LogDuplicateTagStatistics(tagCounts, "Rebuild")

	return &build.ParsedCache{
		Outbounds: jsonStringsToRawMessages(result.OutboundsJSON),
		Endpoints: jsonStringsToRawMessages(result.EndpointsJSON),
	}, nil
}

// ErrRawCacheIncomplete — sentinel для отсутствующих .raw файлов.
// Rebuild делает auto-Update fallback при этой ошибке.
var ErrRawCacheIncomplete = fmt.Errorf("raw cache incomplete")

// buildBodyLookup — формирует URL → decoded body map для всех subscription
// source'ов в state. Decoded — потому что FetchSubscription возвращает
// уже decoded content (после base64 strip), а LookupCachedBody hook должен
// мимикрировать тот же контракт.
func buildBodyLookup(s *state.State, subsDir string) map[string][]byte {
	out := make(map[string][]byte, len(s.Connections.Sources))
	for _, src := range s.Connections.Sources {
		if src.Type != state.SourceTypeSubscription || !src.Enabled || src.URL == "" {
			continue
		}
		raw, err := v5.ReadRawBody(subsDir, src.ID)
		if err != nil {
			debuglog.WarnLog("buildBodyLookup: read raw for %s: %v", src.ID, err)
			continue
		}
		// FetchSubscription возвращает decoded — мимикрируем тот же контракт.
		if dec, err := subscription.DecodeSubscriptionContent(raw); err == nil {
			out[src.URL] = dec
		} else {
			out[src.URL] = raw
		}
	}
	return out
}
