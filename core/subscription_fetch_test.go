package core

// Поведенческие тесты fetch-конвейера подписки (SPEC 118 W3, §4.D):
// скачать → SubMeta → ParseSubscriptionBody → MergeSubscriptionNodes →
// updateStatus. Без внешней сети — тела отдаёт httptest-стаб; фан-аут не
// участвует (тестируется единичный refreshOneSubscriptionSource — ровно
// уровень fetch-сервиса).

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
)

// b64Body — sing-box JSON тело подписки в проводном виде: фетчер принимает
// JSON-объект только base64-обёрнутым (голый {} он отвергает как «конфиг
// вместо подписки» — поведение фетчера, не W3).
func b64Body(body string) string {
	return base64.StdEncoding.EncodeToString([]byte(body))
}

// stubSub — httptest-сервер с переключаемым телом/статусом между fetch'ами.
type stubSub struct {
	mu     sync.Mutex
	body   string
	status int
	srv    *httptest.Server
}

func newStubSub(t *testing.T, body string) *stubSub {
	t.Helper()
	s := &stubSub{body: body, status: http.StatusOK}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		body, status := s.body, s.status
		s.mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stubSub) set(body string, status int) {
	s.mu.Lock()
	s.body, s.status = body, status
	s.mu.Unlock()
}

func newFetchTestSource(url string) *state.Source {
	return &state.Source{
		Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true},
		ID:   "01FETCHSUB0000000000000000",
		Name: "sub",
		URL:  url,
	}
}

func fetchOnce(t *testing.T, src *state.Source, settings locale.Settings) {
	t.Helper()
	if !refreshOneSubscriptionSource(src, state.Defaults{}, settings, t.TempDir()) {
		t.Fatal("refreshOneSubscriptionSource ничего не изменил")
	}
}

func nodeTags(src *state.Source) string {
	tags := make([]string, 0, len(src.Nodes))
	for i := range src.Nodes {
		tags = append(tags, src.Nodes[i].Tag)
	}
	return strings.Join(tags, ",")
}

func vlessLine(i int, tag string) string {
	return fmt.Sprintf("vless://%d%d%d%d%d%d%d%d-1111-4111-8111-111111111111@s%d.example:443?security=tls&sni=s%d.example#%s",
		i, i, i, i, i, i, i, i, i, i, tag)
}

// §4.D.1 — skip отсекает до рождения узла; правка skip без fetch не меняет
// nodes[]; отсечённый выпадает на следующем merge как исчезнувший.
func TestFetchSkipActsOnNextFetchOnly(t *testing.T) {
	body := vlessLine(1, "RU-1") + "\n" + vlessLine(2, "NL-1")
	stub := newStubSub(t, body)
	src := newFetchTestSource(stub.srv.URL)

	fetchOnce(t, src, locale.Settings{})
	if nodeTags(src) != "RU-1,NL-1" {
		t.Fatalf("первый fetch: %s", nodeTags(src))
	}

	// Правка skip БЕЗ fetch — nodes[] нетронуты (Т4: правка модели не
	// запускает fetch и ничего не перепарсивает).
	src.Skip = []map[string]string{{"tag": "/(RU)/i"}}
	if nodeTags(src) != "RU-1,NL-1" {
		t.Fatalf("правка skip изменила nodes без fetch: %s", nodeTags(src))
	}

	fetchOnce(t, src, locale.Settings{})
	if nodeTags(src) != "NL-1" {
		t.Fatalf("после fetch со skip: %s", nodeTags(src))
	}
}

// §4.D.2 — дедуп по подписи: записи, различающиеся только фрагментом,
// схлопываются (регресс v1.5.2); члены Auto перепривязываются на выжившего.
func TestFetchDedupCollapsesAndRebindsMembers(t *testing.T) {
	var lines []string
	for i := 0; i < 32; i++ {
		lines = append(lines, fmt.Sprintf("ss://YWVzLTEyOC1nY206cGFzc3dvcmQ=@srv.example:8388#name-%d", i))
	}
	stub := newStubSub(t, strings.Join(lines, "\n"))
	src := newFetchTestSource(stub.srv.URL)
	fetchOnce(t, src, locale.Settings{})
	if len(src.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1 (регресс v1.5.2)", len(src.Nodes))
	}

	// Группа поверх дублей: оба члена схлопнуты в одного выжившего.
	body := `{"outbounds": [
	  {"type": "vless", "tag": "dup-a", "server": "x.example", "server_port": 443, "uuid": "11111111-1111-4111-8111-111111111111", "tls": {"enabled": true, "server_name": "x.example"}},
	  {"type": "vless", "tag": "dup-b", "server": "x.example", "server_port": 443, "uuid": "11111111-1111-4111-8111-111111111111", "tls": {"enabled": true, "server_name": "x.example"}},
	  {"type": "selector", "tag": "pick", "outbounds": ["dup-a", "dup-b"]}
	]}`
	stub.set(b64Body(body), http.StatusOK)
	fetchOnce(t, src, locale.Settings{})
	var group *state.Node
	for i := range src.Nodes {
		if src.Nodes[i].Kind == state.SourceKindAuto {
			group = &src.Nodes[i]
		}
	}
	if group == nil {
		t.Fatalf("группа не материализована: %s", nodeTags(src))
	}
	if len(group.Group.Members) != 1 || group.Group.Members[0].Tag != "dup-a" {
		t.Fatalf("члены не перепривязаны на выжившего: %+v", group.Group.Members)
	}
}

// §4.D.3 — дубли тегов провайдера: X, X-2, X → оба живут (X, X-2, X-3).
func TestFetchUniquifiesDuplicateRawTags(t *testing.T) {
	body := strings.Join([]string{vlessLine(1, "X"), vlessLine(2, "X-2"), vlessLine(3, "X")}, "\n")
	stub := newStubSub(t, body)
	src := newFetchTestSource(stub.srv.URL)
	fetchOnce(t, src, locale.Settings{})
	if nodeTags(src) != "X,X-2,X-3" {
		t.Fatalf("теги: %s", nodeTags(src))
	}
}

// §4.D.4 — кап max_nodes реально останавливает разбор; truncated=true;
// резолв: настройка подписки → дефолт настроек приложения.
func TestFetchMaxNodesCap(t *testing.T) {
	var lines []string
	for i := 0; i < 5; i++ {
		lines = append(lines, vlessLine(i, fmt.Sprintf("N-%d", i)))
	}
	stub := newStubSub(t, strings.Join(lines, "\n"))

	src := newFetchTestSource(stub.srv.URL)
	src.MaxNodes = 2
	fetchOnce(t, src, locale.Settings{})
	if len(src.Nodes) != 2 {
		t.Fatalf("кап подписки: nodes = %d, want 2", len(src.Nodes))
	}
	if src.UpdateStatus == nil || !src.UpdateStatus.Truncated {
		t.Fatalf("truncated не выставлен: %+v", src.UpdateStatus)
	}
	if src.Meta == nil || !src.Meta.Truncated {
		t.Fatal("мостовая Meta.Truncated не выставлена")
	}

	// Ступень 2: дефолт настроек приложения.
	src2 := newFetchTestSource(stub.srv.URL)
	fetchOnce(t, src2, locale.Settings{DefaultSubscriptionMaxNodes: 1})
	if len(src2.Nodes) != 1 {
		t.Fatalf("кап настроек приложения: nodes = %d, want 1", len(src2.Nodes))
	}

	// Без капа: аварийный потолок-константа не мешает малым телам.
	src3 := newFetchTestSource(stub.srv.URL)
	fetchOnce(t, src3, locale.Settings{})
	if len(src3.Nodes) != 5 || src3.UpdateStatus.Truncated {
		t.Fatalf("без капа: nodes = %d, truncated = %v", len(src3.Nodes), src3.UpdateStatus.Truncated)
	}
}

// §4.D.5 — merge: совпавший тег сохранил enabled=false и detour при свежем
// body; новый — включён; исчезнувший — удалён.
func TestFetchMergePreservesUserMarks(t *testing.T) {
	stub := newStubSub(t, strings.Join([]string{vlessLine(1, "A"), vlessLine(2, "B"), vlessLine(3, "C")}, "\n"))
	src := newFetchTestSource(stub.srv.URL)
	fetchOnce(t, src, locale.Settings{})

	// Пользовательские пометки.
	src.Nodes[0].Detour = &state.NodeLink{Tag: "uplink"}
	src.Nodes[1].Enabled = false

	// Провайдер: C исчез, D появился, A/B живут.
	stub.set(strings.Join([]string{vlessLine(1, "A"), vlessLine(2, "B"), vlessLine(4, "D")}, "\n"), http.StatusOK)
	fetchOnce(t, src, locale.Settings{})

	if nodeTags(src) != "A,B,D" {
		t.Fatalf("состав после merge: %s", nodeTags(src))
	}
	if src.Nodes[0].Detour == nil || src.Nodes[0].Detour.Tag != "uplink" {
		t.Fatalf("detour не пережил fetch: %+v", src.Nodes[0].Detour)
	}
	if src.Nodes[1].Enabled {
		t.Fatal("enabled=false не пережил fetch")
	}
	if !src.Nodes[2].Enabled {
		t.Fatal("новый узел обязан быть включённым")
	}
}

// §4.D.6 — 113-A: ошибка сети / пустое тело / мусорное тело → nodes[]
// неизменны, ошибка в updateStatus; truncated-разбор не удаляет.
func TestFetch113AGuardsNodes(t *testing.T) {
	stub := newStubSub(t, strings.Join([]string{vlessLine(1, "A"), vlessLine(2, "B")}, "\n"))
	src := newFetchTestSource(stub.srv.URL)
	fetchOnce(t, src, locale.Settings{})
	successAt := src.UpdateStatus.LastSuccessAt

	// Ошибка сети (HTTP 500).
	stub.set("", http.StatusInternalServerError)
	fetchOnce(t, src, locale.Settings{})
	if nodeTags(src) != "A,B" {
		t.Fatalf("HTTP-ошибка тронула nodes: %s", nodeTags(src))
	}
	if src.UpdateStatus.LastStatus != "err" || src.UpdateStatus.ErrorCount != 1 {
		t.Fatalf("updateStatus после ошибки: %+v", src.UpdateStatus)
	}
	if src.UpdateStatus.LastSuccessAt != successAt {
		t.Fatal("память об успехе потеряна")
	}

	// Пустое тело (200 + 0 байт).
	stub.set("", http.StatusOK)
	fetchOnce(t, src, locale.Settings{})
	if nodeTags(src) != "A,B" {
		t.Fatalf("пустое тело тронуло nodes: %s", nodeTags(src))
	}
	if src.UpdateStatus.ErrorCount != 2 {
		t.Fatalf("ErrorCount = %d, want 2", src.UpdateStatus.ErrorCount)
	}

	// Мусорное тело (HTML вместо подписки): записи деградировали, узлов
	// ноль — разбор недостоверен, nodes[] не трогаются.
	stub.set("<html><body>Access denied battle://page</body></html>", http.StatusOK)
	fetchOnce(t, src, locale.Settings{})
	if nodeTags(src) != "A,B" {
		t.Fatalf("мусорное тело тронуло nodes: %s", nodeTags(src))
	}
	if src.UpdateStatus.LastStatus != "err" {
		t.Fatalf("мусорное тело не помечено ошибкой: %+v", src.UpdateStatus)
	}

	// Truncated: удаления нет, обновление и добавление есть.
	stub.set(strings.Join([]string{vlessLine(5, "E"), vlessLine(6, "F"), vlessLine(7, "G")}, "\n"), http.StatusOK)
	src.MaxNodes = 2
	fetchOnce(t, src, locale.Settings{})
	if nodeTags(src) != "E,F,A,B" {
		t.Fatalf("truncated-merge: %s", nodeTags(src))
	}
}

// §4.D.8 — body материализованного узла чист от detour (и tag) при
// наличии общего detour у источника.
func TestFetchBodyFreeOfDetour(t *testing.T) {
	stub := newStubSub(t, vlessLine(1, "A"))
	src := newFetchTestSource(stub.srv.URL)
	src.Detour = &state.NodeLink{Tag: "uplink"} // общий detour подписки
	fetchOnce(t, src, locale.Settings{})
	if len(src.Nodes) != 1 {
		t.Fatalf("nodes: %s", nodeTags(src))
	}
	var body map[string]interface{}
	if err := json.Unmarshal(src.Nodes[0].Body, &body); err != nil {
		t.Fatalf("body не JSON: %v", err)
	}
	if _, has := body["detour"]; has {
		t.Fatal("body несёт ключ detour — запекание умерло в v7")
	}
	if _, has := body["tag"]; has {
		t.Fatal("body несёт ключ tag — тег живёт в Node.Tag")
	}
}

// §4.D.9 — Auto: члены-NodeLink на узлы своей подписки; вложенная группа —
// потеря с warning; selector сохраняет type и default.
func TestFetchAutoMaterialization(t *testing.T) {
	body := `{"outbounds": [
	  {"type": "vless", "tag": "srv-a", "server": "a.example", "server_port": 443, "uuid": "11111111-1111-4111-8111-111111111111", "tls": {"enabled": true, "server_name": "a.example"}},
	  {"type": "vless", "tag": "srv-b", "server": "b.example", "server_port": 443, "uuid": "22222222-2222-4222-8222-222222222222", "tls": {"enabled": true, "server_name": "b.example"}},
	  {"type": "selector", "tag": "pick", "outbounds": ["srv-a", "srv-b"], "default": "srv-b"},
	  {"type": "urltest", "tag": "outer", "outbounds": ["pick", "srv-a"]}
	]}`
	stub := newStubSub(t, b64Body(body))
	src := newFetchTestSource(stub.srv.URL)
	fetchOnce(t, src, locale.Settings{})

	var pick, outer *state.Node
	for i := range src.Nodes {
		switch src.Nodes[i].Tag {
		case "pick":
			pick = &src.Nodes[i]
		case "outer":
			outer = &src.Nodes[i]
		}
	}
	if pick == nil || pick.Kind != state.SourceKindAuto || pick.Group == nil {
		t.Fatalf("selector не материализован: %s", nodeTags(src))
	}
	if pick.Group.GroupType != state.AutoGroupSelector || pick.Group.Default != "srv-b" {
		t.Fatalf("selector потерял тип/default: %+v", pick.Group)
	}
	for _, m := range pick.Group.Members {
		if m.FolderID != src.ID {
			t.Fatalf("член группы не ссылается на свою подписку: %+v", m)
		}
	}
	// Вложенная группа-член теряется с warning, остальные члены живут.
	if outer == nil || len(outer.Group.Members) != 1 || outer.Group.Members[0].Tag != "srv-a" {
		t.Fatalf("вложенный член не отсечён: %+v", outer)
	}
	foundWarn := false
	for _, w := range src.UpdateStatus.Warnings {
		if strings.Contains(w.Message, "outer") && strings.Contains(w.Message, "not resolvable") {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Fatalf("потеря вложенной группы обязана быть warning'ом: %+v", src.UpdateStatus.Warnings)
	}
}

// Вердикт O2 — pending_disabled применяется на первом достоверном fetch и
// стирается; несматченный тег даёт merge-warning в updateStatus.
func TestFetchAppliesPendingDisabled(t *testing.T) {
	stub := newStubSub(t, strings.Join([]string{vlessLine(1, "A"), vlessLine(2, "B")}, "\n"))
	src := newFetchTestSource(stub.srv.URL)
	src.PendingDisabled = []string{"B", "ghost"}
	fetchOnce(t, src, locale.Settings{})

	if src.Nodes[1].Enabled {
		t.Fatal("pending-отметка не применилась")
	}
	if src.PendingDisabled != nil {
		t.Fatalf("pending_disabled не стёрт: %v", src.PendingDisabled)
	}
	foundWarn := false
	for _, w := range src.UpdateStatus.Warnings {
		if w.Kind == "merge" && strings.Contains(w.Message, "ghost") {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Fatalf("несматченная отметка обязана дать warning: %+v", src.UpdateStatus.Warnings)
	}
	// Согласованность моста: карта выключенных отражает канон.
	if _, ok := src.DisabledNodes["B"]; !ok {
		t.Fatalf("мостовая карта не отражает enabled=false: %v", src.DisabledNodes)
	}
}

// Фикс ревью W3 (блокер 1б, fetch-уровень): отметка выключения, записанная
// ТОЛЬКО в легаси-карту DisabledNodes (старый state.json, легаси-путь UI),
// втягивается в канон и переживает fetch — узел не «оживает».
func TestFetchLegacyDisabledMapSurvivesFetch(t *testing.T) {
	stub := newStubSub(t, strings.Join([]string{vlessLine(1, "A"), vlessLine(2, "B")}, "\n"))
	src := newFetchTestSource(stub.srv.URL)
	fetchOnce(t, src, locale.Settings{})

	// Легаси-путь: только карта, канонический enabled не тронут.
	src.DisabledNodes = map[string]int64{"B": 7}

	fetchOnce(t, src, locale.Settings{})
	if src.Nodes[1].Enabled {
		t.Fatal("легаси-отметка карты не втянулась в канон — узел ожил после fetch")
	}
	if src.DisabledNodes["B"] != 7 {
		t.Fatalf("timestamp легаси-отметки потерян: %v", src.DisabledNodes)
	}
}

// Фикс ревью W3 (среднее): недостоверное тело не трогает ни nodes[], ни .raw
// (симметрия 113-A на мостовую эпоху) — иначе легаси-путь сборки терял бы
// узлы, которые канон сохранил.
func TestFetchUntrustedBodyKeepsRawCache(t *testing.T) {
	stub := newStubSub(t, vlessLine(1, "A"))
	src := newFetchTestSource(stub.srv.URL)
	subsDir := t.TempDir()
	if !refreshOneSubscriptionSource(src, state.Defaults{}, locale.Settings{}, subsDir) {
		t.Fatal("первый fetch ничего не изменил")
	}
	rawBefore, err := state.ReadRawBody(subsDir, src.ID)
	if err != nil || len(rawBefore) == 0 {
		t.Fatalf(".raw после успешного fetch: err=%v, %d байт", err, len(rawBefore))
	}

	// Мусорное тело (HTML вместо подписки) → недостоверно.
	stub.set("<html><body>Access denied battle://page</body></html>", http.StatusOK)
	if !refreshOneSubscriptionSource(src, state.Defaults{}, locale.Settings{}, subsDir) {
		t.Fatal("недостоверный fetch обязан изменить updateStatus")
	}
	if nodeTags(src) != "A" {
		t.Fatalf("недостоверное тело тронуло nodes: %s", nodeTags(src))
	}
	rawAfter, err := state.ReadRawBody(subsDir, src.ID)
	if err != nil {
		t.Fatalf("чтение .raw после недостоверного fetch: %v", err)
	}
	if !bytes.Equal(rawBefore, rawAfter) {
		t.Fatal(".raw затёрт недостоверным телом — мостовая сборка потеряла бы узлы")
	}
}

// §4.D.7 (fetch-уровень) — подписка, ни разу не сфетченная: nodes[] пуст,
// успехов нет; неудачная попытка фиксируется в updateStatus и не рождает
// узлов (warning в отчёте сборки — читатель updateStatus, волна W4).
func TestFetchNeverSucceededLeavesNodesEmpty(t *testing.T) {
	stub := newStubSub(t, "")
	stub.set("", http.StatusForbidden)
	src := newFetchTestSource(stub.srv.URL)
	fetchOnce(t, src, locale.Settings{})
	if len(src.Nodes) != 0 {
		t.Fatalf("узлы из ниоткуда: %s", nodeTags(src))
	}
	if src.UpdateStatus == nil || src.UpdateStatus.LastStatus != "err" || src.UpdateStatus.LastSuccessAt != "" {
		t.Fatalf("updateStatus: %+v", src.UpdateStatus)
	}
}
