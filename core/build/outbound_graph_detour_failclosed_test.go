package build

import (
	"encoding/json"
	"testing"
)

// Правило 1 санитайзера — недоступная цель detour (контракт 1.1.171, D-148,
// решение владельца 16.11.2026: «если detour не доступен собираем без него и
// даем ⚠»). Ключ detour снимается, узел остаётся и ходит напрямую, потеря
// идёт в отчёт записью detour_unavailable — одной на пару источник × цель.
// С SPEC 113-B до 1.1.171 здесь выбрасывался носитель (fail-closed); кольца
// через detour по-прежнему выбрасывают носителя (правило 5).

// sanitizeWithOrigins — как sanitizeHelper, но с картой происхождения узлов:
// без неё санитайзер знает лишь тег и не может назвать источник.
func sanitizeWithOrigins(t *testing.T, outbounds []string, origins map[string]NodeOrigin, templateTags ...string) (map[string]map[string]interface{}, graphLosses) {
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
	out, losses := sanitizeOutboundGraph(cache, finalTags)
	got := make(map[string]map[string]interface{})
	for _, raw := range out.Outbounds {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("emitted entry is not JSON: %s", raw)
		}
		got[m["tag"].(string)] = m
	}
	return got, losses
}

// Source-detour на селектор, которого в финальном конфиге нет (выключили
// мульти-VPN — «vpn ②» исчез из шаблона). Узлы подписки остаются без хопа,
// источник назван одной записью вместе с целью и узлами.
func TestSanitizeSourceDetourOntoMissingSelectorStripsHop(t *testing.T) {
	origins := map[string]NodeOrigin{
		"Proton NL 1": {SourceID: "01SUB", SourceLabel: "Proton NL"},
		"Proton NL 2": {SourceID: "01SUB", SourceLabel: "Proton NL"},
	}
	got, losses := sanitizeWithOrigins(t, []string{
		`{"tag":"Proton NL 1","type":"vless","server":"a","detour":"vpn ②"}`,
		`{"tag":"Proton NL 2","type":"vless","server":"b","detour":"vpn ②"}`,
		`{"tag":"other","type":"vless","server":"c"}`,
	}, origins, "vpn ①")

	for _, tag := range []string{"Proton NL 1", "Proton NL 2", "other"} {
		m, ok := got[tag]
		if !ok {
			t.Errorf("узел %q выпал, а должен остаться", tag)
			continue
		}
		if _, has := m["detour"]; has {
			t.Errorf("у %q остался detour на недоступную цель: %v", tag, m["detour"])
		}
	}
	if len(losses.excluded) != 0 {
		t.Errorf("недоступная цель выбросила узлы: %+v", losses.excluded)
	}
	if len(losses.stripped) != 1 {
		t.Fatalf("записей о снятом хопе %d, ожидалась одна на пару источник × цель: %+v", len(losses.stripped), losses.stripped)
	}
	st := losses.stripped[0]
	if st.SourceID != "01SUB" || st.SourceLabel != "Proton NL" || st.Target != "vpn ②" || len(st.Nodes) != 2 {
		t.Errorf("запись о снятом хопе не опознаёт источник, цель или узлы: %+v", st)
	}
}

// Ручной узел шаблона без источника: хоп снимается так же, а записи нет —
// привязать её не к чему (предупреждение уходит в лог).
func TestSanitizeManualNodeDetourOntoMissingTagStripsHop(t *testing.T) {
	got, losses := sanitizeWithOrigins(t, []string{
		`{"tag":"manual","type":"vless","server":"a","detour":"ghost-out"}`,
		`{"tag":"healthy","type":"vless","server":"b","detour":"direct-out"}`,
	}, nil, "direct-out")

	m, ok := got["manual"]
	if !ok {
		t.Fatal("узел с висячим detour выпал, а должен остаться")
	}
	if _, has := m["detour"]; has {
		t.Errorf("висячий detour не снят: %v", m["detour"])
	}
	if d, _ := got["healthy"]["detour"].(string); d != "direct-out" {
		t.Errorf("рабочий detour снят или изменён: %q", d)
	}
	if len(losses.stripped) != 0 || len(losses.excluded) != 0 {
		t.Errorf("узел без источника дал запись: %+v", losses)
	}
}

// Каскада нет: n2 теряет хоп на «ghost», n1 ходит через живой n2 и свой
// detour сохраняет.
func TestSanitizeStrippedHopDoesNotCascade(t *testing.T) {
	got, _ := sanitizeWithOrigins(t, []string{
		`{"tag":"n1","type":"vless","server":"a","detour":"n2"}`,
		`{"tag":"n2","type":"vless","server":"b","detour":"ghost"}`,
		`{"tag":"n3","type":"vless","server":"c","detour":"direct-out"}`,
	}, nil, "direct-out")

	if len(got) != 3 {
		t.Fatalf("узлы выпали: %v", got)
	}
	if _, has := got["n2"]["detour"]; has {
		t.Error("висячий detour у n2 не снят")
	}
	if d, _ := got["n1"]["detour"].(string); d != "n2" {
		t.Errorf("n1 потерял detour на живой n2: %q", d)
	}
	if d, _ := got["n3"]["detour"].(string); d != "direct-out" {
		t.Errorf("здоровый detour изменён: %q", d)
	}
}
