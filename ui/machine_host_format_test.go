package ui

import (
	"strings"
	"testing"

	"singbox-launcher/internal/lxdclient"
)

// Тесты форматирования телеметрии хоста.
//
// Главное, что здесь проверяется, — граница «нет данных» против «ноль».
// Первый ответ демона отдаёт проценты и скорости как null, и подмена их
// нулём соврала бы ровно в тот момент, когда на экран смотрят ради диагноза.

func f64(v float64) *float64 { return &v }
func iptr(v int) *int        { return &v }
func u64(v uint64) *uint64   { return &v }

func TestHostPercentNilIsDash(t *testing.T) {
	if got := hostPercent(nil); got != hostDash {
		t.Fatalf("hostPercent(nil) = %q, хотели прочерк %q", got, hostDash)
	}
	if got := hostPercent(f64(43.9)); got != "43.9%" {
		t.Fatalf("hostPercent(43.9) = %q", got)
	}
	// Ноль — это утверждение «простаивает», а не отсутствие данных.
	if got := hostPercent(f64(0)); got != "0.0%" {
		t.Fatalf("hostPercent(0) = %q, ноль обязан печататься как ноль", got)
	}
}

func TestHostBarValueClamps(t *testing.T) {
	cases := []struct {
		name string
		in   *float64
		want float64
	}{
		{"nil даёт пустую полоску", nil, 0},
		{"обычное значение", f64(50), 0.5},
		{"отрицательное подрезается", f64(-5), 0},
		{"больше ста подрезается", f64(140), 1},
		{"ровно сто", f64(100), 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hostBarValue(c.in); got != c.want {
				t.Fatalf("hostBarValue = %v, хотели %v", got, c.want)
			}
		})
	}
}

func TestHostCPUSummaryFirstSample(t *testing.T) {
	// Первый ответ демона: процентов ещё нет — дельту не с чем считать.
	first := lxdclient.HostCPU{Cores: 2, UsagePercent: nil, IntervalSeconds: 0}
	got := hostCPUSummary(first)
	if strings.Contains(got, "0") {
		t.Fatalf("hostCPUSummary на первом замере = %q: ноль читается как «простаивает»", got)
	}
	if got == hostDash || got == "" {
		t.Fatalf("hostCPUSummary на первом замере = %q: нужно сказать, чего ждём", got)
	}

	second := lxdclient.HostCPU{Cores: 2, UsagePercent: f64(12.4), IntervalSeconds: 2}
	if got := hostCPUSummary(second); got != "12.4%" {
		t.Fatalf("hostCPUSummary на втором замере = %q", got)
	}
}

func TestHostLoadAvailableOnFirstSample(t *testing.T) {
	// load average приходит от ядра готовым: он есть уже в первом ответе,
	// когда процентов ещё нет.
	c := lxdclient.HostCPU{UsagePercent: nil, Load1: f64(0.04), Load5: f64(0.1), Load15: f64(0.08)}
	got := hostLoadText(c)
	if got == hostDash {
		t.Fatal("hostLoadText: load есть в первом ответе, прочерк тут неверен")
	}
	for _, want := range []string{"0.04", "0.10", "0.08"} {
		if !strings.Contains(got, want) {
			t.Fatalf("hostLoadText = %q, нет %q", got, want)
		}
	}
	if got := hostLoadText(lxdclient.HostCPU{}); got != hostDash {
		t.Fatalf("hostLoadText без данных = %q, хотели прочерк", got)
	}
}

func TestHostThermalNilIsNotZero(t *testing.T) {
	// Нет датчиков — это не «0 °C».
	if got := hostThermalText(nil); got != hostDash {
		t.Fatalf("hostThermalText(nil) = %q, хотели прочерк", got)
	}
	th := &lxdclient.HostThermal{
		Zones:      []lxdclient.HostThermalZone{{Name: "cpu-thermal", Celsius: 63.9}},
		MaxCelsius: 63.9,
	}
	if got := hostThermalText(th); got != "63.9 °C" {
		t.Fatalf("hostThermalText = %q", got)
	}
}

func TestHostFDTextRequiresBoth(t *testing.T) {
	if got := hostFDText(iptr(18), nil); got != hostDash {
		t.Fatalf("hostFDText без лимита = %q: «18 / —» не говорит о близости к потолку", got)
	}
	if got := hostFDText(nil, iptr(4095)); got != hostDash {
		t.Fatalf("hostFDText без open = %q", got)
	}
	if got := hostFDText(iptr(18), iptr(4095)); got != "18 / 4095" {
		t.Fatalf("hostFDText = %q", got)
	}
	if got := hostFDRatio(iptr(18), iptr(0)); got != 0 {
		t.Fatalf("hostFDRatio при нулевом лимите = %v, деления на ноль быть не должно", got)
	}
}

func TestHostMountFlags(t *testing.T) {
	cases := []struct {
		name string
		in   lxdclient.HostMount
		want string
	}{
		// squashfs-корень на OpenWrt вечно 100% — помечаем, иначе строка
		// читается как незамеченная авария.
		{"read-only", lxdclient.HostMount{ReadOnly: true}, "ro"},
		{"state-dir", lxdclient.HostMount{HoldsStateDir: true}, "state"},
		{"обычная", lxdclient.HostMount{}, ""},
		{"оба флага", lxdclient.HostMount{ReadOnly: true, HoldsStateDir: true}, "ro · state"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hostMountFlags(c.in); got != c.want {
				t.Fatalf("hostMountFlags = %q, хотели %q", got, c.want)
			}
		})
	}
}

func TestHostIfaceErrorsQuietWhenClean(t *testing.T) {
	// Ноль ошибок — норма; печатать его в каждой строке значит приучить глаз
	// пролистывать колонку, в которой однажды появится ненулевое число.
	if got := hostIfaceErrors(lxdclient.HostInterface{}); got != "" {
		t.Fatalf("hostIfaceErrors на чистом интерфейсе = %q, хотели пусто", got)
	}
	// Реальные цифры eth1 роутера: дропы без единой ошибки.
	dirty := lxdclient.HostInterface{RxErrors: 120, RxDropped: 51870}
	got := hostIfaceErrors(dirty)
	if !strings.Contains(got, "120") || !strings.Contains(got, "51870") {
		t.Fatalf("hostIfaceErrors = %q, ждали обе цифры", got)
	}
}

func TestHostIfaceRatesFirstSample(t *testing.T) {
	// До второго замера скоростей нет — в обеих колонках прочерк.
	first := lxdclient.HostInterface{RxBytes: 100, TxBytes: 200}
	if got := hostIfaceRxRate(first); got != hostDash {
		t.Fatalf("hostIfaceRxRate на первом замере = %q, хотели прочерк", got)
	}
	if got := hostIfaceTxRate(first); got != hostDash {
		t.Fatalf("hostIfaceTxRate на первом замере = %q, хотели прочерк", got)
	}
	// Счётчики при этом уже есть: именно они переживают разрывы.
	live := lxdclient.HostInterface{RxBytes: 24059590498, TxBytes: 24263091456}
	if got := hostIfaceRxTotal(live); strings.Contains(got, hostDash) {
		t.Fatalf("hostIfaceRxTotal = %q: счётчики доступны сразу", got)
	}
	if got := hostIfaceTxTotal(live); strings.Contains(got, hostDash) {
		t.Fatalf("hostIfaceTxTotal = %q: счётчики доступны сразу", got)
	}
}

// Значения ячеек не несут стрелок: направление задаёт колонка, а пробел после
// ↑/↓ в этом шрифте рвёт отрисовку.
func TestHostIfaceCellsCarryNoArrows(t *testing.T) {
	i := lxdclient.HostInterface{RxBytes: 1 << 20, TxBytes: 1 << 21, MTU: 1500}
	for name, got := range map[string]string{
		"rx_total": hostIfaceRxTotal(i),
		"tx_total": hostIfaceTxTotal(i),
		"rx_rate":  hostIfaceRxRate(i),
		"tx_rate":  hostIfaceTxRate(i),
		"mtu":      hostIfaceMTU(i),
	} {
		if strings.ContainsAny(got, "↑↓") {
			t.Fatalf("%s = %q: стрелка живёт в шапке колонки, не в ячейке", name, got)
		}
	}
	if got := hostIfaceMTU(lxdclient.HostInterface{}); got != hostDash {
		t.Fatalf("hostIfaceMTU без MTU = %q: нулевого MTU не бывает", got)
	}
}

func TestHostSwapDisabled(t *testing.T) {
	// На роутере swap обычно не настроен: «0 B of 0 B» заставляет гадать.
	off := hostSwapText(lxdclient.HostMemory{})
	if !strings.Contains(off, "disabled") {
		t.Fatalf("hostSwapText без swap = %q", off)
	}
	m := lxdclient.HostMemory{SwapTotalBytes: 1 << 30, SwapFreeBytes: 1 << 29}
	on := hostSwapText(m)
	if strings.Contains(on, "disabled") {
		t.Fatalf("hostSwapText со swap = %q", on)
	}
	// 512 МБ занято из гигабайта — обе цифры на месте.
	if !strings.Contains(on, "512.0 MB") || !strings.Contains(on, "1.00 GB") {
		t.Fatalf("hostSwapText со swap = %q", on)
	}
}

func TestHostMemoryDetailUsesAvailable(t *testing.T) {
	// Цифры живого роутера: free 155 МБ, но available 272 МБ. «Занято»
	// обязано считаться от available, иначе кричит «занято» при свободных
	// сотнях мегабайт, лежащих в page cache.
	m := lxdclient.HostMemory{
		TotalBytes:     509353984,
		AvailableBytes: 285843456,
		FreeBytes:      163196928,
		CachedBytes:    u64(176496640),
		UsedPercent:    f64(43.9),
	}
	got := hostMemoryDetail(m)
	// used = total - available = 223510528 ≈ 213.2 МБ. От free вышло бы
	// 346 МБ — заметно другое число.
	if !strings.Contains(got, "213") {
		t.Fatalf("hostMemoryDetail = %q: занятое считается от available", got)
	}
	if !strings.Contains(got, "cache") {
		t.Fatalf("hostMemoryDetail = %q: кеш объясняет разницу free/available", got)
	}
}

// Обратный порядок полей не должен уводить вычитание uint64 в переполнение:
// «занято» тогда стало бы числом в эксабайтах вместо нуля.
func TestHostMemoryDetailNoUnderflow(t *testing.T) {
	m := lxdclient.HostMemory{TotalBytes: 100, AvailableBytes: 200}
	if got := hostMemoryDetail(m); !strings.Contains(got, "0 B used") {
		t.Fatalf("hostMemoryDetail при available > total = %q", got)
	}
	s := lxdclient.HostMemory{SwapTotalBytes: 100, SwapFreeBytes: 200}
	if got := hostSwapText(s); !strings.Contains(got, "0 B of") {
		t.Fatalf("hostSwapText при free > total = %q", got)
	}
}

func TestHostUptime(t *testing.T) {
	if got := hostUptime(0); got != hostDash {
		t.Fatalf("hostUptime(0) = %q", got)
	}
	// 121092 с с живого роутера = 1 сутки 09:38.
	if got := hostUptime(121092); got != "1d 09:38" {
		t.Fatalf("hostUptime(121092) = %q", got)
	}
	if got := hostUptime(3720); got != "01:02" {
		t.Fatalf("hostUptime(3720) = %q", got)
	}
}

func TestHostIntervalEmptyWhenNoSample(t *testing.T) {
	if got := hostInterval(0); got != "" {
		t.Fatalf("hostInterval(0) = %q: окна ещё нет", got)
	}
	if got := hostInterval(2); !strings.Contains(got, "2.0") {
		t.Fatalf("hostInterval(2) = %q", got)
	}
}

func TestHostBytes(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{512, "512 B"},
		{2048, "2 KB"},
		{5 << 20, "5.0 MB"},
		{3860496384, "3.60 GB"}, // /overlay живого роутера
	}
	for _, c := range cases {
		if got := hostBytes(c.in); got != c.want {
			t.Fatalf("hostBytes(%d) = %q, хотели %q", c.in, got, c.want)
		}
	}
}

func TestHostMachineLineSkipsEmpty(t *testing.T) {
	// Демон вправе не знать модель — пустое поле не должно оставлять висящий
	// разделитель.
	h := lxdclient.HostInfo{OS: "RouteRich 24.10.5", Kernel: "6.6.119", Arch: "arm64", UptimeSeconds: 121092}
	got := hostMachineLine(h)
	if strings.HasPrefix(got, " · ") || strings.Contains(got, " ·  · ") {
		t.Fatalf("hostMachineLine = %q: пустое поле оставило разделитель", got)
	}
	if !strings.Contains(got, "1d 09:38") {
		t.Fatalf("hostMachineLine = %q: нет аптайма", got)
	}
	// Стрелка тут не нужна: «↑ » с пробелом в этом шрифте даёт тофу.
	if strings.ContainsAny(got, "↑↓") {
		t.Fatalf("hostMachineLine = %q: стрелке в шапке не место", got)
	}
}
