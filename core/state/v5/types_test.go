package v5

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestStateJSONRoundTrip — encode/decode цикл сохраняет данные.
func TestStateJSONRoundTrip(t *testing.T) {
	tr := true
	expire := int64(1717171717)
	src := State{
		Meta: MetaSection{
			Version:   SchemaVersion,
			Comment:   "test snapshot",
			CreatedAt: "2026-04-28T10:00:00Z",
			UpdatedAt: "2026-04-28T10:00:00Z",
		},
		Connections: ConnectionsSection{
			Sources: []Source{
				{
					ID:      "01ABCSUB",
					Type:    SourceTypeSubscription,
					Enabled: true,
					URL:     "https://example.com/sub",
					Tag:     &TagSpec{Prefix: "T:"},
					Meta: &SubscriptionMeta{
						ProfileTitle: "Test",
						UserInfo: &UserInfo{
							TotalBytes: 1 << 40,
							ExpireUnix: expire,
						},
						LastStatus: "ok",
					},
					Update: &UpdateSpec{IntervalHours: 4, AutoRefresh: &tr},
				},
				{
					ID:      "01ABCSRV",
					Type:    SourceTypeServer,
					Enabled: false,
					Label:   "wg-parnas",
					URI:     "wireguard://...#wg-parnas",
				},
			},
			Defaults: Defaults{Reload: "4h", MaxNodes: DefaultMaxNodes},
		},
		ConfigParams: []ConfigParam{{Name: "route.final", Value: "proxy-out"}},
		CustomRules:  []CustomRule{},
	}

	b, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got State
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, b)
	}

	if got.Meta.Version != SchemaVersion {
		t.Errorf("Version: got %d, want %d", got.Meta.Version, SchemaVersion)
	}
	if len(got.Connections.Sources) != 2 {
		t.Fatalf("Sources len: got %d, want 2", len(got.Connections.Sources))
	}
	sub := got.Connections.Sources[0]
	if sub.Type != SourceTypeSubscription {
		t.Errorf("sub.Type = %q, want %q", sub.Type, SourceTypeSubscription)
	}
	if sub.Tag == nil || sub.Tag.Prefix != "T:" {
		t.Errorf("sub.Tag prefix lost: %+v", sub.Tag)
	}
	if sub.Meta == nil || sub.Meta.UserInfo == nil || sub.Meta.UserInfo.ExpireUnix != expire {
		t.Errorf("sub.Meta.UserInfo lost")
	}
	if sub.Update == nil || sub.Update.IntervalHours != 4 || sub.Update.AutoRefresh == nil || !*sub.Update.AutoRefresh {
		t.Errorf("sub.Update lost")
	}
	srv := got.Connections.Sources[1]
	if srv.Type != SourceTypeServer || srv.Label != "wg-parnas" {
		t.Errorf("srv mangled: %+v", srv)
	}
}

// TestSourceOmitempty — пустые поля исчезают из JSON.
func TestSourceOmitempty(t *testing.T) {
	s := Source{
		ID:      "01ABC",
		Type:    SourceTypeServer,
		Enabled: true,
		Label:   "wg-parnas",
		URI:     "wireguard://...",
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	str := string(b)

	// Ожидаем что поля type=subscription не появятся.
	for _, want := range []string{`"id":"01ABC"`, `"type":"server"`, `"label":"wg-parnas"`, `"uri":"wireguard://..."`} {
		if !strings.Contains(str, want) {
			t.Errorf("missing %s in %s", want, str)
		}
	}
	for _, unwanted := range []string{`"url":`, `"skip":`, `"tag":`, `"outbounds":`, `"meta":`, `"update":`, `"max_nodes":`, `"expose_group_tags_to_global":`} {
		if strings.Contains(str, unwanted) {
			t.Errorf("unexpected %s in %s", unwanted, str)
		}
	}
}

// TestTagSpecIsZero — корректно определяем «пустой» TagSpec.
func TestTagSpecIsZero(t *testing.T) {
	cases := []struct {
		name string
		t    *TagSpec
		zero bool
	}{
		{"nil", nil, true},
		{"empty", &TagSpec{}, true},
		{"prefix", &TagSpec{Prefix: "X:"}, false},
		{"postfix", &TagSpec{Postfix: ":X"}, false},
		{"mask", &TagSpec{Mask: "{$tag}"}, false},
	}
	for _, c := range cases {
		if c.t.IsZero() != c.zero {
			t.Errorf("%s: IsZero = %v, want %v", c.name, c.t.IsZero(), c.zero)
		}
	}
}

// TestSubscriptionMetaOmitempty — пустой meta-объект сериализуется как `{}`,
// но никаких лишних полей быть не должно.
func TestSubscriptionMetaOmitempty(t *testing.T) {
	m := SubscriptionMeta{}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != "{}" {
		t.Errorf("empty meta: got %s, want {}", b)
	}
}
