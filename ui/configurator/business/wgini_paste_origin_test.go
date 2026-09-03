package business

import (
	"strings"
	"testing"

	corestate "singbox-launcher/core/state"
)

// TestPastedWGConfKeepsBlockAsOrigin — вставка конфига wg-quick руками даёт
// узел, чьё происхождение — САМ БЛОК, а не выведенная из него ссылка
// (SPEC 119).
//
// До правки классификатор конвертировал блок в wireguard://-URI ещё до
// материализации, и в origin уезжала ссылка: комментарии блока (метка
// локации, закомментированный запасной Endpoint) пропадали, а Regen
// пересобирал узел из нашего же вывода.
func TestPastedWGConfKeepsBlockAsOrigin(t *testing.T) {
	const block = `[Interface]
# VPN Accelerator = on
PrivateKey = 0GCSi+xv9uacc7rK5S8WmwNlf/eqD/6+I34xw6+iDnU=
Address = 10.2.0.2/32
DNS = 10.2.0.1

[Peer]
# US-FREE#109
PublicKey = 0q5TxQQMNVQ6wEcLqnHa20G0DP/fpk8YdgLJUYApfTo=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = 149.22.88.129:51820

# Uncomment the following line to connect using IPv6.
# Endpoint = [2a02:6ea0:d802:5519::10]:51820
PersistentKeepalive = 25`

	parsed, err := parseSourceInput(block, 0)
	if err != nil {
		t.Fatalf("разбор вставленного конфига: %v", err)
	}
	if len(parsed.Nodes) != 1 {
		t.Fatalf("узлов %d, ожидался 1", len(parsed.Nodes))
	}
	node := parsed.Nodes[0]
	if node.Origin == nil {
		t.Fatal("у узла нет происхождения")
	}
	if node.Origin.Kind != corestate.OriginKindWGIni {
		t.Errorf("origin.kind = %q, ожидался %q", node.Origin.Kind, corestate.OriginKindWGIni)
	}
	if node.Origin.Raw != block {
		t.Errorf("origin.raw не равен вставленному блоку:\n--- получено ---\n%s", node.Origin.Raw)
	}
	for _, must := range []string{"# US-FREE#109", "# Endpoint = [2a02:6ea0:d802:5519::10]:51820"} {
		if !strings.Contains(node.Origin.Raw, must) {
			t.Errorf("в origin.raw потерян комментарий %q", must)
		}
	}
}

// TestPastedLinkKeepsItselfAsOrigin — у обычной ссылки исходник это она сама:
// пара «URI + raw» не должна сместить origin соседних записей.
func TestPastedLinkKeepsItselfAsOrigin(t *testing.T) {
	const link = "vless://aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee@host.example:443?encryption=none&type=tcp&security=tls#SRV"
	parsed, err := parseSourceInput(link, 0)
	if err != nil {
		t.Fatalf("разбор ссылки: %v", err)
	}
	if len(parsed.Nodes) != 1 {
		t.Fatalf("узлов %d, ожидался 1", len(parsed.Nodes))
	}
	origin := parsed.Nodes[0].Origin
	if origin == nil || origin.Kind != corestate.OriginKindURI || origin.Raw != link {
		t.Fatalf("происхождение ссылки испорчено: %+v", origin)
	}
}

// TestPastedMixedLinkAndConfOrigins — ссылки и конфиг вставлены вместе:
// каждому узлу достаётся СВОЙ исходник, а не соседский.
func TestPastedMixedLinkAndConfOrigins(t *testing.T) {
	const link = "vless://aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee@host.example:443?encryption=none&type=tcp&security=tls#SRV"
	const block = `[Interface]
PrivateKey = 0GCSi+xv9uacc7rK5S8WmwNlf/eqD/6+I34xw6+iDnU=
Address = 10.2.0.2/32

[Peer]
# NL-1
PublicKey = 0q5TxQQMNVQ6wEcLqnHa20G0DP/fpk8YdgLJUYApfTo=
Endpoint = 1.1.1.1:51820`

	parsed, err := parseSourceInput(link+"\n"+block, 0)
	if err != nil {
		t.Fatalf("разбор смешанного ввода: %v", err)
	}
	if len(parsed.Nodes) != 2 {
		t.Fatalf("узлов %d, ожидалось 2", len(parsed.Nodes))
	}
	var sawURI, sawINI bool
	for _, n := range parsed.Nodes {
		if n.Origin == nil {
			t.Fatal("у узла нет происхождения")
		}
		switch n.Origin.Kind {
		case corestate.OriginKindURI:
			sawURI = true
			if n.Origin.Raw != link {
				t.Errorf("ссылка получила чужой исходник: %q", n.Origin.Raw)
			}
		case corestate.OriginKindWGIni:
			sawINI = true
			if !strings.Contains(n.Origin.Raw, "# NL-1") {
				t.Errorf("блок получил чужой исходник: %q", n.Origin.Raw)
			}
		}
	}
	if !sawURI || !sawINI {
		t.Fatalf("ожидались оба вида происхождения, получено uri=%v ini=%v", sawURI, sawINI)
	}
}
