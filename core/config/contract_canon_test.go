package config

// Канонизатор для конформанс-корпуса контракта (SPEC 103, contract/docs/CANON.md).
//
// Живёт в пакете config (а не subscription): эмиссия GenerateNodeJSON здесь,
// а config уже импортирует subscription — обратный импорт дал бы цикл.
//
// Файл намеренно с суффиксом _test.go: контракт-код не должен попадать
// в обычную сборку (в т.ч. в легаси-джобу win7 на go1.20).

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// contractEnvelope — форма contract/schema/node.schema.json.
type contractEnvelope struct {
	V       int              `json:"v"`
	Nodes   []contractNode   `json:"nodes"`
	Dropped []contractDrop   `json:"dropped,omitempty"`
	Meta    map[string]any   `json:"meta,omitempty"`
}

type contractNode struct {
	Kind     string         `json:"kind"`
	Scheme   string         `json:"scheme"`
	Label    string         `json:"label,omitempty"`
	Entry    map[string]any `json:"entry"`
	Chain    []contractNode `json:"chain,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

type contractDrop struct {
	Ref    string `json:"ref"`
	Reason string `json:"reason"`
}

// canonNode превращает разобранный узел в канонический вид конверта.
func canonNode(node *configtypes.ParsedNode) (contractNode, error) {
	if node == nil {
		return contractNode{}, fmt.Errorf("nil node")
	}

	kind := "outbound"
	var raw string
	var err error
	switch {
	case node.Scheme == configtypes.SchemeGroup:
		kind = "group"
		raw, err = GenerateNodeJSON(node)
	case node.Scheme == "wireguard":
		kind = "endpoint"
		raw, err = GenerateEndpointJSON(node)
	default:
		raw, err = GenerateNodeJSON(node)
	}
	if err != nil {
		return contractNode{}, fmt.Errorf("emit %s: %w", node.Scheme, err)
	}

	entry, err := decodeEmittedEntry(raw)
	if err != nil {
		return contractNode{}, fmt.Errorf("decode emitted %s: %w", node.Scheme, err)
	}

	// CANON §2.1-2.2: tag и detour в канон не входят.
	delete(entry, "tag")
	delete(entry, "detour")

	out := contractNode{
		Kind:   kind,
		Scheme: node.Scheme,
		Label:  node.Label,
		Entry:  canonValue(entry).(map[string]any),
	}

	for _, hop := range node.Chain {
		hopNode, err := canonNode(hop)
		if err != nil {
			return contractNode{}, fmt.Errorf("chain hop: %w", err)
		}
		out.Chain = append(out.Chain, hopNode)
	}
	return out, nil
}

// decodeEmittedEntry вытаскивает JSON-объект из эмитированного фрагмента
// вида "\t// <label>\n\t{...},".
func decodeEmittedEntry(raw string) (map[string]any, error) {
	start := strings.Index(raw, "{")
	if start < 0 {
		return nil, fmt.Errorf("no JSON object in emitted fragment")
	}
	body := strings.TrimSpace(raw[start:])
	body = strings.TrimSuffix(body, ",")

	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// canonValue рекурсивно приводит значение к каноническому виду:
// целые float64 → int64 (CANON §2.5 «числа — числами»), порядок списков
// сохраняется, ключи map сортируются при маршалинге (canonMarshal).
func canonValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = canonValue(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = canonValue(val)
		}
		return out
	case float64:
		if t == float64(int64(t)) {
			return int64(t)
		}
		return t
	default:
		return v
	}
}

// canonMarshal сериализует значение по правилам CANON §2:
// сортировка ключей, компактно, без HTML-escaping (D-007).
func canonMarshal(v any) ([]byte, error) {
	var sb strings.Builder
	if err := writeCanonJSON(&sb, v); err != nil {
		return nil, err
	}
	return []byte(sb.String()), nil
}

func writeCanonJSON(sb *strings.Builder, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sb.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				sb.WriteByte(',')
			}
			if err := writeCanonScalar(sb, k); err != nil {
				return err
			}
			sb.WriteByte(':')
			if err := writeCanonJSON(sb, t[k]); err != nil {
				return err
			}
		}
		sb.WriteByte('}')
		return nil
	case []any:
		sb.WriteByte('[')
		for i, val := range t {
			if i > 0 {
				sb.WriteByte(',')
			}
			if err := writeCanonJSON(sb, val); err != nil {
				return err
			}
		}
		sb.WriteByte(']')
		return nil
	default:
		return writeCanonScalar(sb, v)
	}
}

// writeCanonScalar пишет скаляр без HTML-escaping (CANON §2.7 / D-007).
func writeCanonScalar(sb *strings.Builder, v any) error {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	sb.WriteString(strings.TrimRight(buf.String(), "\n"))
	return nil
}
