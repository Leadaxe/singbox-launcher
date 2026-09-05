// File node_rule_ref.go — строки Rules для правил, которые узлы носят с собой
// (SPEC 121 §10.4).
//
// # Что это
//
// У узла в `sections.rules[]` лежат обычные записи правил лаунчера — со своими
// `enabled` и `order_num`. В списке Rules каждая из них получает СВОЮ строку:
// её видно, её можно двигать и выключать. Тела правится у узла — на вкладке
// JSON окна источника, — поэтому шестерёнки и удаления у строки нет.
//
// # Обратный указатель
//
// Строка не владеет записью: запись живёт в `Sources[..].Sections.Rules[Index]`.
// Модель держит только адрес — NodeRuleRef{Link, Index} — и синхронизация при
// Save раскладывает номера и тумблеры по домам: корневые правила в `rules[]`
// состояния, узловые обратно в свой узел.
//
// # Почему список производный
//
// Он пересевается по составу источников при каждой правке узла: правило
// появляется вместе с секцией и исчезает вместе с ней. Пользователю
// принадлежит только позиция на оси и тумблер, а не существование строки.
package models

import (
	corestate "singbox-launcher/core/state"
)

// NodeRuleRef — адрес одной записи `sections.rules[]` плюс то, что показывает
// и правит строка списка.
type NodeRuleRef struct {
	// Link — ссылка на узел-владелец ({FolderID, сырой тег}).
	Link corestate.NodeLink

	// Index — позиция записи внутри `Sections.Rules` этого узла. Обратный
	// указатель живёт в ПАМЯТИ модели: в состоянии его нет, и заводить там
	// вторую нумерацию значило бы держать два места в согласии.
	Index int

	// Enabled / OrderNum — копии полей записи, которыми правит строка. При
	// Save они уезжают обратно в запись узла.
	Enabled  bool
	OrderNum *int

	// Name — подпись записи (InlineBody.Name / SrsBody.Name) с уже
	// подставленным финальным тегом узла, для строки списка.
	Name string
}

// NodeTag — тег узла-владельца (для подписи и tooltip).
func (n *NodeRuleRef) NodeTag() string {
	if n == nil {
		return ""
	}
	return n.Link.Tag
}

// Clone — глубокая копия.
func (n *NodeRuleRef) Clone() *NodeRuleRef {
	if n == nil {
		return nil
	}
	cp := *n
	if n.OrderNum != nil {
		v := *n.OrderNum
		cp.OrderNum = &v
	}
	return &cp
}

// SeedNodeRuleRefs пересевает model.NodeRuleRefs по текущему составу
// источников.
//
// Строка заводится каждой записи `sections.rules[]`, которой ещё нет в списке,
// и снимается у записи, которая исчезла. Позиция и тумблер уже существующей
// строки берутся ИЗ ЗАПИСИ УЗЛА: она источник истины, а строка — её вид.
//
// Возвращает true, если состав строк изменился, — вызывающий по этому признаку
// пересобирает слоты и перерисовывает вкладку.
func SeedNodeRuleRefs(m *WizardModel) bool {
	if m == nil {
		return false
	}
	var out []*NodeRuleRef
	visit := func(node *corestate.Node, link corestate.NodeLink) {
		if node == nil || node.Kind != corestate.SourceKindServer || !node.Sections.HasRules() {
			return
		}
		for i := range node.Sections.Rules {
			r := &node.Sections.Rules[i]
			ref := &NodeRuleRef{
				Link:     link,
				Index:    i,
				Enabled:  r.Enabled,
				OrderNum: copyOrderNum(r.OrderNum),
				Name:     nodeRuleDisplayName(*r, link.Tag),
			}
			out = append(out, ref)
		}
	}
	for i := range m.Sources {
		src := &m.Sources[i]
		switch src.Kind {
		case corestate.SourceKindServer:
			// Тег корневого узла — тот, под которым его знает конфиг
			// (canonicalProjection берёт NodeTagOrLabel).
			visit(&src.Node, corestate.NodeLink{Tag: src.NodeTagOrLabel()})
		case corestate.SourceKindFolder, corestate.SourceKindSubscription:
			for j := range src.Nodes {
				visit(&src.Nodes[j], corestate.NodeLink{FolderID: src.ID, Tag: src.Nodes[j].Tag})
			}
		}
	}

	changed := len(out) != len(m.NodeRuleRefs)
	if !changed {
		for i := range out {
			cur := m.NodeRuleRefs[i]
			if cur == nil || cur.Link != out[i].Link || cur.Index != out[i].Index {
				changed = true
				break
			}
			// Позиция и тумблер берутся из записи узла — она источник истины;
			// но если строку только что подвинули, а узел ещё не сохранён,
			// расхождения быть не должно: Save пишет в узел до пересева.
			out[i].Enabled = cur.Enabled
			out[i].OrderNum = copyOrderNum(cur.OrderNum)
		}
	}
	m.NodeRuleRefs = out
	return changed
}

// nodeRuleDisplayName — подпись записи с подставленным тегом узла.
//
// Тег здесь СЫРОЙ (тег узла в контейнере), а не финальный: финальный знает
// только эмиссия, а строка списка правил рисуется без неё. Для подписи разница
// видна лишь у папки с тег-политикой, и звать ради неё тег-машину на каждую
// перерисовку списка дороже, чем показать имя узла таким, каким его знает
// дерево источников.
func nodeRuleDisplayName(r corestate.Rule, nodeTag string) string {
	body, err := r.DecodeBody()
	if err != nil {
		return ""
	}
	switch b := body.(type) {
	case *corestate.InlineBody:
		return corestate.SubstituteSelfInString(b.Name, nodeTag)
	case *corestate.SrsBody:
		return corestate.SubstituteSelfInString(b.Name, nodeTag)
	}
	return ""
}

// SyncNodeRuleRefsToSources раскладывает `enabled` и `order_num` строк обратно
// по узлам (SPEC 121 §10.4).
//
// Зовётся на Save рядом с эмиссией корневых правил: у оси один порядок на всех,
// и ленивый сдвиг мог задеть соседей любого происхождения — узловых в том
// числе. Без этой раскладки перетаскивание жило бы до первой перезагрузки.
func SyncNodeRuleRefsToSources(m *WizardModel) {
	if m == nil {
		return
	}
	for _, ref := range m.NodeRuleRefs {
		if ref == nil {
			continue
		}
		node := FindNodeByLink(m, ref.Link)
		if node == nil || node.Sections == nil {
			continue
		}
		if ref.Index < 0 || ref.Index >= len(node.Sections.Rules) {
			continue
		}
		node.Sections.Rules[ref.Index].Enabled = ref.Enabled
		node.Sections.Rules[ref.Index].OrderNum = copyOrderNum(ref.OrderNum)
	}
}

// NodeRuleRefNodeEnabled — включён ли САМ узел строки (не строка).
//
// Выключенный узел в конфиг не едет, и его правил там нет — строка обязана
// сказать об этом, иначе выключенный узел читается как работающее правило.
// Узел не найден → считаем выключенным: показывать «работает» нечему.
func NodeRuleRefNodeEnabled(m *WizardModel, ref *NodeRuleRef) bool {
	if m == nil || ref == nil {
		return false
	}
	node := FindNodeByLink(m, ref.Link)
	return node != nil && node.Enabled
}

// FindNodeByLink — узел источников по ссылке {FolderID, Tag}.
//
// Корневой узел адресуется тем же именем, что его знает конфиг
// (NodeTagOrLabel) — так же, как его собирает пересев.
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
