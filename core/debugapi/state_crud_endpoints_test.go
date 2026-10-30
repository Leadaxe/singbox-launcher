package debugapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// TestStateCRUDEndpoints — сценарий инцидента через API (SPEC 160): узел
// добавляется точкой /state/servers, правила при этом целы и config.json
// пересобран; неизвестная цель правила — 422; правило снимается по num.
func TestStateCRUDEndpoints(t *testing.T) {
	st := state.New()
	st.Directions = []configtypes.Direction{{Tag: "proxy-out", Type: "selector", AddOutbounds: []string{"direct-out"}}}
	n1000, n1001 := 1000, 1001
	st.Rules = []state.Rule{
		{Kind: state.RuleKindInline, Name: "r1", Enabled: true, Num: &n1000, Body: json.RawMessage(`{"domain_suffix":["a.com"],"outbound":"proxy-out"}`)},
		{Kind: state.RuleKindInline, Name: "r2", Enabled: true, Num: &n1001, Body: json.RawMessage(`{"domain_suffix":["b.com"],"outbound":"direct-out"}`)},
	}
	ff := &fakeFacade{stateValue: st}
	base, _ := newTestServer(t, ff)

	body, _ := json.Marshal(map[string]string{
		"input": "vless://2ee2a715-d541-416a-8713-d66567448c2e@91.98.155.240:443?encryption=none&security=none&type=grpc#de-1",
	})
	var added struct {
		OK            bool `json:"ok"`
		ConfigRebuilt bool `json:"config_rebuilt"`
		Added         []struct {
			Tag string `json:"tag"`
		} `json:"added"`
	}
	status, raw := doJSON(t, authedReq(t, http.MethodPost, base+"/state/servers", body), &added)
	if status != http.StatusOK || len(added.Added) != 1 || added.Added[0].Tag != "de-1" {
		t.Fatalf("POST /state/servers: %d %s", status, raw)
	}
	if ff.savedState == nil || len(ff.savedState.Rules) != 2 {
		t.Fatalf("rules after adding a server: %+v", ff.savedState)
	}
	if !added.ConfigRebuilt || ff.rebuilds != 1 {
		t.Errorf("config not rebuilt: config_rebuilt=%v rebuilds=%d", added.ConfigRebuilt, ff.rebuilds)
	}

	if status, raw := doJSON(t, authedReq(t, http.MethodDelete, base+"/state/servers?tag=nope", nil), nil); status != http.StatusNotFound {
		t.Errorf("DELETE unknown server: %d %s, want 404", status, raw)
	}

	ff.savedState = nil
	bad := []byte(`{"kind":"inline","name":"x","body":{"domain":["x.com"],"outbound":"nowhere"}}`)
	var fieldResp struct {
		Field string `json:"field"`
	}
	status, raw = doJSON(t, authedReq(t, http.MethodPost, base+"/state/rules", bad), &fieldResp)
	if status != http.StatusUnprocessableEntity || fieldResp.Field != "body.outbound" {
		t.Errorf("POST /state/rules unknown target: %d %s, want 422 body.outbound", status, raw)
	}
	if ff.savedState != nil {
		t.Error("rejected rule must not save state")
	}

	status, raw = doJSON(t, authedReq(t, http.MethodDelete, base+"/state/rules?num=1000", nil), nil)
	if status != http.StatusOK {
		t.Fatalf("DELETE /state/rules?num=1000: %d %s", status, raw)
	}
	if got := ff.savedState.Rules; len(got) != 1 || got[0].Name != "r2" {
		t.Errorf("rules after delete: %+v, want only r2", got)
	}
}

// TestStateFolderDetourFlow — сценарий issue #152 через API: завести пустую
// папку, положить в неё копию узла с detour на корневой узел, удалить папку.
// Непустая папка без with_nodes=1 не удаляется; с ним уходит вместе с узлами,
// и ссылки на её узлы снимаются у других носителей.
func TestStateFolderDetourFlow(t *testing.T) {
	st := state.New()
	st.Sources = []state.Source{
		state.NewServerSource("hop", json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)),
	}
	ff := &fakeFacade{stateValue: st}
	base, _ := newTestServer(t, ff)

	var created struct {
		Folder struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"folder"`
	}
	status, raw := doJSON(t, authedReq(t, http.MethodPost, base+"/state/folders", []byte(`{"name":"lx-test"}`)), &created)
	if status != http.StatusOK || created.Folder.ID == "" {
		t.Fatalf("POST /state/folders: %d %s", status, raw)
	}
	if status, raw := doJSON(t, authedReq(t, http.MethodPost, base+"/state/folders", []byte(`{"name":"lx-test"}`)), nil); status != http.StatusUnprocessableEntity {
		t.Errorf("duplicate folder name: %d %s, want 422", status, raw)
	}

	link := "vless://2ee2a715-d541-416a-8713-d66567448c2e@91.98.155.240:443?encryption=none&security=none&type=grpc#copy"
	bad, _ := json.Marshal(map[string]any{"input": link, "folder": "lx-test", "detour": map[string]string{"tag": "nowhere"}})
	if status, raw := doJSON(t, authedReq(t, http.MethodPost, base+"/state/servers", bad), nil); status != http.StatusUnprocessableEntity {
		t.Errorf("unknown detour target: %d %s, want 422", status, raw)
	}
	body, _ := json.Marshal(map[string]any{"input": link, "folder": "lx-test", "detour": map[string]string{"tag": "hop"}})
	if status, raw := doJSON(t, authedReq(t, http.MethodPost, base+"/state/servers", body), nil); status != http.StatusOK {
		t.Fatalf("POST /state/servers with detour: %d %s", status, raw)
	}
	folder := ff.stateValue.Sources[1]
	if len(folder.Nodes) != 1 || folder.Nodes[0].Detour == nil || *folder.Nodes[0].Detour != (state.NodeLink{Tag: "hop"}) {
		t.Fatalf("folder node detour: %+v", folder.Nodes)
	}

	// Корневой узел ходит через узел папки — удаление папки обязано снять ссылку.
	ff.stateValue.Sources[0].Detour = &state.NodeLink{FolderID: created.Folder.ID, Tag: "copy"}
	if status, raw := doJSON(t, authedReq(t, http.MethodDelete, base+"/state/folders?folder=lx-test", nil), nil); status != http.StatusUnprocessableEntity {
		t.Errorf("DELETE non-empty folder without with_nodes: %d %s, want 422", status, raw)
	}
	var deleted struct {
		Deleted struct {
			NodesRemoved []string `json:"nodes_removed"`
			LinksRemoved []string `json:"links_removed"`
		} `json:"deleted"`
	}
	status, raw = doJSON(t, authedReq(t, http.MethodDelete, base+"/state/folders?folder="+created.Folder.ID+"&with_nodes=1", nil), &deleted)
	if status != http.StatusOK || len(deleted.Deleted.NodesRemoved) != 1 || len(deleted.Deleted.LinksRemoved) != 1 {
		t.Fatalf("DELETE /state/folders with_nodes=1: %d %s", status, raw)
	}
	if got := ff.stateValue.Sources; len(got) != 1 || got[0].Detour != nil {
		t.Errorf("after folder delete: %+v, want only hop without detour", got)
	}
}
