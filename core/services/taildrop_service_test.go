package services

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestTaildropServiceFilesContract — кодек служебных файлов против нормы
// contract/docs/SERVICE_FILES.md и констант registry/service_files.json:
// имя и тело отправителя, граница «служебный / обычный файл» получателя.
func TestTaildropServiceFilesContract(t *testing.T) {
	r, err := TaildropServiceRulesFromRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Отправитель: имя lx-<UTC>-<метка>.json, тело {"lx":1,"commands":[…]}.
	at := time.Date(2026, 11, 2, 23, 31, 53, 0, time.FixedZone("MSK", 3*3600))
	name, body, err := r.BuildTaildropService(TaildropServiceCommand{Kind: TaildropServiceClipboard, Text: "a <b> & \"c\"\n"}, at)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^lx-20261102T203153Z-[a-z0-9]{5}\.json$`).MatchString(name) {
		t.Fatalf("name %q", name)
	}
	if want := `{"lx":1,"commands":[{"kind":"clipboard","text":"a <b> & \"c\"\n"}]}`; string(body) != want {
		t.Fatalf("body %s", body)
	}
	if !r.TaildropServiceCandidate(name, int64(len(body))) {
		t.Fatal("own file is not a candidate")
	}
	cmds, ok := r.ParseTaildropService(body)
	if !ok || len(cmds) != 1 || cmds[0].Kind != TaildropServiceClipboard {
		t.Fatalf("round trip: %v %+v", ok, cmds)
	}

	// Тело LxBox с эмулятора (стенд 03.11.2026) и порядок команд.
	cmds, ok = r.ParseTaildropService([]byte(`{"lx":1,"commands":[{"kind":"clipboard","text":"\ufeffhello"},{"kind":"open_url","url":"https://example.com/x?y=1"},{"kind":"beep"}]}`))
	if !ok || len(cmds) != 3 {
		t.Fatalf("lxbox body: %v %+v", ok, cmds)
	}
	if text, ok := cmds[0].ClipboardText(); !ok || text != "hello" {
		t.Fatalf("BOM not stripped: %q", text)
	}
	if !r.OpenURLAllowed(cmds[1].URL) || cmds[2].Kind != "beep" {
		t.Fatalf("commands %+v", cmds)
	}

	// Обычные файлы входящих (§4): не служебные.
	for _, b := range []string{
		``, `not json`, `{"commands":[]}`, `{"lx":2,"commands":[]}`, `{"lx":"1","commands":[]}`,
		`{"lx":1}`, `{"lx":1,"commands":{}}`, "\xff\xfe",
		`{"lx":1,"commands":[]}` + strings.Repeat(" ", int(r.MaxBytes)),
	} {
		if _, ok := r.ParseTaildropService([]byte(b)); ok {
			t.Errorf("parsed as service: %.40q", b)
		}
	}
	if r.TaildropServiceCandidate("photo.json", 10) || r.TaildropServiceCandidate("lx-a.json", r.MaxBytes+1) {
		t.Error("candidate by name/size")
	}
	if !r.TaildropServiceCandidate("lx-20261102T233153Z-copxm (1).json", 74) {
		t.Error("renamed by core must stay a candidate")
	}

	// Ссылки и отказы отправителя (§7).
	if _, ok := r.SingleURL("https://a.example/b"); !ok {
		t.Error("single url")
	}
	for _, s := range []string{"ftp://a.example", "https://a b", "two https://a https://b", "javascript:alert(1)"} {
		if _, ok := r.SingleURL(s); ok {
			t.Errorf("single url %q", s)
		}
	}
	if _, _, err := r.BuildTaildropService(TaildropServiceCommand{Kind: TaildropServiceClipboard}, at); err != ErrTaildropServiceEmpty {
		t.Errorf("empty: %v", err)
	}
	if _, _, err := r.BuildTaildropService(TaildropServiceCommand{Kind: TaildropServiceClipboard, Text: strings.Repeat("x", int(r.MaxBytes))}, at); err != ErrTaildropServiceTooBig {
		t.Errorf("too big: %v", err)
	}
	if _, _, err := r.BuildTaildropService(TaildropServiceCommand{Kind: TaildropServiceClipboard, Text: "\xff"}, at); err != ErrTaildropServiceNotUTF8 {
		t.Errorf("not utf-8: %v", err)
	}
	if r.TTL != 120*time.Second {
		t.Errorf("ttl %v", r.TTL)
	}
}
