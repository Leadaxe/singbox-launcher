package business

import (
	"strings"
	"testing"
	"time"
)

type noopTiming struct{}

func (noopTiming) LogTiming(string, time.Duration) {}

// TestClassifyInputLines_WGConfText — SPEC 076: вставленный [Interface]/[Peer]
// текст выделяется до построчного разбора и попадает в connections как
// wireguard://-URI; ссылки в том же тексте продолжают классифицироваться.
func TestClassifyInputLines_WGConfText(t *testing.T) {
	input := "https://example.com/sub\n" +
		"vless://uuid@host:443?security=tls#srv\n" +
		"[Interface]\n" +
		"PrivateKey = UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMA=\n" +
		"Address = 10.8.1.2/32\n" +
		"Jc = 4\n" +
		"Jmin = 40\n" +
		"Jmax = 70\n" +
		"[Peer]\n" +
		"PublicKey = QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo=\n" +
		"AllowedIPs = 0.0.0.0/0\n" +
		"Endpoint = 203.0.113.7:38291\n"

	subs, conns := classifyInputLines(input, noopTiming{})
	if len(subs) != 1 || subs[0] != "https://example.com/sub" {
		t.Errorf("subscriptions = %v, want the single sub URL", subs)
	}
	if len(conns) != 2 {
		t.Fatalf("connections = %v, want vless + converted wireguard URI", conns)
	}
	if !strings.HasPrefix(conns[0], "vless://") {
		t.Errorf("conns[0] = %q, want the vless link", conns[0])
	}
	if !strings.HasPrefix(conns[1], "wireguard://") || !strings.Contains(conns[1], "jc=4") {
		t.Errorf("conns[1] = %q, want converted wireguard:// URI with AWG params", conns[1])
	}
}

// Невалидный блок пропускается, остальной ввод обрабатывается.
func TestClassifyInputLines_BrokenWGConfBlock(t *testing.T) {
	input := "vless://uuid@host:443#srv\n[Interface]\nPrivateKey = x\n"
	subs, conns := classifyInputLines(input, noopTiming{})
	if len(subs) != 0 || len(conns) != 1 || !strings.HasPrefix(conns[0], "vless://") {
		t.Errorf("broken conf block must not break the paste: subs=%v conns=%v", subs, conns)
	}
}
