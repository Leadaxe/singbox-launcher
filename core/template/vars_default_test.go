package template

import (
	"encoding/json"
	"testing"
)

func TestVarDefaultValueUnmarshal_scalarString(t *testing.T) {
	var v VarDefaultValue
	if err := json.Unmarshal([]byte(`"hello"`), &v); err != nil {
		t.Fatal(err)
	}
	if v.Scalar != "hello" || len(v.PerPlatform) != 0 {
		t.Fatalf("%+v", v)
	}
}

func TestVarDefaultValueUnmarshal_scalarNumber(t *testing.T) {
	var v VarDefaultValue
	if err := json.Unmarshal([]byte(`1492`), &v); err != nil {
		t.Fatal(err)
	}
	if v.Scalar != "1492" {
		t.Fatalf("%q", v.Scalar)
	}
}

func TestVarDefaultValueUnmarshal_object(t *testing.T) {
	var v VarDefaultValue
	if err := json.Unmarshal([]byte(`{"Win7":"gvisor","DEFAULT":"system"}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.Scalar != "" {
		t.Fatalf("scalar %q", v.Scalar)
	}
	if v.PerPlatform["win7"] != "gvisor" || v.PerPlatform["default"] != "system" {
		t.Fatalf("%+v", v.PerPlatform)
	}
}

func TestVarDefaultValueForPlatform_goosLikePlatforms(t *testing.T) {
	// linux_amd64 в объекте не участвует в переборе — берётся ключ linux (как в platforms: только GOOS).
	v := VarDefaultValue{PerPlatform: map[string]string{"linux_amd64": "ignored", "linux": "y", "default": "z"}}
	if got := v.ForPlatform("linux", "amd64"); got != "y" {
		t.Fatalf("got %q want linux", got)
	}
	v2 := VarDefaultValue{PerPlatform: map[string]string{"linux": "only"}}
	if got := v2.ForPlatform("linux", "arm64"); got != "only" {
		t.Fatalf("got %q", got)
	}
}

func TestVarDefaultValueForPlatform_win7BeforeWindows(t *testing.T) {
	v := VarDefaultValue{PerPlatform: map[string]string{"win7": "gvisor", "windows": "system"}}
	if got := v.ForPlatform("windows", "386"); got != "gvisor" {
		t.Fatalf("windows/386: got %q", got)
	}
	if got := v.ForPlatform("windows", "amd64"); got != "system" {
		t.Fatalf("windows/amd64: got %q", got)
	}
}

func TestVarDefaultValueMarshal_roundTrip(t *testing.T) {
	v := VarDefaultValue{PerPlatform: map[string]string{"a": "b"}}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var v2 VarDefaultValue
	if err := json.Unmarshal(b, &v2); err != nil {
		t.Fatal(err)
	}
	if v2.PerPlatform["a"] != "b" {
		t.Fatalf("%+v", v2)
	}
}
