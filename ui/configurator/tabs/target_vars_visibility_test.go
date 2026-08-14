package tabs

import (
	"path/filepath"
	"testing"

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/constants"
)

// SPEC 097: видимость полей визарда описывается ТЕМ ЖЕ языком предикатов
// (#if), что и ветки внутри config-секций — никаких параллельных механизмов
// вроде отдельного поля targets[]. Запись if/if_or может быть JSON-предикатом:
//
//	"if": ["{\"@runtime.target\": \"local\"}"]
//
// Здесь фиксируется: (1) предикат реально вычисляется при отрисовке;
// (2) условие, зависящее только от @runtime.*, СКРЫВАЕТ строку (её нельзя
// удовлетворить из UI), а не показывает выключенной.
func TestClashVarsHiddenOnRemoteTarget(t *testing.T) {
	td, err := wizardtemplate.LoadTemplateData(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Skipf("bundled template not loadable: %v", err)
	}
	vi := wizardtemplate.VarIndex(td.Vars)

	for _, name := range []string{"clash_api", "clash_secret"} {
		v, ok := wizardtemplate.VarByName(td.Vars, name)
		if !ok {
			t.Fatalf("var %q missing from template", name)
		}
		// Условие статично для сборки → строку скрываем, а не гасим:
		// пользователь не может «включить» local-ность, находясь в remote.
		if !wizardtemplate.VarConditionIsTargetOnly(v) {
			t.Errorf("%s: condition must depend only on @runtime.* (hide, not disable)", name)
		}
	}

	for _, tc := range []struct {
		name    string
		target  wizardtemplate.TargetSpec
		visible bool
	}{
		{"local", wizardtemplate.TargetSpec{GOOS: "darwin", GOARCH: "arm64", Target: constants.ConfigTargetLocal}, true},
		{"remote", wizardtemplate.TargetSpec{GOOS: "linux", GOARCH: "arm64", Target: constants.ConfigTargetRemote}, false},
	} {
		resolved := wizardtemplate.ResolveTemplateVarsFor(td.Vars, nil, td.RawTemplate, tc.target)
		for _, name := range []string{"clash_api", "clash_secret"} {
			v, _ := wizardtemplate.VarByName(td.Vars, name)
			if got := wizardtemplate.VarUISatisfiedFor(v, vi, resolved, tc.target); got != tc.visible {
				t.Errorf("%s/%s: visible=%v, want %v", tc.name, name, got, tc.visible)
			}
		}
	}
}

// Условие со ссылкой на другую var (@tun) — НЕ target-only: такую строку
// пользователь может разблокировать, включив переключатель выше, поэтому она
// гасится, а не исчезает. Разграничение важно: спутать их — значит либо
// показывать мёртвые поля, либо прятать те, что юзер может включить.
func TestVarConditionKindDistinguished(t *testing.T) {
	td, err := wizardtemplate.LoadTemplateData(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Skipf("bundled template not loadable: %v", err)
	}
	for _, name := range []string{"tun_interface_name", "gateway_mode", "gateway_include_interface"} {
		v, ok := wizardtemplate.VarByName(td.Vars, name)
		if !ok {
			t.Fatalf("var %q missing", name)
		}
		if wizardtemplate.VarConditionIsTargetOnly(v) {
			t.Errorf("%s: depends on other vars (@tun/@gateway_mode) — must be disabled, not hidden", name)
		}
	}
}

// Предикатная форма в if[] проходит валидацию загрузчика: если бы валидатор
// её отвергал, LoadTemplateData вернул бы ошибку и визард остался без
// шаблона (именно так это и сломалось при первой попытке).
func TestPredicateFormLoadsCleanly(t *testing.T) {
	if _, err := wizardtemplate.LoadTemplateData(filepath.Join("..", "..", "..")); err != nil {
		t.Fatalf("template with predicate-form if[] must load: %v", err)
	}
}

// SPEC 097: значение, показанное в строке Settings, обязано совпадать с тем,
// что реально уедет в конфиг для этого таргета. Регрессия, найденная в UI:
// DisplaySettingValue резолвил дефолты по LOCAL-таргету, поэтому на remote
// поле показывало singbox-tun0, тогда как в конфиг шёл lxd-tun0.
func TestSettingsRowValueMatchesTargetDefault(t *testing.T) {
	td, err := wizardtemplate.LoadTemplateData(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Skipf("bundled template not loadable: %v", err)
	}
	for _, tc := range []struct {
		name   string
		target wizardtemplate.TargetSpec
		want   string
	}{
		{"local", wizardtemplate.TargetSpec{GOOS: "darwin", GOARCH: "arm64", Target: constants.ConfigTargetLocal}, "singbox-tun0"},
		{"remote", wizardtemplate.TargetSpec{GOOS: "linux", GOARCH: "arm64", Target: constants.ConfigTargetRemote}, "lxd-tun0"},
	} {
		got := wizardtemplate.DisplaySettingValueFor(
			td.Vars, nil, td.RawTemplate, "tun_interface_name", tc.target)
		if got != tc.want {
			t.Errorf("%s: UI shows %q, want %q", tc.name, got, tc.want)
		}
	}

	// State-override сильнее дефолта — на обоих таргетах.
	override := map[string]string{"tun_interface_name": "custom-tun9"}
	for _, target := range []wizardtemplate.TargetSpec{
		{GOOS: "darwin", GOARCH: "arm64", Target: constants.ConfigTargetLocal},
		{GOOS: "linux", GOARCH: "arm64", Target: constants.ConfigTargetRemote},
	} {
		if got := wizardtemplate.DisplaySettingValueFor(
			td.Vars, override, td.RawTemplate, "tun_interface_name", target); got != "custom-tun9" {
			t.Errorf("%s: state override lost, got %q", target.Target, got)
		}
	}
}
