package ui

import (
	"testing"

	"singbox-launcher/core/services"
)

// Маркер — единственное, что сообщает о падении удалённой машины, и вся его
// правда сходится в markerFor. Тест держит именно её: границу «моргнуло» и
// «легло», и то, что ответ с плохой новостью не красится зелёным.
func TestMarkerFor(t *testing.T) {
	live := func(fails int) machineLiveness { return machineLiveness{FailStreak: fails} }

	cases := []struct {
		name      string
		connected bool
		health    services.RemoteHealth
		liveness  machineLiveness
		want      markerState
	}{
		{
			name:      "не подключались — серый",
			connected: false,
			health:    services.RemoteHealth{CoreStatus: "started"},
			want:      markerIdle,
		},
		{
			name:      "отвечает, ядро работает — зелёный",
			connected: true,
			health:    services.RemoteHealth{CoreStatus: "started"},
			want:      markerLive,
		},
		{
			name:      "ядро остановлено, но машина отвечает — зелёный (это про канал)",
			connected: true,
			health:    services.RemoteHealth{CoreStatus: "idle"},
			want:      markerLive,
		},
		{
			name:      "один промах — жёлтый, вердикт не вынесен",
			connected: true,
			health:    services.RemoteHealth{CoreStatus: "started"},
			liveness:  live(1),
			want:      markerFlaky,
		},
		{
			name:      "два промаха подряд — красный",
			connected: true,
			health:    services.RemoteHealth{CoreStatus: "started"},
			liveness:  live(heartbeatFailThreshold),
			want:      markerDown,
		},
		{
			name:      "отвечает, но ядро упало — красный, а не зелёный",
			connected: true,
			health:    services.RemoteHealth{CoreStatus: "fatal"},
			want:      markerDown,
		},
		{
			name:      "последний опрос вернул ошибку — красный",
			connected: true,
			health:    services.RemoteHealth{Err: "connection refused"},
			want:      markerDown,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := markerFor(c.connected, c.health, c.liveness); got != c.want {
				t.Errorf("markerFor() = %v, ожидалось %v", got, c.want)
			}
		})
	}
}
