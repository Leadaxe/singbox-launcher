//go:build cgo

// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл saver.go содержит функции для сохранения конфигурации:
//   - SaveConfigWithBackup - сохранение конфигурации с созданием бэкапа и генерацией случайного secret для Clash API
//   - NextBackupPath - генерация пути для следующего бэкапа (config-old.json, config-old-1.json и т.д.)
//   - FileServiceAdapter - адаптер для services.FileService, предоставляющий доступ к путям конфигурации
//
// SaveConfigWithBackup выполняет:
//  1. Валидацию JSON конфигурации (включая поддержку JSONC с комментариями)
//  2. Генерацию случайного secret для experimental.clash_api.secret (если отсутствует)
//  3. Создание бэкапа существующего файла конфигурации
//  4. Сохранение новой конфигурации в файл
//
// Эти функции работают только с данными (текст конфигурации, путь к файлу),
// без зависимостей от GUI и WizardState, что делает их тестируемыми и переиспользуемыми.
//
// Сохранение конфигурации - это отдельная ответственность от парсинга и генерации.
// Содержит логику работы с файловой системой и бэкапами.
// Используется презентером (presenter_save.go) для финального сохранения конфигурации.
//
// Используется в:
//   - presenter_save.go - SaveConfig вызывает SaveConfigWithBackup для сохранения финальной конфигурации
package business

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// FileServiceAdapter адаптирует services.FileService для использования в бизнес-логике.
// Реализует интерфейс FileServiceInterface, определенный в interfaces.go.
type FileServiceAdapter struct {
	FileService *services.FileService
}

func (a *FileServiceAdapter) ConfigPath() string {
	return a.FileService.ConfigPath
}

func (a *FileServiceAdapter) ExecDir() string {
	return a.FileService.ExecDir
}

// SaveConfigWithBackup сохраняет конфигурацию с созданием бэкапа.
func SaveConfigWithBackup(fileService FileServiceInterface, configText string) (string, error) {
	jsonBytes := jsonc.ToJSON([]byte(configText))
	var configJSON map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &configJSON); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}

	randomSecret := generateRandomSecret(24)

	finalText := configText
	secretReplaced := false

	simpleSecretPattern := regexp.MustCompile(`("secret"\s*:\s*)"[^"]*"`)
	if simpleSecretPattern.MatchString(configText) && strings.Contains(configText, "clash_api") {
		finalText = simpleSecretPattern.ReplaceAllString(configText, fmt.Sprintf(`$1"%s"`, randomSecret))
		secretReplaced = true
	}

	if !secretReplaced {
		if experimental, ok := configJSON["experimental"].(map[string]interface{}); ok {
			if clashAPI, ok := experimental["clash_api"].(map[string]interface{}); ok {
				clashAPI["secret"] = randomSecret
			} else {
				experimental["clash_api"] = map[string]interface{}{
					"external_controller": "127.0.0.1:9090",
					"secret":              randomSecret,
				}
			}
		} else {
			configJSON["experimental"] = map[string]interface{}{
				"clash_api": map[string]interface{}{
					"external_controller": "127.0.0.1:9090",
					"secret":              randomSecret,
				},
			}
		}

		finalJSONBytes, err := json.MarshalIndent(configJSON, "", IndentBase)
		if err != nil {
			return "", fmt.Errorf("failed to marshal config: %w", err)
		}
		finalText = string(finalJSONBytes)
	}

	configPath := fileService.ConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return "", err
	}
	if info, err := os.Stat(configPath); err == nil && !info.IsDir() {
		backup := NextBackupPath(configPath)
		if err := os.Rename(configPath, backup); err != nil {
			return "", err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.WriteFile(configPath, []byte(finalText), 0o644); err != nil {
		return "", err
	}
	return configPath, nil
}

// NextBackupPath генерирует путь для следующего бэкапа файла.
func NextBackupPath(path string) string {
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	candidate := filepath.Join(dir, fmt.Sprintf("%s-old%s", base, ext))
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	for i := 1; ; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-old-%d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func generateRandomSecret(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))[:length]
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length]
}

// ValidateConfigWithSingBox validates configuration file using sing-box check command.
// Works on all platforms (Windows, macOS, Linux) with console window hidden.
// Returns nil if valid or if sing-box is not available (graceful degradation).
func ValidateConfigWithSingBox(configPath, singBoxPath string) error {
	// Skip validation if sing-box path is not provided
	if singBoxPath == "" {
		debuglog.Log("ConfigValidator", debuglog.LevelVerbose, debuglog.UseGlobal,
			"Skipping sing-box validation: singBoxPath is empty")
		return nil
	}

	// Check if sing-box executable exists
	if _, err := os.Stat(singBoxPath); os.IsNotExist(err) {
		debuglog.Log("ConfigValidator", debuglog.LevelVerbose, debuglog.UseGlobal,
			"Skipping sing-box validation: executable not found at %s", singBoxPath)
		return nil
	}

	// Prepare command
	cmd := exec.Command(singBoxPath, "check", "-c", configPath)

	// Hide console window on all platforms
	platform.PrepareCommand(cmd)

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	debuglog.Log("ConfigValidator", debuglog.LevelVerbose, debuglog.UseGlobal,
		"Running validation: %s check -c %s", singBoxPath, configPath)

	// Run validation
	err := cmd.Run()

	if err != nil {
		// Validation failed - extract meaningful error message
		errorMsg := stderr.String()
		if errorMsg == "" {
			errorMsg = stdout.String()
		}
		if errorMsg == "" {
			errorMsg = err.Error()
		}

		debuglog.Log("ConfigValidator", debuglog.LevelError, debuglog.UseGlobal,
			"Config validation failed: %v", err)
		debuglog.LogTextFragment("ConfigValidator", debuglog.LevelError, debuglog.UseGlobal,
			"Validation error output", errorMsg, 500)

		return fmt.Errorf("sing-box config validation failed: %s", errorMsg)
	}

	debuglog.Log("ConfigValidator", debuglog.LevelInfo, debuglog.UseGlobal,
		"Config validation passed successfully")

	return nil
}
