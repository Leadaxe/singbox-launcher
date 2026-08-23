package build

// Конформанс-раннер корпуса DNS-СЕРВЕРОВ (SPEC 109, фаза 6).
//
// Гоняет contract/corpus/dns/*.dns.json через тот же путь, что боевая
// сборка: записи шаблона → template.NormalizeDNSOptions → ResolveDNS →
// тела для dns.servers[]. Проверяется то, что увидит ядро, а не
// промежуточная структура: иначе раннер начал бы жить своей жизнью.
//
// Тот же корпус читает LxBox — расхождение expected означает, что стороны
// по-разному собирают одну и ту же запись.
//
// Регенерация: go test ./core/build -run TestContractCorpusDNS -update-dns

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

var updateDNSCorpus = flag.Bool("update-dns", false,
	"перезаписать expected-файлы корпуса DNS")

type corpusDNSCase struct {
	Doc     string                   `json:"_"`
	Servers []map[string]interface{} `json:"servers"`
	Vars    map[string]string        `json:"vars,omitempty"`
	State   []corpusDNSStateEntry    `json:"state,omitempty"`
}

type corpusDNSStateEntry struct {
	Tag     string `json:"tag"`
	Enabled bool   `json:"enabled"`
}

type corpusDNSExpected struct {
	Servers []map[string]interface{} `json:"servers"`
}

func TestContractCorpusDNS(t *testing.T) {
	dir := filepath.Join("..", "..", "contract", "corpus", "dns")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("корпус недоступен: %v", err)
	}
	cases := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".dns.json") {
			continue
		}
		caseName := strings.TrimSuffix(name, ".dns.json")
		cases++
		t.Run(caseName, func(t *testing.T) { runDNSCorpusCase(t, dir, caseName) })
	}
	if cases == 0 {
		t.Fatal("корпус DNS пуст — раннер молча проходил бы всегда")
	}
}

func runDNSCorpusCase(t *testing.T, dir, caseName string) {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, caseName+".dns.json"))
	if err != nil {
		t.Fatalf("read case: %v", err)
	}
	var in corpusDNSCase
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatalf("parse case: %v", err)
	}

	// Шов контракта (разрыв N8): вложенные записи разворачиваются в плоские,
	// их vars становятся переменными шаблона.
	section, err := json.Marshal(map[string]interface{}{"servers": in.Servers})
	if err != nil {
		t.Fatalf("marshal section: %v", err)
	}
	normalized, declaredVars := template.NormalizeDNSOptions(section)

	td := &template.TemplateData{
		DNSOptionsRaw: normalized,
		Vars:          declaredVars,
	}
	st := &corestate.State{}
	for _, e := range in.State {
		st.DNS.Servers = append(st.DNS.Servers, corestate.DNSServer{
			Kind:    corestate.DNSServerKindTemplate,
			Tag:     e.Tag,
			Enabled: e.Enabled,
		})
	}

	resolved := ResolveDNS(st, td, in.Vars, template.TargetSpec{})

	got := corpusDNSExpected{Servers: []map[string]interface{}{}}
	for _, srv := range resolved.Servers {
		if !srv.Active || !srv.Enabled {
			continue // выключенная запись не эмитится вовсе
		}
		got.Servers = append(got.Servers, srv.Body)
	}

	expPath := filepath.Join(dir, caseName+".expected.json")
	if *updateDNSCorpus {
		data, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatalf("marshal expected: %v", err)
		}
		if err := os.WriteFile(expPath, append(data, '\n'), 0o644); err != nil {
			t.Fatalf("write expected: %v", err)
		}
		return
	}

	expRaw, err := os.ReadFile(expPath)
	if err != nil {
		t.Fatalf("read expected: %v", err)
	}
	var want corpusDNSExpected
	if err := json.Unmarshal(expRaw, &want); err != nil {
		t.Fatalf("parse expected: %v", err)
	}

	gotJSON, _ := json.MarshalIndent(got, "", "  ")
	wantJSON, _ := json.MarshalIndent(want, "", "  ")
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("%s\n--- получено ---\n%s\n--- ожидалось ---\n%s",
			in.Doc, gotJSON, wantJSON)
	}
}
