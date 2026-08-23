package api

import "testing"

// TestPingTestTimeoutNormalize — настройка приходит из settings.json, который
// правят руками: мусор обязан давать рабочее значение, а не ноль (мгновенный
// таймаут превратил бы весь список в ошибки).
func TestPingTestTimeoutNormalize(t *testing.T) {
	t.Cleanup(func() { SetPingTestTimeoutMs(DefaultPingTestTimeoutMs) })

	cases := []struct{ in, want int }{
		{0, DefaultPingTestTimeoutMs},
		{-1, DefaultPingTestTimeoutMs},
		{50, MinPingTestTimeoutMs},
		{15000, 15000},
		{9999999, MaxPingTestTimeoutMs},
	}
	for _, c := range cases {
		SetPingTestTimeoutMs(c.in)
		if got := GetPingTestTimeoutMs(); got != c.want {
			t.Errorf("SetPingTestTimeoutMs(%d): want %d, got %d", c.in, c.want, got)
		}
	}
}
