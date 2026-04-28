package business

import (
	"fmt"
	"time"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// RebuildPreviewCache rebuilds the preview cache (parsed nodes) for the wizard model.
// It uses the same subscription loader as the core config generator (subscription.LoadNodesFromSource)
// to ensure identical parsing and tag processing.
//
// It fills:
//   - model.PreviewNodes: all nodes from all sources;
//   - model.PreviewNodesBySource: nodes grouped by source index in ParserConfig.ParserConfig.Proxies.
//
// It returns the number of sources that failed to load (errorCount) and an error for fatal failures.
func RebuildPreviewCache(model *wizardmodels.WizardModel) (int, error) {
	timing := debuglog.StartTiming("wizardPreviewCache")
	defer timing.EndWithDefer()

	if model == nil {
		return 0, fmt.Errorf("wizard model is nil")
	}

	// SPEC 052 phase 8: ParserConfig — derived view; если не заполнен,
	// перегенерируем из canonical Sources/GlobalOutbounds/Defaults.
	if model.ParserConfig == nil {
		model.RefreshDerivedParserConfig()
	}

	if model.ParserConfig == nil {
		model.PreviewNodes = nil
		model.PreviewNodesBySource = nil
		return 0, nil
	}

	proxies := model.ParserConfig.ParserConfig.Proxies
	totalSources := len(proxies)
	if totalSources == 0 {
		model.PreviewNodes = nil
		model.PreviewNodesBySource = nil
		return 0, nil
	}

	tagCounts := make(map[string]int)
	nodesBySource := make(map[int][]*config.ParsedNode, totalSources)
	allNodes := make([]*config.ParsedNode, 0)
	errorCount := 0

	loadTimingStart := time.Now()
	debuglog.DebugLog("wizardPreviewCache: starting LoadNodesFromSource for %d sources", totalSources)

	for i, ps := range proxies {
		nodes, err := subscription.LoadNodesFromSource(ps, tagCounts, nil, i, totalSources)
		if err != nil {
			errorCount++
			debuglog.DebugLog("wizardPreviewCache: LoadNodesFromSource error for source %d/%d: %v", i+1, totalSources, err)
			continue
		}
		if len(nodes) == 0 {
			continue
		}
		for _, n := range nodes {
			n.SourceIndex = i
		}
		nodesBySource[i] = nodes
		allNodes = append(allNodes, nodes...)
	}

	timing.LogTiming("load nodes for preview", time.Since(loadTimingStart))
	debuglog.DebugLog("wizardPreviewCache: loaded %d nodes from %d sources (errors: %d)", len(allNodes), totalSources, errorCount)

	model.PreviewNodes = allNodes
	if len(nodesBySource) > 0 {
		model.PreviewNodesBySource = nodesBySource
	} else {
		model.PreviewNodesBySource = nil
	}

	return errorCount, nil
}

// InvalidatePreviewCache clears the preview cache so that the next consumer (Sources Refresh, View, Edit Outbound Preview) will rebuild via RebuildPreviewCache.
// Call this whenever ParserConfig or sources change (Add/Del source, prefix change, configurator apply, manual JSON apply).
func InvalidatePreviewCache(model *wizardmodels.WizardModel) {
	if model == nil {
		return
	}
	model.PreviewNodes = nil
	model.PreviewNodesBySource = nil
	model.AvailableOutboundsMemoKey = ""
	model.AvailableOutboundsMemoTags = nil
}

