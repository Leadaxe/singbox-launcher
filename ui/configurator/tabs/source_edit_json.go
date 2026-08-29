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

// unpackedNodesResult — итог сборки документа из эмитированных узлов.
type unpackedNodesResult struct {
	// Text — документ {"outbounds":[...],"endpoints":[...]} или "" при
	// ошибке сериализации (тогда заполнен Err).
	Text string
	// Emitted — сколько узлов реально попало в документ.
	Emitted int
	// Dropped — сколько узлов эмиттер отверг (он сам пишет warn в лог, так
	// же ведёт себя сборка). Считаются только те, до которых дошли: после
	// обрыва по лимиту остаток не разбирался.
	Dropped int
	// Truncated — вывод оборван лимитом (в состав узлов это не вносит
	// изменений: обрезается ТЕКСТ, а не папка).
	Truncated bool
	Err       error
}

// unpackNodesDoc эмитит узлы тем же кодом, что и сборка (config.EmitNodeJSONs
// — единственная точка эмиссии, SPEC 116 §O2 вариант А), и собирает их в один
// документ {"outbounds":[...],"endpoints":[...]} — те самые записи, которые
// источник добавит в config.json, с ФИНАЛЬНЫМИ тегами (тег-машина отработала
// в EmitCanonicalSource у вызывающего).
//
// limit <= 0 означает «без лимита»: показ в MultiLineEntry обрезается, а
// выгрузка в буфер — нет, иначе «взять всю папку» отдало бы не всю папку.
func unpackNodesDoc(nodes []*config.ParsedNode, limit int) unpackedNodesResult {
	// json.RawMessage, а не map: MarshalIndent переиндентирует вложенные
	// объекты, не трогая порядок полей внутри них.
	type unpackedDoc struct {
		Outbounds []json.RawMessage `json:"outbounds,omitempty"`
		Endpoints []json.RawMessage `json:"endpoints,omitempty"`
	}
	doc := unpackedDoc{}
	res := unpackedNodesResult{}
	for _, node := range nodes {
		if limit > 0 && res.Emitted >= limit {
			// Обрезаем ВЫВОД, а не состав: узлы все на месте, но
			// MultiLineEntry без виртуализации подвешивает окно на
			// полутысяче outbound'ов (fyne-io/fyne#2935).
			res.Truncated = true
			break
		}
		outJSONs, epJSON, err := config.EmitNodeJSONs(node)
		if err != nil {
			res.Dropped++
			continue
		}
		if epJSON != "" {
			doc.Endpoints = append(doc.Endpoints, json.RawMessage(stripEmittedDecorations(epJSON)))
			res.Emitted++
			continue
		}
		for _, oj := range outJSONs {
			doc.Outbounds = append(doc.Outbounds, json.RawMessage(stripEmittedDecorations(oj)))
		}
		res.Emitted++
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		res.Err = err
		return res
	}
	res.Text = string(b)
	return res
}

// renderUnpackedNodes — вкладка JSON окна источника: тот же документ, но с
// лимитом на длину вывода.
//
// Возвращает (текст, статус-строка). Узлы, которые эмиттер отверг, молча
// пропускаются (он сам пишет warn в лог) — так же ведёт себя сборка.
func renderUnpackedNodes(nodes []*config.ParsedNode) (string, string) {
	res := unpackNodesDoc(nodes, previewNodeCap)
	if res.Err != nil {
		return "", res.Err.Error()
	}
	status := locale.Tf("Unpacked nodes: %d", len(nodes))
	if res.Truncated {
		status += " " + locale.Tf("(showing first %d)", previewNodeCap)
	}
	return res.Text, status
}
