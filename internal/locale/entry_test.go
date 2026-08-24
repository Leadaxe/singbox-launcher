package locale

import "testing"

func TestEntryBareString(t *testing.T) {
	m, skipped, err := parseCatalog([]byte(`{"Save": "Сохранить"}`))
	if err != nil || len(skipped) != 0 {
		t.Fatalf("parseCatalog: err=%v skipped=%v", err, skipped)
	}
	if got := m["Save"].Value.Text; got != "Сохранить" {
		t.Errorf("bare string: got %q", got)
	}
}

func TestEntryValueObject(t *testing.T) {
	m, _, err := parseCatalog([]byte(`{"Save": {"value": "Сохранить"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := m["Save"].Value.Text; got != "Сохранить" {
		t.Errorf("value object: got %q", got)
	}
}

func TestEntryPluralForms(t *testing.T) {
	m, _, err := parseCatalog([]byte(`{
		"%d nodes": {"value": {"one": "%d узел", "few": "%d узла", "many": "%d узлов", "other": "%d узла"}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	v := m["%d nodes"].Value
	if v.Text != "" || len(v.Forms) != 4 || v.Forms["many"] != "%d узлов" {
		t.Errorf("plural forms parsed wrong: %+v", v)
	}
}

func TestEntrySpecial(t *testing.T) {
	m, _, err := parseCatalog([]byte(`{
		"Copy": {"value": "Копировать", "special": {"1": {"value": "Скопировать"}}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	e := m["Copy"]
	if e.Value.Text != "Копировать" {
		t.Errorf("root: %q", e.Value.Text)
	}
	if sp := e.Special["1"]; sp.Value.Text != "Скопировать" {
		t.Errorf("special[1]: %q", sp.Value.Text)
	}
}

func TestEntryMalformedSkipped(t *testing.T) {
	// один битый entry не валит каталог
	m, skipped, err := parseCatalog([]byte(`{
		"Good": "ok",
		"Bad": 42,
		"AlsoBad": {"nothing": true}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 2 {
		t.Errorf("skipped = %v, want 2 entries", skipped)
	}
	if m["Good"].Value.Text != "ok" {
		t.Error("good entry lost")
	}
}

func TestEntryTopLevelError(t *testing.T) {
	if _, _, err := parseCatalog([]byte(`not json`)); err == nil {
		t.Error("top-level garbage must be an error")
	}
}

// --- рендер через тестовый каталог ---

func withTestCatalog(t *testing.T, code string, data string) {
	t.Helper()
	m, skipped, err := parseCatalog([]byte(data))
	if err != nil || len(skipped) > 0 {
		t.Fatalf("test catalog: err=%v skipped=%v", err, skipped)
	}
	mu.Lock()
	catalogs[code] = m
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		delete(catalogs, code)
		lang = "en"
		mu.Unlock()
	})
}

func TestTNSpecialForms(t *testing.T) {
	withTestCatalog(t, "xx", `{
		"Copy": {"value": "Копировать", "special": {"1": {"value": "Скопировать"}}}
	}`)
	SetLang("xx")

	if got := T("Copy"); got != "Копировать" {
		t.Errorf("T = %q", got)
	}
	if got := TN(1, "Copy"); got != "Скопировать" {
		t.Errorf("TN(1) = %q", got)
	}
	// отсутствующая special-форма падает на форму 0
	if got := TN(2, "Copy"); got != "Копировать" {
		t.Errorf("TN(2) fallback = %q", got)
	}
	// промах ключа деградирует в сам ключ
	if got := TN(1, "No such key"); got != "No such key" {
		t.Errorf("miss = %q", got)
	}
}

func TestPluralRender(t *testing.T) {
	withTestCatalog(t, "ru", `{
		"%d nodes": {"value": {"one": "%d узел", "few": "%d узла", "many": "%d узлов", "other": "%d узла"}}
	}`)
	SetLang("ru")

	cases := map[int]string{1: "1 узел", 3: "3 узла", 7: "7 узлов", 21: "21 узел"}
	for n, want := range cases {
		if got := Plural("%d nodes", n); got != want {
			t.Errorf("Plural(%d) = %q, want %q", n, got, want)
		}
	}
	// промах ключа: английский ключ как шаблон
	if got := Plural("%d hops", 2); got != "2 hops" {
		t.Errorf("miss = %q", got)
	}
}

func TestPluralSingleFormGraceful(t *testing.T) {
	// язык перевёл plural-ключ одной строкой — не ломаемся
	withTestCatalog(t, "xx", `{"%d nodes": "узлов: %d"}`)
	SetLang("xx")
	if got := Plural("%d nodes", 5); got != "узлов: 5" {
		t.Errorf("graceful single form = %q", got)
	}
}

func TestFallbackSubstitutesArgs(t *testing.T) {
	SetLang("en")
	if got := Tf("Connected to %s", "host1"); got != "Connected to host1" {
		t.Errorf("fallback Tf = %q", got)
	}
	if got := TfN(1, "%s available", "v2"); got != "v2 available" {
		t.Errorf("fallback TfN = %q", got)
	}
}
