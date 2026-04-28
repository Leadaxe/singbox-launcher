package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"singbox-launcher/core/config/configtypes"
	v5 "singbox-launcher/core/state/v5"
)

// Save атомарно записывает s в path в v5-формате:
//  1. SyncConnectionsFromLegacy — обновляет Connections.Sources на основе
//     текущего ParserConfig.Proxies, сохраняя ID/Meta существующих source'ов
//     где возможно (match по URL для subscription, по URI для server);
//  2. сериализует s в v5-форму;
//  3. atomic write через .tmp + Rename.
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
	s.Version = SchemaVersion

	// Sync legacy → v5 canonical перед сериализацией.
	syncConnectionsFromLegacy(s)

	data, err := s.marshalDisk()
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
