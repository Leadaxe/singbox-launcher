package config

import (
	"strings"
	"testing"
)

// SPEC 112-B часть B — исключение источника fail-closed видно за пределами
// лога: сборка отдаёт реестр {source_id, подпись, причина}, а он переписывается
// целиком на каждой сборке.
//
// Формулировки строк тестами НЕ фиксируются: причина проверяется на то, что она
// называет обе стороны, а не на побуквенное совпадение.

// Висячая ссылка → реестр несёт запись с верным source_id и человекочитаемой
// причиной. До SPEC 112-B исключение уходило только в лог: строка источника
// показывала галку и «N nodes», и связать её с пропавшим трафиком было нечем.
func TestResolveNodeDetours_RecordsExclusion(t *testing.T) {
	hop := parseNodeForTagDetour(t, tagDetourHopURI)
	hop.IdentityTag = "hop"
	hop.Tag = "hop"
	hop.SourceIndex = 0

	chained := parseNodeForTagDetour(t, tagDetourChainedURI)
	chained.SourceIndex = 1

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01HOP", Label: "Proton", Connections: []string{tagDetourHopURI}},
		ProxySource{ID: "01SUB", Label: "Proton NL", Connections: []string{"..."},
			DetourNodeSourceID: "01HOP", DetourNodeTag: "уехавший узел"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {chained}}
	all, excluded := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{hop, chained})

	if len(all) != 1 {
		t.Fatalf("ожидался fail-closed drop зависимого источника; осталось %d узлов", len(all))
	}
	if len(excluded) != 1 {
		t.Fatalf("в реестре %d записей, ожидалась 1", len(excluded))
	}
	e := excluded[0]
	if e.SourceID != "01SUB" {
		t.Errorf("source_id = %q, ожидался ULID выпавшего источника (01SUB)", e.SourceID)
	}
	if e.SourceLabel != "Proton NL" {
		t.Errorf("подпись = %q, ожидалась подпись выпавшего источника", e.SourceLabel)
	}
	// Причина обязана называть обе стороны: где искали и что (SPEC 112-A).
	if !strings.Contains(e.Reason, "Proton") || !strings.Contains(e.Reason, "уехавший узел") {
		t.Errorf("причина %q не называет ни источник-цель, ни ненайденный узел", e.Reason)
	}
	if strings.Contains(e.Reason, "01HOP") || strings.Contains(e.Reason, "01SUB") {
		t.Errorf("причина %q светит ULID, хотя человеческие имена известны", e.Reason)
	}
}

// Чистая сборка реестр не наполняет: пометка ⚠ не имеет права появиться там,
// где источник в конфиг попал.
func TestResolveNodeDetours_CleanBuildRecordsNothing(t *testing.T) {
	hop := parseNodeForTagDetour(t, tagDetourHopURI)
	hop.IdentityTag = "hop"
	hop.Tag = "hop"
	hop.SourceIndex = 0

	chained := parseNodeForTagDetour(t, tagDetourChainedURI)
	chained.SourceIndex = 1

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01HOP", Label: "Proton", Connections: []string{tagDetourHopURI}},
		ProxySource{ID: "01SUB", Label: "Proton NL", Connections: []string{"..."},
			DetourNodeSourceID: "01HOP", DetourNodeTag: "hop"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {chained}}
	all, excluded := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{hop, chained})

	if len(all) != 2 {
		t.Fatalf("узлы выпали на чистой сборке; осталось %d", len(all))
	}
	if len(excluded) != 0 {
		t.Fatalf("чистая сборка положила в реестр %d записей", len(excluded))
	}
}

// Реестр — итог ПОСЛЕДНЕЙ сборки: чистая сборка обязана снять прежние пометки,
// иначе ⚠ переживёт свою причину.
func TestExcludedSourcesRegistryOverwrittenByEveryBuild(t *testing.T) {
	t.Cleanup(func() { SetExcludedSources(nil) })

	SetExcludedSources([]SourceExclusion{{SourceID: "01SUB", SourceLabel: "Proton NL", Reason: "хоп не найден"}})
	if got := ExcludedSourceReason("01SUB"); got == "" {
		t.Fatal("причина не найдена по source_id — строка источника не сможет показать пометку")
	}
	if got := ExcludedSourceReason("01OTHER"); got != "" {
		t.Errorf("чужой источник получил причину %q", got)
	}
	if got := ExcludedSourceReason(""); got != "" {
		t.Errorf("пустой source_id совпал с записью: %q", got)
	}

	SetExcludedSources(nil)
	if got := ExcludedSourceReason("01SUB"); got != "" {
		t.Fatalf("пометка пережила чистую сборку: %q", got)
	}
	if len(ExcludedSources()) != 0 {
		t.Fatal("реестр не очистился")
	}
}

// Реестр отдаёт КОПИЮ: читатель из UI не должен уметь испортить его следующей
// сборке.
func TestExcludedSourcesReturnsCopy(t *testing.T) {
	t.Cleanup(func() { SetExcludedSources(nil) })

	SetExcludedSources([]SourceExclusion{{SourceID: "01SUB", Reason: "r"}})
	got := ExcludedSources()
	got[0].Reason = "испорчено"

	if ExcludedSourceReason("01SUB") != "r" {
		t.Fatal("правка копии дошла до реестра")
	}
}
