package build

import "testing"

// TestURLTestIntervalGetsIdleTimeout — провайдерский interval больше
// дефолтного idle_timeout ядра (30m): пара достраивается, а САМ interval
// остаётся нетронутым. Это главное свойство правила — урезание интервала
// означало бы пробить провайдера чаще, чем он просил (issue #118).
func TestURLTestIntervalGetsIdleTimeout(t *testing.T) {
	got, _ := sanitizeHelper(t, []string{
		`{"tag":"n1","type":"vless","server":"a"}`,
		`{"tag":"auto","type":"urltest","outbounds":["n1"],"interval":"3h"}`,
	})
	auto := got["auto"]
	if auto["interval"] != "3h" {
		t.Errorf("interval must be kept as the provider set it: %v", auto["interval"])
	}
	if auto["idle_timeout"] != "3h" {
		t.Errorf("idle_timeout must be added to match the interval: %v", auto["idle_timeout"])
	}
}

// TestURLTestIdleTimeoutRaised — заданы оба, но пара не сходится: вверх
// тянется idle_timeout, не вниз interval.
func TestURLTestIdleTimeoutRaised(t *testing.T) {
	got, _ := sanitizeHelper(t, []string{
		`{"tag":"n1","type":"vless","server":"a"}`,
		`{"tag":"auto","type":"urltest","outbounds":["n1"],"interval":"1h","idle_timeout":"10m"}`,
	})
	auto := got["auto"]
	if auto["interval"] != "1h" {
		t.Errorf("interval must not be shortened: %v", auto["interval"])
	}
	if auto["idle_timeout"] != "1h" {
		t.Errorf("idle_timeout must be raised to the interval: %v", auto["idle_timeout"])
	}
}

// TestURLTestValidPairUntouched — валидные записи санитайзер не трогает:
// ни короткий interval без idle_timeout (ядро подставит свои 30m), ни
// уже сходящуюся пару. Иначе правило переписывало бы каждый конфиг подряд.
func TestURLTestValidPairUntouched(t *testing.T) {
	got, _ := sanitizeHelper(t, []string{
		`{"tag":"n1","type":"vless","server":"a"}`,
		`{"tag":"short","type":"urltest","outbounds":["n1"],"interval":"5m"}`,
		`{"tag":"pair","type":"urltest","outbounds":["n1"],"interval":"10m","idle_timeout":"1h"}`,
		`{"tag":"sel","type":"selector","outbounds":["n1"],"interval":"9h"}`,
	})
	if _, ok := got["short"]["idle_timeout"]; ok {
		t.Error("interval below the core default must not gain an idle_timeout")
	}
	if got["pair"]["idle_timeout"] != "1h" {
		t.Errorf("a valid pair must survive unchanged: %v", got["pair"]["idle_timeout"])
	}
	// selector не пингует узлы и поля interval не имеет — правило его не касается.
	if _, ok := got["sel"]["idle_timeout"]; ok {
		t.Error("selector must not be touched by the urltest rule")
	}
}

// TestCoreDurationDays — ядро понимает суффикс «d», которого нет у
// time.ParseDuration: «1d» обязан читаться как сутки, иначе рабочая группа
// была бы принята за мусор и осталась без пары.
func TestCoreDurationDays(t *testing.T) {
	cases := map[string]float64{
		"1d":     24,
		"2d":     48,
		"0.5d":   12,
		"1d12h":  36,
		"90m":    1.5,
		"3h":     3,
		"1h30m":  1.5,
		"12h30m": 12.5,
	}
	for in, wantHours := range cases {
		got, err := parseCoreDuration(in)
		if err != nil {
			t.Errorf("parseCoreDuration(%q): %v", in, err)
			continue
		}
		if got.Hours() != wantHours {
			t.Errorf("parseCoreDuration(%q) = %v, want %v hours", in, got, wantHours)
		}
	}
	for _, bad := range []string{"", "3", "abc", "1x"} {
		if _, err := parseCoreDuration(bad); err == nil {
			t.Errorf("parseCoreDuration(%q) must fail", bad)
		}
	}
}

// TestURLTestDayInterval — сквозная проверка: «1d» из подписки доезжает до
// правила и получает пару, а не игнорируется как неразобранное значение.
func TestURLTestDayInterval(t *testing.T) {
	got, _ := sanitizeHelper(t, []string{
		`{"tag":"n1","type":"vless","server":"a"}`,
		`{"tag":"auto","type":"urltest","outbounds":["n1"],"interval":"1d"}`,
	})
	if got["auto"]["idle_timeout"] != "1d" {
		t.Errorf("a day-suffixed interval must be recognised: %v", got["auto"]["idle_timeout"])
	}
}

// TestURLTestBrokenIntervalLeftToCore — мусор в interval не подменяется:
// ядро отвергнет его с сообщением про сам ключ, а тихая подстановка
// спрятала бы ошибку конфига.
func TestURLTestBrokenIntervalLeftToCore(t *testing.T) {
	got, _ := sanitizeHelper(t, []string{
		`{"tag":"n1","type":"vless","server":"a"}`,
		`{"tag":"auto","type":"urltest","outbounds":["n1"],"interval":"nonsense"}`,
	})
	if got["auto"]["interval"] != "nonsense" {
		t.Errorf("a broken interval must be left as is: %v", got["auto"]["interval"])
	}
	if _, ok := got["auto"]["idle_timeout"]; ok {
		t.Error("a broken interval must not produce an idle_timeout")
	}
}
