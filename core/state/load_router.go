package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// ErrNotFound — state-файл не существует. Вызывающий обычно интерпретирует
// это как «свежая установка», а не как ошибку.
var ErrNotFound = errors.New("state: file not found")

// Load читает state.json по пути path.
//
// Поведение:
//   - файл отсутствует → ErrNotFound;
//   - v5 (top-level "meta" с "version":5) → парсим напрямую;
//   - v2 / v3 / v4 (top-level "version") → legacy decode + auto-миграция в v5;
//   - неизвестная версия → ошибка «regenerate via wizard»;
//   - битый JSON → ошибка с понятным контекстом.
//
// SPEC 056-R-N: при загрузке v6 файла со старым дев-shape (`dns.template_servers`/
// `extra_servers`/`extra_rules`) `parseCurrent` читает его через legacyDevDNSToOptions
// fallback и конвертит in-memory в новый flat shape. На ближайшем Save файл
// перезаписывается в новом layout'е. Никакого backup'а не делаем — конверсия
// lossless (TestRoundTrip покрывает), v6 не релизился (только dev-state).
//
// Save после Load пишет либо v5, либо v6 в новом shape.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("state: read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse — Load из уже прочитанных байтов.
func Parse(data []byte) (*State, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("state: empty payload")
	}

	// Шаг 1: попытаться распознать формат — есть ли top-level "meta"?
	var probe struct {
		TopLevelVersion int `json:"version"`
		Meta            struct {
			Version int `json:"version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("state: parse json: %w", err)
	}

	switch {
	case probe.Meta.Version >= 6:
		return parseCurrent(data)
	case probe.Meta.Version == 5:
		return parseV5Legacy(data)
	case probe.TopLevelVersion >= 2 && probe.TopLevelVersion <= 4:
		return parseLegacyAndMigrate(data)
	case probe.TopLevelVersion == 0 && probe.Meta.Version == 0:
		return nil, fmt.Errorf("state: unknown schema (neither legacy version nor meta.version present)")
	default:
		return nil, fmt.Errorf("state: unsupported version (top=%d, meta.version=%d) — regenerate via Configurator",
			probe.TopLevelVersion, probe.Meta.Version)
	}
}
