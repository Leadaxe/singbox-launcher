package state

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"singbox-launcher/core/config/configtypes"
)

// Save атомарно записывает s в path.
//
// SPEC 060 Phase 5: single write path. После collapse v5/v6 namespaces и
// migration с SPEC 053/056/058 все state'ы пишутся в canonical (v6) shape.
// `useV6` gate и dual marshalDisk удалены.
//
// SPEC 058-R-N: backup перед первым перезаписыванием когда outbounds содержат
// referenced entries (post-migration shape). Lossless rollback гарантирован.
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

	// Sync legacy → canonical перед сериализацией.
	syncConnectionsFromLegacy(s)

	s.Version = SchemaVersionV6

	// SPEC 058-R-N: backup перед первым перезаписыванием когда outbounds
	// содержат referenced entries (post-migration shape). Gate idempotent
	// (maybeBackupSPEC058 skip если .pre-058.bak уже есть) — backup создаётся
	// единственный раз. Lossless rollback гарантирован.
	if hasReferencedOutbounds(s) {
		if err := maybeBackupSPEC058(path); err != nil {
			_ = err // non-fatal
		}
	}

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

// marshalDisk — сериализация State в canonical (v6) shape (SPEC 053 + SPEC 056-R-N).
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
// SPEC 060 Phase 5: единственный write path — `marshalDiskV6` rename'нут в
// `marshalDisk`, старый v5-marshaller удалён. Legacy `s.CustomRules` /
// `s.DNSOptions` НЕ сериализуются — источник истины Rules / DNS.
func (s *State) marshalDisk() ([]byte, error) {
	out := diskStateV6{
		Meta: MetaSection{
			Version:   SchemaVersionV6,
			Schema:    SchemaName,
			Comment:   s.Comment,
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
			UpdatedAt: s.UpdatedAt.Format(time.RFC3339),
		},
		Connections: s.Connections,
		Rules:       s.Rules,
		Vars:        s.Vars,
		DNSOptions:  s.DNS,
	}
	if out.Rules == nil {
		out.Rules = []Rule{}
	}
	if out.Connections.Sources == nil {
		out.Connections.Sources = []Source{}
	}
	if out.Connections.Outbounds == nil {
		out.Connections.Outbounds = []configtypes.OutboundConfig{}
	}
	return json.MarshalIndent(out, "", "  ")
}

// hasReferencedOutbounds — true если хотя бы один outbound в state.Connections.Outbounds
// имеет непустой Ref (referenced shape, SPEC 058).
func hasReferencedOutbounds(s *State) bool {
	for _, ob := range s.Connections.Outbounds {
		if ob.Ref != "" {
			return true
		}
	}
	return false
}

// maybeBackupSPEC058 — копирует существующий state.json в state.json.pre-058.bak,
// если backup ещё не создан. Создаётся однократно перед первым перезаписыванием
// после миграции в SPEC 058 referenced shape (Lossless rollback гарантирован —
// юзер может вернуть .bak → state.json и установить предыдущий build).
//
// Идемпотентно: повторные вызовы — no-op.
func maybeBackupSPEC058(path string) error {
	backupPath := path + ".pre-058.bak"
	if _, err := os.Stat(backupPath); err == nil {
		return nil // backup уже есть
	}
	src, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // fresh install
		}
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}
