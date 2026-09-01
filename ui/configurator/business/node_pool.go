// File node_pool.go — пул узлов модели: то, что увидит сборка.
//
// SPEC 118 W5: прежний «кэш превью» умер вместе с ленивым разбором подписок.
// Тела разобраны ОДИН раз — при fetch или миграции — и лежат в состоянии
// (`sources[].nodes[]`). Здесь остаётся ровно то же, что делает сборка:
// эмиссия узлов из тел (EmitCanonicalSource) и сборка цепочек
// (ResolveChainSources). Ни сети, ни парсеров подписок.
//
// Кэш при этом остаётся: эмиссия сотен узлов не бесплатна, а строка списка
// перерисовывается на каждое движение мыши. Но три состояния прежнего
// ленивого кэша («не готов / готов и пуст / есть») схлопнулись в два: данные
// в модели есть всегда, а пул — их производная.
package business

import (
	"fmt"
	"sort"
	"sync"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// nodePoolMu сериализует пересборки пула: два фоновых захода (вкладка
// Sources и окно цепочки) иначе вперемешку писали бы одни и те же поля
// модели.
var nodePoolMu sync.Mutex

// RebuildNodePool пересобирает пул узлов модели.
//
// Заполняет:
//   - model.NodePool — все узлы всех источников с ФИНАЛЬНЫМИ тегами;
//   - model.NodePoolBySource — те же узлы по индексу источника.
//
// Возвращает число источников, из которых не родилось ни одного узла
// (errorCount — для строки статуса), и ошибку только на фатальных сбоях.
func RebuildNodePool(model *wizardmodels.WizardModel) (int, error) {
	nodePoolMu.Lock()
	defer nodePoolMu.Unlock()
	timing := debuglog.StartTiming("wizardNodePool")
	defer timing.EndWithDefer()

	if model == nil {
		return 0, fmt.Errorf("wizard model is nil")
	}

	// SPEC 117: одноразовая проекция canonical → сборочная форма. Строится
	// локально на входе и выбрасывается — модель проекцию не хранит.
	pc := model.AsParserConfig()

	proxies := pc.ParserConfig.Proxies
	totalSources := len(proxies)
	if totalSources == 0 {
		model.NodePool = nil
		model.NodePoolBySource = nil
		return 0, nil
	}

	tagCounts := make(map[string]int)
	nodesBySource := make(map[int][]*config.ParsedNode, totalSources)
	allNodes := make([]*config.ParsedNode, 0)
	errorCount := 0

	for i := range proxies {
		ps := proxies[i]
		// Выключенные источники пропускаются — пул обязан совпадать со
		// сборкой (GenerateOutboundsFromParserConfig делает то же), иначе на
		// вкладке Outbounds появлялись бы узлы выключенных подписок.
		if ps.Disabled {
			continue
		}
		emitted := config.EmitCanonicalSource(ps, i, tagCounts)
		for _, w := range emitted.Warnings {
			debuglog.DebugLog("wizardNodePool: источник %d: %s", i+1, w)
		}
		nodes := emitted.Nodes
		if len(nodes) == 0 {
			errorCount++
			continue
		}
		for _, n := range nodes {
			n.SourceIndex = i
		}
		nodesBySource[i] = nodes
		allNodes = append(allNodes, nodes...)
	}

	debuglog.DebugLog("wizardNodePool: собрано %d узлов из %d источников (пустых: %d)", len(allNodes), totalSources, errorCount)

	// SPEC 110: источники-цепочки становятся узлами ровно тем же вызовом,
	// что и на сборке, — иначе пул показывал бы не тот состав, из которого
	// собирается конфиг (баг #91: фильтр Направления «не брал» цепочку).
	//
	// Позиции цепочек резолвятся тем же единым резолвом NodeLink, что и на
	// сборке: без него хоп на узел папки не превратился бы в финальный тег.
	linkTargets := config.BuildNodeLinkTargets(pc.ParserConfig.Proxies, nodesBySource, nodePoolRootTargets(model))
	config.ResolveCanonicalChainHops(pc, linkTargets)
	chainPool, broken := config.ResolveChainSources(
		pc, allNodes, nodesBySource, nodePoolDirectionTags(model))
	for _, b := range broken {
		debuglog.DebugLog("wizardNodePool: цепочка %q не стала узлом: %s", b.Tag, b.Reason)
	}
	allNodes = chainPool

	model.NodePool = allNodes
	if len(nodesBySource) > 0 {
		model.NodePoolBySource = nodesBySource
	} else {
		model.NodePoolBySource = nil
	}

	return errorCount, nil
}

// nodePoolDirectionTags — теги включённых Направлений: позиция цепочки
// вправе ссылаться на Направление, и без этого списка такая цепочка
// деградировала бы в пуле с причиной «позиция не найдена», хотя в конфиге
// собирается (там тот же список строит генератор).
func nodePoolDirectionTags(model *wizardmodels.WizardModel) map[string]bool {
	if model == nil {
		return nil
	}
	dirs := model.GlobalOutbounds
	tags := make(map[string]bool, len(dirs))
	for _, d := range dirs {
		if d.Tag != "" && !d.Disabled {
			tags[d.Tag] = true
		}
	}
	return tags
}

// nodePoolRootTargets — цели корневого пространства для резолва NodeLink:
// теги включённых Направлений (узлы и replace-теги добавляет сам
// BuildNodeLinkTargets по проекции источников).
func nodePoolRootTargets(model *wizardmodels.WizardModel) []string {
	tags := nodePoolDirectionTags(model)
	out := make([]string, 0, len(tags))
	for tag := range tags {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

// InvalidateNodePool снимает пул, чтобы следующий потребитель (Sources
// Refresh, View, вкладка Preview в Edit Outbound) пересобрал его.
// Зовётся всякий раз, когда меняется состав источников или их тег-политика.
func InvalidateNodePool(model *wizardmodels.WizardModel) {
	if model == nil {
		return
	}
	model.NodePoolGeneration++
	model.NodePool = nil
	model.NodePoolBySource = nil
	model.AvailableOutboundsMemoRev = 0
	model.AvailableOutboundsMemoTags = nil
	// Счётчики узлов выведены из пула и пережить его не могут: иначе список
	// Sources показывал бы числа от прошлого состава.
	model.SourceNodeCounts = nil
}

// NodesForDirectionPicker — узлы, которые предлагаются в выборе НАПРАВЛЕНИЯ.
//
// Отличается от полного пула ровно одним: служебные узлы (релеи провайдера,
// SPEC 120) по умолчанию не предлагаются — релей это дозвонщик внутри чужого
// маршрута, а не «страна», которую выбирают. Подписка может это
// переопределить своей галкой `RelaysInDirections`: тогда её релеи идут в выбор как
// обычные узлы.
//
// Пул при этом НЕ фильтруется: в конфиг релей попадает всегда (иначе detour
// на него повис бы), и в позициях цепочки он виден тоже всегда — там он и
// нужен. Отсюда отдельная функция, а не правка RebuildNodePool.
func NodesForDirectionPicker(model *wizardmodels.WizardModel) []*config.ParsedNode {
	if model == nil {
		return nil
	}
	// Какие источники разрешили свои релеи в выборе.
	exposed := make(map[int]bool, len(model.Sources))
	for i := range model.Sources {
		if model.Sources[i].RelaysInDirections {
			exposed[i] = true
		}
	}
	out := make([]*config.ParsedNode, 0, len(model.NodePool))
	for _, n := range model.NodePool {
		if n == nil {
			continue
		}
		if n.Service && !exposed[n.SourceIndex] {
			continue
		}
		out = append(out, n)
	}
	return out
}
