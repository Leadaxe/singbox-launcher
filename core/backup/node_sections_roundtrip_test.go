package backup

// Секции узла в бэкапе (SPEC 121 §10.5, NODE_SECTIONS.md §5, SPEC 127 §6.2).
//
// Секции едут в файле ТОЛЬКО в формате 1.0 (норма ONE_NAMESPACE §4): в 0.12
// схема обещала одну форму записей, а писатель клал другую (ловушка CODEMAP
// §7.6), и волна 2 закрыла расхождение снятием секций из 0.12, а не подгонкой
// формы; сам писатель 0.12 снят в v1.6.0 (D-110). Поэтому сценарии разнесены
// по входам:
//
//	1.0  — поле пишется, узловые правила в rules[] не уезжают, круг
//	       «экспорт → Parse → импорт» восстанавливает записи и позиции,
//	       файл замещает локальные секции, файл без поля их не трогает;
//	0.12 на чтение — файл, написанный прежней сборкой, открывается как
//	       прежде: такие файлы уже у пользователей на руках.
//
// Плюс два требования волны 2: чужой `kind` внутри секций отбрасывается с
// кодом, а корень и узлы стоят на ОДНОЙ оси порядка номерами файла.

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

const sectionsNodeURI = "trojan://pw@1.2.3.4:443#ts-node"

// stateWithSections — состояние с одним корневым узлом, несущим секции.
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

// exportParseImport10 гоняет состояние через ПОЛНЫЙ круг файла 1.0:
// Export10 → Marshal → Parse → Import. Круг, а не проверка одной стороны:
// потеря на любой из границ даёт один и тот же симптом — узел приезжает без
// своей связки, и обнаруживается это не раньше, чем перестанет работать
// маршрут, ради которого секции заводились.
func exportParseImport10(t *testing.T, src, dst *state.State) (*state.State, []Warning) {
	t.Helper()
	b, _, err := Export10(src, ExportOptions{AppVersion: "test", Platform: "darwin"})
	if err != nil {
		t.Fatalf("Export10: %v", err)
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
		if w.Code == WarnBackupUnknownField {
			t.Errorf("scanUnknown ругается на свой же файл 1.0: %s", w)
		}
	}

	res, err := ImportFile(dst, parsed, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return dst, append(warns, res.Warnings...)
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

func TestBackupNodeSectionsFormat10(t *testing.T) {
	// 1. Экспорт 1.0 пишет секции ЗАПИСЯМИ СОСТОЯНИЯ — с `kind`, `enabled`,
	//    `num` и `body` (одна форма на состояние, файл и LxBox).
	src := stateWithSections(t, "ts-dns", 945)
	b, _, err := Export10(src, ExportOptions{AppVersion: "test", Platform: "darwin"})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	if len(b.Sources) != 1 || b.Sources[0].Sections == nil {
		t.Fatal("экспорт 1.0 не записал sources[].sections")
	}
	raw, err := json.Marshal(b.Sources[0].Sections)
	if err != nil {
		t.Fatalf("marshal sections: %v", err)
	}
	for _, want := range []string{`"kind":"inline"`, `"num":945`, `"kind":"user"`, `"body":`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("секции 1.0 без %s: %s", want, raw)
		}
	}

	// 2. Импорт в ПУСТОЕ состояние восстанавливает записи и их позиции.
	fresh := &state.State{}
	exportParseImport10(t, src, fresh)

	sec := nodeSectionsOf(t, fresh)
	if sec.IsEmpty() {
		t.Fatal("импорт в пустое состояние не восстановил секции")
	}
	if len(sec.DNSServers()) != 1 || len(sec.DNSRules()) != 1 || len(sec.Rules) != 1 {
		t.Errorf("состав секций разошёлся: servers=%d rules=%d route=%d",
			len(sec.DNSServers()), len(sec.DNSRules()), len(sec.Rules))
	}
	if ep, _ := sec.DNSServers()[0].Body["endpoint"].(string); ep != state.SelfPlaceholder {
		t.Errorf("плейсхолдер %s не пережил круг — связка потеряла привязку к узлу (endpoint=%q)",
			state.SelfPlaceholder, ep)
	}

	// 3. Импорт ПОВЕРХ состояния с тем же узлом: секции файла замещают
	//    локальные (тело то же, узел тот же — BACKUP.md §9 п. 2).
	local := stateWithSections(t, "local-dns", 1000)
	exportParseImport10(t, src, local)
	sec = nodeSectionsOf(t, local)
	if len(sec.DNSServers()) != 1 || sec.DNSServers()[0].Tag != "ts-dns" {
		t.Errorf("секции файла не заместили локальные: %v", sec.DNSServers())
	}

	// 4. Файл БЕЗ поля локальные секции не трогает: молчание файла не значит
	//    «сотри».
	plain := stateWithSections(t, "keep-me", 945)
	noSections := stateWithSections(t, "unused", 945)
	noSections.Sources[0].Node.Sections = nil
	exportParseImport10(t, noSections, plain)
	sec = nodeSectionsOf(t, plain)
	if sec.IsEmpty() || sec.DNSServers()[0].Tag != "keep-me" {
		t.Errorf("файл без sections снёс локальные секции: %v", sec)
	}

	// 5. Экспорт — СНИМОК, а не окно в живые данные: правка состояния после
	//    экспорта не обязана менять уже собранный файл.
	snapshot := stateWithSections(t, "ts-dns", 945)
	file, _, err := Export10(snapshot, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	snapshot.Sources[0].Node.Sections.Rules[0].Name = "changed after export"
	if got := file.Sources[0].Sections.Rules[0].Name; strings.Contains(got, "changed") {
		t.Errorf("файл делит память с состоянием: правка состояния изменила файл (%q)", got)
	}
}

// Файл 0.12 С секциями (написанный прежней сборкой) читается как прежде.
//
// Такие файлы уже у пользователей на руках (П3): снять писателя можно, снять
// читателя — нет.
func TestBackupNodeSectionsLegacyFileStillRead(t *testing.T) {
	legacy := []byte(`{
	  "lx_backup": 1,
	  "exported_by": {"app": "launcher", "version": "1.5.9"},
	  "exported_at": "2026-09-10T00:00:00Z",
	  "servers": [{
	    "id": "01J00000000000000000000SRV",
	    "node_tag": "ts-node",
	    "uri": "` + sectionsNodeURI + `",
	    "enabled": true,
	    "sections": {
	      "rules": [{"kind":"inline","name":"@{self} network","enabled":true,"num":945,
	                 "body":{"ip_cidr":["100.64.0.0/10"],"outbound":"@self"}}],
	      "dns": {
	        "servers": [{"kind":"user","tag":"ts-dns","enabled":true,
	                     "body":{"type":"tailscale","endpoint":"@self"}}],
	        "rules":   [{"kind":"user","enabled":true,
	                     "body":{"domain_suffix":[".ts.net"],"server":"ts-dns"}}]
	      }
	    }
	  }]
	}`)
	parsed, parseWarns, err := Parse(legacy)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, w := range parseWarns {
		if w.Code == WarnBackupUnknownField && strings.Contains(w.Detail, "sections") {
			t.Errorf("scanUnknown ругается на объявленное поле 0.12: %s", w)
		}
	}
	dst := &state.State{}
	if _, err := ImportFile(dst, parsed, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	sec := nodeSectionsOf(t, dst)
	if sec.IsEmpty() || len(sec.Rules) != 1 || len(sec.DNSServers()) != 1 || len(sec.DNSRules()) != 1 {
		t.Fatalf("секции файла 0.12 не прочитаны: %+v", sec)
	}
	if sec.DNSServers()[0].Tag != "ts-dns" {
		t.Errorf("тег DNS-сервера секции потерян: %+v", sec.DNSServers()[0])
	}

	// Файл 0.12 БЕЗ поля локальные секции не трогает — та же норма, что у 1.0.
	plain := stateWithSections(t, "keep-me", 945)
	noSections, _, err := Parse([]byte(`{
	  "lx_backup": 1,
	  "exported_by": {"app": "launcher", "version": "1.5.9"},
	  "exported_at": "2026-09-10T00:00:00Z",
	  "servers": [{"id":"01J00000000000000000000SRV","node_tag":"ts-node",
	               "uri":"` + sectionsNodeURI + `","enabled":true}]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := ImportFile(plain, noSections, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if sec := nodeSectionsOf(t, plain); sec.IsEmpty() || sec.DNSServers()[0].Tag != "keep-me" {
		t.Errorf("файл без sections снёс локальные секции: %v", sec)
	}
}

// W2.5: запись ЧУЖОГО ВИДА внутри секций отбрасывается с кодом, а остальные
// записи узла живут.
//
// Проверяются оба входа: у 0.12 секции приезжают непрозрачным блоком, у 1.0 —
// записями состояния, и отсев обязан быть один и тот же. Раньше отсев делал
// dropForeignKinds в состоянии и писал только в WarnLog — пользователь,
// принёсший файл, о потере не узнавал вовсе.
func TestBackupSectionForeignKindDropped(t *testing.T) {
	body := `{
	      "rules": [
	        {"kind":"preset","ref":"traffic-processing","enabled":true},
	        {"kind":"inline","name":"keep","enabled":true,"num":945,
	         "body":{"ip_cidr":["100.64.0.0/10"],"outbound":"@self"}}
	      ],
	      "dns": {
	        "servers": [{"kind":"template","tag":"google_dot","enabled":true},
	                    {"kind":"user","tag":"ts-dns","enabled":true,
	                     "body":{"type":"tailscale","endpoint":"@self"}}],
	        "rules":   [{"kind":"preset","ref":"russian","enabled":true}]
	      }
	    }`

	check := func(t *testing.T, name string, raw []byte) {
		t.Helper()
		parsed, _, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: Parse: %v", name, err)
		}
		dst := &state.State{}
		res, err := ImportFile(dst, parsed, ImportOptions{})
		if err != nil {
			t.Fatalf("%s: Import: %v", name, err)
		}
		dropped := 0
		for _, w := range res.Warnings {
			if w.Code == WarnBackupSectionRecordDropped {
				dropped++
				if !strings.Contains(w.Detail, "ts-node") {
					t.Errorf("%s: предупреждение не называет узел: %q", name, w.Detail)
				}
			}
		}
		// Три чужие записи — три предупреждения: правило preset, DNS-сервер
		// template и DNS-правило preset.
		if dropped != 3 {
			t.Errorf("%s: предупреждений о чужих записях %d, ожидалось 3: %v", name, dropped, res.Warnings)
		}
		sec := nodeSectionsOf(t, dst)
		if sec == nil || len(sec.Rules) != 1 || sec.Rules[0].Name != "keep" {
			t.Fatalf("%s: свои записи не пережили отсев: %+v", name, sec)
		}
		if len(sec.DNSServers()) != 1 || sec.DNSServers()[0].Tag != "ts-dns" {
			t.Errorf("%s: user-сервер секции потерян: %+v", name, sec.DNSServers())
		}
		if len(sec.DNSRules()) != 0 {
			t.Errorf("%s: preset-правило DNS не отброшено: %+v", name, sec.DNSRules())
		}
	}

	check(t, "0.12", []byte(`{
	  "lx_backup": 1,
	  "exported_by": {"app": "launcher", "version": "1.5.9"},
	  "exported_at": "2026-09-10T00:00:00Z",
	  "servers": [{"id":"01J00000000000000000000SRV","node_tag":"ts-node",
	               "uri":"`+sectionsNodeURI+`","enabled":true,
	               "sections": `+body+`}]
	}`))

	check(t, "1.0", []byte(`{
	  "lx_backup": 2,
	  "exported_by": {"app": "launcher", "version": "1.6.0"},
	  "exported_at": "2026-09-14T00:00:00Z",
	  "sources": [{"kind":"server","id":"01J00000000000000000000SRV","tag":"ts-node",
	               "enabled":true,
	               "origin":{"kind":"uri","raw":"`+sectionsNodeURI+`"},
	               "sections": `+body+`}]
	}`))
}

// TestBackupSectionOnForeignNodeKindNamed — поле `sections` у узла, которому
// оно НЕ ПОЛОЖЕНО, снимается с тем же кодом, а не молча.
//
// Секции носит только одиночный сервер (NODE_SECTIONS.md §1): у подписки
// секция принадлежала бы кэшу провайдера, у цепочки и провайдерской группы —
// ссылочной сущности, которой нечего инъецировать. Отсев по ВИДУ ЗАПИСИ делал
// импорт и называл потерю кодом, а отсев по ВИДУ УЗЛА — состояние, и молча:
// пользователь, принёсший файл, о второй потере не узнавал вовсе, хотя реестр
// backup_warnings.json обещает код именно на этот случай.
func TestBackupSectionOnForeignNodeKindNamed(t *testing.T) {
	const sections = `{"rules":[{"kind":"inline","name":"r","enabled":true,
	                             "body":{"domain":"x.com","outbound":"@self"}}]}`

	for _, tc := range []struct{ name, source string }{
		{"chain", `{"kind":"chain","id":"01J0000000000000000000CHN","tag":"ch",
		            "enabled":true,"sections":` + sections + `}`},
		{"subscription", `{"kind":"subscription","id":"01J0000000000000000000SUB",
		                   "enabled":true,"url":"https://example.invalid/s",
		                   "sections":` + sections + `}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(`{
			  "lx_backup": 2,
			  "exported_by": {"app": "launcher", "version": "1.6.0"},
			  "exported_at": "2026-09-14T00:00:00Z",
			  "sources": [` + tc.source + `]
			}`)
			parsed, _, err := Parse(raw)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			dst := &state.State{}
			res, err := ImportFile(dst, parsed, ImportOptions{})
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			if len(dst.Sources) != 1 {
				t.Fatalf("источников %d, ожидался 1", len(dst.Sources))
			}
			if dst.Sources[0].Node.Sections != nil {
				t.Errorf("секции осели на узле вида %s: %+v", tc.name, dst.Sources[0].Node.Sections)
			}
			if !hasWarn(res.Warnings, WarnBackupSectionRecordDropped) {
				t.Errorf("потеря поля sections у узла вида %s не названа: %v", tc.name, res.Warnings)
			}
		})
	}
}

// W2.5: корень и секции приехавших узлов стоят на ОДНОЙ оси
// (NODE_SECTIONS.md §5).
//
// Когда импорт перенумеровывал только s.Rules, узловые правила сохраняли
// номера файла и пересекались с перенумерованными (SPEC 126 L2): два правила с
// одним номером встают в конфиг в порядке, который зависит от того, кто
// попался сборке первым. Теперь номера файла держат оба вида — пересечений,
// которых не было в файле, взяться неоткуда.
func TestBackupImportAxisWithNodeSections(t *testing.T) {
	src := stateWithSections(t, "ts-dns", 945) // правило узла стоит ПЕРВЫМ на оси
	src.Rules = []state.Rule{
		mkInlineRule("second", "direct", 2000),
		mkInlineRule("third", "direct", 3000),
	}
	dst := &state.State{}
	exportParseImport10(t, src, dst)

	if len(dst.Rules) != 2 {
		t.Fatalf("корневых правил после импорта: %d", len(dst.Rules))
	}
	sec := nodeSectionsOf(t, dst)
	if sec == nil || len(sec.Rules) != 1 || sec.Rules[0].Num == nil {
		t.Fatalf("правило узла потеряло позицию: %+v", sec)
	}

	// Номера файла: правило узла на 945 — ПЕРВЫМ на оси, корневые за ним.
	nodeNum := *sec.Rules[0].Num
	if nodeNum != 945 {
		t.Errorf("правило узла ушло со своего места: num=%d, в файле 945", nodeNum)
	}
	for i, want := range []int{2000, 3000} {
		if dst.Rules[i].Num == nil || *dst.Rules[i].Num != want {
			t.Errorf("корневое правило %q: num=%v, ожидалось %d",
				dst.Rules[i].Name, dst.Rules[i].Num, want)
		}
	}
	// Пересечения быть не должно ни при каких обстоятельствах: номер,
	// занятый узловым правилом, не может повториться в корне.
	for _, r := range dst.Rules {
		if r.Num != nil && *r.Num == nodeNum {
			t.Errorf("номер %d занят и узловым, и корневым правилом %q", nodeNum, r.Name)
		}
	}
}

// Пустой набор в файле ЗАМЕЩАЕТ локальные секции, а отсутствие поля — нет.
//
// Разница нормативна (BACKUP.md §9 п. 2) и легко теряется при рефакторинге:
// пустой набор нормализуется в nil, и если смотреть на «есть ли записи», а не
// на «было ли поле», то снятие секций на другой машине сюда не доедет — узел
// молча оставит себе старую связку.
func TestBackupEmptySectionsReplaceLocalOnes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		file  string
		empty bool // ожидаем ли, что локальные секции сняты
	}{
		{
			name: "0.12 пустой набор",
			file: `{"lx_backup":1,"exported_by":{"app":"launcher","version":"1.5.9"},
			        "exported_at":"2026-09-10T00:00:00Z",
			        "servers":[{"node_tag":"ts-node","uri":"` + sectionsNodeURI + `",
			                    "enabled":true,"sections":{}}]}`,
			empty: true,
		},
		{
			name: "1.0 пустой набор",
			file: `{"lx_backup":2,"exported_by":{"app":"launcher","version":"1.6.0"},
			        "exported_at":"2026-09-14T00:00:00Z",
			        "sources":[{"kind":"server","tag":"ts-node","enabled":true,
			                    "origin":{"kind":"uri","raw":"` + sectionsNodeURI + `"},
			                    "sections":{}}]}`,
			empty: true,
		},
		{
			name: "1.0 без поля",
			file: `{"lx_backup":2,"exported_by":{"app":"launcher","version":"1.6.0"},
			        "exported_at":"2026-09-14T00:00:00Z",
			        "sources":[{"kind":"server","tag":"ts-node","enabled":true,
			                    "origin":{"kind":"uri","raw":"` + sectionsNodeURI + `"}}]}`,
			empty: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local := stateWithSections(t, "keep-me", 945)
			parsed, _, err := Parse([]byte(tc.file))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if _, err := ImportFile(local, parsed, ImportOptions{}); err != nil {
				t.Fatalf("Import: %v", err)
			}
			sec := nodeSectionsOf(t, local)
			if tc.empty && !sec.IsEmpty() {
				t.Errorf("пустой набор файла не снял локальные секции: %+v", sec)
			}
			if !tc.empty && sec.IsEmpty() {
				t.Errorf("файл без поля снёс локальные секции")
			}
		})
	}
}
