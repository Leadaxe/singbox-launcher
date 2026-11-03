package linkmap

// Раннер сверки «движок vs ожидания корпуса тел» для входа wg-quick `.conf`
// (SPEC 133).
//
// Третий в семье после TestEngineVsFixtures (ссылки) и TestEngineVsXrayCorpus
// (Xray-JSON), и роль та же: ожидания `contract/corpus/body/wgconf` сняты
// БОЕВЫМ путём — тем, что вёл рукописный конвертер `.conf` → ссылка с
// разбором получившегося URI секцией `mappers.uri`. Перевод входа на секцию
// `conf` обязан был их сохранить, и раннер это стерёг.
//
// Раннер заведён ДО перевода намеренно: секция `mappers.conf` написана
// волнами раньше и не исполнялась никем — а «написано, но не подключено»
// по правилу кампании считается НЕПРОВЕРЕННЫМ. Ровно так в общем блоке
// `transports#xray` пережили несколько ревизий неверные пути у ВСЕХ записей.
// Оправдалось: на расширенном корпусе раннер сразу нашёл Q133-60.
//
// Запуск:
//
//	go test ./core/config/linkmap -run TestEngineVsConfCorpus
//	go test ./core/config/linkmap -run TestEngineVsConfCorpus -v

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

const confCorpusRoot = "../../../contract/corpus/body/wgconf"

// confKind — имя вида источника, секции которого сверяет этот раннер.
// Живёт в ТЕСТЕ по той же причине, что и xrayKind: движок берёт вид
// параметром, и называть его сам он не вправе.
const confKind = "conf"

// TestEngineVsConfCorpus — сверка тел, собранных движком, с ожиданиями.
func TestEngineVsConfCorpus(t *testing.T) {
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

	files, err := filepath.Glob(filepath.Join(confCorpusRoot, "*.body"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("корпус wgconf пуст — сверять нечего")
	}
	sort.Strings(files)

	checked := 0
	for _, bodyPath := range files {
		name := strings.TrimSuffix(filepath.Base(bodyPath), ".body")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(bodyPath)
			if err != nil {
				t.Fatalf("чтение тела: %v", err)
			}
			text := stripCorpusComments(string(raw))

			want, wantOK := singleExpectedEntry(t, bodyPath)
			if !wantOK {
				t.Skip("ожидание несёт не один узел — кейс уровня документа")
			}

			scheme, plan, ok := selectConfSection(plans, text)
			if !ok {
				t.Fatalf("ни одна секция %s не опознала текст", confKind)
			}
			res, err := ParseURI(plan, text, reg.SingboxType(scheme), nil)
			if err != nil {
				if reason, has := allowed["wgconf/"+name]; has {
					t.Logf("разрешённая дельта: %s", reason)
					return
				}
				t.Fatalf("движок не разобрал: %v", err)
			}
			sr := nodeflow.SanitizeFrom(scheme, plan.Mapper.BodySource, res.Body)
			if sr.Drop != nil {
				t.Fatalf("санитайзер отверг узел: %s", sr.Drop.Code)
			}
			checkConfWarnings(t, bodyPath, res.Notes, sr.Warnings)
			got := canonString(sr.Clean, reg.Order(scheme))
			wantStr := canonString(want, reg.Order(scheme))
			if got == wantStr {
				checked++
				return
			}
			if reason, has := allowed["wgconf/"+name]; has {
				t.Logf("разрешённая дельта: %s\n got: %s\nwant: %s", reason, got, wantStr)
				return
			}
			t.Errorf("тело движка расходится с ожиданием\n got: %s\nwant: %s", got, wantStr)
		})
	}
	// Ноль СВЕРЕННЫХ кейсов — отказ (см. TestEngineVsXrayCorpus).
	if checked == 0 {
		t.Fatal("ни один кейс wgconf не сверен — все пропущены")
	}
	t.Logf("сверено кейсов: %d", checked)
}

// stripCorpusComments снимает ШАПКУ кейса — ведущие строки `#`, которыми
// корпус описывает происхождение тела.
//
// Пара к xrayElements, но резать приходится иначе и осторожнее: у ini
// комментарий — ЗАКОННАЯ часть тела (`ini.$comment.Peer` — первое звено
// метки), и снести все строки `#` значило бы отобрать у кейса имя узла.
// Поэтому режется только непрерывная шапка до первой непустой строки,
// которая комментарием не является.
func stripCorpusComments(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	cut := 0
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			cut++
			continue
		}
		break
	}
	return strings.Join(lines[cut:], "\n")
}

// singleExpectedEntry читает ожидание корпуса, когда в нём ровно один узел.
//
// Копия xraySingleExpected: снимаются `tag` и `type`. Оба — ключи СБОРКИ,
// а не тела узла (`nodeflow.buildManagedKeys`): тег приходит с идентичности
// узла (SPEC 112), тип — из реестра схемы. Ожидание корпуса снято боевым
// путём и потому несёт их оба; маппер не ставит ни того, ни другого.
func singleExpectedEntry(t *testing.T, bodyPath string) (map[string]interface{}, bool) {
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
	delete(body, "tag")
	delete(body, "type")
	return body, true
}

// confWarning — код из ожидания корпуса: путь и параметры сверяются, только
// если ожидание их несёт.
type confWarning struct {
	Code   string            `json:"code"`
	Path   string            `json:"path"`
	Params map[string]string `json:"params"`
}

// checkConfWarnings сверяет коды ожидания с кодами движка и санитайзера.
//
// Сверка — ВКЛЮЧЕНИЕМ: каждый код ожидания обязан найтись среди
// поставленных, лишние не ошибка. Тела корпуса сняты боевым путём, и коды
// в них записаны не все; нормативны те, что записаны (контракт 1.1.140 —
// первые кейсы, где код и есть предмет: снятый пир).
func checkConfWarnings(t *testing.T, bodyPath string, notes []Note, warnings []nodeflow.Warning) {
	t.Helper()
	data, err := os.ReadFile(strings.TrimSuffix(bodyPath, ".body") + ".expected.json")
	if err != nil {
		return
	}
	var env struct {
		Nodes []struct {
			Warnings []confWarning `json:"warnings"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(data, &env); err != nil || len(env.Nodes) != 1 {
		return
	}
	got := make([]confWarning, 0, len(notes)+len(warnings))
	for _, n := range notes {
		got = append(got, confWarning{Code: n.Code, Path: n.Path, Params: n.Params})
	}
	for _, w := range warnings {
		got = append(got, confWarning{Code: w.Code, Path: w.Path, Params: w.Params})
	}
	for _, want := range env.Nodes[0].Warnings {
		found := false
		for _, g := range got {
			if g.Code != want.Code || (want.Path != "" && g.Path != want.Path) {
				continue
			}
			match := true
			for k, v := range want.Params {
				if g.Params[k] != v {
					match = false
					break
				}
			}
			if match {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("нет кода %s (path %q, params %v); поставлены: %+v", want.Code, want.Path, want.Params, got)
		}
	}
}

// selectConfSection находит секцию вида `conf`, чей detect опознаёт текст.
//
// Тонкая обёртка над SelectKind движка: вход приезжает ТЕКСТОМ (как у
// ссылки), но секцию выбирает `detect` по ini-предикату, а не по написанию
// схемы. Общая функция выбора по виду появилась в движке вместе с боевым
// переключением — раннер зовёт ровно ту, что работает в бою.
func selectConfSection(plans *PlanSet, text string) (string, *Plan, bool) {
	return SelectKind(plans, confKind, text)
}
