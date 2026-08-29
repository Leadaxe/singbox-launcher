// File source_node_counts_test.go — счётчики узлов на строке источника
// (SPEC 118 Т8, поведенческие проверки W6).
//
// Счёт идёт ПРЯМО из материализованных `nodes[]`: разбирать нечего, состав
// лежит в состоянии. Прежний ленивый кэш с тремя состояниями («не готов /
// готов и пуст / есть») схлопнулся в два — и это ровно та развилка, на
// которой форма однажды объявила рабочие позиции потерянными.
package business

import (
	"testing"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

func subWithNodes(enabled ...bool) wizardmodels.Source {
	src := wizardmodels.Source{Node: wizardmodels.Node{
		Kind: wizardmodels.SourceKindSubscription, Enabled: true}, ID: "S"}
	for i, on := range enabled {
		src.Nodes = append(src.Nodes, wizardmodels.Node{
			Kind: wizardmodels.SourceKindServer, Tag: string(rune('a' + i)), Enabled: on})
	}
	return src
}

func TestSourceNodeCounts(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		// 0: подписка, половина узлов выключена галками — именно этот случай
		// был неотличим от полной, и понять его можно было только открыв её.
		subWithNodes(true, false, true, false),
		// 1: узловой источник — ровно один узел, его судьбу решает свой enabled.
		{Node: wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Tag: "srv", Enabled: true}},
		// 2: выключенный узловой источник.
		{Node: wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Tag: "off", Enabled: false}},
		// 3: подписка, которую ни разу не фетчили — узлов нет вовсе.
		{Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true}, ID: "empty"},
	}}

	if !EnsureSourceNodeCounts(m) {
		t.Fatal("первый счёт не выполнен — список не перерисуется")
	}

	if c := m.SourceNodeCounts[0]; c.Total != 4 || c.Enabled != 2 {
		t.Errorf("подписка: %d из %d, ожидали 2 из 4", c.Enabled, c.Total)
	}
	if c := m.SourceNodeCounts[1]; c.Total != 1 || c.Enabled != 1 {
		t.Errorf("включённый server: %d из %d, ожидали 1 из 1", c.Enabled, c.Total)
	}
	if c := m.SourceNodeCounts[2]; c.Total != 1 || c.Enabled != 0 {
		t.Errorf("выключенный server: %d из %d, ожидали 0 из 1", c.Enabled, c.Total)
	}
	// Нефетченная подписка в карту не попадает вовсе: «0 узлов» на строке
	// читалось бы как «подписка пустая», а честный ответ — «её не обновляли»,
	// и его даёт updateStatus, а не счётчик.
	if _, ok := m.SourceNodeCounts[3]; ok {
		t.Error("нефетченная подписка получила счётчик — строка соврёт про пустоту")
	}

	// Повторный вызов бесплатен: кэш живёт до явной инвалидации.
	if EnsureSourceNodeCounts(m) {
		t.Error("повторный счёт выполнен — список перерисовывался бы вхолостую")
	}
}

// Смена состава обязана снять кэш: иначе список показывает числа от прошлого
// состава — то самое «50 nodes» у подписки, которая только что опустела.
func TestSourceNodeCountsInvalidated(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{subWithNodes(true, true)}}
	EnsureSourceNodeCounts(m)
	if c := m.SourceNodeCounts[0]; c.Total != 2 {
		t.Fatalf("исходный счёт = %d, ожидали 2", c.Total)
	}

	// Узел выключили — состав тот же, но в конфиг пойдёт меньше.
	m.Sources[0].Nodes[1].Enabled = false
	InvalidateSourceNodeCounts(m)
	if !EnsureSourceNodeCounts(m) {
		t.Fatal("после инвалидации счёт не пересчитан")
	}
	if c := m.SourceNodeCounts[0]; c.Enabled != 1 || c.Total != 2 {
		t.Errorf("после снятия галки: %d из %d, ожидали 1 из 2", c.Enabled, c.Total)
	}

	// InvalidateNodePool снимает счётчики вместе с пулом: они его производная
	// и пережить его не могут.
	InvalidateNodePool(m)
	if m.SourceNodeCounts != nil {
		t.Error("счётчики пережили инвалидацию пула — покажут числа от прошлого состава")
	}
}
