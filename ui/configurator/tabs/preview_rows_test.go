// File preview_rows_test.go — строки списка Preview (SPEC 116 W11).
//
// Данные-критично ровно одно: КАКОЙ узел стоит за какой строкой. Промахнись
// сопоставление на один — и галка «выключить» уехала бы на соседа, а операции
// меню (переименовать, удалить, перенести) адресовали бы чужой узел.
package tabs

import (
	"testing"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
)

func emittedNode(rawTag, finalTag string) *config.ParsedNode {
	return &config.ParsedNode{
		Scheme:      "trojan",
		Tag:         finalTag,
		IdentityTag: rawTag,
		Outbound:    map[string]interface{}{"type": "trojan"},
	}
}

func stateServer(rawTag string) corestate.Node {
	return corestate.Node{
		Kind:    corestate.SourceKindServer,
		Tag:     rawTag,
		Enabled: true,
		Origin:  &corestate.Origin{Kind: corestate.OriginKindURI, Raw: "trojan://pw@x:443#" + rawTag},
	}
}

// Строки идут в порядке состава, неразобранная запись стоит на своём месте, и
// каждой строке достался её собственный узел — сопоставление по СЫРОМУ тегу,
// а не по позиции в эмиссии (в ней неразобранной записи нет вовсе).
func TestBuildPreviewRowsPairsNodesByRawTag(t *testing.T) {
	stateNodes := []corestate.Node{
		stateServer("A"),
		corestate.NewUnsupportedNode("junk", "record rejected", corestate.OriginKindURI, "wtf://x"),
		stateServer("B"),
	}
	// Эмиссия отдала два узла — неразобранной записи в ней нет.
	emitted := []*config.ParsedNode{
		emittedNode("A", "[P] A"),
		emittedNode("B", "[P] B"),
	}

	rows := buildPreviewRows(stateNodes, emitted)
	if len(rows) != 3 {
		t.Fatalf("строк = %d, ожидали 3 (состав целиком, включая неразобранную запись)", len(rows))
	}
	if rows[0].RawTag != "A" || rows[0].Node == nil || rows[0].Node.Tag != "[P] A" {
		t.Errorf("первой строке достался не свой узел: %+v", rows[0])
	}
	if !rows[1].Unsupported {
		t.Fatalf("вторая строка не помечена неразобранной: %+v", rows[1])
	}
	if rows[1].Node != nil {
		t.Error("у неразобранной записи оказался узел — эмиссия её не создаёт")
	}
	if rows[1].OriginRaw != "wtf://x" || rows[1].Reason == "" {
		t.Errorf("исходник или причина потеряны: %+v", rows[1])
	}
	if rows[2].RawTag != "B" || rows[2].Node == nil || rows[2].Node.Tag != "[P] B" {
		t.Errorf("третьей строке достался не свой узел (сдвиг на неразобранную запись): %+v", rows[2])
	}
	if previewRowsSupported(rows) != 2 || previewRowsUnsupported(rows) != 1 {
		t.Errorf("счёт разошёлся: supported=%d unsupported=%d",
			previewRowsSupported(rows), previewRowsUnsupported(rows))
	}
}

// Выключенный узел эмиссию не проходит, но из состава не исчезает: без строки
// снятую галку нельзя было бы вернуть.
func TestBuildPreviewRowsKeepsDisabledNode(t *testing.T) {
	off := stateServer("B")
	off.Enabled = false
	rows := buildPreviewRows(
		[]corestate.Node{stateServer("A"), off},
		[]*config.ParsedNode{emittedNode("A", "A")},
	)
	if len(rows) != 2 {
		t.Fatalf("строк = %d, ожидали 2 — выключенный узел обязан остаться в списке", len(rows))
	}
	if rows[1].RawTag != "B" || rows[1].Unsupported {
		t.Errorf("выключенный узел подменён: %+v", rows[1])
	}
}

// Узел-группа идентичности не имеет (SPEC 112, NodeIdentity возвращает ""),
// но её строка обязана получить свой узел — иначе чекбокс группы остался бы
// задизейбленным (недоделка W6) и её нельзя было бы выключить.
func TestBuildPreviewRowsPairsGroupWithoutIdentity(t *testing.T) {
	group := &config.ParsedNode{
		Scheme:   configtypes.SchemeGroup,
		Tag:      "[P] auto",
		Outbound: map[string]interface{}{"type": "urltest"},
	}
	stateNodes := []corestate.Node{
		stateServer("A"),
		{Kind: corestate.SourceKindAuto, Tag: "auto", Enabled: true,
			Group: &corestate.AutoGroup{GroupType: corestate.AutoGroupURLTest}},
	}
	rows := buildPreviewRows(stateNodes, []*config.ParsedNode{emittedNode("A", "A"), group})
	if len(rows) != 2 {
		t.Fatalf("строк = %d, ожидали 2", len(rows))
	}
	if rows[1].Node != group {
		t.Fatalf("строке группы достался не тот узел: %+v", rows[1].Node)
	}
	if rows[1].RawTag != "auto" {
		t.Errorf("строка группы без сырого тега (%q) — чекбокс останется задизейбленным", rows[1].RawTag)
	}
}
