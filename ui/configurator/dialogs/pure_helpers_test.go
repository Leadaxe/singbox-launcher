package dialogs

import (
	"reflect"
	"testing"
)

func TestNormalizeProcName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"plain name", "chrome.exe", "chrome.exe"},
		{"plain name trimmed", "  chrome.exe  ", "chrome.exe"},
		{"legacy PID prefix", "1234: chrome.exe", "chrome.exe"},
		{"legacy PID prefix with surrounding space", "  1234: chrome.exe  ", "chrome.exe"},
		{"legacy PID name has spaces", "42: Some App", "Some App"},
		{"name without space after colon is not split", "1234:chrome.exe", "1234:chrome.exe"},
		{"only first ': ' splits", "1: a: b", "a: b"},
		{"trailing space in name part trimmed", "7:   spaced   ", "spaced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeProcName(tt.in); got != tt.want {
				t.Errorf("normalizeProcName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSortProcessStrings(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, []string{}},
		{"empty", []string{}, []string{}},
		{"single", []string{"a"}, []string{"a"}},
		{"already sorted", []string{"alpha", "beta", "gamma"}, []string{"alpha", "beta", "gamma"}},
		{"reverse", []string{"gamma", "beta", "alpha"}, []string{"alpha", "beta", "gamma"}},
		{"case-insensitive order", []string{"Zebra", "apple", "Mango"}, []string{"apple", "Mango", "Zebra"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// sort is in place; operate on a copy of the input slice
			var items []string
			if tt.in != nil {
				items = make([]string, len(tt.in))
				copy(items, tt.in)
			}
			sortProcessStrings(items)
			got := items
			if got == nil {
				got = []string{}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("sortProcessStrings(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSortProcessStrings_InPlace(t *testing.T) {
	items := []string{"c", "a", "b"}
	ret := items // alias to confirm in-place mutation (func has no return)
	sortProcessStrings(items)
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(ret, want) {
		t.Errorf("sortProcessStrings did not sort underlying slice in place: got %v, want %v", ret, want)
	}
}

func TestDedupeProcessStrings(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, []string{}},
		{"empty", []string{}, []string{}},
		{"no dupes", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"case-insensitive dedupe keeps first", []string{"Chrome", "chrome", "CHROME"}, []string{"Chrome"}},
		{"empty and whitespace dropped", []string{"", "  ", "a"}, []string{"a"}},
		{"normalizes legacy PID prefix then dedupes", []string{"1: chrome.exe", "2: chrome.exe"}, []string{"chrome.exe"}},
		{"normalize then case-insensitive dedupe", []string{"10: Chrome", "chrome"}, []string{"Chrome"}},
		{"preserve first-seen order", []string{"b", "a", "B", "c", "A"}, []string{"b", "a", "c"}},
		{"empty entries dropped, valid kept", []string{"", "real", "  "}, []string{"real"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupeProcessStrings(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dedupeProcessStrings(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestExtractStringArray(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want []string
	}{
		{"nil", nil, nil},
		{"wrong type", 42, nil},
		{"string (not a slice)", "abc", nil},
		{"empty []interface{}", []interface{}{}, []string{}},
		{"[]interface{} of strings", []interface{}{"a", "b"}, []string{"a", "b"}},
		{"[]interface{} mixed types skips non-strings", []interface{}{"a", 1, "b", true, nil}, []string{"a", "b"}},
		{"[]interface{} all non-strings", []interface{}{1, 2, 3}, []string{}},
		{"[]string passthrough", []string{"x", "y", "z"}, []string{"x", "y", "z"}},
		{"empty []string", []string{}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractStringArray(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractStringArray(%#v) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseLines(t *testing.T) {
	tests := []struct {
		name             string
		text             string
		preserveOriginal bool
		want             []string
	}{
		{"empty string", "", false, []string{}},
		{"whitespace only", "   \n\t\n  ", false, []string{}},
		{"single line", "hello", false, []string{"hello"}},
		{"single line trimmed", "  hello  ", false, []string{"hello"}},
		{"single line preserve original", "  hello  ", true, []string{"  hello  "}},
		{"multiline trimmed drops empties", "a\n\n b \n\nc\n", false, []string{"a", "b", "c"}},
		{"multiline preserve original keeps spaces", " a \n\n  b  \n", true, []string{" a ", "  b  "}},
		{"blank lines only", "\n\n\n", false, []string{}},
		{"preserve original skips blank lines", "x\n   \ny", true, []string{"x", "y"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLines(tt.text, tt.preserveOriginal)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseLines(%q, %v) = %#v, want %#v", tt.text, tt.preserveOriginal, got, tt.want)
			}
		})
	}
}
