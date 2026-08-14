package services

import (
	"testing"

	daemonpb "singbox-launcher/internal/daemonpb"
)

// Ответы приходят в wire order; CNAME-цепочка восстанавливается именно из
// него, поэтому порядок внутри CNAMEs обязан сохраняться, а A/AAAA — уезжать
// в Answers, не перемешиваясь с цепочкой.
func TestDNSQueryFromProtoSplitsCNAMEChain(t *testing.T) {
	ev := &daemonpb.DnsQueryEvent{
		Domain:    "www.example.com",
		DnsServer: "1.1.1.1",
		ProcessInfo: &daemonpb.ProcessInfo{
			ProcessPath: "/Applications/Safari.app/Contents/MacOS/Safari",
		},
		Answers: []*daemonpb.DnsAnswer{
			{Type: dnsTypeCNAME, Rdata: "edge.example.net"},
			{Type: dnsTypeCNAME, Rdata: "cdn.example.org"},
			{Type: 1, Rdata: "93.184.216.34"},
			{Type: 1, Rdata: ""}, // пустая rdata — разрыв в цепочке, пропускаем
		},
	}
	q := DNSQueryFromProto(ev)

	if q.Domain != "www.example.com" || q.DNSServer != "1.1.1.1" {
		t.Fatalf("домен/сервер потеряны: %+v", q)
	}
	if q.ProcessPath == "" {
		t.Fatal("ProcessInfo не разложен — этого поля в логе нет вовсе")
	}
	want := []string{"edge.example.net", "cdn.example.org"}
	if len(q.CNAMEs) != len(want) {
		t.Fatalf("CNAMEs = %v, want %v", q.CNAMEs, want)
	}
	for i := range want {
		if q.CNAMEs[i] != want[i] {
			t.Fatalf("порядок цепочки нарушен: %v, want %v", q.CNAMEs, want)
		}
	}
	if len(q.Answers) != 1 || q.Answers[0] != "93.184.216.34" {
		t.Fatalf("Answers = %v", q.Answers)
	}
}

func TestDNSQueryFromProtoFailure(t *testing.T) {
	q := DNSQueryFromProto(&daemonpb.DnsQueryEvent{
		Domain: "broken.test",
		Failed: true,
		Error:  "timeout",
	})
	if !q.Failed || q.Error != "timeout" {
		t.Fatalf("отказ не разложен: %+v", q)
	}
	// nil не должен ронять: стрим может отдать пустое событие.
	if got := DNSQueryFromProto(nil); got.Domain != "" {
		t.Fatalf("nil-событие: %+v", got)
	}
}
