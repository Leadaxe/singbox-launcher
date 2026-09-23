// File source_node_warnings.go — сводки предупреждений узла для списков
// источников и превью (SPEC 131 §6, волна W3).
//
// Тексты самих кодов здесь не живут и жить не могут: их дом — реестр
// контракта (`internal/nodewarn` → `registry.WarningText`), один на лаунчер,
// LxBox и генерацию документации. Здесь только счёт и вёрстка обвязки.
package tabs

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/nodewarn"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).

// warnedNodesFieldsText — сводка превью, когда сняты КОНКРЕТНЫЕ поля.
const warnedNodesFieldsText = "⚠ %d field(s) stripped on %d node(s)" // l10n-key

// warnedNodesPlainText — сводка превью для кодов уровня узла: полей там не
// снимали, и говорить «снято полей» было бы неправдой.
const warnedNodesPlainText = "⚠ %d warning(s) on %d node(s)" // l10n-key

// infoNodesText — заголовок группы «к сведению» под составом.
//
// Без ⚠ и без слова warnings: группа отвечает «делать ничего не нужно», и
// знак тревоги рядом с ней спорил бы с её собственным текстом.
const infoNodesText = "For your information: %d node(s)" // l10n-key

// sourceWarnedNodes — сколько узлов источника несут ПРОБЛЕМУ: код уровня
// error или warning.
//
// Считается по СОСТАВУ, а не по эмиссии: warnings принадлежат записи узла и
// живут в ней независимо от того, выпустила ли её сборка. Узловой источник
// (server/chain/auto) состава не имеет — там узел и есть сам источник.
//
// info сюда не входит намеренно. Это число рисует «⚠ N node(s) with
// warnings» в СПИСКЕ источников — строке, которую пользователь читает, не
// открывая ничего, — и ставить там ⚠ ради «к сведению» значит звать
// разбираться туда, где разбираться не с чем. Инфо-кодам место внутри
// контейнера (сводка под составом, секция в окне узла), а не в общем списке:
// у подписки на 500 узлов они дали бы ⚠ у каждой второй строки и обесценили
// бы сам знак.
func sourceWarnedNodes(src *wizardmodels.Source) int {
	if src == nil {
		return 0
	}
	if len(src.Nodes) == 0 {
		if nodewarn.HasProblems(src.Warnings) {
			return 1
		}
		return 0
	}
	n := 0
	for i := range src.Nodes {
		if nodewarn.HasProblems(src.Nodes[i].Warnings) {
			n++
		}
	}
	return n
}

// previewWarningsSummary — строка под списком узлов: «⚠ снято N полей у M
// узлов» (SPEC 131 §6).
//
// Два вида счёта, потому что коды бывают двух уровней: у кода уровня ПОЛЯ
// есть путь, и сказать про него «снято поле» честно; у кода уровня УЗЛА пути
// нет, и та же формулировка соврала бы. Когда есть и те и другие, показываем
// формулировку про поля — она конкретнее, а узловые коды всё равно видны в
// строках и в окне узла.
//
// Считаются только узлы с ПРОБЛЕМОЙ (error/warning) и только их коды: у
// info-узла ничего не снимали и ничего не приводили, и «снято N полей» про
// него было бы прямой неправдой. «К сведению» живёт своей группой ниже
// (previewInfoSummary).
//
// Пустая строка = показывать нечего.
func previewWarningsSummary(rows []previewRow) string {
	fields, nodeLevel, nodes := 0, 0, 0
	for _, r := range rows {
		if !nodewarn.HasProblems(r.Warnings) {
			continue
		}
		nodes++
		f, n := nodewarn.FieldsStripped(r.Warnings)
		fields += f
		nodeLevel += n
	}
	if nodes == 0 {
		return ""
	}
	if fields > 0 {
		return locale.Tf(warnedNodesFieldsText, fields, nodes)
	}
	return locale.Tf(warnedNodesPlainText, nodeLevel, nodes)
}

// previewInfoSummary — заголовок группы «к сведению»: узлы, у которых КРОМЕ
// info ничего нет.
//
// Узел с error и info сюда не попадает: про него уже сказано сильнее, и
// второе упоминание в списке ниже читалось бы как второй, отдельный факт.
//
// Пустая строка = таких узлов нет.
func previewInfoSummary(rows []previewRow) string {
	nodes := 0
	for _, r := range rows {
		if nodewarn.InfoOnly(r.Warnings) {
			nodes++
		}
	}
	if nodes == 0 {
		return ""
	}
	return locale.Tf(infoNodesText, nodes)
}

// previewWarningsBlock — сводка под списком узлов с раскрытием построчно.
//
// `widget.Accordion`, а не свой тумблер: раскрывающихся блоков в проекте
// готовых нет (анонс провайдера раскрывает шапка контейнера своим кодом, и
// переиспользовать его вне шапки нечем), а заводить второй механизм ради
// одной сводки — ровно то умножение копий, от которого уходит кампания.
//
// Закрыт по умолчанию: сводка отвечает на «всё ли в порядке» одной строкой,
// а список из двухсот путей нужен тому, кто уже решил разбираться.
//
// Групп ДВЕ, и они не сливаются: «⚠ …» — узлы, с которыми что-то сделали, и
// «For your information: N» — узлы, про которые есть что сказать, но делать
// ничего не нужно. Одной строкой их считали до разведения уровней, и она
// объявляла двенадцать спокойных узлов предупреждениями. Порядок тот же, что
// в секции окна узла: сперва проблемы, потом «к сведению».
//
// Wrapping у каждой строки обязателен (Л19): путь поля вместе с заголовком
// кода длиннее окна, и Label без переноса раздул бы его на весь экран.
func previewWarningsBlock(rows []previewRow) fyne.CanvasObject {
	accItems := make([]*widget.AccordionItem, 0, 2)

	if summary := previewWarningsSummary(rows); summary != "" {
		if lines := previewWarningLines(rows, nodewarn.HasProblems); len(lines) > 0 {
			accItems = append(accItems,
				widget.NewAccordionItem(summary, container.NewVBox(lines...)))
		}
	}
	if summary := previewInfoSummary(rows); summary != "" {
		if lines := previewWarningLines(rows, nodewarn.InfoOnly); len(lines) > 0 {
			accItems = append(accItems,
				widget.NewAccordionItem(summary, container.NewVBox(lines...)))
		}
	}
	if len(accItems) == 0 {
		return nil
	}
	return widget.NewAccordion(accItems...)
}

// previewWarningLines — строки «узел · путь — заголовок кода» у тех строк
// списка, которые прошли отбор.
//
// Отбор параметром, а не двумя копиями цикла: вёрстка строки у обеих групп
// одна и та же, и разъехаться ей нельзя — это один и тот же факт про узел,
// показанный в двух разных контекстах.
func previewWarningLines(rows []previewRow, pick func([]corestate.NodeWarning) bool) []fyne.CanvasObject {
	items := make([]fyne.CanvasObject, 0, 8)
	for _, r := range rows {
		if !pick(r.Warnings) {
			continue
		}
		tag := previewRowTitle(r)
		for _, t := range nodewarn.Describe(r.Warnings) {
			line := "• " + tag
			if t.Path != "" {
				line += "  ·  " + t.Path
			}
			line += "  —  " + t.Title
			lbl := widget.NewLabel(line)
			lbl.Wrapping = fyne.TextWrapWord
			lbl.Importance = widget.MediumImportance
			items = append(items, lbl)
		}
	}
	return items
}
