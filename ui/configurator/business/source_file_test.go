package business

import (
	"strings"
	"testing"
)

func TestReadSourceFileText(t *testing.T) {
	t.Run("trims and returns content", func(t *testing.T) {
		got, err := ReadSourceFileText(strings.NewReader("  vless://u@h:443#a\n\n"))
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got != "vless://u@h:443#a" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("conf text preserved (inner newlines kept)", func(t *testing.T) {
		conf := "[Interface]\nPrivateKey = x\n[Peer]\nEndpoint = h:1\n"
		got, err := ReadSourceFileText(strings.NewReader(conf))
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !strings.HasPrefix(got, "[Interface]") || !strings.Contains(got, "[Peer]") {
			t.Errorf("conf mangled: %q", got)
		}
	})

	t.Run("nil reader errors", func(t *testing.T) {
		if _, err := ReadSourceFileText(nil); err == nil {
			t.Error("want error for nil reader")
		}
	})

	t.Run("over-cap errors, not truncated", func(t *testing.T) {
		big := strings.NewReader(strings.Repeat("a", maxSourceFileBytes+10))
		if _, err := ReadSourceFileText(big); err == nil {
			t.Error("want error for oversized file")
		}
	})

	t.Run("exactly at cap is fine", func(t *testing.T) {
		atCap := strings.NewReader(strings.Repeat("a", maxSourceFileBytes))
		got, err := ReadSourceFileText(atCap)
		if err != nil {
			t.Fatalf("at-cap err: %v", err)
		}
		if len(got) != maxSourceFileBytes {
			t.Errorf("len = %d, want %d", len(got), maxSourceFileBytes)
		}
	})
}
