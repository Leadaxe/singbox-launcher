package state

import (
	"bytes"
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
// Единственный формат записи — v7 (SPEC 118 W1). Ветвления по версии схемы
// нет и быть не может: старые формы существуют только на чтении (Load
// роутит их в миграцию), а на диск состояние уходит всегда каноническим —
// иначе один и тот же state давал бы разные файлы в зависимости от того,
// откуда он приехал.
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

	// SPEC 117 (W4): обратного синка legacy → canonical больше нет. Save
	// сериализует ТОЛЬКО canonical (s.Sources/s.Directions/...); s.ParserConfig
	// — read-only Load-проекция, Save её не читает. Все мутации обязаны идти
	// в canonical-поля. SPEC 118 (W1): единственный формат записи — v7.
	s.Version = SchemaVersionV7

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

// MarshalV7 — состояние в форме v7, БЕЗ записи на диск и без мутаций
// (SPEC 118 Т10).
//
// Нужна отладочным поверхностям (`GET /state/full` и близнец машины). Без неё
// они отдавали Go-структуру `State` как есть: PascalCase-ключи, мёртвые
// легаси-поля (`Defaults`, `SelectableRuleStates`, `RulesLibraryMerged`,
// `DNSOptions:null`) и read-only Load-проекция `ParserConfig`. То есть ответ
// показывал ВНУТРЕННОСТИ загрузчика, а не состояние, и расходился с тем, что
// лежит в файле, — при отладке миграции v7 это худшая из возможных подмен.
//
// Форма следует за схемой без обязательств совместимости: это локальный
// интерфейс, а контракт переноса — бэкап 0.11.
func (s *State) MarshalV7() ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("state: MarshalV7 called on nil receiver")
	}
	return s.marshalDisk()
}

// marshalDisk — сериализация State в canonical (v7) shape (SPEC 118).
//
//	{
//	  "meta":          { version: 7, schema: "sources_v7", ... },
//	  "sources":       [ {kind, tag, enabled, ...} ],
//	  "directions":    [ ... ],
//	  "rules":         [ {kind, ref|id, enabled, body} ],
//	  "vars":          [ ... ],                                 // dns_* scalars
//	  "dns_options":   { servers: [...], rules: [...] },        // SPEC 056-R-N
//	  "warp_accounts": { ... }
//	}
//
// Legacy `s.CustomRules` / `s.DNSOptions` НЕ сериализуются — источник истины
// Rules / DNS. Легаси-ключей v6 нет ни в корне, ни внутри sources[]: их
// читает только вход миграции, и записывать их некуда (SPEC Т1).
func (s *State) marshalDisk() ([]byte, error) {
	out := diskStateV7{
		Meta: MetaSection{
			Version:   SchemaVersionV7,
			Schema:    SchemaNameV7,
			Comment:   s.Comment,
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
			UpdatedAt: s.UpdatedAt.Format(time.RFC3339),

			Target:         s.Target,
			TargetPlatform: s.TargetPlatform,
			TargetArch:     s.TargetArch,
		},
		Sources:      s.Sources,
		Directions:   s.Directions,
		Rules:        s.Rules,
		Vars:         s.Vars,
		DNSOptions:   s.DNS,
		WarpAccounts: s.WarpAccounts,
	}
	if out.Rules == nil {
		out.Rules = []Rule{}
	}
	if out.Sources == nil {
		out.Sources = []Source{}
	}
	if out.Directions == nil {
		out.Directions = []configtypes.Direction{}
	}
	// SetEscapeHTML(false): по умолчанию encoding/json экранирует «&», «<» и
	// «>» в & и подобное — защита для вставки JSON в HTML-страницу,
	// которая здесь не нужна. В state.json попадают URL подписок и строки
	// превью с query-параметрами, и в файле они превращались в нечитаемое
	// «members=11&outbounds[]=…». Значение при чтении то же самое, но
	// файл смотрят глазами.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	// Encode дописывает перевод строки — MarshalIndent его не давал, и
	// golden-тесты сравнивают точный байт-в-байт результат.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// hasReferencedOutbounds — true если хотя бы одно Направление в s.Directions
// имеет непустой Ref (referenced shape, SPEC 058).
func hasReferencedOutbounds(s *State) bool {
	for _, ob := range s.Directions {
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
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()
	_, err = io.Copy(dst, src)
	return err
}
