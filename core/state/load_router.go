package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
)

// ErrNotFound — state-файл не существует. Вызывающий обычно интерпретирует
// это как «свежая установка», а не как ошибку.
var ErrNotFound = errors.New("state: file not found")

// Load читает state.json по пути path.
//
// Поведение:
//   - файл отсутствует → ErrNotFound;
//   - v7 (meta.version = 7) → parseV7 напрямую;
//   - v6 / v5 / v2–4 → легаси-парс + структурный перенос W1 + семантическая
//     миграция v6→v7 (SPEC 118 Т7); перед миграцией рядом с файлом пишется
//     бэкап-копия `<path>.v6.bak` (страховка необратимого шага 8, риск Р5);
//   - неизвестная версия → ошибка «regenerate via wizard»;
//   - битый JSON → ошибка с понятным контекстом.
//
// Save после Load всегда пишет v7.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("state: read %s: %w", path, err)
	}

	lc := deriveLoadContext(path)

	if top, meta, probeErr := sniffSchemaVersion(data); probeErr == nil {
		legacy := meta >= 2 && meta <= 6 || (meta == 0 && top >= 2 && top <= 4)
		if legacy {
			// Бэкап исходника ПЕРЕД миграцией; идемпотентно (существующая
			// копия не перетирается — первая и есть исходник).
			if bak, bakErr := writeLegacyBackupOnce(path, data); bakErr != nil {
				debuglog.WarnLog("state: backup before migration failed: %v", bakErr)
			} else if bak != "" {
				debuglog.InfoLog("state: legacy state backed up to %s before v7 migration", bak)
			}
		}
	}

	s, err := parseWithContext(data, lc)
	if err != nil {
		return nil, err
	}
	if s.Migration != nil && s.Migration.BackupPath == "" {
		s.Migration.BackupPath = legacyBackupPath(path)
	}
	// Разовая правка masque-URI со старым `?network=` (masque_uri_migration.go).
	// Персистится сразу: иначе тело узла пересобиралось бы на каждом старте,
	// а файл так и нёс бы legacy-форму.
	if n := rewriteLegacyMasqueURIs(s); n > 0 {
		if err := s.Save(path); err != nil {
			debuglog.WarnLog("state: masque uri migration (%d nodes) not persisted: %v", n, err)
		} else {
			debuglog.InfoLog("state: masque uri migration persisted (%d nodes) to %s", n, path)
		}
	}
	// SPEC 118 W6 (хвост W2): отчёт — на диск. Мигрирует ПЕРВЫЙ, кто откроет
	// состояние, а на старте лаунчера это фоновая загрузка без окна: к
	// открытию конфигуратора файл уже v7, и отчёта в памяти нет ни у кого.
	// Файл рядом в bin/ переживает эту дистанцию.
	PersistMigrationReport(lc.BinDir, s.Migration, path)

	// Шаг 8 миграции (снос легаси) — гейт до W5 (PLAN §6): включается
	// migrationPurgesLegacy. Порядок обязателен: сначала успешная запись
	// v7-файла, только потом необратимый снос raw-кэша и легаси-полей.
	if migrationPurgesLegacy && s.Migration != nil {
		if err := s.Save(path); err != nil {
			debuglog.WarnLog("state: migrated v7 not persisted (%v) — legacy purge skipped", err)
		} else {
			purgeLegacyAfterMigration(s, lc)
			if err := s.Save(path); err != nil {
				debuglog.WarnLog("state: save after legacy purge: %v", err)
			}
		}
	}
	return s, nil
}

// Parse — Load из уже прочитанных байтов. Контекста путей нет: миграция
// легаси-схемы выполняется без материализации raw-кэша (nodes[] остаются
// пустыми с предупреждением) и без бэкап-копии.
func Parse(data []byte) (*State, error) {
	s, err := parseWithContext(data, LoadContext{})
	if err != nil {
		return nil, err
	}
	rewriteLegacyMasqueURIs(s)
	return s, nil
}

func parseWithContext(data []byte, lc LoadContext) (*State, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("state: empty payload")
	}

	top, meta, err := sniffSchemaVersion(data)
	if err != nil {
		return nil, fmt.Errorf("state: parse json: %w", err)
	}

	switch {
	case meta >= 7:
		// v7 и минорные добавки поверх (PLAN §1.3: незнакомые ключи
		// игнорируются, пока мажор 7; неизвестный kind внутри — отказ).
		return parseV7(data)
	case meta == 6:
		return parseV6Legacy(data, lc)
	case meta == 5:
		return parseV5Legacy(data, lc)
	case top >= 2 && top <= 4:
		return parseLegacyAndMigrate(data, lc)
	case top == 0 && meta == 0:
		return nil, fmt.Errorf("state: unknown schema (neither legacy version nor meta.version present)")
	default:
		return nil, fmt.Errorf("state: unsupported version (top=%d, meta.version=%d) — regenerate via Configurator",
			top, meta)
	}
}

// sniffSchemaVersion — быстрый пробник формата: top-level "version"
// (v2–v4) и "meta.version" (v5+).
func sniffSchemaVersion(data []byte) (top, meta int, err error) {
	var probe struct {
		TopLevelVersion int `json:"version"`
		Meta            struct {
			Version int `json:"version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return 0, 0, err
	}
	return probe.TopLevelVersion, probe.Meta.Version, nil
}

// deriveLoadContext — вывод путей контекста из пути state-файла:
//
//	local  → <bin>/wizard_states/<file>.json  → subs=<bin>/subscriptions
//	remote → …/wizard_states/remote/<id>/state.json → subs=<dir>/subscriptions
//
// BinDir — ближайший предок с именем bin: настройки приложения одни на все
// состояния (features/state.md «Что ушло из состояния в настройки»).
func deriveLoadContext(path string) LoadContext {
	dir := filepath.Dir(path)
	lc := LoadContext{StatePath: path}

	if filepath.Base(dir) == constants.WizardStatesDirName {
		bin := filepath.Dir(dir)
		lc.BinDir = bin
		lc.SubsDir = filepath.Join(bin, constants.SubscriptionsDirName)
		return lc
	}

	// Машинная директория remote/<id>/ — raw-кэш лежит рядом (SPEC 098).
	lc.SubsDir = filepath.Join(dir, constants.SubscriptionsDirName)
	for p := dir; ; {
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		if filepath.Base(parent) == constants.BinDirName {
			lc.BinDir = parent
			break
		}
		p = parent
	}
	return lc
}

// legacyBackupPath — путь бэкап-копии исходного легаси-файла.
func legacyBackupPath(path string) string {
	bak := path + ".v6.bak"
	if _, err := os.Stat(bak); err != nil {
		return ""
	}
	return bak
}

// writeLegacyBackupOnce — копия исходных байтов рядом с файлом (O_EXCL:
// существующая копия — самый первый исходник, его не перетираем).
// Возвращает путь копии, если она записана этим вызовом.
func writeLegacyBackupOnce(path string, data []byte) (string, error) {
	bak := path + ".v6.bak"
	f, err := os.OpenFile(bak, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return "", nil
		}
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(bak)
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return bak, nil
}
