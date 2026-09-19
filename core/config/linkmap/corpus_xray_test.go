package linkmap

// Раннер сверки «движок vs ожидания корпуса тел» для входа Xray (SPEC 133).
//
// Пара к TestEngineVsFixtures, и роль у него та же: ожидания
// contract/corpus/body/xray сняты СТАРЫМ рукописным конвертером
// (xray_outbound_convert.go и соседи) и потому являются базой сравнения.
// Перевод входа на движок обязан их сохранить — у LxBox на том же переезде
// молча уехало ~25 полей XHTTP, и поймал это ровно такой прогон.
//
// Расхождение допустимо ТОЛЬКО если объявлено в deltas_allowed.json под
// ключом "xray/<кейс>" — машинном списке, парном к DELTAS.md.
//
// Запуск:
//
//	go test ./core/config/linkmap -run TestEngineVsXrayCorpus
//	go test ./core/config/linkmap -run TestEngineVsXrayCorpus -v

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
)

const xrayCorpusRoot = "../../../contract/corpus/body/xray"

// xrayKind — имя вида источника, секции которого сверяет этот раннер.
//
// Имя живёт в ТЕСТЕ, а не в движке: движок берёт вид параметром, и назови он
// его сам, вернулся бы скрытый диспетчер диалекта (страж
// TestNoSchemeNamesInEngine). Тесту называть вид законно — он кормит движок
// конкретным корпусом и ждёт конкретных тел.
const xrayKind = "xray"

// TestEngineVsXrayCorpus — сверка тел, собранных движком, с ожиданиями.
//
// Сверяются только те кейсы, где ожидание несёт РОВНО ОДИН узел: кейсы с
// цепочками, балансерами и мультиузловыми документами решает уровень
// ДОКУМЕНТА (какой элемент становится узлом, какой хопом), а он на движок
// ещё не переведён — сверять их здесь значило бы требовать от таблицы узла
// чужой работы. Такие кейсы держит TestContractCorpusBody целиком.
func TestEngineVsXrayCorpus(t *testing.T) {
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

	files, err := filepath.Glob(filepath.Join(xrayCorpusRoot, "*.body"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("корпус xray пуст — сверять нечего")
	}
	sort.Strings(files)

	checked := 0
	for _, bodyPath := range files {
		name := strings.TrimSuffix(filepath.Base(bodyPath), ".body")
		t.Run(name, func(t *testing.T) {
			elems := xrayElements(t, bodyPath)
			want, wantOK := xraySingleExpected(t, bodyPath)
			if !wantOK {
				t.Skip("ожидание несёт не один узел — кейс уровня документа")
			}
			if len(elems) != 1 {
				t.Skip("в теле не один элемент-outbound — кейс уровня документа")
			}

			scheme, plan, ok := SelectElementSection(plans, xrayKind, elems[0])
			if !ok {
				if reason, has := allowed["xray/"+name]; has {
					t.Logf("разрешённая дельта: %s", reason)
					return
				}
				t.Fatalf("ни одна секция xray не опознала элемент")
			}
			res, err := ParseElement(plan, elems[0], reg.SingboxType(scheme), nil)
			if err != nil {
				if reason, has := allowed["xray/"+name]; has {
					t.Logf("разрешённая дельта: %s", reason)
					return
				}
				t.Fatalf("движок не разобрал: %v", err)
			}
			// Тело маппера прогоняется через САНИТАЙЗЕР — стадия 6
			// конвейера: ожидания сняты ПОСЛЕ него, а маппер значения не
			// судит.
			sr := nodeflow.SanitizeFrom(scheme, plan.Mapper.BodySource, res.Body)
			if sr.Drop != nil {
				if reason, has := allowed["xray/"+name]; has {
					t.Logf("разрешённая дельта: %s", reason)
					return
				}
				t.Fatalf("санитайзер отверг узел: %s", sr.Drop.Code)
			}
			got := canonString(sr.Clean, reg.Order(scheme))
			wantStr := canonString(want, reg.Order(scheme))
			if got == wantStr {
				checked++
				return
			}
			if reason, has := allowed["xray/"+name]; has {
				t.Logf("разрешённая дельта: %s\n got: %s\nwant: %s", reason, got, wantStr)
				return
			}
			t.Errorf("расхождение\n got: %s\nwant: %s", got, wantStr)
		})
	}
	t.Logf("сверено кейсов: %d", checked)
}

// xrayElements достаёт элементы outbounds из тела корпуса.
//
// Строки-комментарии (#) снимает сам корпус: они несут происхождение кейса.
func xrayElements(t *testing.T, path string) []interface{} {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var clean []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		clean = append(clean, line)
	}
	var doc interface{}
	if err := json.Unmarshal([]byte(strings.Join(clean, "\n")), &doc); err != nil {
		t.Skipf("тело не JSON: %v", err)
	}
	var out []interface{}
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch t := v.(type) {
		case []interface{}:
			for _, e := range t {
				walk(e)
			}
		case map[string]interface{}:
			obs, _ := t["outbounds"].([]interface{})
			out = append(out, obs...)
		}
	}
	walk(doc)
	return out
}

// xraySingleExpected читает ожидание, когда в нём ровно один узел.
func xraySingleExpected(t *testing.T, bodyPath string) (map[string]interface{}, bool) {
	t.Helper()
	path := strings.TrimSuffix(bodyPath, ".body") + ".expected.json"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var env struct {
		Nodes []struct {
			Entry map[string]interface{} `json:"entry"`
		} `json:"nodes"`
		Dropped []json.RawMessage `json:"dropped"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if len(env.Nodes) != 1 || len(env.Dropped) > 0 {
		return nil, false
	}
	body := env.Nodes[0].Entry
	// tag/type ставит не маппер узла: тег приходит с уровня документа
	// (remarks/tag элемента), тип — из реестра схемы. Ровно так же их
	// снимает fixtureBody у сверки ссылок.
	delete(body, "tag")
	delete(body, "type")
	return body, true
}
