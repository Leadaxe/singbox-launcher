package ui

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// SPEC 095 — подзаголовок узла в списке серверов.
//
// Формат перенесён из LxBox (widgets/node_row.dart): под тегом идёт
// «протокол·транспорт·security» — `vless·ws·Reality`, `wireguard·awg2+`, —
// а у групп вместо протокола метка режима с размером пула и значками членов:
// `🔀 [15] 🇩🇪🇳🇱🇫🇮`.
//
// Данные берутся из сгенерированного config.json: Clash API их не отдаёт.

// maxPoolBadges — сколько значков членов показывать у группы.
//
// Пул провайдера бывает на 15+ узлов; вывести все значит растянуть строку и
// утопить в ней метку режима. Три-четыре флага дают понять состав, остальное
// видно в «Info».
const maxPoolBadges = 4

// serversNameTextSize — размер шрифта имени узла.
//
// Чуть меньше дефолтного размера темы: имя и подзаголовок должны уместиться
// в прежнюю высоту строки, с которой список был плотным.
const serversNameTextSize = 13

// serversNameMaxRunes — предел длины имени узла.
//
// canvas.Text не обрезает себя сам, а длинное имя вытолкнуло бы кнопки.
const serversNameMaxRunes = 34

// serversSubtitleTextSize — размер шрифта подзаголовка.
//
// Заметно мельче имени: подзаголовок поясняет строку, а не спорит с ней за
// внимание.
const serversSubtitleTextSize = 10

// serversTitleSubtitleGap — зазор между именем узла и подзаголовком.
//
// Оба текста — canvas.Text без собственных отступов, поэтому зазор нужен
// минимальный: строки иначе слипаются. Один пункт разделяет их ровно
// настолько, чтобы читались как заголовок с пояснением.
const serversTitleSubtitleGap = 1

// serversSubtitleMaxRunes — предел длины подзаголовка.
//
// canvas.Text не умеет обрезать себя многоточием, а длинная строка раздвигает
// строку списка и выталкивает кнопки за край. Обрезаем по рунам, а не байтам:
// в подзаголовке живут эмодзи-флаги.
const serversSubtitleMaxRunes = 42

// serversDelayTextSize — размер шрифта замера.
//
// Чуть крупнее подзаголовка: число — то, ради чего в список смотрят чаще
// всего, и оно должно читаться с первого взгляда.
const serversDelayTextSize = 12

// serversDelayColumnWidth — ширина зоны замера.
//
// Фиксированная, иначе кнопка ▶ прыгает по горизонтали от строки к строке:
// «31 ms» и «1194 ms» дают разную ширину, и список выглядит дёргано.
const serversDelayColumnWidth = 62

// serversDelayCellHeight — высота зоны замера.
//
// Совпадает с высотой соседней кнопки: иначе подложка выглядит приплюснутой
// рядом с ней.
const serversDelayCellHeight = 30

// canvasTextSetter адаптирует canvas.Text под интерфейс SetText, которого
// ждёт pingProxy.
//
// Замер показывается цветным текстом, а не кнопкой: widget.Button не даёт
// покрасить свой текст — Fyne предлагает только Importance, а тот заливает
// весь фон, и число тонет в заливке.
type canvasTextSetter struct {
	text *canvas.Text
}

// SetText обновляет текст и перерисовывает его.
func (s *canvasTextSetter) SetText(v string) {
	if s == nil || s.text == nil {
		return
	}
	s.text.Text = v
	s.text.Refresh()
}

// serversDelayText форматирует замер для колонки.
//
// Когда замера ещё не было — «Ping»: зона кликабельна, и пустая ячейка этого
// не сообщает, пользователь просто не знает, что по ней можно нажать.
func serversDelayText(delay int64) string {
	switch {
	case delay > 0:
		return locale.Tf("servers.ping_format_ms", delay)
	case delay == -1:
		return locale.T("servers.ping_button_error")
	default:
		return locale.T("servers.button_ping")
	}
}

// serversDelayColor красит замер по качеству связи.
//
// Цвет несёт СЛОВО, а не фон: заливка кнопки через Importance топит текст и
// заставляет строку кричать. Шкала как на мобиле — зелёный до 150 мс,
// жёлтый до 300, оранжевый выше, красный на ошибке.
func serversDelayColor(delay int64) color.Color {
	switch {
	case delay == -1:
		return color.NRGBA{R: 235, G: 90, B: 90, A: 255}
	case delay <= 0:
		return theme.Color(theme.ColorNamePlaceHolder)
	case delay < 150:
		return color.NRGBA{R: 80, G: 200, B: 110, A: 255}
	case delay < 300:
		return color.NRGBA{R: 220, G: 190, B: 80, A: 255}
	default:
		return color.NRGBA{R: 235, G: 150, B: 70, A: 255}
	}
}

// truncateRunes обрезает строку по рунам, добавляя многоточие.
//
// По рунам, а не байтам: в тегах живут эмодзи-флаги, и обрезка по байтам
// разрубила бы их посередине.
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// truncateSubtitle обрезает подзаголовок до serversSubtitleMaxRunes.
func truncateSubtitle(s string) string {
	runes := []rune(s)
	if len(runes) <= serversSubtitleMaxRunes {
		return s
	}
	return string(runes[:serversSubtitleMaxRunes-1]) + "…"
}

// serversNodeSubtitle строит подзаголовок для строки списка.
//
// Пустая строка означает «показывать нечего» — узла нет в конфиге либо это
// служебный outbound. Вызывающий в этом случае прячет Label.
func serversNodeSubtitle(ac *core.AppController, proxyInfo api.ProxyInfo) string {
	if ac == nil || ac.FileService == nil {
		return ""
	}
	nodes := wizardbusiness.LoadConfigNodes(ac.FileService.ConfigPath)
	node := nodes.Lookup(proxyInfo.Name)
	if node == nil || node.IsService() {
		return ""
	}

	if node.IsGroup() {
		return groupSubtitle(node)
	}
	return strings.Join(node.SubtitleParts(), "·")
}

// groupSubtitle описывает группу: режим, размер пула и значки членов.
//
// urltest ядро проверяет замерами и выбирает лучший — «🎯»; selector отдаёт
// выбор пользователю — «🔀». Иконки повторяют смысл, принятый в LxBox.
func groupSubtitle(node *wizardbusiness.ConfigNode) string {
	icon, mode := groupModeLabel(node)

	label := fmt.Sprintf("%s [%d]", icon, len(node.GroupMembers))
	if mode != "" {
		label += " " + mode
	}

	if badges := poolBadges(node.GroupMembers); badges != "" {
		label += " " + badges
	}
	return label
}

// groupModeLabel подбирает значок и подпись режима отбора.
//
// Тип outbound'а («urltest») говорит лишь о том, что ядро само выбирает узел,
// но НЕ как именно: у urltest есть собственное поле `mode` (расширение форка,
// SPEC 088) — «round_robin» раздаёт трафик по пулу, «least_test» держит один
// самый быстрый. Раньше подзаголовок читал только тип, и смена режима на
// экране никак не отражалась.
func groupModeLabel(node *wizardbusiness.ConfigNode) (icon, mode string) {
	if node.Type != "urltest" {
		// selector — выбор делает пользователь, режима отбора нет.
		return "🔀", ""
	}

	raw, _ := node.Raw["mode"].(string)
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "round_robin":
		// Балансировка по пулу: трафик расходится между узлами.
		return "⚖️", locale.T("servers.group_mode_round_robin")
	case "least_test", "":
		// Умолчание urltest — один самый быстрый по замерам.
		return "🎯", locale.T("servers.group_mode_least_test")
	default:
		// Незнакомый режим: показываем как есть, не выдумывая перевод.
		return "🎯", raw
	}
}

// poolBadges собирает значки членов пула — по флагу с каждого тега.
//
// Флаг несёт больше смысла, чем усечённое имя: пользователь читает состав
// одним взглядом. Члены без флага пропускаются, а не заменяются заглушкой,
// иначе строка забивается мусором.
func poolBadges(members []string) string {
	var b strings.Builder
	seen := make(map[string]struct{}, maxPoolBadges)
	shown := 0

	for _, tag := range members {
		if shown >= maxPoolBadges {
			break
		}
		flag := leadingFlag(tag)
		if flag == "" {
			continue
		}
		if _, dup := seen[flag]; dup {
			// Пул часто состоит из серверов одной страны — повторять один
			// флаг четырежды бессмысленно.
			continue
		}
		seen[flag] = struct{}{}
		b.WriteString(flag)
		shown++
	}

	if shown > 0 && len(members) > shown {
		b.WriteString("…")
	}
	return b.String()
}

// leadingFlag достаёт из тега флаг страны — пару Regional Indicator.
//
// Ищет по всему тегу, а не только в начале: провайдеры ставят перед флагом
// префикс источника («AL:🇩🇪-Германия»).
func leadingFlag(tag string) string {
	runes := []rune(tag)
	for i := 0; i+1 < len(runes); i++ {
		if isRegionalIndicator(runes[i]) && isRegionalIndicator(runes[i+1]) {
			return string(runes[i : i+2])
		}
	}
	return ""
}

// isRegionalIndicator сообщает, входит ли руна в блок Regional Indicator
// Symbol (U+1F1E6…U+1F1FF) — из пар этих символов состоят флаги.
func isRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}
