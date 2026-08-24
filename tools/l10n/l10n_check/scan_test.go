package main

import (
	"go/token"
	"testing"
)

const testSrc = `package demo

import "singbox-launcher/internal/locale"

func f(n int) {
	_ = locale.T("Save")
	_ = locale.Tf("Connected to %s", "h")
	_ = locale.TN(1, "Copy")
	_ = locale.Plural("%d nodes", n)
	key := "Indirect key" // l10n-key
	_ = locale.T(key)
	_ = locale.T(dynamic())
	Helper("Helper key", 1)
}
`

func scanTest(t *testing.T, src string, helpers Helpers) *ScanResult {
	t.Helper()
	res := &ScanResult{Used: map[string]*Usage{}}
	if err := ScanSource(token.NewFileSet(), "demo.go", []byte(src), helpers, res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestScanCollectsCalls(t *testing.T) {
	res := scanTest(t, testSrc, Helpers{"Helper": []int{0}})

	for _, key := range []string{"Save", "Connected to %s", "Copy", "%d nodes", "Indirect key", "Helper key"} {
		if res.Used[key] == nil {
			t.Errorf("key %q not collected", key)
		}
	}
	if u := res.Used["Copy"]; u == nil || len(u.Forms) != 1 || u.Forms[0] != 1 {
		t.Errorf("TN form not collected: %+v", res.Used["Copy"])
	}
	if u := res.Used["%d nodes"]; u == nil || !u.PluralFamily || u.TFamily {
		t.Errorf("plural family wrong: %+v", res.Used["%d nodes"])
	}
	// два динамических вызова: locale.T(key) и locale.T(dynamic())
	if len(res.Dynamic) != 2 {
		t.Errorf("dynamic = %v, want 2", res.Dynamic)
	}
}

func TestMarkerSameLineOnly(t *testing.T) {
	src := `package demo

func f() {
	a := "Marked" // l10n-key
	b := "Next line"
	_, _ = a, b
}
`
	res := scanTest(t, src, nil)
	if res.Used["Marked"] == nil {
		t.Error("marked literal not collected")
	}
	if res.Used["Next line"] != nil {
		t.Error("marker must not spill to the next line")
	}
}

func TestPlaceholdersArity(t *testing.T) {
	if !sameArity(placeholders("%s of %d"), placeholders("%d из %s")) {
		// мультимножество вербов упорядочено — %s+%d в любом порядке равны
		t.Error("sorted verbs must match")
	}
	if sameArity(placeholders("%s"), placeholders("%s %s")) {
		t.Error("count mismatch must fail")
	}
	if sameArity(placeholders("%s"), placeholders("%d")) {
		t.Error("verb mismatch must fail")
	}
	if len(placeholders("100%% done")) != 0 {
		t.Error("%% is a literal, not a placeholder")
	}
	if len(placeholders("take %[1]s and %[2]s")) != 2 {
		t.Error("indexed verbs must count")
	}
}

func TestConstResolution(t *testing.T) {
	res := scanTest(t, `package demo

const longHintText = "A very long hint"

func f() {
	_ = locale.T(longHintText)
}
`, nil)
	if res.Used["A very long hint"] == nil {
		t.Error("const-referenced key not resolved")
	}
	if len(res.Dynamic) != 0 {
		t.Errorf("resolved const counted as dynamic: %v", res.Dynamic)
	}
}
