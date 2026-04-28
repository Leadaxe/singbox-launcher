// Package business — хранение и загрузка состояний визарда.
//
// SPEC 052 phase 7: StateStore — тонкий wrapper вокруг core/state.{Save,Load}.
// Wizard'овский WizardStateFile теперь алиас на corestate.State; вся логика
// сериализации (v5 layout, atomic write, migration v2-v4 → v5) живёт в
// core/state. Здесь только path resolution + listing snapshots.
package business

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

const (
	// WizardStatesDir — имя директории состояний визарда.
	WizardStatesDir = constants.WizardStatesDirName

	// MaxStateFileSize — максимальный размер файла состояния (256 KB).
	// Информационный лимит для warn-логов; core/state не enforces.
	MaxStateFileSize = 256 * 1024
)

// StateStore — фасад для bin/wizard_states/.
type StateStore struct {
	fileService FileServiceInterface
	statesDir   string
}

// NewStateStore создаёт StateStore.
func NewStateStore(fileService FileServiceInterface) *StateStore {
	return &StateStore{
		fileService: fileService,
		statesDir:   platform.GetWizardStatesDir(fileService.ExecDir()),
	}
}

func (ss *StateStore) ensureStatesDir() error {
	if err := os.MkdirAll(ss.statesDir, platform.DefaultDirMode); err != nil {
		return fmt.Errorf("failed to create wizard states directory: %w", err)
	}
	return nil
}

// getStateFilePath — путь к файлу для данного ID; "" → state.json.
func (ss *StateStore) getStateFilePath(id string) string {
	if id == "" {
		return filepath.Join(ss.statesDir, wizardmodels.StateFileName)
	}
	return filepath.Join(ss.statesDir, id+".json")
}

// SaveWizardState сохраняет state в файл с указанным ID.
//
// Pipeline:
//  1. Валидация ID (если задан);
//  2. set CreatedAt если zero, refresh UpdatedAt;
//  3. corestate.Save — atomic write в v5-формате.
func (ss *StateStore) SaveWizardState(state *wizardmodels.WizardStateFile, id string) error {
	if state == nil {
		return fmt.Errorf("state cannot be nil")
	}

	if id != "" {
		if err := wizardmodels.ValidateStateID(id); err != nil {
			return fmt.Errorf("invalid state ID: %w", err)
		}
		if state.ID == "" {
			state.ID = id
		} else if state.ID != id {
			return fmt.Errorf("state ID mismatch: state has %q, but requested %q", state.ID, id)
		}
	}

	if err := ss.ensureStatesDir(); err != nil {
		return err
	}

	filePath := ss.getStateFilePath(id)
	if err := state.Save(filePath); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	debuglog.InfoLog("SaveWizardState: saved state to %s", filePath)
	return nil
}

// SaveCurrentState сохраняет состояние в state.json.
func (ss *StateStore) SaveCurrentState(state *wizardmodels.WizardStateFile) error {
	return ss.SaveWizardState(state, "")
}

// LoadWizardState загружает state по ID через corestate.Load (auto-migrate v2-v4 → v5).
func (ss *StateStore) LoadWizardState(id string) (*wizardmodels.WizardStateFile, error) {
	if id == "" {
		return nil, fmt.Errorf("state ID cannot be empty")
	}
	return ss.loadStateFromFile(ss.getStateFilePath(id), id)
}

// LoadCurrentState загружает state.json.
func (ss *StateStore) LoadCurrentState() (*wizardmodels.WizardStateFile, error) {
	return ss.loadStateFromFile(ss.getStateFilePath(""), "")
}

// loadStateFromFile — общий путь Load через corestate.
func (ss *StateStore) loadStateFromFile(filePath string, expectedID string) (*wizardmodels.WizardStateFile, error) {
	state, err := corestate.Load(filePath)
	if err != nil {
		if errors.Is(err, corestate.ErrNotFound) {
			if expectedID == "" {
				return nil, fmt.Errorf("state.json not found")
			}
			return nil, fmt.Errorf("state file not found: %s", filePath)
		}
		return nil, err
	}

	// Информационная проверка размера (только для warn-логов).
	if info, statErr := os.Stat(filePath); statErr == nil && info.Size() > MaxStateFileSize {
		debuglog.WarnLog("loadStateFromFile: state file size (%d bytes) exceeds recommended maximum (%d bytes)",
			info.Size(), MaxStateFileSize)
	}

	if expectedID != "" && state.ID != "" && state.ID != expectedID {
		debuglog.WarnLog("loadStateFromFile: state ID mismatch: file has %q, but expected %q", state.ID, expectedID)
	}

	debuglog.InfoLog("loadStateFromFile: loaded state from %s", filePath)
	return state, nil
}

// ListWizardStates возвращает список всех состояний с метаданными.
//
// Метаданные парсятся «cheap-way»: open-read-decode top-level fields из v5-формата
// (meta.created_at, meta.updated_at, meta.comment). Для legacy v4-файлов читаем
// старые top-level поля (created_at, updated_at, comment).
func (ss *StateStore) ListWizardStates() ([]wizardmodels.WizardStateMetadata, error) {
	if err := ss.ensureStatesDir(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(ss.statesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read wizard states directory: %w", err)
	}

	var states []wizardmodels.WizardStateMetadata
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileName := entry.Name()
		if !strings.HasSuffix(fileName, ".json") {
			continue
		}

		id, isCurrent := extractStateIDFromFileName(fileName)
		filePath := filepath.Join(ss.statesDir, fileName)

		fileInfo, _ := entry.Info()
		var modTime time.Time
		if fileInfo != nil {
			modTime = fileInfo.ModTime()
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			debuglog.WarnLog("ListWizardStates: read %s: %v", filePath, err)
			continue
		}
		md := parseMetadataAny(data, modTime)
		if md.ID == "" && !isCurrent {
			md.ID = id
		}
		md.ID = id
		md.IsCurrent = isCurrent
		states = append(states, md)
	}

	debuglog.DebugLog("ListWizardStates: found %d states", len(states))
	return states, nil
}

// ListWizardStateNames возвращает только имена файлов без чтения содержимого.
func (ss *StateStore) ListWizardStateNames() ([]wizardmodels.WizardStateMetadata, error) {
	if err := ss.ensureStatesDir(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(ss.statesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read wizard states directory: %w", err)
	}

	var states []wizardmodels.WizardStateMetadata
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileName := entry.Name()
		if !strings.HasSuffix(fileName, ".json") {
			continue
		}
		id, isCurrent := extractStateIDFromFileName(fileName)
		var modTime time.Time
		if fi, err := entry.Info(); err == nil {
			modTime = fi.ModTime()
		}
		states = append(states, wizardmodels.WizardStateMetadata{
			ID:        id,
			IsCurrent: isCurrent,
			CreatedAt: modTime,
			UpdatedAt: modTime,
		})
	}

	debuglog.DebugLog("ListWizardStateNames: found %d states", len(states))
	return states, nil
}

// DeleteWizardState удаляет файл state по ID.
func (ss *StateStore) DeleteWizardState(id string) error {
	if id == "" {
		return fmt.Errorf("state ID cannot be empty")
	}
	filePath := ss.getStateFilePath(id)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("state file not found: %s", filePath)
	}
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete state file: %w", err)
	}
	debuglog.InfoLog("DeleteWizardState: deleted state file %s", filePath)
	return nil
}

// StateExists проверяет существование state файла.
func (ss *StateStore) StateExists(id string) bool {
	_, err := os.Stat(ss.getStateFilePath(id))
	return err == nil
}

// extractStateIDFromFileName — id из имени файла, isCurrent для state.json.
func extractStateIDFromFileName(fileName string) (string, bool) {
	if fileName == wizardmodels.StateFileName {
		return "", true
	}
	return strings.TrimSuffix(fileName, ".json"), false
}

// parseMetadataAny извлекает базовые поля state-файла, поддерживает оба
// layout'а: v5 (meta.{...}) и legacy v2-v4 (top-level).
func parseMetadataAny(data []byte, modTime time.Time) wizardmodels.WizardStateMetadata {
	var probe struct {
		ID        string `json:"id,omitempty"`
		Comment   string `json:"comment,omitempty"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Meta      struct {
			Comment   string `json:"comment,omitempty"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(data, &probe)

	md := wizardmodels.WizardStateMetadata{
		ID:      probe.ID,
		Comment: firstNonEmpty(probe.Meta.Comment, probe.Comment),
	}
	md.CreatedAt = parseTimeOr(firstNonEmpty(probe.Meta.CreatedAt, probe.CreatedAt), modTime)
	md.UpdatedAt = parseTimeOr(firstNonEmpty(probe.Meta.UpdatedAt, probe.UpdatedAt), modTime)
	return md
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func parseTimeOr(rfc3339 string, fallback time.Time) time.Time {
	if rfc3339 == "" {
		return fallback
	}
	if t, err := time.Parse(time.RFC3339, rfc3339); err == nil {
		return t
	}
	return fallback
}
