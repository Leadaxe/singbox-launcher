package tabs

import (
	"path/filepath"
	"testing"

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/constants"
)

func loadBundledTemplate(t *testing.T) *wizardtemplate.TemplateData {
	t.Helper()
	td, err := wizardtemplate.LoadTemplateData(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Skipf("bundled template not loadable: %v", err)
	}
	return td
}

// SPEC 097: gateway_mode и его LAN-интерфейсы живут на шаге 0 (вкладка
// Target), а НЕ в Settings. Причина не косметическая: от gateway_mode
// зависит default_value других полей (proxy_in_listen), а дефолты резолвятся
// однопроходно в порядке объявления — переключатель обязан стоять выше
// зависимых и быть виден раньше.
func TestGatewayVarsLiveOnTargetTab(t *testing.T) {
	td := loadBundledTemplate(t)

	targetVars := wizardtemplate.TargetTabVars(td.Vars)
	names := map[string]bool{}
	for _, v := range targetVars {
		names[v.Name] = true
	}
	for _, want := range []string{"gateway_mode", "gateway_include_interface"} {
		if !names[want] {
			t.Errorf("%s must be a target-tab var (wizard_ui:%q)", want, wizardtemplate.WizardUITarget)
		}
		v, ok := wizardtemplate.VarByName(td.Vars, want)
		if !ok {
			t.Fatalf("var %q missing from template", want)
		}
		// У УДАЛЁННОЙ цели вкладка Target есть, и дублировать её поля на
		// Settings нельзя.
		if settingsVarVisible(v, "linux", true) {
			t.Errorf("%s must NOT be rendered on Settings for a remote target (it lives on step 0)", want)
		}
		// У ЛОКАЛЬНОЙ вкладки Target нет вовсе (решение владельца, SPEC 106):
		// раздача в LAN настраивается на Settings, иначе стала бы недоступна.
		if !settingsVarVisible(v, "linux", false) {
			t.Errorf("%s must be rendered on Settings for a local target — there is no Target tab there", want)
		}
	}
}

// gateway_mode объявлен ВЫШЕ всех полей, чьи дефолты от него зависят.
// Инвариант защищает от перестановки vars в шаблоне: forward-ссылка в
// default_value молча даёт else-ветку (валидатор её ловит, но лучше
// зафиксировать и намерение).
func TestGatewayModeDeclaredBeforeDependents(t *testing.T) {
	td := loadBundledTemplate(t)

	indexOf := func(name string) int {
		for i, v := range td.Vars {
			if !v.Separator && v.Name == name {
				return i
			}
		}
		return -1
	}
	gw := indexOf("gateway_mode")
	if gw < 0 {
		t.Fatal("gateway_mode missing from template")
	}
	for _, dependent := range []string{"gateway_include_interface", "proxy_in_listen"} {
		if i := indexOf(dependent); i < 0 || i < gw {
			t.Errorf("%s (index %d) must be declared after gateway_mode (index %d)", dependent, i, gw)
		}
	}
}

// Поведенческая проверка самой зависимости: переключение gateway_mode меняет
// разрешённый дефолт proxy_in_listen. Именно ради этого галка вынесена на
// шаг 0 — пользователь должен увидеть переключатель до зависимых значений.
func TestGatewayModeDrivesProxyListenDefault(t *testing.T) {
	td := loadBundledTemplate(t)
	target := wizardtemplate.TargetSpec{
		GOOS: "linux", GOARCH: "arm64", Target: constants.ConfigTargetRemote,
	}

	off := wizardtemplate.ResolveTemplateVarsFor(td.Vars,
		map[string]string{"enable_proxy_in": "true"}, td.RawTemplate, target)
	if got := off["proxy_in_listen"].Scalar; got != "127.0.0.1" {
		t.Errorf("non-gateway proxy_in_listen: want 127.0.0.1, got %q", got)
	}

	on := wizardtemplate.ResolveTemplateVarsFor(td.Vars,
		map[string]string{"enable_proxy_in": "true", "gateway_mode": "true"}, td.RawTemplate, target)
	if got := on["proxy_in_listen"].Scalar; got != "0.0.0.0" {
		t.Errorf("gateway proxy_in_listen: want 0.0.0.0, got %q", got)
	}
}

// Поле LAN-интерфейсов активно только при включённых TUN и gateway_mode.
func TestGatewayIncludeInterfaceGating(t *testing.T) {
	td := loadBundledTemplate(t)
	target := wizardtemplate.TargetSpec{
		GOOS: "linux", GOARCH: "arm64", Target: constants.ConfigTargetRemote,
	}
	vi := wizardtemplate.VarIndex(td.Vars)
	v, ok := wizardtemplate.VarByName(td.Vars, "gateway_include_interface")
	if !ok {
		t.Fatal("gateway_include_interface missing")
	}

	for _, tc := range []struct {
		name  string
		state map[string]string
		want  bool
	}{
		{"tun+gateway", map[string]string{"tun": "true", "gateway_mode": "true"}, true},
		{"tun only", map[string]string{"tun": "true"}, false},
		{"gateway without tun", map[string]string{"tun": "false", "gateway_mode": "true"}, false},
	} {
		resolved := wizardtemplate.ResolveTemplateVarsFor(td.Vars, tc.state, td.RawTemplate, target)
		if got := wizardtemplate.VarUISatisfied(v, vi, resolved, target.GOOS); got != tc.want {
			t.Errorf("%s: enabled=%v, want %v", tc.name, got, tc.want)
		}
	}
}

// Язык проекта — английский: тексты шаблона не должны содержать кириллицу
// (локализация идёт через bin/locale/*.json, не через шаблон).
func TestTemplateVarTextsAreEnglish(t *testing.T) {
	td := loadBundledTemplate(t)
	for _, v := range td.Vars {
		if v.Separator {
			continue
		}
		for _, field := range []struct{ name, text string }{
			{"title", v.Title}, {"tooltip", v.Tooltip},
		} {
			for _, r := range field.text {
				if r >= 'а' && r <= 'я' || r >= 'А' && r <= 'Я' || r == 'ё' || r == 'Ё' {
					t.Errorf("var %q %s contains Cyrillic: %q", v.Name, field.name, field.text)
					break
				}
			}
		}
	}
}
