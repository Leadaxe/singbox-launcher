package subscription

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
)

// issueConfigOneNode — конфиг из пользовательского issue (SPEC 121 §6):
// один endpoint и его связка — DNS-сервер через него, DNS-правило на его
// домены, правило маршрута на его подсеть.
//
// Endpoint здесь `wireguard`, а не `tailscale`: схема `tailscale` до SPEC 122
// импортом не принимается, и узел стал бы kind=unsupported — тогда тест
// проверял бы отказ, а не извлечение.
const issueConfigOneNode = `{"log":{"level":"info"},"inbounds":[{"type":"tun","tag":"tun-in","address":["172.18.0.1/30"],"auto_route":true}],
"endpoints":[{"type":"wireguard","tag":"ts-node","address":["10.0.0.2/32"],"private_key":"wG5+8u3v2Vz9Yk3sQm0y6c7QeX1uJm8h3G5rXK3XjE8=","peers":[{"address":"1.2.3.4","port":51820,"public_key":"Yc3lcHkOcKvzXnR4qzXqfE8ZQpLbGfW1p5qC1nZk3xk=","allowed_ips":["0.0.0.0/0"]}]}],
"dns":{"servers":[{"type":"local","tag":"local-dns"},{"type":"udp","tag":"ts-dns","server":"100.100.100.100","detour":"ts-node"}],"rules":[{"domain_suffix":[".ts.net"],"server":"ts-dns"}],"final":"local-dns"},
"route":{"auto_detect_interface":true,"default_domain_resolver":"local-dns","rules":[{"ip_cidr":["100.64.0.0/10"],"outbound":"ts-node"}],"final":"proxy-out"},
"outbounds":[{"type":"selector","tag":"proxy-out","outbounds":["direct"]},{"type":"direct","tag":"direct"}]}`

func TestParseSingboxBody_NodeSectionsFromWholeConfig(t *testing.T) {
	res, err := ParseSingboxBody(issueConfigOneNode, BodyKindSingboxConfig, nil)
	if err != nil {
		t.Fatalf("ParseSingboxBody: %v", err)
	}
	// selector — группа, узлом связки не считается; direct — служебный тип.
	var sections *configtypes.NodeSections
	found := false
	for _, n := range res.Nodes {
		if n != nil && n.Tag == "ts-node" {
			found = true
			sections = n.Sections
		}
	}
	if !found {
		t.Fatalf("node ts-node not imported; got %d node(s)", len(res.Nodes))
	}
	if sections.IsEmpty() {
		t.Fatalf("node ts-node carries no sections")
	}
	if res.SectionFragments != 3 {
		t.Fatalf("SectionFragments = %d, want 3", res.SectionFragments)
	}

	// Извлечённое приезжает в ХРАНИМОЙ форме (SPEC 121 §10.1) — тем же
	// переводом, что у вкладки JSON узла.
	decoded := corestate.NodeSectionsFromConfigTypes(sections)
	if decoded == nil {
		t.Fatal("extracted sections cannot be read back")
	}

	if got := len(decoded.DNSServers()); got != 1 {
		t.Fatalf("dns servers = %d, want 1 (local-dns must not be taken)", got)
	}
	srv := decoded.DNSServers()[0]
	if srv.Kind != corestate.DNSServerKindUser || srv.Tag != "ts-dns" {
		t.Fatalf("dns server entry = %+v, want a user entry tagged ts-dns", srv)
	}
	if srv.Body["type"] != "udp" || srv.Body["server"] != "100.100.100.100" {
		t.Fatalf("dns server body = %v", srv.Body)
	}
	if srv.Body["detour"] != corestate.SelfPlaceholder {
		t.Fatalf("detour = %v, want %s", srv.Body["detour"], corestate.SelfPlaceholder)
	}

	if got := len(decoded.DNSRules()); got != 1 {
		t.Fatalf("dns rules = %d, want 1", got)
	}
	if got := decoded.DNSRules()[0].Body["server"]; got != "ts-dns" {
		t.Fatalf("dns rule server = %v, want ts-dns (the node's own server)", got)
	}

	if got := len(decoded.Rules); got != 1 {
		t.Fatalf("route rules = %d, want 1", got)
	}
	routeRule := decoded.Rules[0]
	if routeRule.Kind != corestate.RuleKindInline {
		t.Fatalf("route rule kind = %q, want inline", routeRule.Kind)
	}
	routeBody, err := routeRule.DecodeBody()
	if err != nil {
		t.Fatalf("route rule body: %v", err)
	}
	if got := routeBody.(*corestate.InlineBody).Outbound; got != corestate.SelfPlaceholder {
		t.Fatalf("route rule outbound = %v, want %s", got, corestate.SelfPlaceholder)
	}
}

// TestParseSingboxBody_NoSectionsWithTwoNodes — правило извлечения узкое:
// связка принадлежит узлу, и с двумя узлами непонятно, чья она.
func TestParseSingboxBody_NoSectionsWithTwoNodes(t *testing.T) {
	twoNodes := `{"endpoints":[{"type":"wireguard","tag":"ts-node","address":["10.0.0.2/32"],"private_key":"wG5+8u3v2Vz9Yk3sQm0y6c7QeX1uJm8h3G5rXK3XjE8=","peers":[{"address":"1.2.3.4","port":51820,"public_key":"Yc3lcHkOcKvzXnR4qzXqfE8ZQpLbGfW1p5qC1nZk3xk=","allowed_ips":["0.0.0.0/0"]}]}],
"outbounds":[{"type":"socks","tag":"sock","server":"127.0.0.1","server_port":1080}],
"dns":{"servers":[{"type":"udp","tag":"ts-dns","server":"100.100.100.100","detour":"ts-node"}],"rules":[{"domain_suffix":[".ts.net"],"server":"ts-dns"}]},
"route":{"rules":[{"ip_cidr":["100.64.0.0/10"],"outbound":"ts-node"}]}}`

	res, err := ParseSingboxBody(twoNodes, BodyKindSingboxConfig, nil)
	if err != nil {
		t.Fatalf("ParseSingboxBody: %v", err)
	}
	if res.SectionFragments != 0 {
		t.Fatalf("SectionFragments = %d, want 0 with two nodes", res.SectionFragments)
	}
	for _, n := range res.Nodes {
		if n != nil && !n.Sections.IsEmpty() {
			t.Fatalf("node %q got sections, but the config carries two nodes", n.Tag)
		}
	}
	// Секции по-прежнему числятся игнорированными — ровно как сегодня.
	if len(res.IgnoredSections) == 0 {
		t.Fatalf("IgnoredSections is empty; dns/route must still be reported as ignored")
	}
}
