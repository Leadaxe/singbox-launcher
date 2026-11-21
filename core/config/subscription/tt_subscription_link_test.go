package subscription

import "testing"

// Ссылка-указатель на подписку отдаёт адрес подписки, ссылка с узлом — нет
// (контракт 1.1.176). Ссылки — из общего корпуса uri/trusttunnel.
func TestSubscriptionURLOfLink(t *testing.T) {
	const subOnly = "tt://?AAECDi1odHRwczovL3VzZXIxOnBhc3MxMjNAc3ViLmV4YW1wbGUtMS5jb20vcy9hYmM"
	got, ok := SubscriptionURLOfLink(subOnly)
	if !ok || got != "https://user1:pass123@sub.example-1.com/s/abc" {
		t.Fatalf("адрес подписки: %q, %v", got, ok)
	}
	const node = "tt://?AAECARF2cG4uZXhhbXBsZS0xLmNvbQUFdXNlcjEGB3Bhc3MxMjMCEDIwMy4wLjExMy4xMDo0NDMMBU5MLTAx"
	if u, ok := SubscriptionURLOfLink(node); ok {
		t.Fatalf("ссылка с узлом принята за подписку: %q", u)
	}
	if u, ok := SubscriptionURLOfLink("https://example.com/sub"); ok {
		t.Fatalf("обычный URL принят за ссылку-указатель: %q", u)
	}
}
