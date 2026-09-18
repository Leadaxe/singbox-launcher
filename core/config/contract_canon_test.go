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
	V       int            `json:"v"`
	Nodes   []contractNode `json:"nodes"`
	Dropped []contractDrop `json:"dropped,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

type contractNode struct {
	Kind   string         `json:"kind"`
	Scheme string         `json:"scheme"`
	Label  string         `json:"label,omitempty"`
	Entry  map[string]any `json:"entry"`
	// Sections — связка узла, извлечённая из ЦЕЛОГО sing-box-конфига
	// (NODE_SECTIONS.md §7/§8): правила маршрута, DNS-серверы и DNS-правила,
	// ссылающиеся на этот узел. Форма — §2 контракта, ссылки на узел уже
	// переписаны в `@self`.
	//
	// Почему не `map[string]any`, а сырой JSON после канонизации: секции
	// приезжают из парсера непрозрачным блоком, и раскладывать их в
	// типизированную структуру раннера значило бы завести ВТОРОЙ читатель
	// формы записей — ровно то, что запрещает NODE_SECTIONS.md §1.
	//
	// Отсутствует у узла без секций (обычное тело, узел не один в конфиге).
	Sections json.RawMessage `json:"sections,omitempty"`
	Chain    []contractNode  `json:"chain,omitempty"`
	// Warnings — записи деградаций конверта (CANON §6, контракт 1.1.0):
	// {code, path?, value?}. `params` раннер пока не пишет — их ставит
	// санитайзер реестра (W2a), до него параметров ни у одного кода нет.
	//
	// Старые ожидания корпуса несут здесь ГОЛЫЕ СТРОКИ-коды; сравнение это
	// терпит (normalizeWarningsForCompare): строка в ожидании = «нормативен
	// только код». Перегенерировать весь корпус ради формы записи — значит
	// смешать шум с настоящими расхождениями.
	Warnings []contractWarning `json:"warnings,omitempty"`
}

// contractWarning — запись warnings[] конверта.
type contractWarning struct {
	Code  string `json:"code"`
	Path  string `json:"path,omitempty"`
	Value string `json:"value,omitempty"`
}

// contractDrop — запись отбраковки в конверте (D-088).
//
// Нормативны `ref` (что именно отвергнуто) и `code` (машинная причина из
// registry/warnings.json). `reason` — человеческий текст СТОРОНЫ: у нас это
// формат ошибки Go, у LxBox — свой; сравнением он не покрывается, иначе
// вторая сторона была бы обязана копировать наши строки. `code` необязателен:
// в старых URI-ожиданиях его нет, там нормативен только `ref`.
type contractDrop struct {
	Ref    string `json:"ref"`
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason"`
}

// canonNodeDrop — canonNode плюс МАШИННАЯ причина отказа.
//
// `code` в записи отбраковки нормативен (D-088, corpus/README §«Отбраковки»),
// а `reason` — нет: он человеческий текст стороны. Пока раннеры знали только
// текст ошибки Go, любая отбраковка приезжала в конверт без кода, и вторая
// сторона сверяла у неё ровно одно поле — `ref`. Отличить «узел выброшен за
// негодный ключ» от «узел выброшен за пересечение заголовков» такой конверт
// не позволял, то есть перенос правил в реестр проверить было нечем.
//
// Код берётся у отказа санитайзера. Пустая строка = отказ пришёл не от него
// (ошибка эмиссии, битый JSON) — там кода и нет.
func canonNodeDrop(node *configtypes.ParsedNode) (contractNode, string, error) {
	if node != nil && node.Scheme != configtypes.SchemeGroup {
		if _, _, drop := materializeParsedNodeBody(node); drop != nil {
			cn, err := canonNode(node)
			return cn, drop.Code, err
		}
	}
	cn, err := canonNode(node)
	return cn, "", err
}

// canonNode превращает разобранный узел в канонический вид конверта.
func canonNode(node *configtypes.ParsedNode) (contractNode, error) {
	if node == nil {
		return contractNode{}, fmt.Errorf("nil node")
	}

	kind := "outbound"
	var entry map[string]any
	// Коды деградации (SPEC 103, фаза 2) — часть контракта: они отвечают на
	// вопрос «что узлу отняли при разборе», и расхождение кодов между
	// приложениями означает, что одно из них молча портит узел.
	//
	// Порядок — как проставлен разбором (CANON §6, Л14), без сортировки:
	// последовательность слоёв нормативна, и сортировка кодов скрыла бы
	// расхождение в том, ЧТО именно сработало первым.
	nodeWarnings := node.Warnings

	if node.Scheme == configtypes.SchemeGroup {
		// Группа тела не имеет: у неё нет ни схемы в реестре, ни полей для
		// санитайзера — её форму задаёт свой эмиттер.
		kind = "group"
		raw, err := GenerateNodeJSON(node)
		if err != nil {
			return contractNode{}, fmt.Errorf("emit %s: %w", node.Scheme, err)
		}
		entry, err = decodeEmittedEntry(raw)
		if err != nil {
			return contractNode{}, fmt.Errorf("decode emitted %s: %w", node.Scheme, err)
		}
	} else {
		// SPEC 131 W2c: конверт корпуса показывает ровно то тело, которое
		// лаунчер СОХРАНИТ, — то есть выход конвейера. Пока здесь стоял
		// GenerateNodeJSON, ожидание описывало per-scheme эмиттер, а в
		// state.Node.Body уезжало другое: контракт сверял не тот артефакт,
		// который живёт.
		//
		// Предикат схемы-endpoint'а нужен только для поля kind конверта:
		// тело обе ветки получают одним конвейером (CANON §2.3).
		if IsEndpointScheme(node.Scheme) {
			kind = "endpoint"
		}
		body, warns, drop := materializeParsedNodeBody(node)
		if drop != nil {
			return contractNode{}, fmt.Errorf("emit %s: %s", node.Scheme, dropReason(drop))
		}
		nodeWarnings = warns
		if err := json.Unmarshal(body, &entry); err != nil {
			return contractNode{}, fmt.Errorf("decode emitted %s: %w", node.Scheme, err)
		}
	}

	// CANON §2.1-2.2: tag и detour в канон не входят.
	delete(entry, "tag")
	delete(entry, "detour")

	var warnings []contractWarning
	for _, w := range nodeWarnings {
		warnings = append(warnings, contractWarning{Code: w.Code, Path: w.Path, Value: w.Value})
	}

	sections, err := canonNodeSections(node)
	if err != nil {
		return contractNode{}, fmt.Errorf("sections %s: %w", node.Scheme, err)
	}

	out := contractNode{
		Kind:     kind,
		Scheme:   node.Scheme,
		Label:    node.Label,
		Entry:    canonValue(entry).(map[string]any),
		Sections: sections,
		Warnings: warnings,
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

// canonNodeSections — секции узла в форме конверта (NODE_SECTIONS.md §8,
// договорённость с LxBox от 14.09.2026).
//
// Из записей снимаются `id` и `num`. Оба — МЕТАДАННЫЕ ОСИ принимающей
// стороны, а не свойство тела: `num` раздаётся извлечением по порядку правил
// узла (NodeRuleDefaultNum и дальше), а на приёмнике всё равно
// перенумеровывается импортом; `id` лаунчер не генерирует вовсе. Оставь их в
// ожидании — и раннер второй стороны, у которой своя нумерация, падал бы на
// каждом кейсе с секциями, ничего содержательного при этом не проверив.
//
// Порядок записей сохраняется: он нормативен (правила узла встают на ось
// подряд, и перестановка меняет маршрутизацию).
func canonNodeSections(node *configtypes.ParsedNode) (json.RawMessage, error) {
	if node == nil || node.Sections == nil || len(node.Sections.Raw) == 0 {
		return nil, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(node.Sections.Raw, &decoded); err != nil {
		return nil, fmt.Errorf("разбор секций: %w", err)
	}
	if rules, ok := decoded["rules"].([]any); ok {
		for _, r := range rules {
			rec, ok := r.(map[string]any)
			if !ok {
				continue
			}
			delete(rec, "id")
			delete(rec, "num")
		}
	}
	if dns, ok := decoded["dns"].(map[string]any); ok {
		for _, key := range []string{"servers", "rules"} {
			list, ok := dns[key].([]any)
			if !ok {
				continue
			}
			for _, item := range list {
				rec, ok := item.(map[string]any)
				if !ok {
					continue
				}
				delete(rec, "id")
				delete(rec, "num")
			}
		}
	}
	raw, err := canonMarshal(canonValue(decoded))
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
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
