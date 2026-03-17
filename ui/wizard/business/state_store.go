// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл state_store.go реализует хранение и загрузку состояний визарда.
//
// StateStore предоставляет методы для:
//   - Сохранения состояния в файл (SaveWizardState, SaveCurrentState)
//   - Загрузки состояния из файла (LoadWizardState, LoadCurrentState)
//   - Получения списка всех состояний (ListWizardStates)
//   - Валидации ID состояния (ValidateStateID)
//
// Состояния хранятся в директории <execDir>/bin/wizard_states/:
//   - state.json - текущее рабочее состояние
//   - <id>.json - именованные сохранённые состояния
//
// Используется в:
//   - presentation/presenter.go - для сохранения/загрузки состояний через презентер
package business

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
	wizardmodels "singbox-launcher/ui/wizard/models"
)

const (
	// WizardStatesDir - имя директории для хранения состояний визарда
	WizardStatesDir = "wizard_states"

	// MaxStateFileSize - максимальный размер файла состояния (256 KB)
	MaxStateFileSize = 256 * 1024
)

// StateStore управляет сохранением и загрузкой состояний визарда.
type StateStore struct {
	fileService FileServiceInterface
	statesDir  string
}

// NewStateStore создает новый StateStore.
func NewStateStore(fileService FileServiceInterface) *StateStore {
	statesDir := filepath.Join(fileService.ExecDir(), "bin", WizardStatesDir)
	return &StateStore{
		fileService: fileService,
		statesDir:  statesDir,
	}
}

// ensureStatesDir создает директорию состояний, если она не существует.
// Используется перед сохранением и загрузкой состояний.
func (ss *StateStore) ensureStatesDir() error {
	if err := os.MkdirAll(ss.statesDir, platform.DefaultDirMode); err != nil {
		return fmt.Errorf("failed to create wizard states directory: %w", err)
	}
	return nil
}

// getStateFilePath возвращает путь к файлу состояния по ID.
// Для пустого ID возвращает путь к state.json.
func (ss *StateStore) getStateFilePath(id string) string {
	if id == "" {
		return filepath.Join(ss.statesDir, wizardmodels.StateFileName)
	}
	return filepath.Join(ss.statesDir, id+".json")
}

// SaveWizardState сохраняет состояние в файл с указанным ID.
func (ss *StateStore) SaveWizardState(state *wizardmodels.WizardStateFile, id string) error {
	if state == nil {
		return fmt.Errorf("state cannot be nil")
	}

	// Валидация ID
	if id != "" {
		if err := wizardmodels.ValidateStateID(id); err != nil {
			return fmt.Errorf("invalid state ID: %w", err)
		}
		// Устанавливаем ID в состояние, если он не задан
		if state.ID == "" {
			state.ID = id
		} else if state.ID != id {
			return fmt.Errorf("state ID mismatch: state has %q, but requested %q", state.ID, id)
		}
	}

	// Устанавливаем версию, если не задана
	if state.Version == 0 {
		state.Version = wizardmodels.WizardStateVersion
	}

	// Устанавливаем время создания, если не задано
	if state.CreatedAt.IsZero() {
		state.CreatedAt = time.Now().UTC()
	}

	// Обновляем время изменения
	state.UpdatedAt = time.Now().UTC()

	// Создаём директорию, если не существует
	if err := ss.ensureStatesDir(); err != nil {
		return err
	}

	// Сериализуем состояние в JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Проверка размера файла
	if len(data) > MaxStateFileSize {
		debuglog.WarnLog("SaveWizardState: state file size (%d bytes) exceeds recommended maximum (%d bytes)", len(data), MaxStateFileSize)
	}

	// Определяем путь к файлу
	filePath := ss.getStateFilePath(id)

	// Сохраняем в файл
	if err := os.WriteFile(filePath, data, platform.DefaultFileMode); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	debuglog.InfoLog("SaveWizardState: saved state to %s", filePath)
	return nil
}

// SaveCurrentState сохраняет состояние в state.json.
func (ss *StateStore) SaveCurrentState(state *wizardmodels.WizardStateFile) error {
	return ss.SaveWizardState(state, "")
}

// loadStateFromFile загружает состояние из файла по пути.
// Внутренняя функция для устранения дублирования кода.
func (ss *StateStore) loadStateFromFile(filePath string, expectedID string) (*wizardmodels.WizardStateFile, error) {
	// Проверяем существование файла
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if expectedID == "" {
			return nil, fmt.Errorf("state.json not found")
		}
		return nil, fmt.Errorf("state file not found: %s", filePath)
	}

	// Читаем файл
	data, err := os.ReadFile(filePath)
	if err != nil {
		if expectedID == "" {
			return nil, fmt.Errorf("failed to read state.json: %w", err)
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Проверка размера файла
	if len(data) > MaxStateFileSize {
		debuglog.WarnLog("loadStateFromFile: state file size (%d bytes) exceeds recommended maximum (%d bytes)", len(data), MaxStateFileSize)
	}

	// Десериализуем JSON
	var state wizardmodels.WizardStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		if expectedID == "" {
			return nil, fmt.Errorf("failed to unmarshal state.json: %w", err)
		}
		return nil, fmt.Errorf("failed to unmarshal state file: %w", err)
	}

	// Валидация версии
	if state.Version != wizardmodels.WizardStateVersion {
		return nil, fmt.Errorf("unsupported state file version: %d (expected %d)", state.Version, wizardmodels.WizardStateVersion)
	}

	// Валидация ID (если задан, должен совпадать с именем файла)
	if expectedID != "" && state.ID != "" && state.ID != expectedID {
		debuglog.WarnLog("loadStateFromFile: state ID mismatch: file has %q, but expected %q", state.ID, expectedID)
	}

	debuglog.InfoLog("loadStateFromFile: loaded state from %s", filePath)
	return &state, nil
}

// LoadWizardState загружает состояние из файла по ID.
func (ss *StateStore) LoadWizardState(id string) (*wizardmodels.WizardStateFile, error) {
	if id == "" {
		return nil, fmt.Errorf("state ID cannot be empty")
	}

	filePath := ss.getStateFilePath(id)
	return ss.loadStateFromFile(filePath, id)
}

// LoadCurrentState загружает состояние из state.json.
func (ss *StateStore) LoadCurrentState() (*wizardmodels.WizardStateFile, error) {
	filePath := ss.getStateFilePath("")
	return ss.loadStateFromFile(filePath, "")
}

// parseStateMetadata парсит метаданные состояния из JSON данных.
// Использует время модификации файла как fallback для дат.
func (ss *StateStore) parseStateMetadata(data []byte, modTime time.Time) (wizardmodels.WizardStateMetadata, error) {
	var metadata wizardmodels.WizardStateMetadata

	// Парсим только метаданные (не весь файл)
	var aux struct {
		ID        string `json:"id,omitempty"`
		Comment   string `json:"comment,omitempty"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return metadata, fmt.Errorf("failed to parse metadata: %w", err)
	}

	metadata.ID = aux.ID
	metadata.Comment = aux.Comment

	// Парсим время создания
	if aux.CreatedAt != "" {
		createdAt, err := time.Parse(time.RFC3339, aux.CreatedAt)
		if err != nil {
			debuglog.WarnLog("parseStateMetadata: invalid created_at: %v, using file mod time", err)
			metadata.CreatedAt = modTime
		} else {
			metadata.CreatedAt = createdAt
		}
	} else {
		metadata.CreatedAt = modTime
	}

	// Парсим время обновления
	if aux.UpdatedAt != "" {
		updatedAt, err := time.Parse(time.RFC3339, aux.UpdatedAt)
		if err != nil {
			debuglog.WarnLog("parseStateMetadata: invalid updated_at: %v, using file mod time", err)
			metadata.UpdatedAt = modTime
		} else {
			metadata.UpdatedAt = updatedAt
		}
	} else {
		metadata.UpdatedAt = modTime
	}

	return metadata, nil
}

// extractStateIDFromFileName извлекает ID состояния из имени файла.
// Возвращает ID и флаг isCurrent (true для state.json).
func extractStateIDFromFileName(fileName string) (id string, isCurrent bool) {
	if fileName == wizardmodels.StateFileName {
		return "", true
	}
	return strings.TrimSuffix(fileName, ".json"), false
}

// ListWizardStates возвращает список всех сохранённых состояний с метаданными.
// Для простого списка имён файлов используйте ListWizardStateNames().
func (ss *StateStore) ListWizardStates() ([]wizardmodels.WizardStateMetadata, error) {
	// Создаём директорию, если не существует (на случай, если состояний ещё нет)
	if err := ss.ensureStatesDir(); err != nil {
		return nil, err
	}

	// Читаем все .json файлы из директории
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

		// Определяем ID из имени файла
		id, isCurrent := extractStateIDFromFileName(fileName)

		// Загружаем метаданные из файла
		filePath := filepath.Join(ss.statesDir, fileName)
		data, err := os.ReadFile(filePath)
		if err != nil {
			debuglog.WarnLog("ListWizardStates: failed to read %s: %v", filePath, err)
			continue
		}

		// Получаем время модификации файла как fallback
		fileInfo, err := entry.Info()
		modTime := time.Now()
		if err == nil {
			modTime = fileInfo.ModTime()
		}

		// Парсим метаданные
		metadata, err := ss.parseStateMetadata(data, modTime)
		if err != nil {
			debuglog.WarnLog("ListWizardStates: failed to parse %s: %v", filePath, err)
			continue
		}

		// Если ID не задан в файле, используем имя файла
		if metadata.ID == "" && !isCurrent {
			metadata.ID = id
		}

		// Устанавливаем ID и флаг isCurrent
		metadata.ID = id
		metadata.IsCurrent = isCurrent

		states = append(states, metadata)
	}

	debuglog.DebugLog("ListWizardStates: found %d states", len(states))
	return states, nil
}

// ListWizardStateNames возвращает только имена файлов состояний без чтения содержимого.
// Используется для простого списка имён файлов в диалоге.
func (ss *StateStore) ListWizardStateNames() ([]wizardmodels.WizardStateMetadata, error) {
	// Создаём директорию, если не существует
	if err := ss.ensureStatesDir(); err != nil {
		return nil, err
	}

	// Читаем только имена файлов из директории
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

		// Определяем ID из имени файла
		id, isCurrent := extractStateIDFromFileName(fileName)

		// Получаем время модификации файла
		fileInfo, err := entry.Info()
		modTime := time.Now()
		if err == nil {
			modTime = fileInfo.ModTime()
		}

		// Создаём метаданные только из имени файла, без чтения содержимого
		metadata := wizardmodels.WizardStateMetadata{
			ID:        id,
			IsCurrent: isCurrent,
			CreatedAt: modTime,
			UpdatedAt: modTime,
		}

		states = append(states, metadata)
	}

	debuglog.DebugLog("ListWizardStateNames: found %d states", len(states))
	return states, nil
}

// DeleteWizardState удаляет состояние по ID.
func (ss *StateStore) DeleteWizardState(id string) error {
	if id == "" {
		return fmt.Errorf("state ID cannot be empty")
	}

	filePath := ss.getStateFilePath(id)

	// Проверяем существование файла
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("state file not found: %s", filePath)
	}

	// Удаляем файл
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete state file: %w", err)
	}

	debuglog.InfoLog("DeleteWizardState: deleted state file %s", filePath)
	return nil
}

// StateExists проверяет, существует ли состояние с указанным ID.
func (ss *StateStore) StateExists(id string) bool {
	filePath := ss.getStateFilePath(id)
	_, err := os.Stat(filePath)
	return err == nil
}

