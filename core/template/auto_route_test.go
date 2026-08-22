package template

import (
	"encoding/json"
	"os"
	"testing"
)

// issue #106 — auto_route настраивается пользователем.
//
// Было: `"auto_route": true` зашито константой в params.inbounds, при том что
// соседние strict_route/stack уже были переменными. Галки в интерфейсе нет, а
// ручная правка config.json перезаписывалась при следующей сборке.
//
// Стало: обычная переменная шаблона — строка Settings появляется сама
// (интерфейс строится из объявлений), значение уезжает в конфиг JSON-булевом.
func TestAutoRouteIsUserControlled(t *testing.T) {
	raw, err := os.ReadFile("../../bin/wizard_template.json")
	if err != nil {
		t.Skipf("шаблон недоступен: %v", err)
	}
	var td struct {
		Vars   []TemplateVar   `json:"vars"`
		Params []TemplateParam `json:"params"`
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(raw, &td); err != nil {
		t.Fatal(err)
	}

	if _, ok := VarByName(td.Vars, "auto_route"); !ok {
		t.Fatal("auto_route не объявлен в vars — строка Settings не появится")
	}

	for _, want := range []bool{true, false} {
		state := map[string]string{"tun": "true", "auto_route": boolStr(want)}
		out, err := ApplyTemplateWithVarsFor(td.Config, td.Params, td.Vars, state, raw, LocalTarget())
		if err != nil {
			t.Fatalf("сборка при auto_route=%v: %v", want, err)
		}
		var probe struct {
			Inbounds []map[string]interface{} `json:"inbounds"`
		}
		if err := json.Unmarshal(out, &probe); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, ib := range probe.Inbounds {
			if ib["type"] != "tun" {
				continue
			}
			found = true
			// Именно bool, а не строка: ядро отвергает "true" в булевом поле.
			got, isBool := ib["auto_route"].(bool)
			if !isBool {
				t.Fatalf("auto_route в конфиге типа %T, ожидался bool", ib["auto_route"])
			}
			if got != want {
				t.Errorf("auto_route=%v, ожидалось %v", got, want)
			}
		}
		if !found {
			t.Fatal("tun-инбаунд не собрался")
		}
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
