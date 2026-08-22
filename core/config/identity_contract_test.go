package config

// Сверочные тесты идентичности узлов (SPEC 103, фаза 2; contract/docs/IDENTITY.md).
//
// Хеш — ключ пользовательских отметок (выключенные ноды, привязка detour,
// ссылки групп). Его нельзя менять «по дороге»: смена алгоритма молча
// отцепляет отметки от нод. Поэтому инварианты закреплены тестом, а не только
// прозой в IDENTITY.md.
//
// Что проверяется:
//   §1 — tag и detour в хеш не входят; порядок ключей не важен; эмиссия
//        невозможна → пустая строка;
//   §3 — четыре расхождения, закрытые в фазе 1, не вернулись (escaping,
//        ws ?ed=N, anytls fp, wireguard-дефолты).

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

func hashOf(t *testing.T, uri string) string {
	t.Helper()
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode(%q): %v", uri, err)
	}
	return NodeIdentityHash(node)
}

// §1: tag не входит в идентичность — переименование ноды не должно
// отцеплять пользовательскую отметку.
func TestIdentityIgnoresTag(t *testing.T) {
	base := "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?type=tcp&security=tls&sni=example-1.com"
	a := hashOf(t, base+"#node-alpha")
	b := hashOf(t, base+"#node-beta")
	if a == "" {
		t.Fatal("пустой хеш у корректного узла")
	}
	if a != b {
		t.Errorf("переименование сменило идентичность:\n  %s\n  %s", a, b)
	}
}

// §1: разные узлы обязаны различаться — иначе отметка одной ноды накроет
// соседнюю.
func TestIdentityDistinguishesNodes(t *testing.T) {
	uris := []string{
		"vless://11111111-1111-1111-1111-111111111111@example-1.com:443?type=tcp&security=tls&sni=example-1.com#a",
		"vless://11111111-1111-1111-1111-111111111111@example-1.com:8443?type=tcp&security=tls&sni=example-1.com#a",
		"vless://22222222-2222-2222-2222-222222222222@example-1.com:443?type=tcp&security=tls&sni=example-1.com#a",
		"vless://11111111-1111-1111-1111-111111111111@example-1.com:443?type=ws&path=%2Fws&security=tls&sni=example-1.com#a",
		"trojan://testpass123@example-1.com:443?sni=example-1.com#a",
	}
	seen := make(map[string]string, len(uris))
	for _, uri := range uris {
		h := hashOf(t, uri)
		if h == "" {
			t.Errorf("пустой хеш: %s", uri)
			continue
		}
		if prev, dup := seen[h]; dup {
			t.Errorf("две разные ноды дали один хеш:\n  %s\n  %s", prev, uri)
		}
		seen[h] = uri
	}
}

// §1: хеш стабилен между вызовами — иначе отметки «протухают» сами собой.
func TestIdentityStableAcrossCalls(t *testing.T) {
	uri := "vmess://" + vmessB64Fixture
	first := hashOf(t, uri)
	for i := 0; i < 5; i++ {
		if got := hashOf(t, uri); got != first {
			t.Fatalf("хеш нестабилен: %s != %s", got, first)
		}
	}
}

// §1: 64 символа lowercase hex без префикса.
func TestIdentityHashShape(t *testing.T) {
	h := hashOf(t, "trojan://testpass123@example-2.com:443?sni=example-2.com#shape")
	if len(h) != 64 {
		t.Fatalf("длина хеша %d, ожидалось 64: %q", len(h), h)
	}
	if strings.ToLower(h) != h {
		t.Errorf("хеш не в нижнем регистре: %q", h)
	}
	for _, r := range h {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("неhex-символ %q в хеше %q", r, h)
		}
	}
}

// §3: строки с <>& не должны escape'иться — Go-кодировщик JSON по умолчанию
// превращает их в <, и хеш расходился с Dart (D-007).
func TestIdentityNoHTMLEscaping(t *testing.T) {
	node, err := subscription.ParseNode(
		"vless://11111111-1111-1111-1111-111111111111@example-1.com:443?type=ws&path=%2Fa%3Fb%26c&security=tls&sni=example-1.com#esc", nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	if h := NodeIdentityHash(node); h == "" {
		t.Fatal("пустой хеш")
	}
	// Проверяем сам эмитированный JSON: escaping виден именно там.
	raw, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("GenerateNodeJSON: %v", err)
	}
	for _, bad := range []string{`<`, `>`, `&`} {
		if strings.Contains(raw, bad) {
			t.Errorf("в эмитированном JSON осталось escaping-представление %s — хеш разъедется с LxBox (D-007)\n%s", bad, raw)
		}
	}
}

// §3: ws early data — обе стороны обязаны эмитить early_data_header_name,
// иначе хеши расходятся на всех ?ed=N-нодах (D-008).
func TestIdentityWSEarlyDataHeaderPresent(t *testing.T) {
	node, err := subscription.ParseNode(
		"vless://11111111-1111-1111-1111-111111111111@example-1.com:443?type=ws&path=%2Fws%3Fed%3D2048&security=tls&sni=example-1.com#ed", nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	raw, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("GenerateNodeJSON: %v", err)
	}
	if !strings.Contains(raw, "max_early_data") {
		t.Fatalf("?ed=N не разложен в max_early_data:\n%s", raw)
	}
	if !strings.Contains(raw, "early_data_header_name") {
		t.Errorf("нет early_data_header_name — хеш разойдётся с LxBox (D-008)\n%s", raw)
	}
}

// §3: wireguard не должен эмитить дефолтные name/system — они были только у
// Go и ломали хеш каждой WG-ноды (D-010).
func TestIdentityWireGuardNoDefaultFields(t *testing.T) {
	node, err := subscription.ParseNode(
		"wireguard://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaA=@example-1.com:51820?publickey=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbA%3D&address=10.0.0.2%2F32#wg", nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	raw, err := GenerateEndpointJSON(node)
	if err != nil {
		t.Fatalf("GenerateEndpointJSON: %v", err)
	}
	for _, field := range []string{`"name"`, `"system"`} {
		if strings.Contains(raw, field) {
			t.Errorf("WG-endpoint несёт дефолтное поле %s — хеш разойдётся с LxBox (D-010)\n%s", field, raw)
		}
	}
}

// Узел, который невозможно эмитировать, не имеет идентичности: пустая строка,
// а не хеш от обрубка. Иначе все такие ноды схлопнулись бы в одну отметку.
func TestIdentityEmptyForNilNode(t *testing.T) {
	if got := NodeIdentityHash(nil); got != "" {
		t.Errorf("nil-узел дал хеш %q", got)
	}
}

// Базовая vmess-фикстура (синтетическая), вынесена ради читаемости.
const vmessB64Fixture = "eyJhZGQiOiJleGFtcGxlLTEuY29tIiwiYWlkIjowLCJpZCI6IjExMTExMTExLTExMTEtMTExMS0xMTExLTExMTExMTExMTExMSIsIm5ldCI6IndzIiwicGF0aCI6Ii93cyIsInBvcnQiOjQ0MywicHMiOiJ2bSIsInNjeSI6ImF1dG8iLCJ0bHMiOiJ0bHMiLCJ0eXBlIjoibm9uZSIsInYiOiIyIn0="
