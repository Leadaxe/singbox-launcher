package state

import (
	"path/filepath"
	"testing"
)

// File node_enabled_test.go — жизненный цикл вердикта страховки «ядро не
// приняло узел» (SPEC 132, PARSING_PRINCIPLES §9.4): он привязан к ТЕЛУ узла, переживает
// пересчёт производных кодов и round-trip состояния, снимается сменой тела и
// включением рукой.

func nodeWithBody(tag, body string) Node {
	return Node{Kind: SourceKindServer, Tag: tag, Enabled: true, Body: []byte(body)}
}

const bodyA = `{"type":"vless","server":"a.example","server_port":443}`
const bodyB = `{"type":"vless","server":"b.example","server_port":443}`

// TestCoreVerdictLifecycle — одна таблица на весь цикл жизни вердикта.
func TestCoreVerdictLifecycle(t *testing.T) {
	t.Run("выключение записывает причину и гасит флажок", func(t *testing.T) {
		n := nodeWithBody("A", bodyA)
		if !n.SetCoreRejected("parse encryption: unknown appearance") {
			t.Fatal("SetCoreRejected вернул false")
		}
		if n.Enabled {
			t.Error("узел остался включённым")
		}
		if got := n.CoreRejectedReason(); got != "parse encryption: unknown appearance" {
			t.Errorf("причина = %q", got)
		}
		// Вердикт уровня узла стоит первым: его читают раньше кодов полей.
		if len(n.Warnings) == 0 || n.Warnings[0].Code != WarnCoreRejected {
			t.Errorf("вердикт не первым в списке: %+v", n.Warnings)
		}
		// Повтор той же причиной — «ничего не изменилось»: по этому ответу
		// цикл страховки останавливается.
		if n.SetCoreRejected("parse encryption: unknown appearance") {
			t.Error("повтор той же причины объявлен изменением")
		}
		// Новая причина — замещает, не дописывает.
		if !n.SetCoreRejected("bad flow") {
			t.Error("новая причина не записалась")
		}
		count := 0
		for _, w := range n.Warnings {
			if w.Code == WarnCoreRejected {
				count++
			}
		}
		if count != 1 {
			t.Errorf("записей вердикта %d, ожидалась одна", count)
		}
	})

	t.Run("неразобранной записи вердикт не адресуется", func(t *testing.T) {
		n := NewUnsupportedNode("X", "not emittable", "uri", "vless://...")
		if n.SetCoreRejected("bad") {
			t.Error("вердикт поставлен на kind=unsupported")
		}
	})

	t.Run("включение рукой стирает вердикт", func(t *testing.T) {
		n := nodeWithBody("A", bodyA)
		n.SetCoreRejected("bad uuid")
		if !n.SetNodeEnabled(true) {
			t.Fatal("включение не объявлено изменением")
		}
		if !n.Enabled {
			t.Error("узел не включился")
		}
		if n.CoreRejectedReason() != "" {
			t.Error("вердикт пережил включение рукой")
		}
	})

	t.Run("выключение рукой тоже стирает вердикт", func(t *testing.T) {
		// Иначе выключенный человеком узел продолжал бы объясняться чужим
		// текстом, будто это решение ядра.
		n := nodeWithBody("A", bodyA)
		n.SetCoreRejected("bad uuid")
		n.SetNodeEnabled(false)
		if n.CoreRejectedReason() != "" {
			t.Error("вердикт пережил выключение рукой")
		}
	})

	t.Run("пересчёт производных кодов вердикт НЕ стирает", func(t *testing.T) {
		n := nodeWithBody("A", bodyA)
		n.SetCoreRejected("bad uuid")
		n.ReplaceDerivedWarnings([]NodeWarning{{Code: "utls_fp_unknown", Path: "tls.utls.fingerprint"}})
		if n.CoreRejectedReason() != "bad uuid" {
			t.Fatal("пересчёт стёр вердикт")
		}
		if len(n.Warnings) != 2 || n.Warnings[0].Code != WarnCoreRejected {
			t.Errorf("список после пересчёта: %+v", n.Warnings)
		}
		// Повторный пересчёт не плодит копии вердикта.
		n.ReplaceDerivedWarnings([]NodeWarning{{Code: "utls_fp_unknown"}})
		if len(n.Warnings) != 2 {
			t.Errorf("после второго пересчёта %d записей: %+v", len(n.Warnings), n.Warnings)
		}
	})

	t.Run("смена тела снимает вердикт и включает узел", func(t *testing.T) {
		n := nodeWithBody("A", bodyA)
		n.SetCoreRejected("bad uuid")
		before := n.Body
		n.Body = []byte(bodyB)
		n.RevalidateCoreVerdictAfterBodyChange(before)
		if !n.Enabled {
			t.Error("узел не включился после смены тела")
		}
		if n.CoreRejectedReason() != "" {
			t.Error("вердикт пережил смену тела")
		}
	})

	t.Run("то же тело в другом написании вердикт держит", func(t *testing.T) {
		// Сравнение семантическое: порядок ключей и пробелы телом не
		// считаются, иначе любая нормализация снимала бы вердикт.
		n := nodeWithBody("A", bodyA)
		n.SetCoreRejected("bad uuid")
		before := n.Body
		n.Body = []byte(`{"server_port": 443, "server": "a.example", "type": "vless"}`)
		n.RevalidateCoreVerdictAfterBodyChange(before)
		if n.Enabled {
			t.Error("узел включился, хотя тело то же")
		}
		if n.CoreRejectedReason() != "bad uuid" {
			t.Error("вердикт снят, хотя тело то же")
		}
	})
}

// TestCoreVerdictSurvivesRefetch — обновление подписки: то же тело держит
// вердикт, другое — снимает и оживляет узел, узел выключенный ЧЕЛОВЕКОМ
// сменой тела не оживает.
func TestCoreVerdictSurvivesRefetch(t *testing.T) {
	newSub := func(nodes ...Node) *Source {
		return &Source{ID: "F1", Node: Node{Kind: SourceKindSubscription, Enabled: true}, Nodes: nodes}
	}

	t.Run("то же тело — узел выключен и с причиной", func(t *testing.T) {
		old := nodeWithBody("A", bodyA)
		old.SetCoreRejected("bad uuid")
		sub := newSub(old)
		_, warns := MergeSubscriptionNodes(sub, &SubFetchMaterial{
			Nodes: []Node{nodeWithBody("A", bodyA)},
		}, true)
		if len(warns) != 0 {
			t.Logf("warnings: %v", warns)
		}
		got := sub.Nodes[0]
		if got.Enabled {
			t.Error("узел включился после refetch с тем же телом")
		}
		if got.CoreRejectedReason() != "bad uuid" {
			t.Errorf("причина после refetch = %q", got.CoreRejectedReason())
		}
	})

	t.Run("другое тело — вердикт снят, узел включён", func(t *testing.T) {
		old := nodeWithBody("A", bodyA)
		old.SetCoreRejected("bad uuid")
		sub := newSub(old)
		MergeSubscriptionNodes(sub, &SubFetchMaterial{
			Nodes: []Node{nodeWithBody("A", bodyB)},
		}, true)
		got := sub.Nodes[0]
		if !got.Enabled {
			t.Error("провайдер починил запись, а узел остался выключенным")
		}
		if got.CoreRejectedReason() != "" {
			t.Error("вердикт о прежнем теле пережил смену тела")
		}
	})

	t.Run("выключенный ЧЕЛОВЕКОМ сменой тела не оживает", func(t *testing.T) {
		old := nodeWithBody("A", bodyA)
		old.SetNodeEnabled(false) // рукой: вердикта нет
		sub := newSub(old)
		MergeSubscriptionNodes(sub, &SubFetchMaterial{
			Nodes: []Node{nodeWithBody("A", bodyB)},
		}, true)
		if sub.Nodes[0].Enabled {
			t.Error("узел, выключенный человеком, включился сам")
		}
	})
}

// TestCoreVerdictSurvivesStateRoundTrip — вердикт переживает запись и чтение
// состояния: он живёт в warnings[], а они персистятся.
func TestCoreVerdictSurvivesStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := &State{Sources: []Source{{
		ID:    "F1",
		Node:  Node{Kind: SourceKindSubscription, Tag: "sub", Enabled: true},
		Nodes: []Node{nodeWithBody("A", bodyA), nodeWithBody("B", bodyA)},
	}}}
	s.Sources[0].Nodes[0].SetCoreRejected("parse encryption: unknown appearance")
	s.Sources[0].Nodes[1].SetNodeEnabled(false) // выключил человек

	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Sources) != 1 || len(loaded.Sources[0].Nodes) != 2 {
		t.Fatalf("состав после чтения: %+v", loaded.Sources)
	}
	a := loaded.Sources[0].Nodes[0]
	if a.Enabled {
		t.Error("узел A включился после чтения")
	}
	if got := a.CoreRejectedReason(); got != "parse encryption: unknown appearance" {
		t.Errorf("причина узла A после чтения = %q", got)
	}
	b := loaded.Sources[0].Nodes[1]
	if b.Enabled {
		t.Error("узел B включился после чтения")
	}
	if b.CoreRejectedReason() != "" {
		t.Error("у узла B, выключенного человеком, появилась причина")
	}

	// Миграция при загрузке (пересчёт производных кодов по телу) вердикт не
	// трогает: у узла A он единственная запись, и «не считали» для
	// производных кодов остаётся в силе.
	if got := a.CoreRejectedReason(); got == "" {
		t.Error("пересчёт кодов на загрузке стёр вердикт")
	}
}
