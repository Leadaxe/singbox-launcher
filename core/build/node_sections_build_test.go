package build

// Сборка конфига с секциями узла (SPEC 121 §8, п. 1–4).
//
// Тест интеграционный и один на все четыре сценария: проверять развёртывание
// по кускам значило бы проверять не то, что реально уезжает в config.json.
// Материал — тот же golden-шаблон real-v088, что и у регрессионного теста
// сборки: он несёт живые dns/route-секции, а значит вставка узловых
// фрагментов проверяется на настоящем окружении, а не на пустом объекте.
//
// Главный инвариант (п. 4): состояние БЕЗ секций даёт байт-в-байт тот же
// конфиг, что и до задачи. Он же охраняется TestGoldenScenarios; здесь он
// перепроверен на том же входе с узлом, у которого секции сняты.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// userRuleMarkerDomain — домен пользовательского правила-маркера: по его
// позиции в собранном конфиге видно, встал ли якорь узла выше зоны 1000.
const userRuleMarkerDomain = "user-zone-marker.example"

// nodeSectionsScenario — один случай таблицы.
type nodeSectionsScenario struct {
	name string
	// finalTag — под каким тегом узел уехал в конфиг (пусто = узел до
	// эмиссии не дошёл: выключен либо снят санитайзером).
	finalTag string
	// anchorEnabled — состояние тумблера якоря на оси.
	anchorEnabled bool
	// withSections — несёт ли узел секции вообще.
	withSections bool

	wantDNSServerTag  string // ожидаемый тег DNS-сервера узла ("" = сервера нет)
	wantDNSRuleServer string // ожидаемое поле server у DNS-правила узла
	wantRouteOutbound string // ожидаемая цель правила маршрута узла
}

func TestBuildWithNodeSections(t *testing.T) {
	const (
		rootTag   = "ts-node"
		folderTag = "DE-ts-node" // тот же узел в папке с TagPolicy prefix "DE-"
	)

	cases := []nodeSectionsScenario{
		{
			name:     "root node carries its sections", // §8 п. 1
			finalTag: rootTag, anchorEnabled: true, withSections: true,
			wantDNSServerTag:  rootTag + ":ts-dns",
			wantDNSRuleServer: rootTag + ":ts-dns",
			wantRouteOutbound: rootTag,
		},
		{
			name:     "folder tag policy feeds the final tag", // §8 п. 2
			finalTag: folderTag, anchorEnabled: true, withSections: true,
			wantDNSServerTag:  folderTag + ":ts-dns",
			wantDNSRuleServer: folderTag + ":ts-dns",
			wantRouteOutbound: folderTag,
		},
		{
			name:     "disabled node emits nothing", // §8 п. 3
			finalTag: "", anchorEnabled: true, withSections: true,
		},
		{
			name:     "no sections at all", // §8 п. 4
			finalTag: rootTag, anchorEnabled: true, withSections: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out := string(buildNodeSectionsConfig(t, tc))

			// Собранный конфиг несёт маркеры-комментарии парсера и потому не
			// является чистым JSON — проверяем по тексту, как это делает
			// golden-тест. Строки уникальны: тег узла синтетический.
			if tc.wantDNSServerTag == "" && tc.wantRouteOutbound == "" {
				// Ничего не ждём → конфиг обязан совпасть БАЙТ-В-БАЙТ с тем,
				// что даёт тот же вход при полностью снятых секциях (SPEC §8
				// пп. 3–4, главный инвариант задачи). Эталон строится из
				// самого сценария, а не из общего «пустого» состояния: иначе
				// сравнивались бы разные конфиги, а не наличие фрагментов.
				ref := tc
				ref.withSections = false
				ref.finalTag = tc.finalTag
				want := string(buildNodeSectionsConfig(t, ref))
				if out != want {
					t.Fatalf("конфиг разошёлся с эталоном без секций "+
						"(эталон %d байт, собранный %d)", len(want), len(out))
				}
				return
			}

			for _, want := range []string{
				`"tag": "` + tc.wantDNSServerTag + `"`,
				`"server": "` + tc.wantDNSRuleServer + `"`,
				`"outbound": "` + tc.wantRouteOutbound + `"`,
			} {
				if !strings.Contains(out, want) {
					t.Errorf("в собранном конфиге нет %s", want)
				}
			}
			// `@self` обязан быть подставлен везде, включая detour сервера.
			if strings.Contains(out, "@self") {
				t.Error("плейсхолдер @self уехал в конфиг неподставленным")
			}
			if !strings.Contains(out, `"detour": "`+tc.finalTag+`"`) {
				t.Errorf("detour DNS-сервера не получил финальный тег %q", tc.finalTag)
			}

			// Якорь стоит на 945 — ниже пользовательской зоны (1000), значит
			// правило узла обязано встать РАНЬШЕ пользовательского правила:
			// подсеть за узлом должна матчиться до общих правил.
			nodeAt := strings.Index(out, `"100.64.0.0/10"`)
			userAt := strings.Index(out, userRuleMarkerDomain)
			if nodeAt < 0 || userAt < 0 {
				t.Fatalf("в конфиге нет обоих правил для сравнения позиций (узел %d, пользовательское %d)", nodeAt, userAt)
			}
			if nodeAt > userAt {
				t.Errorf("правило узла (байт %d) стоит позже пользовательского (байт %d) — "+
					"якорь %d обязан быть выше зоны %d",
					nodeAt, userAt, state.NodeRuleDefaultNum, state.UserRuleNumStart)
			}
		})
	}
}

// buildNodeSectionsConfig собирает конфиг по сценарию на golden-шаблоне.
func buildNodeSectionsConfig(t *testing.T, tc nodeSectionsScenario) []byte {
	t.Helper()

	dir := filepath.Join("testdata", "golden", "real-v088")
	tmplBytes, err := os.ReadFile(filepath.Join(dir, "template.json"))
	if err != nil {
		t.Skipf("golden-шаблон недоступен: %v", err)
	}
	td, err := parseGoldenTemplate(tmplBytes)
	if err != nil {
		t.Fatalf("разбор шаблона: %v", err)
	}

	link := NodeLink{Tag: "ts-node"}
	// clash_secret шаблон материализует случайным, если его нет в состоянии, —
	// а сравнение байт-в-байт этого не переживёт. Фиксируем значение.
	st := &state.State{Vars: []state.SettingVar{{Name: "clash_secret", Value: "test-secret"}}}

	cache := &ParsedCache{}
	if tc.finalTag != "" {
		cache.Outbounds = []json.RawMessage{
			json.RawMessage(`{"type":"trojan","tag":"` + tc.finalTag + `","server":"1.2.3.4","server_port":443,"password":"p"}`),
		}
		if tc.withSections {
			cache.NodeSections = []NodeSectionSet{{
				FinalTag: tc.finalTag,
				Link:     link,
				DNSServers: []json.RawMessage{
					json.RawMessage(`{"type":"udp","tag":"ts-dns","server":"100.100.100.100","detour":"@self"}`),
				},
				DNSRules: []json.RawMessage{
					json.RawMessage(`{"domain_suffix":[".ts.net"],"server":"ts-dns"}`),
				},
				Rules: []json.RawMessage{
					json.RawMessage(`{"ip_cidr":["100.64.0.0/10"],"outbound":"@self"}`),
				},
			}}
		}
	}

	// Пользовательское правило в своей зоне (1000): относительно него и
	// проверяется позиция якоря узла (945).
	userBody, err := json.Marshal(state.InlineBody{
		Name:     "user-marker",
		Match:    map[string]interface{}{"domain_suffix": []string{userRuleMarkerDomain}},
		Outbound: "direct-out",
	})
	if err != nil {
		t.Fatalf("кодирование пользовательского правила: %v", err)
	}
	userNum := state.DefaultRuleNum
	st.Rules = append(st.Rules, state.Rule{
		Kind:     state.RuleKindInline,
		Enabled:  true,
		OrderNum: &userNum,
		Body:     userBody,
	})

	// Якорь на оси живёт в состоянии независимо от того, дошёл ли узел до
	// эмиссии: выключенный узел оставляет якорь в списке (SPEC §2).
	if tc.withSections {
		num := state.NodeRuleDefaultNum
		body, err := json.Marshal(state.NodeRuleBody{FolderID: link.FolderID, Tag: link.Tag})
		if err != nil {
			t.Fatalf("кодирование тела якоря: %v", err)
		}
		st.Rules = append(st.Rules, state.Rule{
			Kind:     state.RuleKindNode,
			Enabled:  tc.anchorEnabled,
			OrderNum: &num,
			Body:     body,
		})
	}

	ctx := BuildContext{
		Template:   td,
		Vars:       stateVarsToMap(st),
		Cache:      cache,
		ForPreview: false,
		DNS:        dnsConfigFromState(st),
		Route:      routeConfigFromState(st),
		Target:     TargetSpecFromState(st),
		Preset:     presetContextFromState(st, td),
	}
	// Секции доезжают до слияния через кэш (buildOrderedSections снимает их
	// после санитайзера) — контекст их не несёт, как и на боевом пути.

	res, err := BuildConfig(ctx)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	return normalizeParserTimestamp(res.ConfigJSON)
}
