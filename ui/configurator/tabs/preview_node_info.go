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
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
)

// SPEC 095 — меню строки узла на вкладке Preview и в списке Sources.
//
// SPEC 116 W13 (обкатка, заход 2): прежнее просмотровое окно
// `showPreviewNodeInfoWindow` (разбор полей + JSON, только чтение) и всплывающее
// окошко клика `showPreviewRowInfoWindow` упразднены — у строки узла ОДНО окно,
// `showPreviewNodeEditWindow` (preview_node_edit_window.go): имя, тело,
// происхождение, и правка там же, где просмотр. Держать три окна на одну строку
// значило бы разводить три набора текстов вокруг одного узла, а пользователю,
// увидевшему битое тело, всё равно было бы нечем его починить.

// previewInfoKeyColumnWidth — ширина колонки с названиями полей.
//
// Фиксированная: иначе «Tag» и «REALITY public key» дают разный отступ, и
// колонка значений разъезжается.
const previewInfoKeyColumnWidth = 168

// showPreviewNodeContextMenu показывает меню строки превью по правому клику.
//
// SPEC 116 W5 (§O4 = вариант А): к пунктам просмотра добавлены операции над
// узлом контейнера — превью у папки перестало быть только просмотром и стало её
// основным рабочим экраном. Второго списка узлов при этом не заведено: он
// разъехался бы с этим.
//
// rawTag — СЫРОЙ тег узла (идентичность в рамках контейнера, SPEC 112), а не
// node.Tag: тот уже прошёл тег-машину эмиссии (префикс папки, уникализация
// `-2`, переменные) и адресом узла в модели не является. Вызывающий знает его
// как identities[id] и обязан передать сюда — выводить его обратно из
// финального тега нельзя, политику не развернуть однозначно.
//
// ops == nil (или контейнер — подписка) — меню остаётся прежним, просмотровым.
//
// W13 заход 2 (принцип «меню = кнопки»): первый пункт открывает ТО ЖЕ окно, что
// карандаш строки, — набор действий у меню и у кнопок справа один.
func showPreviewNodeContextMenu(
	win fyne.Window,
	r previewRow,
	rawTag string,
	ops *previewNodeOps,
	pe *fyne.PointEvent,
) {
	if win == nil || pe == nil {
		return
	}
	node := r.Node

	// node == nil — узел есть в составе, но эмиссия его не выпустила
	// (выключен). Пункты про эмитированный JSON и финальный тег ему не
	// подходят: показывать нечего, а JSON у него ровно тот, которого в конфиге
	// нет. Окно узла и операции при этом остаются: он в составе контейнера, и
	// смотреть/двигать/переименовывать/удалять его пользователь вправе —
	// иначе выключенный узел стал бы неприкасаемым.
	var items []*fyne.MenuItem
	// Окно узла адресует его парой (контейнер из ops, сырой тег): без
	// контекста открывать нечего, и пункт не показывается.
	if ops != nil {
		items = append(items, fyne.NewMenuItem(locale.T("Node info…"), func() {
			showPreviewNodeEditWindow(r, rawTag, ops)
		}))
	}
	if node != nil {
		items = append(items,
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
