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
// Интегрирован с GUI через UIUpdater (статусы/preview шаблона, текст кнопки Save; без отдельного прогресс-бара парсинга на вкладке Outbounds).
//
// Реальная логика парсинга находится в:
//   - core/config/parser - парсинг @ParserConfig блоков из файлов
//   - core/config/subscription - парсинг URL подписок и прямых ссылок
//   - core/config - генерация outbounds из ParserConfig
package business

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardutils "singbox-launcher/ui/configurator/utils"
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

	// Save остаётся доступной; при сохранении presenter_save.ensureOutboundsParsed ждёт AutoParseInProgress и при необходимости вызывает ParseAndPreview.

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

	// SPEC 057-R-N: shape parser_config для generator'а.
	//
	//   1. Sync — мутирует parserConfig.Outbounds (adopt legacy globals, drop
	//      stale preset entries, материализует Updates[] стеки на target'ах).
	//      Это safely persistable shape — Updates[] стек сохраняется в state.
	//   2. Merge — flatten Updates[] стек в финальное body. **Это
	//      destructive** для state shape (теряется Updates[] стек).
	//
	// Generator знает только base body — поэтому Merge нужен для нужно ему.
	// НО результат Merge **не должен** попасть обратно в model.ParserConfig
	// (иначе Save запишет merged body без updates[] стека, и при следующем
	// Sync preset patches применятся вторично — двойной merge).
	//
	// Решение: после Sync копируем parserConfig в parserConfigForGen,
	// Merge только на копии, generator работает с копией, в model.ParserConfig
	// уходит несмерженная версия с Updates[] стеком intact.
	parserConfigForGen := parserConfig
	if model.TemplateData != nil {
		wizardmodels.ReconcileRuleOrder(model)
		rulesV6 := wizardmodels.SyncRulesByOrderToStateRulesV6(
			model.RuleOrder, model.PresetRefs, model.CustomRules,
		)
		build.SyncOutboundsWithActivePresets(rulesV6, &parserConfig.ParserConfig.Outbounds, model.TemplateData.Presets)
		// Deep-copy outbounds slice для generator-only Merge.
		// Per-element copy чтобы Updates[] стек не shared.
		genOutbounds := make([]config.OutboundConfig, len(parserConfig.ParserConfig.Outbounds))
		for i, ob := range parserConfig.ParserConfig.Outbounds {
			genOutbounds[i] = ob
			if len(ob.Updates) > 0 {
				genOutbounds[i].Updates = append([]config.OutboundUpdate(nil), ob.Updates...)
			}
		}
		parserConfigForGen.ParserConfig.Outbounds = genOutbounds
		build.MergeOutboundUpdatesInPlace(&parserConfigForGen)
	}

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
		// Колбэк прогресса от генератора не выводится отдельным UI на вкладке Outbounds (только throttling по времени).
		_ = s
	}

	result, err := configService.GenerateOutboundsFromParserConfig(
		&parserConfigForGen, tagCounts, progressCallback)
	if err != nil {
		timing.LogTiming("generate outbounds", time.Since(generateStartTime))
		debuglog.DebugLog("parseAndPreview: Failed to generate outbounds: %v", err)
		updater.UpdateSaveButtonText("Save")
		return fmt.Errorf("failed to generate outbounds: %w", err)
	}

	// Риск: пока шла генерация, пользователь мог изменить ParserConfig (OnChanged → MergeGUIToModel).
	// Запись outbounds от старого снимка при новом JSON даёт несогласованный config при Save
	// (ensureOutboundsParsed увидит непустые outbounds и не перепарсит).
	if strings.TrimSpace(model.ParserConfigJSON) != parserConfigJSON {
		debuglog.InfoLog("parseAndPreview: ParserConfigJSON changed during generation, discarding outbound results")
		model.GeneratedOutbounds = nil
		model.GeneratedEndpoints = nil
		model.PreviewNeedsParse = true
		updater.UpdateSaveButtonText("Save")
		return nil
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
	data, err := json.MarshalIndent(configToSerialize, "", build.IndentBase)
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

// tagPrefixFromSubscriptionFragment returns a tag_prefix derived from the URL fragment (part after #),
// e.g. https://host/sub.json#abvpn → "abvpn:". Empty string if there is no usable fragment.
func tagPrefixFromSubscriptionFragment(raw string) string {
	raw = strings.TrimSpace(raw)
	if !subscription.IsSubscriptionURL(raw) {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	frag := strings.TrimSpace(u.Fragment)
	if frag == "" {
		return ""
	}
	if dec, err := url.PathUnescape(frag); err == nil {
		frag = strings.TrimSpace(dec)
	}
	frag = sanitizeTagPrefixFromURLFragment(frag)
	if frag == "" {
		return ""
	}
	if !strings.HasSuffix(frag, ":") {
		frag += ":"
	}
	return frag
}

const maxURLFragmentTagPrefixRunes = 120

// sanitizeTagPrefixFromURLFragment strips control characters and limits length for a safe tag_prefix.
func sanitizeTagPrefixFromURLFragment(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(utf8.RuneCountInString(s))
	n := 0
	for _, r := range s {
		if n >= maxURLFragmentTagPrefixRunes {
			break
		}
		if r == '\t' || r == '\n' || r == '\r' {
			r = ' '
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}
