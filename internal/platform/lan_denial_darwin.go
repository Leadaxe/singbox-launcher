//go:build darwin
// +build darwin

package platform

import (
	"errors"
	"net"
	"syscall"
	"time"
)

const (
	lanProbeTimeout = 3 * time.Second
	// lanInstantFail — порог «отказ мгновенный». Честный сетевой отказ до
	// on-link адреса так быстро не приходит: выключенная машина молчит до
	// таймаута ARP (секунды), а EHOSTUNREACH за миллисекунды возвращает сама
	// система, не отправив ни одного пакета.
	lanInstantFail = 150 * time.Millisecond
)

// DiagnoseLanDenial проверяет сигнатуру блокировки macOS «Локальная сеть».
//
// macOS применяет разрешение Local Network per-app по подписи приложения.
// Когда печать бандла сломана (типично: dev-бинарь, подложенный в
// установленный .app), nehelper не может атрибутировать процесс — в
// system.log это видно как «Failed to get the signing identifier for <pid>»
// (com.apple.networkextension) — и connect() к LAN-адресу отклоняется
// мгновенным EHOSTUNREACH ещё в ядре ОС, до TUN и до правил sing-box.
// Наблюдалось 2026-08-25: лаунчер получал «no route to host» к
// 192.168.10.1:19091, пока curl из соседнего процесса отвечал за 15 мс.
//
// Диагноз по трём признакам: адрес приватный и on-link (в подсети одного из
// физических интерфейсов), контрольный dial падает EHOSTUNREACH, падение
// мгновенное. Политика флапает при перепроверках, поэтому успешный
// контрольный dial — тоже вердикт (Recovered).
//
// Сетевой вызов с таймаутом lanProbeTimeout — не звать на UI-потоке.
func DiagnoseLanDenial(hostport string) LanDenialVerdict {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return LanDenialNotApplicable
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsPrivate() || !onLink(ip) {
		return LanDenialNotApplicable
	}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", hostport, lanProbeTimeout)
	if err == nil {
		conn.Close()
		return LanDenialRecovered
	}
	if errors.Is(err, syscall.EHOSTUNREACH) && time.Since(start) < lanInstantFail {
		return LanDenialSuspected
	}
	return LanDenialNotApplicable
}

// onLink — адрес лежит в подсети одного из поднятых интерфейсов.
// Loopback и point-to-point (TUN) пропускаем: их подсети — не LAN, и
// EHOSTUNREACH внутри туннеля — не про «Локальную сеть».
func onLink(ip net.IP) bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 ||
			iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.Contains(ip) {
				return true
			}
		}
	}
	return false
}
