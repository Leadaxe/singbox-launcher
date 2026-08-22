package backup

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// Главное, ради чего Направления попали в бэкап: правило, чья цель приехала
// в том же файле, обязано прийти РАБОЧИМ. До схемы v1.1 такое правило
// импортировалось выключенным — цели на принимающей стороне не существовало.
func TestRuleWithImportedDirectionArrivesEnabled(t *testing.T) {
	b := &Backup{
		LxBackup:   FormatVersion,
		ExportedBy: ExportedBy{App: AppLxBox, Version: "2.1.0"},
		Directions: []Direction{{Tag: "vpn-3", Label: "Германия", Filter: "🇩🇪"}},
		Rules: []Rule{{
			Kind:     RuleInline,
			Name:     "Germany",
			Outbound: "vpn-3",
			Match:    json.RawMessage(`{"domain_suffix":["de.example-1.com"]}`),
		}},
	}
	dst := &state.State{}

	// KnownOutbounds намеренно НЕ содержит vpn-3: цель приезжает файлом.
	res, err := Import(dst, b, ImportOptions{KnownOutbounds: []string{"direct-out"}})
	if err != nil {
		t.Fatalf("импорт: %v", err)
	}
	if res.AppliedDirections != 1 {
		t.Fatalf("направление не создано: %+v", res)
	}
	if len(dst.Rules) != 1 {
		t.Fatalf("правил %d", len(dst.Rules))
	}
	if !dst.Rules[0].Enabled {
		t.Fatalf("правило пришло выключенным, хотя его цель приехала в этом же файле; предупреждения: %v", res.Warnings)
	}
}

// Существующее Направление не перезаписывается: у принимающей стороны свои
// настройки под тем же тегом, и молча стереть их нельзя.
func TestExistingDirectionIsNotOverwritten(t *testing.T) {
	dst := &state.State{}
	dst.Connections.Outbounds = []configtypes.Direction{{
		Tag: "vpn-1", Label: "Моё имя",
		Filters: configtypes.SetDirectionFilterTag(nil, "🇳🇱", false),
	}}

	b := &Backup{
		LxBackup:   FormatVersion,
		ExportedBy: ExportedBy{App: AppLxBox, Version: "2.1.0"},
		Directions: []Direction{{Tag: "vpn-1", Label: "Чужое имя", Filter: "🇩🇪"}},
	}
	res, err := Import(dst, b, ImportOptions{})
	if err != nil {
		t.Fatalf("импорт: %v", err)
	}
	if res.AppliedDirections != 0 {
		t.Fatalf("существующее направление перезаписано")
	}
	if dst.Connections.Outbounds[0].Label != "Моё имя" {
		t.Fatalf("имя затёрто: %q", dst.Connections.Outbounds[0].Label)
	}
	body, _ := configtypes.DirectionFilterTag(dst.Connections.Outbounds[0].Filters)
	if body != "🇳🇱" {
		t.Fatalf("отбор затёрт: %q", body)
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Code != WarnBackupDirectionExists {
		t.Fatalf("нет предупреждения о совпадении тега: %v", res.Warnings)
	}
}

// Круг замыкается: экспорт → импорт даёт то же Направление. Иначе перенос
// между платформами тихо терял бы настройки.
func TestDirectionRoundTrip(t *testing.T) {
	interrupt := false
	src := &state.State{}
	src.Connections.Outbounds = []configtypes.Direction{{
		Tag:              "vpn-2",
		Label:            "Моя Германия",
		Disabled:         true,
		Filters:          configtypes.SetDirectionFilterTag(nil, "🇩🇪|🇳🇱", true),
		PreferredDefault: configtypes.SetDirectionFilterTag(nil, "🇳🇱", false),
		AddOutbounds:     []string{"vpn-1", "direct-out", "block-out"},
		Options:          map[string]interface{}{"interrupt_exist_connections": false},
		Auto: &configtypes.DirectionAuto{
			Mode: configtypes.AutoModeRoundRobin, URL: "http://cp.example/generate_204",
			Interval: "15m", Tolerance: configtypes.NewTemplateInt(50), Pool: 3, PoolTolerance: configtypes.NewTemplateInt(20),
			StickyHash:                []string{"process"},
			InterruptExistConnections: &interrupt,
		},
	}}

	b, err := Export(src, ExportOptions{AppVersion: "1.4.2"})
	if err != nil {
		t.Fatalf("экспорт: %v", err)
	}
	if len(b.Directions) != 1 {
		t.Fatalf("направлений в бэкапе: %d", len(b.Directions))
	}
	got := b.Directions[0]
	if got.Filter != "🇩🇪|🇳🇱" || !got.Invert {
		t.Fatalf("отбор: (%q, %v)", got.Filter, got.Invert)
	}
	if got.Default != "🇳🇱" {
		t.Fatalf("умолчание: %q", got.Default)
	}
	if !got.IncludeDirect || !got.IncludeBlock {
		t.Fatalf("служебные опции потеряны: %+v", got)
	}
	if len(got.Include) != 1 || got.Include[0] != "vpn-1" {
		t.Fatalf("ссылка на другое направление потеряна: %v", got.Include)
	}
	if got.Enabled == nil || *got.Enabled {
		t.Fatalf("выключение не перенесено: %v", got.Enabled)
	}
	if got.Auto == nil || got.Auto.Mode != configtypes.AutoModeRoundRobin || got.Auto.Pool != 3 {
		t.Fatalf("автовыбор потерян: %+v", got.Auto)
	}

	dst := &state.State{}
	if _, err := Import(dst, b, ImportOptions{}); err != nil {
		t.Fatalf("импорт: %v", err)
	}
	back := dst.Connections.Outbounds[0]
	if !back.Disabled || back.Label != "Моя Германия" {
		t.Fatalf("round-trip потерял поля: %+v", back)
	}
	body, invert := configtypes.DirectionFilterTag(back.Filters)
	if body != "🇩🇪|🇳🇱" || !invert {
		t.Fatalf("round-trip отбора: (%q, %v)", body, invert)
	}
	if back.Auto == nil || func() bool { n, _ := back.Auto.PoolTolerance.Int(); return n != 20 }() {
		t.Fatalf("round-trip автовыбора: %+v", back.Auto)
	}
	if back.Auto.InterruptExistConnections == nil || *back.Auto.InterruptExistConnections {
		t.Fatalf("трёхзначный interrupt потерян при переносе")
	}
}
