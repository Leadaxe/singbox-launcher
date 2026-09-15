package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// v8-файл dev-сборки волны 1 SPEC 127: идентичность подписки лежит плоскими
// ключами. Загрузка обязана перенести их в identity, первое сохранение — не
// потерять, повторный круг — ничего не менять.
//
// Три подписки разводят правило «на ключ»: у первой identity нет вовсе (всё
// переносится), у второй identity уже несёт UA (плоский UA отбрасывается,
// недостающий флаг переносится), у третьей плоские значения пустые (переносить
// нечего). Папка с плоским UA — не подписка: у неё ключ не читается.
func TestLoadV8LiftsFlatSubscriptionIdentity(t *testing.T) {
	const file = `{
  "meta": {"version": 8, "schema": "sources_v8", "created_at": "2026-09-01T00:00:00Z", "updated_at": "2026-09-01T00:00:00Z"},
  "sources": [
    {"kind": "subscription", "id": "01J0000000000000000000SUBA", "enabled": false, "url": "https://example.invalid/a",
     "user_agent": "Happ/3.3.6", "hwid": "hw-a", "send_hwid": false, "hash_device_model": true},
    {"kind": "subscription", "id": "01J0000000000000000000SUBB", "enabled": true, "url": "https://example.invalid/b",
     "user_agent": "Old/1.0", "send_hwid": true, "identity": {"user_agent": "New/2.0"}},
    {"kind": "subscription", "id": "01J0000000000000000000SUBC", "enabled": true, "url": "https://example.invalid/c",
     "user_agent": "", "send_hwid": null},
    {"kind": "folder", "id": "01J000000000000000000FOLDR", "enabled": true, "name": "F", "user_agent": "Folder/1.0"}
  ],
  "directions": [],
  "rules": [],
  "dns": {}
}`
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte(file), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	byID := func(s *State, id string) *Source {
		for i := range s.Sources {
			if s.Sources[i].ID == id {
				return &s.Sources[i]
			}
		}
		t.Fatalf("источник %s потерян", id)
		return nil
	}
	check := func(s *State, stage string) {
		t.Helper()
		a := byID(s, "01J0000000000000000000SUBA")
		if a.IdentityUserAgent() != "Happ/3.3.6" || a.IdentityHWID() != "hw-a" {
			t.Errorf("%s: подписка A: UA %q, HWID %q — плоские ключи не перенесены", stage, a.IdentityUserAgent(), a.IdentityHWID())
		}
		if v := a.IdentitySendHWID(); v == nil || *v {
			t.Errorf("%s: подписка A: send_hwid=false потерян (%v)", stage, v)
		}
		if v := a.IdentityHashDeviceModel(); v == nil || !*v {
			t.Errorf("%s: подписка A: hash_device_model=true потерян (%v)", stage, v)
		}
		b := byID(s, "01J0000000000000000000SUBB")
		if b.IdentityUserAgent() != "New/2.0" {
			t.Errorf("%s: подписка B: UA %q — плоский ключ перебил identity", stage, b.IdentityUserAgent())
		}
		if v := b.IdentitySendHWID(); v == nil || !*v {
			t.Errorf("%s: подписка B: send_hwid, которого не было в identity, не перенесён (%v)", stage, v)
		}
		if c := byID(s, "01J0000000000000000000SUBC"); c.Identity != nil {
			t.Errorf("%s: подписка C: пустые плоские значения завели identity %+v", stage, c.Identity)
		}
		if f := byID(s, "01J000000000000000000FOLDR"); f.Identity != nil {
			t.Errorf("%s: папка получила identity из плоского ключа: %+v", stage, f.Identity)
		}
	}
	check(s, "Load")

	// Первое сохранение пишет identity и не пишет плоских ключей.
	saved := filepath.Join(dir, "saved.json")
	if err := s.Save(saved); err != nil {
		t.Fatalf("Save: %v", err)
	}
	first, err := os.ReadFile(saved)
	if err != nil {
		t.Fatalf("read saved: %v", err)
	}
	var doc struct {
		Sources []map[string]json.RawMessage `json:"sources"`
	}
	if err := json.Unmarshal(first, &doc); err != nil {
		t.Fatalf("parse saved: %v", err)
	}
	for i, src := range doc.Sources {
		for _, k := range flatIdentityKeys {
			if _, ok := src[k]; ok {
				t.Errorf("sources[%d]: плоский ключ %s пережил Save", i, k)
			}
		}
	}
	if !strings.Contains(string(first), `"Happ/3.3.6"`) {
		t.Fatalf("UA подписки A не доехал до диска:\n%s", first)
	}

	// Круг Load → Save на уже перенесённом файле байт-в-байт.
	again, err := Load(saved)
	if err != nil {
		t.Fatalf("Load saved: %v", err)
	}
	check(again, "Load после Save")
	resaved := filepath.Join(dir, "resaved.json")
	if err := again.Save(resaved); err != nil {
		t.Fatalf("Save again: %v", err)
	}
	second, err := os.ReadFile(resaved)
	if err != nil {
		t.Fatalf("read resaved: %v", err)
	}
	// meta.updated_at штампуется сохранением — сравнивается остальное.
	strip := func(b []byte) string {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("parse: %v", err)
		}
		delete(m, "meta")
		out, _ := json.Marshal(m)
		return string(out)
	}
	if strip(first) != strip(second) {
		t.Errorf("повторный круг Load→Save изменил файл:\n%s\n---\n%s", first, second)
	}
}
