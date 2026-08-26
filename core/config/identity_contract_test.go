package config

// Сверочные тесты идентичности узлов и канонической эмиссии
// (SPEC 103 фаза 2 + SPEC 112; contract/docs/IDENTITY.md).
//
// Идентичность — ключ пользовательских отметок (выключенные ноды, привязка
// detour). С SPEC 112 это ТЕГ узла в рамках источника, а не хеш содержимого;
// менять правило «по дороге» нельзя — смена молча отцепляет отметки от нод.
//
// Что проверяется:
//   §1 — идентичность = тег: содержимое её не меняет, разные имена различимы,
//        уникализация внутри источника, у групп идентичности нет;
//   §3 — канонические расхождения эмиссии, закрытые в фазе 1, не вернулись
//        (escaping, ws ?ed=N, wireguard-дефолты). Они по-прежнему нормативны:
//        на побайтовой эмиссии держится общий корпус, а не хеш.

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// identityOf разбирает URI и снимает идентичность ровно тем же путём, что
// и парсер источника: сначала штамп сырого тега, потом чтение.
func identityOf(t *testing.T, uri string, idCounts map[string]int) string {
	t.Helper()
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode(%q): %v", uri, err)
	}
	subscription.StampNodeIdentity(node, idCounts)
	return NodeIdentity(node)
}

// §1: идентичность — это тег. Содержимое ноды её не задаёт: провайдер вправе
// поменять сервер, порт, ключ и транспорт под тем же именем.
func TestIdentityIsTheTag(t *testing.T) {
	same := []string{
		"vless://11111111-1111-1111-1111-111111111111@example-1.com:443?type=tcp&security=tls&sni=example-1.com#🇩🇪 DE",
		"vless://22222222-2222-2222-2222-222222222222@example-9.com:8443?type=ws&path=%2Fws&security=tls&sni=example-9.com#🇩🇪 DE",
		"trojan://testpass123@example-5.com:443?sni=example-5.com#🇩🇪 DE",
	}
	for _, uri := range same {
		if got := identityOf(t, uri, map[string]int{}); got != "🇩🇪 DE" {
			t.Errorf("идентичность %q, ожидалась «🇩🇪 DE» (%s)", got, uri)
		}
	}
}

// §1: узлы с разными именами обязаны различаться — иначе отметка одной ноды
// накроет соседнюю.
func TestIdentityDistinguishesNodes(t *testing.T) {
	uris := []string{
		"vless://11111111-1111-1111-1111-111111111111@example-1.com:443?type=tcp&security=tls&sni=example-1.com#a",
		"vless://11111111-1111-1111-1111-111111111111@example-1.com:8443?type=tcp&security=tls&sni=example-1.com#b",
		"trojan://testpass123@example-1.com:443?sni=example-1.com#c",
	}
	seen := make(map[string]string, len(uris))
	for _, uri := range uris {
		id := identityOf(t, uri, map[string]int{})
		if id == "" {
			t.Errorf("пустая идентичность: %s", uri)
			continue
		}
		if prev, dup := seen[id]; dup {
			t.Errorf("две разные ноды дали одну идентичность:\n  %s\n  %s", prev, uri)
		}
		seen[id] = uri
	}
}

// §1: одноимённые узлы ОДНОГО источника разводятся тем же правилом, что теги
// конфига: первый «X», следующий «X-2».
func TestIdentityUniquifiesWithinSource(t *testing.T) {
	idCounts := map[string]int{}
	base := "vless://11111111-1111-1111-1111-111111111111@example-%d.com:443?type=tcp&security=tls&sni=example-1.com#🇳🇱 NL"
	want := []string{"🇳🇱 NL", "🇳🇱 NL-2", "🇳🇱 NL-3"}
	for i, expect := range want {
		uri := strings.Replace(base, "%d", string(rune('1'+i)), 1)
		if got := identityOf(t, uri, idCounts); got != expect {
			t.Fatalf("идентичность #%d = %q, ожидалась %q", i+1, got, expect)
		}
	}
}

// §1: идентичность стабильна между вызовами — иначе отметки «протухают» сами
// собой.
func TestIdentityStableAcrossCalls(t *testing.T) {
	uri := "vmess://" + vmessB64Fixture
	first := identityOf(t, uri, map[string]int{})
	if first == "" {
		t.Fatal("пустая идентичность")
	}
	for i := 0; i < 5; i++ {
		if got := identityOf(t, uri, map[string]int{}); got != first {
			t.Fatalf("идентичность нестабильна: %s != %s", got, first)
		}
	}
}

// §3: строки с <>& не должны escape'иться — Go-кодировщик JSON по умолчанию
// превращает их в &lt;, и эмиссия расходилась с Dart (D-007). Проверка живёт и
// после SPEC 112: на побайтовой эмиссии держится общий корпус.
func TestIdentityNoHTMLEscaping(t *testing.T) {
	node, err := subscription.ParseNode(
		"vless://11111111-1111-1111-1111-111111111111@example-1.com:443?type=ws&path=%2Fa%3Fb%26c&security=tls&sni=example-1.com#esc", nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	// Проверяем сам эмитированный JSON: escaping виден именно там.
	raw, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("GenerateNodeJSON: %v", err)
	}
	for _, bad := range []string{`<`, `>`, `&`} {
		if strings.Contains(raw, bad) {
			t.Errorf("в эмитированном JSON осталось escaping-представление %s — эмиссия разъедется с LxBox (D-007)\n%s", bad, raw)
		}
	}
}

// §3: ws early data — обе стороны обязаны эмитить early_data_header_name,
// иначе эмиссия расходится на всех ?ed=N-нодах (D-008).
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
		t.Errorf("нет early_data_header_name — эмиссия разойдётся с LxBox (D-008)\n%s", raw)
	}
}

// §3: wireguard не должен эмитить дефолтные name/system — они были только у
// Go и ломали эмиссию каждой WG-ноды (D-010).
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
			t.Errorf("WG-endpoint несёт дефолтное поле %s — эмиссия разойдётся с LxBox (D-010)\n%s", field, raw)
		}
	}
}

// Узел без имени идентичности не имеет: пустая строка, а не общий ключ «».
// Иначе все безымянные ноды схлопнулись бы в одну отметку.
func TestIdentityEmptyForNilAndUnnamed(t *testing.T) {
	if got := NodeIdentity(nil); got != "" {
		t.Errorf("nil-узел дал идентичность %q", got)
	}
	if got := NodeIdentity(&ParsedNode{Scheme: "vless"}); got != "" {
		t.Errorf("безымянный узел дал идентичность %q", got)
	}
}

// Базовая vmess-фикстура (синтетическая), вынесена ради читаемости.
const vmessB64Fixture = "eyJhZGQiOiJleGFtcGxlLTEuY29tIiwiYWlkIjowLCJpZCI6IjExMTExMTExLTExMTEtMTExMS0xMTExLTExMTExMTExMTExMSIsIm5ldCI6IndzIiwicGF0aCI6Ii93cyIsInBvcnQiOjQ0MywicHMiOiJ2bSIsInNjeSI6ImF1dG8iLCJ0bHMiOiJ0bHMiLCJ0eXBlIjoibm9uZSIsInYiOiIyIn0="
