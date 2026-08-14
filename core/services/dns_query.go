package services

import (
	daemonpb "singbox-launcher/internal/daemonpb"
)

// dnsTypeCNAME — код записи CNAME в DNS (RFC 1035).
const dnsTypeCNAME = 5

// DNSQuery — одно событие DNS-плоскости ядра (SPEC 018 форка).
//
// Структурный поток вместо разбора лога: приходит и сервер, который отвечал, и
// цепочка переадресаций, и признак отказа — то, что из текстовой строки лога
// пришлось бы вытаскивать регулярками. Форма одна на оба источника: локальное
// ядро в daemon-режиме и удалённая машина отдают один и тот же DnsQueryEvent.
type DNSQuery struct {
	Domain  string
	Failed  bool
	Error   string
	Answers []string
	// CNAMEs — цепочка переадресаций, собранная из ответов типа CNAME.
	CNAMEs      []string
	DNSServer   string
	ProcessPath string
}

// DNSQueryFromProto раскладывает gRPC-событие в DNSQuery.
//
// Ответы приходят в wire order (CNAME hops, затем A/AAAA), и порядок здесь
// сохраняется: цепочка переадресаций читается только целиком и по порядку.
// Пустые rdata пропускаем — в цепочке они не значат ничего, а в UI выглядели
// бы разрывом.
func DNSQueryFromProto(ev *daemonpb.DnsQueryEvent) DNSQuery {
	if ev == nil {
		return DNSQuery{}
	}
	q := DNSQuery{
		Domain:    ev.GetDomain(),
		Failed:    ev.GetFailed(),
		Error:     ev.GetError(),
		DNSServer: ev.GetDnsServer(),
	}
	if pi := ev.GetProcessInfo(); pi != nil {
		q.ProcessPath = pi.GetProcessPath()
	}
	for _, a := range ev.GetAnswers() {
		rdata := a.GetRdata()
		if rdata == "" {
			continue
		}
		// Type 5 = CNAME: из этих записей клиент и восстанавливает цепочку
		// переадресаций, отдельной сущностью она не приходит.
		if a.GetType() == dnsTypeCNAME {
			q.CNAMEs = append(q.CNAMEs, rdata)
			continue
		}
		q.Answers = append(q.Answers, rdata)
	}
	return q
}
