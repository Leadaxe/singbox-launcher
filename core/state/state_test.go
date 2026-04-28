package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"singbox-launcher/core/config/configtypes"
)

// TestNew — конструктор отдаёт state с актуальной версией и заполненным временем.
func TestNew(t *testing.T) {
	s := New()
	if s.Version != SchemaVersion {
		t.Fatalf("Version: want %d, got %d", SchemaVersion, s.Version)
	}
	if s.CreatedAt.IsZero() || s.UpdatedAt.IsZero() {
		t.Fatalf("times must be set: %+v %+v", s.CreatedAt, s.UpdatedAt)
	}
	if s.ConfigParams == nil || s.CustomRules == nil {
		t.Fatalf("default slices must be non-nil")
	}
}

// TestLoad_NotFound — отсутствующий файл даёт ErrNotFound, а не другую ошибку.
func TestLoad_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(filepath.Join(dir, "missing.json"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// TestLoad_EmptyFile — пустой payload — ошибка с понятным сообщением.
func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "state.json")
	if err := os.WriteFile(p, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(p)
	if err == nil {
		t.Fatalf("want error on empty file, got nil")
	}
}

// TestLoad_V4Minimal — читает базовый v4-снимок и мигрирует в in-memory v5.
//
// Контракт SPEC 052: после Load v4 файл считается v5 (auto-migration).
// Legacy ParserConfig view заполнен из мигрированных Connections для
// backward-compat callsite'ов.
func TestLoad_V4Minimal(t *testing.T) {
	s, err := Load("testdata/v4_minimal.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Version != SchemaVersion {
		t.Fatalf("Version: want %d (migrated), got %d", SchemaVersion, s.Version)
	}
	// ID — legacy field; v4-фикстура содержит его, в памяти оставляем.
	if s.ID != "test-state" {
		t.Fatalf("ID: want 'test-state', got %q", s.ID)
	}
	// Connections.Sources — должен содержать одну subscription.
	if got := len(s.Connections.Sources); got != 1 {
		t.Fatalf("Connections.Sources: want 1, got %d", got)
	}
	if s.Connections.Sources[0].Type != SourceTypeSubscription {
		t.Fatalf("Source[0].Type: %q", s.Connections.Sources[0].Type)
	}
	if s.Connections.Sources[0].URL != "https://example.com/sub-a" {
		t.Fatalf("URL mismatch: %+v", s.Connections.Sources[0])
	}
	// Legacy view тоже заполнен.
	if got := len(s.ParserConfig.ParserConfig.Proxies); got != 1 {
		t.Fatalf("ParserConfig.Proxies: want 1, got %d", got)
	}
	if s.ParserConfig.ParserConfig.Proxies[0].Source != "https://example.com/sub-a" {
		t.Fatalf("Source mismatch: %+v", s.ParserConfig.ParserConfig.Proxies[0])
	}
	if !s.RulesLibraryMerged {
		t.Fatalf("RulesLibraryMerged should be true (preserved from v4)")
	}
	if len(s.Vars) != 2 {
		t.Fatalf("Vars: want 2, got %d", len(s.Vars))
	}
	if s.DNSOptions == nil {
		t.Fatalf("DNSOptions must be present")
	}
	wantUpdated := time.Date(2026, 4, 26, 20, 0, 0, 0, time.UTC)
	if !s.UpdatedAt.Equal(wantUpdated) {
		t.Fatalf("UpdatedAt: want %v, got %v", wantUpdated, s.UpdatedAt)
	}
}

// TestLoad_V3LegacyShapes — старые формы selectable/custom rules мигрируются
// в v5. После Load Version=5, но SelectableRuleStates и custom_rules
// сохраняются (custom_rules в новом формате, selectable_rule_states только
// в памяти для UI-кода).
func TestLoad_V3LegacyShapes(t *testing.T) {
	s, err := Load("testdata/v3_legacy_rules.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Version != SchemaVersion {
		t.Fatalf("Version: want %d (post-migration), got %d", SchemaVersion, s.Version)
	}

	if got := len(s.SelectableRuleStates); got != 1 {
		t.Fatalf("SelectableRuleStates: want 1 after migration, got %d", got)
	}
	sr := s.SelectableRuleStates[0]
	if sr.Label != "block-ads" || !sr.Enabled || sr.SelectedOutbound != "block" {
		t.Fatalf("selectable migration wrong: %+v", sr)
	}

	if got := len(s.CustomRules); got != 1 {
		t.Fatalf("CustomRules: want 1 after migration, got %d", got)
	}
	cr := s.CustomRules[0]
	if cr.Label != "private-net" || cr.Type != "ips" || !cr.Enabled {
		t.Fatalf("custom migration wrong: %+v", cr)
	}
	if cr.Rule == nil {
		t.Fatalf("custom rule body lost during migration")
	}
}

// TestSave_RoundTrip — Save → Load даёт эквивалентный объект.
//
// SPEC 052: ID не сериализуется в v5 (snapshot-имена живут в имени файла).
// Comment, CreatedAt, UpdatedAt — в meta. ParserConfig — derived view,
// заполняется на Load из Connections.
func TestSave_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	original := &State{
		Version:      SchemaVersion,
		Comment:      "roundtrip test",
		CreatedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ConfigParams: []ConfigParam{{Name: "route_final", Value: "vpn-1"}},
		Vars:         []SettingVar{{Name: "log_level", Value: "info"}},
		CustomRules:  []CustomRule{},
	}
	original.ParserConfig.ParserConfig.Version = 4
	original.ParserConfig.ParserConfig.Proxies = []configtypes.ProxySource{
		{Source: "https://x/sub", TagPrefix: "[X] "},
	}
	original.ParserConfig.ParserConfig.Outbounds = []configtypes.OutboundConfig{}

	if err := original.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Comment != original.Comment {
		t.Fatalf("Comment mismatch: %q vs %q", loaded.Comment, original.Comment)
	}
	if loaded.Version != SchemaVersion {
		t.Fatalf("Version: want %d, got %d", SchemaVersion, loaded.Version)
	}
	if len(loaded.Connections.Sources) != 1 {
		t.Fatalf("Connections.Sources: want 1, got %d", len(loaded.Connections.Sources))
	}
	if loaded.Connections.Sources[0].Tag == nil || loaded.Connections.Sources[0].Tag.Prefix != "[X] " {
		t.Fatalf("Tag prefix not preserved: %+v", loaded.Connections.Sources[0].Tag)
	}
	// Legacy ParserConfig — derived from Connections at Load.
	if len(loaded.ParserConfig.ParserConfig.Proxies) != 1 {
		t.Fatalf("Proxies count")
	}
	if loaded.ParserConfig.ParserConfig.Proxies[0].TagPrefix != "[X] " {
		t.Fatalf("TagPrefix not preserved in legacy view")
	}
	// CreatedAt должен быть сохранён (был не-zero перед Save).
	if !loaded.CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("CreatedAt: want %v, got %v", original.CreatedAt, loaded.CreatedAt)
	}
	// UpdatedAt — обновился на текущий момент.
	if loaded.UpdatedAt.Before(original.CreatedAt) {
		t.Fatalf("UpdatedAt not refreshed: %v vs %v", loaded.UpdatedAt, original.CreatedAt)
	}
	// Source.ID — должен быть auto-сгенерирован на Save.
	if loaded.Connections.Sources[0].ID == "" {
		t.Fatalf("Source.ID was not auto-generated on Save")
	}
}

// TestLoadSave_IdempotentV5 — load v5 → save → load → save → ... файл
// сходится после первого save (ID stabilization). Гарантирует что v5
// round-trip не теряет данные и не дрейфует.
func TestLoadSave_IdempotentV5(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Старт с v4-фикстуры — миграция произойдёт на первом Load.
	src, err := os.ReadFile("testdata/v4_minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	// First load+save: v4 → v5.
	s1, err := Load(path)
	if err != nil {
		t.Fatalf("Load 1: %v", err)
	}
	if err := s1.Save(path); err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Second load+save: v5 → v5 (только UpdatedAt меняется).
	s2, err := Load(path)
	if err != nil {
		t.Fatalf("Load 2: %v", err)
	}
	// Source.ID должен сохраниться между save'ами.
	if len(s1.Connections.Sources) != 1 || len(s2.Connections.Sources) != 1 {
		t.Fatalf("source count drift: s1=%d s2=%d", len(s1.Connections.Sources), len(s2.Connections.Sources))
	}
	if s1.Connections.Sources[0].ID != s2.Connections.Sources[0].ID {
		t.Errorf("Source.ID changed across save/load: %q → %q",
			s1.Connections.Sources[0].ID, s2.Connections.Sources[0].ID)
	}

	if err := s2.Save(path); err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// updated_at будет отличаться, но всё остальное должно быть identical.
	// Грубая проверка: размер совпадает в пределах нескольких байт (timestamp).
	if abs(len(first)-len(second)) > 30 {
		t.Errorf("v5 round-trip drift: first=%d bytes, second=%d bytes", len(first), len(second))
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// TestSave_AtomicLeavesNoTmpOnSuccess — после успешного Save в директории
// нет .tmp файла.
func TestSave_AtomicLeavesNoTmpOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	if err := New().Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == "state.json.tmp" {
			t.Fatalf("leftover .tmp file: %v", entries)
		}
	}
}

// TestSave_NilReceiver — программная ошибка вызывающего, понятная диагностика.
func TestSave_NilReceiver(t *testing.T) {
	var s *State
	err := s.Save("/tmp/nonexistent")
	if err == nil {
		t.Fatalf("want error on nil receiver")
	}
}

// TestSave_BumpsVersionToCurrent — даже если state в памяти имеет старую
// версию (например, был прочитан из v3-файла), Save пишет SchemaVersion.
func TestSave_BumpsVersionToCurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := New()
	s.Version = 3 // simulating загрузку из v3
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Version != SchemaVersion {
		t.Fatalf("Version after Save: want %d, got %d", SchemaVersion, loaded.Version)
	}
}

// TestDiff_Empty — два одинаковых state дают пустой Diff.
func TestDiff_Empty(t *testing.T) {
	a := buildSampleState()
	b := buildSampleState()
	d := DiffStates(a, b)
	if !d.IsEmpty() {
		t.Fatalf("expected empty diff, got %+v", d)
	}
	if d.AffectsParser() || d.AffectsTemplate() {
		t.Fatalf("affects flags must be false on identical states")
	}
}

// TestDiff_VsNilPrev — nil в качестве prev = всё новое.
func TestDiff_VsNilPrev(t *testing.T) {
	cur := buildSampleState()
	d := DiffStates(nil, cur)
	if d.IsEmpty() {
		t.Fatalf("nil prev → diff must be non-empty")
	}
	if !d.ProxiesChanged {
		t.Fatalf("ProxiesChanged should be true")
	}
}

// TestDiff_ProxiesAdded — добавление подписки → ProxiesChanged → AffectsParser.
func TestDiff_ProxiesAdded(t *testing.T) {
	a := buildSampleState()
	b := buildSampleState()
	b.ParserConfig.ParserConfig.Proxies = append(b.ParserConfig.ParserConfig.Proxies,
		configtypes.ProxySource{Source: "https://new.example/sub"})

	d := DiffStates(a, b)
	if !d.ProxiesChanged {
		t.Fatalf("ProxiesChanged should be true")
	}
	if !d.AffectsParser() {
		t.Fatalf("AffectsParser should be true")
	}
	if d.AffectsTemplate() {
		t.Fatalf("AffectsTemplate should be false (only proxies changed)")
	}
}

// TestDiff_VarsChanged — правка settings vars → VarsChanged → AffectsTemplate.
func TestDiff_VarsChanged(t *testing.T) {
	a := buildSampleState()
	b := buildSampleState()
	b.Vars = []SettingVar{
		{Name: "log_level", Value: "debug"}, // было "info"
	}

	d := DiffStates(a, b)
	if !d.VarsChanged {
		t.Fatalf("VarsChanged should be true")
	}
	if !d.AffectsTemplate() {
		t.Fatalf("AffectsTemplate should be true")
	}
	if d.AffectsParser() {
		t.Fatalf("AffectsParser should be false")
	}
}

// TestDiff_VarsReordered — vars в другом порядке считаются равными
// (сравнение по name, не по позиции).
func TestDiff_VarsReordered(t *testing.T) {
	a := &State{Vars: []SettingVar{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}}}
	b := &State{Vars: []SettingVar{{Name: "b", Value: "2"}, {Name: "a", Value: "1"}}}
	d := DiffStates(a, b)
	if d.VarsChanged {
		t.Fatalf("Vars reorder must not be a change: %+v", d)
	}
}

// TestDiff_BothFlags — одновременное изменение proxies и vars поднимает оба.
func TestDiff_BothFlags(t *testing.T) {
	a := buildSampleState()
	b := buildSampleState()
	b.ParserConfig.ParserConfig.Proxies[0].TagPrefix = "[CHANGED] "
	b.Vars = []SettingVar{{Name: "log_level", Value: "warn"}}

	d := DiffStates(a, b)
	if !d.AffectsParser() {
		t.Fatalf("AffectsParser must be true")
	}
	if !d.AffectsTemplate() {
		t.Fatalf("AffectsTemplate must be true")
	}
}

// TestDiff_DNSOptions — изменение DNS rules.
func TestDiff_DNSOptions(t *testing.T) {
	a := buildSampleState()
	b := buildSampleState()
	b.DNSOptions = &DNSOptions{
		Servers: []json.RawMessage{json.RawMessage(`{"address":"8.8.8.8"}`)},
	}

	d := DiffStates(a, b)
	if !d.DNSOptionsChanged {
		t.Fatalf("DNSOptionsChanged should be true")
	}
	if !d.AffectsTemplate() {
		t.Fatalf("AffectsTemplate should be true")
	}
}

// TestDiff_CustomRules — добавление custom rule.
func TestDiff_CustomRules(t *testing.T) {
	a := buildSampleState()
	b := buildSampleState()
	b.CustomRules = append(b.CustomRules, CustomRule{
		Label:   "new-rule",
		Type:    RuleTypeIPS,
		Enabled: true,
	})

	d := DiffStates(a, b)
	if !d.CustomRulesChanged {
		t.Fatalf("CustomRulesChanged should be true")
	}
	if !d.AffectsTemplate() {
		t.Fatalf("AffectsTemplate should be true")
	}
}

// TestDiff_NilCur — nil cur — пустой Diff (программная ошибка вызывающего).
func TestDiff_NilCur(t *testing.T) {
	a := buildSampleState()
	d := DiffStates(a, nil)
	if !d.IsEmpty() {
		t.Fatalf("nil cur must yield empty diff")
	}
}

// TestIsKnownRuleType — проверка enum.
func TestIsKnownRuleType(t *testing.T) {
	for _, k := range []string{RuleTypeIPS, RuleTypeURLs, RuleTypeProcesses, RuleTypeSRS, RuleTypeRaw} {
		if !IsKnownRuleType(k) {
			t.Fatalf("%q must be known", k)
		}
	}
	if IsKnownRuleType("nonsense") {
		t.Fatalf("'nonsense' must not be known")
	}
}

// --- helpers ---

func buildSampleState() *State {
	s := &State{
		Version:            SchemaVersion,
		ID:                 "sample",
		ConfigParams:       []ConfigParam{{Name: "route_final", Value: "vpn-1"}},
		Vars:               []SettingVar{{Name: "log_level", Value: "info"}},
		CustomRules:        []CustomRule{},
		RulesLibraryMerged: true,
		DNSOptions:         &DNSOptions{Servers: []json.RawMessage{}, Rules: []json.RawMessage{}},
	}
	s.ParserConfig.ParserConfig.Version = 4
	s.ParserConfig.ParserConfig.Proxies = []configtypes.ProxySource{
		{Source: "https://a.example/sub"},
	}
	s.ParserConfig.ParserConfig.Outbounds = []configtypes.OutboundConfig{}
	return s
}
