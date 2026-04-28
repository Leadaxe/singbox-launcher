package template

import (
	"encoding/json"
	"runtime"
	"testing"
)

func TestVarDefaultValueForPlatform_win7AndOther(t *testing.T) {
	v := VarDefaultValue{PerPlatform: map[string]string{"win7": "gvisor", "default": "system"}}
	if got := v.ForPlatform("windows", "386"); got != "gvisor" {
		t.Fatalf("windows/386: %q", got)
	}
	if got := v.ForPlatform("linux", "amd64"); got != "system" {
		t.Fatalf("linux: %q", got)
	}
}

func TestResolveTemplateVars_tunStackPerPlatformDefault(t *testing.T) {
	raw := `[{"name":"tun_stack","type":"enum","default_value":{"win7":"gvisor","default":"system"}}]`
	var vars []TemplateVar
	if err := json.Unmarshal([]byte(raw), &vars); err != nil {
		t.Fatal(err)
	}
	res := ResolveTemplateVars(vars, nil, nil)
	want := "system"
	if runtime.GOOS == "windows" && runtime.GOARCH == "386" {
		want = "gvisor"
	}
	if res["tun_stack"].Scalar != want {
		t.Fatalf("tun_stack: got %q want %q", res["tun_stack"].Scalar, want)
	}
}

func TestParamIfSatisfied(t *testing.T) {
	vars := []TemplateVar{
		{Name: "tun", Type: "bool"},
		{Name: "x", Type: "text"},
	}
	vi := VarIndex(vars)
	res := map[string]ResolvedVar{"tun": {Scalar: "true"}}
	if !ParamIfSatisfied([]string{"tun"}, vi, res, "darwin") {
		t.Fatal("tun true")
	}
	if ParamIfSatisfied([]string{"tun"}, vi, map[string]ResolvedVar{"tun": {Scalar: "false"}}, "darwin") {
		t.Fatal("tun false")
	}
	if ParamIfSatisfied([]string{"x"}, vi, res, "darwin") {
		t.Fatal("non-bool in if")
	}
}

func TestParamBoolVarTrue_respectsVarPlatforms(t *testing.T) {
	vi := VarIndex([]TemplateVar{
		{Name: "tun", Type: "bool", Platforms: []string{"darwin"}},
	})
	res := map[string]ResolvedVar{"tun": {Scalar: "true"}}
	if ParamBoolVarTrue("tun", vi, res, "linux") {
		t.Fatal("tun is darwin-only, must be false on linux")
	}
	if !ParamBoolVarTrue("tun", vi, res, "darwin") {
		t.Fatal("tun true on darwin")
	}
}

func TestParamIfSatisfied_falseWhenVarNotOnGOOSEvenIfResolvedTrue(t *testing.T) {
	vi := VarIndex([]TemplateVar{
		{Name: "tun", Type: "bool", Platforms: []string{"darwin"}},
	})
	res := map[string]ResolvedVar{"tun": {Scalar: "true"}}
	if ParamIfSatisfied([]string{"tun"}, vi, res, "linux") {
		t.Fatal("if [tun]: on linux darwin-only var must be false, not use resolved true")
	}
}

func TestVarUISatisfied_ifOr(t *testing.T) {
	vi := VarIndex([]TemplateVar{
		{Name: "tun_builtin", Type: "bool", Platforms: []string{"windows", "linux"}},
		{Name: "tun", Type: "bool", Platforms: []string{"darwin"}},
		{Name: "mtu", Type: "text", IfOr: []string{"tun_builtin", "tun"}},
	})
	res := map[string]ResolvedVar{
		"tun_builtin": {Scalar: "true"},
		"tun":         {Scalar: "false"},
	}
	v := TemplateVar{Name: "mtu", Type: "text", IfOr: []string{"tun_builtin", "tun"}}
	if !VarUISatisfied(v, vi, res, "linux") {
		t.Fatal("linux: tun_builtin true → row enabled")
	}
	if VarUISatisfied(v, vi, res, "darwin") {
		t.Fatal("darwin: tun false → row disabled")
	}
}

func TestVarUISatisfied_ifAndIfOrInvalid(t *testing.T) {
	v := TemplateVar{Name: "z", Type: "text", If: []string{"a"}, IfOr: []string{"b"}}
	vi := VarIndex([]TemplateVar{{Name: "a", Type: "bool"}, {Name: "b", Type: "bool"}})
	res := map[string]ResolvedVar{"a": {Scalar: "true"}, "b": {Scalar: "true"}}
	if VarUISatisfied(v, vi, res, "darwin") {
		t.Fatal("both if and if_or → false")
	}
}

func TestParamIfSatisfied_AND_falseWhenOneOperandNotOnGOOS(t *testing.T) {
	vi := VarIndex([]TemplateVar{
		{Name: "tun_builtin", Type: "bool", Platforms: []string{"windows", "linux"}},
		{Name: "tun", Type: "bool", Platforms: []string{"darwin"}},
	})
	res := map[string]ResolvedVar{
		"tun_builtin": {Scalar: "true"},
		"tun":         {Scalar: "true"},
	}
	if ParamIfSatisfied([]string{"tun_builtin", "tun"}, vi, res, "darwin") {
		t.Fatal("darwin: tun_builtin not on GOOS → AND must be false")
	}
	if ParamIfSatisfied([]string{"tun_builtin", "tun"}, vi, res, "linux") {
		t.Fatal("linux: tun not on GOOS → AND must be false")
	}
	if ParamIfSatisfied([]string{"tun_builtin", "tun"}, vi, res, "windows") {
		t.Fatal("windows: tun not on GOOS → AND must be false")
	}
}

func TestParamIfOrSatisfied(t *testing.T) {
	vi := VarIndex([]TemplateVar{
		{Name: "tun_builtin", Type: "bool", Platforms: []string{"windows", "linux"}},
		{Name: "tun", Type: "bool", Platforms: []string{"darwin"}},
	})
	res := map[string]ResolvedVar{
		"tun_builtin": {Scalar: "true"},
		"tun":         {Scalar: "true"},
	}
	if !ParamIfOrSatisfied([]string{"tun_builtin", "tun"}, vi, res, "windows") {
		t.Fatal("windows: tun_builtin wins")
	}
	if !ParamIfOrSatisfied([]string{"tun_builtin", "tun"}, vi, res, "darwin") {
		t.Fatal("darwin: tun wins")
	}
	resOff := map[string]ResolvedVar{
		"tun_builtin": {Scalar: "true"},
		"tun":         {Scalar: "false"},
	}
	if ParamIfOrSatisfied([]string{"tun_builtin", "tun"}, vi, resOff, "darwin") {
		t.Fatal("darwin tun false: neither branch for macOS TUN")
	}
	if !ParamIfOrSatisfied([]string{"tun_builtin", "tun"}, vi, resOff, "linux") {
		t.Fatal("linux: tun_builtin still true")
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (n int, err error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestMaybeGenerateClashSecret(t *testing.T) {
	old := ClashSecretReader
	defer func() { ClashSecretReader = old }()
	ClashSecretReader = zeroReader{}

	m := map[string]ResolvedVar{
		"clash_secret": {Scalar: "CHANGE_THIS_TO_YOUR_SECRET_TOKEN"},
	}
	MaybeGenerateClashSecret(m)
	if len(m["clash_secret"].Scalar) != 16 {
		t.Fatalf("len %d: %q", len(m["clash_secret"].Scalar), m["clash_secret"].Scalar)
	}
	if m["clash_secret"].Scalar != "AAAAAAAAAAAAAAAA" {
		t.Fatalf("deterministic secret: %q", m["clash_secret"].Scalar)
	}
}

func TestVarDisplayTitle_precedence(t *testing.T) {
	if got := VarDisplayTitle(TemplateVar{Name: "x", Title: "T"}); got != "T" {
		t.Fatalf("title: %q", got)
	}
	if got := VarDisplayTitle(TemplateVar{Name: "n", Title: "  "}); got != "n" {
		t.Fatalf("whitespace title falls back to name: %q", got)
	}
	if got := VarDisplayTitle(TemplateVar{Name: "n"}); got != "n" {
		t.Fatalf("name: %q", got)
	}
}

func TestVarDisplayTooltip(t *testing.T) {
	if got := VarDisplayTooltip(TemplateVar{Tooltip: "  hi  "}); got != "hi" {
		t.Fatalf("trim: %q", got)
	}
	if got := VarDisplayTooltip(TemplateVar{}); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

func TestGenerateClashSecretDeterministic(t *testing.T) {
	old := ClashSecretReader
	defer func() { ClashSecretReader = old }()
	ClashSecretReader = zeroReader{}
	s, err := GenerateClashSecret()
	if err != nil {
		t.Fatal(err)
	}
	if s != "AAAAAAAAAAAAAAAA" {
		t.Fatalf("got %q", s)
	}
}
