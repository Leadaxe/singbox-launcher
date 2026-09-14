package backup

// Тесты чистоты экспорта и roundtrip'а сущностей (SPEC 114, П1).
//
// Здесь проверяется не «поле доехало», а само свойство формата: файл — это
// СЕРИАЛИЗАЦИЯ СОСТОЯНИЯ и ничего кроме. Пока в формате жил механизм
// extensions, оба свойства были ложны: экспорт зависел от того, что принесла
// прошлая загрузка, а состояние помнило чужой груз.

import (
	"bytes"
	"encoding/json"
	"reflect"
	"regexp"
	"testing"
	"time"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// fixedExport — экспорт с прибитым моментом времени: exported_at обязан быть
// одинаковым, иначе сравнение байтов проверяло бы часы, а не чистоту.
func fixedExport(t *testing.T, s *state.State) []byte {
	t.Helper()
	b, _, err := Export012(s, ExportOptions{
		AppVersion: "test", Platform: "darwin", Now: time.Unix(1750000000, 0),
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// richState — состояние со ВСЕМИ сущностями формата разом. Нужно именно
// такое: чистоту легко удержать на пустом состоянии и легко потерять на
// поле, которое обходится по map'у или дописывается «на провоз».
func richState() *state.State {
	auto := true
	stripOff := false
	s := &state.State{}
	s.Directions = []configtypes.Direction{
		{Tag: "vpn-de", Type: "selector", AddOutbounds: []string{"direct-out"}},
	}
	s.Sources = []state.Source{
		{
			ID: "01SUB0000000000000000000",
			Node: state.Node{
				Kind: state.SourceKindSubscription, Enabled: true,
				Detour: &state.NodeLink{FolderID: "01SRV0000000000000000000", Tag: "🔥 WARP"},
			},
			URL: "https://example-1.com/sub", Name: "Main", MaxNodes: 200,
			TagPolicy:       &state.TagPolicy{Prefix: "[A] "},
			Update:          &state.UpdateSpec{IntervalHours: 12, AutoRefresh: &auto},
			PendingDisabled: []string{"node-a", "node-b"},
			Skip:            []map[string]string{{"field": "tag", "contains": "trial"}},
			Replace:         &state.FolderReplace{Mode: state.FolderReplaceManual, Tag: "1:select"},
		},
		{
			ID: "01SRV0000000000000000000",
			Node: state.Node{
				Kind: state.SourceKindServer, Enabled: true, Tag: "🔥 WARP",
				Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "vless://11111111-1111-1111-1111-111111111111@example-2.com:443?type=tcp#s"},
				Detour: &state.NodeLink{Tag: "hop-1"},
			},
		},
		{
			ID: "01CHN0000000000000000000",
			Node: state.Node{
				Kind: state.SourceKindChain, Enabled: true, Tag: "relay",
				Body: configtypes.ChainBody(&configtypes.SourceChain{
					IdleTimeout:  "0s",
					StripEvasion: &stripOff,
					Strip:        map[string]bool{"tls.utls": false},
					Rewrite:      map[string]interface{}{"vless": map[string]interface{}{"flow": nil}},
				}),
				Hops: []state.NodeLink{{Tag: "vpn-de"}, {Tag: "🔥 WARP"}},
			},
		},
	}
	// Номера — из своей же зоны оси (UserRuleNumStart и дальше): состояние,
	// настроенное руками в лаунчере, уже размечено ею. Взять произвольные
	// числа значило бы мерить нормализацию оси, а не чистоту экспорта.
	s.Rules = []state.Rule{
		mkPresetRule("traffic-processing", state.UserRuleNumStart, true),
		mkInlineRule("Work", "vpn-de", state.UserRuleNumStart+1),
		mkInlineRule("Chained", "relay", state.UserRuleNumStart+2),
	}
	s.Vars = []state.SettingVar{
		{Name: "log_level", Value: "debug"},
		{Name: "tun_interface", Value: "utun9"},
		{Name: "route_final", Value: "vpn-de"},
	}
	s.DNS.Final = "dns_shield"
	s.DNS.Strategy = "ipv4_only"
	s.DNS.Servers = []state.DNSServer{
		{Kind: state.DNSServerKindTemplate, Tag: "google_dot", Enabled: true},
		{Kind: state.DNSServerKindUser, Tag: "my_dns", Enabled: true,
			Body: map[string]interface{}{"type": "udp", "server": "10.0.0.1"}},
	}
	s.DNS.Rules = []state.DNSRule{
		{Kind: state.DNSRuleKindUser, Enabled: false,
			Body: map[string]interface{}{"domain_suffix": "example.com", "server": "my_dns"}},
	}
	s.WarpAccounts = &state.WarpAccountsSection{
		WG: &state.WarpWGAccount{PrivateKey: "priv", PeerPublic: "pub", ClientV4: "172.16.0.2"},
	}
	return s
}

// importKnowsEverything — принимающая сторона знает все цели богатого
// состояния: иначе правила приехали бы выключенными по постороннему поводу и
// сравнение экспортов измеряло бы не чистоту, а полноту опций.
func importKnowsEverything() ImportOptions {
	return ImportOptions{
		KnownOutbounds: []string{"vpn-de", "relay", "hop-1", "🔥 WARP"},
		KnownPresets:   []string{"traffic-processing"},
	}
}

// П1: экспорт — чистая функция состояния. Два экспорта одного состояния
// обязаны быть БАЙТ-идентичны: любая примесь «откуда взялось» делает файл
// зависимым от истории, а не от настройки.
func TestExportIsPureFunctionOfState(t *testing.T) {
	s := richState()
	first := fixedExport(t, s)
	second := fixedExport(t, s)
	if string(first) != string(second) {
		t.Fatalf("два экспорта одного состояния разошлись:\n--- 1 ---\n%s\n--- 2 ---\n%s", first, second)
	}
}

// П1: состояние после импорта неотличимо от настроенного руками — значит и
// экспорт из него обязан совпасть байт в байт с исходным файлом. Это и есть
// «нет теневых полей»: карман бы здесь всплыл лишней записью.
func TestExportIndependentOfImportOrigin(t *testing.T) {
	handMade := richState()
	want := fixedExport(t, handMade)

	b, _, err := Parse(want)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	imported := &state.State{}
	if _, err := ImportFile(imported, b, importKnowsEverything()); err != nil {
		t.Fatalf("Import: %v", err)
	}
	got := fixedExport(t, imported)
	if string(got) != string(want) {
		t.Fatalf("экспорт зависит от того, импортировано состояние или настроено руками:\n--- руками ---\n%s\n--- после импорта ---\n%s", want, got)
	}
}

// П1 на всех сущностях сразу: export → import → export байт-идентичен.
//
// Тест-ловушка C1 (SPEC 114). На старом коде он падал на ЦЕПОЧКЕ С id:
// applyLauncherSourceExtensions присваивал src.ID = own.ID безусловно, а у
// цепочки блоб extensions.launcher нёс только id — и та же функция затирала
// NodeTag пустой строкой, уводя тег цепочки в Label. Тег — идентичность
// (П5): на него ссылаются rules[].outbound, route.final и позиции других
// цепочек, поэтому «тег жив» проверяется здесь отдельным утверждением, а не
// только через равенство байтов.
func TestRoundTripAllEntitiesByteIdentical(t *testing.T) {
	s := richState()
	first := fixedExport(t, s)

	b, warns, err := Parse(first)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("свой же файл вызвал предупреждения: %v", warns)
	}
	restored := &state.State{}
	if _, err := ImportFile(restored, b, importKnowsEverything()); err != nil {
		t.Fatalf("Import: %v", err)
	}

	var chain *state.Source
	for i := range restored.Sources {
		if restored.Sources[i].Kind == state.SourceKindChain {
			chain = &restored.Sources[i]
		}
	}
	if chain == nil {
		t.Fatal("цепочка потеряна на roundtrip")
	}
	if chain.Tag != "relay" {
		t.Errorf("тег цепочки после импорта %q, ожидался %q — ссылки правил и позиций разъедутся", chain.Tag, "relay")
	}
	// Source.Label — поле `json:"-"`: подпись, положенная импортом, умерла бы
	// на первом Save. Имя цепочки в v7 одно — тег, и он проверен выше.
	if chain.Label != "" {
		t.Errorf("импорт заполнил Label (json:\"-\") — подпись умрёт на первом Save: %q", chain.Label)
	}
	if chain.ID != "01CHN0000000000000000000" {
		t.Errorf("id цепочки потерян: %q", chain.ID)
	}

	second := fixedExport(t, restored)
	if string(second) != string(first) {
		t.Fatalf("roundtrip не тождественен:\n--- до ---\n%s\n--- после ---\n%s", first, second)
	}
}

// ── формат 1.0 ─────────────────────────────────────────────────────

// fixedExport10 — экспорт 1.0 с прибитым моментом времени.
func fixedExport10(t *testing.T, s *state.State) []byte {
	t.Helper()
	b, _, err := Export10(s, ExportOptions{
		AppVersion: "test", Platform: "darwin", Now: time.Unix(1750000000, 0),
	})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// richState10 — богатое состояние ПЛЮС то, что выражает только формат 1.0:
// папка с настройками и составом трёх видов, узел с секциями, цепочка с
// адресным хопом в папку, подписка с identity и disabled.
//
// Отдельная функция, а не правка richState: richState — вход 0.12-писателя, и
// дописать туда папку значило бы поменять эталоны прежнего формата.
func richState10() *state.State {
	s := richState()
	send := false
	s.Sources[0].SetIdentity("Happ/1.0", "7c9e6679-7425-40de-944b-e07fc1f90ae7", &send, nil)
	s.Sources[0].PendingDisabled = []string{"node-a", "node-b"}
	// Настройка вида лаунчера, у которой в 1.0 ПОЯВИЛСЯ дом (§6.0): в 0.12
	// она была local-only, и слияние её не применяло по этой причине.
	s.Sources[0].RelaysInDirections = true

	// Узел с секциями: связка, которую 0.12 выразить не мог вовсе.
	sectionRule := state.NewInlineRule(
		state.SelfPlaceholderBraced+" network",
		map[string]interface{}{"ip_cidr": []interface{}{"100.64.0.0/10"}},
		state.SelfPlaceholder,
	)
	sectionRule.Enabled = true
	sectionNum := state.NodeRuleDefaultNum
	sectionRule.Num = &sectionNum
	sections := &state.NodeSections{Rules: []state.Rule{sectionRule}}
	sections.SetDNS(
		[]state.DNSServer{{
			Kind: state.DNSServerKindUser, Tag: "ts-dns", Enabled: true,
			Body: map[string]interface{}{"type": "tailscale", "endpoint": state.SelfPlaceholder},
		}},
		[]state.DNSRule{{
			Kind: state.DNSRuleKindUser, Enabled: true,
			Body: map[string]interface{}{"domain_suffix": []interface{}{".ts.net"}, "server": "ts-dns"},
		}},
	)
	s.Sources = append(s.Sources, state.Source{
		ID: "01TS00000000000000000000",
		Node: state.Node{
			Kind: state.SourceKindServer, Enabled: true, Tag: "ts-node",
			Origin:   &state.Origin{Kind: state.OriginKindURI, Raw: "trojan://pw@1.2.3.4:443#ts-node"},
			Sections: sections,
		},
	})

	// Папка с политикой тегов, общим detour, СВЁРТКОЙ и составом трёх видов.
	//
	// Свёртка у папки — та самая, ради которой 1.0 завёл ей дом (§6.0), и её
	// тег ЯВНЫЙ, не совпадающий с позиционным деривативом подписок: пока имя
	// выводилось формулой, «Proton-select» молча превращался в «1:select»,
	// причём в тот же тег, что у первой свёрнутой подписки.
	s.Sources = append(s.Sources, state.Source{
		ID:        "01FLD0000000000000000000",
		Name:      "Proton",
		TagPolicy: &state.TagPolicy{Prefix: "[P] "},
		Replace: &state.FolderReplace{
			Mode: state.FolderReplaceManual, Tag: "Proton-select",
		},
		Node: state.Node{
			Kind: state.SourceKindFolder, Enabled: true,
			Detour: &state.NodeLink{Tag: "🔥 WARP"},
		},
		Nodes: []state.Node{
			{
				Kind: state.SourceKindServer, Enabled: true, Tag: "Amsterdam",
				Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "vless://22222222-2222-2222-2222-222222222222@ams.example:443?type=tcp#ams"},
			},
			{
				Kind: state.SourceKindServer, Enabled: false, Tag: "Zurich",
				Body: json.RawMessage(`{"type":"trojan","server":"zrh.example","server_port":443}`),
			},
			{
				Kind: state.SourceKindAuto, Enabled: true, Tag: "auto-eu",
				Group: &state.AutoGroup{
					GroupType: state.AutoGroupURLTest,
					Members:   []state.NodeLink{{Tag: "Amsterdam"}, {Tag: "Zurich"}},
				},
			},
		},
	})

	// Цепочка с АДРЕСНЫМ хопом в папку: ровно та ссылка, которую 0.12 не нёс
	// и которую импорт 1.0 обязан переписать по карте id.
	s.Sources = append(s.Sources, state.Source{
		ID: "01CHN1000000000000000000",
		Node: state.Node{
			Kind: state.SourceKindChain, Enabled: true, Tag: "via-proton",
			Hops: []state.NodeLink{{FolderID: "01FLD0000000000000000000", Tag: "Amsterdam"}},
		},
	})

	// Правила ВСЕХ трёх видов и обе формы цели (W2.7 п. 1). У richState есть
	// preset и inline с тегом; здесь добавляются srs с набором и два правила,
	// у которых цель выражена `action` — самостоятельным эффектом (sniff) и
	// отказом (reject). Цель в теле — самая тонкая часть формы v8: она едет
	// не полем записи, а ключом sing-box внутри body, и потеряться может
	// молча.
	srs := state.NewSrsRule("Ads", []string{
		"https://example.com/ads.srs", "https://example.com/track.srs",
	}, "reject")
	srs.Enabled = true
	srsNum := state.UserRuleNumStart + 3
	srs.Num = &srsNum

	sniff := state.NewInlineRule("Sniff", map[string]interface{}{
		"inbound": "tun-in",
		"action":  "sniff",
	}, "")
	sniff.Enabled = true
	sniffNum := state.UserRuleNumStart + 4
	sniff.Num = &sniffNum

	reject := state.NewInlineRule("Block ads",
		map[string]interface{}{"domain_suffix": []interface{}{"ads.example"}}, "drop")
	reject.Enabled = false
	rejectNum := state.UserRuleNumStart + 5
	reject.Num = &rejectNum

	s.Rules = append(s.Rules, srs, sniff, reject)

	// DNS: к template- и user-записям richState добавляются ССЫЛОЧНЫЕ
	// (kind=preset) сервер и правило — у них тела нет вовсе, и писатель,
	// требующий body, вырезал бы их из файла.
	s.DNS.Servers = append(s.DNS.Servers, state.DNSServer{
		Kind: state.DNSServerKindPreset, Ref: "ru-inside", Enabled: true,
	})
	s.DNS.Rules = append(s.DNS.Rules, state.DNSRule{
		Kind: state.DNSRuleKindPreset, Ref: "ru-inside", Enabled: true,
	})
	// Третий скаляр секции dns (§6.0). Он писался в файл с самого начала, но
	// читателя у него не было — круг терял его молча, потому что фикстура
	// его не задавала.
	s.DNS.DefaultDomainResolver = "local"
	return s
}

func importKnowsEverything10() ImportOptions {
	opts := importKnowsEverything()
	opts.KnownOutbounds = append(opts.KnownOutbounds, "ts-node", "via-proton", "[P] Amsterdam")
	return opts
}

// П1 для 1.0: экспорт — чистая функция состояния, и файл не делит с ним
// память. Второе проверяется отдельно: общий указатель не видно в байтах
// первого экспорта — он проявится позже, когда состояние поправят.
func TestExport10IsPureFunctionOfState(t *testing.T) {
	s := richState10()
	first := fixedExport10(t, s)
	second := fixedExport10(t, s)
	if string(first) != string(second) {
		t.Fatalf("два экспорта одного состояния разошлись:\n--- 1 ---\n%s\n--- 2 ---\n%s", first, second)
	}

	// Четыре места, где легко оставить указатель в живые данные: тело узла,
	// состав папки, секции и записи правил.
	file, _, err := Export10(s, ExportOptions{AppVersion: "test", Platform: "darwin", Now: time.Unix(1750000000, 0)})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	before := string(fixedExport10(t, s))
	s.Rules[1].Name = "changed"
	for i := range s.Sources {
		if s.Sources[i].Kind == state.SourceKindFolder && len(s.Sources[i].Nodes) > 0 {
			s.Sources[i].Nodes[0].Tag = "changed"
		}
		if s.Sources[i].Node.Sections != nil && len(s.Sources[i].Node.Sections.Rules) > 0 {
			s.Sources[i].Node.Sections.Rules[0].Name = "changed"
		}
		if len(s.Sources[i].Node.Body) > 0 {
			s.Sources[i].Node.Body[0] = ' '
		}
	}
	after, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(after) != before {
		t.Errorf("файл делит память с состоянием: правка состояния после экспорта изменила файл\n--- до ---\n%s\n--- после ---\n%s", before, after)
	}
}

// Состав файла 1.0 (§6.0): что обязано быть и чего быть не должно.
func TestExport10FileShape(t *testing.T) {
	raw := fixedExport10(t, richState10())
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, _ := doc["lx_backup"].(float64); int(v) != FormatVersion10 {
		t.Errorf("lx_backup = %v, ожидалось %d", doc["lx_backup"], FormatVersion10)
	}
	for _, key := range []string{"sources", "rules", "dns", "vars", "route", "warp"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("в файле нет секции %q", key)
		}
	}
	// Плоских секций 0.12 в файле 1.0 быть не может: у него одна sources[].
	for _, key := range []string{"subscriptions", "servers", "chains"} {
		if _, ok := doc[key]; ok {
			t.Errorf("в файле 1.0 секция 0.12 %q", key)
		}
	}
	sources, _ := doc["sources"].([]any)
	if len(sources) == 0 {
		t.Fatal("sources[] пуст")
	}
	for _, item := range sources {
		rec, _ := item.(map[string]any)
		for _, forbidden := range []string{"meta", "update_status", "pending_disabled", "replace", "label"} {
			if _, ok := rec[forbidden]; ok {
				t.Errorf("в записи источника ключ %q, которого в файле быть не должно: %v", forbidden, rec)
			}
		}
		if rec["kind"] == "subscription" {
			if _, ok := rec["nodes"]; ok {
				t.Errorf("кэш узлов подписки уехал в файл: %v", rec["nodes"])
			}
			if _, ok := rec["identity"].(map[string]any); !ok {
				t.Errorf("identity не объектом: %v", rec["identity"])
			}
			if _, ok := rec["disabled"].(map[string]any); !ok {
				t.Errorf("отметки выключенных узлов не поехали: %v", rec["disabled"])
			}
		}
		if rec["kind"] == "folder" {
			if _, ok := rec["nodes"].([]any); !ok {
				t.Errorf("состав папки не поехал: %v", rec)
			}
			if _, ok := rec["tag_policy"]; !ok {
				t.Errorf("настройки папки не поехали: %v", rec)
			}
		}
	}
	// Правила и DNS — записи СОСТОЯНИЯ: `body` вместо разобранных match /
	// outbound, `refs` вместо ref, тело DNS в `body`, а не в `value`.
	rules, _ := doc["rules"].([]any)
	if len(rules) == 0 {
		t.Fatal("rules[] пуст")
	}
	for _, item := range rules {
		rec, _ := item.(map[string]any)
		for _, forbidden := range []string{"match", "outbound", "value"} {
			if _, ok := rec[forbidden]; ok {
				t.Errorf("в записи правила ключ формы 0.12 %q: %v", forbidden, rec)
			}
		}
	}
	dns, _ := doc["dns"].(map[string]any)
	servers, _ := dns["servers"].([]any)
	for _, item := range servers {
		rec, _ := item.(map[string]any)
		if _, ok := rec["value"]; ok {
			t.Errorf("тело DNS-сервера в `value` (форма 0.12): %v", rec)
		}
		if _, ok := rec["name"]; ok {
			t.Errorf("тег DNS-сервера в `name` (форма 0.12): %v", rec)
		}
	}
}

// Поле, добавленное в state.Source, обязано появиться в файле 1.0 либо быть
// объявлено исключением с причиной.
//
// Рефлексией, а не списком: список разъехался бы с состоянием на первом же
// новом поле — и оно молча не доехало бы до второй машины.
func TestSource10CoversStateSourceKeys(t *testing.T) {
	stateKeys := jsonKeysOf(reflect.TypeOf(state.Source{}))
	fileKeys := jsonKeysOf(reflect.TypeOf(Source10{}))
	for k := range stateKeys {
		if fileKeys[k] {
			continue
		}
		if why, ok := source10ExcludedStateKeys[k]; ok {
			if why == "" {
				t.Errorf("ключ %q исключён без причины", k)
			}
			continue
		}
		t.Errorf("поле состояния %q не едет в файле 1.0 и не объявлено исключением "+
			"(добавить в Source10 либо в source10ExcludedStateKeys с причиной)", k)
	}
	// И наоборот: исключение, которого в состоянии уже нет, — мусор в списке.
	for k := range source10ExcludedStateKeys {
		if !stateKeys[k] {
			t.Errorf("исключение %q: такого поля в state.Source больше нет", k)
		}
	}
}

// jsonKeysOf — json-имена полей структуры, включая встроенные (state.Source
// встраивает state.Node, и его ключи — такие же ключи записи).
func jsonKeysOf(t reflect.Type) map[string]bool {
	return jsonKeys(t)
}

// W2.7 п. 1 — ГЛАВНЫЙ инвариант волны: круг «экспорт 1.0 → Parse → импорт в
// пустое состояние → экспорт 1.0» БАЙТ-ИДЕНТИЧЕН.
//
// Байты, а не сравнение полей: сравнение полей проверяет то, о чём тест
// вспомнил, а байты — всё сразу. Ни одно поле §6.0 не может потеряться молча:
// identity, skip, tag_policy, fold, disabled, hops с folder_id, detour,
// секции всех трёх списков, num, enabled, vars, refs.
//
// ПОЧЕМУ КРУГ СЧИТАЕТСЯ СО ВТОРОГО ЭКСПОРТА. Импорт перенумеровывает ось
// порядка — это норма, а не потеря: «абсолютные номера у сторон свои, важен
// относительный порядок» (BACKUP.md §9 п. 7, NODE_SECTIONS.md §5). Правило
// узла, стоявшее на 945 (перед якорем шаблона), при слиянии оси встаёт в
// пользовательскую зону вместе с корневыми, и первый круг обязан сдвинуть
// номера. Требовать тождества ОТ ПЕРВОГО экспорта значило бы требовать, чтобы
// импорт номера не трогал, — то есть отменить перенумерацию и вернуть
// пересечение номеров, ради снятия которого она и делается.
//
// Со второго круга состояние уже размечено принимающей стороной, и дальше
// тождество обязано держаться ВЕЧНО: любой дрейф здесь — это поле, которое
// каждый импорт чуть-чуть меняет, и через N переносов файл перестанет
// описывать исходную настройку.
func TestRoundTrip10ByteIdentical(t *testing.T) {
	s := richState10()
	first := fixedExport10(t, s)

	parsed, warns, err := Parse(first)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("свой же файл 1.0 вызвал предупреждения: %v", warns)
	}
	if parsed.Format != ExportFormat10 || parsed.V10 == nil {
		t.Fatalf("файл 1.0 прочитан не своим входом: %v", parsed.Format)
	}

	restored := &state.State{}
	if _, err := ImportFile(restored, parsed, importKnowsEverything10()); err != nil {
		t.Fatalf("Import: %v", err)
	}
	second := fixedExport10(t, restored)

	// Первый круг отличается РОВНО номерами оси и ничем больше: сравниваем
	// файлы, стерев номера. Иначе перенумерация прикрывала бы собой любую
	// другую потерю — «ну там же номера разные».
	if stripAxisNums(string(second)) != stripAxisNums(string(first)) {
		t.Fatalf("первый круг потерял не только номера оси:\n--- до ---\n%s\n--- после ---\n%s", first, second)
	}

	// СОСТОЯНИЕ после импорта эквивалентно исходному (инвариант 3 волны).
	// Равенство файлов этого не доказывает: файл — проекция, и поле,
	// потерянное ОДИНАКОВО на обоих концах круга, из байтов невидимо.
	assertStateEquivalent10(t, s, restored)

	// Второй круг — тождество байт в байт.
	parsed2, warns2, err := Parse(second)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warns2) != 0 {
		t.Fatalf("свой же файл 1.0 вызвал предупреждения: %v", warns2)
	}
	again := &state.State{}
	if _, err := ImportFile(again, parsed2, importKnowsEverything10()); err != nil {
		t.Fatalf("Import: %v", err)
	}
	third := fixedExport10(t, again)
	if string(third) != string(second) {
		t.Fatalf("круг не байт-идентичен:\n--- до ---\n%s\n--- после ---\n%s", second, third)
	}
}

// assertStateEquivalent10 — состояние после импорта описывает ту же настройку,
// что исходное.
//
// Не DeepEqual: у импорта есть ОДНО разрешённое расхождение (номера оси,
// §9 п. 7), а у состояния — поля, которых в файле нет по решению §6.0 (кэш
// узлов подписки, рантайм fetch'а). Поэтому сверяются те свойства, ради
// которых перенос и делается: записи на месте, их идентичность цела, ссылки
// разрешимы, ось сохранила ОТНОСИТЕЛЬНЫЙ порядок.
func assertStateEquivalent10(t *testing.T, want, got *state.State) {
	t.Helper()

	// Источники: те же ключи идентичности в том же порядке (§9 п. 8).
	if len(got.Sources) != len(want.Sources) {
		t.Fatalf("источников после импорта %d, было %d", len(got.Sources), len(want.Sources))
	}
	for i := range want.Sources {
		w, g := &want.Sources[i], &got.Sources[i]
		if w.Kind != g.Kind {
			t.Errorf("sources[%d]: вид %q → %q", i, w.Kind, g.Kind)
			continue
		}
		if w.ID != g.ID {
			t.Errorf("sources[%d] (%s): id %q → %q — ссылки detour/hops указывают на id",
				i, w.Kind, w.ID, g.ID)
		}
		switch w.Kind {
		case state.SourceKindSubscription:
			if w.URL != g.URL {
				t.Errorf("sources[%d]: url %q → %q (ключ слияния подписки)", i, w.URL, g.URL)
			}
			if w.IdentityUserAgent() != g.IdentityUserAgent() || w.IdentityHWID() != g.IdentityHWID() {
				t.Errorf("sources[%d]: identity подписки потеряна", i)
			}
			if len(w.Skip) != len(g.Skip) {
				t.Errorf("sources[%d]: skip[] %d → %d", i, len(w.Skip), len(g.Skip))
			}
			if w.RelaysInDirections != g.RelaysInDirections {
				t.Errorf("sources[%d]: relays_in_directions %v → %v (список целей Направлений)",
					i, w.RelaysInDirections, g.RelaysInDirections)
			}
		case state.SourceKindFolder:
			if w.Name != g.Name {
				t.Errorf("sources[%d]: имя папки %q → %q (ключ слияния папки)", i, w.Name, g.Name)
			}
			if len(w.Nodes) != len(g.Nodes) {
				t.Errorf("sources[%d] (%s): состав папки %d → %d узлов", i, w.Name, len(w.Nodes), len(g.Nodes))
			}
			// Собственные настройки папки: 1.0 завёл им дом (§6.0), и круг
			// обязан вернуть их такими же — иначе «дом есть» значит только
			// «место в файле», а не перенос.
			if (w.TagPolicy == nil) != (g.TagPolicy == nil) ||
				(w.TagPolicy != nil && *w.TagPolicy != *g.TagPolicy) {
				t.Errorf("sources[%d] (%s): политика тегов папки %+v → %+v", i, w.Name, w.TagPolicy, g.TagPolicy)
			}
			if (w.Detour == nil) != (g.Detour == nil) ||
				(w.Detour != nil && *w.Detour != *g.Detour) {
				t.Errorf("sources[%d] (%s): общий detour папки %+v → %+v", i, w.Name, w.Detour, g.Detour)
			}
		default:
			if w.Tag != g.Tag {
				t.Errorf("sources[%d]: тег %q → %q (на него метят правила)", i, w.Tag, g.Tag)
			}
		}
		// Свёртка: РЕЖИМ и ИМЯ группы. Имя — настоящая пользовательская
		// настройка (её правят руками), и на него метят правила того же
		// файла; пока 1.0 везла только режим, имя выводилось позиционной
		// формулой подписок и подменялось молча — у папки к тому же тем же
		// «1:select», что у первой свёрнутой подписки.
		switch {
		case (w.Replace == nil) != (g.Replace == nil):
			t.Errorf("sources[%d] (%s): свёртка %v → %v", i, w.Kind, w.Replace != nil, g.Replace != nil)
		case w.Replace != nil:
			if w.Replace.Tag != g.Replace.Tag {
				t.Errorf("sources[%d] (%s): тег группы свёртки %q → %q — правила метят в это имя",
					i, w.Kind, w.Replace.Tag, g.Replace.Tag)
			}
			if w.Replace.Mode != g.Replace.Mode {
				t.Errorf("sources[%d] (%s): режим свёртки %q → %q", i, w.Kind, w.Replace.Mode, g.Replace.Mode)
			}
		}
		if (w.Node.Sections == nil) != (g.Node.Sections == nil) {
			t.Errorf("sources[%d] (%s): секции узла %v → %v", i, w.Kind, w.Node.Sections != nil, g.Node.Sections != nil)
		}
		if w.Node.Sections != nil && g.Node.Sections != nil {
			if len(w.Node.Sections.Rules) != len(g.Node.Sections.Rules) {
				t.Errorf("sources[%d]: правил секции %d → %d", i, len(w.Node.Sections.Rules), len(g.Node.Sections.Rules))
			}
			if len(w.Node.Sections.DNSServers()) != len(g.Node.Sections.DNSServers()) ||
				len(w.Node.Sections.DNSRules()) != len(g.Node.Sections.DNSRules()) {
				t.Errorf("sources[%d]: DNS секции %d/%d → %d/%d", i,
					len(w.Node.Sections.DNSServers()), len(w.Node.Sections.DNSRules()),
					len(g.Node.Sections.DNSServers()), len(g.Node.Sections.DNSRules()))
			}
		}
		// Адресный хоп обязан указывать в СУЩЕСТВУЮЩУЮ здесь папку: ссылка,
		// которую импорт не переписал, — это цепочка, уходящая fail-closed.
		for h, hop := range g.Hops {
			if hop.FolderID == "" {
				continue
			}
			if !hasSourceID(got, hop.FolderID) {
				t.Errorf("sources[%d]: hops[%d].folder_id=%q не разрешается в этом состоянии", i, h, hop.FolderID)
			}
		}
	}

	// Правила: вид, имя, цель, включённость и ОТНОСИТЕЛЬНЫЙ порядок.
	if len(got.Rules) != len(want.Rules) {
		t.Fatalf("правил после импорта %d, было %d", len(got.Rules), len(want.Rules))
	}
	for i := range want.Rules {
		w, g := want.Rules[i], got.Rules[i]
		if w.Kind != g.Kind || w.Name != g.Name || w.Ref != g.Ref || w.Enabled != g.Enabled {
			t.Errorf("rules[%d]: %+v → %+v", i, w, g)
			continue
		}
		if len(w.Refs) != len(g.Refs) {
			t.Errorf("rules[%d] (%s): наборов srs %d → %d", i, w.Name, len(w.Refs), len(g.Refs))
		}
		// Тело сравнивается СЖАТЫМ: декодер 1.0 кладёт в запись сырые байты
		// файла как есть (ловушка §7.3 — порядок ключей тела нормативен), а
		// файл записан с отступами. Отступы умирают на первом же Save
		// (encoding/json сжимает RawMessage), поэтому расхождение по пробелам
		// — не потеря; расхождение по ПОРЯДКУ ключей было бы ею, и compact
		// его сохраняет.
		if compactJSONForCompare(t, w.Body) != compactJSONForCompare(t, g.Body) {
			t.Errorf("rules[%d] (%s): тело %s → %s", i, w.Name, w.Body, g.Body)
		}
		if (w.Num == nil) != (g.Num == nil) {
			t.Errorf("rules[%d] (%s): разметка оси %v → %v", i, w.Name, w.Num != nil, g.Num != nil)
		}
		if i > 0 && g.Num != nil && got.Rules[i-1].Num != nil && *got.Rules[i-1].Num >= *g.Num {
			t.Errorf("rules[%d] (%s): номер %d не больше предыдущего %d — относительный порядок оси нарушен",
				i, w.Name, *g.Num, *got.Rules[i-1].Num)
		}
	}

	// DNS: записи всех видов на месте, ссылочные не потеряли ref.
	if len(got.DNS.Servers) != len(want.DNS.Servers) || len(got.DNS.Rules) != len(want.DNS.Rules) {
		t.Errorf("DNS после импорта %d/%d записей, было %d/%d",
			len(got.DNS.Servers), len(got.DNS.Rules), len(want.DNS.Servers), len(want.DNS.Rules))
	}
	// Три скаляра секции, а не два: default_domain_resolver перечислен в
	// форме 1.0 (§6.0) наравне с ними, но читателя у него не было — писатель
	// клал ключ в файл, импорт ронял его молча.
	if got.DNS.Final != want.DNS.Final || got.DNS.Strategy != want.DNS.Strategy ||
		got.DNS.DefaultDomainResolver != want.DNS.DefaultDomainResolver {
		t.Errorf("DNS final/strategy/default_domain_resolver: %q/%q/%q → %q/%q/%q",
			want.DNS.Final, want.DNS.Strategy, want.DNS.DefaultDomainResolver,
			got.DNS.Final, got.DNS.Strategy, got.DNS.DefaultDomainResolver)
	}
	if (want.WarpAccounts != nil) != (got.WarpAccounts != nil) {
		t.Errorf("регистрации warp: %v → %v", want.WarpAccounts != nil, got.WarpAccounts != nil)
	}
	if len(got.Directions) != len(want.Directions) {
		t.Errorf("Направлений после импорта %d, было %d", len(got.Directions), len(want.Directions))
	}
}

// compactJSONForCompare сжимает тело записи, сохраняя порядок ключей.
func compactJSONForCompare(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("тело записи не JSON: %v (%s)", err, raw)
	}
	return buf.String()
}

// hasSourceID — есть ли в состоянии источник с таким id. Отдельно от
// findSourceByID (convert_v7_test.go): тот роняет тест на отсутствии, а здесь
// отсутствие — проверяемое утверждение, а не повод останавливать проверку.
func hasSourceID(s *state.State, id string) bool {
	for i := range s.Sources {
		if s.Sources[i].ID == id {
			return true
		}
	}
	return false
}

// stripAxisNums стирает значения `num` — единственное, что импорту РАЗРЕШЕНО
// менять (перенумерация оси). Всё прочее обязано доехать дословно.
func stripAxisNums(doc string) string {
	return axisNumRe.ReplaceAllString(doc, `"num": N`)
}

var axisNumRe = regexp.MustCompile(`"num": \d+`)

// Ссылки на папки переживают импорт на ДРУГОЙ машине: id папки в файле чужой,
// а локальная папка держит свой (§6.0, инвариант 5 волны).
//
// Это ровно тот случай, ради которого заведена карта id: без переписки хоп
// цепочки указывал бы в папку, которой на этой машине нет, и цепочка уходила
// бы fail-closed при каждой сборке.
func TestImport10RewritesFolderLinksToLocalIDs(t *testing.T) {
	file := fixedExport10(t, richState10())
	parsed, _, err := Parse(file)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// На принимающей машине папка с тем же именем УЖЕ есть — со своим id.
	local := &state.State{Sources: []state.Source{{
		ID:   "01LOCALFOLDER00000000000",
		Name: "Proton",
		Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
	}}}
	if _, err := ImportFile(local, parsed, importKnowsEverything10()); err != nil {
		t.Fatalf("Import: %v", err)
	}

	var chain *state.Source
	for i := range local.Sources {
		if local.Sources[i].Kind == state.SourceKindChain && local.Sources[i].Tag == "via-proton" {
			chain = &local.Sources[i]
		}
	}
	if chain == nil || len(chain.Hops) != 1 {
		t.Fatalf("цепочка с адресным хопом потеряна: %+v", chain)
	}
	if chain.Hops[0].FolderID != "01LOCALFOLDER00000000000" {
		t.Errorf("хоп указывает на id файла %q, а не на локальную папку %q",
			chain.Hops[0].FolderID, "01LOCALFOLDER00000000000")
	}
	// Папка осталась ОДНА: «одно имя = одна папка» (§9 п. 3).
	folders := 0
	for i := range local.Sources {
		if local.Sources[i].Kind == state.SourceKindFolder {
			folders++
		}
	}
	if folders != 1 {
		t.Errorf("папок после импорта %d — имя совпало, а папка завелась второй", folders)
	}
}
