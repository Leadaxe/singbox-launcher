package business

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"singbox-launcher/core/backup"
	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// nodeInDirectionState — состояние сборки до 1.6.0: в опциях Направления
// `vpn` стоит ФИНАЛЬНЫЙ тег узла подписки (`nl:US-1`, вписан сырым JSON),
// рядом — тег свёртки `pick` и строка без цели `ghost`. Корневой узел
// `tokyo` ходит через тот же узел корневой ссылкой `{tag: "nl:US-1"}`,
// которая разрешалась только через опцию Направления.
const nodeInDirectionState = `{
  "meta": {"version": 8, "schema": "sources_v8", "created_at": "2026-09-01T00:00:00Z", "updated_at": "2026-09-01T00:00:00Z"},
  "sources": [
    {
      "kind": "subscription", "id": "01SUBNL0000000000000000000", "name": "NL", "enabled": true,
      "url": "https://example.invalid/nl", "tag_policy": {"prefix": "nl:"},
      "nodes": [
        {"kind": "server", "tag": "US-1", "enabled": true, "body": {"type": "trojan", "server": "us1.example", "server_port": 443, "password": "p1"}},
        {"kind": "server", "tag": "US-2", "enabled": true, "body": {"type": "trojan", "server": "us2.example", "server_port": 443, "password": "p2"}}
      ]
    },
    {
      "kind": "folder", "id": "01FLDFOLD00000000000000000", "name": "Fold", "enabled": true,
      "replace": {"mode": "manual", "tag": "pick"},
      "nodes": [
        {"kind": "server", "tag": "de-1", "enabled": true, "body": {"type": "trojan", "server": "de1.example", "server_port": 443, "password": "p3"}}
      ]
    },
    {"kind": "server", "tag": "tokyo", "enabled": true, "body": {"type": "trojan", "server": "tokyo.example", "server_port": 443, "password": "p4"},
     "detour": {"tag": "nl:US-1"}}
  ],
  "directions": [
    {"tag": "vpn", "type": "selector", "addOutbounds": ["direct-out", "nl:US-1", "pick", "ghost"]}
  ],
  "rules": [],
  "dns": {}
}`

// NODE_LINK.md §8, решение владельца 15.09.2026 (вариант А): Направление на
// узлы не ссылается. Сырой JSON узел в опции не пропускает; уже сохранённое
// состояние собирается с прежним составом и предупреждением; корневая ссылка
// на узел, законная только через опцию, после закрытия лазейки поднимается до
// пары и разрешается; экспорт кладёт в `include` только теги Направлений.
func TestDirectionsDoNotHoldNodes(t *testing.T) {
	st, err := corestate.Parse([]byte(nodeInDirectionState))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	const sub = "01SUBNL0000000000000000000"

	// S5′: корневая ссылка на узел из опции — пара этого узла.
	tokyo := &st.Sources[2]
	if tokyo.Detour == nil || *tokyo.Detour != (corestate.NodeLink{FolderID: sub, Tag: "US-1"}) {
		t.Fatalf("detour {tag: nl:US-1} не поднят до пары: %+v", tokyo.Detour)
	}

	m := &wizardmodels.WizardModel{Sources: st.Sources, GlobalOutbounds: st.Directions}

	// Сырой JSON: узел — отказ «используйте фильтр», неизвестное имя — «не
	// найден», объявленные имена проходят.
	if err := ValidateDirectionOptions(m, "vpn", false, []string{"direct-out", "nl:US-1"}); err == nil ||
		err.Error() != fmt.Sprintf(locale.T(directionOptionNodeText), "nl:US-1") {
		t.Errorf("узел в опциях сырого JSON: %v", err)
	}
	if err := ValidateDirectionOptions(m, "vpn", false, []string{"ghost"}); err == nil ||
		err.Error() != fmt.Sprintf(locale.T(directionOptionUnknownText), "ghost") {
		t.Errorf("неизвестное имя в опциях сырого JSON: %v", err)
	}
	if err := ValidateDirectionOptions(m, "vpn", true, []string{"direct-out", "pick", "vpn-auto"}); err != nil {
		t.Errorf("объявленные имена отвергнуты: %v", err)
	}

	// Сборка: состав прежний, узел назван предупреждением, detour разрешён.
	res, err := config.GenerateOutboundsFromParserConfig(m.AsParserConfig(), map[string]int{}, nil,
		config.DirectionBuildOptions{BlockTag: "block-out", DirectTag: "direct-out"})
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	objects := map[string]map[string]interface{}{}
	for _, raw := range res.OutboundsJSON {
		start := strings.Index(raw, "{")
		if start < 0 {
			continue
		}
		var obj map[string]interface{}
		if json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimSpace(raw[start:]), ",")), &obj) == nil {
			if tag, _ := obj["tag"].(string); tag != "" {
				objects[tag] = obj
			}
		}
	}
	vpn := objects["vpn"]
	if vpn == nil {
		t.Fatalf("Направление не собралось: %v", res.OutboundsJSON)
	}
	members, _ := vpn["outbounds"].([]interface{})
	hasMember := func(tag string) bool {
		for _, m := range members {
			if m == tag {
				return true
			}
		}
		return false
	}
	if !hasMember("nl:US-1") || !hasMember("pick") || !hasMember("direct-out") {
		t.Errorf("состав Направления изменился: %v", members)
	}
	if got := objects["tokyo"]; got == nil || got["detour"] != "nl:US-1" {
		t.Errorf("detour через узел из опции не разрешился: %v", got)
	}
	var nodeWarn, unknownWarn bool
	for _, w := range res.EmissionWarnings {
		if w.DirectionTag != "vpn" {
			continue
		}
		switch {
		case strings.Contains(w.Text, `"nl:US-1"`):
			nodeWarn = w.Text == fmt.Sprintf(locale.T("Direction %q: node %q is listed as an option — a node cannot be added to a Direction directly, use a filter"), "vpn", "nl:US-1")
		case strings.Contains(w.Text, `"ghost"`):
			unknownWarn = true
		case strings.Contains(w.Text, `"pick"`) || strings.Contains(w.Text, `"direct-out"`):
			t.Errorf("объявленное имя названо предупреждением: %q", w.Text)
		}
	}
	if !nodeWarn || !unknownWarn {
		t.Errorf("опции Направления не названы (узел=%v, неизвестное=%v): %v", nodeWarn, unknownWarn, res.EmissionWarnings)
	}

	// Экспорт: в `include` только теги Направлений; остальное — local-only.
	file, warns, err := backup.Export10(st, backup.ExportOptions{})
	if err != nil {
		t.Fatalf("экспорт: %v", err)
	}
	if len(file.Directions) != 1 || len(file.Directions[0].Include) != 0 || !file.Directions[0].IncludeDirect {
		t.Errorf("include Направления: %+v", file.Directions)
	}
	var localOnly string
	for _, w := range warns {
		if w.Code == backup.WarnBackupLocalOnlyDropped {
			localOnly = w.Detail
		}
	}
	for _, opt := range []string{"nl:US-1", "pick", "ghost"} {
		if !strings.Contains(localOnly, opt) {
			t.Errorf("опция %q не названа предупреждением экспорта: %v", opt, warns)
		}
	}
}
