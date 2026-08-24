package traffic

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
)

// TestParseLogLine_KnownSamples runs the parser over the golden log file and
// asserts kind + key field for each line. If sing-box log format changes
// between releases, this test fails fast and pinpoints the regex to fix
// (rather than letting the profiler silently emit nothing).
//
// The fixture file `testdata/sing-box-logs/sample.log` is sanitized
// (no real user data) and checked into the repo via a `.gitignore`
// exception (`!internal/traffic/testdata/sing-box-logs/*.log`). The
// surrounding `*.log` rule still protects runtime logs under `bin/logs/`.
// The os.IsNotExist branch below is a belt-and-suspenders skip so the
// test degrades to SKIP (not FAIL) if the fixture ever goes missing again.
func TestParseLogLine_KnownSamples(t *testing.T) {
	path := filepath.Join("testdata", "sing-box-logs", "sample.log")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("fixture missing at %s — sanitized golden log expected to be in repo via .gitignore exception", path)
		}
		t.Fatalf("open sample log: %v", err)
	}
	defer func() { _ = f.Close() }()

	want := []struct {
		kind        EventKind
		connID      string
		domain      string
		ip          string
		cname       string
		processPath string
		rule        string
		outbound    string
		failReason  string
		port        int
	}{
		{kind: EventDNSResolve, connID: "12345", domain: "cdn.t-bank-app.ru", ip: "193.17.93.194"},
		{kind: EventDNSResolve, connID: "12345", domain: "cdn.t-bank-app.ru", ip: "2a02:6b8::1"},
		{kind: EventDNSResolve, connID: "12346", domain: "certs.t-bank-app.ru", cname: "eq09pc7nbi.a.trbcdn.net"},
		{kind: EventDNSResolve, connID: "12346", domain: "eq09pc7nbi.a.trbcdn.net", ip: "81.222.127.186"},
		{kind: EventDNSFail, connID: "12347", domain: "certs.t-bank-app.ru", failReason: "context deadline exceeded"},
		{kind: "", connID: "12348", processPath: "/Applications/Slack.app/Contents/MacOS/Slack"},
		{kind: EventRouterMatch, connID: "12348", rule: "domain_suffix=example.com", outbound: "vpn-1"},
		{kind: "", connID: "12348", ip: "1.2.3.4", port: 443},
		{kind: EventDNSResolve, connID: "12349", domain: "api.example.com", ip: "5.6.7.8"},
		{kind: "", connID: "12349", ip: "api.example.com", port: 443},
	}

	sc := bufio.NewScanner(f)
	idx := 0
	for sc.Scan() {
		line := sc.Text()
		if idx >= len(want) {
			t.Fatalf("more log lines than expectations at line %d: %q", idx, line)
		}
		got, ok := ParseLogLine(line)
		if !ok {
			t.Errorf("line %d not parsed: %q", idx, line)
			idx++
			continue
		}
		exp := want[idx]
		if got.Kind != exp.kind {
			t.Errorf("line %d kind: want %q got %q (%q)", idx, exp.kind, got.Kind, line)
		}
		if got.ConnID != exp.connID {
			t.Errorf("line %d conn: want %q got %q", idx, exp.connID, got.ConnID)
		}
		if exp.domain != "" && got.Domain != exp.domain {
			t.Errorf("line %d domain: want %q got %q", idx, exp.domain, got.Domain)
		}
		if exp.ip != "" && got.IP != exp.ip {
			t.Errorf("line %d ip: want %q got %q", idx, exp.ip, got.IP)
		}
		if exp.cname != "" && got.CnameTarget != exp.cname {
			t.Errorf("line %d cname: want %q got %q", idx, exp.cname, got.CnameTarget)
		}
		if exp.processPath != "" && got.ProcessPath != exp.processPath {
			t.Errorf("line %d processPath: want %q got %q", idx, exp.processPath, got.ProcessPath)
		}
		if exp.rule != "" && got.Rule != exp.rule {
			t.Errorf("line %d rule: want %q got %q", idx, exp.rule, got.Rule)
		}
		if exp.outbound != "" && got.Outbound != exp.outbound {
			t.Errorf("line %d outbound: want %q got %q", idx, exp.outbound, got.Outbound)
		}
		if exp.failReason != "" && got.FailReason != exp.failReason {
			t.Errorf("line %d failReason: want %q got %q", idx, exp.failReason, got.FailReason)
		}
		if exp.port != 0 && got.Port != exp.port {
			t.Errorf("line %d port: want %d got %d", idx, exp.port, got.Port)
		}
		idx++
	}
	if idx != len(want) {
		t.Errorf("expected %d lines, parsed %d", len(want), idx)
	}
}

func TestParseLogLine_Garbage(t *testing.T) {
	cases := []string{
		"",
		"foo bar baz",
		"2026-05-24 12:34:15 INFO  [42] proxy: starting", // not a known pattern
	}
	for _, c := range cases {
		if _, ok := ParseLogLine(c); ok {
			t.Errorf("garbage parsed unexpectedly: %q", c)
		}
	}
}

func TestIsDNSTimeout(t *testing.T) {
	yes := []string{
		"context deadline exceeded",
		"i/o timeout",
		"timeout awaiting response headers",
		"DNS query timeout (5s)",
	}
	no := []string{
		"server refused",
		"no such host",
		"network unreachable",
	}
	for _, r := range yes {
		if !IsDNSTimeout(r) {
			t.Errorf("want true for %q", r)
		}
	}
	for _, r := range no {
		if IsDNSTimeout(r) {
			t.Errorf("want false for %q", r)
		}
	}
}

// Значение резолва бывает трёх видов: адрес, CNAME и rdata записи, которую мы
// не разбираем. Третий вид раньше уезжал в колонку IP целиком — признаком
// адреса считалось «есть двоеточие», а в HTTPS/SVCB-записи оно есть внутри
// ipv6hint. Chrome спрашивает такие записи постоянно, так что мусор был виден
// в каждой сессии.
func TestIPLooksLikeIPRejectsSVCB(t *testing.T) {
	svcb := `1 . alpn="h3,h2" ipv4hint="162.159.61.3,172.64.41.3" ipv6hint="2606:4700::1111"`
	if ipLooksLikeIP(svcb) {
		t.Fatal("rdata HTTPS/SVCB принята за IP")
	}
	if looksLikeHostname(svcb) {
		t.Fatal("rdata HTTPS/SVCB принята за CNAME — мусор просто переехал бы в цепочку")
	}

	for _, ip := range []string{"162.159.61.3", "2606:4700:4700::1111", "::1", "0.0.0.0"} {
		if !ipLooksLikeIP(ip) {
			t.Errorf("%q не распознан как IP", ip)
		}
	}
	for _, host := range []string{"example.com", "cdn.example.co.uk", "xn--80ak6aa92e.com", "a-b_c.test"} {
		if ipLooksLikeIP(host) {
			t.Errorf("%q принят за IP", host)
		}
		if !looksLikeHostname(host) {
			t.Errorf("%q не распознан как имя", host)
		}
	}
	// Без точки это не доменное имя: односегментные значения в CNAME не ходят.
	if looksLikeHostname("localhost") {
		t.Error("односегментное значение принято за имя")
	}
	// Старая проверка на «256» в октете должна сохраниться.
	if ipLooksLikeIP("999.1.1.1") {
		t.Error("999.1.1.1 принят за IP")
	}
}

// Метка времени: живой лаунчер пишет лог через враппер, добавляющий префикс
// смещения зоны (`+0300 2026-08-24 14:39:37 INFO …`). Раньше регулярка ждала
// дату строго в начале строки — не совпадала НИ ОДНА живая строка, и TS у всех
// событий лога оставался нулевым. Нулевая метка «старше» 3-часового окна
// сессии, поэтому событие вытеснялось следующим же добавлением, лишь увеличив
// events_dropped. Ни одна форма строки не должна давать нулевой TS.
func TestParseLogLine_TimestampNeverZero(t *testing.T) {
	const body = "INFO [1234567890 0ms] inbound/tun[tun-in]: inbound connection from 192.168.1.5:5000"
	cases := map[string]string{
		"с префиксом зоны":       "+0300 2026-08-24 15:13:27 " + body,
		"отрицательное смещение": "-0800 2026-08-24 15:13:27 " + body,
		"без префикса":           "2026-08-24 15:13:27 " + body,
		"дробные секунды":        "+0300 2026-08-24 15:13:27.123 " + body,
		"без метки вовсе":        body,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			ll, _ := ParseLogLine(line)
			if ll.TS.IsZero() {
				t.Fatal("нулевой TS: событие будет вытеснено из буфера как протухшее")
			}
		})
	}

	// Там, где метка есть, читается именно она, а не время разбора.
	ll, _ := ParseLogLine("+0300 2026-08-24 15:13:27 " + body)
	if got := ll.TS.Format("2006-01-02 15:04:05"); got != "2026-08-24 15:13:27" {
		t.Errorf("TS = %s, ожидалась метка из строки", got)
	}
}
