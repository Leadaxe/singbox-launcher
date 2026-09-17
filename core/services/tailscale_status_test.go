package services

import (
	"testing"
	"time"

	daemonpb "singbox-launcher/internal/daemonpb"
)

// Конвертер pb → домен и кеш стрима — единственная логика SPEC 130, от
// которой зависит диагноз; форматтеры строк и UI не тестируются.
func TestTailscaleStatusConvertAndCache(t *testing.T) {
	upd := &daemonpb.TailscaleStatusUpdate{Endpoints: []*daemonpb.TailscaleEndpointStatus{
		{
			EndpointTag:  "ts",
			BackendState: TailscaleStateRunning,
			NetworkName:  "example.ts.net",
			KeyAuth:      true,
			Self:         &daemonpb.TailscalePeer{HostName: "mac", TailscaleIPs: []string{"100.64.0.2"}},
			ExitNode:     &daemonpb.TailscalePeer{HostName: "router", Online: true, ExitNode: true},
			UserGroups: []*daemonpb.TailscaleUserGroup{{
				LoginName: "sasha@example.com",
				Peers: []*daemonpb.TailscalePeer{
					{HostName: "router", Online: true, ExitNodeOption: true, LastSeen: 1_700_000_000},
					{HostName: "phone", Online: false},
					nil, // мусор в списке не валит конвертер
				},
			}},
		},
		{EndpointTag: "", BackendState: TailscaleStateRunning}, // без тега — пропуск
		nil,
		{EndpointTag: "ts2", BackendState: TailscaleStateNeedsLogin, AuthURL: "https://login/x"},
	}}

	at := time.Unix(1_800_000_000, 0)
	got := TailscaleStatusesFromPB(upd, at)
	if len(got) != 2 {
		t.Fatalf("want 2 statuses (пустой тег и nil отброшены), got %d: %v", len(got), got)
	}

	ts := got["ts"]
	if ts.BackendState != TailscaleStateRunning || !ts.KeyAuth || ts.NetworkName != "example.ts.net" {
		t.Errorf("шапка не перенесена: %+v", ts)
	}
	if ts.Self == nil || ts.Self.HostName != "mac" || ts.Self.TailscaleIPs[0] != "100.64.0.2" {
		t.Errorf("Self не перенесён: %+v", ts.Self)
	}
	if ts.ExitNode == nil || !ts.ExitNode.ExitNode || !ts.ExitNode.Online {
		t.Errorf("ExitNode не перенесён: %+v", ts.ExitNode)
	}
	if online, total := ts.PeersOnline(); online != 1 || total != 2 {
		t.Errorf("PeersOnline = %d/%d, want 1/2 (nil-пир отброшен)", online, total)
	}
	router := ts.UserGroups[0].Peers[0]
	if !router.ExitNodeOption || router.LastSeen.Unix() != 1_700_000_000 {
		t.Errorf("поля пира: %+v", router)
	}
	if !ts.ReceivedAt.Equal(at) {
		t.Errorf("ReceivedAt = %v, want %v", ts.ReceivedAt, at)
	}

	// NeedsLogin без Self — nil, не паника, AuthURL на месте.
	ts2 := got["ts2"]
	if ts2.Self != nil || ts2.AuthURL != "https://login/x" {
		t.Errorf("NeedsLogin: Self=%v AuthURL=%q", ts2.Self, ts2.AuthURL)
	}

	// Кеш: Apply заменяет снимок целиком, MarkDead снимает живость, не снимок.
	var c TailscaleStatusCache
	if _, ok := c.Get("ts"); ok || c.Live() {
		t.Fatal("пустой кеш не должен ничего отдавать")
	}
	fired := 0
	c.OnSnapshot(func() { fired++ })
	c.Apply(upd)
	if !c.Live() || fired != 1 {
		t.Errorf("после Apply: live=%v fired=%d", c.Live(), fired)
	}
	if st, ok := c.Get("ts"); !ok || st.EndpointTag != "ts" {
		t.Errorf("Get после Apply: ok=%v st=%+v", ok, st)
	}
	c.Apply(&daemonpb.TailscaleStatusUpdate{Endpoints: []*daemonpb.TailscaleEndpointStatus{
		{EndpointTag: "ts2", BackendState: TailscaleStateRunning},
	}})
	if _, ok := c.Get("ts"); ok {
		t.Error("ушедший из снимка endpoint остался в кеше — Apply обязан заменять, не сливать")
	}
	c.MarkDead()
	if c.Live() {
		t.Error("MarkDead не снял живость")
	}
	if _, ok := c.Get("ts2"); !ok {
		t.Error("MarkDead стёр снимок — должен оставить")
	}
}
