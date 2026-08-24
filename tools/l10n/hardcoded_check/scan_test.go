package main

import (
	"go/token"
	"testing"
)

var testPos = Positions{"NewLabel": {0}, "ShowErrorText": {1, 2}}

func scanTest(t *testing.T, src string) []Site {
	t.Helper()
	sites, err := ScanFile(token.NewFileSet(), "demo.go", []byte(src), testPos)
	if err != nil {
		t.Fatal(err)
	}
	return sites
}

func TestFindsHardcoded(t *testing.T) {
	sites := scanTest(t, `package demo

func f() {
	_ = NewLabel("Plain hardcode")
	ShowErrorText(nil, "Title here", "Body here")
}
`)
	if len(sites) != 3 {
		t.Fatalf("sites = %v, want 3", sites)
	}
}

func TestLocaleWrapIsLegal(t *testing.T) {
	sites := scanTest(t, `package demo

func f() {
	_ = NewLabel(locale.T("Wrapped"))
}
`)
	if len(sites) != 0 {
		t.Errorf("wrapped literal flagged: %v", sites)
	}
}

func TestExemptSameLineAndAbove(t *testing.T) {
	sites := scanTest(t, `package demo

func f() {
	_ = NewLabel("wire-tag") // l10n-exempt: wire tag
	// l10n-exempt: sample
	_ = NewLabel("sample text")
	_ = NewLabel("Not exempt")
}
`)
	if len(sites) != 1 || sites[0].Text != "Not exempt" {
		t.Errorf("sites = %v, want only Not exempt", sites)
	}
}

func TestSymbolsSkipped(t *testing.T) {
	sites := scanTest(t, `package demo

func f() {
	_ = NewLabel("?")
	_ = NewLabel("")
	_ = NewLabel("→ · ✕")
	_ = NewLabel("100%")
}
`)
	if len(sites) != 0 {
		t.Errorf("symbol-only literals flagged: %v", sites)
	}
}

func TestUnwatchedFunctionIgnored(t *testing.T) {
	sites := scanTest(t, `package demo

func f() {
	log("Some log message")
	_ = errors.New("not a display position")
}
`)
	if len(sites) != 0 {
		t.Errorf("unwatched calls flagged: %v", sites)
	}
}
