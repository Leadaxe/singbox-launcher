// File source_folder_drilldown_test.go — вход в папку прямо в списке Sources
// (SPEC 116 W13, обкатка).
//
// Данные-критичны две вещи, и обе про АДРЕС, а не про вид:
//
//  1. открытая папка адресуется ULID'ом, а не позицией в списке — иначе
//     перестановка или удаление соседа увели бы режим в чужой источник
//     (тот же довод, что у showFolderDeleteDialog);
//  2. строки режима — это СОСТАВ папки той же моделью, что на вкладке
//     Preview, и n-я строка обязана нести n-й узел: промахнись
//     сопоставление на один, и галка «выключить» уехала бы на соседа, а
//     Move/Rename/Delete из контекстного меню адресовали бы чужой узел.
//
// Вёрстка и формулировки не проверяются — на них тестов в проекте нет.
package tabs

import (
	"testing"

	corestate "singbox-launcher/core/state"
)

func drillFolder(id, name string, nodes ...corestate.Node) corestate.Source {
	s := corestate.NewFolderSource(name)
	s.ID = id
	s.Nodes = nodes
	return s
}

func drillServerSource(id, tag string) corestate.Source {
	s := corestate.Source{
		Node: corestate.Node{Kind: corestate.SourceKindServer, Tag: tag, Enabled: true},
		ID:   id,
	}
	return s
}

// Папка ищется по ULID, а не по позиции: список между открытием и показом
// вправе поехать (фоновый fetch, перетаскивание, удаление соседа).
func TestFolderDrillIndexAddressesByULID(t *testing.T) {
	sources := []corestate.Source{
		drillServerSource("SRV1", "srv-1"),
		drillFolder("FLD1", "Home"),
		drillFolder("FLD2", "Work"),
	}
	if got := folderDrillIndex(sources, "FLD2"); got != 2 {
		t.Fatalf("folderDrillIndex(FLD2) = %d, want 2", got)
	}

	// Сосед удалён — позиция папки уехала, ULID тот же.
	sources = append(sources[:0:0], sources[1:]...)
	if got := folderDrillIndex(sources, "FLD2"); got != 1 {
		t.Fatalf("после удаления соседа folderDrillIndex(FLD2) = %d, want 1", got)
	}

	// Пропавшая папка режим не ломает — вызывающий вернёт корень сам.
	if got := folderDrillIndex(sources, "FLD2-GONE"); got != -1 {
		t.Fatalf("folderDrillIndex(отсутствующей) = %d, want -1", got)
	}
	// Источник с таким ULID есть, но он НЕ папка: провалиться в него нельзя.
	if got := folderDrillIndex(sources, "SRV1"); got != -1 {
		t.Fatalf("folderDrillIndex(не-папки) = %d, want -1", got)
	}
	// Пустой адрес = режима нет.
	if got := folderDrillIndex(sources, "  "); got != -1 {
		t.Fatalf("folderDrillIndex(пусто) = %d, want -1", got)
	}
}

// Строки режима идут порядком состава папки, и у каждой свой сырой тег —
// адрес всех операций меню. Неразобранная запись занимает свою позицию, а не
// выпадает: иначе индексы строк и узлов разошлись бы на единицу.
func TestBuildFolderDrillRowsKeepsCompositionOrder(t *testing.T) {
	sources := []corestate.Source{
		drillServerSource("SRV1", "srv-1"),
		drillFolder("FLD1", "Home",
			stateServer("A"),
			corestate.NewUnsupportedNode("junk", "record rejected",
				corestate.OriginKindURI, "wtf://x"),
			stateServer("B"),
		),
	}

	input, ok := buildFolderDrillRows(sources, "FLD1")
	if !ok {
		t.Fatal("buildFolderDrillRows: папка не найдена")
	}
	if input.SourceIndex != 1 {
		t.Fatalf("SourceIndex = %d, want 1", input.SourceIndex)
	}
	if input.Name != "Home" {
		t.Fatalf("Name = %q, want \"Home\"", input.Name)
	}
	wantTags := []string{"A", "junk", "B"}
	if len(input.Rows) != len(wantTags) {
		t.Fatalf("строк %d, want %d", len(input.Rows), len(wantTags))
	}
	if len(input.Identities) != len(input.Rows) {
		t.Fatalf("Identities %d != Rows %d — адреса операций разъедутся со строками",
			len(input.Identities), len(input.Rows))
	}
	for i, want := range wantTags {
		if input.Rows[i].RawTag != want {
			t.Fatalf("строка %d: RawTag = %q, want %q", i, input.Rows[i].RawTag, want)
		}
		if input.Identities[i] != want {
			t.Fatalf("строка %d: Identities = %q, want %q", i, input.Identities[i], want)
		}
	}
	if !input.Rows[1].Unsupported {
		t.Fatal("неразобранная запись обязана остаться строкой со своим признаком")
	}
	if input.Rows[0].Unsupported || input.Rows[2].Unsupported {
		t.Fatal("собравшиеся узлы помечены неразобранными")
	}
}

// Пустая папка — законное состояние: строки нет ни одной, но режим открыт
// (список покажет строку возврата и подсказку, а не тупик).
func TestBuildFolderDrillRowsEmptyFolderStaysOpen(t *testing.T) {
	sources := []corestate.Source{drillFolder("FLD1", "Fresh")}
	input, ok := buildFolderDrillRows(sources, "FLD1")
	if !ok {
		t.Fatal("пустая папка обязана открываться")
	}
	if len(input.Rows) != 0 || len(input.Identities) != 0 {
		t.Fatalf("у пустой папки %d строк(и), want 0", len(input.Rows))
	}
	if input.Name != "Fresh" {
		t.Fatalf("Name = %q, want \"Fresh\"", input.Name)
	}
}

// Заход 2: провалиться можно и в ПОДПИСКУ — у неё тоже состав, а не один узел.
// Права над этим составом при этом другие, и знать о них обязана модель, а не
// экран: узлы подписки принадлежат провайдеру, руками в неё не льют.
func TestFolderDrillOpensSubscriptionWithLockedNodes(t *testing.T) {
	sub := corestate.NewSubscriptionSource("Liberty", "https://example.com/sub")
	sub.ID = "SUB1"
	sub.Nodes = []corestate.Node{stateServer("A"), stateServer("B")}
	sources := []corestate.Source{
		drillFolder("FLD1", "Home", stateServer("H")),
		sub,
		drillServerSource("SRV1", "srv-1"),
	}

	if got := folderDrillIndex(sources, "SUB1"); got != 1 {
		t.Fatalf("folderDrillIndex(подписки) = %d, want 1", got)
	}
	// Узловой источник составом не обладает — в него не проваливаются.
	if got := folderDrillIndex(sources, "SRV1"); got != -1 {
		t.Fatalf("folderDrillIndex(server) = %d, want -1", got)
	}

	input, ok := buildFolderDrillRows(sources, "SUB1")
	if !ok {
		t.Fatal("подписка обязана открываться тем же списком")
	}
	if input.Kind != corestate.SourceKindSubscription {
		t.Fatalf("Kind = %q, want subscription — на нём стоят все права над узлами", input.Kind)
	}
	if len(input.Rows) != 2 || input.Identities[0] != "A" || input.Identities[1] != "B" {
		t.Fatalf("состав подписки разъехался со строками: %+v", input.Identities)
	}

	// Свобода узлов — по контейнеру, а не по экрану: в папку класть можно, в
	// подписку нельзя, в корне (режим выключен) Add кладёт источники.
	d := &folderDrillState{}
	if !d.nodesAreFree(sources) {
		t.Fatal("в корне Add обязан работать")
	}
	d.enter("FLD1")
	if !d.nodesAreFree(sources) {
		t.Fatal("в папку узлы класть можно")
	}
	d.enter("SUB1")
	if d.nodesAreFree(sources) {
		t.Fatal("в подписку узлы руками не льют — их унесёт первый же fetch")
	}
	// Контейнер исчез, пока на него смотрели: наливать некуда.
	d.enter("GONE")
	if d.nodesAreFree(sources) {
		t.Fatal("исчезнувший контейнер объявлен свободным для наполнения")
	}
}

// Состояние режима: enter/leave и active — единственный переключатель вкладки.
func TestFolderDrillStateEnterLeave(t *testing.T) {
	d := &folderDrillState{}
	if d.active() {
		t.Fatal("свежее состояние обязано быть корнем")
	}
	d.enter("  FLD1  ")
	if !d.active() || d.folderID != "FLD1" {
		t.Fatalf("enter: active=%v folderID=%q", d.active(), d.folderID)
	}
	// Пробелы вместо адреса — не режим: строка папки без ULID существовать не
	// может, и открывать «папку без имени» нечем.
	d.enter("   ")
	if d.active() {
		t.Fatal("enter(пусто) не должен открывать режим")
	}
	d.enter("FLD2")
	d.leave()
	if d.active() {
		t.Fatal("leave обязан вернуть корень")
	}
}
