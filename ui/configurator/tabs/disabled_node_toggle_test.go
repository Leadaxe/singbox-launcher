package tabs

import (
	"testing"

	"singbox-launcher/core/config"
	"singbox-launcher/core/state"
)

// SPEC 094 D4 + SPEC 112 — переключатель «нода включена» в превью источника.
//
// Ключ отметки — идентичность узла (тег в рамках источника). Для самого
// переключателя ключ непрозрачен: он лишь кладёт и снимает отметку.

func TestSetNodeEnabledMarksAndClears(t *testing.T) {
	ps := &config.ProxySource{Source: "https://example.invalid/sub"}

	setNodeEnabled(ps, "🇩🇪 DE", false)
	if _, off := ps.DisabledNodes["🇩🇪 DE"]; !off {
		t.Fatal("disabling a node must record a mark")
	}
	if ps.DisabledNodes["🇩🇪 DE"] <= 0 {
		t.Fatalf("mark timestamp = %d, want a unix time", ps.DisabledNodes["🇩🇪 DE"])
	}

	setNodeEnabled(ps, "🇩🇪 DE", true)
	if _, off := ps.DisabledNodes["🇩🇪 DE"]; off {
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

func TestSetNodeEnabledIgnoresEmptyIdentity(t *testing.T) {
	ps := &config.ProxySource{}
	// Нода без идентичности: отметку не к чему привязать, и она поехала бы
	// на соседа при следующем обновлении.
	setNodeEnabled(ps, "", false)
	if len(ps.DisabledNodes) != 0 {
		t.Fatalf("пустая идентичность не должна создавать отметку, получено %v", ps.DisabledNodes)
	}

	setNodeEnabled(nil, "🇩🇪 DE", false) // не должно паниковать
}

// Полная цепочка сохранения: отметка в окне редактирования доезжает до Source,
// а оттуда обратно в ProxySource, который читает парсер.
func TestDisabledNodesSurviveEditRoundTrip(t *testing.T) {
	ps := &config.ProxySource{Source: "https://example.invalid/sub"}
	setNodeEnabled(ps, "🇳🇱 Amsterdam", false)

	src := &state.Source{Type: state.SourceTypeSubscription}
	applyProxyEditToSource(ps, src)

	if _, off := src.DisabledNodes["🇳🇱 Amsterdam"]; !off {
		t.Fatalf("mark lost on the way to Source: %v", src.DisabledNodes)
	}

	// Source → ProxySource: этот путь читает парсер при генерации конфига.
	back := src.ToProxySourceV4()
	if _, off := back.DisabledNodes["🇳🇱 Amsterdam"]; !off {
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
