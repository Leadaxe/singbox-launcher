package services

import (
	"testing"
	"time"

	daemonpb "singbox-launcher/internal/daemonpb"
)

// Разбор полного состояния для вкладки Network: текст ядра, отметка shared,
// StableID, группы (SPEC 148 §6, LxBox §581 «Верификация»).
func TestTailscaleStatusFromPBNetworkFields(t *testing.T) {
	upd := &daemonpb.TailscaleStatusUpdate{Endpoints: []*daemonpb.TailscaleEndpointStatus{{
		EndpointTag:  "ts",
		BackendState: "InUseOtherUser",
		StateText:    "In use by another user",
		UserGroups: []*daemonpb.TailscaleUserGroup{
			{LoginName: "a@x", DisplayName: "A", Peers: []*daemonpb.TailscalePeer{
				{HostName: "nas", ShareeNode: true, StableID: "n1", DnsName: "nas.tail.ts.net.", KeyExpiry: 1_900_000_000},
			}},
			{LoginName: "b@x", Peers: []*daemonpb.TailscalePeer{{HostName: "pc"}}},
		},
	}}}
	st := TailscaleStatusesFromPB(upd, time.Unix(1, 0))["ts"]
	if st.StateText != "In use by another user" {
		t.Errorf("StateText = %q", st.StateText)
	}
	if len(st.UserGroups) != 2 || st.UserGroups[0].DisplayName != "A" || st.UserGroups[1].LoginName != "b@x" {
		t.Fatalf("группы: %+v", st.UserGroups)
	}
	nas := st.UserGroups[0].Peers[0]
	if !nas.ShareeNode || nas.StableID != "n1" || nas.DNSName != "nas.tail.ts.net." || nas.KeyExpiry.Unix() != 1_900_000_000 {
		t.Errorf("поля устройства: %+v", nas)
	}
}

// Четыре строки таблицы поведения блока Exit node.
func TestCompareExitNode(t *testing.T) {
	router := &TailscalePeer{HostName: "router", DNSName: "router.tail.ts.net.", TailscaleIPs: []string{"100.64.0.1", "fd7a::1"}}
	cases := []struct {
		name    string
		written string
		eff     *TailscalePeer
		want    ExitNodeDiff
	}{
		{"оба пусты", "", nil, ExitNodeSame},
		{"совпадают по IP", "100.64.0.1", router, ExitNodeSame},
		{"совпадают по имени", "Router", router, ExitNodeSame},
		{"совпадают по DNS-имени", "router.tail.ts.net", router, ExitNodeSame},
		{"в узле нет, на ходу выбран", "", router, ExitNodeSelectedNotSaved},
		{"в узле есть, на ходу снят", "100.64.0.1", nil, ExitNodeClearedNotSaved},
		{"записан другой", "100.64.0.9", router, ExitNodeOtherNotSaved},
	}
	for _, c := range cases {
		if got := CompareExitNode(c.written, c.eff); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestExitNodeValueFor(t *testing.T) {
	if v := ExitNodeValueFor(TailscalePeer{HostName: "r", TailscaleIPs: []string{"100.64.0.1"}}); v != "100.64.0.1" {
		t.Errorf("с адресом: %q", v)
	}
	if v := ExitNodeValueFor(TailscalePeer{HostName: "r"}); v != "r" {
		t.Errorf("без адреса: %q", v)
	}
}

// Порядок блока Devices и список выходов.
func TestSortDevicesAndExitOptions(t *testing.T) {
	peers := []TailscalePeer{
		{HostName: "zeta", Online: false},
		{HostName: "beta", Online: true, ExitNodeOption: true},
		{HostName: "Alpha", Online: false, ExitNodeOption: true},
		{HostName: "alef", Online: true},
	}
	got := SortDevices(peers)
	want := []string{"alef", "beta", "Alpha", "zeta"}
	for i := range want {
		if got[i].HostName != want[i] {
			t.Fatalf("порядок: %v", got)
		}
	}
	if peers[0].HostName != "zeta" {
		t.Error("SortDevices изменил вход")
	}
	opts := ExitNodeOptions(TailscaleStatus{UserGroups: []TailscaleUserGroup{{Peers: peers}}})
	if len(opts) != 2 || opts[0].HostName != "Alpha" || opts[1].HostName != "beta" {
		t.Errorf("выходы: %v", opts)
	}
}
