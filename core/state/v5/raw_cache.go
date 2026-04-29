package v5

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// rawSuffix — расширение файлов кеша. id.raw.
const rawSuffix = ".raw"

// rawTmpSuffix — расширение временного файла для atomic-write.
const rawTmpSuffix = ".raw.tmp"

// rawDirMode / rawFileMode — права на bin/subscriptions/ и его файлы.
// На Windows значения нерелевантны (Go смотрит только на 0200 для readonly).
const (
	rawDirMode  os.FileMode = 0o755
	rawFileMode os.FileMode = 0o644
)

// validateID проверяет что id безопасен для использования как имя файла:
// только Crockford-base32 символы (исключает path traversal, separator'ы).
//
// MakeULID генерирует ровно такие id; ручные тесты могут передавать
// "id-1" — для этого тоже допускаем '-' и нижний регистр.
func validateID(id string) error {
	if id == "" {
		return fmt.Errorf("v5.raw_cache: empty source id")
	}
	if len(id) > 128 {
		return fmt.Errorf("v5.raw_cache: source id too long (%d chars)", len(id))
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'A' && c <= 'Z',
			c >= 'a' && c <= 'z',
			c >= '0' && c <= '9',
			c == '-', c == '_':
			// allowed
		default:
			return fmt.Errorf("v5.raw_cache: source id %q has forbidden char %q", id, c)
		}
	}
	return nil
}

// rawPath возвращает абсолютный путь bin/subscriptions/<id>.raw
// (или error если id невалиден).
func rawPath(subsDir, id string) (string, error) {
	if err := validateID(id); err != nil {
		return "", err
	}
	return filepath.Join(subsDir, id+rawSuffix), nil
}

// WriteRawBody атомарно записывает body в bin/subscriptions/<id>.raw.
//
// Порядок:
//  1. ensure subsDir (mkdir -p);
//  2. write to <id>.raw.tmp;
//  3. fsync, close;
//  4. os.Rename → <id>.raw (атомарно на одной FS).
//
// На любой ошибке .tmp убирается, оригинальный <id>.raw (если был)
// остаётся целым.
func WriteRawBody(subsDir, id string, body []byte) error {
	target, err := rawPath(subsDir, id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(subsDir, rawDirMode); err != nil {
		return fmt.Errorf("v5.raw_cache: mkdir %s: %w", subsDir, err)
	}
	tmp := target + ".tmp" // <id>.raw.tmp

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, rawFileMode)
	if err != nil {
		return fmt.Errorf("v5.raw_cache: open %s: %w", tmp, err)
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("v5.raw_cache: write %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("v5.raw_cache: fsync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("v5.raw_cache: close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("v5.raw_cache: rename %s → %s: %w", tmp, target, err)
	}
	return nil
}

// ReadRawBody читает bin/subscriptions/<id>.raw. Возвращает (nil, ErrRawNotFound)
// если файла нет — вызывающий обычно интерпретирует это как «нужен Update».
func ReadRawBody(subsDir, id string) ([]byte, error) {
	target, err := rawPath(subsDir, id)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrRawNotFound
		}
		return nil, fmt.Errorf("v5.raw_cache: read %s: %w", target, err)
	}
	return body, nil
}

// ErrRawNotFound — raw body для данного source id не существует на диске.
var ErrRawNotFound = fmt.Errorf("v5.raw_cache: raw body not found")

// DeleteRawBody удаляет bin/subscriptions/<id>.raw, если он есть.
// Отсутствие файла — не ошибка.
func DeleteRawBody(subsDir, id string) error {
	target, err := rawPath(subsDir, id)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("v5.raw_cache: remove %s: %w", target, err)
	}
	return nil
}

// ListRawBodyIDs возвращает список id'ов всех .raw файлов в subsDir.
// Если каталог не существует — возвращает пустой список без ошибки.
func ListRawBodyIDs(subsDir string) ([]string, error) {
	entries, err := os.ReadDir(subsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("v5.raw_cache: readdir %s: %w", subsDir, err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Игнорируем .tmp (мусор от crash mid-write).
		if strings.HasSuffix(name, rawTmpSuffix) {
			continue
		}
		if !strings.HasSuffix(name, rawSuffix) {
			continue
		}
		id := strings.TrimSuffix(name, rawSuffix)
		// Defensive: пропускаем файлы с невалидными именами
		// (artefacts чужих процессов / corrupt FS).
		if err := validateID(id); err != nil {
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

// DeleteOrphans удаляет .raw файлы, чьих id нет в knownIDs.
// Используется для lazy GC: при каждом Update'е чистим файлы удалённых
// source'ов. Возвращает список удалённых id (для логов/метрик).
//
// Также убирает .tmp-мусор (orphan from crashed write).
func DeleteOrphans(subsDir string, knownIDs []string) ([]string, error) {
	knownSet := make(map[string]struct{}, len(knownIDs))
	for _, id := range knownIDs {
		knownSet[id] = struct{}{}
	}

	entries, err := os.ReadDir(subsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("v5.raw_cache: readdir %s: %w", subsDir, err)
	}

	deleted := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()

		// orphan .tmp → удалить безусловно
		if strings.HasSuffix(name, rawTmpSuffix) {
			_ = os.Remove(filepath.Join(subsDir, name))
			continue
		}

		if !strings.HasSuffix(name, rawSuffix) {
			continue
		}
		id := strings.TrimSuffix(name, rawSuffix)
		if _, ok := knownSet[id]; ok {
			continue
		}
		// id не в known → orphan, удаляем. bin/subscriptions/ —
		// managed cache: содержимое должно быть консистентно state'у,
		// чужих файлов тут не предполагается.
		if err := os.Remove(filepath.Join(subsDir, name)); err == nil {
			deleted = append(deleted, id)
		}
	}
	return deleted, nil
}
