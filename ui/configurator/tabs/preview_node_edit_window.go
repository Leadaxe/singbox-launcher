// File preview_node_edit_window.go — ОДНО окно правки узла контейнера
// (SPEC 116 W13, обкатка заход 2, пункты 6 и 7).
//
// # Что упразднено и почему
//
// До этого захода у строки узла было ДВА окна: «Node info…»
// (`showPreviewNodeInfoWindow`, разбор + JSON, только чтение) и всплывающее
// окошко клика `showPreviewRowInfoWindow` (имя + `origin.raw`). Оба
// показывали, ни одно не правило, и пользователь, увидев в подписке узел с
// битым телом, не мог его починить, не забирая копию в папку через окно
// источника. Требование обкатки дословно: «карандаш и пункт меню открывают
// существующее окно правки узла (редактор body + имя + происхождение,
// source_body_edit-путь)».
//
// Поэтому здесь одно окно на обе роли: у узла ПАПКИ оно правит, у узла
// ПОДПИСКИ и у неразобранной записи — показывает то же самое read-only.
// Развилка «правится / не правится» одна и та же, что у меню
// (`previewNodeOps.nodeOpsAllowed`): состав подписки принадлежит провайдеру,
// и правка тела пережила бы ровно до первого fetch'а.
//
// # Почему вкладки, а не один свиток
//
// Первая редакция окна складывала имя, тело и происхождение в одну колонку —
// и читалась как ТРЕТИЙ, просмотровый экран поверх уже существующей правки:
// до исходника приходилось скроллить мимо редактора, а обратно — тем же
// путём. У узла ровно те же две роли, что у server-источника в окне
// источника: «что это за узел» (имя, происхождение, причина отбраковки) и
// «его outbound» (тело). Расклад вкладок здесь тот же — Settings | JSON, — и
// второго вида окна узла не заводится.
//
// У неразобранной записи вкладки JSON нет вовсе: тела за ней не стоит, и
// вкладка была бы пустым редактором с кнопками в никуда.
//
// # Почему путь правки — тот же source_body_edit
//
// `applyServerBodyJSON` / `regenServerBodyFromRaw` — единственная точка
// «текст → тело узла» (SPEC 118 Т8): та же `config.MaterializeServerNode`,
// что зовут fetch и миграция, тот же откат при неразбираемом вводе. Второй
// реализации не заводится — ловушка «эмиттер и парсер ходят парой». Отличие
// от окна источника ровно одно: там правится `&scratch.Node` буфера формы,
// здесь — узел в составе контейнера, и мутация НЕМЕДЛЕННАЯ (у списка нет ни
// scratch'а, ни Save — тот же довод, что у Move/Copy в шапке
// preview_node_ops.go).
//
// # Почему переименование зовёт готовый applyRename
//
// Имя узла контейнера — его идентичность (SPEC 112), и смена тега тянет за
// собой перепись ссылок (`RepointContainerNodeLinks`), разыменование от
// подписки (Д5) и предупреждение о протухшем выборе. Всё это уже собрано в
// `previewNodeOps.applyRename` — окно только отдаёт ему новое имя.
package tabs

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	// nodeEditBodyHintText — подсказка над редактором тела.
	nodeEditBodyHintText = "The sing-box outbound this node is built from. Edit and press Apply; Regen from raw rebuilds it from the origin below. Tag and detour are restamped by the launcher at build time."
	// nodeEditReadOnlySubText — почему узел подписки не правится.
	nodeEditReadOnlySubText = "Read-only: nodes of a subscription are refreshed from the provider, so an edit here would be lost on the next update. Use “Copy to folder…” to take this node for yourself."
	// nodeEditUnsupportedText — почему у неразобранной записи нет тела.
	nodeEditUnsupportedText = "This entry could not be parsed, so it has no outbound body — only the original line below. Fix it by pasting a corrected link into a folder."
)

// showPreviewNodeEditWindow — окно правки одного узла контейнера.
//
// Адрес узла — пара (контейнер из `ops`, сырой тег `rawTag`), как у всех
// операций W5: индекс строки сюда не едет, а тег ищется в живой записи на
// каждое действие — пока висит окно, состав вправе поехать (фоновый fetch,
// вторая операция).
//
// `ops == nil` невозможен по построению (окно зовут только строки, у которых
// контекст есть), но проверяется: без контекста ни найти узел, ни применить
// правку нечем.
func showPreviewNodeEditWindow(r previewRow, rawTag string, ops *previewNodeOps) {
	if ops == nil || ops.presenter == nil {
		return
	}
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	tag := strings.TrimSpace(rawTag)
	if tag == "" {
		tag = strings.TrimSpace(r.RawTag)
	}

	// Правится ТОЛЬКО узел папки и только разобранный: у подписки состав
	// принадлежит провайдеру, у неразобранной записи тела нет вовсе.
	editable := ops.nodeOpsAllowed() && !r.Unsupported && tag != ""

	win := app.NewWindow(locale.Tf("Node: %s", previewRowTitle(r)))

	// --- имя узла -----------------------------------------------------------
	nameEntry := widget.NewEntry()
	nameEntry.SetText(tag)
	nameRow := fyne.CanvasObject(nameEntry)
	if editable {
		// Переименование — отдельной кнопкой, а не «по потере фокуса»: за ним
		// тянутся перепись ссылок, разыменование и предупреждение о протухшем
		// выборе, и запускать это на каждый набранный символ нельзя.
		renameBtn := widget.NewButton(locale.T("Rename"), func() {
			newTag := strings.TrimSpace(nameEntry.Text)
			if newTag == "" || newTag == tag {
				return
			}
			ops.applyRename(tag, newTag)
			// Окно адресует узел прежним тегом; после переименования этот
			// адрес мёртв, и держать окно открытым значило бы дать применить
			// следующую правку в никуда.
			win.Close()
		})
		nameRow = container.NewBorder(nil, nil, nil, renameBtn, nameEntry)
	} else {
		// Ввод откатывается, а не Disable(): на macOS выключенный Entry красит
		// текст цветом фона (та же ловушка, что у read-only полей Overview), а
		// имя обязано оставаться читаемым и копируемым.
		frozenName := nameEntry.Text
		nameEntry.OnChanged = func(s string) {
			if s != frozenName {
				nameEntry.SetText(frozenName)
			}
		}
	}

	// Подсказка над телом: чем узел правится, либо почему не правится.
	hintText := nodeEditBodyHintText
	if r.Unsupported {
		hintText = nodeEditUnsupportedText
	} else if !editable {
		hintText = nodeEditReadOnlySubText
	}
	hint := widget.NewLabel(locale.T(hintText))
	// Ловушка fyne-label-minwidth-trap: Label без Wrapping задаёт окну
	// min-width своей строкой и раздувает его на весь экран.
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance

	// Вкладка Settings — «что это за узел»: имя (его идентичность, SPEC 112),
	// причина отбраковки у неразобранной записи и происхождение. Тело узла
	// уехало на свою вкладку JSON — ровно тот же расклад, что у окна
	// server-источника, где Settings и JSON тоже разделены.
	settingsBox := container.NewVBox(
		previewSectionHeader(locale.T("Name")),
		nameRow,
	)

	// Причина отбраковки — сразу под именем: у неразобранной записи это
	// главное, что о ней вообще известно.
	if r.Unsupported {
		settingsBox.Add(previewInfoRow(locale.T("Reason"), previewRowReason(r)))
	}
	// Подсказка «почему тела нет» встаёт под причину и только у неразобранной
	// записи: у неё вкладки JSON нет вовсе (ниже), и объяснению больше негде
	// стоять. У остальных та же переменная — шапка вкладки JSON.
	if r.Unsupported {
		settingsBox.Add(hint)
	}

	// --- тело узла ----------------------------------------------------------
	bodyEntry := widget.NewMultiLineEntry()
	bodyEntry.Wrapping = fyne.TextWrapOff
	bodyEntry.SetText(nodeBodyText(ops, tag, r))

	// Шапка вкладки JSON. У неразобранной записи заголовка «Outbound JSON»
	// нет: тела за ней не стоит вовсе, и заголовок над пустым полем обещал бы
	// объект, которого не существует, — ей остаётся одна подсказка-причина.
	jsonHead := container.NewVBox()
	if !r.Unsupported {
		jsonHead.Add(previewSectionHeader(locale.T("Outbound JSON")))
	}
	jsonHead.Add(hint)

	if !editable {
		// Ввод откатывается, а не Disable(): на macOS выключенный Entry
		// красит текст цветом фона (тот же приём, что у read-only полей
		// Overview), а выделить и скопировать тело обязано быть можно.
		frozen := bodyEntry.Text
		bodyEntry.OnChanged = func(s string) {
			if s != frozen {
				bodyEntry.SetText(frozen)
			}
		}
	}

	// «Copy JSON» есть у обоих видов окна: скопировать тело можно и у узла,
	// который не правится, — это ровно то, ради чего пункт «Copy JSON» живёт
	// в меню строки.
	bodyBtnItems := []fyne.CanvasObject{
		layout.NewSpacer(),
		widget.NewButton(locale.T("Copy JSON"), func() {
			fynewidget.SetClipboard(bodyEntry.Text)
		}),
	}
	if editable {
		regenBtn := widget.NewButton(locale.T("Regen from raw"), func() {
			dialog.ShowConfirm(
				locale.T("Regen from raw"),
				locale.T("Rebuild the outbound from the original URI/JSON, discarding manual edits?"),
				func(ok bool) {
					if !ok {
						return
					}
					if regenNodeBody(ops, tag, win) {
						bodyEntry.SetText(nodeBodyText(ops, tag, r))
					}
				}, win)
		})
		applyBtn := widget.NewButton(locale.T("Apply"), func() {
			if applyNodeBodyEdit(ops, tag, bodyEntry.Text, win) {
				bodyEntry.SetText(nodeBodyText(ops, tag, r))
			}
		})
		applyBtn.Importance = widget.HighImportance
		bodyBtnItems = append(bodyBtnItems, regenBtn, applyBtn)
	}
	bodyButtons := container.NewHBox(bodyBtnItems...)

	// --- происхождение ------------------------------------------------------
	origin := r.OriginRaw
	if origin == "" {
		origin = nodeOriginRaw(ops, tag)
	}
	originEntry := widget.NewMultiLineEntry()
	originEntry.Wrapping = fyne.TextWrapBreak
	originEntry.SetText(origin)
	// Происхождение — факт о том, откуда узел взялся; правится оно только
	// через Regen (пересборкой тела), поэтому поле здесь только для чтения.
	frozenOrigin := origin
	originEntry.OnChanged = func(s string) {
		if s != frozenOrigin {
			originEntry.SetText(frozenOrigin)
		}
	}

	if strings.TrimSpace(origin) != "" {
		// Происхождение — часть ответа «что это за узел», и живёт оно на
		// Settings рядом с именем. У узла, собранного руками с нуля, исходника
		// нет вовсе: пустая рамка под заголовком «Origin» читалась бы как
		// потеря данных, поэтому блок не добавляется, а не прячется.
		settingsBox.Add(widget.NewSeparator())
		settingsBox.Add(previewSectionHeader(locale.T("Origin")))
		settingsBox.Add(originEntry)
		settingsBox.Add(container.NewHBox(layout.NewSpacer(),
			widget.NewButton(locale.T("Copy"), func() {
				fynewidget.SetClipboard(frozenOrigin)
			})))
	}

	// Вкладки, а не один свиток: у узла ровно те же две роли, что у
	// server-источника в окне источника, — «что это за узел» (имя,
	// происхождение) и «его outbound» (тело). Смешанные в одну колонку, они
	// заставляли скроллить мимо редактора к исходнику и обратно; расклад
	// вкладок здесь тот же, что там, и второго вида окна узла не заводится.
	//
	// Тело на своей вкладке растягивается на всю высоту — правят именно его,
	// а шапка и кнопки прижаты к краям.
	jsonTabBody := container.NewBorder(jsonHead, bodyButtons, nil, nil, bodyEntry)

	tabItems := []*container.TabItem{
		container.NewTabItem(locale.T("Settings"), container.NewVScroll(settingsBox)),
	}
	if !r.Unsupported {
		// У неразобранной записи вкладки JSON нет вовсе: тела за ней не стоит,
		// и вкладка была бы пустым редактором с кнопками в никуда. Причина,
		// объяснение и исходник у неё уже на Settings — этого достаточно.
		tabItems = append(tabItems, container.NewTabItem(locale.T("JSON"), jsonTabBody))
	}

	tabs := container.NewAppTabs(tabItems...)
	tabs.SetTabLocation(container.TabLocationTop)

	win.SetContent(tabs)
	win.Resize(fyne.NewSize(680, 620))
	win.CenterOnScreen()
	win.Show()
}

// nodeBodyText — тело узла как текст для редактора.
//
// Читается из ЖИВОЙ записи модели, а не из эмитированного `ParsedNode`: в
// состоянии лежит ровно то, что правится (`Node.Body` без tag/detour), а
// эмиссия уже проштамповала финальный тег и подставила переменные — сохранить
// её обратно значило бы вморозить результат тег-машины в состав.
func nodeBodyText(ops *previewNodeOps, rawTag string, r previewRow) string {
	node := lookupContainerNode(ops, rawTag)
	if node != nil && len(node.Body) == 0 && node.Group != nil {
		// Авто-группа: тела у неё нет, а эмитированный r.Node собран без
		// прохода резолва — его outbounds всегда пуст, и показывать его
		// значило бы врать «группа пустая». Показываем запись состава как
		// она хранится: тип, члены-ссылки, стратегия.
		if b, err := json.MarshalIndent(node, "", "  "); err == nil {
			return string(b)
		}
	}
	if node == nil || len(node.Body) == 0 {
		if r.Node != nil {
			// Узла в составе нет (узловой источник: server/chain/auto — он сам
			// себе Source): показываем то, что уедет в конфиг.
			return previewNodeJSON(r.Node)
		}
		return ""
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, node.Body, "", "  "); err != nil {
		// Тело не форматируется — показываем как лежит: испортить редактор
		// сообщением об ошибке хуже, чем показать однострочный JSON.
		return string(node.Body)
	}
	return pretty.String()
}

// nodeOriginRaw — исходник узла из живой записи (когда строка его не несёт).
func nodeOriginRaw(ops *previewNodeOps, rawTag string) string {
	node := lookupContainerNode(ops, rawTag)
	if node == nil || node.Origin == nil {
		return ""
	}
	return node.Origin.Raw
}

// lookupContainerNode — узел контейнера по сырому тегу в ЖИВОЙ модели.
//
// Указатель, а не копия: правка тела пишется прямо в состав (немедленная
// мутация, см. шапку). nil = узла или контейнера больше нет.
func lookupContainerNode(ops *previewNodeOps, rawTag string) *corestate.Node {
	if ops == nil || ops.presenter == nil || strings.TrimSpace(rawTag) == "" {
		return nil
	}
	m := ops.presenter.Model()
	if m == nil || ops.sourceIndex < 0 || ops.sourceIndex >= len(m.Sources) {
		return nil
	}
	nodes := m.Sources[ops.sourceIndex].Nodes
	for i := range nodes {
		if nodes[i].Tag == rawTag {
			return &nodes[i]
		}
	}
	return nil
}

// applyNodeBodyEdit — Apply редактора: текст → тело узла контейнера.
//
// Тот же `applyServerBodyJSON`, что у вкладки JSON окна источника, и то же
// разыменование (Д5, критерий A4): ручная правка тела снимает связь узла с
// подпиской — иначе следующая заливка перезаписала бы правку телом провайдера.
//
// Возвращает true при успехе; ошибка = ОТКАТ, узел остаётся прежним.
func applyNodeBodyEdit(ops *previewNodeOps, rawTag, text string, win fyne.Window) bool {
	node := lookupContainerNode(ops, rawTag)
	if node == nil {
		dialog.ShowError(errors.New(locale.Tf("Node %q is gone.", rawTag)), win)
		return false
	}
	hadSubURL := node.Origin != nil && node.Origin.SubURL != ""
	if err := applyServerBodyJSON(node, text); err != nil {
		dialog.ShowError(errors.New(locale.Tf("Invalid JSON: %s", err.Error())), win)
		return false
	}
	dereferenced := wizardbusiness.DereferenceNodeOrigin(node)
	applySourceMutation(ops.presenter, ops.guiState)
	ops.afterModelMutation()
	if dereferenced || hadSubURL {
		notifyNodeDereferenced(win, rawTag)
	}
	return true
}

// regenNodeBody — «Regen from raw» для узла контейнера.
func regenNodeBody(ops *previewNodeOps, rawTag string, win fyne.Window) bool {
	node := lookupContainerNode(ops, rawTag)
	if node == nil {
		dialog.ShowError(errors.New(locale.Tf("Node %q is gone.", rawTag)), win)
		return false
	}
	hadSubURL := node.Origin != nil && node.Origin.SubURL != ""
	if err := regenServerBodyFromRaw(node); err != nil {
		dialog.ShowError(errors.New(locale.Tf(
			"URI does not unpack: %s. You can write the outbound JSON by hand and press Apply.",
			err.Error())), win)
		return false
	}
	dereferenced := wizardbusiness.DereferenceNodeOrigin(node)
	applySourceMutation(ops.presenter, ops.guiState)
	ops.afterModelMutation()
	if dereferenced || hadSubURL {
		notifyNodeDereferenced(win, rawTag)
	}
	return true
}
