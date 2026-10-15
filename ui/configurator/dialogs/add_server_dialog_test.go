package dialogs

import (
	"strings"
	"testing"
)

// Превью и результат строятся из разбора входа — проверяем именно его, а не
// вёрстку: что попадёт в конфиг, а что форма отвергнет.
func TestParseAddServerInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{"single share-uri", "socks5://u:p@10.0.0.1:1080#a", 1},
		{"several uris", "socks5://10.0.0.1:1080\nproxy-http://10.0.0.2:8080", 2},
		{"single outbound json", `{"type":"socks","tag":"s","server":"1.1.1.1","server_port":1080}`, 1},
		{"outbound array", `[{"type":"socks","tag":"a","server":"1.1.1.1","server_port":1080},
		                     {"type":"http","tag":"b","server":"2.2.2.2","server_port":8080}]`, 2},
		{"full config", `{"outbounds":[{"type":"socks","tag":"s","server":"1.1.1.1","server_port":1080}]}`, 1},
		// Импорт — запись на КАЖДЫЙ узел документа; «первый узел» только у
		// вкладки JSON окна узла (контракт 1.1.89).
		{"document with several nodes", `{"outbounds":[{"type":"direct","tag":"direct"},
		                     {"type":"socks","tag":"a","server":"1.1.1.1","server_port":1080},
		                     {"type":"http","tag":"b","server":"2.2.2.2","server_port":8080}]}`, 2},
		{"garbage", "not a link at all", 0},
		{"empty", "   ", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseAddServerInput(tc.input); len(got) != tc.want {
				t.Errorf("want %d nodes, got %d", tc.want, len(got))
			}
		})
	}
}

// Недонабранный ввод не должен ронять превью: оно пересчитывается на каждый
// символ, и паника здесь means падение всего окна.
func TestParseAddServerInput_NoPanicOnPartialInput(t *testing.T) {
	for _, p := range []string{
		"{", "[", `{"type"`, `{"type":"soc`, "socks5:/", "proxy-http:",
		"[Interface]", "[Interface]\nPrivateKey =", "]}{[",
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on %q: %v", p, r)
				}
			}()
			parseAddServerInput(p)
		}()
	}
}

// Ручная правка одиночного объекта сохраняется побайтово — в этом её смысл.
func TestManualJSONResult_SingleObjectKeepsUnknownFields(t *testing.T) {
	edited := `{"type":"socks","tag":"hand","server":"9.9.9.9","server_port":9999,"custom_field":"kept"}`
	res, err := manualJSONResult(edited, "label")
	if err != nil {
		t.Fatalf("manualJSONResult: %v", err)
	}
	if len(res.ConfigJSON) == 0 {
		t.Fatal("single object must come back as ConfigJSON")
	}
	body := string(res.ConfigJSON)
	if !strings.Contains(body, "9.9.9.9") {
		t.Errorf("edited server lost: %s", body)
	}
	// Поле, которого наш парсер не знает, обязано выжить.
	if !strings.Contains(body, "custom_field") {
		t.Errorf("unknown field dropped: %s", body)
	}
	if res.Text != "" {
		t.Errorf("Text must stay empty for ConfigJSON path, got %q", res.Text)
	}
	if res.Label != "label" {
		t.Errorf("label lost: %q", res.Label)
	}
}

// Массив и целый конфиг уходят текстом — их разберёт общий путь Add.
func TestManualJSONResult_MultiGoesAsText(t *testing.T) {
	for _, body := range []string{
		`[{"type":"socks","tag":"a","server":"1.1.1.1","server_port":1080}]`,
		`{"outbounds":[{"type":"socks","tag":"s","server":"1.1.1.1","server_port":1080}]}`,
		`{"outbounds":[{"type":"socks","tag":"a","server":"1.1.1.1","server_port":1080},{"type":"http","tag":"b","server":"2.2.2.2","server_port":8080}]}`,
	} {
		res, err := manualJSONResult(body, "")
		if err != nil {
			t.Fatalf("manualJSONResult(%s): %v", body, err)
		}
		if len(res.ConfigJSON) != 0 {
			t.Errorf("multi-node body must not become ConfigJSON: %s", body)
		}
		if res.Text == "" {
			t.Errorf("multi-node body must come back as Text: %s", body)
		}
	}
}

// Пустой и битый JSON доходят до пользователя ошибкой.
func TestManualJSONResult_Rejects(t *testing.T) {
	if _, err := manualJSONResult("   ", ""); err == nil {
		t.Error("empty JSON must fail")
	}
	if _, err := manualJSONResult(`{"tag":"no-type"}`, ""); err == nil {
		t.Error("object without type must fail")
	}
}
