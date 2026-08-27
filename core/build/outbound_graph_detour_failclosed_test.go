package build

import (
	"encoding/json"
	"strings"
	"testing"
)

// SPEC 113-B — единая строгость для целей detour на последнем рубеже.
// Правило 1 санитайзера больше НЕ снимает detour: снятие ключа означало бы,
// что узел, настроенный ходить через переход, молча пошёл напрямую. Носитель
// перехода выбрасывается, источник попадает в реестр исключений.
//
// Прежнее поведение (fail-open, «dropping detour (direct dial)») закрывал
// TestDropDanglingNodeDetours — он удалён вместе с самой функцией.

// sanitizeWithOrigins — как sanitizeHelper, но с картой происхождения узлов:
// без неё санитайзер знает лишь тег и не может назвать источник.
func sanitizeWithOrigins(t *testing.T, outbounds []string, origins map[string]NodeOrigin, templateTags ...string) (map[string]map[string]interface{}, []SourceExclusion) {
	t.Helper()
	cache := &ParsedCache{NodeOrigins: origins}
	finalTags := make(map[string]bool)
	for _, tt := range templateTags {
		finalTags[tt] = true
	}
	for _, o := range outbounds {
		cache.Outbounds = append(cache.Outbounds, json.RawMessage(o))
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(o), &m); err != nil {
			t.Fatalf("bad fixture: %v", err)
		}
		finalTags[m["tag"].(string)] = true
	}
	out, excluded := sanitizeOutboundGraph(cache, finalTags)
	got := make(map[string]map[string]interface{})
	for _, raw := range out.Outbounds {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("emitted entry is not JSON: %s", raw)
		}
		got[m["tag"].(string)] = m
	}
	return got, excluded
}

// Тест 4 (SPEC-B): source-detour на селектор, которого в финальном конфиге нет
// (выключили мульти-VPN — «vpn ②» исчез из шаблона). Раньше правило 1 снимало
// detour, и все узлы подписки молча ехали напрямую. Теперь узлы выпадают, а
// источник называется в реестре исключений вместе с целью.
func TestSanitizeSourceDetourOntoMissingSelectorExcludesSource(t *testing.T) {
	origins := map[string]NodeOrigin{
		"Proton NL 1": {SourceID: "01SUB", SourceLabel: "Proton NL"},
		"Proton NL 2": {SourceID: "01SUB", SourceLabel: "Proton NL"},
	}
	got, excluded := sanitizeWithOrigins(t, []string{
		`{"tag":"Proton NL 1","type":"vless","server":"a","detour":"vpn ②"}`,
		`{"tag":"Proton NL 2","type":"vless","server":"b","detour":"vpn ②"}`,
		`{"tag":"other","type":"vless","server":"c"}`,
	}, origins, "vpn ①")

	for _, tag := range []string{"Proton NL 1", "Proton NL 2"} {
		if m, ok := got[tag]; ok {
			t.Errorf("узел %q с недоступной целью detour обязан выпасть, а он остался: %v", tag, m)
		}
	}
	if _, ok := got["other"]; !ok {
		t.Error("узел без detour выпадать не должен")
	}
	if len(excluded) != 1 {
		t.Fatalf("в реестре %d записей, ожидалась одна на источник: %+v", len(excluded), excluded)
	}
	if excluded[0].SourceID != "01SUB" || excluded[0].SourceLabel != "Proton NL" {
		t.Errorf("запись не опознаёт источник: %+v", excluded[0])
	}
	if !strings.Contains(excluded[0].Reason, "vpn ②") {
		t.Errorf("причина %q не называет цель перехода — чинить нечего", excluded[0].Reason)
	}
}

// Тест 5 (SPEC-B): node-detour из ручного config_json на тег, которого нет.
// Скипается сам УЗЕЛ (гранулярность = носитель), а не подписка: источника у
// такого узла может не быть вовсе, и реестр он тогда не наполняет.
func TestSanitizeManualNodeDetourOntoMissingTagSkipsNode(t *testing.T) {
	got, excluded := sanitizeWithOrigins(t, []string{
		`{"tag":"manual","type":"vless","server":"a","detour":"ghost-out"}`,
		`{"tag":"healthy","type":"vless","server":"b","detour":"direct-out"}`,
	}, nil, "direct-out")

	if m, ok := got["manual"]; ok {
		t.Errorf("узел с висячим detour обязан выпасть, а он остался: %v", m)
	}
	h, ok := got["healthy"]
	if !ok {
		t.Fatal("узел с разрешающимся detour выпал")
	}
	if d, _ := h["detour"].(string); d != "direct-out" {
		t.Errorf("рабочий detour снят или изменён: %q", d)
	}
	// Источника у ручного узла нет — привязать пометку не к чему.
	if len(excluded) != 0 {
		t.Errorf("узел без источника наполнил реестр: %+v", excluded)
	}
}

// Санитайзер не имеет права «спасать» переход снятием ключа ни в одном
// сценарии: после прохода ни у одного выжившего узла detour не изменился.
func TestSanitizeNeverStripsDetour(t *testing.T) {
	got, _ := sanitizeWithOrigins(t, []string{
		`{"tag":"n1","type":"vless","server":"a","detour":"n2"}`,
		`{"tag":"n2","type":"vless","server":"b","detour":"ghost"}`,
		`{"tag":"n3","type":"vless","server":"c","detour":"direct-out"}`,
	}, nil, "direct-out")

	for tag, m := range got {
		if _, hadDetour := m["detour"]; !hadDetour {
			t.Errorf("у выжившего узла %q detour снят — это тихий прямой дозвон", tag)
		}
	}
	// n2 выпал за висячий detour, следом n1 — его цель исчезла из конфига.
	if _, ok := got["n2"]; ok {
		t.Error("носитель висячего detour остался")
	}
	if _, ok := got["n1"]; ok {
		t.Error("каскад не дошёл до узла, чья цель выпала")
	}
	if _, ok := got["n3"]; !ok {
		t.Error("здоровый узел выпал")
	}
}
