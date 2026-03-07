// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл parser.go является оркестратором/координатором, который вызывает реальные парсеры
// из core-модулей, но сам не содержит логику парсинга. Его функции:
//   - ParseAndPreview - координирует генерацию outbounds через ConfigService.GenerateOutboundsFromParserConfig
//   - ApplyURLToParserConfig - применяет URL к ParserConfig (работает со структурами config.ParserConfig)
//   - SerializeParserConfig - сериализует через config.NormalizeParserConfig
//
// Файл работает в контексте визарда (использует WizardModel и UIUpdater для обновления GUI).
// Координирует вызовы реальных парсеров из core/config/subscription и core/config.
// Интегрирован с GUI через UIUpdater (обновляет GUI прогресс, статусы и preview).
//
// Реальная логика парсинга находится в:
//   - core/config/parser - парсинг @ParserConfig блоков из файлов
//   - core/config/subscription - парсинг URL подписок и прямых ссылок
//   - core/config - генерация outbounds из ParserConfig
package business

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardutils "singbox-launcher/ui/wizard/utils"
)

// ParseAndPreview parses ParserConfig and generates outbounds preview.
// It reads model from ctx and updates UI through ctx (UIUpdater).
func ParseAndPreview(ctx UIUpdater, configService ConfigService) error {
	model := ctx.Model()
	updater := ctx
	timing := debuglog.StartTiming("parseAndPreview")
	defer func() {
		timing.End()
		model.AutoParseInProgress = false
	}()

	// Save button stays visible; save flow waits for parse if needed (waitForParsingIfNeeded)

	// Parse ParserConfig from field
	parseStartTime := time.Now()
	parserConfigJSON := strings.TrimSpace(model.ParserConfigJSON)
	debuglog.DebugLog("parseAndPreview: ParserConfig text length: %d bytes", len(parserConfigJSON))
	if parserConfigJSON == "" {
		debuglog.DebugLog("parseAndPreview: ParserConfig is empty, returning early")
		updater.UpdateSaveButtonText("Save")
		return fmt.Errorf("parserConfig is empty")
	}

	// Validate JSON size before parsing
	if err := ValidateJSONSize([]byte(parserConfigJSON)); err != nil {
		debuglog.DebugLog("parseAndPreview: ParserConfig JSON size validation failed: %v", err)
		updater.UpdateSaveButtonText("Save")
		return err
	}

	var parserConfig config.ParserConfig
	if err := json.Unmarshal([]byte(parserConfigJSON), &parserConfig); err != nil {
		timing.LogTiming("parse ParserConfig JSON", time.Since(parseStartTime))
		debuglog.DebugLog("parseAndPreview: Failed to parse ParserConfig JSON: %v", err)
		updater.UpdateSaveButtonText("Save")
		return fmt.Errorf("failed to parse ParserConfig JSON: %w", err)
	}

	// Validate ParserConfig structure
	if err := ValidateParserConfig(&parserConfig); err != nil {
		debuglog.DebugLog("parseAndPreview: ParserConfig validation failed: %v", err)
		updater.UpdateSaveButtonText("Save")
		return err
	}
	timing.LogTiming("parse ParserConfig", time.Since(parseStartTime))
	debuglog.DebugLog("parseAndPreview: Parsed ParserConfig (sources: %d, outbounds: %d)",
		len(parserConfig.ParserConfig.Proxies), len(parserConfig.ParserConfig.Outbounds))

	// Generate outbounds from current ParserConfig only. Do not apply SourceURLs here:
	// applying would replace all proxies with the URL field content and drop other sources
	// (e.g. after reopening wizard and editing prefixes, switching to Preview would overwrite).

	// Generate all outbounds using unified function
	// This eliminates code duplication and adds support for local outbounds
	generateStartTime := time.Now()
	debuglog.DebugLog("parseAndPreview: Starting outbound generation using unified function")

	tagCounts := make(map[string]int)
	debuglog.DebugLog("parseAndPreview: Initializing tag deduplication tracker")

	var lastProgressUpdate time.Time
	progressCallback := func(p float64, s string) {
		now := time.Now()
		if now.Sub(lastProgressUpdate) < wizardutils.ProgressUpdateInterval {
			return
		}
		lastProgressUpdate = now
		// Progress no longer shown in UI (Outbounds preview removed)
		_ = s
	}

	result, err := configService.GenerateOutboundsFromParserConfig(
		&parserConfig, tagCounts, progressCallback)
	if err != nil {
		timing.LogTiming("generate outbounds", time.Since(generateStartTime))
		debuglog.DebugLog("parseAndPreview: Failed to generate outbounds: %v", err)
		updater.UpdateSaveButtonText("Save")
		return fmt.Errorf("failed to generate outbounds: %w", err)
	}

	subscription.LogDuplicateTagStatistics(tagCounts, "ConfigWizard")

	model.OutboundStats.NodesCount = result.NodesCount
	model.OutboundStats.EndpointsCount = result.EndpointsCount
	model.OutboundStats.LocalSelectorsCount = result.LocalSelectorsCount
	model.OutboundStats.GlobalSelectorsCount = result.GlobalSelectorsCount
	model.GeneratedOutbounds = result.OutboundsJSON
	model.GeneratedEndpoints = result.EndpointsJSON

	timing.LogTiming("total outbound generation", time.Since(generateStartTime))

	updater.UpdateSaveButtonText("Save")
	model.ParserConfig = &parserConfig
	model.PreviewNeedsParse = false
	// RefreshOutboundOptions will be called by presenter
	if model.TemplateData != nil && (len(model.GeneratedOutbounds) > 0 || len(model.GeneratedEndpoints) > 0) {
		model.TemplatePreviewNeedsUpdate = true
		// go UpdateTemplatePreviewAsync(model, updater) // This will be called by presenter
	}
	return nil
}

// ApplyURLToParserConfig applies URL input to ParserConfig, correctly separating subscriptions and connections.
// It preserves existing local outbounds, tag_prefix, and tag_postfix for each source.
// Reads model from ctx (UIUpdater).
func ApplyURLToParserConfig(ctx UIUpdater, input string) error {
	model := ctx.Model()
	updater := ctx
	timing := debuglog.StartTiming("applyURLToParserConfig")
	defer timing.EndWithDefer()
	debuglog.DebugLog("applyURLToParserConfig: input length: %d bytes", len(input))

	// Validate input
	if err := validateApplyURLInput(input, model.ParserConfigJSON); err != nil {
		return err
	}

	// Parse ParserConfig
	parserConfig, err := parseParserConfigForApply(model.ParserConfigJSON, timing)
	if err != nil {
		return err
	}

	// Classify input lines into subscriptions and connections
	subscriptions, connections := classifyInputLines(input, timing)

	// Preserve existing properties from current ParserConfig
	existingProps := preserveExistingProperties(parserConfig)

	// Build proxies from unified list (subscriptions + connection block); indices 1, 2, 3, ...
	items := toProxyInputs(subscriptions, connections)
	newProxies := buildProxiesFromInputs(items, existingProps, nil, 1)

	if len(newProxies) == 0 {
		newProxies = []config.ProxySource{{}}
	}

	return updateAndSerializeParserConfig(parserConfig, newProxies, subscriptions, connections, model, updater, timing)
}

// AppendURLsToParserConfig appends URL(s) from input to existing ParserConfig proxies.
// Existing sources are kept; only Del button removes them.
// Reads model from ctx (UIUpdater).
func AppendURLsToParserConfig(ctx UIUpdater, input string) error {
	model := ctx.Model()
	updater := ctx
	timing := debuglog.StartTiming("appendURLsToParserConfig")
	defer timing.EndWithDefer()
	debuglog.DebugLog("appendURLsToParserConfig: input length: %d bytes", len(input))

	if err := validateApplyURLInput(input, model.ParserConfigJSON); err != nil {
		return err
	}

	parserConfig, err := parseParserConfigForApply(model.ParserConfigJSON, timing)
	if err != nil {
		return err
	}

	subscriptions, connections := classifyInputLines(input, timing)
	if len(subscriptions) == 0 && len(connections) == 0 {
		debuglog.DebugLog("appendURLsToParserConfig: no valid URLs to add")
		return fmt.Errorf("no valid URLs to add")
	}

	existingProxies := parserConfig.ParserConfig.Proxies
	existingProps := preserveExistingProperties(parserConfig)

	// Skip subscription URLs that already exist (no duplicates)
	existingSources := make(map[string]bool)
	for _, p := range existingProxies {
		if p.Source != "" {
			existingSources[p.Source] = true
		}
	}
	var uniqueSubs []string
	for _, s := range subscriptions {
		if !existingSources[s] {
			uniqueSubs = append(uniqueSubs, s)
		}
	}

	// Common index: new proxies get indices after existing (e.g. existing 1,2,3 -> new get 4, 5, ...)
	items := toProxyInputs(uniqueSubs, connections)
	additionalProxies := buildProxiesFromInputs(items, existingProps, existingProxies, len(existingProxies)+1)

	if len(additionalProxies) == 0 {
		debuglog.DebugLog("appendURLsToParserConfig: all URLs already present, nothing to add")
		return nil
	}

	newProxies := append(existingProxies, additionalProxies...)
	return updateAndSerializeParserConfig(parserConfig, newProxies, uniqueSubs, connections, model, updater, timing)
}

// validateApplyURLInput проверяет входные данные перед применением URL.
func validateApplyURLInput(input, parserConfigJSON string) error {
	if input == "" {
		debuglog.DebugLog("applyURLToParserConfig: input is empty, returning early")
		return fmt.Errorf("input is empty")
	}
	text := strings.TrimSpace(parserConfigJSON)
	if text == "" {
		debuglog.DebugLog("applyURLToParserConfig: ParserConfigJSON text is empty, returning early")
		return fmt.Errorf("parserConfigJSON is empty")
	}
	return nil
}

// parseParserConfigForApply парсит ParserConfig из JSON строки.
func parseParserConfigForApply(parserConfigJSON string, timing interface{ LogTiming(string, time.Duration) }) (*config.ParserConfig, error) {
	parseStartTime := time.Now()
	var parserConfig config.ParserConfig
	text := strings.TrimSpace(parserConfigJSON)
	if err := json.Unmarshal([]byte(text), &parserConfig); err != nil {
		timing.LogTiming("parse ParserConfig", time.Since(parseStartTime))
		debuglog.DebugLog("applyURLToParserConfig: Failed to parse ParserConfig: %v", err)
		return nil, fmt.Errorf("failed to parse ParserConfig: %w", err)
	}
	timing.LogTiming("parse ParserConfig", time.Since(parseStartTime))
	debuglog.DebugLog("applyURLToParserConfig: Parsed ParserConfig (outbounds: %d)",
		len(parserConfig.ParserConfig.Outbounds))
	return &parserConfig, nil
}

// classifyInputLines классифицирует входные строки на подписки и прямые ссылки.
func classifyInputLines(input string, timing interface{ LogTiming(string, time.Duration) }) (subscriptions []string, connections []string) {
	splitStartTime := time.Now()
	lines := strings.Split(input, "\n")
	debuglog.DebugLog("applyURLToParserConfig: Split input into %d lines", len(lines))

	subscriptions = make([]string, 0)
	connections = make([]string, 0)

	for _, line := range lines {
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

	timing.LogTiming("classify lines", time.Since(splitStartTime))
	debuglog.DebugLog("applyURLToParserConfig: Classified lines: %d subscriptions, %d connections",
		len(subscriptions), len(connections))
	return subscriptions, connections
}

// existingProperties содержит сохраненные свойства существующих ProxySource.
type existingProperties struct {
	OutboundsMap         map[string][]config.OutboundConfig
	TagPrefixMap         map[string]string
	TagPostfixMap        map[string]string
	ConnectionsProxies   []config.ProxySource
}

// preserveExistingProperties сохраняет существующие свойства из текущего ParserConfig.
func preserveExistingProperties(parserConfig *config.ParserConfig) *existingProperties {
	props := &existingProperties{
		OutboundsMap:       make(map[string][]config.OutboundConfig),
		TagPrefixMap:       make(map[string]string),
		TagPostfixMap:      make(map[string]string),
		ConnectionsProxies: make([]config.ProxySource, 0),
	}

	for _, existingProxy := range parserConfig.ParserConfig.Proxies {
		if existingProxy.Source != "" {
			props.OutboundsMap[existingProxy.Source] = existingProxy.Outbounds
			if existingProxy.TagPrefix != "" {
				props.TagPrefixMap[existingProxy.Source] = existingProxy.TagPrefix
			}
			if existingProxy.TagPostfix != "" {
				props.TagPostfixMap[existingProxy.Source] = existingProxy.TagPostfix
			}
		} else if isConnectionOnlyProxy(existingProxy) {
			props.ConnectionsProxies = append(props.ConnectionsProxies, existingProxy)
		}
	}

	return props
}

// proxyInput is one entry for the proxies section: either a subscription (URL) or a connection-only block.
type proxyInput struct {
	Subscription string   // non-empty = subscription source
	Connections  []string // non-empty = connection-only block
}

// toProxyInputs builds a single list of proxy inputs from classified subscriptions and connections.
func toProxyInputs(subscriptions, connections []string) []proxyInput {
	items := make([]proxyInput, 0, len(subscriptions)+1)
	for _, sub := range subscriptions {
		items = append(items, proxyInput{Subscription: sub})
	}
	if len(connections) > 0 {
		items = append(items, proxyInput{Connections: connections})
	}
	return items
}

// buildProxiesFromInputs builds []config.ProxySource from a unified list of inputs (subscriptions and connection block).
// For each input: match existing from existingProps or create new with common index.
// startIndex: 1-based index for the first proxy added (so Append continues numbering after existingProxies; use 1 for Apply).
// skipConnectionsIfIn: when non-nil (Append mode), do not add a connection proxy if its connections are already in this list.
func buildProxiesFromInputs(
	items []proxyInput,
	existingProps *existingProperties,
	skipConnectionsIfIn []config.ProxySource,
	startIndex int,
) []config.ProxySource {
	result := make([]config.ProxySource, 0, len(items))
	for _, item := range items {
		nextIndex := startIndex + len(result)
		if item.Subscription != "" {
			proxySource := config.ProxySource{Source: item.Subscription}
			if existingOutbounds, ok := existingProps.OutboundsMap[item.Subscription]; ok {
				proxySource.Outbounds = existingOutbounds
				debuglog.DebugLog("applyURLToParserConfig: Restored %d local outbounds for subscription: %s", len(existingOutbounds), item.Subscription)
			}
			restoreTagPrefixAndPostfix(&proxySource, item.Subscription, existingProps, fmt.Sprintf("subscription: %s", item.Subscription))
			if proxySource.TagPrefix == "" {
				proxySource.TagPrefix = GenerateTagPrefix(nextIndex)
				debuglog.DebugLog("applyURLToParserConfig: Added default tag_prefix '%s' for subscription: %s", proxySource.TagPrefix, item.Subscription)
			}
			result = append(result, proxySource)
			continue
		}
		if len(item.Connections) > 0 {
			if skipConnectionsIfIn != nil && proxyListHasConnections(skipConnectionsIfIn, item.Connections) {
				continue
			}
			matched := false
			for _, existingConnectionsProxy := range existingProps.ConnectionsProxies {
				if connectionsMatch(existingConnectionsProxy.Connections, item.Connections) {
					matchedProxy := config.ProxySource{
						Connections: item.Connections,
						Outbounds:   existingConnectionsProxy.Outbounds,
						TagPrefix:   existingConnectionsProxy.TagPrefix,
						TagPostfix:  existingConnectionsProxy.TagPostfix,
						TagMask:     existingConnectionsProxy.TagMask,
						Skip:        existingConnectionsProxy.Skip,
					}
					result = append(result, matchedProxy)
					debuglog.DebugLog("applyURLToParserConfig: Matched existing connections proxy, preserved tag_prefix '%s', tag_postfix '%s', tag_mask '%s'",
						matchedProxy.TagPrefix, matchedProxy.TagPostfix, matchedProxy.TagMask)
					matched = true
					break
				}
			}
			if !matched {
				proxySource := config.ProxySource{
					Connections: item.Connections,
					TagPrefix:   GenerateTagPrefix(nextIndex),
				}
				debuglog.DebugLog("applyURLToParserConfig: Adding new ProxySource with %d connections, tag_prefix '%s'", len(item.Connections), proxySource.TagPrefix)
				result = append(result, proxySource)
				// Only in Apply (replace) mode: other connection proxies from old config are not in the new list
				if skipConnectionsIfIn == nil && len(existingProps.ConnectionsProxies) > 0 {
					debuglog.DebugLog("applyURLToParserConfig: Not preserving %d other connection ProxySources (user removed them)", len(existingProps.ConnectionsProxies)-1)
				}
			}
		}
	}
	return result
}

// restoreTagPrefixAndPostfix восстанавливает tag_prefix и tag_postfix из сохраненных свойств.
func restoreTagPrefixAndPostfix(proxySource *config.ProxySource, lookupKey string, existingProps *existingProperties, logContext string) {
	if existingTagPrefix, ok := existingProps.TagPrefixMap[lookupKey]; ok {
		proxySource.TagPrefix = existingTagPrefix
		debuglog.DebugLog("applyURLToParserConfig: Restored tag_prefix '%s' for %s", existingTagPrefix, logContext)
	}
	if existingTagPostfix, ok := existingProps.TagPostfixMap[lookupKey]; ok {
		proxySource.TagPostfix = existingTagPostfix
		debuglog.DebugLog("applyURLToParserConfig: Restored tag_postfix '%s' for %s", existingTagPostfix, logContext)
	}
}

// proxyListHasConnections returns true if proxies contains a proxy with the same connections.
func proxyListHasConnections(proxies []config.ProxySource, connections []string) bool {
	for _, p := range proxies {
		if connectionsMatch(p.Connections, connections) {
			return true
		}
	}
	return false
}

// connectionsMatch проверяет, совпадают ли два массива connections (порядок не важен).
func connectionsMatch(conn1, conn2 []string) bool {
		if len(conn1) != len(conn2) {
			return false
		}
		// Create maps for comparison
		map1 := make(map[string]int)
		map2 := make(map[string]int)
		for _, c := range conn1 {
			map1[strings.TrimSpace(c)]++
		}
		for _, c := range conn2 {
			map2[strings.TrimSpace(c)]++
		}
		if len(map1) != len(map2) {
			return false
		}
		for k, v := range map1 {
			if map2[k] != v {
				return false
			}
		}
		return true
	}

// isConnectionOnlyProxy returns true for proxies that have only Connections (no Source).
func isConnectionOnlyProxy(p config.ProxySource) bool {
	return p.Source == "" && len(p.Connections) > 0
}

// updateAndSerializeParserConfig обновляет ParserConfig и сериализует его.
func updateAndSerializeParserConfig(
	parserConfig *config.ParserConfig,
	newProxies []config.ProxySource,
	subscriptions []string,
	connections []string,
	model *wizardmodels.WizardModel,
	updater UIUpdater,
	timing interface{ LogTiming(string, time.Duration) },
) error {
	// Update proxies array
	parserConfig.ParserConfig.Proxies = newProxies
	debuglog.DebugLog("applyURLToParserConfig: Created %d proxy sources (%d subscriptions, %d with connections)",
		len(newProxies), len(subscriptions), len(connections))

	// Serialize
	serializeStartTime := time.Now()
	serialized, err := SerializeParserConfig(parserConfig)
	if err != nil {
		timing.LogTiming("serialize ParserConfig", time.Since(serializeStartTime))
		debuglog.DebugLog("applyURLToParserConfig: Failed to serialize ParserConfig: %v", err)
		return fmt.Errorf("failed to serialize ParserConfig: %w", err)
	}
	timing.LogTiming("serialize ParserConfig", time.Since(serializeStartTime))
	debuglog.DebugLog("applyURLToParserConfig: Serialized ParserConfig (result length: %d bytes, outbounds before: %d)",
		len(serialized), len(parserConfig.ParserConfig.Outbounds))

	// Update model and UI (both so Save and state use correct data; entry is updated so Add handler's UpdateParserConfig(model.ParserConfigJSON) does not overwrite with stale JSON)
	model.ParserConfigJSON = serialized
	model.ParserConfig = parserConfig
	updater.UpdateParserConfig(serialized)
	model.PreviewNeedsParse = true
	InvalidatePreviewCache(model)
	return nil
}

// SerializeParserConfig serializes ParserConfig to JSON string.
func SerializeParserConfig(parserConfig *config.ParserConfig) (string, error) {
	if parserConfig == nil {
		return "", fmt.Errorf("parserConfig is nil")
	}

	// Normalize ParserConfig (migrate version, set defaults, but don't update last_updated)
	config.NormalizeParserConfig(parserConfig, false)

	// Serialize in version 2 format (version inside ParserConfig, not at top level)
	configToSerialize := map[string]interface{}{
		"ParserConfig": parserConfig.ParserConfig,
	}
	data, err := json.MarshalIndent(configToSerialize, "", IndentBase)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GenerateTagPrefix returns tag prefix by common 1-based index: 1:, 2:, 3:, ...
// Index is shared across all sources (subscriptions then connection-only blocks).
func GenerateTagPrefix(index int) string {
	return fmt.Sprintf("%d:", index)
}
