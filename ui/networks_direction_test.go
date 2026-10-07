package ui

import (
	"strings"
	"testing"
	"time"

	"singbox-launcher/core/services"
)

// SPEC 148 §2, LxBox §579 раздел 3: каждая строка таблицы состояния.
func TestNetworksRowState(t *testing.T) {
	cases := []struct {
		name    string
		running bool
		st      services.TailscaleStatus
		ok      bool
		want    string
		tone    networksTone
	}{
		{"VPN выключен", false, services.TailscaleStatus{BackendState: "Running"}, true, "", networksToneNeutral},
		{"записи ещё нет", true, services.TailscaleStatus{}, false, "starting", networksToneNeutral},
		{"Running", true, services.TailscaleStatus{BackendState: "Running"}, true, "running", networksToneOK},
		{"NeedsLogin", true, services.TailscaleStatus{BackendState: "NeedsLogin"}, true, "sign-in needed", networksToneWarn},
		{"Stopped", true, services.TailscaleStatus{BackendState: "Stopped"}, true, "stopped", networksToneWarn},
		{"прочее — текст ядра", true, services.TailscaleStatus{BackendState: "InUseOtherUser", StateText: "In use"}, true, "In use", networksToneNeutral},
	}
	for _, c := range cases {
		got, tone := networksRowState(c.running, c.st, c.ok)
		if got != c.want || tone != c.tone {
			t.Errorf("%s: got %q/%d, want %q/%d", c.name, got, tone, c.want, c.tone)
		}
	}
}

// Направление пользователя с тегом NETWORKS не совпадает с псевдо-направлением.
func TestNetworksOptionLabel(t *testing.T) {
	if got := networksOptionLabel([]string{"proxy-out"}); got != "NETWORKS" {
		t.Errorf("без совпадения: %q", got)
	}
	got := networksOptionLabel([]string{"NETWORKS", "proxy-out"})
	if got == "NETWORKS" || strings.TrimSpace(strings.ReplaceAll(got, " ", " ")) != "NETWORKS" {
		t.Errorf("при совпадении пункт обязан отличаться строкой, но читаться так же: %q", got)
	}
	rows := networksRows([]string{"ts-a", "ts-b"})
	if len(rows) != 2 || rows[0].Name != "ts-a" || rows[1].Name != "ts-b" {
		t.Errorf("строки: %+v", rows)
	}
}

// LxBox §581 раздел 2: что вкладка показывает без данных.
func TestTailscaleNetworkViewFor(t *testing.T) {
	if v := tailscaleNetworkViewFor(false, true, true); v != tsViewVPNOff {
		t.Errorf("VPN выключен: %d", v)
	}
	if v := tailscaleNetworkViewFor(true, false, false); v != tsViewWaiting {
		t.Errorf("данных ещё нет: %d", v)
	}
	if v := tailscaleNetworkViewFor(true, true, false); v != tsViewNotRunning {
		t.Errorf("узла нет в работающем конфиге: %d", v)
	}
	if v := tailscaleNetworkViewFor(true, true, true); v != tsViewStatus {
		t.Errorf("состояние есть: %d", v)
	}
}

// Три текста у знака предупреждения блока Exit node; при совпадении текста нет.
func TestExitNodeWarningText(t *testing.T) {
	if exitNodeWarningText(services.ExitNodeSame) != "" {
		t.Error("совпадают — текста нет")
	}
	want := map[services.ExitNodeDiff]string{
		services.ExitNodeSelectedNotSaved: "Traffic is not routed through this node",
		services.ExitNodeClearedNotSaved:  "has no exit until you save",
		services.ExitNodeOtherNotSaved:    "lost after restart",
	}
	for d, frag := range want {
		if got := exitNodeWarningText(d); !strings.Contains(got, frag) {
			t.Errorf("%d: %q", d, got)
		}
	}
}

// Отметки строки устройства (§6).
func TestTailscaleDeviceDetails(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	p := services.TailscalePeer{
		HostName: "nas", DNSName: "nas.tail.ts.net.", TailscaleIPs: []string{"100.64.0.7"}, OS: "linux",
		LastSeen: now.Add(-2 * time.Hour), Expired: true, ShareeNode: true, ExitNodeOption: true,
	}
	got := tailscaleDeviceDetails(p, now)
	for _, frag := range []string{"linux", "last seen", "key expired", "shared", "exit node"} {
		if !strings.Contains(got, frag) {
			t.Errorf("нет %q в %q", frag, got)
		}
	}
	p.Online = true
	if got := tailscaleDeviceDetails(p, now); strings.Contains(got, "last seen") {
		t.Errorf("в сети: %q", got)
	}
}
