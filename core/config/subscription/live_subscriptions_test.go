//go:build live

package subscription

import (
	"bufio"
	"net/http"
	"strings"
	"testing"
)

// TestLiveParsePublicSubscriptionFiles fetches public list files and checks ParseNode on every vless/trojan/vmess line.
// Run: go test -tags=live ./core/config/subscription/... -run TestLiveParsePublicSubscriptionFiles -count=1
func TestLiveParsePublicSubscriptionFiles(t *testing.T) {
	urls := []string{
		"https://xray.abvpn.ru/vless/f4294d89-874b-4d9b-ab85-ddbc29bd87e2/126309188.json",
		"https://raw.githubusercontent.com/AvenCores/goida-vpn-configs/refs/heads/main/githubmirror/22.txt",
		"https://raw.githubusercontent.com/igareck/vpn-configs-for-russia/main/Vless-Reality-White-Lists-Rus-Mobile.txt",
		"https://raw.githubusercontent.com/igareck/vpn-configs-for-russia/main/BLACK_VLESS_RUS_mobile.txt",
	}
	for _, u := range urls {
		u := u
		t.Run(u, func(t *testing.T) {
			resp, err := http.Get(u)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status %s", resp.Status)
			}
			sc := bufio.NewScanner(resp.Body)
			const max = 1024 * 1024
			sc.Buffer(make([]byte, 0, 64*1024), max)
			var n, bad int
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if !strings.HasPrefix(line, "vless://") && !strings.HasPrefix(line, "trojan://") && !strings.HasPrefix(line, "vmess://") {
					continue
				}
				n++
				if _, err := ParseNode(line, nil); err != nil {
					bad++
					t.Errorf("parse error: %v\nline: %.200s", err, line)
				}
			}
			if err := sc.Err(); err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				t.Fatal("no protocol lines found")
			}
			t.Logf("ok lines=%d failures=%d", n, bad)
		})
	}
}
