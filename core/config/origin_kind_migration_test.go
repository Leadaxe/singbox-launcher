package config

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/state"
)

// Контракт 1.1.165 (PARSING_PRINCIPLES §11 п.6): старый вид `json` при
// чтении получает диалект по реестру — sing-box тело → `singbox`, Xray тело
// → `xray`, JSON без опознанного вида → `singbox`; текст источника не
// меняется. Хук классификации ставит пакет config, поэтому тест живёт здесь.
func TestMigrateLegacyOriginKindsByRegistry(t *testing.T) {
	const sb = `{"type":"trojan","server":"example-1.com","server_port":443,"password":"testpass722"}`
	const xr = `{"tag":"xr","protocol":"vless","settings":{"vnext":[{"address":"example-1.com","port":443,` +
		`"users":[{"id":"11111111-2222-3333-4444-555555555555"}]}]},"streamSettings":{"network":"tcp","security":"none"}}`
	const broken = `{"type": `
	const doc = `{"outbounds":[` + sb + `],"route":{"final":"x"}}`

	mk := func(tag, kind, raw string) state.Source {
		return state.Source{ID: "01" + tag, Node: state.Node{
			Kind: state.SourceKindServer, Tag: tag, Enabled: true,
			Body:   json.RawMessage(sb),
			Origin: &state.Origin{Kind: kind, Raw: raw},
		}}
	}
	s := &state.State{Sources: []state.Source{
		mk("sb", state.LegacyOriginKindJSON, sb),
		mk("xr", state.LegacyOriginKindJSON, xr),
		mk("broken", state.LegacyOriginKindJSON, broken),
		mk("doc", state.LegacyOriginKindJSON, doc),
		// JSON под видом `uri` (правка поля Origin до SPEC 157) — то же правило.
		mk("uri-json", state.OriginKindURI, xr),
		mk("uri", state.OriginKindURI, "trojan://testpass722@example-1.com:443#x"),
		mk("ini", state.OriginKindWGIni, "[Interface]\nPrivateKey = x\n[Peer]\nEndpoint = 192.0.2.1:51820\n"),
	}}
	if n := state.MigrateLegacyOriginKinds(s); n != 5 {
		t.Fatalf("переписано %d записей, ожидали 5", n)
	}
	want := map[string]string{
		"sb": state.OriginKindSingbox, "xr": state.OriginKindXray, "broken": state.OriginKindSingbox,
		"doc": state.OriginKindSingbox, "uri-json": state.OriginKindXray,
		"uri": state.OriginKindURI, "ini": state.OriginKindWGIni,
	}
	raws := map[string]string{"sb": sb, "xr": xr, "broken": broken, "doc": doc, "uri-json": xr}
	for _, src := range s.Sources {
		if got := src.Node.Origin.Kind; got != want[src.Tag] {
			t.Errorf("%s: вид %q, ожидали %q", src.Tag, got, want[src.Tag])
		}
		if raw, ok := raws[src.Tag]; ok && src.Node.Origin.Raw != raw {
			t.Errorf("%s: текст источника изменён миграцией: %s", src.Tag, src.Node.Origin.Raw)
		}
	}
	// Повторный проход ничего не трогает: вид уже диалект.
	if n := state.MigrateLegacyOriginKinds(s); n != 0 {
		t.Errorf("повторная миграция переписала %d записей", n)
	}
	// Документ sing-box остаётся документом и материализуется первым узлом.
	mat, err := MaterializeServerNode(doc, nil)
	if err != nil {
		t.Fatalf("документ не материализуется: %v", err)
	}
	if mat.OriginKind != state.OriginKindSingbox || mat.OriginRaw != doc {
		t.Errorf("материализация документа: kind=%q raw=%s", mat.OriginKind, mat.OriginRaw)
	}
	// Xray-текст материализуется разбором Xray и получает вид xray.
	mat, err = MaterializeServerNode(xr, nil)
	if err != nil {
		t.Fatalf("Xray outbound не материализуется: %v", err)
	}
	if mat.OriginKind != state.OriginKindXray || mat.OriginRaw != xr {
		t.Errorf("материализация Xray: kind=%q raw=%s", mat.OriginKind, mat.OriginRaw)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(mat.Body, &body); err != nil || body["type"] != "vless" || body["server"] != "example-1.com" {
		t.Errorf("тело Xray-узла: %s", mat.Body)
	}
}
