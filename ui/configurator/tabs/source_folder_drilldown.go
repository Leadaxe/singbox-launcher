// File source_folder_drilldown.go — вход В КОНТЕЙНЕР прямо в списке Sources
// (SPEC 116 W13, обкатка; требование дословно: «нажать на папку и в том же
// окне, не меняя интерфейса, провалиться в папку». Заход 2 распространил то же
// на ПОДПИСКУ).
//
// # Что это такое и чем НЕ является
//
// Это не второй список узлов и не новое окно. Вкладка Sources получает одно
// состояние — ULID открытого контейнера, — и ТА ЖЕ функция `refreshSourcesList`
// наполняет ТОТ ЖЕ `sourcesBox` либо корнем, либо составом контейнера. Виджеты
// те же: `HoverRow` + `SecondaryTapWrap` снаружи (та же ловушка глубины Fyne,
// что у W13-меню верхнего узла), `DragHandle` из той же `DragReorderGroup`,
// `widget.Check`.
//
// Второго списка узлов заводить нельзя по той же причине, по которой W5 не
// стала заводить его в окне источника (§O4 = вариант А): состав контейнера
// один, и вторая его модель разъехалась бы с первой. Поэтому строки строятся
// ТЕМ ЖЕ `buildPreviewRows` (preview_rows.go) и той же эмиссией
// `config.EmitCanonicalSource`, что вкладка Preview, а рисуются той же парой
// `canvas.Text` в `previewTightVBox`, что строка списка Preview: заход 2
// исправил вёрстку, разъехавшуюся с превью (заголовок и подстрока не
// выравнивались с чекбоксом, `widget.Label` давал лишние отступы).
//
// # Папка и подписка: один экран, разные права
//
// Различие целиком в МОДЕЛИ, а не в экране: состав подписки принадлежит
// провайдеру (features/sources.md §«Свобода и несвобода узлов»), и Move /
// Rename / Delete там запрещены — следующий fetch вернул бы удалённый узел и
// переименовал переименованный. Экран это уже знает одним местом —
// `previewNodeOps.nodeOpsAllowed()` (= «kind == folder»): меню само не
// показывает запрещённых пунктов, корзина у строки не рисуется, а поле Add
// наверху вкладки в режиме подписки гаснет с подсказкой (в подписку руками не
// льют). Развилку «а это подписка?» по экрану не разносить — она одна и живёт
// в модели прав.
//
// # Почему операции — те же самые
//
// Правый клик по узлу в этом режиме зовёт `showPreviewRowContextMenu` с
// контекстом `previewNodeOps`, у которого `win` = главное окно визарда, а
// `reloadScratch`/`refreshPreview` пусты — рабочей копии у списка нет, список
// целиком перестраивает `applySourceMutation` (ровно как в
// source_row_node_ops.go). Move/Copy/Rename/Delete и реестр переписи ссылок
// берутся целиком; вторых диалогов волна не заводит. Кнопки справа у строки —
// ТОТ ЖЕ набор действий, что в меню (принцип «меню = кнопки»): карандаш зовёт
// `showPreviewNodeEditWindow`, корзина — `previewNodeOps.showDeleteDialog`.
//
// # Почему Add-поле в режиме папки льёт в папку
//
// Поле ввода наверху вкладки — единственный Add, который видит пользователь,
// и в режиме папки «добавить» может означать только «добавить сюда». Разбор
// текста при этом ТОТ ЖЕ (`parseSourceInput` через
// `business.AppendNodesToFolder`, W6): корень и папка отличаются адресом
// назначения, а не разбором (ловушка «эмиттер и парсер ходят парой», дыра Д6).
//
// # Чего в режиме контейнера нет
//
// «Preview all servers…» остаётся кнопкой КОРНЯ (она про все источники
// сразу), а кнопка ↻ у узла бессмысленна: URL есть у подписки целиком, а не у
// её узла. Порядок узлов таскается внутри ПАПКИ (`applyReorder`, W5) — у
// подписки порядок задаёт тело провайдера, и захват там не показывается.
// Выход возвращает корень ровно таким, каким он был: порядок `m.Sources` режим
// не трогает вовсе.
//
// # Окно строк на больших контейнерах
//
// Подписка на CIDR-диапазон даёт 500+ узлов, и до этой волны каждая
// перерисовка строила ВСЕ строки: снятая галка → applySourceMutation →
// RefreshSourcesList → пятьсот HoverRow заново, плюс эмиссия всего состава.
// Один клик замораживал окно.
//
// Лечится ОКНОМ строк: строятся только те, что видны, плюс запас сверху и
// снизу (folderDrillWindowMargin), а место скрытых держат две прозрачные
// распорки. Арифметика диапазона и высот — в source_folder_drill_window.go,
// чистыми функциями с таблицей: распорки обязаны дать ровно ту же высоту
// содержимого и те же позиции строк, что полная отрисовка, иначе смещение
// прокрутки после перестройки указывает не туда.
//
// Порог — folderDrillWindowThreshold: до него состав рисуется целиком, как
// раньше, до единой строки. Ниже порога поведение не меняется вовсе; окно —
// только для тех контейнеров, где полная отрисовка уже не работает.
//
// ПОЧЕМУ НЕ widget.List. Он переиспользует объекты строк, и это ломает сразу
// два инварианта режима: строки контейнера живут в ТОМ ЖЕ VBox, что строки
// корня (второго списка не заводим — см. выше), а группа перетаскивания у них
// общая, и переиспользуемая строка держала бы в ней сразу два индекса
// (RegisterRecycled лечит это только внутри своего List). Окно оставляет
// строки обычными: каждая построена под свой индекс и живёт до следующей
// перестройки.
//
// ЗАМОРОЗКА НА БРОСКЕ. Пока `dragGroup.Dragging()`, окно не сдвигается:
// autoScroll во время перетаскивания сам крутит список, и перестройка выдернула
// бы из-под пальца и перетаскиваемую строку, и полосы, по которым считается
// точка вставки. После броска OnReorder ведёт в applySourceMutation, и окно
// пересчитывается на общих основаниях.
//
// ИНДЕКСЫ. В оконном режиме строка регистрируется под АБСОЛЮТНЫМ номером узла
// в составе (тем же, что в `input.Rows`), а группе выставляется `Total` = весь
// состав: без него бросок за нижний край окна упёрся бы в индекс последней
// ПОСТРОЕННОЙ строки. Вне окна `Total` возвращается в 0 — группа общая с
// корневым списком, поэтому выставляется он явно на КАЖДОЙ перерисовке в обе
// стороны.
package tabs

import (
	"errors"
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
	wizardutils "singbox-launcher/ui/configurator/utils"
)

// folderDrillBackGap — неразрывный пробел между иконкой возврата и именем.
//
// U+00A0, а не обычный пробел: имя и знак возврата — одна подпись одной
// строки, и перенос между ними оторвал бы стрелку от того, куда она ведёт.
// Текстовой «←» здесь больше нет: глифа U+2190 в шрифте Fyne нет, и обкатка
// показала на его месте «�» (заход 2, пункт 2). Знак возврата рисует
// `theme.NavigateBackIcon()` — иконка, а не символ.
const folderDrillBackGap = " "

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	// folderDrillHintText — подсказка под полем ввода в режиме папки.
	//
	// Отличается от корневой ровно тем, чем отличается адрес: подписка в папку
	// не кладётся (вложенных контейнеров нет), и перечислять её среди
	// принимаемого значило бы обещать то, что ядро отвергнет.
	folderDrillHintText = "Adds nodes to this folder: direct links (vless://, vmess://, trojan://, ss://, hysteria://, hysteria2://, ssh://, wireguard://) or sing-box JSON. One per line. A subscription URL goes to Sources, not into a folder."
	// subDrillHintText — подсказка в режиме ПОДПИСКИ.
	//
	// Поле Add там выключено: состав подписки приезжает от провайдера, и узел,
	// добавленный руками, исчез бы на первом же обновлении. Сказать это словами
	// обязательно — выключенное поле без объяснения читается как поломка.
	subDrillHintText = "Nodes of a subscription come from the provider and cannot be added by hand — an entry added here would be gone on the next update. Use “Copy to folder…” on a node to take it for yourself."
)

// folderDrillState — состояние вкладки «мы внутри контейнера».
//
// ULID, а не индекс: пока пользователь смотрит состав, порядок `m.Sources`
// вправе поменяться (фоновый fetch, перетаскивание из второго окна, удаление
// соседа), и индекс увёл бы список в чужой источник. ULID — единственная
// идентификация контейнера (SPEC 118).
type folderDrillState struct {
	// folderID — ULID открытого контейнера (папки или подписки); пусто = режим
	// корня. Имя поля историческое: режим начинался с папок, а адресация у
	// обоих контейнеров одна и та же.
	folderID string
	// announceOpen — раскрыт ли блок объявления провайдера под шапкой.
	//
	// Живёт в состоянии ВКЛАДКИ, а не в модели: это положение экрана, оно не
	// сохраняется и не имеет смысла для сборки. Свёрнут по умолчанию —
	// объявление бывает многострочным и оттеснило бы состав, ради которого
	// сюда и заходят. Сбрасывается на выходе: раскрыв анонс одной подписки,
	// человек не просил раскрывать его у следующей.
	announceOpen bool

	// Кэш эмиссии состава. `buildFolderDrillRows` зовёт
	// config.EmitCanonicalSource на ВЕСЬ контейнер, и на подписке в 500+ узлов
	// это самая дорогая часть перерисовки. Состав меняется только вместе с
	// моделью, а модель об этом честно сообщает двумя счётчиками
	// (Revision бампает applySourceMutation, NodePoolGeneration —
	// InvalidateNodePool в той же цепочке), поэтому ключ = (контейнер,
	// ревизия, поколение пула). Кэш работает НЕЗАВИСИМО от порога: он дёшев и
	// полезен любому контейнеру.
	cacheFolderID string
	cacheRevision uint64
	cacheNodeGen  int
	cacheValid    bool
	cacheInput    folderDrillRowsInput

	// Текущее окно строк: [winStart, winEnd) в номерах узлов состава.
	// winTotal — размер состава, под который окно посчитано; winActive —
	// «окно вообще применяется» (состав больше порога). Всё вместе живёт в
	// состоянии ВКЛАДКИ, а не в модели: это положение экрана.
	winStart, winEnd, winTotal int
	winActive                  bool

	// rowHeight — измеренная высота строки узла. Шаблон строки один на все
	// (source_node_row.go), поэтому одного замера хватает на весь список; 0 =
	// «ещё не мерили», тогда окно строится от начала.
	rowHeight float32

	// rebuilding — идёт перестройка окна. Защита от повторного входа:
	// перестройка трогает содержимое того же VScroll, чей OnScrolled её и
	// позвал, и Refresh изнутри колбэка вернулся бы сюда же.
	rebuilding bool
}

// invalidateCache снимает кэш эмиссии и забывает окно.
//
// Зовётся на выходе из контейнера: следующий вход может быть в другой
// контейнер, а окно от прошлого состава указывало бы в чужие строки.
func (d *folderDrillState) invalidateCache() {
	if d == nil {
		return
	}
	d.cacheValid = false
	d.cacheInput = folderDrillRowsInput{}
	d.cacheFolderID = ""
	d.winStart, d.winEnd, d.winTotal = 0, 0, 0
	d.winActive = false
}

// rowsFor отдаёт состав контейнера, переиспользуя кэш при совпадении ключа.
//
// Второй результат — то же «контейнер ещё существует», что у
// buildFolderDrillRows: пропавший контейнер не кэшируется, вызывающий по нему
// возвращается в корень.
func (d *folderDrillState) rowsFor(
	sources []corestate.Source,
	folderID string,
	revision uint64,
	nodeGen int,
) (folderDrillRowsInput, bool) {
	if d != nil && d.cacheValid &&
		d.cacheFolderID == folderID && d.cacheRevision == revision && d.cacheNodeGen == nodeGen {
		return d.cacheInput, true
	}
	input, ok := buildFolderDrillRows(sources, folderID)
	if !ok {
		if d != nil {
			d.cacheValid = false
		}
		return folderDrillRowsInput{}, false
	}
	if d != nil {
		d.cacheFolderID = folderID
		d.cacheRevision = revision
		d.cacheNodeGen = nodeGen
		d.cacheInput = input
		d.cacheValid = true
	}
	return input, true
}

// active — вкладка сейчас показывает состав контейнера.
func (d *folderDrillState) active() bool {
	return d != nil && strings.TrimSpace(d.folderID) != ""
}

// enter открывает контейнер, leave возвращает корень. Перерисовку зовёт
// вызывающий: обе операции меняют ровно одно поле, и лишний refresh при
// повторном клике по той же строке был бы мельканием списка.
func (d *folderDrillState) enter(folderID string) {
	id := strings.TrimSpace(folderID)
	if id != d.folderID {
		// Вход в ДРУГОЙ контейнер: окно от прошлого состава указывает в чужие
		// строки, а прокрутку тут же обнулит resetScrollOnModeChange.
		d.invalidateCache()
	}
	d.folderID = id
	d.announceOpen = false
}
func (d *folderDrillState) leave() {
	d.folderID = ""
	d.announceOpen = false
	// Кэш и окно — про КОНКРЕТНЫЙ состав: следующий вход может быть в другой
	// контейнер, и старое окно указывало бы в чужие строки.
	d.invalidateCache()
}

// nodesAreFree — в открытый контейнер можно КЛАСТЬ узлы руками.
//
// Ровно у папки. У подписки состав принадлежит провайдеру, и добавленный
// руками узел исчез бы на первом же fetch'е — то же правило и та же причина,
// что у `previewNodeOps.nodeOpsAllowed`; ссылаться на модель прав здесь нечем
// (контекста операций у шапки вкладки нет), поэтому вид читается из модели.
//
// В корне (режим не активен) свобода полная: там Add кладёт ИСТОЧНИКИ.
func (d *folderDrillState) nodesAreFree(sources []corestate.Source) bool {
	if !d.active() {
		return true
	}
	idx := folderDrillIndex(sources, d.folderID)
	if idx < 0 {
		return false
	}
	return sources[idx].Kind == corestate.SourceKindFolder
}

// drillContainerKind — в какой контейнер можно провалиться прямо из списка.
//
// Папка и подписка, и обе по одной причине: у них не один узел, а СОСТАВ, и
// смотреть на него строкой контейнера негде. У server/chain/auto состава нет —
// узел там и есть сам Source, его правит окно источника.
func drillContainerKind(kind corestate.SourceKind) bool {
	switch kind {
	case corestate.SourceKindFolder, corestate.SourceKindSubscription:
		return true
	default:
		return false
	}
}

// folderDrillIndex — позиция открытого контейнера в m.Sources; -1, если его не
// стало.
//
// Пропавший контейнер (удалили из второго окна, откатили импортом бэкапа) режим
// не ломает: список сам возвращается в корень — показывать состав того, чего
// больше нет, не из чего.
func folderDrillIndex(sources []corestate.Source, folderID string) int {
	id := strings.TrimSpace(folderID)
	if id == "" {
		return -1
	}
	for i := range sources {
		if sources[i].ID == id && drillContainerKind(sources[i].Kind) {
			return i
		}
	}
	return -1
}

// folderDrillRowsInput — всё, что нужно строкам режима контейнера.
//
// Собирается ОДНИМ проходом на перерисовку: эмиссия стоит денег (тег-машина +
// уникализация), и звать её по разу на строку значило бы пересобирать состав N
// раз на один показ.
type folderDrillRowsInput struct {
	// SourceIndex — позиция контейнера на момент сборки строк.
	SourceIndex int
	// Kind — вид контейнера: от него зависят права над узлами (см. шапку).
	Kind corestate.SourceKind
	// Rows — состав контейнера строками (та же модель, что у вкладки Preview).
	Rows []previewRow
	// Identities — сырые теги строк: адрес всех операций над узлом.
	Identities []string
	// Name — как контейнер зовут пользователю (для строки возврата).
	Name string
}

// buildFolderDrillRows собирает состав открытого контейнера для показа.
//
// Эмиссия — ТА ЖЕ, что у вкладки Preview и у сборки конфига
// (`config.EmitCanonicalSource` со СВОИМ пустым `tagCounts`): иначе список
// показывал бы не тот состав, из которого собирается конфиг — ровно баг #91,
// от которого лечили пул кандидатов Направлений.
func buildFolderDrillRows(sources []corestate.Source, folderID string) (folderDrillRowsInput, bool) {
	idx := folderDrillIndex(sources, folderID)
	if idx < 0 {
		return folderDrillRowsInput{}, false
	}
	src := sources[idx]
	out := folderDrillRowsInput{
		SourceIndex: idx,
		Kind:        src.Kind,
		Name:        wizardbusiness.SourceDisplayName(src),
	}
	// Для подписки без своего имени SourceDisplayName падает в URL — шапка
	// выглядела бы адресной строкой. Имя от провайдера (profile-title) —
	// то же, каким подписку зовёт заголовок окна источника.
	if src.Kind == corestate.SourceKindSubscription && src.Meta != nil {
		if t := strings.TrimSpace(src.Meta.ProfileTitle); t != "" {
			out.Name = t
		}
	}
	if len(src.Nodes) > 0 {
		emitted := config.EmitCanonicalSource(src.ToProxySourceV4(), idx, map[string]int{})
		out.Rows = buildPreviewRows(src.Nodes, emitted.Nodes)
		annotatePreviewGroupRows(out.Rows, src.Nodes, sources)
	}
	out.Identities = make([]string, len(out.Rows))
	for i := range out.Rows {
		out.Identities[i] = out.Rows[i].RawTag
	}
	return out, true
}

// folderDrillBackLabel — текст первой строки режима: вид и имя контейнера.
//
// Знак возврата — ИКОНКА слева от подписи (см. folderDrillBackGap), поэтому в
// тексте остаётся неразрывный пробел, слово вида («Folder:»/«Subscription:»)
// и имя. Вид нужен потому, что имя ПОДПИСКИ по умолчанию — её URL: без
// префикса шапка выглядела бы адресной строкой, а не «где я». Обрезается
// только ИМЯ, тем же `TruncateStringEllipsis` и той же длиной, что имена
// источников в корне: строка возврата стоит в том же списке и обязана вести
// себя как его строка, а не расширять окно длинным именем.
func folderDrillBackLabel(kind corestate.SourceKind, name string) string {
	word := locale.T("Folder")
	if kind == corestate.SourceKindSubscription {
		word = locale.T("Subscription")
	}
	n := strings.TrimSpace(name)
	if n == "" {
		return folderDrillBackGap + word
	}
	return folderDrillBackGap + word + ": " +
		wizardutils.TruncateStringEllipsis(n, wizardutils.MaxLabelRunes, "...")
}

// newFolderDrillNodeOps — контекст операций над узлом контейнера для СПИСКА.
//
// Отличается от оконного ровно двумя вещами, обе те же, что в
// source_row_node_ops.go: владелец диалогов — главное окно визарда, а
// reloadScratch/refreshPreview пусты (рабочей копии у списка нет; всё
// перерисовывает applySourceMutation).
//
// `kind` — НАСТОЯЩИЙ вид контейнера, а не «папка всегда»: на нём стоят все
// права над узлами (`nodeOpsAllowed` / `reorderAllowed`), и соврать здесь
// значило бы показать у подписки Delete, который отменит первый же fetch.
func newFolderDrillNodeOps(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	sourceIndex int,
	kind corestate.SourceKind,
) *previewNodeOps {
	return &previewNodeOps{
		presenter:   presenter,
		guiState:    guiState,
		win:         guiState.Window,
		sourceIndex: sourceIndex,
		kind:        kind,
	}
}

// folderDrillHeader — закреплённая шапка режима: «где я», сводка ошибок,
// кнопки контейнера и раскрывающийся блок объявления провайдера.
//
// Строка ОДНА и она же заголовок: отдельного «Folder: имя» над списком нет
// (заход 2, пункт 2) — две подписи об одном месте спорили друг с другом.
// Первой строкой СПИСКА она тоже больше не живёт (заход 3): уезжала за
// прокрутку вместе с составом, и «где я» пропадало с экрана.
//
// Кнопки справа — тот же набор и порядок, что у строки контейнера в списке
// Sources (принцип «меню = кнопки»): Support (иконка Telegram у t.me), Edit
// (окно источника), ↻ Fetch у подписки. Корзины здесь нет намеренно: удалять
// контейнер, стоя внутри него, — операция, за которой сразу следует «а куда
// меня выкинуло»; удаление живёт в списке Sources и в меню правого клика.
//
// Объявление провайдера — под шапкой, СВЁРНУТОЕ (кнопка 📢): оно бывает
// многострочным и в развёрнутом виде оттеснило бы состав, ради которого сюда
// заходят. Кнопки съедают свой клик сами (лежат глубже обёртки), клик по
// остальной строке — выход в корень.
func folderDrillHeader(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	src *corestate.Source,
	sourceIndex int,
	name string,
	rows []previewRow,
	warnCount int,
	announceOpen bool,
	onToggleAnnounce func(),
	onLeave func(),
) fyne.CanvasObject {
	kind := src.Kind
	lbl := widget.NewLabel(folderDrillBackLabel(kind, name))
	lbl.Wrapping = fyne.TextWrapOff
	lbl.Truncation = fyne.TextTruncateEllipsis
	lbl.TextStyle = fyne.TextStyle{Bold: true}

	meta := diagOf(src)
	announce := sourcePreviewAnnounceText(meta)

	var rowGetter func() *fynewidget.HoverRow
	var row *fynewidget.HoverRow
	rowGetter = func() *fynewidget.HoverRow { return row }

	rightItems := make([]fyne.CanvasObject, 0, 5)

	// Сводка ошибок состава — В ШАПКЕ, а не только у строк: сломанные записи
	// могут стоять глубоко в списке, и без сводки состав выглядит здоровым,
	// пока не проскроллишь до них (обкатка, заход 3).
	if warnCount > 0 {
		w := widget.NewLabel(fmt.Sprintf("%s %d %s",
			previewUnsupportedMark, warnCount, locale.T("node error(s)")))
		w.Importance = widget.WarningImportance
		rightItems = append(rightItems, w)
	}

	// 📢 — только когда объявление есть: кнопка, за которой пусто, обещала бы
	// содержимое, которого нет.
	if announce != "" {
		icon := theme.MenuExpandIcon()
		if announceOpen {
			icon = theme.MenuDropDownIcon()
		}
		btn := fynewidget.NewHoverForwardButtonWithIcon("", icon, func() {
			if onToggleAnnounce != nil {
				onToggleAnnounce()
			}
		}, rowGetter)
		btn.Importance = widget.LowImportance
		fynewidget.SetToolTipSafe(btn, locale.T("Provider message"))
		rightItems = append(rightItems, btn)
	}

	if support := supportLinkButton(meta, rowGetter); support != nil {
		rightItems = append(rightItems, support)
	}

	editBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.DocumentCreateIcon(), func() {
		presenter.MergeGUIToModel()
		m := presenter.Model()
		if m == nil || sourceIndex < 0 || sourceIndex >= len(m.Sources) {
			return
		}
		showSourceEditWindow(presenter, guiState, guiState.Window, sourceIndex,
			wizardbusiness.SourceDisplayName(m.Sources[sourceIndex]))
	}, rowGetter)
	editBtn.Importance = widget.LowImportance
	fynewidget.SetToolTipSafe(editBtn, locale.T("Edit"))
	rightItems = append(rightItems, editBtn)

	// ↻ — только у подписки: URL есть у неё, а не у папки.
	if kind == corestate.SourceKindSubscription && strings.TrimSpace(src.ID) != "" {
		id := src.ID
		fetchBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.ViewRefreshIcon(), func() {
			refreshOneSourceFromUI(presenter, guiState, id)
		}, rowGetter)
		fetchBtn.Importance = widget.LowImportance
		fynewidget.SetToolTipSafe(fetchBtn, locale.T("Fetch this subscription now"))
		rightItems = append(rightItems, fetchBtn)
	}

	// Полоса прокрутки списка рисуется ПОВЕРХ строк, поэтому её ширина
	// резервируется в самой строке — тем же ScrollGutter, что у строк корня
	// (source_tab.go). Без него скроллбар ложился на иконки справа (обкатка
	// заход 3). Кнопки пакуются вплотную, а gutter отделён обычным отступом
	// HBox — как в корне.
	rightControls := container.NewHBox(
		container.New(tightHBox{spacing: rowIconGap}, rightItems...),
		components.NewScrollGutter(),
	)

	// Слева — стрелка выхода и мастер-галка состава (когда есть что
	// переключать). Галка стоит в колонке галок строк: заголовок таблицы с
	// «выбрать всё» читается без объяснений. Свой клик она съедает сама, как
	// кнопки справа; клик по остальной строке — выход.
	leftItems := []fyne.CanvasObject{widget.NewIcon(theme.NavigateBackIcon())}
	if master := folderDrillMasterCheck(presenter, guiState, src.ID, rows); master != nil {
		leftItems = append(leftItems, master)
	}
	row = fynewidget.NewHoverRow(
		container.NewBorder(nil, nil, container.NewHBox(leftItems...), rightControls, lbl),
		fynewidget.HoverRowConfig{IsSelected: func() bool { return false }},
	)
	wrap := fynewidget.NewSecondaryTapWrap(row)
	wrap.OnPrimary = func(fyne.KeyModifier) {
		if onLeave != nil {
			onLeave()
		}
	}

	if announce == "" || !announceOpen {
		return wrap
	}
	msg := widget.NewLabel(announce)
	msg.Wrapping = fyne.TextWrapWord
	msg.Importance = widget.LowImportance
	return container.NewVBox(wrap, msg)
}

// folderDrillNodeRow — строка одного узла контейнера внутри списка Sources.
//
// Вёрстки здесь НЕТ: она одна на все списки и живёт в source_node_row.go
// (обкатка заход 3 — «шаблон строки внутри папки и на root-уровне должен быть
// один и правиться из одного места»). Эта функция только переводит узел
// контейнера в спецификацию строки: что показать, что можно нажать и какие
// права у пользователя над этим узлом.
//
// Чекбокс пишет `Enabled` НАПРЯМУЮ в модель (а не в журнал правок окна): у
// списка нет ни scratch'а, ни Save — он живёт по образцу тумблера источника,
// который тут же зовёт applySourceMutation.
func folderDrillNodeRow(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	ops *previewNodeOps,
	dragGroup *fynewidget.DragReorderGroup,
	folderID string,
	rowIndex int,
	pr previewRow,
) fyne.CanvasObject {
	identity := pr.RawTag

	spec := sourceNodeRowSpec{
		Title:        previewRowTitle(pr),
		Subtitle:     previewRowSubtitle(pr),
		SubtitleWarn: pr.Unsupported,
		Service:      pr.Service,
		ToolTip:      previewRowToolTip(pr),
		OnOpen:       func() { showPreviewNodeEditWindow(pr, identity, ops) },
		OnMenu: func(pe *fyne.PointEvent) {
			showPreviewRowContextMenu(guiState.Window, pr, identity, ops, pe)
		},
	}

	// Галка. Неразобранная запись включению не подлежит (собирать из неё
	// нечего); узел без идентичности пометить некуда — отметка поехала бы на
	// соседа при следующем обновлении. Оба исхода те же, что в списке Preview.
	if pr.Unsupported || identity == "" {
		spec.Checked = !pr.Unsupported
		spec.CheckDisabled = true
		spec.Dimmed = true
	} else {
		enabled := folderDrillNodeEnabled(presenter, folderID, identity)
		spec.Checked = enabled
		spec.Dimmed = !enabled
		spec.OnCheckChanged = func(on bool) {
			if !folderDrillSetNodeEnabled(presenter, folderID, identity, on) {
				return
			}
			applySourceMutation(presenter, guiState)
		}
	}

	// Захват — только там, где порядок принадлежит пользователю (папка): у
	// подписки его задаёт тело провайдера, и перестановка потерялась бы на
	// первом же обновлении. nil → шаблон ставит распорку той же ширины.
	//
	// Корзина — только у узла ПАПКИ: в подписке у строки остаётся одна галка
	// (обкатка заход 3), состав там принадлежит провайдеру. Карандаша нет ни
	// у кого — окно открывает клик по строке, как в корне.
	if ops != nil && ops.nodeOpsAllowed() && identity != "" {
		spec.OnDelete = func() { ops.showDeleteDialog(identity) }
	}

	var wrap fyne.CanvasObject
	var row *fynewidget.HoverRow
	if ops != nil && ops.reorderAllowed() {
		// Захват держит ссылку на строку (проброс hover), а строка ещё не
		// собрана — отсюда отложенный getter.
		var built *fynewidget.HoverRow
		spec.Drag = fynewidget.NewDragHandle(dragGroup, rowIndex,
			func() *fynewidget.HoverRow { return built })
		wrap, row = newSourceNodeRow(spec)
		built = row
	} else {
		wrap, row = newSourceNodeRow(spec)
	}
	_ = row
	return wrap
}

// applyFolderDrillChrome переключает ОБВЯЗКУ вкладки между корнем и
// контейнером.
//
// Виджеты те же — меняются только тексты, доступность поля Add и видимость
// кнопки: требование звучало «не меняя интерфейса», и второй шапки под режим
// контейнера волна не заводит.
//
// Заголовок списка меняет вызывающий (заход 3): в контейнере на его месте
// стоит закреплённая шапка `folderDrillHeader`, она же выход.
//
//   - подсказка ПОЛЯ ввода (тултип, заход 3 — под полем она занимала три
//     строки высоты постоянно): про подписки и ссылки ⇄ про узлы папки ⇄ про
//     несвободу узлов подписки;
//   - поле Add и кнопка «Add»: в режиме ПОДПИСКИ выключены — руками в подписку
//     не льют (следующий fetch унёс бы добавленное);
//   - «Preview all servers…» гаснет: она про ВСЕ источники сразу, а внутри
//     контейнера открывала бы окно не про то, на что смотрит пользователь.
func applyFolderDrillChrome(
	hintTarget fyne.CanvasObject,
	previewAllBtn *widget.Button,
	addEntry *widget.Entry,
	addBtn *widget.Button,
	overflowBtn *widget.Button,
	kind corestate.SourceKind,
	inContainer bool,
) {
	setHint := func(text string) {
		if hintTarget != nil {
			fynewidget.SetToolTipSafe(hintTarget, locale.T(text))
		}
	}
	setAdd := func(enabled bool) {
		if addEntry != nil {
			if enabled {
				addEntry.Enable()
			} else {
				addEntry.Disable()
			}
		}
		for _, b := range []*widget.Button{addBtn, overflowBtn} {
			if b == nil {
				continue
			}
			if enabled {
				b.Enable()
			} else {
				b.Disable()
			}
		}
	}
	if !inContainer {
		setHint(sourceHintText)
		if previewAllBtn != nil {
			previewAllBtn.Show()
		}
		setAdd(true)
		return
	}
	if previewAllBtn != nil {
		previewAllBtn.Hide()
	}
	if kind == corestate.SourceKindSubscription {
		setHint(subDrillHintText)
		setAdd(false)
		return
	}
	setHint(folderDrillHintText)
	setAdd(true)
}

// renderFolderDrillRows наполняет список вкладки составом открытого контейнера.
//
// Возвращает вид контейнера и признак «он ещё существует». false = контейнера
// в модели уже нет: вызывающий тогда сам возвращает вкладку в корень.
//
// Строка возврата в СПИСКЕ больше не живёт (обкатка заход 3): она уезжала за
// прокрутку вместе с составом, и «где я» пропадало с экрана. Теперь выход —
// закреплённая шапка секции (folderDrillHeader вместо заголовка «Sources»),
// её строит вызывающий из возвращённого input. Здесь — только строки узлов;
// захват перетаскивания регистрируется по индексу строки узла как и раньше.
//
// На большом составе строки кладутся ОКНОМ (см. раздел шапки «Окно строк на
// больших контейнерах»): окно считается от ТЕКУЩЕГО смещения прокрутки, а не
// всегда от начала, — иначе после снятой галки список прыгал бы в начало.
func renderFolderDrillRows(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	drill *folderDrillState,
	dragGroup *fynewidget.DragReorderGroup,
	sourcesBox *fyne.Container,
	sourcesScroll *container.Scroll,
	reorder *func(from, to int),
	refresh func(),
) (folderDrillRowsInput, bool) {
	m := presenter.Model()
	if m == nil {
		return folderDrillRowsInput{}, false
	}
	input, ok := drill.rowsFor(m.Sources, drill.folderID, m.Revision, m.NodePoolGeneration)
	if !ok {
		return folderDrillRowsInput{}, false
	}

	ops := newFolderDrillNodeOps(presenter, guiState, input.SourceIndex, input.Kind)
	// Перестановка адресует узлы СЫРЫМИ тегами, а не индексами в nodes[].
	// Порядок и длина списка сегодня повторяют `src.Nodes` (buildPreviewRows
	// даёт строку на каждый элемент состава, включая выключенные и
	// неразобранные), так что резолв по тегу — не обход расхождения, а
	// страховка от него: ровно тот же довод, что у applyReorder на вкладке
	// Preview.
	identities := input.Identities
	if reorder != nil {
		if ops.reorderAllowed() {
			*reorder = func(from, to int) {
				ops.applyReorder(identities, from, to)
			}
		} else {
			// Порядок узлов подписки задаёт провайдер: бросок обязан быть
			// холостым, а не переставить то, что вернётся обратно.
			*reorder = nil
		}
	}

	if len(input.Rows) == 0 {
		// Пустой контейнер — законное состояние (папку только что создали,
		// подписку ещё не обновляли): говорим об этом словами; выход живёт
		// в закреплённой шапке, тупика нет.
		text := locale.T("Folder is empty — paste links above to add nodes.")
		if input.Kind == corestate.SourceKindSubscription {
			text = locale.T("No nodes yet — refresh this subscription in the sources list.")
		}
		hint := widget.NewLabel(text)
		hint.Wrapping = fyne.TextWrapWord
		hint.Importance = widget.LowImportance
		sourcesBox.Add(hint)
		drill.winStart, drill.winEnd, drill.winTotal = 0, 0, 0
		drill.winActive = false
		// Пустой контейнер — не виртуальный список: Total обязан вернуться в 0
		// (группа общая с корнем, см. ниже).
		dragGroup.Total = 0
		return input, true
	}

	total := len(input.Rows)
	// Свежий вход в контейнер: окна ещё нет (invalidateCache обнулил его в
	// enter/leave), а смещение прокрутки на этот момент ещё от ПРОШЛОГО
	// списка — resetScrollOnModeChange обнулит его сразу после нас. Читать
	// его здесь значило бы построить окно вокруг чужой позиции; строим от
	// начала, куда список и встанет.
	fresh := drill.winTotal == 0
	drill.winTotal = total
	drill.winActive = total > folderDrillWindowThreshold

	start, end := 0, total
	if drill.winActive {
		first, last := 0, 0
		if !fresh {
			// Окно считается от ТЕКУЩЕГО смещения: полная перерисовка после
			// мутации (снятая галка, бросок) обязана оставить человека там же,
			// где он стоял, а не увезти в начало списка.
			first, last = folderDrillVisibleRowsForScroll(sourcesScroll, drill.rowHeight)
		}
		start, end = folderDrillWindowRange(total, first, last, folderDrillWindowMargin)
	}
	fillFolderDrillWindow(presenter, guiState, drill, ops, dragGroup, sourcesBox, input, start, end)
	return input, true
}

// folderDrillVisibleRowsForScroll — видимый диапазон строк по состоянию
// прокрутки.
//
// Вынесено из renderFolderDrillRows, потому что то же самое нужно хуку
// прокрутки. Пока строка не измерена (rowHeight == 0) отдаёт первый экран:
// до первой раскладки честного ответа нет, а окно от начала — ровно то, что
// нужно показать первым кадром.
func folderDrillVisibleRowsForScroll(sourcesScroll *container.Scroll, rowHeight float32) (int, int) {
	if sourcesScroll == nil || rowHeight <= 0 {
		return 0, 0
	}
	return folderDrillVisibleRows(
		sourcesScroll.Offset.Y, sourcesScroll.Size().Height, rowHeight+theme.Padding())
}

// fillFolderDrillWindow кладёт в список распорки и строки окна [start, end).
//
// ОДНА функция и для полной отрисовки, и для сдвига окна по прокрутке: две
// реализации одного раскладки разъехались бы на первой же правке шаблона
// строки, а расхождение здесь видно не сразу — списком, который «немного
// прыгает».
//
// Вызывающий обязан очистить sourcesBox и сбросить группу перетаскивания до
// вызова: строки регистрируются под своими АБСОЛЮТНЫМИ индексами, и записи
// прошлого окна иначе заявляли бы полосы уже мёртвых виджетов.
func fillFolderDrillWindow(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	drill *folderDrillState,
	ops *previewNodeOps,
	dragGroup *fynewidget.DragReorderGroup,
	sourcesBox *fyne.Container,
	input folderDrillRowsInput,
	start, end int,
) {
	total := len(input.Rows)
	if start < 0 {
		start = 0
	}
	if end > total {
		end = total
	}
	if end < start {
		end = start
	}
	drill.winStart, drill.winEnd = start, end
	// Total — в обе стороны и на КАЖДОЙ перерисовке: группа общая с корневым
	// списком, и значение от контейнера иначе пережило бы выход в корень.
	if drill.winActive {
		dragGroup.Total = total
	} else {
		dragGroup.Total = 0
	}

	// Строки строятся ДО распорок: высота строки меряется у первой
	// построенной, а от неё зависят высоты распорок. Иначе на первом входе
	// в контейнер (высота ещё не известна) нижняя распорка не ставилась бы,
	// и полоса прокрутки показывала бы список из сотни строк вместо
	// полутысячи, пока пользователь не докрутит до края окна.
	reorderable := ops.reorderAllowed()
	rows := make([]fyne.CanvasObject, 0, end-start)
	for i := start; i < end; i++ {
		rowObj := folderDrillNodeRow(
			presenter, guiState, ops, dragGroup, drill.folderID, i, input.Rows[i])
		if reorderable {
			// Индекс АБСОЛЮТНЫЙ — номер узла в контейнере, а не в окне:
			// точка вставки и applyReorder адресуют полный состав.
			dragGroup.Register(i, rowObj)
		}
		rows = append(rows, rowObj)
		if i == start {
			if h := rowObj.MinSize().Height; h > 0 {
				drill.rowHeight = h
			}
		}
	}

	topH, bottomH := float32(0), float32(0)
	if drill.winActive && drill.rowHeight > 0 {
		topH, bottomH = folderDrillSpacerHeights(
			start, end, total, drill.rowHeight, theme.Padding())
	}
	if topH > 0 {
		sourcesBox.Add(folderDrillSpacer(topH))
	}
	for _, rowObj := range rows {
		sourcesBox.Add(rowObj)
	}
	if bottomH > 0 {
		sourcesBox.Add(folderDrillSpacer(bottomH))
	}
}

// folderDrillSpacer — прозрачная распорка на месте непостроенных строк.
//
// Прямоугольник, а не layout.NewSpacer(): спейсер VBox делит между собой
// ОСТАТОК высоты, а нам нужна ровно заданная (см. арифметику в
// folderDrillSpacerHeights). Ширина нулевая намеренно — распорка не должна
// расширять список.
func folderDrillSpacer(h float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(0, h))
	return r
}

// folderDrillScrollHook — сдвиг окна строк вслед за прокруткой.
//
// Возвращает функцию для `container.Scroll.OnScrolled`; nil-состояние и режим
// корня она отрабатывает молча. Перестройка меняет ТОЛЬКО строки и распорки в
// sourcesBox — шапка секции живёт вне списка (sourcesTitleSwap) и не трогается,
// поэтому «где я» не мигает.
//
// Смещение прокрутки не трогаем: распорки держат общую высоту содержимого
// неизменной (тест TestFolderDrillSpacerHeightsMatchFullRender), и текущая
// позиция остаётся валидной.
func folderDrillScrollHook(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	drill *folderDrillState,
	dragGroup *fynewidget.DragReorderGroup,
	sourcesBox *fyne.Container,
	sourcesScroll *container.Scroll,
) func(fyne.Position) {
	return func(fyne.Position) {
		if drill == nil || !drill.active() || !drill.winActive || drill.rebuilding {
			return
		}
		// Пока тащат строку — не трогаем список: autoScroll сам крутит
		// прокрутку во время броска, и перестройка выдернула бы из-под пальца
		// и строку, и полосы, по которым считается точка вставки.
		if dragGroup.Dragging() {
			return
		}
		if presenter == nil || sourcesBox == nil || sourcesScroll == nil {
			return
		}
		m := presenter.Model()
		if m == nil {
			return
		}
		input, ok := drill.rowsFor(m.Sources, drill.folderID, m.Revision, m.NodePoolGeneration)
		if !ok || len(input.Rows) == 0 {
			return
		}
		total := len(input.Rows)
		first, last := folderDrillVisibleRowsForScroll(sourcesScroll, drill.rowHeight)
		if !folderDrillWindowNeedsShift(
			drill.winStart, drill.winEnd, total, first, last, folderDrillWindowSlack) {
			return
		}
		start, end := folderDrillWindowRange(total, first, last, folderDrillWindowMargin)
		if start == drill.winStart && end == drill.winEnd {
			return
		}

		drill.rebuilding = true
		defer func() { drill.rebuilding = false }()

		ops := newFolderDrillNodeOps(presenter, guiState, input.SourceIndex, input.Kind)
		// Группа держит полосы ПОСТРОЕННЫХ строк: после подмены окна прежние
		// записи указывают на виджеты, которых в списке уже нет.
		dragGroup.Reset()
		sourcesBox.Objects = sourcesBox.Objects[:0]
		fillFolderDrillWindow(
			presenter, guiState, drill, ops, dragGroup, sourcesBox, input, start, end)
		sourcesBox.Refresh()
	}
}

// applyAddedSourcesToFolderNamed — путь Add в режиме папки, с ИМЕНЕМ для
// безымянных узлов.
//
// Разбор ТОТ ЖЕ (`business.AppendNodesToFolder` → `parseSourceInput`), адрес
// назначения другой. Подписка узлом не становится: её отвергает то же ядро
// сентинелом `ErrSubscriptionInFolder`, и текст ошибки берётся тот же, что у
// кнопки «Add nodes…» в окне папки (folderAddNodesSubscriptionText) — двух
// формулировок одного отказа не заводим.
//
// `defaultTag` нужен ровно там же, где он нужен окну папки: имя файла у
// импорта и поле тега у формы сервера. У ссылки со своим `#fragment` он не
// применяется — правило одно и живёт в ядре (AppendNodesToFolder), здесь
// только адрес.
//
// Возвращает true, когда узлы легли: только тогда поле ввода очищается —
// отвергнутый текст обязан остаться на экране, иначе человек потеряет то, что
// вставил, вместе с сообщением об ошибке.
func applyAddedSourcesToFolderNamed(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	folderID, text, defaultTag string,
) bool {
	body := strings.TrimSpace(text)
	if body == "" {
		return false
	}
	m := presenter.Model()
	if m == nil {
		return false
	}
	res, err := wizardbusiness.AppendNodesToFolder(m, folderID, body, strings.TrimSpace(defaultTag))
	if err != nil {
		if errors.Is(err, wizardbusiness.ErrSubscriptionInFolder) {
			err = fmt.Errorf("%s", locale.T(folderAddNodesSubscriptionText))
		}
		dialog.ShowError(err, guiState.Window)
		return false
	}
	if res.Added == 0 {
		if res.SkippedSubscriptions > 0 {
			dialog.ShowError(
				fmt.Errorf("%s", locale.T(folderAddNodesSubscriptionText)), guiState.Window)
			return false
		}
		// Нажатие без результата — тоже результат, о нём говорят.
		dialogs.ShowAutoHideInfo(fyne.CurrentApp(), guiState.Window,
			locale.T("Nothing added"), locale.T(addNothingAddedText))
		return false
	}
	applySourceMutation(presenter, guiState)
	if res.SkippedSubscriptions > 0 {
		// Смешанный вход: узлы легли, подписочная строка — нет. Молчать про
		// неё нельзя, но и ронять операцию незачем.
		dialogs.ShowAutoHideInfo(fyne.CurrentApp(), guiState.Window,
			locale.T("Nodes added"),
			locale.T(folderAddNodesSubscriptionText))
		return true
	}
	dialogs.ShowAutoHideInfo(fyne.CurrentApp(), guiState.Window,
		locale.T("Nodes added"),
		locale.Tf("%d node(s) added to the folder.", res.Added))
	return true
}

// folderDrillNodeEnabled — включён ли узел контейнера (по сырому тегу).
func folderDrillNodeEnabled(
	presenter *wizardpresentation.WizardPresenter,
	folderID, rawTag string,
) bool {
	m := presenter.Model()
	if m == nil {
		return false
	}
	idx := folderDrillIndex(m.Sources, folderID)
	if idx < 0 {
		return false
	}
	nodes := m.Sources[idx].Nodes
	for i := range nodes {
		if nodes[i].Tag == rawTag {
			return nodes[i].Enabled
		}
	}
	return false
}

// folderDrillSetNodeEnabled ставит отметку включённости узла контейнера.
//
// Возвращает false, когда ставить некуда (контейнер или узел исчезли, пока
// висел список) — вызывающий тогда не гоняет побочки мутации впустую.
func folderDrillSetNodeEnabled(
	presenter *wizardpresentation.WizardPresenter,
	folderID, rawTag string,
	enabled bool,
) bool {
	m := presenter.Model()
	if m == nil {
		return false
	}
	idx := folderDrillIndex(m.Sources, folderID)
	if idx < 0 {
		return false
	}
	nodes := m.Sources[idx].Nodes
	for i := range nodes {
		if nodes[i].Tag == rawTag {
			if nodes[i].Enabled == enabled {
				return false
			}
			nodes[i].Enabled = enabled
			return true
		}
	}
	return false
}

// folderDrillMasterCheck — галка «все вкл / все выкл» в шапке контейнера.
//
// Считает только те записи, у которых есть своя галка в строке (разобранные
// и с идентичностью): неразобранным включаться не во что, а узел без тега
// пометить некуда. Три состояния: все включены — отмечена, никто — пустая,
// часть — Partial. Клик по Partial включает всех (Fyne снимает Partial в
// сторону «отмечена»), клик по отмеченной — выключает всех: ровно как у
// заголовка таблицы. Возвращает nil, когда переключать нечего — пустая
// галка над пустым составом обещала бы действие без результата.
//
// Мутация одна на весь состав, а не по узлу: applySourceMutation поднимает
// ревизию и пересобирает производные, и сто подъёмов подряд на один клик
// означали бы сто пересборок пула кандидатов.
func folderDrillMasterCheck(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	folderID string,
	rows []previewRow,
) fyne.CanvasObject {
	tags := make([]string, 0, len(rows))
	enabled := 0
	for _, pr := range rows {
		if pr.Unsupported || pr.RawTag == "" {
			continue
		}
		tags = append(tags, pr.RawTag)
		if folderDrillNodeEnabled(presenter, folderID, pr.RawTag) {
			enabled++
		}
	}
	if len(tags) == 0 {
		return nil
	}
	check := widget.NewCheck("", nil)
	switch {
	case enabled == len(tags):
		check.Checked = true
		fynewidget.SetToolTipSafe(check, locale.T("Disable all nodes"))
	case enabled == 0:
		fynewidget.SetToolTipSafe(check, locale.T("Enable all nodes"))
	default:
		check.Partial = true
		fynewidget.SetToolTipSafe(check, locale.T("Enable all nodes"))
	}
	check.OnChanged = func(on bool) {
		if !folderDrillSetNodesEnabled(presenter, folderID, tags, on) {
			return
		}
		applySourceMutation(presenter, guiState)
	}
	return check
}

// folderDrillSetNodesEnabled ставит одну отметку сразу нескольким узлам
// контейнера. Возвращает true, если хоть один узел сменил состояние.
func folderDrillSetNodesEnabled(
	presenter *wizardpresentation.WizardPresenter,
	folderID string,
	tags []string,
	enabled bool,
) bool {
	m := presenter.Model()
	if m == nil {
		return false
	}
	idx := folderDrillIndex(m.Sources, folderID)
	if idx < 0 {
		return false
	}
	want := make(map[string]bool, len(tags))
	for _, t := range tags {
		want[t] = true
	}
	changed := false
	nodes := m.Sources[idx].Nodes
	for i := range nodes {
		if !want[nodes[i].Tag] || nodes[i].Enabled == enabled {
			continue
		}
		nodes[i].Enabled = enabled
		changed = true
	}
	return changed
}
