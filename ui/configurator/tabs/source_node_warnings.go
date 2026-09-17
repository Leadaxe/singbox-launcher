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

// sourceWarnedNodes — сколько узлов источника несут хоть одну деградацию.
//
// Считается по СОСТАВУ, а не по эмиссии: warnings принадлежат записи узла и
// живут в ней независимо от того, выпустила ли её сборка. Узловой источник
// (server/chain/auto) состава не имеет — там узел и есть сам источник.
func sourceWarnedNodes(src *wizardmodels.Source) int {
	if src == nil {
		return 0
	}
	if len(src.Nodes) == 0 {
		if len(src.Warnings) > 0 {
			return 1
		}
		return 0
	}
	n := 0
	for i := range src.Nodes {
		if len(src.Nodes[i].Warnings) > 0 {
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
// Пустая строка = показывать нечего.
func previewWarningsSummary(rows []previewRow) string {
	fields, nodeLevel, nodes := 0, 0, 0
	for _, r := range rows {
		if len(r.Warnings) == 0 {
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
// Wrapping у каждой строки обязателен (Л19): путь поля вместе с заголовком
// кода длиннее окна, и Label без переноса раздул бы его на весь экран.
func previewWarningsBlock(rows []previewRow) fyne.CanvasObject {
	summary := previewWarningsSummary(rows)
	if summary == "" {
		return nil
	}
	items := make([]fyne.CanvasObject, 0, 8)
	for _, r := range rows {
		if len(r.Warnings) == 0 {
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
	if len(items) == 0 {
		return nil
	}
	acc := widget.NewAccordion(widget.NewAccordionItem(summary, container.NewVBox(items...)))
	return acc
}

// nodeWarningsOfSource — деградации узла источника по его сырому тегу.
//
// Нужен окнам узла, которые адресуют узел тегом в рамках источника
// (идентичность, SPEC 112), а не индексом строки: пока висит окно, состав
// вправе поехать фоновым fetch'ем.
func nodeWarningsOfSource(src *wizardmodels.Source, rawTag string) []corestate.NodeWarning {
	if src == nil || rawTag == "" {
		return nil
	}
	for i := range src.Nodes {
		if src.Nodes[i].Tag == rawTag {
			return src.Nodes[i].Warnings
		}
	}
	if src.Tag == rawTag {
		return src.Warnings
	}
	return nil
}
