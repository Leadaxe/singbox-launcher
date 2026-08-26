// File source_node_counts_test.go — счётчики узлов на строке источника.
package business

import (
	"testing"

	"singbox-launcher/core/config"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

func modelWithNodes() *wizardmodels.WizardModel {
	// IdentityTag ставит парсер (SPEC 112) — здесь он проставлен явно, как
	// после LoadNodesFromSource: превью и сборка идут одним путём.
	n1 := &config.ParsedNode{Tag: "AL:a", IdentityTag: "a", Scheme: "socks", Server: "10.0.0.1", Port: 1080}
	n2 := &config.ParsedNode{Tag: "AL:b", IdentityTag: "b", Scheme: "socks", Server: "10.0.0.2", Port: 1080}
	n3 := &config.ParsedNode{Tag: "AL:c", IdentityTag: "c", Scheme: "socks", Server: "10.0.0.3", Port: 1080}
	return &wizardmodels.WizardModel{
		Sources:      []wizardmodels.Source{{ID: "s1", Type: wizardmodels.SourceTypeSubscription}},
		PreviewNodes: []*config.ParsedNode{n1, n2, n3},
		PreviewNodesBySource: map[int][]*config.ParsedNode{
			0: {n1, n2, n3},
		},
	}
}

func TestSourceNodeCounts_AllEnabled(t *testing.T) {
	m := modelWithNodes()
	if !EnsureSourceNodeCounts(m) {
		t.Fatal("счёт не выполнен")
	}
	c := m.SourceNodeCounts[0]
	if c.Total != 3 || c.Enabled != 3 {
		t.Errorf("счёт = %d/%d, ожидали 3/3", c.Enabled, c.Total)
	}
}

// Снятая галка ноды уменьшает «пойдёт в конфиг», но не «всего»: иначе
// «2 узла» читалось бы как потеря третьего, а не как выключение.
func TestSourceNodeCounts_DisabledNodeCountedApart(t *testing.T) {
	m := modelWithNodes()
	off := config.NodeIdentity(m.PreviewNodesBySource[0][1])
	if off == "" {
		t.Fatal("идентичность пуста — тест не проверяет ничего")
	}
	m.Sources[0].DisabledNodes = map[string]int64{off: 1}

	EnsureSourceNodeCounts(m)

	c := m.SourceNodeCounts[0]
	if c.Total != 3 || c.Enabled != 2 {
		t.Errorf("счёт = %d/%d, ожидали 2/3", c.Enabled, c.Total)
	}
}

// Второй вызов не пересчитывает: в этом и смысл кэша — строка списка
// перерисовывается на каждое движение мыши.
func TestSourceNodeCounts_CachedOnSecondCall(t *testing.T) {
	m := modelWithNodes()
	if !EnsureSourceNodeCounts(m) {
		t.Fatal("первый счёт не выполнен")
	}
	if EnsureSourceNodeCounts(m) {
		t.Error("второй вызов пересчитал — кэш не работает")
	}
}

// Инвалидация превью обязана снимать и счётчики: они из него выведены, и
// пережить его не могут — иначе список показывал бы числа прошлого состава.
func TestSourceNodeCounts_DroppedWithPreviewCache(t *testing.T) {
	m := modelWithNodes()
	EnsureSourceNodeCounts(m)

	InvalidatePreviewCache(m)

	if m.SourceNodeCounts != nil {
		t.Error("счётчики пережили сброс превью — покажут устаревшие числа")
	}
}

// Источник без узлов в строку счёт не добавляет: «0 узлов» у ещё не
// загруженной подписки выглядит как приговор, а не как «пока не считали».
func TestSourceNodeCounts_EmptySourceSkipped(t *testing.T) {
	m := &wizardmodels.WizardModel{
		Sources:              []wizardmodels.Source{{ID: "s1"}},
		PreviewNodes:         []*config.ParsedNode{{Tag: "x", Scheme: "socks"}},
		PreviewNodesBySource: map[int][]*config.ParsedNode{},
	}
	EnsureSourceNodeCounts(m)
	if _, ok := m.SourceNodeCounts[0]; ok {
		t.Error("для источника без узлов счёт не должен появляться")
	}
}
