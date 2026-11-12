package state

import (
	"encoding/json"
	"strings"
)

// Authored — тело узла АВТОРСКОЕ (контракт 1.1.87, PARSING_PRINCIPLES
// §10.1; решение владельца 27.09.2026, LxBox §577).
//
// Условий четыре, и первое — контейнер — вызывающий обязан проверить сам:
// звать только у своего сервера в корне и у члена папки, не у узла
// подписки. Здесь три остальных: узел — сервер (не группа автовыбора),
// вид источника — `singbox` (контракт 1.1.165, §11 п.5: дословное тело —
// только у sing-box источника; `xray` — перевод, не текст автора в форме
// ядра), и текст источника разбирается как JSON (голое тело, документ или
// массив — тело узла из него берёт материализация).
//
// Условием НЕ являются: кто набрал текст, факт правки.
func (n *Node) Authored() bool {
	if n == nil || n.Kind != SourceKindServer || n.Origin == nil || n.Origin.Kind != OriginKindSingbox {
		return false
	}
	return isJSONObjectOrArray(strings.TrimSpace(n.Origin.Raw))
}

// MigrateLegacyOriginKinds — записи, у которых вид источника записан до
// контракта 1.1.165, получают вид-диалект (PARSING_PRINCIPLES §11 п.6):
//
//   - `json` (JSON любого диалекта) → классификация текста движком реестра
//     (хук ClassifyOriginKind): `singbox` либо `xray`; движок вида не дал или
//     дал не JSON-диалект → `singbox` (прежнее поведение `json`);
//   - `uri`, чей текст — JSON (правка поля Origin до того, как вид стал
//     определяться по тексту, SPEC 157) → то же правило.
//
// Текст источника НЕ меняется: документ остаётся документом (обрезка до
// узловых массивов — на входе текста, не при чтении), тело узла не
// трогается. Обходятся ВСЕ узлы с происхождением — корневые серверы, члены
// папок и подписок, включая группы и неразобранные записи: у них вид тот же.
//
// Возвращает число переписанных записей: вызывающему нужно знать, стоит ли
// сохранять файл.
func MigrateLegacyOriginKinds(s *State) int {
	if s == nil {
		return 0
	}
	n := 0
	for i := range s.Sources {
		src := &s.Sources[i]
		if migrateLegacyOriginKind(src.Node.Origin) {
			n++
		}
		for j := range src.Nodes {
			if migrateLegacyOriginKind(src.Nodes[j].Origin) {
				n++
			}
		}
	}
	return n
}

func migrateLegacyOriginKind(o *Origin) bool {
	if o == nil {
		return false
	}
	raw := strings.TrimSpace(o.Raw)
	switch o.Kind {
	case LegacyOriginKindJSON:
		o.Kind = classifyJSONOriginKind(raw)
		return true
	case OriginKindURI:
		if isJSONObjectOrArray(raw) {
			o.Kind = classifyJSONOriginKind(raw)
			return true
		}
	}
	return false
}

// classifyJSONOriginKind — вид JSON-текста по реестру; не опознан или не
// JSON-диалект → `singbox`.
func classifyJSONOriginKind(raw string) string {
	if migrationHooks.ClassifyOriginKind != nil {
		if kind := migrationHooks.ClassifyOriginKind(raw); IsJSONOriginKind(kind) {
			return kind
		}
	}
	return OriginKindSingbox
}

// isJSONObjectOrArray — текст есть валидный JSON-объект или массив. Ссылка
// и блок wg-quick так не начинаются; разбирать текст как узел здесь не
// нужно — вид говорит лишь, КАК его читать.
func isJSONObjectOrArray(raw string) bool {
	if raw == "" || (raw[0] != '{' && raw[0] != '[') {
		return false
	}
	return json.Valid([]byte(raw))
}
