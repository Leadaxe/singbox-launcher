package tabs

import (
	"encoding/json"
	"testing"

	wizardtemplate "singbox-launcher/core/template"
)

// Ловушка на отбор переменных, принадлежащих ЗАПИСИ DNS-сервера.
//
// От него зависит, пересоберётся ли список серверов после правки параметра
// в окне «DNS server (from template)»: без пересборки строка продолжает
// показывать прежний outbound/адрес, потому что подписи считаются из
// значений переменных на момент сборки списка (dnsVarValues).
func TestDNSServerOwnedVar(t *testing.T) {
	td := &wizardtemplate.TemplateData{
		DNSOptionsRaw: json.RawMessage(`{"servers":[
			{"tag":"google_udp"},
			{"tag":"google_doh"},
			{"tag":"google_doh_vpn"}
		]}`),
	}

	cases := []struct {
		name string
		want bool
	}{
		// Параметры записей — правятся в окне сервера, влияют на подпись строки.
		{"dns_google_udp_outbound", true},
		{"dns_google_udp_dns_ip", true},
		{"dns_google_doh_vpn_outbound", true},
		// Переменные самой вкладки DNS: у них своя ветка (dnsTabOwnedVar).
		{"dns_strategy", false},
		{"dns_final", false},
		{"dns_default_domain_resolver", false},
		// Чужой префикс и точное совпадение с тегом без параметра.
		{"tun_address", false},
		{"dns_google_udp", false},
	}
	for _, c := range cases {
		if got := dnsServerOwnedVar(td, c.name); got != c.want {
			t.Errorf("dnsServerOwnedVar(%q) = %v, want %v", c.name, got, c.want)
		}
	}

	if dnsServerOwnedVar(nil, "dns_google_udp_outbound") {
		t.Error("nil template: ожидалось false")
	}
}
