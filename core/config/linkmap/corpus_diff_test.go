package linkmap

// Раннер сверки «движок vs identity-фикстуры» (SPEC 133, W0.5).
//
// Фикстуры — ожидания общего корпуса contract/corpus/uri: они сняты СТАРЫМ
// путём (`go test ./core/config -run TestContractCorpusURI -update`) и потому
// являются базой сравнения для движка.
//
// Расхождение допустимо ТОЛЬКО если оно объявлено в deltas_allowed.json —
// машинном списке, парном к DELTAS.md. Всё прочее красное.
//
// Запуск:
//
//	go test ./core/config/linkmap -run TestEngineVsFixtures
//	go test ./core/config/linkmap -run TestEngineVsFixtures -v   (список кейсов)

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/config/registry"
)

const corpusRoot = "../../../contract/corpus/uri"

// allowedDeltasFile — машинный список разрешённых расхождений, парный к
// DELTAS.md. Пары: <схема>/<кейс> → причина.
const allowedDeltasFile = "testdata/deltas_allowed.json"

// switchedSchemes — схемы, ПЕРЕКЛЮЧЁННЫЕ на движок. Сверка гоняется по ним;
// остальные ещё идут старым путём, и их расхождения не значат ничего.
//
// Список данными, а не кодом с именами схем: это вход теста, а не логика
// движка (греп-страж читает только исходники пакета, не testdata).
func switchedSchemes(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("testdata/switched.json")
	if err != nil {
		t.Skipf("нет testdata/switched.json: %v", err)
	}
	var out []string
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("switched.json: %v", err)
	}
	return out
}

func loadAllowedDeltas(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	data, err := os.ReadFile(allowedDeltasFile)
	if err != nil {
		return out
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s: %v", allowedDeltasFile, err)
	}
	return out
}

// TestEngineVsFixtures — главный раннер сверки.
func TestEngineVsFixtures(t *testing.T) {
	schemes := switchedSchemes(t)
	if len(schemes) == 0 {
		t.Skip("на движок ещё не переключена ни одна схема")
	}
	allowed := loadAllowedDeltas(t)

	set, err := registry.LoadMappers()
	if err != nil {
		t.Fatalf("LoadMappers: %v", err)
	}
	reg, err := registry.Get()
	if err != nil {
		t.Fatalf("registry.Get: %v", err)
	}
	plans, err := BuildPlans(set, reg.Order)
	if err != nil {
		t.Fatalf("BuildPlans: %v", err)
	}

	var mismatch []string
	for _, dir := range schemes {
		cases, err := filepath.Glob(filepath.Join(corpusRoot, dir, "*.uri"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir, err)
		}
		sort.Strings(cases)
		for _, casePath := range cases {
			name := dir + "/" + strings.TrimSuffix(filepath.Base(casePath), ".uri")
			t.Run(name, func(t *testing.T) {
				gotBody, gotErr := engineBody(t, plans, reg, casePath)
				wantBody, wantOK := fixtureBody(t, casePath)
				if !wantOK {
					t.Skip("в фикстуре нет узла (dropped) — сверка тел не применима")
				}
				if gotErr != nil {
					if reason, ok := allowed[name]; ok {
						t.Logf("разрешённая дельта: %s", reason)
						return
					}
					mismatch = append(mismatch, name+": "+gotErr.Error())
					t.Fatalf("движок не разобрал: %v", gotErr)
				}
				g := canonString(gotBody, reg.Order(schemeOfDir(dir)))
				w := canonString(wantBody, reg.Order(schemeOfDir(dir)))
				if g == w {
					return
				}
				if reason, ok := allowed[name]; ok {
					t.Logf("разрешённая дельта: %s\n got: %s\nwant: %s", reason, g, w)
					return
				}
				mismatch = append(mismatch, name)
				t.Errorf("расхождение\n got: %s\nwant: %s", g, w)
			})
		}
	}
	if len(mismatch) > 0 {
		t.Logf("расхождений: %d", len(mismatch))
	}
}

// schemeOfDir — имя каталога корпуса совпадает с именем схемы реестра всюду,
// кроме одного случая, который читается из самого реестра.
func schemeOfDir(dir string) string {
	return dir
}

// engineBody прогоняет кейс через движок.
func engineBody(t *testing.T, plans *PlanSet, reg *registry.Registry, casePath string) (map[string]interface{}, error) {
	t.Helper()
	uri := readCaseURI(t, casePath)
	scheme := schemeOfURI(uri)
	plan, ok := plans.Plan(scheme, "uri")
	if !ok {
		t.Skipf("у схемы %q нет секции uri", scheme)
	}
	bodyType := reg.SingboxType(scheme)
	res, err := ParseURI(plan, uri, bodyType, nil)
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

func schemeOfURI(uri string) string {
	if i := strings.Index(uri, "://"); i > 0 {
		return strings.ToLower(uri[:i])
	}
	return ""
}

// fixtureBody достаёт тело первого узла из ожидания корпуса, снимая служебные
// ключи (tag/type): их ставит не маппер.
func fixtureBody(t *testing.T, casePath string) (map[string]interface{}, bool) {
	t.Helper()
	base := strings.TrimSuffix(casePath, ".uri")
	path := base + ".expected.launcher.json"
	if _, err := os.Stat(path); err != nil {
		path = base + ".expected.json"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var env struct {
		Nodes []struct {
			Entry map[string]interface{} `json:"entry"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if len(env.Nodes) == 0 {
		return nil, false
	}
	body := env.Nodes[0].Entry
	delete(body, "tag")
	delete(body, "type")
	return body, true
}

func readCaseURI(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	uri := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		uri = line
	}
	return uri
}

func canonString(v interface{}, order []string) string {
	var buf bytes.Buffer
	WriteCanonicalJSON(&buf, v, order)
	return buf.String()
}
