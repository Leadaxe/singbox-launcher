package backup

import (
	"encoding/json"
	"strings"
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
		Directions: []Direction{{Tag: "vpn-3", Filter: "🇩🇪"}},
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
	dst.Directions = []configtypes.Direction{{
		Tag:     "vpn-1",
		Filters: configtypes.SetDirectionFilterTag(nil, "🇳🇱", false),
	}}

	b := &Backup{
		LxBackup:   FormatVersion,
		ExportedBy: ExportedBy{App: AppLxBox, Version: "2.1.0"},
		Directions: []Direction{{Tag: "vpn-1", Filter: "🇩🇪"}},
	}
	res, err := Import(dst, b, ImportOptions{})
	if err != nil {
		t.Fatalf("импорт: %v", err)
	}
	if res.AppliedDirections != 0 {
		t.Fatalf("существующее направление перезаписано")
	}
	body, _ := configtypes.DirectionFilterTag(dst.Directions[0].Filters)
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
	src.Directions = []configtypes.Direction{{
		Tag:              "vpn-2",
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
	back := dst.Directions[0]
	if !back.Disabled || back.Tag != "vpn-2" {
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

// Контракт 0.9.0 снёс у Направления поле label: имя ровно одно — tag.
// Приехавший label чужой стороны — обычный неизвестный ключ (П3): warning,
// именем остаётся тег, ничего никуда не провозится.
func TestDirectionForeignLabelDroppedWithWarning(t *testing.T) {
	raw := []byte(`{"lx_backup":1,"exported_by":{"app":"lxbox","version":"2.1.0"},` +
		`"exported_at":"2026-08-22T00:00:00Z",` +
		`"directions":[{"tag":"vpn-3","label":"Германия","filter":"🇩🇪"}]}`)
	b, warns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	named := false
	for _, w := range warns {
		if w.Code == WarnBackupUnknownField && w.Detail == "directions[vpn-3].label" {
			named = true
		}
	}
	if !named {
		t.Fatalf("чужой label Направления отброшен молча: %v", warns)
	}

	dst := &state.State{}
	if _, err := Import(dst, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(dst.Directions) != 1 || dst.Directions[0].Tag != "vpn-3" {
		t.Fatalf("Направление не создано или переименовано: %+v", dst.Directions)
	}
	// Провоза больше нет: обратный экспорт обязан быть без чужого имени.
	back, err := Export(dst, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(back)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "Германия") {
		t.Fatalf("чужая подпись Направления провезена: %s", out)
	}
}
