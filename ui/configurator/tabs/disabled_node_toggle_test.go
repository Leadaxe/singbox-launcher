package tabs

import (
	"testing"

	"singbox-launcher/core/config"
	"singbox-launcher/core/state"
)

// SPEC 094 D4 — переключатель «нода включена» в превью источника.

func TestSetNodeEnabledMarksAndClears(t *testing.T) {
	ps := &config.ProxySource{Source: "https://example.invalid/sub"}

	setNodeEnabled(ps, "hash-a", false)
	if _, off := ps.DisabledNodes["hash-a"]; !off {
		t.Fatal("disabling a node must record a mark")
	}
	if ps.DisabledNodes["hash-a"] <= 0 {
		t.Fatalf("mark timestamp = %d, want a unix time", ps.DisabledNodes["hash-a"])
	}

	setNodeEnabled(ps, "hash-a", true)
	if _, off := ps.DisabledNodes["hash-a"]; off {
		t.Fatal("enabling a node must clear its mark")
	}
	// Пустая карта обнуляется: omitempty не пишет её в state.json.
	if ps.DisabledNodes != nil {
		t.Fatalf("empty mark map must be nil, got %v", ps.DisabledNodes)
	}
}

func TestSetNodeEnabledKeepsOtherMarks(t *testing.T) {
	ps := &config.ProxySource{}
	setNodeEnabled(ps, "a", false)
	setNodeEnabled(ps, "b", false)

	setNodeEnabled(ps, "a", true)

	if _, off := ps.DisabledNodes["b"]; !off {
		t.Fatal("enabling one node must not clear the others")
	}
	if len(ps.DisabledNodes) != 1 {
		t.Fatalf("marks = %v, want only b", ps.DisabledNodes)
	}
}

func TestSetNodeEnabledIgnoresEmptyHash(t *testing.T) {
	ps := &config.ProxySource{}
	// Нода без вычислимого хеша: отметку не к чему привязать, и она поехала бы
	// на соседа при следующем обновлении.
	setNodeEnabled(ps, "", false)
	if len(ps.DisabledNodes) != 0 {
		t.Fatalf("empty hash must not create a mark, got %v", ps.DisabledNodes)
	}

	setNodeEnabled(nil, "hash", false) // не должно паниковать
}

// Полная цепочка сохранения: отметка в окне редактирования доезжает до Source,
// а оттуда обратно в ProxySource, который читает парсер.
func TestDisabledNodesSurviveEditRoundTrip(t *testing.T) {
	ps := &config.ProxySource{Source: "https://example.invalid/sub"}
	setNodeEnabled(ps, "node-hash", false)

	src := &state.Source{Type: state.SourceTypeSubscription}
	applyProxyEditToSource(ps, src)

	if _, off := src.DisabledNodes["node-hash"]; !off {
		t.Fatalf("mark lost on the way to Source: %v", src.DisabledNodes)
	}

	// Source → ProxySource: этот путь читает парсер при генерации конфига.
	back := src.ToProxySourceV4()
	if _, off := back.DisabledNodes["node-hash"]; !off {
		t.Fatalf("mark lost on the way back to ProxySource: %v", back.DisabledNodes)
	}
}

// Источник без выключенных нод не тащит пустую карту в состояние.
func TestNoDisabledNodesMeansNoField(t *testing.T) {
	ps := &config.ProxySource{Source: "https://example.invalid/sub"}
	src := &state.Source{Type: state.SourceTypeSubscription}
	applyProxyEditToSource(ps, src)

	if src.DisabledNodes != nil {
		t.Fatalf("expected no marks, got %v", src.DisabledNodes)
	}
}
