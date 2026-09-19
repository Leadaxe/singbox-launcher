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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
)

const corpusRoot = "../../../contract/corpus/uri"

// allowedDeltasFile — машинный список разрешённых расхождений, парный к
// DELTAS.md. Пары: <схема>/<кейс> → причина.
const allowedDeltasFile = "testdata/deltas_allowed.json"

// schemesWithURISection — схемы, у которых есть секция `uri`. Сверка гоняется
// по ним.
//
// Список БОЛЬШЕ НЕ ДАННЫЕ: пока кампания шла волнами, он перечислял
// переключённые схемы поимённо (testdata/switched.json), потому что остальные
// ещё вёл рукописный парсер и их расхождения ничего не значили. Рукописных
// парсеров не осталось, и «переключённая» теперь означает ровно «секция
// есть» — а это знает реестр. Держать рядом второй список значило бы забыть
// дописать в него следующую схему и не заметить, что её никто не сверяет.
//
// Имён схем в коде по-прежнему нет: они приходят с диска.
func schemesWithURISection(t *testing.T, set *registry.MapperSet) []string {
	t.Helper()
	var out []string
	for _, scheme := range set.Schemes() {
		if _, ok := set.Mapper(scheme, "uri"); !ok {
			continue
		}
		// Каталог корпуса назван по ФАЙЛУ протокола, а не по схеме: у
		// shadowsocks схема зовётся `ss`, а каталог и файл —
		// `shadowsocks`. Прежний поимённый список это скрывал, называя
		// каталог; выведенный из реестра давал `ss`, каталога с таким
		// именем нет, и вся схема сверялась НУЛЁМ кейсов молча.
		out = append(out, protocolFileFor(scheme))
	}
	sort.Strings(out)
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
	allowed := loadAllowedDeltas(t)

	set, err := registry.LoadMappers()
	if err != nil {
		t.Fatalf("LoadMappers: %v", err)
	}
	schemes := schemesWithURISection(t, set)
	if len(schemes) == 0 {
		t.Fatal("ни у одной схемы нет секции uri — сверять нечего")
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
		// Схема с секцией, но без единого кейса — не «нечего сверять», а
		// дыра в покрытии: именно так shadowsocks молча выпал из сверки.
		if len(cases) == 0 {
			t.Errorf("%s: секция uri есть, а кейсов корпуса нет — сверять нечем", dir)
			continue
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
	// Схему выбирает РЕЕСТР своим detect, а не префикс ссылки: написание и
	// схема — разные вещи (`hy2://` → hysteria2, `socks5://` → socks,
	// `naive+quic://` → naive), и сверка обязана идти тем же путём, каким
	// пойдёт разбор.
	scheme, plan, ok := selectPlanFor(plans, uri)
	if !ok {
		t.Skipf("ни одна секция uri не опознала ссылку")
	}
	bodyType := reg.SingboxType(scheme)
	res, err := ParseURI(plan, uri, bodyType, nil)
	if err != nil {
		return nil, err
	}
	// Тело маппера прогоняется через САНИТАЙЗЕР — стадия 6 конвейера
	// (MAPPER_ENGINE.md §1). Без неё сверка невозможна по построению: фикстуры
	// сняты ПОСЛЕ санитайзера, а маппер значения не судит (норма: «маппер
	// переводит диалект, годность судит санитайзер»). Сравнивать его сырое
	// тело с фикстурой значило бы требовать от движка чужой работы.
	sr := nodeflow.SanitizeFrom(scheme, plan.Mapper.BodySource, res.Body)
	if sr.Drop != nil {
		return nil, fmt.Errorf("санитайзер отверг узел: %s", sr.Drop.Code)
	}
	return sr.Clean, nil
}

// selectPlanFor находит секцию, чей detect опознал ссылку.
func selectPlanFor(plans *PlanSet, uri string) (string, *Plan, bool) {
	content := NewContent(uri)
	var hits []string
	for _, scheme := range plans.Schemes() {
		plan, ok := plans.Plan(scheme, "uri")
		if !ok || plan.Mapper.Detect == nil {
			continue
		}
		if Matches(plan.Mapper.Detect, content) {
			hits = append(hits, scheme)
		}
	}
	if len(hits) != 1 {
		return "", nil, false
	}
	plan, _ := plans.Plan(hits[0], "uri")
	return hits[0], plan, true
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
