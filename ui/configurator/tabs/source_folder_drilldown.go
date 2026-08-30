// File source_folder_drilldown.go — вход В ПАПКУ прямо в списке Sources
// (SPEC 116 W13, обкатка; требование дословно: «нажать на папку и в том же
// окне, не меняя интерфейса, провалиться в папку»).
//
// # Что это такое и чем НЕ является
//
// Это не второй список узлов и не новое окно. Вкладка Sources получает одно
// состояние — ULID открытой папки (`openFolderID` в замыкании
// `CreateSourcesTab`), — и ТА ЖЕ функция `refreshSourcesList` наполняет ТОТ ЖЕ
// `sourcesBox` либо корнем, либо составом папки. Виджеты те же: `HoverRow` +
// `SecondaryTapWrap` снаружи (та же ловушка глубины Fyne, что у W13-меню
// верхнего узла), `DragHandle` из той же `DragReorderGroup`, `widget.Check`.
//
// Второго списка узлов заводить нельзя по той же причине, по которой W5 не
// стала заводить его в окне источника (§O4 = вариант А): состав контейнера
// один, и вторая его модель разъехалась бы с первой. Поэтому строки папки
// строятся ТЕМ ЖЕ `buildPreviewRows` (preview_rows.go) и той же эмиссией
// `config.EmitCanonicalSource`, что вкладка Preview, а рисуются
// `previewRowTitle`/`previewRowSubtitle` — теми же текстами.
//
// # Почему операции — те же самые
//
// Правый клик по узлу папки в этом режиме зовёт `showPreviewRowContextMenu` с
// контекстом `previewNodeOps`, у которого `win` = главное окно визарда, а
// `reloadScratch`/`refreshPreview` пусты — рабочей копии у списка нет, список
// целиком перестраивает `applySourceMutation` (ровно как в
// source_row_node_ops.go). Move/Copy/Rename/Delete и реестр переписи ссылок
// берутся целиком; вторых диалогов волна не заводит.
//
// # Почему Add-поле в режиме папки льёт в папку
//
// Поле ввода наверху вкладки — единственный Add, который видит пользователь,
// и в режиме папки «добавить» может означать только «добавить сюда». Разбор
// текста при этом ТОТ ЖЕ (`parseSourceInput` через
// `business.AppendNodesToFolder`, W6): корень и папка отличаются адресом
// назначения, а не разбором (ловушка «эмиттер и парсер ходят парой», дыра Д6).
//
// # Чего в режиме папки нет
//
// «Preview all servers…» остаётся кнопкой КОРНЯ (она про все источники
// сразу), а кнопка ↻ у узла папки бессмысленна: URL есть у подписки, а не у
// узла. Порядок узлов таскается внутри папки (`applyReorder`, W5), выход
// возвращает корень ровно таким, каким он был: порядок `m.Sources` режим не
// трогает вовсе.
package tabs

import (
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
	wizardutils "singbox-launcher/ui/configurator/utils"
)

// folderDrillBackMark — знак возврата в строке «← имя папки».
//
// «←» U+2190 уже живёт в шрифте и в проекте (новых глифов волна не заводит —
// правило ui-visuals-approve-first). Стрелка стоит ПЕРЕД именем: строка
// читается как действие «выйти отсюда», а не как заголовок.
const folderDrillBackMark = "←"

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	// folderDrillHintText — подсказка под полем ввода в режиме папки.
	//
	// Отличается от корневой ровно тем, чем отличается адрес: подписка в папку
	// не кладётся (вложенных контейнеров нет), и перечислять её среди
	// принимаемого значило бы обещать то, что ядро отвергнет.
	folderDrillHintText = "Adds nodes to this folder: direct links (vless://, vmess://, trojan://, ss://, hysteria2://, ssh://, wireguard://) or sing-box JSON. One per line. A subscription URL goes to Sources, not into a folder."
)

// folderDrillState — состояние вкладки «мы внутри папки».
//
// ULID, а не индекс: пока пользователь смотрит состав, порядок `m.Sources`
// вправе поменяться (фоновый fetch, перетаскивание из второго окна, удаление
// соседа), и индекс увёл бы список в чужой источник. ULID — единственная
// идентификация папки (SPEC 118).
type folderDrillState struct {
	// folderID — ULID открытой папки; пусто = режим корня.
	folderID string
}

// active — вкладка сейчас показывает состав папки.
func (d *folderDrillState) active() bool {
	return d != nil && strings.TrimSpace(d.folderID) != ""
}

// enter открывает папку, leave возвращает корень. Перерисовку зовёт
// вызывающий: обе операции меняют ровно одно поле, и лишний refresh при
// повторном клике по той же строке был бы мельканием списка.
func (d *folderDrillState) enter(folderID string) { d.folderID = strings.TrimSpace(folderID) }
func (d *folderDrillState) leave()                { d.folderID = "" }

// folderDrillIndex — позиция открытой папки в m.Sources; -1, если её не стало.
//
// Пропавшая папка (удалили из второго окна, откатили импортом бэкапа) режим не
// ломает: список сам возвращается в корень — показывать состав того, чего
// больше нет, не из чего.
func folderDrillIndex(sources []corestate.Source, folderID string) int {
	id := strings.TrimSpace(folderID)
	if id == "" {
		return -1
	}
	for i := range sources {
		if sources[i].ID == id && sources[i].Kind == corestate.SourceKindFolder {
			return i
		}
	}
	return -1
}

// folderDrillRowsInput — всё, что нужно строкам режима папки.
//
// Собирается ОДНИМ проходом на перерисовку: эмиссия папки стоит денег
// (тег-машина + уникализация), и звать её по разу на строку значило бы
// пересобирать состав N раз на один показ.
type folderDrillRowsInput struct {
	// SourceIndex — позиция папки на момент сборки строк.
	SourceIndex int
	// Rows — состав папки строками (та же модель, что у вкладки Preview).
	Rows []previewRow
	// Identities — сырые теги строк: адрес всех операций над узлом.
	Identities []string
	// Name — как папку зовут пользователю (для строки возврата).
	Name string
}

// buildFolderDrillRows собирает состав открытой папки для показа в списке.
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
		Name:        wizardbusiness.SourceDisplayName(src),
	}
	if len(src.Nodes) > 0 {
		emitted := config.EmitCanonicalSource(src.ToProxySourceV4(), idx, map[string]int{})
		out.Rows = buildPreviewRows(src.Nodes, emitted.Nodes)
	}
	out.Identities = make([]string, len(out.Rows))
	for i := range out.Rows {
		out.Identities[i] = out.Rows[i].RawTag
	}
	return out, true
}

// folderDrillBackLabel — текст первой строки режима: «← имя папки».
//
// Имя обрезается тем же `TruncateStringEllipsis` и той же длиной, что имена
// источников в корне: строка возврата стоит в том же списке и обязана вести
// себя как его строка, а не расширять окно длинным именем.
func folderDrillBackLabel(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		n = locale.T("Folder")
	}
	return folderDrillBackMark + " " + wizardutils.TruncateStringEllipsis(n, wizardutils.MaxLabelRunes, "...")
}

// newFolderDrillNodeOps — контекст операций над узлом папки для СПИСКА.
//
// Отличается от оконного ровно двумя вещами, обе те же, что в
// source_row_node_ops.go: владелец диалогов — главное окно визарда, а
// reloadScratch/refreshPreview пусты (рабочей копии у списка нет; всё
// перерисовывает applySourceMutation).
func newFolderDrillNodeOps(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	sourceIndex int,
) *previewNodeOps {
	return &previewNodeOps{
		presenter:   presenter,
		guiState:    guiState,
		win:         guiState.Window,
		sourceIndex: sourceIndex,
		kind:        corestate.SourceKindFolder,
	}
}

// folderDrillBackRow — первая строка режима: выход в корень.
//
// Тот же `HoverRow`, что у строк источников (подсветка наведения одна на весь
// список), и тот же клик-обработчик через `SecondaryTapWrap.OnPrimary` —
// новых кликабельных виджетов волна не заводит. Правого клика у неё нет:
// строка возврата не узел, операций над ней не бывает.
func folderDrillBackRow(name string, onLeave func()) fyne.CanvasObject {
	lbl := widget.NewLabel(folderDrillBackLabel(name))
	lbl.Wrapping = fyne.TextWrapOff
	lbl.Truncation = fyne.TextTruncateEllipsis
	lbl.TextStyle = fyne.TextStyle{Bold: true}

	row := fynewidget.NewHoverRow(
		container.NewBorder(nil, nil, widget.NewIcon(theme.NavigateBackIcon()), nil, lbl),
		fynewidget.HoverRowConfig{IsSelected: func() bool { return false }},
	)
	wrap := fynewidget.NewSecondaryTapWrap(row)
	wrap.OnPrimary = func(fyne.KeyModifier) {
		if onLeave != nil {
			onLeave()
		}
	}
	return wrap
}

// folderDrillNodeRow — строка одного узла папки внутри списка Sources.
//
// Состав строки — тот же, что на вкладке Preview: [захват][галка] имя /
// подстрока. Чекбокс пишет `Enabled` НАПРЯМУЮ в модель (а не в журнал правок
// окна): у списка нет ни scratch'а, ни Save — он живёт по образцу тумблера
// источника, который тут же зовёт applySourceMutation.
//
// Правый клик — `showPreviewRowContextMenu`: те же Move/Copy/Rename/Delete и
// «Node info…», что в Preview, включая ветку неразобранной записи.
func folderDrillNodeRow(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	ops *previewNodeOps,
	dragGroup *fynewidget.DragReorderGroup,
	folderID string,
	rowIndex int,
	pr previewRow,
) fyne.CanvasObject {
	var row *fynewidget.HoverRow
	rowGetter := func() *fynewidget.HoverRow { return row }

	name := widget.NewLabel(previewRowTitle(pr))
	name.Wrapping = fyne.TextWrapOff
	name.Truncation = fyne.TextTruncateEllipsis

	sub := widget.NewLabel(previewRowSubtitle(pr))
	sub.Wrapping = fyne.TextWrapOff
	sub.Truncation = fyne.TextTruncateEllipsis
	sub.TextStyle = fyne.TextStyle{Italic: true}
	if pr.Unsupported {
		sub.Importance = widget.WarningImportance
	} else {
		sub.Importance = widget.LowImportance
	}

	check := widget.NewCheck("", nil)
	identity := pr.RawTag
	if pr.Unsupported || identity == "" {
		// Неразобранная запись включению не подлежит (собирать из неё нечего) —
		// галка пустая; узел без идентичности пометить некуда (отметка поехала
		// бы на соседа при следующем обновлении) — галка стоит, но не
		// нажимается. Обещать включение, которого модель не допускает, нельзя —
		// та же развилка и те же два исхода, что в списке Preview.
		check.SetChecked(!pr.Unsupported)
		check.Disable()
		name.Importance = widget.LowImportance
	} else {
		enabled := folderDrillNodeEnabled(presenter, folderID, identity)
		check.SetChecked(enabled)
		if !enabled {
			name.Importance = widget.LowImportance
		}
		check.OnChanged = func(on bool) {
			if !folderDrillSetNodeEnabled(presenter, folderID, identity, on) {
				return
			}
			applySourceMutation(presenter, guiState)
		}
	}

	// Ведущий кластер — как у строки источника: захват ЛЕВЕЕ галки. Захват
	// есть всегда (порядок узлов папки принадлежит пользователю, П5); у
	// неразобранной записи он тоже есть — её позиция в составе такая же
	// пользовательская, как у остальных.
	grip := fynewidget.NewDragHandle(dragGroup, rowIndex, rowGetter)
	lead := container.NewHBox(grip, fynewidget.CheckLeadingWrap(check))

	titleBox := container.New(tightVBox{}, name, sub)
	row = fynewidget.NewHoverRow(
		container.NewBorder(nil, nil, lead, nil, titleBox),
		fynewidget.HoverRowConfig{IsSelected: func() bool { return false }},
	)

	// Обёртка СНАРУЖИ HoverRow — та же ловушка Fyne, что у меню верхнего узла
	// (FindObjectAtPositionMatching отдаёт событие самому глубокому
	// подходящему объекту): внутри обёртка перехватила бы hover и погасила
	// подсветку строки.
	wrap := fynewidget.NewSecondaryTapWrap(row)
	wrap.SetToolTip(previewRowToolTip(pr))
	wrap.OnPrimary = func(fyne.KeyModifier) {
		showPreviewRowInfoWindow(pr)
	}
	wrap.OnSecondary = func(pe *fyne.PointEvent) {
		showPreviewRowContextMenu(guiState.Window, pr, identity, ops, pe)
	}
	return wrap
}

// applyFolderDrillChrome переключает ОБВЯЗКУ вкладки между корнем и папкой.
//
// Виджеты те же — меняются только их тексты и видимость кнопки: требование
// звучало «не меняя интерфейса», и второй шапки под режим папки волна не
// заводит. Пустое `folderName` = вернуть корневой вид.
//
//   - заголовок списка: «Sources» ⇄ имя папки (человек обязан видеть, ГДЕ он);
//   - подсказка под полем ввода: про подписки и ссылки ⇄ про узлы папки —
//     подписку в папку не кладут, и обещать её значило бы врать;
//   - «Preview all servers…» гаснет: она про ВСЕ источники сразу, а внутри
//     папки открывала бы окно не про то, на что смотрит пользователь.
func applyFolderDrillChrome(
	sourcesLabel *widget.Label,
	hintLabel *widget.Label,
	previewAllBtn *widget.Button,
	folderName string,
) {
	name := strings.TrimSpace(folderName)
	if name == "" {
		if sourcesLabel != nil {
			sourcesLabel.SetText(locale.T("Sources"))
		}
		if hintLabel != nil {
			hintLabel.SetText(locale.T(sourceHintText))
		}
		if previewAllBtn != nil {
			previewAllBtn.Show()
		}
		return
	}
	if sourcesLabel != nil {
		sourcesLabel.SetText(locale.Tf("Folder: %s",
			wizardutils.TruncateStringEllipsis(name, wizardutils.MaxLabelRunes, "...")))
	}
	if hintLabel != nil {
		hintLabel.SetText(locale.T(folderDrillHintText))
	}
	if previewAllBtn != nil {
		previewAllBtn.Hide()
	}
}

// renderFolderDrillRows наполняет список вкладки составом открытой папки.
//
// Возвращает имя папки и признак «папка ещё существует». false = папки в
// модели уже нет: вызывающий тогда сам возвращает вкладку в корень. Своего
// «а покажем пусто» здесь нет — пустой список без строки возврата был бы
// тупиком.
//
// Порядок строк: сначала «← имя папки», затем узлы в порядке `nodes[]`. Захват
// перетаскивания регистрируется по индексу СТРОКИ УЗЛА (без строки возврата) —
// иначе бросок уехал бы на одну позицию, а сама строка возврата стала бы
// слотом, в который можно бросить узел.
func renderFolderDrillRows(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	drill *folderDrillState,
	dragGroup *fynewidget.DragReorderGroup,
	sourcesBox *fyne.Container,
	reorder *func(from, to int),
	refresh func(),
) (string, bool) {
	m := presenter.Model()
	if m == nil {
		return "", false
	}
	input, ok := buildFolderDrillRows(m.Sources, drill.folderID)
	if !ok {
		return "", false
	}

	ops := newFolderDrillNodeOps(presenter, guiState, input.SourceIndex)
	// Перестановка адресует узлы СЫРЫМИ тегами, а не индексами в nodes[]:
	// список показывает состав, из которого эмиссия могла что-то не выпустить
	// (выключенный, неразобранный), и индексы совпадать не обязаны — ровно тот
	// же довод, что у applyReorder на вкладке Preview.
	identities := input.Identities
	if reorder != nil {
		*reorder = func(from, to int) {
			ops.applyReorder(identities, from, to)
		}
	}

	sourcesBox.Add(folderDrillBackRow(input.Name, func() {
		drill.leave()
		if reorder != nil {
			*reorder = nil
		}
		if refresh != nil {
			refresh()
		}
	}))

	if len(input.Rows) == 0 {
		// Пустая папка — законное состояние (её только что создали): говорим
		// об этом словами и оставляем строку возврата на месте.
		hint := widget.NewLabel(locale.T("Folder is empty — paste links above to add nodes."))
		hint.Wrapping = fyne.TextWrapWord
		hint.Importance = widget.LowImportance
		sourcesBox.Add(hint)
		return input.Name, true
	}

	// Total намеренно НЕ ставится: строки папки живут в том же VBox, что и
	// строки источников, и регистрируются все до одной (виртуализации тут нет).
	// Total нужен только виртуализированному widget.List, а оставшись
	// ненулевым, он разрешил бы бросок в слот, которого в корне уже нет.
	for i := range input.Rows {
		rowObj := folderDrillNodeRow(
			presenter, guiState, ops, dragGroup, drill.folderID, i, input.Rows[i])
		// Регистрируем КАЖДУЮ строку: вычисление точки вставки просматривает
		// полосы всех строк, не только перетаскиваемой (контракт
		// DragReorderGroup, тот же, что у списка источников).
		dragGroup.Register(i, rowObj)
		sourcesBox.Add(rowObj)
	}
	return input.Name, true
}

// applyAddedSourcesToFolder — путь Add в режиме папки.
//
// Разбор ТОТ ЖЕ (`business.AppendNodesToFolder` → `parseSourceInput`), адрес
// назначения другой. Подписка узлом не становится: её отвергает то же ядро
// сентинелом `ErrSubscriptionInFolder`, и текст ошибки берётся тот же, что у
// кнопки «Add nodes…» в окне папки (folderAddNodesSubscriptionText) — двух
// формулировок одного отказа не заводим.
//
// Возвращает true, когда узлы легли: только тогда поле ввода очищается —
// отвергнутый текст обязан остаться на экране, иначе человек потеряет то, что
// вставил, вместе с сообщением об ошибке.
func applyAddedSourcesToFolder(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	folderID, text string,
) bool {
	return applyAddedSourcesToFolderNamed(presenter, guiState, folderID, text, "")
}

// applyAddedSourcesToFolderNamed — тот же путь с ИМЕНЕМ для безымянных узлов.
//
// `defaultTag` нужен ровно там же, где он нужен окну папки: имя файла у
// импорта и поле тега у формы сервера. У ссылки со своим `#fragment` он не
// применяется — правило одно и живёт в ядре (AppendNodesToFolder), здесь
// только адрес.
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
		}
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

// folderDrillNodeEnabled — включён ли узел папки (по сырому тегу).
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

// folderDrillSetNodeEnabled ставит отметку включённости узла папки.
//
// Возвращает false, когда ставить некуда (папка или узел исчезли, пока висел
// список) — вызывающий тогда не гоняет побочки мутации впустую.
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
