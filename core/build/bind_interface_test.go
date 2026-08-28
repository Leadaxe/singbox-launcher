package build

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"singbox-launcher/core/netiface"
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

// SPEC 113-F: чужой туннель (системный WireGuard/AmneziaWG) — ВАЛИДНЫЙ выбор
// аплинка. Warning ему не полагается: лишняя строка в отчёте сборки заставляла
// бы искать поломку там, где всё сделано намеренно.
func TestWarnBindInterfaceSilentForAnyListedInterface(t *testing.T) {
	for _, ifc := range netiface.ListOrEmpty() {
		if !ifc.Up {
			// Лежачий интерфейс — свой отдельный warning, он к делу не идёт.
			continue
		}
		if w := warningsFor(t, ifc.Name, template.LocalTarget()); len(w) != 0 {
			t.Errorf("warnings для предложенного %q = %v, ожидалось молчание", ifc.Name, w)
		}
	}
}

// Петля на месте под любым из двух имён — причину обязаны называть по факту, а
// не сваливать всё в «нет IP-адреса».
func TestWarnBindInterfaceNamesLoopbackReason(t *testing.T) {
	seen := false
	for _, name := range []string{"lo0", "lo"} {
		if !netiface.Exists(name) {
			continue
		}
		seen = true
		w := warningsFor(t, name, template.LocalTarget())
		if len(w) != 1 || !strings.Contains(w[0], "loopback") {
			t.Fatalf("warnings для %q = %v, ожидалось предупреждение про петлю", name, w)
		}
	}
	if !seen {
		t.Skip("на этой машине нет ни lo0, ни lo — сверять не с чем")
	}
}

// Собственный TUN ядра назван по имени из конфига: диагностика обязана сказать
// именно про петлю, а не про отсутствующий адрес (адрес у него есть).
func TestWarnBindInterfaceNamesOwnTunReason(t *testing.T) {
	var candidate string
	for _, ifc := range netiface.ListOrEmpty() {
		if ifc.Up {
			candidate = ifc.Name
			break
		}
	}
	if candidate == "" {
		t.Skip("на этой машине нет ни одного поднятого интерфейса")
	}
	// Объявляем реальный интерфейс собственным TUN — ровно то, что делает
	// лаунчер, прочитав tun.interface_name из config.json.
	netiface.SetOwnTunNames(candidate)
	defer netiface.SetOwnTunNames()

	w := warningsFor(t, candidate, template.LocalTarget())
	if len(w) != 1 || !strings.Contains(w[0], "own tunnel") {
		t.Fatalf("warnings для собственного TUN %q = %v, ожидалось предупреждение про петлю", candidate, w)
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
