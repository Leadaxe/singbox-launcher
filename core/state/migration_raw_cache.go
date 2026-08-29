// File migration_raw_cache.go — ЧТЕНИЕ упразднённого raw-кэша подписок.
//
// SPEC 118 W5 («смерть легаси»): raw-кэша больше нет — тела подписок живут
// материализованными узлами (`nodes[]`). Но одноразовая миграция v6→v7
// обязана этот кэш ПРОЧИТАТЬ (шаг 1) и УДАЛИТЬ (шаг 8), иначе состояния
// живых установок приедут в v7 без единого узла.
//
// Поэтому здесь остались ровно три вещи — чтение, путь и удаление, — и они
// живут в файле миграции, а не в общем API пакета: писателя (WriteRawBody),
// сборщика сирот (DeleteOrphans) и экспортируемых имён больше нет. Это
// санкционированное исключение grep-инвариантов SPEC §4.A («читатели
// миграции — единственное исключение»).
package state

import (
	"fmt"
	"os"
	"path/filepath"
)

// legacyRawSuffix — расширение файлов упразднённого кэша: <id>.raw.
const legacyRawSuffix = ".raw"

// validateLegacyRawID — id безопасен как имя файла (Crockford-base32 плюс
// '-'/'_' ручных тестовых id): защита от path traversal.
func validateLegacyRawID(id string) error {
	if id == "" {
		return fmt.Errorf("state: empty source id")
	}
	if len(id) > 128 {
		return fmt.Errorf("state: source id too long (%d chars)", len(id))
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
			return fmt.Errorf("state: source id %q has forbidden char %q", id, c)
		}
	}
	return nil
}

// legacyRawPath — путь <subsDir>/<id>.raw.
func legacyRawPath(subsDir, id string) (string, error) {
	if err := validateLegacyRawID(id); err != nil {
		return "", err
	}
	return filepath.Join(subsDir, id+legacyRawSuffix), nil
}

// readLegacyRawBody — тело подписки из упразднённого кэша. Файла нет →
// ошибка: миграция трактует это как «узлы появятся после первого обновления».
func readLegacyRawBody(subsDir, id string) ([]byte, error) {
	target, err := legacyRawPath(subsDir, id)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("state: read legacy raw cache %s: %w", target, err)
	}
	return body, nil
}
