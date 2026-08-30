package tabs

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/locale"
)

// SPEC 095 — подзаголовок узла на вкладке Preview окна источника.
//
// Отличие от списка серверов: там данные берутся из сгенерированного
// config.json, а здесь узла в конфиге ещё нет — подписка только что разобрана.
// Поэтому подзаголовок строится прямо из ParsedNode.

const (
	// previewNameTextSize — размер имени узла.
	previewNameTextSize = 13
	// previewSubtitleTextSize — размер подзаголовка, заметно мельче имени.
	previewSubtitleTextSize = 10
	// previewTitleSubtitleGap — зазор между строками, минимальный: они читаются
	// как заголовок с пояснением, а не как два независимых элемента.
	previewTitleSubtitleGap = 1
)

// previewNodeSubtitle описывает узел строкой «протокол·транспорт·security».
//
// У групп вместо этого — режим и размер пула: адреса у них нет, а состав это
// единственное, что о группе стоит знать в списке.
func previewNodeSubtitle(node *config.ParsedNode) string {
	if node == nil {
		return ""
	}

	if node.Scheme == configtypes.SchemeGroup {
		return previewGroupSubtitle(node)
	}

	parts := make([]string, 0, 3)
	if node.Scheme != "" {
		parts = append(parts, node.Scheme)
	}
	if tr := previewTransportName(node); tr != "" {
		parts = append(parts, tr)
	}
	if sec := previewSecurityName(node); sec != "" {
		parts = append(parts, sec)
	}
	return strings.Join(parts, "·")
}

// previewGroupSubtitle описывает группу: режим отбора и размер пула.
func previewGroupSubtitle(node *config.ParsedNode) string {
	return previewGroupSubtitleCounted(node, -1)
}

// previewGroupSubtitleCounted — то же с внешним счётчиком живых членов
// (annotatePreviewGroupRows); alive < 0 = счёта не было, размер из узла.
func previewGroupSubtitleCounted(node *config.ParsedNode, alive int) string {
	groupType, _ := node.Outbound["type"].(string)

	// Тип говорит лишь «ядро выбирает само», но не КАК: у urltest есть своё
	// поле mode (расширение форка, SPEC 088). Значок и подпись должны его
	// отражать — иначе смена режима на экране никак не видна.
	// selector — выбор делает пользователь, режима отбора нет.
	icon := "\U0001F590"
	mode := ""
	if groupType == "urltest" {
		raw, _ := node.Outbound["mode"].(string)
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "round_robin":
			// «⭐» U+2B50 — подобрана в каталоге глифов: «🔀» на кегле
			// подзаголовка не читался, а «⚡» U+26A1 не рисуется вовсе
			// (пустой глиф в EmojiOneColor).
			icon, mode = "\U00002B50", locale.T("balanced")
		case "least_test", "":
			icon, mode = "\U0001F6A9", locale.T("fastest")
		default:
			icon, mode = "\U0001F6A9", raw
		}
	}

	members := alive
	if members < 0 {
		members = 0
		if raw, ok := node.Outbound[configtypes.GroupMembersKey].([]interface{}); ok {
			members = len(raw)
		}
		if members == 0 && len(node.CanonicalGroupMembers) > 0 {
			// Превью зовёт эмиссию БЕЗ прохода 2: состав канонической группы
			// там ещё ссылки NodeLink, а не outbounds, и без этой ветки любая
			// живая авто-группа подписки показывала бы «[0]». Заявленный
			// размер — fallback для мест без annotatePreviewGroupRows.
			members = len(node.CanonicalGroupMembers)
		}
	}

	label := fmt.Sprintf("%s [%d]", icon, members)
	if mode != "" {
		label += " " + mode
	}
	return label
}

// previewTransportName достаёт тип транспорта.
//
// Пустой транспорт означает голый TCP — так и подписываем, иначе строка
// «vless··Reality» выглядит поломанной.
func previewTransportName(node *config.ParsedNode) string {
	if node.Outbound == nil {
		return ""
	}
	// Протоколы поверх собственного стека транспорта не имеют: приписать им
	// «tcp» значит соврать. WireGuard и MASQUE ходят по UDP, QUIC-протоколы
	// несут транспорт внутри себя.
	switch node.Scheme {
	case "wireguard", "masque", "hysteria2", "tuic":
		return ""
	}
	tr, ok := node.Outbound["transport"].(map[string]interface{})
	if !ok {
		return "tcp"
	}
	if t, ok := tr["type"].(string); ok && t != "" {
		return t
	}
	return "tcp"
}

// previewSecurityName описывает слой шифрования: Reality, TLS или ничего.
//
// Vision дописывается к Reality: это заметная характеристика узла, по ней
// пользователи и различают варианты одного сервера.
func previewSecurityName(node *config.ParsedNode) string {
	if node.Outbound == nil {
		return ""
	}
	tls, ok := node.Outbound["tls"].(map[string]interface{})
	if !ok {
		return ""
	}
	if enabled, ok := tls["enabled"].(bool); ok && !enabled {
		return ""
	}

	name := "TLS"
	if _, hasReality := tls["reality"].(map[string]interface{}); hasReality {
		name = "Reality"
	}
	if flow, ok := node.Outbound["flow"].(string); ok && strings.Contains(flow, "vision") {
		name += "+Vision"
	}
	return name
}

// previewTightVBox — вертикальная укладка без зазора темы.
//
// container.NewVBox ставит между элементами theme.Padding(), и подзаголовок
// отрывается от имени: строка читается как две независимые.
type previewTightVBox struct {
	gap float32
}

// Layout расставляет элементы сверху вниз по их минимальной высоте.
func (l previewTightVBox) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for _, o := range objects {
		if o == nil || !o.Visible() {
			continue
		}
		h := o.MinSize().Height
		o.Resize(fyne.NewSize(size.Width, h))
		o.Move(fyne.NewPos(0, y))
		y += h + l.gap
	}
}

// MinSize — сумма высот с зазорами и максимальная ширина.
func (l previewTightVBox) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var width, height float32
	visible := 0
	for _, o := range objects {
		if o == nil || !o.Visible() {
			continue
		}
		m := o.MinSize()
		if m.Width > width {
			width = m.Width
		}
		height += m.Height
		visible++
	}
	if visible > 1 {
		height += l.gap * float32(visible-1)
	}
	return fyne.NewSize(width, height)
}
