package tabs

import (
	"encoding/json"
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
)

// SPEC 095 — окно «Info» для узла на вкладке Preview.
//
// Отдельное от servers_node_info.go: то читает config.json по тегу, а здесь
// конфига ещё нет — подписка только разобрана, пользователь её даже не
// сохранил. Данные берутся прямо из ParsedNode.

// previewInfoKeyColumnWidth — ширина колонки с названиями полей.
//
// Фиксированная: иначе «Tag» и «REALITY public key» дают разный отступ, и
// колонка значений разъезжается.
const previewInfoKeyColumnWidth = 168

// previewInfoScrollGutter — отступ справа под полосу прокрутки.
const previewInfoScrollGutter = 5

// showPreviewNodeContextMenu показывает меню строки превью по правому клику.
//
// SPEC 116 W5 (§O4 = вариант А): к трём пунктам просмотра добавлены операции
// над узлом контейнера — превью у папки перестало быть только просмотром и
// стало её основным рабочим экраном. Второго списка узлов при этом не
// заведено: он разъехался бы с этим.
//
// rawTag — СЫРОЙ тег узла (идентичность в рамках контейнера, SPEC 112), а не
// node.Tag: тот уже прошёл тег-машину эмиссии (префикс папки, уникализация
// `-2`, переменные) и адресом узла в модели не является. Вызывающий знает его
// как identities[id] и обязан передать сюда — выводить его обратно из
// финального тега нельзя, политику не развернуть однозначно.
//
// ops == nil (или контейнер — подписка) — меню остаётся прежним, просмотровым.
func showPreviewNodeContextMenu(
	win fyne.Window,
	node *config.ParsedNode,
	rawTag string,
	ops *previewNodeOps,
	pe *fyne.PointEvent,
) {
	if win == nil || pe == nil {
		return
	}

	// node == nil — узел есть в составе, но эмиссия его не выпустила
	// (выключен). Пункты просмотра ему не подходят: показывать нечего, а JSON
	// у него ровно тот, которого в конфиге нет. Операции над узлом при этом
	// остаются: он в составе контейнера, и двигать/переименовывать/удалять его
	// пользователь вправе — иначе выключенный узел стал бы неприкасаемым.
	var items []*fyne.MenuItem
	if node != nil {
		items = append(items,
			fyne.NewMenuItem(locale.T("Node info…"), func() {
				showPreviewNodeInfoWindow(node)
			}),
			fyne.NewMenuItem(locale.T("Copy JSON"), func() {
				fynewidget.SetClipboard(previewNodeJSON(node))
			}),
			fyne.NewMenuItem(locale.T("Copy tag"), func() {
				fynewidget.SetClipboard(node.Tag)
			}),
		)
	}

	// Операции адресуют узел сырым тегом: без него команда не знает, что
	// двигать, и показывать её было бы обманом.
	if ops != nil && strings.TrimSpace(rawTag) != "" {
		if len(items) > 0 {
			items = append(items, fyne.NewMenuItemSeparator())
		}
		// «Copy to folder…» есть ВСЕГДА, в том числе у подписки: копия ничего
		// в источнике не меняет, а это ровно требование П2 — забрать узел
		// провайдера себе.
		items = append(items, fyne.NewMenuItem(locale.T("Copy to folder…"), func() {
			ops.showMoveOrCopyDialog(rawTag, false)
		}))
		// Move / Rename / Delete правят СОСТАВ контейнера, а состав подписки
		// принадлежит провайдеру (features/sources.md §«Свобода и несвобода
		// узлов»): следующий fetch вернул бы удалённый узел и переименовал
		// переименованный. Поэтому у подписки этих пунктов нет вовсе —
		// отключённый пункт обещал бы то, чего мы не сделаем.
		if ops.nodeOpsAllowed() {
			items = append(items,
				fyne.NewMenuItem(locale.T("Move to folder…"), func() {
					ops.showMoveOrCopyDialog(rawTag, true)
				}),
				fyne.NewMenuItem(locale.T("Rename…"), func() {
					ops.showRenameDialog(rawTag)
				}),
				fyne.NewMenuItem(locale.T("Delete"), func() {
					ops.showDeleteDialog(rawTag)
				}),
			)
		}
	}

	if len(items) == 0 {
		// Пустое меню — пустая рамка под курсором: показывать её незачем.
		return
	}
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), win.Canvas(), pe.AbsolutePosition)
}

// showPreviewNodeInfoWindow открывает окно с разбором узла и его JSON.
func showPreviewNodeInfoWindow(node *config.ParsedNode) {
	if node == nil {
		return
	}

	win := fyne.CurrentApp().NewWindow(
		locale.Tf("Node: %s", node.Tag))

	body := container.NewVBox()

	body.Add(previewSectionHeader(locale.T("General")))
	body.Add(previewInfoRow(locale.T("Tag"), node.Tag))
	body.Add(previewInfoRow(locale.T("Type"), node.Scheme))

	if node.Scheme == configtypes.SchemeGroup {
		appendPreviewGroupRows(body, node)
	} else {
		if node.Server != "" {
			body.Add(previewInfoRow(locale.T("Server"),
				fmt.Sprintf("%s:%d", node.Server, node.Port)))
		}
		if tr := previewTransportName(node); tr != "" {
			body.Add(previewInfoRow(locale.T("Transport"), tr))
		}
		if sec := previewSecurityName(node); sec != "" {
			body.Add(previewInfoRow(locale.T("Security"), sec))
		}
	}

	// Цепочка detour: узел ходит через другие — это важнее многих полей.
	if len(node.Chain) > 0 {
		body.Add(widget.NewSeparator())
		body.Add(previewSectionHeader(locale.T("Chain positions (%d)")))
		for i, hop := range node.Chain {
			if hop == nil {
				continue
			}
			body.Add(previewInfoRow(
				fmt.Sprintf("%d", i+1),
				fmt.Sprintf("%s  (%s:%d)", hop.Tag, hop.Server, hop.Port)))
		}
	}

	// JSON — отдельной вкладкой: он длинный и оттеснял бы разобранные поля.
	jsonText := previewNodeJSON(node)
	jsonEntry := widget.NewMultiLineEntry()
	jsonEntry.SetText(jsonText)
	jsonEntry.Wrapping = fyne.TextWrapOff

	jsonTab := container.NewBorder(
		nil,
		widget.NewButton(locale.T("Copy JSON"), func() {
			fynewidget.SetClipboard(jsonText)
		}),
		nil, nil,
		jsonEntry,
	)

	tabs := container.NewAppTabs(
		container.NewTabItem(locale.T("Details"),
			previewWithScrollGutter(body)),
		container.NewTabItem(locale.T("Outbound JSON"), jsonTab),
	)

	win.SetContent(tabs)
	win.Resize(fyne.NewSize(620, 640))
	win.CenterOnScreen()
	win.Show()
}

// appendPreviewGroupRows добавляет строки, специфичные для узла-группы.
func appendPreviewGroupRows(body *fyne.Container, node *config.ParsedNode) {
	if node.Outbound == nil {
		return
	}

	if url, ok := node.Outbound["url"].(string); ok && url != "" {
		body.Add(previewInfoRow(locale.T("Test URL"), url))
	}
	if interval, ok := node.Outbound["interval"].(string); ok && interval != "" {
		body.Add(previewInfoRow(locale.T("Test interval"), interval))
	}
	if def, ok := node.Outbound["default"].(string); ok && def != "" {
		body.Add(previewInfoRow(locale.T("Default"), def))
	}

	members := previewGroupMemberTags(node)
	body.Add(widget.NewSeparator())
	body.Add(previewSectionHeader(
		locale.Tf("Group members (%d)", len(members))))
	for _, tag := range members {
		l := widget.NewLabel("   · " + tag)
		l.Truncation = fyne.TextTruncateEllipsis
		body.Add(l)
	}
}

// previewGroupMemberTags возвращает состав узла-группы.
func previewGroupMemberTags(node *config.ParsedNode) []string {
	if node == nil || node.Outbound == nil {
		return nil
	}
	raw, _ := node.Outbound[configtypes.GroupMembersKey].([]interface{})
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// previewNodeJSON форматирует outbound узла для показа.
//
// Это тот самый объект, который уйдёт в config.json, — по нему видно всё,
// включая поля, которые UI не разбирает.
func previewNodeJSON(node *config.ParsedNode) string {
	if node == nil || node.Outbound == nil {
		return "{}"
	}
	data, err := json.MarshalIndent(node.Outbound, "", "  ")
	if err != nil {
		return fmt.Sprintf("// %v", err)
	}
	return string(data)
}

// previewSectionHeader — заголовок секции.
func previewSectionHeader(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.TextStyle.Bold = true
	return l
}

// previewInfoRow — строка «ключ: значение».
//
// Значение в Entry, а не Label: его можно выделить и скопировать, а длинное
// значение не растягивает окно.
func previewInfoRow(key, value string) *fyne.Container {
	keyLabel := widget.NewLabel(key)
	keyLabel.TextStyle.Bold = true
	keyLabel.Truncation = fyne.TextTruncateEllipsis

	keySpacer := canvas.NewRectangle(color.Transparent)
	keySpacer.SetMinSize(fyne.NewSize(previewInfoKeyColumnWidth, 0))
	keyCell := container.NewStack(keySpacer, keyLabel)

	valueEntry := widget.NewEntry()
	valueEntry.SetText(value)
	valueEntry.Wrapping = fyne.TextWrapOff

	return container.NewBorder(nil, nil, keyCell, nil, valueEntry)
}

// previewWithScrollGutter добавляет отступ под полосу прокрутки.
func previewWithScrollGutter(body fyne.CanvasObject) fyne.CanvasObject {
	gutter := canvas.NewRectangle(color.Transparent)
	gutter.SetMinSize(fyne.NewSize(previewInfoScrollGutter, 0))
	return container.NewScroll(container.NewBorder(nil, nil, nil, gutter, body))
}
