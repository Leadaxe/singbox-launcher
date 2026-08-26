package business

import (
	"fmt"
	"sync"
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
// previewRebuildMu сериализует пересборки кэша: два фоновых захода (вкладка
// Sources и окно цепочки) иначе разбирали бы все подписки параллельно и
// вперемешку писали одни и те же поля модели.
var previewRebuildMu sync.Mutex

func RebuildPreviewCache(model *wizardmodels.WizardModel) (int, error) {
	previewRebuildMu.Lock()
	defer previewRebuildMu.Unlock()
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
		model.PreviewIgnoredSectionsBySource = nil
		return 0, nil
	}

	tagCounts := make(map[string]int)
	nodesBySource := make(map[int][]*config.ParsedNode, totalSources)
	allNodes := make([]*config.ParsedNode, 0)
	ignoredBySource := make(map[int][]string)
	errorCount := 0

	loadTimingStart := time.Now()
	debuglog.DebugLog("wizardPreviewCache: starting LoadNodesFromSource for %d sources", totalSources)

	for i, ps := range proxies {
		// Skip disabled sources — UI preview должен совпадать с build pipeline
		// (GenerateOutboundsFromParserConfig тоже скипает disabled). Без этого
		// юзер видит outbound'ы от выключенных подписок в Outbounds tab.
		if ps.Disabled {
			continue
		}
		res, err := subscription.LoadNodesFromSourceEx(ps, tagCounts, nil, i, totalSources)
		if err != nil {
			errorCount++
			debuglog.DebugLog("wizardPreviewCache: LoadNodesFromSource error for source %d/%d: %v", i+1, totalSources, err)
			continue
		}
		if res == nil {
			continue
		}
		nodes := res.Nodes

		// SPEC 112: парсер переписал legacy-ключи отметок выключения на
		// тег-идентичность — сохраняем результат в canonical Sources.
		// В самом парсере сохранять некуда: он не видит ни состояния, ни
		// файла, и без этого хеши мигрировали бы заново на каждом запуске.
		if res.DisabledMigrated {
			applyMigratedDisabledKeys(model, i, res.DisabledNodes)
		}

		// SPEC 094 A4: подписка отдала целый sing-box конфиг — показываем, какие
		// его секции импорт не читает, чтобы «проглочено молча» не выглядело
		// как потеря данных.
		if len(res.IgnoredSections) > 0 {
			ignoredBySource[i] = res.IgnoredSections
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

	// SPEC 110: источники-цепочки становятся узлами ровно тем же вызовом,
	// что и на сборке, — иначе превью показывало бы не тот пул, из которого
	// собирается конфиг.
	//
	// Это и был баг #91 «regex Направления не подхватывает цепочки»: в
	// config.json цепочка в состав входила (ResolveChainSources зовётся из
	// GenerateOutboundsFromParserConfig), а здесь её не было вовсе, и
	// пользователь, глядя на превью и на flag picker, делал вывод, что
	// фильтр цепочки не берёт. Врало превью, а не отбор.
	//
	// Деградировавшие цепочки (ядро без with_lx_chain, недошедшая позиция)
	// в пул не попадают — как и в конфиге; их причины показывает сборка.
	chainPool, broken := config.ResolveChainSources(
		model.ParserConfig, allNodes, nodesBySource, previewDirectionTags(model))
	for _, b := range broken {
		debuglog.DebugLog("wizardPreviewCache: цепочка %q не стала узлом: %s", b.Tag, b.Reason)
	}
	allNodes = chainPool

	model.PreviewNodes = allNodes
	if len(nodesBySource) > 0 {
		model.PreviewNodesBySource = nodesBySource
	} else {
		model.PreviewNodesBySource = nil
	}
	if len(ignoredBySource) > 0 {
		model.PreviewIgnoredSectionsBySource = ignoredBySource
	} else {
		model.PreviewIgnoredSectionsBySource = nil
	}

	return errorCount, nil
}

// previewDirectionTags — теги включённых Направлений: позиция цепочки
// вправе ссылаться на Направление, и без этого списка такая цепочка
// деградировала бы в превью с причиной «позиция не найдена», хотя в
// конфиге собирается (там тот же список строит генератор).
func previewDirectionTags(model *wizardmodels.WizardModel) map[string]bool {
	if model == nil || model.ParserConfig == nil {
		return nil
	}
	dirs := model.ParserConfig.ParserConfig.Outbounds
	tags := make(map[string]bool, len(dirs))
	for _, d := range dirs {
		if d.Tag != "" && !d.Disabled {
			tags[d.Tag] = true
		}
	}
	return tags
}

// InvalidatePreviewCache clears the preview cache so that the next consumer (Sources Refresh, View, Edit Outbound Preview) will rebuild via RebuildPreviewCache.
// Call this whenever ParserConfig or sources change (Add/Del source, prefix change, configurator apply, manual JSON apply).
func InvalidatePreviewCache(model *wizardmodels.WizardModel) {
	if model == nil {
		return
	}
	model.PreviewCacheGeneration++
	model.PreviewNodes = nil
	model.PreviewNodesBySource = nil
	model.PreviewIgnoredSectionsBySource = nil
	model.AvailableOutboundsMemoKey = ""
	model.AvailableOutboundsMemoTags = nil
	// Счётчики узлов выведены из этого кэша и пережить его не могут:
	// иначе список Sources показывал бы числа от прошлого состава.
	model.SourceNodeCounts = nil
}

// applyMigratedDisabledKeys кладёт переписанные парсером отметки выключения
// обратно в canonical Source (SPEC 112).
//
// proxyIndex — индекс в ParserConfig.ParserConfig.Proxies; canonical Sources
// идут тем же порядком (RefreshDerivedParserConfig строит derived-view из них
// один к одному), поэтому индекс общий.
//
// Времена lastSeen из парса сюда НЕ переносятся отдельно: карта приезжает
// целиком, и продление меток — штатная часть того же прогона.
func applyMigratedDisabledKeys(model *wizardmodels.WizardModel, proxyIndex int, migrated map[string]int64) {
	if model == nil || proxyIndex < 0 || proxyIndex >= len(model.Sources) {
		return
	}
	if len(migrated) == 0 {
		model.Sources[proxyIndex].DisabledNodes = nil
	} else {
		model.Sources[proxyIndex].DisabledNodes = migrated
	}
	debuglog.DebugLog("wizardPreviewCache: источник %d — отметки выключения переведены на тег-идентичность (SPEC 112)", proxyIndex+1)
}
