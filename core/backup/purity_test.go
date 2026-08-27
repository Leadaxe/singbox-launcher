package backup

// Тесты чистоты экспорта и roundtrip'а сущностей (SPEC 114, П1).
//
// Здесь проверяется не «поле доехало», а само свойство формата: файл — это
// СЕРИАЛИЗАЦИЯ СОСТОЯНИЯ и ничего кроме. Пока в формате жил механизм
// extensions, оба свойства были ложны: экспорт зависел от того, что принесла
// прошлая загрузка, а состояние помнило чужой груз.

import (
	"encoding/json"
	"testing"
	"time"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// fixedExport — экспорт с прибитым моментом времени: exported_at обязан быть
// одинаковым, иначе сравнение байтов проверяло бы часы, а не чистоту.
func fixedExport(t *testing.T, s *state.State) []byte {
	t.Helper()
	b, err := Export(s, ExportOptions{
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
	s.Connections.Outbounds = []configtypes.Direction{
		{Tag: "vpn-de", Type: "selector", AddOutbounds: []string{"direct-out"}},
	}
	s.Connections.Sources = []state.Source{
		{
			ID: "01SUB0000000000000000000", Type: state.SourceTypeSubscription, Enabled: true,
			URL: "https://example-1.com/sub", Label: "Main", MaxNodes: 200,
			Tag:                     &state.TagSpec{Prefix: "[A] ", Mask: "%s"},
			Update:                  &state.UpdateSpec{IntervalHours: 12, AutoRefresh: &auto},
			DisabledNodes:           map[string]int64{"node-a": 1750000000, "node-b": 1750000001},
			Skip:                    []map[string]string{{"field": "tag", "contains": "trial"}},
			Outbounds:               []configtypes.Direction{{Tag: "local-select", Type: "selector"}},
			Fold:                    &configtypes.SourceFold{Mode: configtypes.FoldModeSelect},
			ExcludeFromGlobal:       true,
			ExposeGroupTagsToGlobal: true,
			DetourNodeSourceID:      "01SRV0000000000000000000",
			DetourNodeTag:           "🔥 WARP",
			DetourNodeLabel:         "WARP hop",
		},
		{
			ID: "01SRV0000000000000000000", Type: state.SourceTypeServer, Enabled: true,
			URI:   "vless://11111111-1111-1111-1111-111111111111@example-2.com:443?type=tcp#s",
			Label: "WARP hop", NodeTag: "🔥 WARP", DetourTag: "hop-1",
		},
		{
			ID: "01CHN0000000000000000000", Type: state.SourceTypeChain, Enabled: true,
			NodeTag: "relay", Label: "Мой маршрут",
			Chain: &configtypes.SourceChain{
				Hops: []string{"vpn-de", "🔥 WARP"}, IdleTimeout: "0s",
				StripEvasion: &stripOff,
				Strip:        map[string]bool{"tls.utls": false},
				Rewrite:      map[string]interface{}{"vless": map[string]interface{}{"flow": nil}},
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
	}
	s.ConfigParams = []state.ConfigParam{{Name: "final", Value: "vpn-de"}}
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
	if _, err := Import(imported, b, importKnowsEverything()); err != nil {
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
	if _, err := Import(restored, b, importKnowsEverything()); err != nil {
		t.Fatalf("Import: %v", err)
	}

	var chain *state.Source
	for i := range restored.Connections.Sources {
		if restored.Connections.Sources[i].Type == state.SourceTypeChain {
			chain = &restored.Connections.Sources[i]
		}
	}
	if chain == nil {
		t.Fatal("цепочка потеряна на roundtrip")
	}
	if chain.NodeTag != "relay" {
		t.Errorf("тег цепочки после импорта %q, ожидался %q — ссылки правил и позиций разъедутся", chain.NodeTag, "relay")
	}
	if chain.Label != "Мой маршрут" {
		t.Errorf("подпись цепочки после импорта %q", chain.Label)
	}
	if chain.ID != "01CHN0000000000000000000" {
		t.Errorf("id цепочки потерян: %q", chain.ID)
	}

	second := fixedExport(t, restored)
	if string(second) != string(first) {
		t.Fatalf("roundtrip не тождественен:\n--- до ---\n%s\n--- после ---\n%s", first, second)
	}
}
