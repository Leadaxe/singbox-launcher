package state

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"singbox-launcher/core/config/configtypes"
	v5 "singbox-launcher/core/state/v5"
	v6 "singbox-launcher/core/state/v6"
)

// Save атомарно записывает s в path.
//
// Формат выбирается автоматически (SPEC 053):
//   - Если state содержит preset-ref правила (любая запись в RulesV6 с
//     kind=preset) → пишем v6-формат + создаём backup state.json.v5.bak
//     при первом upgrade.
//   - Иначе → v5-формат (текущее поведение, backward-compat).
//
// Auto-upgrade триггерится когда UI Phase 6 добавит preset-ref правило.
// До этого момента юзеры с pure inline/srs правилами остаются на v5.
//
// Save мутирует UpdatedAt текущим временем (UTC); CreatedAt — только если zero.
func (s *State) Save(path string) error {
	if s == nil {
		return fmt.Errorf("state: Save called on nil receiver")
	}
	now := time.Now().UTC()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.UpdatedAt = now

	// Sync legacy → v5 canonical перед сериализацией.
	syncConnectionsFromLegacy(s)

	useV6 := hasPresetRefs(s.RulesV6)

	if useV6 {
		s.Version = v6.SchemaVersion
		// Backup перед первым перезаписыванием с v5 на v6.
		if err := maybeBackupV5(path); err != nil {
			// Backup failure non-fatal — продолжаем save (warning логируется
			// callsite'ом, у нас здесь нет debuglog без расширения signature).
			_ = err
		}
	} else {
		s.Version = v5.SchemaVersion
	}

	var data []byte
	var err error
	if useV6 {
		data, err = s.marshalDiskV6()
	} else {
		data, err = s.marshalDisk()
	}
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("state: open %s: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("state: write %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("state: fsync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("state: close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("state: rename %s → %s: %w", tmp, path, err)
	}

	// Best-effort fsync на каталог. На Windows — no-op.
	if dirF, err := os.Open(dir); err == nil {
		_ = dirF.Sync()
		_ = dirF.Close()
	}
	return nil
}

// marshalDisk сериализует State в v5-форму:
//
//	{
//	  "meta":         { version:5, comment, created_at, updated_at },
//	  "connections":  { sources, outbounds, defaults },
//	  "config_params": [...],
//	  "custom_rules":  [...],
//	  "vars":          [...],
//	  "dns_options":   {...}
//	}
//
// Legacy-поля (id, parser_config, rules_library_merged, selectable_rule_states)
// в v5 не сериализуются.
func (s *State) marshalDisk() ([]byte, error) {
	out := struct {
		Meta         v5.MetaSection        `json:"meta"`
		Connections  v5.ConnectionsSection `json:"connections"`
		ConfigParams []ConfigParam         `json:"config_params"`
		CustomRules  []CustomRule          `json:"custom_rules"`
		Vars         []SettingVar          `json:"vars,omitempty"`
		DNSOptions   *DNSOptions           `json:"dns_options"`
	}{
		Meta: v5.MetaSection{
			Version:   SchemaVersion,
			Comment:   s.Comment,
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
			UpdatedAt: s.UpdatedAt.Format(time.RFC3339),
		},
		Connections:  s.Connections,
		ConfigParams: s.ConfigParams,
		CustomRules:  s.CustomRules,
		Vars:         s.Vars,
		DNSOptions:   s.DNSOptions,
	}
	if out.ConfigParams == nil {
		out.ConfigParams = []ConfigParam{}
	}
	if out.CustomRules == nil {
		out.CustomRules = []CustomRule{}
	}
	if out.Connections.Sources == nil {
		out.Connections.Sources = []Source{}
	}
	if out.Connections.Outbounds == nil {
		out.Connections.Outbounds = []configtypes.OutboundConfig{}
	}
	return json.MarshalIndent(out, "", "  ")
}

// marshalDiskV6 — сериализация State в v6-форму (SPEC 053 + SPEC 056-R-N).
//
//	{
//	  "meta":        { version: 6, schema: "presets_v1", ... },
//	  "connections": { ... },
//	  "rules":       [ {kind, ref|id, enabled, body} ],
//	  "vars":        [ ... ],                                  // dns_* scalars живут здесь
//	  "dns_options": {                                          // SPEC 056-R-N
//	    "servers": [ {kind:template|preset|user, tag|ref, enabled, ...body} ],
//	    "rules":   [ {kind:preset|user, ref|..., enabled, ...body} ]
//	  }
//	}
//
// legacy CustomRules / DNSOptions в v6 не сериализуются (источник — RulesV6 / DNSV6).
func (s *State) marshalDiskV6() ([]byte, error) {
	out := struct {
		Meta        v6.MetaSection        `json:"meta"`
		Connections v5.ConnectionsSection `json:"connections"`
		Rules       []v6.Rule             `json:"rules"`
		Vars        []SettingVar          `json:"vars,omitempty"`
		DNSOptions  v6.DNSOptions         `json:"dns_options"`
	}{
		Meta: v6.MetaSection{
			Version:   v6.SchemaVersion,
			Schema:    v6.SchemaName,
			Comment:   s.Comment,
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
			UpdatedAt: s.UpdatedAt.Format(time.RFC3339),
		},
		Connections: s.Connections,
		Rules:       s.RulesV6,
		Vars:        s.Vars,
		DNSOptions:  s.DNS,
	}
	if out.Rules == nil {
		out.Rules = []v6.Rule{}
	}
	if out.Connections.Sources == nil {
		out.Connections.Sources = []Source{}
	}
	if out.Connections.Outbounds == nil {
		out.Connections.Outbounds = []configtypes.OutboundConfig{}
	}
	return json.MarshalIndent(out, "", "  ")
}

// hasPresetRefs — true если state имеет хотя бы одно правило kind=preset.
// Триггер для v6 save.
func hasPresetRefs(rules []v6.Rule) bool {
	for _, r := range rules {
		if r.Kind == v6.RuleKindPreset {
			return true
		}
	}
	return false
}

// maybeBackupV5 — копирует существующий state.json в state.json.v5.bak,
// если backup ещё не создан И если текущий файл реально v5-формата.
// Идемпотентно: повторные вызовы — no-op.
func maybeBackupV5(path string) error {
	backupPath := path + ".v5.bak"
	if _, err := os.Stat(backupPath); err == nil {
		// Backup уже есть.
		return nil
	}
	src, err := os.Open(path)
	if err != nil {
		// Файла нет — fresh install, nothing to backup.
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer src.Close()

	// Проверим что это действительно v5 (не v6 уже).
	head := make([]byte, 4096)
	n, _ := src.Read(head)
	head = head[:n]
	if v6.IsV6(head) {
		return nil
	}
	if _, err := src.Seek(0, 0); err != nil {
		return err
	}

	dst, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}
