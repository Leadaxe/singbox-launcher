//go:build darwin || (windows && !386)

package core

import (
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/lxdclient"
)

// TestDaemonServiceProcessVerdict — третий шаг классификатора SPEC 136 §4
// поверх файлового вердикта: паспорт работающего демона (lx.11 и старое ядро
// без полей). Шаг сравнивает строки и на диск не ходит — пути фиктивные.
func TestDaemonServiceProcessVerdict(t *testing.T) {
	corePath := filepath.Join("service", "sing-box-lxd")
	launcherCore := filepath.Join("data", "bin", "sing-box")
	copySHA := strings.Repeat("ab", 32)
	c := DaemonServiceCheck{State: DaemonServiceOK, ServicePath: corePath, CopySHA256: copySHA, LauncherSHA256: copySHA}
	expect := func(t *testing.T, got DaemonServiceCheck, want DaemonServiceState) {
		t.Helper()
		if got.State != want {
			t.Fatalf("state = %s, want %s (detail: %s)", got.State, want, got.Detail)
		}
	}

	// Процесс: паспорт демона lx.11 (executable, executable_sha256).
	process := func(info lxdclient.InfoData, launcherVersion string) DaemonServiceCheck {
		pc := c
		pc.LauncherVersion = launcherVersion
		compareDaemonServiceProcess(&pc, info, corePath)
		return pc
	}
	expect(t, process(lxdclient.InfoData{Executable: corePath, ExecutableSHA256: c.CopySHA256}, ""), DaemonServiceOK)
	expect(t, process(lxdclient.InfoData{Executable: corePath, ExecutableSHA256: "00" + c.CopySHA256[2:]}, ""), DaemonServiceProcessStale)
	expect(t, process(lxdclient.InfoData{Executable: launcherCore, ExecutableSHA256: c.CopySHA256}, ""), DaemonServiceProcessStale)
	// lx.11 сразу после старта: executable есть, executable_sha256 ещё
	// считается в фоне и пуст — «неизвестно», не ProcessStale; судит версия.
	expect(t, process(lxdclient.InfoData{Executable: corePath, Version: "1.14.1-lx.11"}, "1.14.1-lx.11"), DaemonServiceOK)
	expect(t, process(lxdclient.InfoData{Executable: corePath, Version: "unknown"}, "1.14.1-lx.11"), DaemonServiceOK)
	expect(t, process(lxdclient.InfoData{Executable: corePath, Version: "1.14.1-lx.10"}, "1.14.1-lx.11"), DaemonServiceProcessStale)
	// Старое ядро (lx.8/lx.10): полей нет — судит версия; dev-сборка не судит.
	expect(t, process(lxdclient.InfoData{Version: "1.14.1-lx.10"}, "1.14.1-lx.11"), DaemonServiceProcessStale)
	expect(t, process(lxdclient.InfoData{Version: "1.14.1-lx.11"}, "1.14.1-lx.11"), DaemonServiceOK)
	expect(t, process(lxdclient.InfoData{Version: "unknown"}, "1.14.1-lx.11"), DaemonServiceOK)
	expect(t, process(lxdclient.InfoData{}, "1.14.1-lx.11"), DaemonServiceOK)

	// Вердикт по файлу сильнее вердикта по процессу.
	stale := c
	stale.State = DaemonServiceStale
	compareDaemonServiceProcess(&stale, lxdclient.InfoData{ExecutableSHA256: "ff"}, corePath)
	expect(t, stale, DaemonServiceStale)

	// Процесс не перебивает NotRunning (демон, запущенный руками, — не служба).
	nr := c
	nr.State = DaemonServiceNotRunning
	compareDaemonServiceProcess(&nr, lxdclient.InfoData{Executable: corePath, ExecutableSHA256: "ff"}, corePath)
	expect(t, nr, DaemonServiceNotRunning)
}

// TestServiceCoreVersionGate — сравнение версий ядра форка и граница
// «ядро умеет root-owned копию» (minCoreForRootOwnedService = lx.12):
// номер lx числом, пре-релиз ниже релиза той же базы (но rc порогового
// lx.12 гейт проходит), неразборчивая версия — не умеет.
func TestServiceCoreVersionGate(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"1.14.1-lx.12-rc1", "1.14.1-lx.8", 1},
		{"1.14.1-lx.10", "1.14.1-lx.9", 1},
		{"1.14.1-lx.12-rc1", "1.14.1-lx.12", -1},
		{"1.14.1-lx.12-rc.1", "1.14.1-lx.12-rc.2", -1},
		{"1.14.1-lx.11", "1.14.1-lx.11", 0},
		{"v1.14.1-lx.11", "1.14.1-lx.11", 0},
		{"1.15.0-lx.1", "1.14.1-lx.40", 1},
		{"1.14.0-lx.40", "1.14.1-lx.1", -1},
	} {
		a, okA := parseCoreBuild(tc.a)
		b, okB := parseCoreBuild(tc.b)
		if !okA || !okB {
			t.Fatalf("parse %q=%v, %q=%v", tc.a, okA, tc.b, okB)
		}
		if got := compareCoreBuilds(a, b); got != tc.want {
			t.Fatalf("compare(%s, %s) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
		if got := compareCoreBuilds(b, a); got != -tc.want {
			t.Fatalf("compare(%s, %s) = %d, want %d", tc.b, tc.a, got, -tc.want)
		}
	}
	for version, want := range map[string]bool{
		"1.14.1-lx.8":       false,
		"1.14.1-lx.10":      false,
		"1.14.1-lx.11-rc1":  false,
		"1.14.1-lx.11":      false, // ранняя раскладка — Unsafe (legacy)
		"1.14.1-lx.12-rc1":  true,
		"1.14.1-lx.12-rc.2": true,
		"1.14.1-lx.12":      true,
		"v1.14.1-lx.12":     true,
		"1.14.0-lx.40":      false,
		"1.15.0-lx.1":       true,
		"1.14.1":            false, // апстрим: lxd нет вовсе
		"unknown":           false, // dev-сборка
		"unnamed-dev":       false,
		"v-local-test":      false,
		"1.14.1-lx.":        false,
		"1.14.1-lx.12x":     false,
		"":                  false,
	} {
		if got := coreSupportsRootOwnedCopy(version); got != want {
			t.Fatalf("coreSupportsRootOwnedCopy(%q) = %v, want %v", version, got, want)
		}
	}
	if !coreSupportsRootOwnedCopy(constants.RequiredCoreVersion) {
		t.Fatalf("the pinned core %s must support the root-owned copy", constants.RequiredCoreVersion)
	}
}
