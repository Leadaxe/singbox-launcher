package subscription

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// TestDuplicatesCollapsedNamedOnSurvivor — SPEC 154: провайдер кладёт один
// сервер в подписку под несколькими именами, парсер схлопывает повторы, а
// выживший узел несёт код duplicates_collapsed с числом и именами схлопнутых.
// Обе формы, которые провайдер отдаёт по User-Agent: список ссылок и
// Xray-массив с балансировщиком (члены пула в имена не входят).
func TestDuplicatesCollapsedNamedOnSurvivor(t *testing.T) {
	const pbk = "Lx2TnXgn4YbYp6h6rVr3m2DkQnq1Yl7yY0pYf0bR3X0"
	servers := map[string]string{
		"a": "a.example.com",
		"b": "b.example.com",
	}
	uuid := func(s string) string {
		return fmt.Sprintf("0000000%s-0000-4000-8000-000000000000", s)
	}
	link := func(s, name string) string {
		return fmt.Sprintf("vless://%s@%s:443?type=tcp&security=reality&pbk=%s&fp=chrome&sni=www.example.org&sid=ab12&flow=xtls-rprx-vision#%s",
			uuid(s), servers[s], pbk, url.PathEscape(name))
	}

	t.Run("uri_list", func(t *testing.T) {
		body := strings.Join([]string{
			link("a", "🇦🇹 Австрия"),
			link("b", "🇵🇱 Польша"),
			link("a", "🇩🇪 Германия"),
			link("b", "🇵🇱 Польша"),
			link("a", "🇷🇺 Россия"),
		}, "\n")
		pb, err := ParseSubscriptionBody([]byte(body), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		got := collapsedByTag(pb)
		want := map[string]string{
			"🇦🇹 Австрия": "2|🇩🇪 Германия, 🇷🇺 Россия",
			// Повтор под тем же именем: называть некого, names — своё имя.
			"🇵🇱 Польша": "1|🇵🇱 Польша",
		}
		assertCollapsed(t, got, want, len(pb.Entries), 2)
		if len(pb.Rejected) != 0 {
			t.Errorf("схлопнутые записи попали в Rejected: %+v", pb.Rejected)
		}
	})

	t.Run("xray_array", func(t *testing.T) {
		outbound := func(tag, s string) string {
			return fmt.Sprintf(`{"tag":%q,"protocol":"vless","settings":{"vnext":[{"address":%q,"port":443,"users":[{"id":%q,"encryption":"none","flow":"xtls-rprx-vision"}]}]},"streamSettings":{"network":"tcp","security":"reality","realitySettings":{"serverName":"www.example.org","publicKey":%q,"shortId":"ab12","fingerprint":"chrome"}}}`,
				tag, servers[s], uuid(s), pbk)
		}
		single := func(remarks, s string) string {
			return fmt.Sprintf(`{"remarks":%q,"outbounds":[%s,{"tag":"direct","protocol":"freedom"}]}`,
				remarks, outbound("proxy", s))
		}
		pool := fmt.Sprintf(`{"remarks":"Авто","outbounds":[%s,%s,{"tag":"direct","protocol":"freedom"}],`+
			`"routing":{"balancers":[{"tag":"Balancer","selector":["bridge"],"strategy":{"type":"leastLoad"}}],`+
			`"rules":[{"type":"field","network":"tcp,udp","balancerTag":"Balancer"}]}}`,
			outbound("bridge", "a"), outbound("bridge-2", "b"))
		body := "[" + strings.Join([]string{
			pool,
			single("🇦🇹 Австрия", "a"),
			single("🇵🇱 Польша", "b"),
			single("🇩🇪 Германия", "a"),
		}, ",") + "]"

		pb, err := ParseSubscriptionBody([]byte(body), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		got := collapsedByTag(pb)
		if len(got) != 1 {
			t.Fatalf("код duplicates_collapsed на %d узлах, ждали на одном: %v", len(got), got)
		}
		for tag, v := range got {
			if !strings.Contains(tag, "Австрия") {
				t.Errorf("код на %q, ждали на узле Австрии", tag)
			}
			if !strings.HasPrefix(v, "1|") || !strings.Contains(v, "Германия") || strings.Contains(v, "bridge") {
				t.Errorf("%q: params %q — ждали одну Германию без членов пула", tag, v)
			}
		}
	})
}

// collapsedByTag — сырой тег → "count|names" у узлов с кодом.
func collapsedByTag(pb *ParsedBody) map[string]string {
	out := map[string]string{}
	for _, e := range pb.Entries {
		for _, w := range e.Node.Warnings {
			if w.Code == WarnDuplicatesCollapsed {
				out[e.RawTag] = w.Params["count"] + "|" + w.Params["names"]
			}
		}
	}
	return out
}

func assertCollapsed(t *testing.T, got, want map[string]string, entries, wantEntries int) {
	t.Helper()
	if entries != wantEntries {
		t.Errorf("узлов %d, ждали %d", entries, wantEntries)
	}
	if len(got) != len(want) {
		t.Errorf("код на %d узлах, ждали на %d: %v", len(got), len(want), got)
	}
	for tag, w := range want {
		if got[tag] != w {
			t.Errorf("%q: %q, ждали %q", tag, got[tag], w)
		}
	}
}
