package platform

import "regexp"

// LanDenialVerdict — итог диагностики «мгновенного no route to host» к адресу
// локальной сети (см. DiagnoseLanDenial в lan_denial_darwin.go).
type LanDenialVerdict int

const (
	// LanDenialNotApplicable — сигнатура не подтвердилась: адрес не из
	// локальной подсети, ошибка другого рода или отказ не мгновенный.
	// Показ общего «машина не отвечает» остаётся честным.
	LanDenialNotApplicable LanDenialVerdict = iota
	// LanDenialSuspected — система отклоняет connect() мгновенно, ещё до
	// выхода пакета в сеть: так выглядит блокировка macOS «Локальная сеть».
	LanDenialSuspected
	// LanDenialRecovered — контрольное соединение прошло: отказ был
	// кратковременным (политика перепроверилась или сеть моргнула),
	// пользователю достаточно повторить Connect.
	LanDenialRecovered
)

// String — стабильные английские метки для логов; локализованные тексты для
// пользователя живут в ui.
func (v LanDenialVerdict) String() string {
	switch v {
	case LanDenialSuspected:
		return "local-network denial suspected"
	case LanDenialRecovered:
		return "recovered"
	}
	return "not applicable"
}

// dialErrHostPort вылавливает host:port из текста dial-ошибки Go/gRPC:
// «dial tcp 192.168.10.1:19091: connect: no route to host». Только IPv4 —
// машины admin-плоскости адресуются v4-адресами LAN.
var dialErrHostPort = regexp.MustCompile(`dial tcp (\d{1,3}(?:\.\d{1,3}){3}:\d+)`)

// HostPortFromDialErr возвращает адрес назначения из текста dial-ошибки или
// пустую строку, если текст не похож на dial-ошибку.
func HostPortFromDialErr(msg string) string {
	m := dialErrHostPort.FindStringSubmatch(msg)
	if m == nil {
		return ""
	}
	return m[1]
}
