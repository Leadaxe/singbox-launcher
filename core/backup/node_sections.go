// File node_sections.go — поле `sections` на входе бэкапа.
//
// Секции узла упразднены (контракт 1.1.85, NODE_SECTIONS.md; решение
// владельца 27.09.2026, LxBox §575): экспорт поле не пишет, импорт снимает
// его у записи ЛЮБОГО вида. Если в снятом поле была хотя бы одна запись
// (правило, DNS-сервер или DNS-правило), потеря называется кодом
// `backup_section_record_dropped` с причиной `not_allowed`; пустой набор
// снимается молча.
package backup

import (
	"encoding/json"
	"fmt"
)

// SectionDropNotAllowed — причина `backup_section_record_dropped`: записи
// этого вида секции не положены (с контракта 1.1.85 — никакой).
const SectionDropNotAllowed = "not_allowed"

// sectionsRecordCount — сколько записей несёт сырой блок `sections`:
// `rules[]`, `dns.servers[]`, `dns.rules[]`. Нечитаемый блок считается
// непустым: молча выбросить то, что не удалось прочитать, нельзя (П3).
func sectionsRecordCount(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var probe struct {
		Rules []json.RawMessage `json:"rules"`
		DNS   *struct {
			Servers []json.RawMessage `json:"servers"`
			Rules   []json.RawMessage `json:"rules"`
		} `json:"dns"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return 1
	}
	n := len(probe.Rules)
	if probe.DNS != nil {
		n += len(probe.DNS.Servers) + len(probe.DNS.Rules)
	}
	return n
}

// sectionsDroppedWarning — предупреждение о снятом непустом `sections`
// записи с тегом tag и видом kind; nil для пустого набора.
func sectionsDroppedWarning(raw json.RawMessage, tag, kind string) []Warning {
	if sectionsRecordCount(raw) == 0 {
		return nil
	}
	return []Warning{{
		Code:   WarnBackupSectionRecordDropped,
		Detail: tag + ": sections",
		Kind:   kind,
		Reason: SectionDropNotAllowed,
	}}
}

// scanSections10 обходит файл 1.0 и называет каждую запись `sources[]` и
// каждый член `sources[].nodes[]` с непустым полем `sections`.
//
// Отдельный проход по сырому файлу, а не по разобранной модели: у модели
// поля нет, а норма требует кода у записи любого вида (подписка, цепочка,
// Направление-член — всё равно).
func scanSections10(data []byte) []Warning {
	var root struct {
		Sources []map[string]json.RawMessage `json:"sources"`
	}
	if json.Unmarshal(data, &root) != nil {
		return nil
	}
	var warns []Warning
	record := func(item map[string]json.RawMessage, fallback string) {
		raw, ok := item["sections"]
		if !ok {
			return
		}
		var tag, kind string
		_ = json.Unmarshal(item["tag"], &tag)
		_ = json.Unmarshal(item["kind"], &kind)
		if tag == "" {
			_ = json.Unmarshal(item["name"], &tag)
		}
		if tag == "" {
			tag = fallback
		}
		warns = append(warns, sectionsDroppedWarning(raw, tag, kind)...)
	}
	for i, src := range root.Sources {
		record(src, fmt.Sprintf("sources[%d]", i))
		var nodes []map[string]json.RawMessage
		if raw, ok := src["nodes"]; ok && json.Unmarshal(raw, &nodes) == nil {
			for j, n := range nodes {
				record(n, fmt.Sprintf("sources[%d].nodes[%d]", i, j))
			}
		}
	}
	return warns
}
