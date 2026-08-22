package subscription

import (
	"strings"
	"testing"
)

// SPEC 103 B11: тело подписки в формате wg-quick.
//
// До этого .conf по ссылке подписки уходил в построчный разбор URI и давал
// ноль узлов молча — ни ошибки, ни предупреждения.

const wgConfBody = `[Interface]
PrivateKey = aFakeKeyForTestsOnlyNotARealSecret0000000000=
Address = 10.7.0.2/32

[Peer]
PublicKey = anotherFakeKeyForTestsOnly000000000000000000=
Endpoint = 198.51.100.10:51820
AllowedIPs = 0.0.0.0/0
`

func TestClassifyWGConfBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want BodyKind
	}{
		{"conf as-is", wgConfBody, BodyKindWGConf},
		{"leading comment", "# provider config\n" + wgConfBody, BodyKindWGConf},
		{"lowercase section", "[interface]\nPrivateKey = x\n[Peer]\nEndpoint = h:1\n", BodyKindWGConf},
		{"blank lines first", "\n\n" + wgConfBody, BodyKindWGConf},
		// Не conf — прежнее поведение обязано сохраниться.
		{"uri list", "vless://uuid@host:443?type=tcp#n", BodyKindURIList},
		{"empty", "", BodyKindURIList},
		{"json array still wins", `[{"type":"vless","tag":"a"}]`, BodyKindSingboxOutboundArray},
		{"bracket but not conf", "[not a section]\nfoo", BodyKindURIList},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifySubscriptionBody(tc.body); got != tc.want {
				t.Fatalf("ClassifySubscriptionBody = %v, want %v", got, tc.want)
			}
		})
	}
}

// Тело с несколькими [Interface] — провайдеры так отдают набор локаций.
func TestWGConfBodyToURIsMultipleBlocks(t *testing.T) {
	body := wgConfBody + "\n" + `[Interface]
PrivateKey = secondFakeKeyForTests00000000000000000000000=
Address = 10.7.0.3/32

[Peer]
PublicKey = peerTwoFakeKey000000000000000000000000000000=
Endpoint = 198.51.100.11:51820
AllowedIPs = 0.0.0.0/0
`
	uris, skipped := WGConfBodyToURIs(body)
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}
	if len(uris) != 2 {
		t.Fatalf("получено %d URI, ожидалось 2: %v", len(uris), uris)
	}
	for _, u := range uris {
		if !strings.HasPrefix(u, "wireguard://") {
			t.Fatalf("не wireguard-URI: %q", u)
		}
	}
}

// Блок без Endpoint непригоден: пропускается со счётчиком, остальные живут.
// Одна битая запись не должна обнулять подписку целиком.
func TestWGConfBodyToURIsSkipsBrokenBlock(t *testing.T) {
	body := "[Interface]\nPrivateKey = x\n\n[Peer]\nPublicKey = y\n\n" + wgConfBody
	uris, skipped := WGConfBodyToURIs(body)
	if skipped != 1 {
		t.Fatalf("skipped = %d, want 1", skipped)
	}
	if len(uris) != 1 {
		t.Fatalf("получено %d URI, ожидался 1", len(uris))
	}
}
