// File node_move.go — перенос узла между контейнерами (SPEC 116 этап 3, W2).
//
// Контейнер узла — это папка, подписка или корневое пространство sources[].
// Перенос узла из одного контейнера в другой МЕНЯЕТ ЕГО ФИНАЛЬНЫЙ ТЕГ: у
// каждой папки своя тег-политика (prefix/postfix), а у корня политики нет
// вовсе. Значит перенос — это переименование, и по features/directions.md §9
// он обязан идти через реестр ссылок: все NodeLink на прежнюю идентичность
// переписываются атомарно, а пользователю говорят, что выбор в селекторах
// живого ядра (cache.db адресован СТАРЫМИ тегами) протухнет.
//
// Адрес узла в модели — это пара (контейнер, СЫРОЙ тег):
//
//   - узел папки/подписки: NodeLink{FolderID: <ULID контейнера>, Tag: сырой};
//   - верхний узел: NodeLink{FolderID: "", Tag: финальный} — у корня политики
//     нет, поэтому финальный тег там равен сырому.
//
// Отсюда правило переписи: move меняет ОБЕ половины адреса разом, и ссылка,
// совпавшая с прежней парой, получает новую. Copy прежний узел на месте
// оставляет — переписывать нечего, старые ссылки продолжают вести к
// оригиналу; список переписи у copy пуст по построению, а не «забыт».
//
// Целей у переноса две, и корень — полноправная из них: MoveNodeToRoot /
// CopyNodeToRoot делают с ОДНИМ узлом то же, что ExtractFolderNodesToRoot со
// всей папкой, и механика у них общая (promoteNodeToRoot) — две реализации
// одного выноса разъехались бы на первой же правке уникализации тега.
//
// Побочки, ВИДИМЫЕ пользователю (BumpRevision / MarkAsChanged / диалоги),
// делает ВЫЗЫВАЮЩИЙ UI: одна пользовательская операция может звать несколько
// функций отсюда (удаление папки = вынос узлов + удаление источника), и
// ревизия обязана дёрнуться один раз в конце, а не по разу на каждый шаг.
//
// А вот инвалидация кэшей (пул узлов и выведенные из него счётчики строк) —
// ЗДЕСЬ, у самой мутации: кэш переживший смену состава показывает числа и
// теги от прошлого состояния, и «забытый сброс = вечный 0 nodes» уже стоил
// нам бага (ловушка cache-invalidation-pairs). InvalidateNodePool
// идемпотентен, поэтому повторный вызов из UI ничего не ломает.
package business

import (
	"encoding/json"
	"fmt"
	"strings"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// nodeContainerRef — куда узел приехал/откуда уехал в терминах NodeLink.
//
// folderID == "" означает корневое пространство: у верхнего узла ссылка
// адресуется одним лишь финальным тегом.
type nodeContainerRef struct {
	folderID string
	tag      string
}

// CopyNodeToFolder кладёт КОПИЮ узла (srcIndex, rawTag) в папку dstFolderID.
//
// Оригинал не тронут. origin.subUrl копии СОХРАНЯЕТСЯ — это прямое правило
// features/sources.md §«Наполнение папки» п.3: копия узла подписки участвует
// в будущей merge-заливке той же подписки, и обнулить связь здесь значило бы
// сделать копию неотличимой от узла, созданного руками.
//
// Сырой тег копии уникализируется в пределах ЦЕЛЕВОЙ папки (суффикс -2, -3…):
// сырой тег — идентичность узла в рамках контейнера, двух одинаковых там быть
// не может.
//
// Возвращает список задетых переписью источников (для окна-предупреждения) —
// у copy он ПУСТ по построению: оригинал остался на месте, ни одна живая
// ссылка адреса не сменила. Список возвращается всё равно, чтобы вызывающий
// UI обрабатывал copy и move одинаково.
func CopyNodeToFolder(m *wizardmodels.WizardModel, srcIndex int, rawTag, dstFolderID string) ([]string, error) {
	_, node, err := lookupNodeForMove(m, srcIndex, rawTag)
	if err != nil {
		return nil, err
	}
	dstIndex, err := lookupFolderIndex(m, dstFolderID)
	if err != nil {
		return nil, err
	}
	if _, err := placeNodeIntoFolder(m, dstIndex, cloneCanonicalNodeForMove(*node)); err != nil {
		return nil, err
	}
	InvalidateNodePool(m)
	return nil, nil
}

// MoveNodeToFolder переносит узел (srcIndex, rawTag) в папку dstFolderID:
// копия кладётся в целевую папку, оригинал удаляется из исходного контейнера,
// а все ссылки на прежний адрес переписываются на новый.
//
// enabled, detour, hops, состав группы и origin (включая subUrl) едут с узлом:
// перенос — это смена места, а не правка узла. Разыменование здесь НЕ
// выполняется — это отдельное событие (Д5, DereferenceNodeOrigin), которое
// наступает от правки содержимого, а не от переезда.
//
// ОТКАЗ, если исходный контейнер — подписка: состав подписки принадлежит
// провайдеру (features/sources.md §«Свобода и несвобода узлов»), убрать из
// неё узел лаунчер не вправе — следующий fetch всё равно вернёт его обратно.
// Из подписки существует только copy.
//
// Возвращает имена того, что задето переносом: источники, чьи NodeLink
// переписаны, и — при переносе ИЗ КОРНЯ — имена ссылок корневого
// пространства, переписать которые нельзя (см. rootOnlyRefsToTag).
func MoveNodeToFolder(m *wizardmodels.WizardModel, srcIndex int, rawTag, dstFolderID string) ([]string, error) {
	srcSource, node, err := lookupNodeForMove(m, srcIndex, rawTag)
	if err != nil {
		return nil, err
	}
	if srcSource.Kind == corestate.SourceKindSubscription {
		return nil, fmt.Errorf("move: node %q belongs to a subscription — its content is owned by the provider (copy only)", rawTag)
	}
	dstIndex, err := lookupFolderIndex(m, dstFolderID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(srcSource.ID) == strings.TrimSpace(dstFolderID) && srcSource.Kind == corestate.SourceKindFolder {
		return nil, nil // переезд в самого себя — no-op, а не «переименование»
	}

	from := containerRefOf(srcSource, node)
	moved := cloneCanonicalNodeForMove(*node)

	// SPEC 122 норма 2, половина «откуда»: ПРЕЖНИЙ финальный тег узла
	// считается ДО правки модели — после переноса исходного контейнера в
	// модели может уже не быть (корневой узел уезжает элементом Sources).
	oldStateDirTag := tailscaleFinalTagOfSource(srcSource, node.Tag)

	// Ссылки, которые переписать НЕЛЬЗЯ, считаем ДО правки модели: узел ещё
	// на месте, и его прежний корневой тег читается однозначно.
	orphans := rootOnlyRefsToTag(m, from)

	// Кладём ДО удаления: если целевая папка откажет (её больше нет), узел не
	// должен пропасть из исходной.
	newTag, err := placeNodeIntoFolder(m, dstIndex, moved)
	if err != nil {
		return nil, err
	}
	if err := removeNodeFromSource(m, srcIndex, rawTag); err != nil {
		return nil, err
	}

	// SPEC 122 норма 2: перенос между контейнерами — это смена финального
	// тега (у каждой папки своя тег-политика), значит каталог состояния
	// tailnet переезжает вместе с узлом.
	moved.Tag = newTag
	renameTailscaleStateDirTo(m, oldStateDirTag, dstFolderID, &moved)

	to := nodeContainerRef{folderID: strings.TrimSpace(dstFolderID), tag: newTag}
	affected := append(repointNodeLinks(m, from, to), orphans...)
	InvalidateNodePool(m)
	return affected, nil
}

// rootOnlyRefsToTag — ссылки КОРНЕВОГО пространства на тег уезжающего узла,
// которые переписать невозможно: цели правил, маршрут по умолчанию, detour
// DNS-серверов, опции и литеральное умолчание Направлений, переменные
// пресетов адресуют только имена корня, и узел, уехавший В ПАПКУ, для них
// перестаёт существовать.
//
// Молча потерять их нельзя (критерий A3: всякая ссылка, чья цель сменила
// финальный тег, либо переписана реестром, либо НАЗВАНА в предупреждении),
// поэтому здесь их только перечисляют — правки не делают (rootRefName в общем
// обходе root_name_refs.go). Осиротевшая цель правила сбросится на direct
// штатным путём загрузки, но узнать об этом пользователь обязан в момент
// операции, а не из лога следующего запуска.
//
// Пусто, когда узел уезжает не из корня (from.folderID != ""): ссылка на узел
// папки в этих классах невозможна по построению.
func rootOnlyRefsToTag(m *wizardmodels.WizardModel, from nodeContainerRef) []string {
	if m == nil || from.folderID != "" || from.tag == "" {
		return nil
	}
	names, _ := editRootNameRefs(m, rootNameIs(from.tag, rootRefName))
	return names
}

// firstNonEmptyRefName — первое непустое имя из перечисленных.
func firstNonEmptyRefName(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// ExtractFolderNodesToRoot выносит все узлы папки folderIndex в корень
// sources[]: каждый узел становится верхним Source со своим ULID.
//
// Порядок узлов и личные enabled сохранены (критерий A7): верхние Source
// встают подряд на место папки в списке, ровно в том порядке, в каком лежали
// в Nodes. Папка остаётся в модели ПУСТОЙ — удалять её (или нет) решает
// вызывающий: «вынести узлы» и «удалить папку» — два разных действия
// пользователя, и склеивать их здесь значило бы отнимать у UI второй исход
// диалога С7.
//
// Тег узла при выносе теряет тег-политику папки (у корня политики нет), то
// есть финальный тег меняется — ссылки переписываются, имена задетых
// источников возвращаются для предупреждения о протухшем выборе.
//
// Сырой тег, уже занятый в корневом пространстве, уникализируется суффиксом:
// корневой тег обязан быть уникален среди всего конфига.
func ExtractFolderNodesToRoot(m *wizardmodels.WizardModel, folderIndex int) []string {
	if m == nil || folderIndex < 0 || folderIndex >= len(m.Sources) {
		return nil
	}
	folder := &m.Sources[folderIndex]
	if folder.Kind != corestate.SourceKindFolder || len(folder.Nodes) == 0 {
		return nil
	}
	folderID := strings.TrimSpace(folder.ID)

	taken := rootTagSet(m)
	promoted := make([]corestate.Source, 0, len(folder.Nodes))
	repoints := make([][2]nodeContainerRef, 0, len(folder.Nodes))

	for i := range folder.Nodes {
		// SPEC 122 норма 2: прежний финальный тег — с политикой ПАПКИ,
		// новый — сырой (у корня политики нет). Считается до folder.Nodes=nil.
		oldStateDirTag := tailscaleFinalTagOfSource(folder, folder.Nodes[i].Tag)
		src, from, to := promoteNodeToRoot(folder.Nodes[i], folderID, taken)
		RenameTailscaleStateDirForNode(nil, oldStateDirTag, nil, &src.Node)
		promoted = append(promoted, src)
		repoints = append(repoints, [2]nodeContainerRef{from, to})
	}

	folder.Nodes = nil

	// Вставляем сразу ЗА папкой: в списке Sources вынесенные узлы окажутся
	// там, где пользователь их и оставил, а не в конце за подписками. Свой
	// цикл, а не insertSourceAfter по разу на узел: пакетная вставка обязана
	// быть одной — поштучная переливала бы слайс на каждом узле и требовала
	// поправки индекса на уже вставленных.
	//
	// go1.20-гард: без slices.Insert — ручная сборка нового слайса.
	rest := append([]corestate.Source(nil), m.Sources[folderIndex+1:]...)
	m.Sources = append(m.Sources[:folderIndex+1], promoted...)
	m.Sources = append(m.Sources, rest...)

	// Перепись — ПОСЛЕ вставки: repointNodeLinks обходит живую модель, и
	// ссылки самих вынесенных узлов (хопы цепочки на соседа по папке) обязаны
	// переписаться тоже.
	var affected []string
	seen := map[string]bool{}
	for _, pair := range repoints {
		for _, name := range repointNodeLinks(m, pair[0], pair[1]) {
			if !seen[name] {
				seen[name] = true
				affected = append(affected, name)
			}
		}
	}
	InvalidateNodePool(m)
	return affected
}

// promoteNodeToRoot готовит ОДИН узел контейнера к жизни верхним Source:
// клонирует его, уникализирует тег в корневом пространстве (занимая имя в
// taken, чтобы следующий вызов с тем же набором не повторил его) и заворачивает
// в новый Source со своим ULID.
//
// Общая механика выноса для обеих операций «папка → корень»: пакетной
// (ExtractFolderNodesToRoot) и поштучной (MoveNodeToRoot). Держится одной
// функцией именно потому, что расхождение здесь незаметно: две реализации
// уникализации разошлись бы на первой же правке суффикса, и вынесенный поштучно
// узел встал бы в корень под именем, которое пакетный вынос считает занятым.
//
// Модель НЕ трогает: вставку в m.Sources и удаление оригинала делает
// вызывающий — у пакетного выноса это одна вставка на все узлы разом.
// Возвращает готовый Source и пару адресов для переписи ссылок.
func promoteNodeToRoot(node corestate.Node, folderID string, taken map[string]bool) (corestate.Source, nodeContainerRef, nodeContainerRef) {
	n := cloneCanonicalNodeForMove(node)
	oldTag := n.Tag
	n.Tag = uniqueTagIn(taken, n.Tag)
	taken[n.Tag] = true
	return corestate.Source{Node: n, ID: corestate.MakeULID()},
		nodeContainerRef{folderID: folderID, tag: oldTag},
		nodeContainerRef{folderID: "", tag: n.Tag}
}

// insertSourceAfter вставляет источник сразу ЗА позицией at.
//
// Сразу за исходным контейнером, а не в хвост списка: узел обязан остаться там,
// где пользователь его видел, а не уехать под подписки в конец. Побочное
// свойство, на которое опирается UI: индекс самого контейнера НЕ меняется —
// сдвигаются только источники ПОСЛЕ него, поэтому открытое окно источника и
// drill-down списка продолжают адресовать свой контейнер прежним индексом.
//
// go1.20-гард: без slices.Insert — ручная сборка нового слайса.
func insertSourceAfter(m *wizardmodels.WizardModel, at int, src corestate.Source) {
	rest := append([]corestate.Source(nil), m.Sources[at+1:]...)
	m.Sources = append(m.Sources[:at+1], src)
	m.Sources = append(m.Sources, rest...)
}

// MoveNodeToRoot выносит ОДИН узел (srcIndex, rawTag) из папки в корень
// sources[]: узел становится верхним Source со своим ULID, встающим сразу за
// папкой-источником.
//
// То же, что ExtractFolderNodesToRoot делает со всем составом папки, но по
// одному узлу — и той же механикой (promoteNodeToRoot).
//
// ОТКАЗ, если исходный контейнер — подписка: её состав принадлежит провайдеру
// (features/sources.md §«Свобода и несвобода узлов») — ровно как у
// MoveNodeToFolder. Из подписки существует только copy.
//
// NO-OP, если исходный Source — не папка: корневой узел уже лежит в корне, и
// «перенести в корень» ему нечего. Возвращается (nil, nil) — вызывающий по
// пустому результату и отсутствию ошибки понимает, что показывать диалог о
// протухшем выборе не за что.
//
// Возвращает имена задетых переписью источников.
func MoveNodeToRoot(m *wizardmodels.WizardModel, srcIndex int, rawTag string) ([]string, error) {
	srcSource, node, err := lookupNodeForMove(m, srcIndex, rawTag)
	if err != nil {
		return nil, err
	}
	if srcSource.Kind == corestate.SourceKindSubscription {
		return nil, fmt.Errorf("move: node %q belongs to a subscription — its content is owned by the provider (copy only)", rawTag)
	}
	if srcSource.Kind != corestate.SourceKindFolder {
		return nil, nil // узел и так корневой — переносить некуда
	}

	folderID := strings.TrimSpace(srcSource.ID)
	// SPEC 122 норма 2: прежний финальный тег — до правки модели (см.
	// MoveNodeToFolder).
	oldStateDirTag := tailscaleFinalTagOfSource(srcSource, node.Tag)
	promotedSrc, from, to := promoteNodeToRoot(*node, folderID, rootTagSet(m))
	// У корня тег-политики нет: финальный тег вынесенного узла = его сырой.
	RenameTailscaleStateDirForNode(nil, oldStateDirTag, nil, &promotedSrc.Node)

	// Кладём ДО удаления — то же правило, что у MoveNodeToFolder: если что-то
	// откажет посередине, узел не должен пропасть из папки.
	insertSourceAfter(m, srcIndex, promotedSrc)
	if err := removeNodeFromSource(m, srcIndex, rawTag); err != nil {
		return nil, err
	}

	// Перепись — ПОСЛЕ вставки: repointNodeLinks обходит живую модель, и ссылки
	// самого вынесенного узла (его хопы на соседей по папке) обязаны попасть под
	// обход тоже.
	affected := repointNodeLinks(m, from, to)
	InvalidateNodePool(m)
	return affected, nil
}

// CopyNodeToRoot кладёт КОПИЮ узла (srcIndex, rawTag) в корень sources[]
// отдельным верхним Source, сразу за исходным источником.
//
// Разрешено из ЛЮБОГО контейнера, включая подписку, — как и CopyNodeToFolder:
// копия ничего в источнике не меняет, это ровно требование «забрать узел
// провайдера себе». origin.subUrl копии СОХРАНЯЕТСЯ (features/sources.md
// §«Наполнение папки» п.2): копия участвует в merge-заливке той же подписки.
//
// Копия узла В СВОЁМ ЖЕ контейнере (корневой узел → корень) законна и даёт
// дубликат с суффиксом в теге — то же поведение, что у copy в собственную
// папку.
//
// Возвращает (nil, nil): переписывать нечего по построению — оригинал остался
// на месте под прежним адресом, ни одна ссылка цели не сменила.
func CopyNodeToRoot(m *wizardmodels.WizardModel, srcIndex int, rawTag string) ([]string, error) {
	srcSource, node, err := lookupNodeForMove(m, srcIndex, rawTag)
	if err != nil {
		return nil, err
	}
	copySrc, _, _ := promoteNodeToRoot(*node, strings.TrimSpace(srcSource.ID), rootTagSet(m))
	if copySrc.Tag == "" {
		return nil, fmt.Errorf("node move: node has no tag")
	}
	insertSourceAfter(m, srcIndex, copySrc)
	InvalidateNodePool(m)
	return nil, nil
}

// DereferenceNodeOrigin обнуляет origin.subUrl узла — «разыменование»
// (features/sources.md §«Авторазыменование»).
//
// Общая точка на все поводы разыменования: ручная правка тега/body/Regen
// (Д5), пометка «исчез у провайдера» при заливке (Д1). Узел без origin или
// без subUrl не меняется.
//
// Возвращает true, если связь действительно была снята — вызывающему это
// нужно, чтобы показать уведомление ровно один раз и только по делу.
func DereferenceNodeOrigin(node *corestate.Node) bool {
	if node == nil || node.Origin == nil || node.Origin.SubURL == "" {
		return false
	}
	// Origin ПЕРЕСАЖИВАЕТСЯ на новый экземпляр, а не правится на месте: Node
	// несёт *Origin, и копии узла (буфер формы, материал заливки) делят его с
	// оригиналом — правка на месте отвязала бы от подписки чужой узел. Та же
	// причина, что у state.setNodeSubURL.
	o := *node.Origin
	o.SubURL = ""
	node.Origin = &o
	return true
}

// RepointContainerNodeLinks переписывает ссылки на узел контейнера, чей СЫРОЙ
// ТЕГ сменился на месте (SPEC 116 W5: команда «Rename…» в списке узлов).
//
// Переименование — это смена идентичности узла (SPEC 112), и по
// features/directions.md §9 оно идёт тем же реестром, что и перенос: контейнер
// не меняется, меняется вторая половина адреса. Отдельная функция, а не голый
// вызов repointNodeLinks у UI, потому что реестр — внутренний контракт этого
// файла: новый вид ссылки добавляется сюда, и все входы получают его разом.
//
// Возвращает имена задетых источников (для предупреждения пользователю).
func RepointContainerNodeLinks(m *wizardmodels.WizardModel, folderID, oldTag, newTag string) []string {
	affected := repointNodeLinks(m,
		nodeContainerRef{folderID: strings.TrimSpace(folderID), tag: oldTag},
		nodeContainerRef{folderID: strings.TrimSpace(folderID), tag: newTag})
	// Переименование = смена идентичности узла (SPEC 112), а значит и его
	// финального тега в пуле: кэш обязан пересобраться.
	InvalidateNodePool(m)
	return affected
}

// ClearContainerNodeLinks гасит ссылки на узел контейнера, которого БОЛЬШЕ НЕТ
// (SPEC 116 W5: команда «Delete» в списке узлов).
//
// Перенаправить их не на что: узел удалён, и подставить вместо него соседа
// нельзя — пользователь выбирал хопом/детуром именно этот. Решение владельца
// (15.09.2026): все ссылки, которые узел использовали, гаснут вместе с ним, и
// пользователь узнаёт об этом в момент удаления, а не из отчёта следующей
// сборки, — поэтому имена задетых источников возвращаются наружу.
//
// Гашение идёт реестром (clearNodeLinks): detour источника снимается совсем,
// позиция выходит из цепочки, член — из группы вместе с умолчанием на нём.
func ClearContainerNodeLinks(m *wizardmodels.WizardModel, folderID, tag string) []string {
	affected := clearNodeLinks(m, nodeContainerRef{folderID: strings.TrimSpace(folderID), tag: tag})
	InvalidateNodePool(m)
	return affected
}

// clearNodeLinks гасит все ссылки NodeLink на адрес from и возвращает имена
// задетых источников. Пустой from.folderID — корневой узел.
func clearNodeLinks(m *wizardmodels.WizardModel, from nodeContainerRef) []string {
	if m == nil || from.tag == "" {
		return nil
	}
	affected, _ := editNodeLinks(m, func(link corestate.NodeLink, space string) (corestate.NodeLink, linkEdit) {
		if !linkAddresses(link, space, from) {
			return link, linkKeep
		}
		return link, linkDrop
	})
	return affected
}

// repointNodeLinks переписывает все ссылки NodeLink с адреса from на адрес to
// и возвращает имена задетых источников.
//
// Реестр ссылок на УЗЕЛ (features/directions.md §9): detour источников, хопы
// цепочек, члены Auto и default селектора. Цели правил, route.final, detour
// DNS-серверов и addOutbounds Направлений сюда НЕ входят по построению: они
// адресуют корневое пространство финальных тегов по имени, и их переписывает
// (или называет) вызывающий — root_name_refs.go. Перенос В корень такую
// ссылку создаёт (новый верхний тег), но не переписывает: адресат появился, а
// не переехал.
//
// Правило блока: новый вид ссылки на узел — сначала в editNodeLinks, потом в
// модель.
func repointNodeLinks(m *wizardmodels.WizardModel, from, to nodeContainerRef) []string {
	if m == nil || from.tag == "" || (from.folderID == to.folderID && from.tag == to.tag) {
		return nil
	}
	affected, _ := editNodeLinks(m, func(link corestate.NodeLink, space string) (corestate.NodeLink, linkEdit) {
		if !linkAddresses(link, space, from) {
			return link, linkKeep
		}
		return corestate.NodeLink{FolderID: to.folderID, Tag: to.tag}, linkReplace
	})
	return affected
}

// linkAddresses — ведёт ли ссылка на адрес target.
//
// space — чьим адресом считается ссылка без folder_id (NODE_LINK.md §5.1):
// у detour и позиции это корень (№ 7), у члена группы внутри контейнера — сам
// контейнер (№ 8). Без этого различия переименование корневого узла уводило
// бы членство провайдерской группы в корень, а перепись узла папки не
// замечала бы члена, записанного без адреса.
func linkAddresses(link corestate.NodeLink, space string, target nodeContainerRef) bool {
	folderID := strings.TrimSpace(link.FolderID)
	if folderID == "" {
		folderID = space
	}
	return folderID == target.folderID && link.Tag == target.tag
}

// linkEdit — что правка делает с одной ссылкой.
type linkEdit int

const (
	// linkKeep — ссылка не задета.
	linkKeep linkEdit = iota
	// linkReplace — ссылка переписана на возвращённый адрес.
	linkReplace
	// linkDrop — ссылка гаснет: detour снимается, позиция и член выходят из
	// своих списков.
	linkDrop
)

// linkEditFunc — решение по одной ссылке: её пространство (см.
// linkAddresses), новое значение и действие.
type linkEditFunc func(link corestate.NodeLink, space string) (corestate.NodeLink, linkEdit)

// editNodeLinks — ЕДИНЫЙ обход реестра ссылок на узел: detour и позиции
// корневых записей и членов контейнеров, члены и умолчания групп в корне и
// в контейнерах (features/directions.md §9, NODE_LINK.md §4.1).
//
// edit получает ссылку и её пространство и решает её судьбу. Возвращает
// имена задетых источников (для окна-предупреждения) и число задетых ссылок.
func editNodeLinks(m *wizardmodels.WizardModel, edit linkEditFunc) ([]string, int) {
	if m == nil {
		return nil, 0
	}
	count := 0
	node := func(n *corestate.Node, space string) bool {
		hits := editDetourLink(&n.Detour, edit)
		if len(n.Hops) > 0 {
			hops, h := editLinkList(n.Hops, "", edit, nil)
			n.Hops = hops
			hits += h
		}
		hits += editGroupLinks(n.Group, space, edit)
		count += hits
		return hits > 0
	}

	var affected []string
	for i := range m.Sources {
		s := &m.Sources[i]
		// Верхняя запись: detour узла или общий detour контейнера, позиции
		// корневой цепочки, члены корневой группы — всё в корневом
		// пространстве.
		touched := node(&s.Node, "")
		// Узлы контейнера несут те же виды ссылок; члены их групп без адреса
		// адресуют сам контейнер.
		space := strings.TrimSpace(s.ID)
		for k := range s.Nodes {
			if node(&s.Nodes[k], space) {
				touched = true
			}
		}
		if touched {
			affected = append(affected, SourceDisplayName(*s))
		}
	}
	return affected, count
}

// editDetourLink — detour узла или контейнера: переписывается на месте либо
// снимается (nil). detour адресует корень, если folder_id пуст (§5.1 № 7).
func editDetourLink(link **corestate.NodeLink, edit linkEditFunc) int {
	if *link == nil {
		return 0
	}
	next, act := edit(**link, "")
	switch act {
	case linkReplace:
		**link = next
	case linkDrop:
		*link = nil
	default:
		return 0
	}
	return 1
}

// editLinkList — список ссылок (позиции цепочки, члены группы): переписанные
// остаются на месте, погасшие выходят из списка. onEdit узнаёт о каждой
// задетой ссылке (прежний тег, новый; пустой новый — погасла). Незадетый
// список возвращается тем же срезом; изменённый — новым, исходный массив не
// портится. Ручной фильтр без slices.* — go1.20-гард (win7-джоба CI).
func editLinkList(links []corestate.NodeLink, space string, edit linkEditFunc, onEdit func(oldTag, newTag string)) ([]corestate.NodeLink, int) {
	hits := 0
	out := links[:0:0]
	for j := range links {
		next, act := edit(links[j], space)
		switch act {
		case linkReplace:
			out = append(out, next)
		case linkDrop:
			next.Tag = ""
		default:
			out = append(out, links[j])
			continue
		}
		hits++
		if onEdit != nil {
			onEdit(links[j].Tag, next.Tag)
		}
	}
	if hits == 0 {
		return links, 0
	}
	return out, hits
}

// editGroupLinks — ЕДИНАЯ точка правки состава группы: члены и умолчание
// вместе, для переписи и для гашения.
//
// Умолчание — ссылка NodeLink на член — решается тем же edit, что члены:
// адрес у них один, поэтому переписанный член уносит умолчание на новый адрес,
// погасший снимает его, и группа не эмитится с default вне состава. Ссылка без
// folder_id внутри контейнера адресует свой контейнер (space, §5.1 № 8).
// Возвращает число задетых ссылок группы (члены и умолчание).
func editGroupLinks(g *corestate.AutoGroup, space string, edit linkEditFunc) int {
	if g == nil {
		return 0
	}
	hits := 0
	if len(g.Members) > 0 {
		members, h := editLinkList(g.Members, space, edit, nil)
		if h > 0 {
			g.Members = members
			hits += h
		}
	}
	if g.Default != nil {
		next, act := edit(*g.Default, space)
		switch act {
		case linkReplace:
			// Новый экземпляр, а не правка по указателю: копии узла (перенос,
			// копия в папку) делят его с оригиналом до глубокого клона.
			g.Default = &next
			hits++
		case linkDrop:
			g.Default = nil
			hits++
		}
	}
	return hits
}

// ── внутренняя механика ──────────────────────────────────────────

// lookupNodeForMove находит узел по (индекс источника, сырой тег).
//
// Узел живёт в двух формах: внутри контейнера (Sources[i].Nodes) и как сам
// верхний Source (Sources[i].Node). Обе адресуются одинаково — индексом
// источника и сырым тегом, — поэтому разбор формы живёт здесь, а не
// расползается по вызывающим.
func lookupNodeForMove(m *wizardmodels.WizardModel, srcIndex int, rawTag string) (*corestate.Source, *corestate.Node, error) {
	if m == nil {
		return nil, nil, fmt.Errorf("node move: model is nil")
	}
	if srcIndex < 0 || srcIndex >= len(m.Sources) {
		return nil, nil, fmt.Errorf("node move: source index %d out of range", srcIndex)
	}
	s := &m.Sources[srcIndex]
	switch s.Kind {
	case corestate.SourceKindFolder, corestate.SourceKindSubscription:
		for i := range s.Nodes {
			if s.Nodes[i].Tag == rawTag {
				return s, &s.Nodes[i], nil
			}
		}
		return nil, nil, fmt.Errorf("node move: node %q not found in %q", rawTag, SourceDisplayName(*s))
	default:
		if s.NodeTagOrLabel() != rawTag {
			return nil, nil, fmt.Errorf("node move: node %q not found in %q", rawTag, SourceDisplayName(*s))
		}
		return s, &s.Node, nil
	}
}

// lookupFolderIndex — индекс папки по ULID. Целью *этих* переносов может быть
// только папка: подписка чужая (её состав принадлежит провайдеру), а корень
// адресуется не folderID (там он пуст у всех сразу), а отдельными операциями —
// MoveNodeToRoot / CopyNodeToRoot. Поэтому пустой ULID здесь — ошибка, а не
// «корень»: тихо принять его значило бы положить узел в первую попавшуюся
// папку.
func lookupFolderIndex(m *wizardmodels.WizardModel, folderID string) (int, error) {
	folderID = strings.TrimSpace(folderID)
	if m == nil || folderID == "" {
		return -1, fmt.Errorf("node move: destination folder is not set")
	}
	for i := range m.Sources {
		if m.Sources[i].Kind == corestate.SourceKindFolder && strings.TrimSpace(m.Sources[i].ID) == folderID {
			return i, nil
		}
	}
	return -1, fmt.Errorf("node move: folder %q not found", folderID)
}

// placeNodeIntoFolder кладёт узел в хвост папки, уникализируя сырой тег в её
// пределах. Возвращает тег, под которым узел лёг.
//
// Хвост, а не сортировка: порядок узлов папки задаёт пользователь
// (features/sources.md §«Наполнение папки»), и приехавший узел не вправе
// раздвигать чужие позиции.
func placeNodeIntoFolder(m *wizardmodels.WizardModel, dstIndex int, node corestate.Node) (string, error) {
	dst := &m.Sources[dstIndex]
	taken := make(map[string]bool, len(dst.Nodes))
	for i := range dst.Nodes {
		taken[dst.Nodes[i].Tag] = true
	}
	node.Tag = uniqueTagIn(taken, node.Tag)
	if node.Tag == "" {
		return "", fmt.Errorf("node move: node has no tag")
	}
	dst.Nodes = append(dst.Nodes, node)
	return node.Tag, nil
}

// removeNodeFromSource убирает узел из контейнера либо весь верхний Source.
func removeNodeFromSource(m *wizardmodels.WizardModel, srcIndex int, rawTag string) error {
	s := &m.Sources[srcIndex]
	switch s.Kind {
	case corestate.SourceKindFolder, corestate.SourceKindSubscription:
		for i := range s.Nodes {
			if s.Nodes[i].Tag == rawTag {
				s.Nodes = append(s.Nodes[:i], s.Nodes[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("node move: node %q vanished from %q", rawTag, SourceDisplayName(*s))
	default:
		m.Sources = append(m.Sources[:srcIndex], m.Sources[srcIndex+1:]...)
		return nil
	}
}

// containerRefOf — прежний адрес узла в терминах NodeLink.
func containerRefOf(s *corestate.Source, node *corestate.Node) nodeContainerRef {
	switch s.Kind {
	case corestate.SourceKindFolder, corestate.SourceKindSubscription:
		return nodeContainerRef{folderID: strings.TrimSpace(s.ID), tag: node.Tag}
	default:
		// Верхний узел адресуется корневым пространством: folderID пуст, тег
		// финальный — а он у корня равен сырому (политики нет).
		return nodeContainerRef{folderID: "", tag: s.NodeTagOrLabel()}
	}
}

// rootTagSet — занятые имена корневого пространства.
//
// Реестр здесь ОДИН — ModelTagOwners (tag_guard_model.go): теги верхних узлов,
// Направления и их `-auto`-твины, replace-теги свёрнутых папок с их
// двойниками, системные теги шаблона и пресетов. Свой сокращённый список
// (только узлы и включённые Направления) стоил уникальности: узел, вынесенный
// из папки под именем свёрнутой соседки, вставал в корень её тегом — и в
// сборке две сущности спорили за одно имя.
//
// Выключенные Направления добавляются сверху тем же списком, что у
// KnownRuleTargetTags: тег временно снятого Направления занят — включат
// обратно, и столкновение всплывёт уже задним числом.
func rootTagSet(m *wizardmodels.WizardModel) map[string]bool {
	if m == nil {
		return map[string]bool{}
	}
	return KnownRuleTargetTags(m)
}

// uniqueTagIn подбирает свободное имя вида `X`, `X-2`, `X-3` — та же форма
// суффикса, что у уникализации на эмиссии (subscription.uniquifyAgainstCounts),
// чтобы имена из разных путей выглядели одинаково.
func uniqueTagIn(taken map[string]bool, tag string) string {
	if tag == "" || !taken[tag] {
		return tag
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", tag, n)
		if !taken[candidate] {
			return candidate
		}
	}
}

// cloneCanonicalNodeForMove — глубокая копия Node: Origin/Body/Detour/Hops/
// Group ссылочные, и копия узла обязана владеть своими экземплярами, иначе
// правка перенесённого узла била бы по оригиналу (и наоборот).
//
// go1.20-гард: без slices./maps. — ручные append и копии структур.
func cloneCanonicalNodeForMove(n corestate.Node) corestate.Node {
	c := n
	if n.Origin != nil {
		o := *n.Origin
		c.Origin = &o
	}
	if n.Body != nil {
		c.Body = append(json.RawMessage(nil), n.Body...)
	}
	if n.Detour != nil {
		d := *n.Detour
		c.Detour = &d
	}
	if n.Hops != nil {
		c.Hops = append([]corestate.NodeLink(nil), n.Hops...)
	}
	if n.Group != nil {
		g := *n.Group
		g.Members = append([]corestate.NodeLink(nil), n.Group.Members...)
		// Умолчание — указатель: копия обязана владеть своим, иначе перепись
		// ссылок у копии переписала бы и оригинал.
		if n.Group.Default != nil {
			d := *n.Group.Default
			g.Default = &d
		}
		// Strategy глубоко: *TemplateInt (Tolerance/PoolTolerance) не должны
		// разделяться указателями между копией и оригиналом.
		g.Strategy = *n.Group.Strategy.Clone()
		c.Group = &g
	}
	return c
}
