package state

// SPEC 118 (W1) / SPEC 127 — canonical roundtrip и стабильность идентичности.
//
// Схема v8 (SPEC 127): плоский корень sources[]/directions[]/rules[]/vars[]/
// dns/warp/meta; запись = метаданные + `body`; Save пишет только v8. Тесты
// фиксируют:
//
//  1. Load→Save→Load→Save сходится байт-в-байт (modulo meta.updated_at —
//     Save штампует текущее время всегда);
//  2. ULID папок/подписок стабильны через циклы мутаций и Save/Load; узлы
//     идентифицируются тегом (id у узлов нет — у мостовых верхних узлов
//     ULID живёт до W5);
//  3. загрузка v6-состояния (структурный перенос W1) тоже даёт стабильный
//     roundtrip со второго Save.
//
// Фикстуры testdata/:
//
//   - v7_roundtrip.json — ЗАМОРОЖЕННЫЙ вход миграции v7→v8: папка с узлами
//     (server + auto) и replace, подписка с материализованными nodes[] и
//     update_status, chain с NodeLink-хопами, верхний server с body/origin и
//     секциями, directions, правила всех трёх видов (в т.ч. srs с двумя
//     наборами и inline с drop), vars, плоский dns_options, warp_accounts.
//     Генератора у него больше нет: v7-форму записей типы уже не умеют.
//   - v8_roundtrip.json — тот же state в целевой форме. Регенерация:
//     GEN_V8_ROUNDTRIP_FIXTURE=1 go test -run TestGenerateV8RoundtripFixture
//     ./core/state/.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// v7RoundtripFixture — вход миграции v7→v8 (только чтение, всегда через копию).
const v7RoundtripFixture = "testdata/v7_roundtrip.json"

// v8RoundtripFixture — то же состояние в целевой форме.
const v8RoundtripFixture = "testdata/v8_roundtrip.json"

// v6RoundtripFixture — старая v6-фикстура; в W1 служит входом структурного
// переноса (полная миграция и её сценарии — волна W2).
const v6RoundtripFixture = "testdata/v6_roundtrip.json"

// fixedUpdatedAtLine — плейсхолдер для сравнения «всё, кроме updated_at».
const fixedUpdatedAtLine = `    "updated_at": "<normalized>",`

// normalizeUpdatedAt заменяет единственную строку meta.updated_at
// плейсхолдером. Требует ровно одного вхождения — второй ключ updated_at в
// файле означал бы дрейф схемы, и тест обязан упасть.
func normalizeUpdatedAt(t *testing.T, data []byte) []byte {
	t.Helper()
	lines := bytes.Split(data, []byte("\n"))
	found := 0
	for i, ln := range lines {
		if bytes.Contains(ln, []byte(`"updated_at":`)) {
			lines[i] = []byte(fixedUpdatedAtLine)
			found++
		}
	}
	if found != 1 {
		t.Fatalf("expected exactly 1 updated_at line, got %d", found)
	}
	return bytes.Join(lines, []byte("\n"))
}

// TestGenerateV8RoundtripFixture — генератор v8-фикстуры из замороженного
// v7-входа; запускается только вручную (GEN_V8_ROUNDTRIP_FIXTURE=1).
// Штампует фиксированные timestamps, чтобы файл в testdata не дрейфовал.
func TestGenerateV8RoundtripFixture(t *testing.T) {
	if os.Getenv("GEN_V8_ROUNDTRIP_FIXTURE") != "1" {
		t.Skip("generator: set GEN_V8_ROUNDTRIP_FIXTURE=1 to (re)write the fixture")
	}
	dir := t.TempDir()
	tmp := filepath.Join(dir, "state.json")
	s, err := Load(legacyFixtureCopy(t, v7RoundtripFixture))
	if err != nil {
		t.Fatalf("Load v7 fixture: %v", err)
	}
	if err := s.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v8RoundtripFixture, freezeFixtureTimestamps(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// freezeFixtureTimestamps — created_at/updated_at на константу, хвост строки
// (запятая есть/нет — решает позиция ключа) сохраняется.
func freezeFixtureTimestamps(data []byte) []byte {
	fixLine := func(ln []byte, key string) []byte {
		prefix := `    "` + key + `": "`
		if !bytes.HasPrefix(ln, []byte(prefix)) {
			return ln
		}
		suffix := ""
		if bytes.HasSuffix(ln, []byte(",")) {
			suffix = ","
		}
		return []byte(prefix + "2026-08-01T00:00:00Z\"" + suffix)
	}
	lines := bytes.Split(data, []byte("\n"))
	for i, ln := range lines {
		lines[i] = fixLine(fixLine(ln, "created_at"), "updated_at")
	}
	return bytes.Join(lines, []byte("\n"))
}

// TestCanonical_LoadSaveLoadSave_ByteIdentical — SPEC 118 Т1 / §4.H:
// Load(f) → Save(p1) → Load(p1) → Save(p2): p1 == p2 байт-в-байт (modulo
// meta.updated_at), а p1 относительно фикстуры отличается ТОЛЬКО updated_at.
func TestCanonical_LoadSaveLoadSave_ByteIdentical(t *testing.T) {
	fixtureBytes, err := os.ReadFile(v8RoundtripFixture)
	if err != nil {
		t.Fatalf("fixture missing (regenerate with GEN_V8_ROUNDTRIP_FIXTURE=1): %v", err)
	}

	dir := t.TempDir()
	p1 := filepath.Join(dir, "p1.json")
	p2 := filepath.Join(dir, "p2.json")

	s1, err := Load(legacyFixtureCopy(t, v8RoundtripFixture))
	if err != nil {
		t.Fatalf("Load fixture: %v", err)
	}
	if err := s1.Save(p1); err != nil {
		t.Fatalf("Save p1: %v", err)
	}
	s2, err := Load(p1)
	if err != nil {
		t.Fatalf("Load p1: %v", err)
	}
	if err := s2.Save(p2); err != nil {
		t.Fatalf("Save p2: %v", err)
	}

	p1b, err := os.ReadFile(p1)
	if err != nil {
		t.Fatal(err)
	}
	p2b, err := os.ReadFile(p2)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(normalizeUpdatedAt(t, p1b), normalizeUpdatedAt(t, p2b)) {
		t.Errorf("p1 != p2 (beyond updated_at): save is not idempotent\n--- p1 ---\n%s\n--- p2 ---\n%s", p1b, p2b)
	}
	if !bytes.Equal(normalizeUpdatedAt(t, fixtureBytes), normalizeUpdatedAt(t, p1b)) {
		t.Errorf("p1 differs from fixture beyond updated_at\n--- fixture ---\n%s\n--- p1 ---\n%s", fixtureBytes, p1b)
	}
}

// TestCanonical_V6StructuralTransfer_RoundtripStable — загрузка v6-состояния
// даёт стабильный v7-roundtrip: Save(p1) → Load(p1) → Save(p2), p1 == p2.
//
// SPEC 118 W5: легаси-полей в типе больше нет — миграция перевела их в канон
// (свёртка → replace, отметки → node.enabled, тройня → NodeLink), а defaults
// переехали в настройки приложения. Проверяем именно КАНОН.
func TestCanonical_V6StructuralTransfer_RoundtripStable(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "p1.json")
	p2 := filepath.Join(dir, "p2.json")

	s1, err := Load(legacyFixtureCopy(t, v6RoundtripFixture))
	if err != nil {
		t.Fatalf("Load v6 fixture: %v", err)
	}
	if err := s1.Save(p1); err != nil {
		t.Fatalf("Save p1: %v", err)
	}
	s2, err := Load(p1)
	if err != nil {
		t.Fatalf("Load p1: %v", err)
	}
	if s2.Version != SchemaVersionV8 {
		t.Fatalf("после Save версия обязана быть v8, got %d", s2.Version)
	}
	if err := s2.Save(p2); err != nil {
		t.Fatalf("Save p2: %v", err)
	}
	p1b, _ := os.ReadFile(p1)
	p2b, _ := os.ReadFile(p2)
	if !bytes.Equal(normalizeUpdatedAt(t, p1b), normalizeUpdatedAt(t, p2b)) {
		t.Errorf("v6→v7 transfer: p1 != p2 (beyond updated_at)\n--- p1 ---\n%s\n--- p2 ---\n%s", p1b, p2b)
	}

	// Структурный перенос без потерь: мостовые поля на месте.
	var sub *Source
	for i := range s2.Sources {
		if s2.Sources[i].Kind == SourceKindSubscription {
			sub = &s2.Sources[i]
		}
	}
	if sub == nil {
		t.Fatal("подписка не доехала до v7-формы")
	}
	if sub.Replace == nil {
		t.Errorf("свёртка подписки не переехала в замену: %+v", sub)
	}
	if sub.TagPolicy == nil {
		t.Errorf("тег-политика подписки потеряна: %+v", sub)
	}
}

// TestCanonical_IDStability — §4.H: ULID папок/подписок (и мостовых верхних
// узлов) неизменен через циклы mutate → Save → Load; ни один Save не выдаёт
// новых ULID существующим источникам.
func TestCanonical_IDStability(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	src, err := os.ReadFile(v8RoundtripFixture)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wantIDs := map[string]bool{}
	for _, x := range s.Sources {
		if x.ID == "" {
			t.Fatalf("fixture source without ULID: %+v", x)
		}
		wantIDs[x.ID] = true
	}
	wantCount := len(s.Sources)

	// Мутации canonical по циклам: правка URL/имени, toggle узла подписки,
	// правка replace, reorder. Каждый цикл — Save → Load с диска.
	for cycle := 0; cycle < 4; cycle++ {
		for i := range s.Sources {
			x := &s.Sources[i]
			switch x.Kind {
			case SourceKindSubscription:
				// Смена адреса подписки не должна выдавать новый ULID.
				x.URL = "https://example.invalid/sub-v" + string(rune('2'+cycle))
				x.Name = "renamed"
				if len(x.Nodes) > 0 {
					x.Nodes[0].Enabled = cycle%2 == 0
				}
			case SourceKindServer:
				x.Enabled = cycle%2 == 0
			case SourceKindFolder:
				if x.Replace != nil {
					x.Replace.Mode = FolderReplaceManual
					x.Replace.Strategy = nil
				}
			case SourceKindChain:
				x.Hops = append([]NodeLink(nil), x.Hops...)
			}
		}
		if cycle == 2 {
			// Reorder: порядок — пользовательская правка, идентичность
			// не должна от него зависеть.
			n := len(s.Sources)
			rev := make([]Source, 0, n)
			for i := n - 1; i >= 0; i-- {
				rev = append(rev, s.Sources[i])
			}
			s.Sources = rev
		}
		if err := s.Save(path); err != nil {
			t.Fatalf("Save cycle %d: %v", cycle, err)
		}
		s, err = Load(path)
		if err != nil {
			t.Fatalf("Load cycle %d: %v", cycle, err)
		}

		if len(s.Sources) != wantCount {
			t.Fatalf("cycle %d: source count drifted: %d → %d", cycle, wantCount, len(s.Sources))
		}
		got := map[string]bool{}
		for _, x := range s.Sources {
			got[x.ID] = true
		}
		for id := range wantIDs {
			if !got[id] {
				t.Errorf("cycle %d: ULID %s lost", cycle, id)
			}
		}
		for id := range got {
			if !wantIDs[id] {
				t.Errorf("cycle %d: Save issued a NEW ULID %s to an existing source", cycle, id)
			}
		}
	}

	// Мутации доехали (Save действительно сохраняет canonical-правки).
	for _, x := range s.Sources {
		if x.Kind == SourceKindSubscription && x.Name != "renamed" {
			t.Errorf("subscription mutation lost after cycles: %+v", x)
		}
	}
}
