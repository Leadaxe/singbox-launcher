package api

import (
	"encoding/json"
	"fmt"
	"os"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/urlredact"

	"github.com/muhammadmuzzammil1998/jsonc"
)

// LoadClashAPIConfig reads the Clash API URL and token from the sing-box config.json
func LoadClashAPIConfig(configPath string) (baseURL, token string, err error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		// Cold-start logging: на свежей инсталляции config.json ещё не существует
		// (пользователь не нажал Save → не пересобирали через RunParser). Это не
		// ошибка приложения, а ожидаемое первое-запуск состояние. Логируем как
		// DEBUG, чтобы не пугать пользователя красным ERROR в логах. Все callers
		// обрабатывают возвращаемую error как «нет clash API» и продолжают работу.
		if os.IsNotExist(err) {
			debuglog.DebugLog("LoadClashAPIConfig: config.json not present yet (cold start): %v", err)
		} else {
			debuglog.ErrorLog("LoadClashAPIConfig: Failed to read config.json: %v", err)
		}
		return "", "", fmt.Errorf("failed to read config.json: %w", err)
	}
	cleanData := jsonc.ToJSON(data)

	var jsonData map[string]interface{}
	if err := json.Unmarshal(cleanData, &jsonData); err != nil {
		debuglog.ErrorLog("LoadClashAPIConfig: Failed to parse JSON: %v", err)
		return "", "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	exp, ok := jsonData["experimental"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("no 'experimental' section found in config.json")
	}
	api, ok := exp["clash_api"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("no 'clash_api' section found under 'experimental' in config.json")
	}

	host, _ := api["external_controller"].(string)
	secret, _ := api["secret"].(string)

	if host == "" || secret == "" {
		return "", "", fmt.Errorf("'external_controller' or 'secret' is empty in Clash API config")
	}

	baseURL = "http://" + host
	token = secret

	debuglog.DebugLog("Clash API loaded from config: %s / token=%s", baseURL, urlredact.RedactToken(token))
	return baseURL, token, nil
}
