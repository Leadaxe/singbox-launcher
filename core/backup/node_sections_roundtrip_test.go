package backup

// Секции узла в бэкапе (SPEC 121 §8 п. 5 в форме §10.5): экспорт → Parse →
// импорт.
//
// Один тест на все пять требований пункта: поле пишется, записи узловых правил
// в rules[] нет (их дом — секции узла), импорт в пустое состояние
// восстанавливает и записи, и их позиции на оси, импорт поверх того же узла
// замещает локальные секции, файл без поля их не трогает, и scanUnknown на
// новое поле не ругается.
//
// Второй тест — чтение СТАРОЙ формы (волны 1–2): файл, написанный прежней
// сборкой, обязан открыться без потери эмитируемого конфига.

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

const sectionsNodeURI = "trojan://pw@1.2.3.4:443#ts-node"

// stateWithSections — состояние с одним корневым узлом, несущим секции в
// форме хранения.
func stateWithSections(t *testing.T, dnsServerTag string, ruleNum int) *state.State {
	t.Helper()
	sectionRule := state.NewInlineRule(
		state.SelfPlaceholderBraced+" network",
		map[string]interface{}{"ip_cidr": []interface{}{"100.64.0.0/10"}},
		state.SelfPlaceholder,
	)
	num := ruleNum
	sectionRule.Enabled = true
	sectionRule.Num = &num
	sections := &state.NodeSections{Rules: []state.Rule{sectionRule}}
	sections.SetDNS(
		[]state.DNSServer{{
			Kind:    state.DNSServerKindUser,
			Tag:     dnsServerTag,
			Enabled: true,
			Body:    map[string]interface{}{"type": "tailscale", "endpoint": state.SelfPlaceholder},
		}},
		[]state.DNSRule{{
			Kind:    state.DNSRuleKindUser,
			Enabled: true,
			Body: map[string]interface{}{
				"domain_suffix": []interface{}{".ts.net"},
				"server":        dnsServerTag,
			},
		}},
	)
	return &state.State{
		Sources: []state.Source{{
			ID: "01J00000000000000000000SRV",
			Node: state.Node{
				Kind:     state.SourceKindServer,
				Tag:      "ts-node",
				Enabled:  true,
				Origin:   &state.Origin{Kind: state.OriginKindURI, Raw: sectionsNodeURI},
				Sections: sections,
			},
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

	// Правила узла в rules[] бэкапа не едут ни при каких условиях: их дом —
	// секции узла (SPEC 121 §10.5).
	for _, r := range b.Rules {
		if strings.Contains(r.Name, "network") {
			t.Fatalf("правило узла уехало в rules[] бэкапа: %q", r.Name)
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

func TestBackupNodeSectionsRoundTrip(t *testing.T) {
	// 1. Экспорт пишет поле в форме хранения — с `kind`, `enabled` и
	//    `num` у записей (state v8; до v8 номер назывался `order_num`).
	src := stateWithSections(t, "ts-dns", 945)
	b, _, err := Export(src, ExportOptions{AppVersion: "test", Platform: "darwin"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(b.Servers) != 1 || b.Servers[0].Sections == nil {
		t.Fatal("экспорт не записал servers[].sections")
	}
	written := string(b.Servers[0].Sections.Raw)
	for _, want := range []string{`"kind":"inline"`, `"num":945`, `"kind":"user"`} {
		if !strings.Contains(written, want) {
			t.Errorf("экспортированные секции без %s: %s", want, written)
		}
	}
	// 2. Импорт в ПУСТОЕ состояние восстанавливает записи и их позиции.
	fresh := &state.State{}
	exportParseImport(t, src, fresh)

	sec := nodeSectionsOf(t, fresh)
	if sec.IsEmpty() {
		t.Fatal("импорт в пустое состояние не восстановил секции")
	}
	if len(sec.DNSServers()) != 1 || len(sec.DNSRules()) != 1 || len(sec.Rules) != 1 {
		t.Errorf("состав секций разошёлся: servers=%d rules=%d route=%d",
			len(sec.DNSServers()), len(sec.DNSRules()), len(sec.Rules))
	}
	if sec.Rules[0].Num == nil || *sec.Rules[0].Num != 945 {
		t.Errorf("позиция правила узла не пережила круг: %v", sec.Rules[0].Num)
	}
	if ep, _ := sec.DNSServers()[0].Body["endpoint"].(string); ep != state.SelfPlaceholder {
		t.Errorf("плейсхолдер %s не пережил круг — связка потеряла привязку к узлу (endpoint=%q)",
			state.SelfPlaceholder, ep)
	}

	// 3. Импорт ПОВЕРХ состояния с тем же узлом: секции файла замещают
	//    локальные (тело то же, узел тот же — CODEMAP §10 п. 29).
	local := stateWithSections(t, "local-dns", 1000)
	exportParseImport(t, src, local)
	sec = nodeSectionsOf(t, local)
	if len(sec.DNSServers()) != 1 || sec.DNSServers()[0].Tag != "ts-dns" {
		t.Errorf("секции файла не заместили локальные: %v", sec.DNSServers())
	}

	// 4. Файл БЕЗ поля локальные секции не трогает.
	plain := stateWithSections(t, "keep-me", 945)
	noSections := stateWithSections(t, "unused", 945)
	noSections.Sources[0].Node.Sections = nil
	exportParseImport(t, noSections, plain)
	sec = nodeSectionsOf(t, plain)
	if sec.IsEmpty() || sec.DNSServers()[0].Tag != "keep-me" {
		t.Errorf("файл без sections снёс локальные секции: %v", sec)
	}
}
