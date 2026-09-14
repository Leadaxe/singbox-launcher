package backup

import (
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// Узел из wg-quick INI обязан пережить экспорт и импорт БЕЗ потери исходника.
//
// Входов два. В 1.0 исходник едет своим полем `origin{kind: wg_ini, raw}`. В
// файле 0.12 третьего ключа не было — контракт знал только `uri` и
// `config_json`, и блок [Interface]/[Peer] ехал тем же `uri`, а вид
// определялся по форме текста. Без этого INI-узел приезжал телом: исходник со
// всеми комментариями (включая имя пира) пропадал, и узел терял «Regen from
// raw».
func TestWGIniSurvivesExportImport(t *testing.T) {
	const ini = "[Interface]\n" +
		"PrivateKey = KHes8lqvbelaDOuiJKyQOIlXnjLlxxeOeUYC5f8fn2I=\n" +
		"Address = 10.2.0.2/32\n\n" +
		"[Peer]\n" +
		"# US-FREE#137\n" +
		"PublicKey = 01HVawb6Snd1f9KKpwjwp5Kaj4RU8pOt2O/iWOkTCEc=\n" +
		"Endpoint = 194.180.34.8:51820\n"

	checkOrigin := func(t *testing.T, format string, origin *state.Origin) {
		t.Helper()
		if origin == nil {
			t.Fatalf("%s: импорт вернул узел без origin", format)
		}
		if origin.Kind != state.OriginKindWGIni {
			t.Errorf("%s: origin.kind = %q, ожидали %q", format, origin.Kind, state.OriginKindWGIni)
		}
		if !strings.Contains(origin.Raw, "# US-FREE#137") {
			t.Errorf("%s: исходник потерял имя пира: %q", format, origin.Raw)
		}
	}

	// 1.0: круг «экспорт → импорт».
	src := &state.State{Sources: []state.Source{{
		Node: state.Node{
			Kind:    state.SourceKindServer,
			Tag:     "US-FREE#137",
			Enabled: true,
			Origin:  &state.Origin{Kind: state.OriginKindWGIni, Raw: ini},
			Body:    []byte(`{"type":"wireguard"}`),
		},
		ID: "SRV",
	}}}
	b, _, err := Export10(src, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	dst := &state.State{}
	if _, err := Import10(dst, b, ImportOptions{}); err != nil {
		t.Fatalf("Import10: %v", err)
	}
	if len(dst.Sources) != 1 {
		t.Fatalf("1.0: источников после импорта %d", len(dst.Sources))
	}
	checkOrigin(t, "1.0", dst.Sources[0].Origin)

	// 0.12: запись servers[], какой её писал прежний писатель, — INI в `uri`.
	back, _ := importServer(Server{ID: "SRV", NodeTag: "US-FREE#137", URI: ini})
	checkOrigin(t, "0.12", back.Origin)
}
