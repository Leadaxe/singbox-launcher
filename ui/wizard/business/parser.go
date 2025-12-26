package business

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
	wizardstate "singbox-launcher/ui/wizard/state"
	wizardutils "singbox-launcher/ui/wizard/utils"
)

// CheckURL validates subscription URLs or direct links and updates the wizard state.
// It checks availability of subscription URLs and validates direct links.
func CheckURL(state *wizardstate.WizardState) {
	startTime := time.Now()
	wizardstate.DebugLog("checkURL: START at %s", startTime.Format("15:04:05.000"))

	input := strings.TrimSpace(state.SourceURLEntry.Text)
	if input == "" {
		wizardstate.DebugLog("checkURL: Empty input, returning early")
		wizardstate.SafeFyneDo(state.Window, func() {
			state.URLStatusLabel.SetText("❌ Please enter a URL or direct link")
			state.SetCheckURLState("", "Check", -1)
		})
		return
	}

	state.CheckURLInProgress = true
	wizardstate.SafeFyneDo(state.Window, func() {
		state.URLStatusLabel.SetText("⏳ Checking...")
		state.SetCheckURLState("", "", 0.0)
	})

	// Split input into lines for processing
	inputLines := strings.Split(input, "\n")
	wizardstate.DebugLog("checkURL: Processing %d input lines", len(inputLines))
	totalValid := 0
	previewLines := make([]string, 0)
	errors := make([]string, 0)

	for i, line := range inputLines {
		lineStartTime := time.Now()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		wizardstate.DebugLog("checkURL: Processing line %d/%d: %s (elapsed: %v)", i+1, len(inputLines),
			func() string {
				if len(line) > 50 {
					return line[:50] + "..."
				}
				return line
			}(), time.Since(startTime))

		wizardstate.SafeFyneDo(state.Window, func() {
			progress := float64(i+1) / float64(len(inputLines))
			state.SetCheckURLState(fmt.Sprintf("⏳ Checking... (%d/%d)", i+1, len(inputLines)), "", progress)
		})

		if subscription.IsSubscriptionURL(line) {
			// Validate URL before fetching
			if err := ValidateURL(line); err != nil {
				wizardstate.DebugLog("checkURL: Invalid subscription URL %d/%d: %v", i+1, len(inputLines), err)
				errors = append(errors, fmt.Sprintf("Invalid subscription URL: %v", err))
				continue
			}

			// This is a subscription URL - check availability
			fetchStartTime := time.Now()
			wizardstate.DebugLog("checkURL: Fetching subscription %d/%d: %s", i+1, len(inputLines), line)
			content, err := subscription.FetchSubscription(line)
			fetchDuration := time.Since(fetchStartTime)
			if err != nil {
				wizardstate.DebugLog("checkURL: Failed to fetch subscription %d/%d (took %v): %v", i+1, len(inputLines), fetchDuration, err)
				errors = append(errors, fmt.Sprintf("Failed to fetch %s: %v", line, err))
				continue
			}

			// Validate response size
			if err := ValidateHTTPResponseSize(int64(len(content))); err != nil {
				wizardstate.DebugLog("checkURL: Subscription response too large %d/%d: %v", i+1, len(inputLines), err)
				errors = append(errors, fmt.Sprintf("Subscription response too large: %v", err))
				continue
			}

			wizardstate.DebugLog("checkURL: Fetched subscription %d/%d: %d bytes in %v", i+1, len(inputLines), len(content), fetchDuration)

			// Check subscription content
			parseStartTime := time.Now()
			subLines := strings.Split(string(content), "\n")
			wizardstate.DebugLog("checkURL: Parsing subscription %d/%d: %d lines", i+1, len(inputLines), len(subLines))
			validInSub := 0
			for _, subLine := range subLines {
				subLine = strings.TrimSpace(subLine)
				if subLine != "" && subscription.IsDirectLink(subLine) {
					validInSub++
					totalValid++
					if len(previewLines) < wizardutils.MaxPreviewLines {
						previewLines = append(previewLines, fmt.Sprintf("%d. %s", totalValid, subLine))
					}
				}
			}
			parseDuration := time.Since(parseStartTime)
			wizardstate.DebugLog("checkURL: Parsed subscription %d/%d: %d valid links in %v (line processing took %v total)",
				i+1, len(inputLines), validInSub, parseDuration, time.Since(lineStartTime))
			if validInSub == 0 {
				errors = append(errors, fmt.Sprintf("Subscription %s contains no valid proxy links", line))
			}
		} else if subscription.IsDirectLink(line) {
			// Validate URI before parsing
			if err := ValidateURI(line); err != nil {
				wizardstate.DebugLog("checkURL: Invalid URI format %d/%d: %v", i+1, len(inputLines), err)
				errors = append(errors, fmt.Sprintf("Invalid URI format: %v", err))
				continue
			}

			// This is a direct link - validate parsing
			parseStartTime := time.Now()
			wizardstate.DebugLog("checkURL: Parsing direct link %d/%d", i+1, len(inputLines))
			_, err := subscription.ParseNode(line, nil)
			parseDuration := time.Since(parseStartTime)
			if err != nil {
				wizardstate.DebugLog("checkURL: Invalid direct link %d/%d (took %v): %v", i+1, len(inputLines), parseDuration, err)
				errors = append(errors, fmt.Sprintf("Invalid direct link: %v", err))
			} else {
				totalValid++
				wizardstate.DebugLog("checkURL: Valid direct link %d/%d (took %v)", i+1, len(inputLines), parseDuration)
				if len(previewLines) < wizardutils.MaxPreviewLines {
					previewLines = append(previewLines, fmt.Sprintf("%d. %s", totalValid, line))
				}
			}
		} else {
			wizardstate.DebugLog("checkURL: Unknown format for line %d/%d: %s", i+1, len(inputLines), line)
			errors = append(errors, fmt.Sprintf("Unknown format: %s", line))
		}
	}

	state.CheckURLInProgress = false
	totalDuration := time.Since(startTime)
	wizardstate.DebugLog("checkURL: Processed all lines in %v (total valid: %d, errors: %d)",
		totalDuration, totalValid, len(errors))

	wizardstate.SafeFyneDo(state.Window, func() {
		if totalValid == 0 {
			errorMsg := "❌ No valid proxy links found"
			if len(errors) > 0 {
				errorMsg += "\n" + strings.Join(errors[:min(3, len(errors))], "\n")
			}
			state.URLStatusLabel.SetText(errorMsg)
		} else {
			statusMsg := fmt.Sprintf("✅ Working! Found %d valid proxy link(s)", totalValid)
			if len(errors) > 0 {
				statusMsg += fmt.Sprintf("\n⚠️ %d error(s)", len(errors))
			}
			state.URLStatusLabel.SetText(statusMsg)
			if len(previewLines) > 0 {
				previewText := strings.Join(previewLines, "\n")
				if totalValid > len(previewLines) {
					previewText += fmt.Sprintf("\n... and %d more", totalValid-len(previewLines))
				}
				SetPreviewText(state, previewText)
			}
		}
		state.SetCheckURLState("", "Check", -1)
	})
	wizardstate.DebugLog("checkURL: END (total duration: %v)", totalDuration)
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ParseAndPreview parses ParserConfig and generates outbounds preview.
// It updates the wizard state with parsed configuration and generated outbounds.
func ParseAndPreview(state *wizardstate.WizardState) {
	startTime := time.Now()
	wizardstate.DebugLog("parseAndPreview: START at %s", startTime.Format("15:04:05.000"))

	defer func() {
		totalDuration := time.Since(startTime)
		wizardstate.DebugLog("parseAndPreview: END (total duration: %v)", totalDuration)
		wizardstate.SafeFyneDo(state.Window, func() {
			state.AutoParseInProgress = false
		})
	}()
	wizardstate.SafeFyneDo(state.Window, func() {
		state.ParseButton.Disable()
		state.ParseButton.SetText("Parsing...")
		SetPreviewText(state, "Parsing configuration...")
	})

	// Parse ParserConfig from field
	parseStartTime := time.Now()
	parserConfigJSON := strings.TrimSpace(state.ParserConfigEntry.Text)
	wizardstate.DebugLog("parseAndPreview: ParserConfig text length: %d bytes", len(parserConfigJSON))
	if parserConfigJSON == "" {
		wizardstate.DebugLog("parseAndPreview: ParserConfig is empty, returning early")
		wizardstate.SafeFyneDo(state.Window, func() {
			SetPreviewText(state, "Error: ParserConfig is empty")
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}

	// Validate JSON size before parsing
	if err := ValidateJSONSize([]byte(parserConfigJSON)); err != nil {
		wizardstate.DebugLog("parseAndPreview: ParserConfig JSON size validation failed: %v", err)
		wizardstate.SafeFyneDo(state.Window, func() {
			SetPreviewText(state, fmt.Sprintf("Error: %v", err))
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}

	var parserConfig config.ParserConfig
	if err := json.Unmarshal([]byte(parserConfigJSON), &parserConfig); err != nil {
		wizardstate.DebugLog("parseAndPreview: Failed to parse ParserConfig JSON (took %v): %v", time.Since(parseStartTime), err)
		wizardstate.SafeFyneDo(state.Window, func() {
			SetPreviewText(state, fmt.Sprintf("Error: Failed to parse ParserConfig JSON: %v", err))
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}

	// Validate ParserConfig structure
	if err := ValidateParserConfig(&parserConfig); err != nil {
		wizardstate.DebugLog("parseAndPreview: ParserConfig validation failed: %v", err)
		wizardstate.SafeFyneDo(state.Window, func() {
			SetPreviewText(state, fmt.Sprintf("Error: Invalid ParserConfig: %v", err))
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}
	wizardstate.DebugLog("parseAndPreview: Parsed ParserConfig in %v (sources: %d, outbounds: %d)",
		time.Since(parseStartTime), len(parserConfig.ParserConfig.Proxies), len(parserConfig.ParserConfig.Outbounds))

	// Check for URL or direct links
	url := strings.TrimSpace(state.SourceURLEntry.Text)
	wizardstate.DebugLog("parseAndPreview: URL text length: %d bytes", len(url))
	if url == "" {
		wizardstate.DebugLog("parseAndPreview: URL is empty, returning early")
		wizardstate.SafeFyneDo(state.Window, func() {
			SetPreviewText(state, "Error: VLESS URL or direct links are empty")
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}

	// Update config through ApplyURLToParserConfig, which correctly separates subscriptions and connections
	applyStartTime := time.Now()
	wizardstate.DebugLog("parseAndPreview: Applying URL to ParserConfig")
	ApplyURLToParserConfig(state, url)
	wizardstate.DebugLog("parseAndPreview: Applied URL to ParserConfig in %v", time.Since(applyStartTime))

	// Reload parserConfig after update
	reloadStartTime := time.Now()
	parserConfigJSON = strings.TrimSpace(state.ParserConfigEntry.Text)
	if parserConfigJSON != "" {
		if err := json.Unmarshal([]byte(parserConfigJSON), &parserConfig); err != nil {
			wizardstate.DebugLog("parseAndPreview: Failed to parse updated ParserConfig JSON (took %v): %v", time.Since(reloadStartTime), err)
			wizardstate.SafeFyneDo(state.Window, func() {
				SetPreviewText(state, fmt.Sprintf("Error: Failed to parse updated ParserConfig JSON: %v", err))
				state.ParseButton.Enable()
				state.ParseButton.SetText("Parse")
				if state.SaveButton != nil {
					state.SaveButton.Enable()
				}
			})
			return
		}
		wizardstate.DebugLog("parseAndPreview: Reloaded ParserConfig in %v (sources: %d)",
			time.Since(reloadStartTime), len(parserConfig.ParserConfig.Proxies))
	}

	// Generate all outbounds using unified function
	// This eliminates code duplication and adds support for local outbounds
	generateStartTime := time.Now()
	wizardstate.DebugLog("parseAndPreview: Starting outbound generation using unified function")

	// Map to track unique tags and their counts
	tagCounts := make(map[string]int)
	wizardstate.DebugLog("parseAndPreview: Initializing tag deduplication tracker")

	// Progress callback for UI updates with throttling (not more than once per 200ms)
	var lastProgressUpdate time.Time
	progressCallback := func(p float64, s string) {
		now := time.Now()
		if now.Sub(lastProgressUpdate) < wizardutils.ProgressUpdateInterval {
			return // Skip update if less than ProgressUpdateInterval passed
		}
		lastProgressUpdate = now
		wizardstate.SafeFyneDo(state.Window, func() {
			SetPreviewText(state, s)
		})
	}

	// Use unified function to generate all outbounds
	result, err := state.Controller.ConfigService.GenerateOutboundsFromParserConfig(
		&parserConfig, tagCounts, progressCallback)
	if err != nil {
		wizardstate.DebugLog("parseAndPreview: Failed to generate outbounds (took %v): %v", time.Since(generateStartTime), err)
		wizardstate.SafeFyneDo(state.Window, func() {
			SetPreviewText(state, fmt.Sprintf("Error: Failed to generate outbounds: %v", err))
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}

	// Log statistics about duplicates
	subscription.LogDuplicateTagStatistics(tagCounts, "ConfigWizard")

	// Save statistics for use in buildParserOutboundsBlock
	state.OutboundStats.NodesCount = result.NodesCount
	state.OutboundStats.LocalSelectorsCount = result.LocalSelectorsCount
	state.OutboundStats.GlobalSelectorsCount = result.GlobalSelectorsCount
	state.GeneratedOutbounds = result.OutboundsJSON // Store full JSON for later use (e.g., saving)

	// Form final preview text
	// If nodes count exceeds MaxNodesForFullPreview, show statistics as comment
	var previewText string
	if result.NodesCount > wizardutils.MaxNodesForFullPreview {
		// Form statistics as comment
		joinStartTime := time.Now()
		statsComment := fmt.Sprintf(`/** @ParserSTART */
	// Generated: %d nodes, %d local selectors, %d global selectors
	// Total outbounds: %d
/** @ParserEND */`,
			result.NodesCount,
			result.LocalSelectorsCount,
			result.GlobalSelectorsCount,
			len(result.OutboundsJSON))
		previewText = statsComment
		wizardstate.DebugLog("parseAndPreview: Generated statistics comment in %v (nodes: %d > %d)", time.Since(joinStartTime), result.NodesCount, wizardutils.MaxNodesForFullPreview)
	} else {
		// Show full text for small number of nodes
		joinStartTime := time.Now()
		previewText = strings.Join(result.OutboundsJSON, "\n")
		wizardstate.DebugLog("parseAndPreview: Joined %d JSON strings in %v (total preview text length: %d bytes)",
			len(result.OutboundsJSON), time.Since(joinStartTime), len(previewText))
	}
	wizardstate.DebugLog("parseAndPreview: Total outbound generation took %v", time.Since(generateStartTime))

	wizardstate.SafeFyneDo(state.Window, func() {
		uiUpdateStartTime := time.Now()
		SetPreviewText(state, previewText)
		state.ParseButton.Enable()
		state.ParseButton.SetText("Parse")
		state.ParserConfig = &parserConfig
		state.PreviewNeedsParse = false
		state.RefreshOutboundOptions()
		wizardstate.DebugLog("parseAndPreview: UI update took %v", time.Since(uiUpdateStartTime))
		// Enable Save button after successful parsing (regardless of preview)
		if state.SaveButton != nil {
			state.SaveButton.Enable()
		}
		// Automatically generate preview on 3rd tab after successful parsing
		if state.TemplateData != nil && len(state.GeneratedOutbounds) > 0 {
			state.TemplatePreviewNeedsUpdate = true
			go UpdateTemplatePreviewAsync(state)
		}
	})
}

// SetPreviewText sets the preview text in the wizard state.
func SetPreviewText(state *wizardstate.WizardState, text string) {
	state.OutboundsPreviewText = text
	if state.OutboundsPreview != nil {
		// Safe SetText call - function is already called from SafeFyneDo in most cases,
		// but wrap in SafeFyneDo for safety
		wizardstate.SafeFyneDo(state.Window, func() {
			state.OutboundsPreview.SetText(text)
		})
	}
}

// ApplyURLToParserConfig applies URL input to ParserConfig, correctly separating subscriptions and connections.
// It preserves existing local outbounds, tag_prefix, and tag_postfix for each source.
func ApplyURLToParserConfig(state *wizardstate.WizardState, input string) {
	startTime := time.Now()
	wizardstate.DebugLog("applyURLToParserConfig: START at %s (input length: %d bytes)",
		startTime.Format("15:04:05.000"), len(input))

	if state.ParserConfigEntry == nil || input == "" {
		wizardstate.DebugLog("applyURLToParserConfig: ParserConfigEntry is nil or input is empty, returning early")
		return
	}
	text := strings.TrimSpace(state.ParserConfigEntry.Text)
	if text == "" {
		wizardstate.DebugLog("applyURLToParserConfig: ParserConfigEntry text is empty, returning early")
		return
	}

	parseStartTime := time.Now()
	var parserConfig config.ParserConfig
	if err := json.Unmarshal([]byte(text), &parserConfig); err != nil {
		wizardstate.DebugLog("applyURLToParserConfig: Failed to parse ParserConfig (took %v): %v",
			time.Since(parseStartTime), err)
		return
	}
	wizardstate.DebugLog("applyURLToParserConfig: Parsed ParserConfig in %v (outbounds: %d)",
		time.Since(parseStartTime), len(parserConfig.ParserConfig.Outbounds))

	// Separate subscriptions and direct links
	splitStartTime := time.Now()
	lines := strings.Split(input, "\n")
	wizardstate.DebugLog("applyURLToParserConfig: Split input into %d lines", len(lines))
	subscriptions := make([]string, 0)
	connections := make([]string, 0)

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
	wizardstate.DebugLog("applyURLToParserConfig: Classified lines: %d subscriptions, %d connections (took %v)",
		len(subscriptions), len(connections), time.Since(splitStartTime))

	// Preserve existing local outbounds, tag_prefix, and tag_postfix for each source
	// Use source URL as key for matching
	existingOutboundsMap := make(map[string][]config.OutboundConfig)
	existingTagPrefixMap := make(map[string]string)
	existingTagPostfixMap := make(map[string]string)
	for i, existingProxy := range parserConfig.ParserConfig.Proxies {
		if existingProxy.Source != "" {
			existingOutboundsMap[existingProxy.Source] = existingProxy.Outbounds
			if existingProxy.TagPrefix != "" {
				existingTagPrefixMap[existingProxy.Source] = existingProxy.TagPrefix
			}
			if existingProxy.TagPostfix != "" {
				existingTagPostfixMap[existingProxy.Source] = existingProxy.TagPostfix
			}
		} else if i == 0 && len(existingProxy.Outbounds) > 0 {
			// If first proxy had no source but had outbounds, preserve them
			// This can be a case when there was only connections without source
			existingOutboundsMap[""] = existingProxy.Outbounds
			if existingProxy.TagPrefix != "" {
				existingTagPrefixMap[""] = existingProxy.TagPrefix
			}
			if existingProxy.TagPostfix != "" {
				existingTagPostfixMap[""] = existingProxy.TagPostfix
			}
		}
	}

	// Create new ProxySource array
	newProxies := make([]config.ProxySource, 0)

	// Automatically add tag_prefix with sequential number only if there are multiple subscriptions
	autoAddPrefix := len(subscriptions) > 1

	// Helper function to restore tag_prefix and tag_postfix
	restoreTagPrefixAndPostfix := func(proxySource *config.ProxySource, lookupKey string, logContext string) {
		if existingTagPrefix, ok := existingTagPrefixMap[lookupKey]; ok {
			proxySource.TagPrefix = existingTagPrefix
			wizardstate.DebugLog("applyURLToParserConfig: Restored tag_prefix '%s' for %s", existingTagPrefix, logContext)
		}
		if existingTagPostfix, ok := existingTagPostfixMap[lookupKey]; ok {
			proxySource.TagPostfix = existingTagPostfix
			wizardstate.DebugLog("applyURLToParserConfig: Restored tag_postfix '%s' for %s", existingTagPostfix, logContext)
		}
	}

	// Create separate ProxySource for each subscription
	for idx, sub := range subscriptions {
		proxySource := config.ProxySource{
			Source: sub,
		}
		// Restore local outbounds if they existed for this source
		if existingOutbounds, ok := existingOutboundsMap[sub]; ok {
			proxySource.Outbounds = existingOutbounds
			wizardstate.DebugLog("applyURLToParserConfig: Restored %d local outbounds for subscription: %s", len(existingOutbounds), sub)
		}
		// Restore tag_prefix and tag_postfix
		restoreTagPrefixAndPostfix(&proxySource, sub, fmt.Sprintf("subscription: %s", sub))
		// Automatically add tag_prefix if not restored and auto-add is enabled
		if proxySource.TagPrefix == "" && autoAddPrefix {
			proxySource.TagPrefix = GenerateTagPrefix(idx + 1)
			wizardstate.DebugLog("applyURLToParserConfig: Added automatic tag_prefix '%s' for subscription: %s", proxySource.TagPrefix, sub)
		}
		newProxies = append(newProxies, proxySource)
	}

	// If there are direct links, create separate ProxySource for them
	if len(connections) > 0 {
		proxySource := config.ProxySource{
			Connections: connections,
		}
		// If first proxy had no source but had outbounds, restore them
		if existingOutbounds, ok := existingOutboundsMap[""]; ok && len(newProxies) == 0 {
			// If there were no subscriptions but there were connections with outbounds
			proxySource.Outbounds = existingOutbounds
			wizardstate.DebugLog("applyURLToParserConfig: Restored %d local outbounds for connections", len(existingOutbounds))
		}
		// Restore tag_prefix and tag_postfix
		restoreTagPrefixAndPostfix(&proxySource, "", "connections")
		newProxies = append(newProxies, proxySource)
	}

	// If there are no subscriptions or connections, create empty array
	if len(newProxies) == 0 {
		newProxies = []config.ProxySource{{}}
	}

	// Update proxies array
	parserConfig.ParserConfig.Proxies = newProxies
	wizardstate.DebugLog("applyURLToParserConfig: Created %d proxy sources (%d subscriptions, %d with connections)",
		len(newProxies), len(subscriptions), len(connections))

	serializeStartTime := time.Now()
	serialized, err := SerializeParserConfig(&parserConfig)
	if err != nil {
		wizardstate.DebugLog("applyURLToParserConfig: Failed to serialize ParserConfig (took %v): %v",
			time.Since(serializeStartTime), err)
		return
	}
	wizardstate.DebugLog("applyURLToParserConfig: Serialized ParserConfig in %v (result length: %d bytes, outbounds before: %d)",
		time.Since(serializeStartTime), len(serialized), len(parserConfig.ParserConfig.Outbounds))

	// Update UI safely from any thread
	wizardstate.SafeFyneDo(state.Window, func() {
		state.ParserConfigUpdating = true
		state.ParserConfigEntry.SetText(serialized)
		state.ParserConfigUpdating = false
	})
	state.ParserConfig = &parserConfig
	state.PreviewNeedsParse = true
	wizardstate.DebugLog("applyURLToParserConfig: END (total duration: %v)", time.Since(startTime))
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
	data, err := json.MarshalIndent(configToSerialize, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GenerateTagPrefix generates a tag prefix for a subscription based on its index.
// Format: "1:", "2:", "3:", etc.
// This function can be easily modified to change the prefix format.
func GenerateTagPrefix(index int) string {
	return fmt.Sprintf("%d:", index)
}
