package dialogs

import (
	"regexp"
	"testing"
)

func TestSimplePatternToRegex(t *testing.T) {
	tests := []struct {
		pattern string
		want   string
	}{
		{"*", "(.*)"},
		{"*/steam/*", "(.*)/steam/(.*)"},
		{"*\\Steam\\*", `(.*)\\Steam\\(.*)`},
		{"C:\\Games\\*", `C:\\Games\\(.*)`},
	}
	for _, tt := range tests {
		got, err := SimplePatternToRegex(tt.pattern)
		if err != nil {
			t.Errorf("SimplePatternToRegex(%q) error: %v", tt.pattern, err)
			continue
		}
		if got != tt.want {
			t.Errorf("SimplePatternToRegex(%q) = %q, want %q", tt.pattern, got, tt.want)
		}
		if _, err := regexp.Compile(got); err != nil {
			t.Errorf("SimplePatternToRegex(%q) produced invalid regex %q: %v", tt.pattern, got, err)
		}
	}
	// Empty pattern should produce invalid regex and error
	_, err := SimplePatternToRegex("")
	if err == nil {
		t.Error("SimplePatternToRegex(\"\") should return error")
	}
}

func TestSimplePatternToRegex_EscapeMeta(t *testing.T) {
	// Literal dots and other meta should be escaped
	got, err := SimplePatternToRegex(`path.to.file`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := regexp.Compile(got); err != nil {
		t.Errorf("invalid regex %q: %v", got, err)
	}
	// Should match "path.to.file" literally
	re := regexp.MustCompile(got)
	if !re.MatchString("path.to.file") {
		t.Errorf("regex %q should match path.to.file", got)
	}
}
