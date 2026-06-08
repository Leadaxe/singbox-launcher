// Package config: outbound_validity.go — the three-pass validity/topology analysis over selectors.
//
// Pass 1 (buildOutboundsInfo) builds map[tag]*outboundInfo for every selector (local and global) with
// filtered nodes and an initial outboundCount. Pass 2 (computeOutboundValidity) topologically sorts by
// addOutbounds dependencies and computes the full outboundCount + isValid for each selector. Pass 3
// (generateSelectorJSONs) emits JSON only for valid selectors. SPEC 026 expose-tag merge into global
// selectors is handled by collectExposeTagCandidates / augmentGlobalOutboundDependenciesForExpose /
// globalOutboundExposeCredit.
package config

import (
	"strings"

	"singbox-launcher/internal/debuglog"
)

// outboundInfo stores information about a dynamically created outbound selector.
// This structure is used during the three-pass generation process to:
// - Pass 1: Store filtered nodes and initial node count
// - Pass 2: Calculate total outboundCount (nodes + valid addOutbounds) and validity
// - Pass 3: Generate JSON only for valid selectors with filtered addOutbounds
type outboundInfo struct {
	config        OutboundConfig // Original outbound configuration
	filteredNodes []*ParsedNode  // Nodes that match this selector's filters
	outboundCount int            // Total count: filteredNodes + valid addOutbounds (calculated in pass 2)
	isValid       bool           // true if outboundCount > 0 (set in pass 2)
	isLocal       bool           // true if it's a local selector (from proxySource.Outbounds), false if global
}

// exposeTagCandidate is a wizard local outbound tag eligible for merge into global selectors (SPEC 026).
type exposeTagCandidate struct {
	Tag     string
	Comment string
}

func commentHasWizardLocalOutboundMarker(comment string) bool {
	if strings.Contains(comment, "WIZARD:auto") {
		return true
	}
	if strings.Contains(comment, "WIZARD:select") || strings.Contains(comment, "WIZARD:selector") {
		return true
	}
	return false
}

func collectExposeTagCandidates(parserConfig *ParserConfig) []exposeTagCandidate {
	if parserConfig == nil {
		return nil
	}
	var out []exposeTagCandidate
	for _, ps := range parserConfig.ParserConfig.Proxies {
		if !ps.ExposeGroupTagsToGlobal {
			continue
		}
		for _, ob := range ps.Outbounds {
			if ob.Tag == "" || !commentHasWizardLocalOutboundMarker(ob.Comment) {
				continue
			}
			out = append(out, exposeTagCandidate{Tag: ob.Tag, Comment: ob.Comment})
		}
	}
	return out
}

func augmentGlobalOutboundDependenciesForExpose(
	outboundsInfo map[string]*outboundInfo,
	parserConfig *ParserConfig,
	exposeCandidates []exposeTagCandidate,
	dependents map[string][]string,
	inDegree map[string]int,
) {
	if parserConfig == nil || len(exposeCandidates) == 0 {
		return
	}
	edgeSeen := make(map[string]map[string]struct{})
	for _, gCfg := range parserConfig.ParserConfig.Outbounds {
		info, ok := outboundsInfo[gCfg.Tag]
		if !ok || info == nil || info.isLocal {
			continue
		}
		for _, c := range exposeCandidates {
			if !SelectorFiltersAcceptNode(gCfg.Filters, ExposeTagSyntheticNode(c.Tag, c.Comment)) {
				continue
			}
			if _, exists := outboundsInfo[c.Tag]; !exists {
				continue
			}
			if edgeSeen[c.Tag] == nil {
				edgeSeen[c.Tag] = make(map[string]struct{})
			}
			if _, dup := edgeSeen[c.Tag][gCfg.Tag]; dup {
				continue
			}
			edgeSeen[c.Tag][gCfg.Tag] = struct{}{}
			dependents[c.Tag] = append(dependents[c.Tag], gCfg.Tag)
			inDegree[gCfg.Tag]++
		}
	}
}

// globalOutboundExposeCredit counts unique expose tags that pass filters and reference a valid dynamic outbound (or unknown tag).
func globalOutboundExposeCredit(info *outboundInfo, outboundsInfo map[string]*outboundInfo, exposeCandidates []exposeTagCandidate) int {
	if info == nil || info.isLocal || len(exposeCandidates) == 0 {
		return 0
	}
	seen := make(map[string]struct{})
	credit := 0
	for _, c := range exposeCandidates {
		if _, dup := seen[c.Tag]; dup {
			continue
		}
		if !SelectorFiltersAcceptNode(info.config.Filters, ExposeTagSyntheticNode(c.Tag, c.Comment)) {
			continue
		}
		seen[c.Tag] = struct{}{}
		if ref, ok := outboundsInfo[c.Tag]; ok {
			if !ref.isValid {
				continue
			}
		}
		credit++
	}
	return credit
}

// buildOutboundsInfo implements pass 1: builds the outboundsInfo map for every selector (local and global).
// For each selector we store config, filtered nodes (from Filters), and outboundCount = len(filteredNodes).
// isValid is left false; it is set in pass 2. Duplicate tags are logged via logDuplicateTagIfExists.
func buildOutboundsInfo(
	parserConfig *ParserConfig,
	nodesBySource map[int][]*ParsedNode,
	globalNodePool []*ParsedNode,
	progressCallback func(float64, string),
) map[string]*outboundInfo {
	if progressCallback != nil {
		progressCallback(60, "Analyzing outbounds (pass 1)...")
	}
	outboundsInfo := make(map[string]*outboundInfo)

	for i, proxySource := range parserConfig.ParserConfig.Proxies {
		if len(proxySource.Outbounds) == 0 {
			continue
		}
		sourceNodes, ok := nodesBySource[i]
		if !ok {
			sourceNodes = []*ParsedNode{}
		}
		for _, outboundConfig := range proxySource.Outbounds {
			filteredNodes := filterNodesForSelector(sourceNodes, outboundConfig.Filters)
			logDuplicateTagIfExists(outboundsInfo, outboundConfig.Tag, "local", i+1)
			outboundsInfo[outboundConfig.Tag] = &outboundInfo{
				config:        outboundConfig,
				filteredNodes: filteredNodes,
				outboundCount: len(filteredNodes),
				isValid:       false,
				isLocal:       true,
			}
		}
	}

	for _, outboundConfig := range parserConfig.ParserConfig.Outbounds {
		filteredNodes := filterNodesForSelector(globalNodePool, outboundConfig.Filters)
		logDuplicateTagIfExists(outboundsInfo, outboundConfig.Tag, "global", 0)
		outboundsInfo[outboundConfig.Tag] = &outboundInfo{
			config:        outboundConfig,
			filteredNodes: filteredNodes,
			outboundCount: len(filteredNodes),
			isValid:       false,
			isLocal:       false,
		}
	}
	return outboundsInfo
}

// logDuplicateTagIfExists logs a warning when a new selector tag already exists in outboundsInfo.
// kind is "local" or "global"; sourceIndex is the 1-based proxy source index (used only when kind == "local").
func logDuplicateTagIfExists(outboundsInfo map[string]*outboundInfo, tag, kind string, sourceIndex int) {
	existingInfo, exists := outboundsInfo[tag]
	if !exists {
		return
	}
	if kind == "local" {
		selectorType := "global"
		if existingInfo.isLocal {
			selectorType = "local"
		}
		debuglog.WarnLog("GenerateOutboundsFromParserConfig: Duplicate tag '%s' detected. "+
			"Local selector from source %d will overwrite %s selector. This may cause unexpected behavior.",
			tag, sourceIndex, selectorType)
	} else {
		if existingInfo.isLocal {
			debuglog.WarnLog("GenerateOutboundsFromParserConfig: Duplicate tag '%s' detected. "+
				"Global selector will overwrite local selector. This may cause unexpected behavior.", tag)
		} else {
			debuglog.WarnLog("GenerateOutboundsFromParserConfig: Duplicate tag '%s' detected. "+
				"Multiple global selectors with same tag. This may cause unexpected behavior.", tag)
		}
	}
}

// computeOutboundValidity implements pass 2: topological sort over selectors by addOutbounds dependencies,
// then for each selector (in that order) sets outboundCount = len(filteredNodes) + count of valid addOutbounds
// (dynamic with outboundCount > 0 plus constants). isValid = (outboundCount > 0). Uses Kahn's algorithm;
// if not all selectors are processed, a cycle is reported in the log.
// Global selectors also gain edges from expose tag targets and count expose credits (SPEC 026).
func computeOutboundValidity(
	outboundsInfo map[string]*outboundInfo,
	parserConfig *ParserConfig,
	exposeCandidates []exposeTagCandidate,
	progressCallback func(float64, string),
) {
	if progressCallback != nil {
		progressCallback(70, "Calculating outbound dependencies (pass 2)...")
	}
	dependents := make(map[string][]string, len(outboundsInfo))
	inDegree := make(map[string]int, len(outboundsInfo))
	for tag := range outboundsInfo {
		inDegree[tag] = 0
		dependents[tag] = []string{}
	}
	for tag, info := range outboundsInfo {
		for _, addTag := range info.config.AddOutbounds {
			if _, exists := outboundsInfo[addTag]; exists {
				dependents[addTag] = append(dependents[addTag], tag)
				inDegree[tag]++
			}
		}
	}
	augmentGlobalOutboundDependenciesForExpose(outboundsInfo, parserConfig, exposeCandidates, dependents, inDegree)

	queue := make([]string, 0, len(outboundsInfo))
	for tag, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, tag)
		}
	}
	processedCount := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		info := outboundsInfo[current]
		totalCount := len(info.filteredNodes)
		if !info.isLocal {
			totalCount += globalOutboundExposeCredit(info, outboundsInfo, exposeCandidates)
		}
		for _, addTag := range info.config.AddOutbounds {
			if addInfo, exists := outboundsInfo[addTag]; exists {
				if addInfo.outboundCount > 0 {
					totalCount++
				}
			} else {
				totalCount++
			}
		}
		info.outboundCount = totalCount
		info.isValid = (totalCount > 0)
		processedCount++
		for _, dependent := range dependents[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}
	if processedCount != len(outboundsInfo) {
		unprocessed := make([]string, 0)
		for tag := range outboundsInfo {
			if inDegree[tag] > 0 {
				unprocessed = append(unprocessed, tag)
			}
		}
		debuglog.WarnLog("GenerateOutboundsFromParserConfig: Not all outbounds processed (processed: %d, total: %d). "+
			"Possible cycles in dependency graph. Unprocessed outbounds: %v",
			processedCount, len(outboundsInfo), unprocessed)
	}
}

// generateSelectorJSONs implements pass 3: iterates local then global selectors, and for each with isValid == true
// calls GenerateSelectorWithFilteredAddOutbounds and appends the result. Returns the slice of selector JSON strings
// and the local and global counts.
func generateSelectorJSONs(
	parserConfig *ParserConfig,
	nodesBySource map[int][]*ParsedNode,
	globalNodePool []*ParsedNode,
	outboundsInfo map[string]*outboundInfo,
	exposeCandidates []exposeTagCandidate,
	progressCallback func(float64, string),
) ([]string, int, int) {
	if progressCallback != nil {
		progressCallback(80, "Generating selectors (pass 3)...")
	}
	var out []string
	localCount := 0
	globalCount := 0

	for i, proxySource := range parserConfig.ParserConfig.Proxies {
		if len(proxySource.Outbounds) == 0 {
			continue
		}
		sourceNodes, ok := nodesBySource[i]
		if !ok {
			sourceNodes = []*ParsedNode{}
		}
		for _, outboundConfig := range proxySource.Outbounds {
			info, exists := outboundsInfo[outboundConfig.Tag]
			if !exists || !info.isValid {
				if exists && !info.isValid {
					debuglog.DebugLog("GenerateOutboundsFromParserConfig: Skipping empty local selector '%s'", outboundConfig.Tag)
				}
				continue
			}
			selectorJSON, err := GenerateSelectorWithFilteredAddOutbounds(sourceNodes, outboundConfig, outboundsInfo, false, nil)
			if err != nil {
				debuglog.WarnLog("GenerateOutboundsFromParserConfig: Failed to generate local selector %s for source %d: %v",
					outboundConfig.Tag, i+1, err)
				continue
			}
			if selectorJSON != "" {
				out = append(out, selectorJSON)
				localCount++
			}
		}
	}

	for _, outboundConfig := range parserConfig.ParserConfig.Outbounds {
		info, exists := outboundsInfo[outboundConfig.Tag]
		if !exists || !info.isValid {
			if exists && !info.isValid {
				debuglog.DebugLog("GenerateOutboundsFromParserConfig: Skipping empty global selector '%s'", outboundConfig.Tag)
			}
			continue
		}
		selectorJSON, err := GenerateSelectorWithFilteredAddOutbounds(globalNodePool, outboundConfig, outboundsInfo, true, exposeCandidates)
		if err != nil {
			debuglog.WarnLog("GenerateOutboundsFromParserConfig: Failed to generate global selector %s: %v",
				outboundConfig.Tag, err)
			continue
		}
		if selectorJSON != "" {
			out = append(out, selectorJSON)
			globalCount++
		}
	}
	return out, localCount, globalCount
}
