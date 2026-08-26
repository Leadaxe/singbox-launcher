package build

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"singbox-launcher/core/template"
)

// substituteRouteForBindIface прогоняет route-секцию боевого шаблона через
// подстановку с заданным значением bind_interface.
func substituteRouteForBindIface(t *testing.T, value string) map[string]interface{} {
	t.Helper()
	// Шаблон берётся БОЕВОЙ, а не переписанный в тесте: копия проверяла бы
	// сама себя и осталась бы зелёной, если условный ключ выпадет из
	// bin/wizard_template.json.
	raw, err := os.ReadFile("../../bin/wizard_template.json")
	if err != nil {
		t.Fatalf("read wizard_template.json: %v", err)
	}
	var whole struct {
		Config struct {
			Route json.RawMessage `json:"route"`
		} `json:"config"`
	}
	if err := json.Unmarshal(raw, &whole); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}
	if len(whole.Config.Route) == 0 {
		t.Fatal("в шаблоне нет секции config.route")
	}
	tpl := `{"route":` + string(whole.Config.Route) + `}`
	vars := []template.TemplateVar{
		{Name: "auto_detect_interface", Type: "bool"},
		{Name: "bind_interface", Type: "interface"},
	}
	resolved := map[string]template.ResolvedVar{
		"auto_detect_interface": {Scalar: "true"},
		"bind_interface":        {Scalar: value},
	}
	out, err := template.SubstituteVarsInJSON([]byte(tpl), vars, resolved, template.LocalTarget())
	if err != nil {
		t.Fatalf("substitute: %v", err)
	}
	var root map[string]map[string]interface{}
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return root["route"]
}

func TestBindInterfaceEmittedWhenSet(t *testing.T) {
	route := substituteRouteForBindIface(t, "en0")
	if got := route["default_interface"]; got != "en0" {
		t.Fatalf("route.default_interface = %v, want en0", got)
	}
}

func TestBindInterfaceKeyAbsentWhenEmpty(t *testing.T) {
	// Пустое значение обязано УБИРАТЬ ключ, а не писать "": sing-box
	// отвергает пустую строку в default_interface, и весь конфиг падает
	// на старте у каждого, кто ни разу не трогал эту настройку.
	route := substituteRouteForBindIface(t, "")
	if v, ok := route["default_interface"]; ok {
		t.Fatalf("route.default_interface присутствует (%q) при пустой переменной", v)
	}
}

// warningsFor собирает предупреждения сборки для заданного bind_interface.
func warningsFor(t *testing.T, name string, target template.TargetSpec) []string {
	t.Helper()
	res := Result{}
	warnBindInterface(map[string]string{bindInterfaceVar: name}, target, &res)
	return res.Validation.Warnings
}

func TestWarnBindInterfaceUnknownName(t *testing.T) {
	w := warningsFor(t, "definitely-no-such-iface-42", template.LocalTarget())
	if len(w) != 1 || !strings.Contains(w[0], "does not exist") {
		t.Fatalf("warnings = %v, ожидалось предупреждение о несуществующем интерфейсе", w)
	}
}

func TestWarnBindInterfaceSilentWhenEmpty(t *testing.T) {
	if w := warningsFor(t, "", template.LocalTarget()); len(w) != 0 {
		t.Fatalf("warnings = %v, пустое значение — штатный режим, предупреждать не о чем", w)
	}
}

func TestWarnBindInterfaceSilentForRemoteTarget(t *testing.T) {
	// Интерфейсы удалённой машины отсюда не видны; сверка с локальными
	// дала бы ложную тревогу на каждом remote-конфиге.
	w := warningsFor(t, "definitely-no-such-iface-42", template.RemoteTarget("linux", "amd64"))
	if len(w) != 0 {
		t.Fatalf("warnings = %v, для remote-таргета проверка неприменима", w)
	}
}
