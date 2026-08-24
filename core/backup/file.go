package backup

// Файловый слой LX Backup (SPEC 103, фаза 4).

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// MaxFileBytes — потолок читаемого файла (8 MiB).
//
// Бэкап — это настройки, а не данные: реальный файл измеряется десятками
// килобайт. Потолок защищает от попытки скормить импортёру произвольный
// большой файл, а не ограничивает пользователя.
const MaxFileBytes = 8 << 20

// WriteFile сохраняет бэкап.
//
// Пишется с отступами: файл читают и правят руками, а компактный JSON в одну
// строку делает это невозможным. Запись атомарная — прерванная запись не
// должна оставить обрезанный файл вместо прежнего.
func WriteFile(path string, b *Backup) error {
	if b == nil {
		return fmt.Errorf("nil backup")
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("backup serialization: %w", err)
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename to %s: %w", path, err)
	}
	return nil
}

// ReadFile читает и разбирает бэкап.
//
// Неизвестные ключи вне extensions — не ошибка, но и не молчание: они
// возвращаются списком, чтобы вызывающий предъявил их пользователю
// (BACKUP.md §1, default-deny).
func ReadFile(path string) (*Backup, []Warning, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("backup file: %w", err)
	}
	if info.Size() > MaxFileBytes {
		return nil, nil, fmt.Errorf("backup file exceeds %d bytes", MaxFileBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse разбирает содержимое бэкапа.
func Parse(data []byte) (*Backup, []Warning, error) {
	var b Backup
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, nil, fmt.Errorf("backup parse: %w", err)
	}
	if b.LxBackup == 0 {
		return nil, nil, fmt.Errorf("not an LX Backup file: lx_backup field missing")
	}
	return &b, unknownRootKeys(data), nil
}

// unknownRootKeys перечисляет ключи корня, которых нет в нашей модели.
//
// Мы не отвергаем такой файл: бэкап более новой минорной версии обязан
// читаться. Но и применять неизвестное молча нельзя — пользователь должен
// знать, что часть файла не учтена.
func unknownRootKeys(data []byte) []Warning {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	known := map[string]bool{
		"lx_backup": true, "exported_by": true, "exported_at": true,
		"subscriptions": true, "servers": true, "rules": true,
		"directions": true, // SPEC 104, схема v1.1
		"dns":        true, "vars": true, "route": true, "warp": true,
		"extensions": true,
	}
	var warns []Warning
	for key := range raw {
		if !known[key] {
			warns = append(warns, Warning{WarnBackupUnknownField, key})
		}
	}
	return warns
}

// SuggestFileName — имя файла по умолчанию для диалога сохранения.
func SuggestFileName(stamp string) string {
	stamp = strings.TrimSpace(stamp)
	if stamp == "" {
		return "lx-backup.json"
	}
	return "lx-backup-" + stamp + ".json"
}
