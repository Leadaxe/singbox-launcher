package backup

import (
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// Узел из wg-quick INI обязан пережить экспорт и импорт БЕЗ потери исходника.
//
// Контракт (общий с LxBox) знает только `uri` и `config_json`, третьего ключа
// в нём нет — блок [Interface]/[Peer] едет тем же `uri`, а вид определяется по
// форме текста. Без этого INI-узел экспортировался телом: исходник со всеми
// комментариями (включая имя пира) пропадал, и узел терял «Regen from raw».
func TestWGIniSurvivesExportImport(t *testing.T) {
	const ini = "[Interface]\n" +
		"PrivateKey = KHes8lqvbelaDOuiJKyQOIlXnjLlxxeOeUYC5f8fn2I=\n" +
		"Address = 10.2.0.2/32\n\n" +
		"[Peer]\n" +
		"# US-FREE#137\n" +
		"PublicKey = 01HVawb6Snd1f9KKpwjwp5Kaj4RU8pOt2O/iWOkTCEc=\n" +
		"Endpoint = 194.180.34.8:51820\n"

	src := state.Source{
		Node: state.Node{
			Kind:    state.SourceKindServer,
			Tag:     "US-FREE#137",
			Enabled: true,
			Origin:  &state.Origin{Kind: state.OriginKindWGIni, Raw: ini},
			Body:    []byte(`{"type":"wireguard"}`),
		},
		ID: "SRV",
	}

	exported := exportServer(src)
	if !strings.Contains(exported.URI, "[Interface]") {
		t.Fatalf("экспорт потерял блок INI: uri=%q config_json=%q", exported.URI, exported.ConfigJSON)
	}

	back, _ := importServer(exported)
	if back.Origin == nil {
		t.Fatal("импорт вернул узел без origin")
	}
	if back.Origin.Kind != state.OriginKindWGIni {
		t.Errorf("origin.kind = %q, ожидали %q", back.Origin.Kind, state.OriginKindWGIni)
	}
	if !strings.Contains(back.Origin.Raw, "# US-FREE#137") {
		t.Errorf("исходник потерял имя пира: %q", back.Origin.Raw)
	}
}
