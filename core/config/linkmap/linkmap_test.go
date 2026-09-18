package linkmap

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/config/registry"
)

// TestLinkmapEngineW0 — движок W0: предикаты detect, разрешение
// неоднозначности по priority, пространство источников.
//
// Тест ОДИН на волну (политика проверок, TASKS §«Политика проверок»):
// корпус остаётся главным тестом, а здесь проверяется то, чего корпус
// коснуться не может, пока ни одна схема на движок не переведена.
func TestLinkmapEngineW0(t *testing.T) {
	t.Run("detect различает четыре вида источника", func(t *testing.T) {
		// Предикаты — те же, что уедут в registry/sources.json; проверяется,
		// что они разделяют реальные тела, а не абстрактные строки.
		xray := mustDetect(t, `{"json":{"type_of":{"outbounds":"array"},"array_elem_any_keys":["outbounds[].protocol"]}}`)
		sbOutbound := mustDetect(t, `{"json":{"required_keys":["type"]}}`)
		sbConfig := mustDetect(t, `{"json":{"any_keys":["outbounds","endpoints"],"key_absent":["type"]}}`)
		wgconf := mustDetect(t, `{"ini":{"sections":["Interface"]}}`)
		vpnLink := mustDetect(t, `{"text":{"prefix_fold":"vpn://"}}`)

		cases := []struct {
			name  string
			text  string
			match *registry.Detect
			miss  []*registry.Detect
		}{
			{
				name:  "xray-конфиг",
				text:  `{"outbounds":[{"protocol":"vless","settings":{}}]}`,
				match: xray,
				miss:  []*registry.Detect{sbOutbound, wgconf, vpnLink},
			},
			{
				name:  "одиночный sing-box outbound",
				text:  `{"type":"vless","server":"a.com","server_port":443}`,
				match: sbOutbound,
				// key_absent:["type"] обязан отсечь: одиночный selector
				// несёт и type, и outbounds — и это outbound, не конфиг.
				miss: []*registry.Detect{xray, sbConfig, wgconf},
			},
			{
				name:  "sing-box конфиг с endpoints",
				text:  `{"endpoints":[{"type":"wireguard"}]}`,
				match: sbConfig,
				miss:  []*registry.Detect{sbOutbound, xray, wgconf},
			},
			{
				name:  "wg-quick",
				text:  "[Interface]\nPrivateKey = aaa\n[Peer]\nEndpoint = h:51820\n",
				match: wgconf,
				miss:  []*registry.Detect{xray, sbOutbound, sbConfig},
			},
			{
				name:  "amnezia vpn://",
				text:  "VPN://abcdef",
				match: vpnLink,
				miss:  []*registry.Detect{xray, sbOutbound, wgconf},
			},
		}

		for _, tc := range cases {
			c := NewContent(tc.text)
			if !Matches(tc.match, c) {
				t.Errorf("%s: свой предикат не сработал", tc.name)
			}
			for i, m := range tc.miss {
				if Matches(m, c) {
					t.Errorf("%s: сработал чужой предикат #%d", tc.name, i)
				}
			}
		}
	})

	t.Run("порядок решает неоднозначность, default не конкурирует", func(t *testing.T) {
		// Тело несёт и type, и outbounds — под оба предиката сразу.
		// Норма (SPEC 133 §3A.3): побеждает меньший priority.
		text := `{"type":"selector","outbounds":["a","b"]}`
		cands := []Candidate{
			SourceCandidate{Kind: registry.SourceKind{
				Kind: "singbox_config", Priority: 50,
				Detect: mustDetect(t, `{"json":{"any_keys":["outbounds"]}}`)}},
			SourceCandidate{Kind: registry.SourceKind{
				Kind: "singbox_outbound", Priority: 40,
				Detect: mustDetect(t, `{"json":{"required_keys":["type"]}}`)}},
			SourceCandidate{Kind: registry.SourceKind{
				Kind: "uri_list", Priority: 10,
				Detect: mustDetect(t, `{"default":true}`)}},
		}

		res := Select(cands, NewContent(text))
		if res.Index < 0 || cands[res.Index].Name() != "singbox_outbound" {
			t.Fatalf("победил %v, ожидался singbox_outbound", res.Index)
		}
		if len(res.Matched) != 2 {
			t.Errorf("сработавших %d, ожидалось 2 (неоднозначность видна линтеру)", len(res.Matched))
		}
		if res.ByDefault {
			t.Error("default не должен выигрывать при сработавшем предикате")
		}

		// Ничего не подошло — берётся default, хотя его priority наименьший.
		res = Select(cands, NewContent("vless://x@h:443"))
		if res.Index < 0 || cands[res.Index].Name() != "uri_list" || !res.ByDefault {
			t.Fatalf("фолбэк не сработал: %+v", res)
		}
	})

	t.Run("source — единственный доступ к значению", func(t *testing.T) {
		s := &Space{Scheme: "vless", Host: "a.com", Port: 443, PortRaw: "443", Fragment: "метка"}
		s.SetQuery(ParseQueryOrdered("sni=x.com&SNI=y.com&pbk=a%2Bb&path=%2Fws%2Bv2"))

		// Регистронезависимость с приоритетом ТОЧНОГО совпадения: при двух
		// написаниях побеждает канон, а не порядок обхода Go-map.
		if v, _ := s.Lookup("query.sni"); v != "x.com" {
			t.Errorf("query.sni = %q, ожидалось x.com (точное совпадение)", v)
		}
		if v, _ := s.Lookup("query.Sni"); v != "x.com" {
			t.Errorf("query.Sni = %q, ожидалось x.com (первое по порядку)", v)
		}

		// `+` доезжает до записи СЫРЫМ: политика — свойство записи
		// (plus_literal от format), а не разбора.
		if v, _ := s.Lookup("query.pbk"); v != "a+b" {
			t.Errorf("query.pbk = %q, ожидалось a+b (сырой плюс)", v)
		}
		if got := PlusToSpace("a+b"); got != "a b" {
			t.Errorf("PlusToSpace = %q, ожидалось 'a b'", got)
		}

		// Несуществующий источник не даёт значения: доступа мимо source нет.
		if _, ok := s.Lookup("query.nope"); ok {
			t.Error("незаявленный источник вернул значение")
		}
		if _, ok := s.Lookup("json.any"); ok {
			t.Error("json-источник доступен в url-пространстве")
		}
	})

	t.Run("authority режет лексер, не url.Parse", func(t *testing.T) {
		cases := []struct{ in, user, host, port string }{
			// multi-port hysteria: платформенный парсер URL отказывает целиком.
			{"pw@h.com:443,20000-30000", "pw", "h.com", "443,20000-30000"},
			{"[2001:db8::1]:8443", "", "[2001:db8::1]", "8443"},
			// Голый IPv6: несколько ':' — портом это не является (G7).
			{"2001:db8::1", "", "2001:db8::1", ""},
			{"h.com:443", "", "h.com", "443"},
		}
		for _, tc := range cases {
			u, h, p := SplitAuthority(tc.in)
			if u != tc.user || h != tc.host || p != tc.port {
				t.Errorf("SplitAuthority(%q) = (%q,%q,%q), ожидалось (%q,%q,%q)",
					tc.in, u, h, p, tc.user, tc.host, tc.port)
			}
		}
		if PortOf("443,20000-30000") != 0 {
			t.Error("multi-port не должен приводиться к числу — он читается из port_raw")
		}
		if PortOf("443") != 443 {
			t.Error("обычный порт обязан приводиться")
		}
	})

	t.Run("json-скаляр не склеивает массив в строку", func(t *testing.T) {
		// Живой баг xrayMapString: ["a.com","b.com"] через fmt.Sprint давал
		// "[a.com b.com]" и уезжал в тело как host.
		var v interface{}
		if err := json.Unmarshal([]byte(`{"httpSettings":{"host":["a.com","b.com"]}}`), &v); err != nil {
			t.Fatal(err)
		}
		s := &Space{}
		s.SetJSON(v)

		if got, ok := s.Lookup("json.httpSettings.host"); ok {
			t.Errorf("массив отдан как скаляр %q — это и есть сегодняшний баг", got)
		}
		raw, ok := s.LookupRaw("json.httpSettings.host")
		if !ok {
			t.Fatal("LookupRaw не нашёл массив")
		}
		if arr, isArr := raw.([]interface{}); !isArr || len(arr) != 2 {
			t.Errorf("LookupRaw вернул %T, ожидался массив из 2", raw)
		}
	})

	t.Run("ini: секции, комментарий под [Peer], индекс массива в пути", func(t *testing.T) {
		text := "[Interface]\nPrivateKey = KEY\nJc = 4\n[Peer]\n# US-FREE#137\nPublicKey = PUB\n"
		c := NewContent(text)

		// Род узла объявляется ДАННЫМИ (keys_any), а не функцией hasAWGParams.
		if !Matches(mustDetect(t, `{"ini":{"keys_any":["Jc","Jmin"]}}`), c) {
			t.Error("awg-признак по ключам не сработал")
		}
		if Matches(mustDetect(t, `{"ini":{"keys_any":["H1","I1"]}}`), c) {
			t.Error("awg3-признак сработал на awg2-файле")
		}

		sections, ok := c.INI()
		if !ok || sections["interface"]["privatekey"] != "KEY" {
			t.Fatalf("ini разобран неверно: %+v", sections)
		}
		// Ключи в нижнем регистре, значения as-is — диалект сохранён дословно.
		if sections["peer"]["publickey"] != "PUB" {
			t.Errorf("peer.publickey = %q", sections["peer"]["publickey"])
		}
	})
}

func mustDetect(t *testing.T, raw string) *registry.Detect {
	t.Helper()
	d := &registry.Detect{}
	if err := json.Unmarshal([]byte(raw), d); err != nil {
		t.Fatalf("detect %s: %v", raw, err)
	}
	return d
}
