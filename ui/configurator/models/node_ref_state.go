// File node_ref_state.go — модель якоря правил узла (SPEC 121, kind=node).
//
// Узел может нести с собой правила маршрута; на оси порядка они встают ОДНИМ
// якорем с именем узла. Якорь — ссылка `{FolderID, Tag}` плюс то, что
// принадлежит пользователю: позиция на оси и тумблер. Тело правил живёт у
// узла, поэтому здесь его нет и здесь его не правят.
//
// Отличие от PresetRefState: якорь ПРОИЗВОДНЫЙ. Он появляется, когда у узла
// есть непустые `sections.rules`, и исчезает вместе с ними или с узлом —
// пересевом (SeedNodeRefsFromSources), а не действием пользователя.
package models

import (
	"encoding/json"

	corestate "singbox-launcher/core/state"
)

// NodeRefState — UI-состояние одного якоря правил узла.
type NodeRefState struct {
	// FolderID — ULID контейнера; "" = узел лежит в корне списка источников.
	FolderID string

	// Tag — СЫРОЙ тег узла в его контейнере (идентичность, SPEC 112).
	Tag string

	// Enabled — включён ли якорь. Выключенный якорь не отдаёт правила в
	// конфиг, но остаётся в списке: это выбор пользователя, а не отсутствие
	// узла.
	Enabled bool

	// OrderNum — позиция на разреженной оси порядка (SPEC 106, D-051).
	// nil — якорь ещё не размечен; ось доразметит при следующей загрузке.
	OrderNum *int
}

// Link — ссылка на узел в форме NodeLink.
func (n *NodeRefState) Link() corestate.NodeLink {
	if n == nil {
		return corestate.NodeLink{}
	}
	return corestate.NodeLink{FolderID: n.FolderID, Tag: n.Tag}
}

// Clone — глубокая копия.
func (n *NodeRefState) Clone() *NodeRefState {
	if n == nil {
		return nil
	}
	cp := &NodeRefState{FolderID: n.FolderID, Tag: n.Tag, Enabled: n.Enabled}
	if n.OrderNum != nil {
		v := *n.OrderNum
		cp.OrderNum = &v
	}
	return cp
}

// SeedNodeRefsFromSources пересевает model.NodeRefs по текущему составу
// источников (SPEC 121 §4 п. 6, зеркало state.SeedNodeRules).
//
// Якорь заводится каждому узлу с непустыми `sections.rules`, которого ещё нет
// в списке, — с номером NodeRuleDefaultNum; якорь узла, который исчез или
// потерял правила, снимается. Позиция и тумблер уже существующего якоря
// сохраняются: они принадлежат пользователю.
//
// Возвращает true, если состав якорей изменился, — вызывающий по этому
// признаку пересобирает слоты и перерисовывает вкладку.
func SeedNodeRefsFromSources(m *WizardModel) bool {
	if m == nil {
		return false
	}
	links := corestate.NodeSectionLinks(m.Sources)

	existing := make(map[corestate.NodeLink]*NodeRefState, len(m.NodeRefs))
	for _, nr := range m.NodeRefs {
		if nr == nil || nr.Tag == "" {
			continue
		}
		if _, dup := existing[nr.Link()]; dup {
			continue
		}
		existing[nr.Link()] = nr
	}

	out := make([]*NodeRefState, 0, len(links))
	seen := make(map[corestate.NodeLink]bool, len(links))
	for _, l := range links {
		if l.Tag == "" || seen[l] {
			continue
		}
		seen[l] = true
		if cur, ok := existing[l]; ok {
			out = append(out, cur)
			continue
		}
		num := corestate.NodeRuleDefaultNum
		out = append(out, &NodeRefState{
			FolderID: l.FolderID,
			Tag:      l.Tag,
			Enabled:  true,
			OrderNum: &num,
		})
	}

	changed := len(out) != len(m.NodeRefs)
	if !changed {
		for i := range out {
			if out[i] != m.NodeRefs[i] {
				changed = true
				break
			}
		}
	}
	m.NodeRefs = out
	return changed
}

// SyncNodeRefsToStateRules — UI → state: якоря как записи kind=node.
//
// Используется fallback-путём эмиссии (RuleOrder пуст); основной путь —
// EmitStateRulesInAxisOrder, который обходит слоты.
func SyncNodeRefsToStateRules(refs []*NodeRefState) []corestate.Rule {
	if len(refs) == 0 {
		return nil
	}
	out := make([]corestate.Rule, 0, len(refs))
	for _, nr := range refs {
		r := nodeRefToStateRule(nr)
		if r != nil {
			out = append(out, *r)
		}
	}
	return out
}

// SyncStateRulesToNodeRefs — state → UI: якоря из state.Rules.
//
// Ссылки на узлы, которых уже нет, здесь НЕ отсеиваются: это делает пересев
// (SeedNodeRefsFromSources) — он один знает текущий состав источников, и
// второе место с тем же знанием разошлось бы с ним.
func SyncStateRulesToNodeRefs(rules []corestate.Rule) []*NodeRefState {
	if len(rules) == 0 {
		return nil
	}
	out := make([]*NodeRefState, 0, len(rules))
	for _, r := range rules {
		if r.Kind != corestate.RuleKindNode {
			continue
		}
		body, err := r.DecodeBody()
		if err != nil {
			continue
		}
		nb, _ := body.(*corestate.NodeRuleBody)
		if nb == nil || nb.Tag == "" {
			continue
		}
		out = append(out, &NodeRefState{
			FolderID: nb.FolderID,
			Tag:      nb.Tag,
			Enabled:  r.Enabled,
			OrderNum: copyOrderNum(r.OrderNum),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// nodeRefToStateRule — одна запись kind=node из якоря.
func nodeRefToStateRule(nr *NodeRefState) *corestate.Rule {
	if nr == nil || nr.Tag == "" {
		return nil
	}
	body, err := jsonMarshalNodeRuleBody(nr)
	if err != nil {
		return nil
	}
	return &corestate.Rule{
		Kind:     corestate.RuleKindNode,
		Enabled:  nr.Enabled,
		OrderNum: copyOrderNum(nr.OrderNum),
		Body:     body,
	}
}

// NodeRefRulesCount — сколько правил маршрута несёт узел якоря.
//
// Считается по источникам модели: у якоря своего тела нет, и число для
// подписи строки берётся там же, где живут сами правила.
func NodeRefRulesCount(m *WizardModel, nr *NodeRefState) int {
	if m == nil || nr == nil {
		return 0
	}
	node := FindNodeByLink(m, nr.Link())
	if node == nil || node.Sections == nil {
		return 0
	}
	return len(node.Sections.Rules)
}

// NodeRefNodeEnabled — включён ли САМ узел якоря (не якорь).
//
// Выключенный узел в конфиг не едет, и его фрагментов там нет — строка обязана
// сказать об этом, иначе выключенный узел читается как работающее правило.
// Узел не найден → считаем выключенным: показывать «работает» нечему.
func NodeRefNodeEnabled(m *WizardModel, nr *NodeRefState) bool {
	if m == nil || nr == nil {
		return false
	}
	node := FindNodeByLink(m, nr.Link())
	return node != nil && node.Enabled
}

// jsonMarshalNodeRuleBody — тело записи kind=node из якоря.
func jsonMarshalNodeRuleBody(nr *NodeRefState) ([]byte, error) {
	return json.Marshal(corestate.NodeRuleBody{FolderID: nr.FolderID, Tag: nr.Tag})
}

// FindNodeByLink — узел источников по ссылке {FolderID, Tag}.
//
// Корневой узел адресуется тем же именем, что его знает конфиг
// (NodeTagOrLabel) — так же, как его собирает state.NodeSectionLinks.
func FindNodeByLink(m *WizardModel, link corestate.NodeLink) *corestate.Node {
	if m == nil || link.Tag == "" {
		return nil
	}
	for i := range m.Sources {
		src := &m.Sources[i]
		if link.FolderID == "" {
			if src.Kind == corestate.SourceKindServer && src.NodeTagOrLabel() == link.Tag {
				return &src.Node
			}
			continue
		}
		if src.ID != link.FolderID {
			continue
		}
		for j := range src.Nodes {
			if src.Nodes[j].Tag == link.Tag {
				return &src.Nodes[j]
			}
		}
	}
	return nil
}
