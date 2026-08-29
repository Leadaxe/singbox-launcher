package tabs

import (
	"testing"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 094 D4 + SPEC 112 — переключатель «нода включена» в превью источника.
//
// Ключ отметки — идентичность узла (тег в рамках источника). Для самого
// переключателя ключ непрозрачен: он лишь кладёт и снимает отметку.
//
// SPEC 117: отметка правится прямо в canonical state.Source (в окне — в его
// рабочей deep-copy); scratch-ProxySource упразднён.

func TestSetNodeEnabledMarksAndClears(t *testing.T) {
	src := &wizardmodels.Source{Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription}, URL: "https://example.invalid/sub"}

	setNodeEnabled(src, "🇩🇪 DE", false)
	if _, off := src.DisabledNodes["🇩🇪 DE"]; !off {
		t.Fatal("disabling a node must record a mark")
	}
	if src.DisabledNodes["🇩🇪 DE"] <= 0 {
		t.Fatalf("mark timestamp = %d, want a unix time", src.DisabledNodes["🇩🇪 DE"])
	}

	setNodeEnabled(src, "🇩🇪 DE", true)
	if _, off := src.DisabledNodes["🇩🇪 DE"]; off {
		t.Fatal("enabling a node must clear its mark")
	}
	// Пустая карта обнуляется: omitempty не пишет её в state.json.
	if src.DisabledNodes != nil {
		t.Fatalf("empty mark map must be nil, got %v", src.DisabledNodes)
	}
}

func TestSetNodeEnabledKeepsOtherMarks(t *testing.T) {
	src := &wizardmodels.Source{}
	setNodeEnabled(src, "a", false)
	setNodeEnabled(src, "b", false)

	setNodeEnabled(src, "a", true)

	if _, off := src.DisabledNodes["b"]; !off {
		t.Fatal("enabling one node must not clear the others")
	}
	if len(src.DisabledNodes) != 1 {
		t.Fatalf("marks = %v, want only b", src.DisabledNodes)
	}
}

func TestSetNodeEnabledIgnoresEmptyIdentity(t *testing.T) {
	src := &wizardmodels.Source{}
	// Нода без идентичности: отметку не к чему привязать, и она поехала бы
	// на соседа при следующем обновлении.
	setNodeEnabled(src, "", false)
	if len(src.DisabledNodes) != 0 {
		t.Fatalf("пустая идентичность не должна создавать отметку, получено %v", src.DisabledNodes)
	}

	setNodeEnabled(nil, "🇩🇪 DE", false) // не должно паниковать
}

// Отметка доезжает до сборки: Source → ProxySource — этот путь читает парсер
// при генерации конфига (прямая одноразовая проекция, Т2).
func TestDisabledNodesSurviveProjection(t *testing.T) {
	src := &wizardmodels.Source{Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription}, URL: "https://example.invalid/sub"}
	setNodeEnabled(src, "🇳🇱 Amsterdam", false)

	back := src.ToProxySourceV4()
	if _, off := back.DisabledNodes["🇳🇱 Amsterdam"]; !off {
		t.Fatalf("mark lost on the way to ProxySource: %v", back.DisabledNodes)
	}
}
