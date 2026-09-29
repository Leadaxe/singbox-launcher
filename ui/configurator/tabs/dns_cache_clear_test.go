package tabs

import (
	"errors"
	"testing"
)

// SPEC 147: кнопка «Clear DNS cache» зовёт ядро только при работающем ядре
// и включённом Clash API; итог и текст зависят от ответа ядра.
func TestClearDNSCacheOutcome(t *testing.T) {
	calls := 0
	okFlush := func(base, tok string) error {
		calls++
		if base != "http://127.0.0.1:9090" || tok != "s" {
			t.Fatalf("flush got %q %q", base, tok)
		}
		return nil
	}
	if ok, _ := clearDNSCacheOutcome(false, true, "http://127.0.0.1:9090", "s", okFlush); ok || calls != 0 {
		t.Fatalf("stopped core: ok=%v calls=%d", ok, calls)
	}
	if ok, _ := clearDNSCacheOutcome(true, false, "http://127.0.0.1:9090", "s", okFlush); ok || calls != 0 {
		t.Fatalf("api off: ok=%v calls=%d", ok, calls)
	}
	if ok, _ := clearDNSCacheOutcome(true, true, "", "s", okFlush); ok || calls != 0 {
		t.Fatalf("empty base: ok=%v calls=%d", ok, calls)
	}
	if ok, _ := clearDNSCacheOutcome(true, true, "http://127.0.0.1:9090", "s", okFlush); !ok || calls != 1 {
		t.Fatalf("running: ok=%v calls=%d", ok, calls)
	}
	fail := func(string, string) error { return errors.New("boom") }
	ok, msg := clearDNSCacheOutcome(true, true, "http://127.0.0.1:9090", "s", fail)
	if ok || msg == "" {
		t.Fatalf("flush error: ok=%v msg=%q", ok, msg)
	}
}
