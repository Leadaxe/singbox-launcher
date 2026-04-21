//go:build cgo

// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл saver.go содержит функции для сохранения конфигурации:
//   - SaveConfigWithBackup - сохранение конфигурации с созданием бэкапа и генерацией случайного secret для Clash API
//
// SaveConfigWithBackup выполняет:
//  1. Подготовку итогового текста (JSON/JSONC, подстановка secret для clash_api)
//  2. При заданном singBoxPath — запись во временный файл, валидация sing-box check, при ошибке возврат без записи в config
//  3. Создание бэкапа существующего файла конфигурации и сохранение новой конфигурации в файл
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
	"singbox-launcher/internal/textnorm"
)

// tempConfigFileName — имя временного файла для валидации конфига перед записью (создаётся в той же директории, затем удаляется).
const tempConfigFileName = "config-check.json"

// SaveConfigWithBackup сохраняет конфигурацию с созданием бэкапа.
// Если fileService.SingboxPath() не пустой, сначала валидирует конфиг через sing-box check по временному файлу,
// и только при успехе пишет в configPath (не перезаписывает рабочий конфиг до успешной валидации).
//
// populateCheckText — опциональный callback, заполняющий маркеры @ParserSTART/@ParserEND
// динамическими outbounds в памяти. Принимает текст конфига (после prepareConfigText),
// возвращает текст с outbounds. Результат записывается и в config-check.json (для валидации),
// и в config.json (конфиг сразу полный, повторный парсинг не нужен).
func SaveConfigWithBackup(fileService FileServiceInterface, configText string, populateCheckText func(string) (string, error)) (string, error) {
	finalText, err := prepareConfigText(configText)
	if err != nil {
		return "", err
	}

	if populateCheckText != nil {
		if populated, err := populateCheckText(finalText); err != nil {
			debuglog.WarnLog("SaveConfigWithBackup: populateCheckText failed (continuing with empty markers): %v", err)
		} else {
			finalText = populated
		}
	}

	configPath := fileService.ConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return "", err
	}

	singBoxPath := fileService.SingboxPath()
	if singBoxPath != "" {
		tmpPath := filepath.Join(filepath.Dir(configPath), tempConfigFileName)
		defer func() {
			if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
				debuglog.WarnLog("SaveConfigWithBackup: failed to remove temp config %s: %v", tmpPath, err)
			}
		}()
		if err := os.WriteFile(tmpPath, []byte(finalText), 0o644); err != nil {
			return "", fmt.Errorf("write temp config: %w", err)
		}
		if err := ValidateConfigWithSingBox(tmpPath, singBoxPath); err != nil {
			return "", &ValidationError{Err: err, ConfigText: finalText}
		}
	}

	if err := services.BackupFile(configPath); err != nil {
		return "", err
	}
	// Atomic swap: stage to .swap then rename over configPath. BackupFile
	// above keeps the prior version for manual restore, but we still prefer
	// not to leave config.json truncated on crash — sing-box refuses to start
	// on malformed JSON and a non-technical user has no obvious recovery path.
	swapPath := configPath + ".swap"
	if err := os.WriteFile(swapPath, []byte(finalText), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(swapPath, configPath); err != nil {
		_ = os.Remove(swapPath)
		return "", err
	}
	return configPath, nil
}

// prepareConfigText подготавливает итоговый текст конфига (JSON/JSONC, подстановка secret для clash_api).
func prepareConfigText(configText string) (string, error) {
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

	return finalText, nil
}

func generateRandomSecret(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))[:length]
	}
	return base64.URLEncoding.EncodeToString(b)[:length]
}

// ValidateConfigWithSingBox валидирует конфигурационный файл через sing-box check.
// Окно консоли скрыто на всех платформах. Возвращает nil при успехе или если sing-box недоступен (graceful degradation).
func ValidateConfigWithSingBox(configPath, singBoxPath string) error {
	if singBoxPath == "" {
		debuglog.DebugLog("Skipping sing-box validation: singBoxPath is empty")
		return nil
	}
	if _, err := os.Stat(singBoxPath); os.IsNotExist(err) {
		debuglog.DebugLog("Skipping sing-box validation: executable not found at %s", singBoxPath)
		return nil
	}

	cmd := exec.Command(singBoxPath, "check", "-c", configPath)
	platform.PrepareCommand(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	debuglog.DebugLog("Running validation: %s check -c %s", singBoxPath, configPath)

	if err := cmd.Run(); err != nil {
		errorMsg := textnorm.StripANSI(stderr.String())
		if errorMsg == "" {
			errorMsg = textnorm.StripANSI(stdout.String())
		}
		if errorMsg == "" {
			errorMsg = err.Error()
		}
		debuglog.ErrorLog("Config validation failed: %v", err)
		debuglog.LogTextFragment("ConfigValidator", debuglog.LevelError,
			"Validation error output", errorMsg, 500)
		return fmt.Errorf("sing-box config validation failed: %s", errorMsg)
	}

	debuglog.InfoLog("Config validation passed successfully")
	return nil
}
