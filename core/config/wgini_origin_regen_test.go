package config

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// protonConfBlock — конфиг Proton VPN как его отдаёт провайдер (SPEC 119).
// Комментарии тут несут рабочие данные: метку локации и ГОТОВЫЙ запасной
// IPv6-endpoint, который пользователь включает, сняв решётку.
const protonConfBlock = `[Interface]
# Bouncing = 0
# NAT-PMP (Port Forwarding) = off
# VPN Accelerator = on
PrivateKey = 0GCSi+xv9uacc7rK5S8WmwNlf/eqD/6+I34xw6+iDnU=
Address = 10.2.0.2/32, 2a07:b944::2:2/128
DNS = 10.2.0.1, 2a07:b944::2:1

[Peer]
# US-FREE#109
PublicKey = 0q5TxQQMNVQ6wEcLqnHa20G0DP/fpk8YdgLJUYApfTo=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = 149.22.88.129:51820

# Uncomment the following line (delete the # symbol) to connect to Proton VPN using IPv6.
# Endpoint = [2a02:6ea0:d802:5519::10]:51820
PersistentKeepalive = 25`

// TestWGIniOriginKeepsBlockVerbatim — фаза 1: узел из тела wg-quick несёт
// СВОЙ блок байт в байт. До SPEC 119 в origin уезжал сгенерированный нами
// wireguard://-URI, и комментарии блока пропадали вместе с запасным
// endpoint'ом — пересобрать узел из конфига провайдера было нечем.
func TestWGIniOriginKeepsBlockVerbatim(t *testing.T) {
	res, err := subscription.ParseSubscriptionBody([]byte(protonConfBlock), nil, 100)
	if err != nil {
		t.Fatalf("разбор тела: %v", err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("узлов %d, ожидался 1 (rejected=%d)", len(res.Entries), len(res.Rejected))
	}
	e := res.Entries[0]
	if e.OriginKind != subscription.OriginKindWGIni {
		t.Errorf("origin.kind = %q, ожидался %q", e.OriginKind, subscription.OriginKindWGIni)
	}
	if e.OriginRaw != protonConfBlock {
		t.Errorf("origin.raw не равен блоку байт в байт:\n--- получено ---\n%s", e.OriginRaw)
	}
	for _, must := range []string{"# US-FREE#109", "# Endpoint = [2a02:6ea0:d802:5519::10]:51820"} {
		if !strings.Contains(e.OriginRaw, must) {
			t.Errorf("в origin.raw потерян комментарий %q", must)
		}
	}
}

// TestWGIniOriginPerBlock — многосекционный conf: каждому узлу свой блок, а
// не весь файл. Иначе Regen любого узла пересобирал бы соседей.
func TestWGIniOriginPerBlock(t *testing.T) {
	body := `[Interface]
PrivateKey = 0GCSi+xv9uacc7rK5S8WmwNlf/eqD/6+I34xw6+iDnU=
Address = 10.2.0.2/32

[Peer]
# NL-1
PublicKey = 0q5TxQQMNVQ6wEcLqnHa20G0DP/fpk8YdgLJUYApfTo=
Endpoint = 1.1.1.1:51820

[Interface]
PrivateKey = 0GCSi+xv9uacc7rK5S8WmwNlf/eqD/6+I34xw6+iDnU=
Address = 10.2.0.3/32

[Peer]
# US-2
PublicKey = 0q5TxQQMNVQ6wEcLqnHa20G0DP/fpk8YdgLJUYApfTo=
Endpoint = 2.2.2.2:51820
`
	res, err := subscription.ParseSubscriptionBody([]byte(body), nil, 100)
	if err != nil {
		t.Fatalf("разбор тела: %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("узлов %d, ожидалось 2", len(res.Entries))
	}
	first, second := res.Entries[0].OriginRaw, res.Entries[1].OriginRaw
	if strings.Contains(first, "US-2") || strings.Contains(second, "NL-1") {
		t.Fatal("блоки перепутаны: узел несёт чужой исходник")
	}
	if !strings.Contains(first, "1.1.1.1") || !strings.Contains(second, "2.2.2.2") {
		t.Fatalf("узел несёт не свой блок:\n1: %s\n2: %s", first, second)
	}
}

// TestWGIniRegenAppliesEditedBlock — фаза 2: пересборка из ПРАВЛЕНОГО
// исходника. Ровно сценарий Proton: пользователь снимает решётку с
// IPv6-endpoint и ставит её на IPv4.
func TestWGIniRegenAppliesEditedBlock(t *testing.T) {
	mat, err := MaterializeServerNode(protonConfBlock, nil)
	if err != nil {
		t.Fatalf("материализация блока: %v", err)
	}
	if mat.OriginKind != "wg_ini" || mat.OriginRaw != protonConfBlock {
		t.Fatalf("происхождение не сохранено: kind=%q", mat.OriginKind)
	}
	if !strings.Contains(string(mat.Body), "149.22.88.129") {
		t.Fatalf("исходный endpoint не попал в тело: %s", mat.Body)
	}

	edited := strings.Replace(protonConfBlock,
		"Endpoint = 149.22.88.129:51820", "# Endpoint = 149.22.88.129:51820", 1)
	edited = strings.Replace(edited,
		"# Endpoint = [2a02:6ea0:d802:5519::10]:51820", "Endpoint = [2a02:6ea0:d802:5519::10]:51820", 1)

	mat2, err := MaterializeServerNode(edited, nil)
	if err != nil {
		t.Fatalf("пересборка из правленого блока: %v", err)
	}
	if !strings.Contains(string(mat2.Body), "2a02:6ea0:d802:5519::10") {
		t.Fatalf("IPv6-endpoint не применился: %s", mat2.Body)
	}
	if strings.Contains(string(mat2.Body), "149.22.88.129") {
		t.Fatalf("закомментированный IPv4-endpoint остался в теле: %s", mat2.Body)
	}
	if mat2.OriginRaw != edited {
		t.Fatal("origin.raw не обновился правленым текстом — следующий Regen потерял бы правку")
	}
}

// TestWGIniRegenRejectsMultiBlock — один узел собирается ровно из одного
// блока; текст с двумя секциями неоднозначен, и выбирать наугад нельзя.
func TestWGIniRegenRejectsMultiBlock(t *testing.T) {
	if _, err := MaterializeServerNode(protonConfBlock+"\n\n"+protonConfBlock, nil); err == nil {
		t.Fatal("текст с двумя [Interface] должен отвергаться")
	}
}
