// File source_edit_json.go — хелперы вкладки JSON окна Source.
//
// Вкладка показывает, как источник распакуется в sing-box, через ту же точку
// эмиссии, что и реальная сборка (config.EmitNodeJSONs) — WYSIWYG. Эмиттер
// возвращает строки в «конфиговом» виде ("\t{...}," + строка-комментарий) —
// здесь они приводятся к чистому JSON для показа и редактирования.
package tabs

import (
	"bytes"
	"encoding/json"
	"strings"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/locale"
)

// stripEmittedDecorations снимает с эмитированной строки то, что делает её
// фрагментом конфига, а не самостоятельным JSON: строки-комментарии
// ("// label"), ведущие табы и хвостовую запятую.
func stripEmittedDecorations(s string) string {
	lines := strings.Split(s, "\n")
	kept := lines[:0]
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "//") {
			continue
		}
		kept = append(kept, ln)
	}
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	return strings.TrimSuffix(out, ",")
}

// emittedToEditableJSON превращает строку эмиттера в pretty JSON-объект.
//
// json.Indent (а не Unmarshal→MarshalIndent): работает на токенах и сохраняет
// порядок полей эмиттера — tag/type первыми, как в config.json.
func emittedToEditableJSON(s string) string {
	clean := stripEmittedDecorations(s)
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(clean), "", "  "); err != nil {
		return clean
	}
	return buf.String()
}

// renderUnpackedNodes эмитит узлы подписки тем же кодом, что и сборка, и
// собирает их в один читаемый документ {"outbounds":[...],"endpoints":[...]}
// — те самые записи, которые источник добавит в config.json.
//
// Возвращает (текст, статус-строка). Узлы, которые эмиттер отверг, молча
// пропускаются (он сам пишет warn в лог) — так же ведёт себя сборка.
func renderUnpackedNodes(nodes []*config.ParsedNode) (string, string) {
	// json.RawMessage, а не map: MarshalIndent переиндентирует вложенные
	// объекты, не трогая порядок полей внутри них.
	type unpackedDoc struct {
		Outbounds []json.RawMessage `json:"outbounds,omitempty"`
		Endpoints []json.RawMessage `json:"endpoints,omitempty"`
	}
	doc := unpackedDoc{}
	emitted := 0
	truncated := false
	for _, node := range nodes {
		if emitted >= previewNodeCap {
			// Обрезаем ВЫВОД, а не состав: узлы все на месте, но
			// MultiLineEntry без виртуализации подвешивает окно на
			// полутысяче outbound'ов (fyne-io/fyne#2935).
			truncated = true
			break
		}
		outJSONs, epJSON, err := config.EmitNodeJSONs(node)
		if err != nil {
			continue
		}
		if epJSON != "" {
			doc.Endpoints = append(doc.Endpoints, json.RawMessage(stripEmittedDecorations(epJSON)))
			emitted++
			continue
		}
		for _, oj := range outJSONs {
			doc.Outbounds = append(doc.Outbounds, json.RawMessage(stripEmittedDecorations(oj)))
		}
		emitted++
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err.Error()
	}
	status := locale.Tf("Unpacked nodes: %d", len(nodes))
	if truncated {
		status += " " + locale.Tf("(showing first %d)", previewNodeCap)
	}
	return string(b), status
}
