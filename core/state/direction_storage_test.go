package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// Состояние, записанное до SPEC 104, лежит под ключом `outbounds`. Оно
// обязано читаться, а следующая запись — переехать на канонический ключ.
func TestLegacyOutboundsKeyAdopted(t *testing.T) {
	raw := `{
	  "meta": {"version": 6, "schema": "` + SchemaNameV6 + `", "created_at": "2026-08-22T00:00:00Z", "updated_at": "2026-08-22T00:00:00Z"},
	  "connections": {
	    "sources": [],
	    "outbounds": [{"tag": "proxy-out", "type": "selector"}],
	    "defaults": {}
	  },
	  "rules": [],
	  "dns_options": {}
	}`
	s, err := parseV6Legacy([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(s.Directions) != 1 || s.Directions[0].Tag != "proxy-out" {
		t.Fatalf("старый ключ не подхвачен: %+v", s.Directions)
	}
}

// Канонический ключ приоритетнее: состояние, потроганное старой версией
// после новой, не должно склеивать два набора направлений.
func TestCanonicalKeyWinsOverLegacy(t *testing.T) {
	raw := `{
	  "meta": {"version": 6, "schema": "` + SchemaNameV6 + `", "created_at": "2026-08-22T00:00:00Z", "updated_at": "2026-08-22T00:00:00Z"},
	  "connections": {
	    "sources": [],
	    "direction_outbounds": [{"tag": "vpn-1", "label": "VPN ①"}],
	    "outbounds": [{"tag": "proxy-out"}],
	    "defaults": {}
	  },
	  "rules": [],
	  "dns_options": {}
	}`
	s, err := parseV6Legacy([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(s.Directions) != 1 || s.Directions[0].Tag != "vpn-1" {
		t.Fatalf("должен побеждать канонический ключ: %+v", s.Directions)
	}
}

// Пустой канонический ключ — это осознанное «направлений нет», а не
// «секции не было»: старый набор подхватывать нельзя.
func TestEmptyCanonicalKeyIsNotSeededFromLegacy(t *testing.T) {
	raw := `{
	  "meta": {"version": 6, "schema": "` + SchemaNameV6 + `", "created_at": "2026-08-22T00:00:00Z", "updated_at": "2026-08-22T00:00:00Z"},
	  "connections": {
	    "sources": [],
	    "direction_outbounds": [],
	    "outbounds": [{"tag": "proxy-out"}],
	    "defaults": {}
	  },
	  "rules": [],
	  "dns_options": {}
	}`
	s, err := parseV6Legacy([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(s.Directions) != 0 {
		t.Fatalf("пустой канонический ключ подменён легаси: %+v", s.Directions)
	}
}

// Записываем только новый ключ. Два набора в одном файле означали бы, что
// следующая загрузка может выбрать не тот.
func TestSaveWritesCanonicalKeyOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &State{Version: SchemaVersion}
	s.Directions = []configtypes.Direction{{Tag: "vpn-1"}}

	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	text := string(data)
	// SPEC 118: канонический v7-ключ — плоский `directions`.
	if !strings.Contains(text, `"directions"`) {
		t.Fatalf("канонического ключа нет в файле:\n%s", text)
	}
	if strings.Contains(text, `"direction_outbounds"`) || strings.Contains(text, `"outbounds":`) {
		t.Fatalf("старые ключи не должны писаться:\n%s", text)
	}

	// Перечитываем — направление на месте.
	back, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(back.Directions) != 1 || back.Directions[0].Tag != "vpn-1" {
		t.Fatalf("round-trip потерял направление: %+v", back.Directions)
	}
}

// Новые поля переживают запись и чтение — иначе выключение и двойник
// молча пропадали бы на следующем старте.
func TestDirectionFieldsSurviveRoundTrip(t *testing.T) {
	interrupt := false
	in := configtypes.Direction{
		Tag:      "vpn-2",
		Disabled: true,
		Filters:  map[string]interface{}{"tag": "/🇩🇪/i"},
		Auto: &configtypes.DirectionAuto{
			Mode:                      configtypes.AutoModeRoundRobin,
			URL:                       "http://cp.cloudflare.com/generate_204",
			Interval:                  "15m",
			Tolerance:                 configtypes.NewTemplateInt(50),
			IdleTimeout:               "30m",
			InterruptExistConnections: &interrupt,
			Pool:                      3,
			PoolTolerance:             configtypes.NewTemplateInt(20),
			StickyHash:                []string{"process", "domain"},
		},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out configtypes.Direction
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Tag != in.Tag || !out.Disabled {
		t.Fatalf("тег/выключение потеряны: %+v", out)
	}
	if out.Auto == nil {
		t.Fatalf("двойник потерян")
	}
	if out.Auto.Mode != configtypes.AutoModeRoundRobin || out.Auto.Pool != 3 || func() bool { n, _ := out.Auto.PoolTolerance.Int(); return n != 20 }() {
		t.Fatalf("параметры пула потеряны: %+v", out.Auto)
	}
	if out.Auto.InterruptExistConnections == nil || *out.Auto.InterruptExistConnections {
		t.Fatalf("явное выключение interrupt потеряно: %+v", out.Auto.InterruptExistConnections)
	}
	if len(out.Auto.StickyHash) != 2 {
		t.Fatalf("sticky_hash потерян: %+v", out.Auto.StickyHash)
	}
}

// Направление без Auto не пишет ключ вовсе, а nil-интеррупт отличается от
// явного false: это разные состояния, и шаблон должен их различать.
func TestDirectionOmitsAbsentAuto(t *testing.T) {
	raw, err := json.Marshal(configtypes.Direction{Tag: "vpn-1"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "auto") {
		t.Fatalf("отсутствующий двойник не должен попадать в JSON: %s", raw)
	}
	if strings.Contains(string(raw), "disabled") || strings.Contains(string(raw), "label") {
		t.Fatalf("нулевые поля не должны засорять state.json: %s", raw)
	}

	var a configtypes.DirectionAuto
	if err := json.Unmarshal([]byte(`{"url":"x"}`), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.InterruptExistConnections != nil {
		t.Fatalf("отсутствие ключа должно читаться как «шаблон решает», got %v", *a.InterruptExistConnections)
	}
}
