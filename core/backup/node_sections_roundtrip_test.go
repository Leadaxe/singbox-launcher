package backup

// Секции узла в бэкапе (SPEC 121 §8 п. 5): экспорт → Parse → импорт.
//
// Один тест на все пять требований пункта: поле пишется вместе с rule_num,
// записи kind=node в rules[] нет, импорт в пустое состояние восстанавливает и
// секции, и позицию якоря, импорт поверх того же узла замещает локальные
// секции, файл без поля их не трогает, и scanUnknown на новое поле не ругается.

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

const sectionsNodeURI = "trojan://pw@1.2.3.4:443#ts-node"

// stateWithSections — состояние с одним корневым узлом, несущим секции, и
// якорем его правил на оси.
func stateWithSections(t *testing.T, dnsServerTag string, anchorNum int) *state.State {
	t.Helper()
	body, err := json.Marshal(state.NodeRuleBody{Tag: "ts-node"})
	if err != nil {
		t.Fatalf("кодирование тела якоря: %v", err)
	}
	num := anchorNum
	return &state.State{
		Sources: []state.Source{{
			ID: "01J00000000000000000000SRV",
			Node: state.Node{
				Kind:    state.SourceKindServer,
				Tag:     "ts-node",
				Enabled: true,
				Origin:  &state.Origin{Kind: state.OriginKindURI, Raw: sectionsNodeURI},
				Sections: &state.NodeSections{
					DNSServers: []json.RawMessage{
						json.RawMessage(`{"type":"tailscale","tag":"` + dnsServerTag + `","endpoint":"@self"}`),
					},
					DNSRules: []json.RawMessage{
						json.RawMessage(`{"domain_suffix":[".ts.net"],"server":"` + dnsServerTag + `"}`),
					},
					Rules: []json.RawMessage{
						json.RawMessage(`{"ip_cidr":["100.64.0.0/10"],"outbound":"@self"}`),
					},
				},
			},
		}},
		Rules: []state.Rule{{
			Kind:     state.RuleKindNode,
			Enabled:  true,
			OrderNum: &num,
			Body:     body,
		}},
	}
}

// exportParseImport гоняет состояние через полный круг файла: Export →
// Marshal → Parse (с обходом неизвестных ключей) → Import.
func exportParseImport(t *testing.T, src, dst *state.State) (*state.State, []Warning) {
	t.Helper()
	b, _, err := Export(src, ExportOptions{AppVersion: "test", Platform: "darwin"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Записи kind=node в rules[] быть не должно ни при каких условиях: якорь
	// производный от узла (SPEC §3.3).
	for _, r := range b.Rules {
		if r.Kind == RuleKind(state.RuleKindNode) {
			t.Fatal("в rules[] бэкапа появилась запись kind=node — она производная от узла")
		}
	}

	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal backup: %v", err)
	}

	parsed, warns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, w := range warns {
		if w.Code == WarnBackupUnknownField && strings.Contains(w.Detail, "sections") {
			t.Errorf("scanUnknown ругается на объявленное поле: %s", w)
		}
	}

	if _, err := Import(dst, parsed, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	return dst, warns
}

// nodeSectionsOf — секции единственного корневого сервера состояния.
func nodeSectionsOf(t *testing.T, s *state.State) *state.NodeSections {
	t.Helper()
	for i := range s.Sources {
		if s.Sources[i].Kind == state.SourceKindServer {
			return s.Sources[i].Node.Sections
		}
	}
	t.Fatal("в состоянии нет корневого сервера")
	return nil
}

// nodeAnchorOf — якорь kind=node состояния (первый).
func nodeAnchorOf(s *state.State) *state.Rule {
	for i := range s.Rules {
		if s.Rules[i].Kind == state.RuleKindNode {
			return &s.Rules[i]
		}
	}
	return nil
}

func TestBackupNodeSectionsRoundTrip(t *testing.T) {
	// 1. Экспорт пишет поле вместе с rule_num.
	src := stateWithSections(t, "ts-dns", 945)
	b, _, err := Export(src, ExportOptions{AppVersion: "test", Platform: "darwin"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(b.Servers) != 1 || b.Servers[0].Sections == nil {
		t.Fatal("экспорт не записал servers[].sections")
	}
	if b.Servers[0].Sections.RuleNum == nil || int(*b.Servers[0].Sections.RuleNum) != 945 {
		t.Errorf("rule_num не записан или разошёлся: %v", b.Servers[0].Sections.RuleNum)
	}

	// 2. Импорт в ПУСТОЕ состояние восстанавливает секции и позицию якоря.
	fresh := &state.State{}
	exportParseImport(t, src, fresh)

	sec := nodeSectionsOf(t, fresh)
	if sec.IsEmpty() {
		t.Fatal("импорт в пустое состояние не восстановил секции")
	}
	if len(sec.DNSServers) != 1 || len(sec.DNSRules) != 1 || len(sec.Rules) != 1 {
		t.Errorf("состав секций разошёлся: servers=%d rules=%d route=%d",
			len(sec.DNSServers), len(sec.DNSRules), len(sec.Rules))
	}
	if !strings.Contains(string(sec.DNSServers[0]), "@self") {
		t.Error("плейсхолдер @self не пережил круг — связка потеряла привязку к узлу")
	}
	anchor := nodeAnchorOf(fresh)
	if anchor == nil {
		t.Fatal("якорь kind=node не пересеян после импорта")
	}
	if anchor.OrderNum == nil {
		t.Error("якорь приехал без позиции на оси")
	}

	// 3. Импорт ПОВЕРХ состояния с тем же узлом: секции файла замещают
	//    локальные (тело то же, узел тот же — CODEMAP §10 п. 29).
	local := stateWithSections(t, "local-dns", 1000)
	exportParseImport(t, src, local)
	sec = nodeSectionsOf(t, local)
	if len(sec.DNSServers) != 1 || !strings.Contains(string(sec.DNSServers[0]), `"ts-dns"`) {
		t.Errorf("секции файла не заместили локальные: %s", sec.DNSServers)
	}

	// 4. Файл БЕЗ поля локальные секции не трогает.
	plain := stateWithSections(t, "keep-me", 945)
	// Тот же узел, но экспортируемое состояние секций не несёт.
	noSections := stateWithSections(t, "unused", 945)
	noSections.Sources[0].Node.Sections = nil
	noSections.Rules = nil
	exportParseImport(t, noSections, plain)
	sec = nodeSectionsOf(t, plain)
	if sec.IsEmpty() || !strings.Contains(string(sec.DNSServers[0]), `"keep-me"`) {
		t.Errorf("файл без sections снёс локальные секции: %v", sec)
	}
}
