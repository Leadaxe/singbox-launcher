package build

import (
	"encoding/json"
	"fmt"
	"strings"
)

// maxNodesForFullPreview — порог, выше которого preview не выводит узлы
// поэлементно, а заменяет их на сводный комментарий. Большие подписки
// (4000 нод) дают неюзабельный preview-блок, и комментарий читается лучше.
//
// Ссылка: ui/wizard/utils/constants.go::MaxNodesForFullPreview (константа
// продолжает существовать там для wizard-side UI; копия здесь — чтобы
// core/build не импортировал ui/).
const maxNodesForFullPreview = 30

// PreviewStats — счётчики, нужные для рендера preview-сводки. Source-of-truth
// для них — `core/config.OutboundGenerationResult`; в wizard это `OutboundStats`.
//
// При forPreview=false значение не используется — секция и так пустая между
// маркерами, parser-update наливает данные позже (legacy путь) или
// outboundscache (новый путь, SPEC 045).
type PreviewStats struct {
	NodesCount           int
	LocalSelectorsCount  int
	GlobalSelectorsCount int
	EndpointsCount       int
}

// BuildOutboundsSection — формирует JSON-секцию `outbounds` с маркерами
// `/** @ParserSTART */` / `/** @ParserEND */` и статическими элементами из
// шаблона.
//
// Поведение определяется тройкой (generatedOutbounds, forPreview, stats):
//
//   - generatedOutbounds пуст → между маркерами ничего;
//   - generatedOutbounds непуст AND (forPreview=false OR stats.NodesCount ≤
//     maxNodesForFullPreview) → все элементы рендерятся inline между маркерами;
//   - generatedOutbounds непуст AND forPreview=true AND stats.NodesCount >
//     maxNodesForFullPreview → текстовая сводка вместо узлов (UI-only).
//
// Это обединяет legacy-пути:
//   - `BuildTemplateConfig(forPreview=false)` + `populateCheckText` (Save-pipe)
//     → cache наливается ВСЕГДА, без truncation;
//   - `BuildTemplateConfig(forPreview=true)` (UI preview) → c truncation
//     для больших подписок.
//
// Pure: без I/O, без shared state.
//
// Ссылка legacy: ui/wizard/business/create_config.go::buildOutboundsSection.
func BuildOutboundsSection(
	templateOutbounds json.RawMessage,
	generatedOutbounds []string,
	forPreview bool,
	stats PreviewStats,
) (string, error) {
	var staticOutbounds []json.RawMessage
	_ = json.Unmarshal(templateOutbounds, &staticOutbounds)

	indent := Indent(2)
	var b strings.Builder
	b.WriteString("[\n")

	hasDynamic := false
	b.WriteString(indent + "/** @ParserSTART */\n")
	switch {
	case len(generatedOutbounds) == 0:
		// nothing between markers
	case forPreview && stats.NodesCount > maxNodesForFullPreview:
		b.WriteString(fmt.Sprintf(
			"%s// Generated: %d nodes, %d local selectors, %d global selectors\n",
			indent, stats.NodesCount, stats.LocalSelectorsCount, stats.GlobalSelectorsCount))
		b.WriteString(fmt.Sprintf("%s// Total outbounds: %d\n",
			indent, len(generatedOutbounds)))
	default:
		// Compact entries (outbounds) идут одной строкой с `\t`-префиксом —
		// формат legacy populateCheckText (без дополнительного 4-space indent
		// от IndentMultiline). Между маркерами получается:
		//
		//   /** @ParserSTART */
		//   \t{"tag":...},
		//   \t{"tag":...}
		//   /** @ParserEND */
		for idx, entry := range generatedOutbounds {
			cleaned := strings.TrimRight(entry, ",\n\r\t ")
			b.WriteString("\t")
			b.WriteString(cleaned)
			if idx < len(generatedOutbounds)-1 || len(staticOutbounds) > 0 {
				b.WriteString(",")
			}
			b.WriteString("\n")
			hasDynamic = true
		}
	}
	// END marker — без leading indent (legacy quirk: PopulateParserMarkers
	// чопал всё что перед маркером, т.к. endIdx указывал на `/`).
	b.WriteString("/** @ParserEND */")

	appendStaticEntries(&b, staticOutbounds, hasDynamic, indent)

	b.WriteString("\n  ]")
	return b.String(), nil
}

// BuildEndpointsSection — аналогично BuildOutboundsSection, но для секции
// `endpoints` (WireGuard-узлы) с маркерами `/** @ParserSTART_E */` /
// `/** @ParserEND_E */`. Preview-сводка короче (только EndpointsCount).
//
// Ссылка legacy: ui/wizard/business/create_config.go::buildEndpointsSection.
func BuildEndpointsSection(
	templateEndpoints json.RawMessage,
	generatedEndpoints []string,
	forPreview bool,
	stats PreviewStats,
) (string, error) {
	var staticEndpoints []json.RawMessage
	_ = json.Unmarshal(templateEndpoints, &staticEndpoints)

	indent := Indent(2)
	var b strings.Builder
	b.WriteString("[\n")

	hasDynamic := false
	b.WriteString(indent + "/** @ParserSTART_E */\n")
	switch {
	case len(generatedEndpoints) == 0:
		// nothing between markers
	case forPreview && stats.EndpointsCount > maxNodesForFullPreview:
		b.WriteString(fmt.Sprintf("%s// Generated: %d endpoints (WireGuard)\n",
			indent, stats.EndpointsCount))
	default:
		for idx, entry := range generatedEndpoints {
			cleaned := strings.TrimRight(entry, ",\n\r\t ")
			b.WriteString(IndentMultiline(cleaned, indent))
			if idx < len(generatedEndpoints)-1 || len(staticEndpoints) > 0 {
				b.WriteString(",")
			}
			b.WriteString("\n")
			hasDynamic = true
		}
	}
	// END marker — без leading indent (legacy quirk, см. BuildOutboundsSection).
	b.WriteString("/** @ParserEND_E */")

	appendStaticEntries(&b, staticEndpoints, hasDynamic, indent)

	b.WriteString("\n  ]")
	return b.String(), nil
}

// appendStaticEntries — общий хвост обеих секций: статические записи из
// шаблона дописываются ПОСЛЕ закрывающего маркера. Разделители:
//   - если был dynamic-блок (hasDynamic=true), последний dynamic-entry уже
//     имеет trailing-запятую (см. цикл в Build*Section), поэтому первая
//     static-запись начинается с `\n` (без `,`).
//   - между static-записями всегда `,\n`.
//   - без dynamic — первая static-запись с `\n` (вместо `,\n`).
func appendStaticEntries(b *strings.Builder, entries []json.RawMessage, hasDynamic bool, indent string) {
	if len(entries) == 0 {
		return
	}
	_ = hasDynamic // переменная сохранена для документирования контракта;
	// фактически разделитель одинаков в обоих случаях: `\n` для первой,
	// `,\n` между остальными.
	for i, item := range entries {
		if i > 0 {
			b.WriteString(",\n")
		} else {
			b.WriteString("\n")
		}
		formatted, err := FormatCompactJSON(item)
		if err != nil {
			formatted = string(item)
		}
		b.WriteString(indent + formatted)
	}
}
