package state

import (
	"encoding/json"
	"strings"
	"testing"
)

// UA подписки — часть канонической записи: он переживает round-trip через
// state.json. Без этого поле есть в форме, но теряется на перезапуске.
func TestSubscriptionUserAgentRoundTrip(t *testing.T) {
	src := NewSubscriptionSource("Liberty", "https://example.invalid/sub")
	src.UserAgent = "Happ/3.3.6"

	b, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"user_agent":"Happ/3.3.6"`) {
		t.Fatalf("UA не попал в JSON: %s", b)
	}

	var back Source
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.UserAgent != "Happ/3.3.6" {
		t.Errorf("UA потерян при чтении: %q", back.UserAgent)
	}

	// Пустой UA не засоряет запись — omitempty.
	src.UserAgent = ""
	b2, _ := json.Marshal(src)
	if strings.Contains(string(b2), "user_agent") {
		t.Errorf("пустой UA попал в JSON: %s", b2)
	}
}
