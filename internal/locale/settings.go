package locale

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// Settings represents the launcher settings stored in bin/settings.json.
type Settings struct {
	Lang string `json:"lang"`
	// PingTestURL — endpoint для Clash GET /proxies/{name}/delay (query url); пусто = не переопредлять дефолт api.
	PingTestURL string `json:"ping_test_url,omitempty"`
	// PingTestAllConcurrency — число параллельных delay-запросов для «test» на вкладке Servers; 0 = не переопредлять.
	PingTestAllConcurrency int `json:"ping_test_all_concurrency,omitempty"`
	// SubscriptionAutoUpdateDisabled — пользователь явно выключил автоматическое обновление
	// подписок. По умолчанию (отсутствует / false) — автообновление включено, как раньше.
	// Manual Update всегда работает независимо от флага.
	SubscriptionAutoUpdateDisabled bool `json:"subscription_auto_update_disabled,omitempty"`
}

// LoadSettings reads settings from binDir/settings.json.
// Returns default settings if file doesn't exist or is invalid.
func LoadSettings(binDir string) Settings {
	s := Settings{Lang: "en"}
	path := filepath.Join(binDir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, &s); err != nil {
		debuglog.WarnLog("locale: failed to parse settings.json: %v", err)
		return Settings{Lang: "en"}
	}
	if s.Lang == "" {
		s.Lang = "en"
	}
	return s
}

// SaveSettings writes settings to binDir/settings.json.
func SaveSettings(binDir string, s Settings) error {
	path := filepath.Join(binDir, "settings.json")
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("locale: marshal settings: %w", err)
	}
	return os.WriteFile(path, data, platform.DefaultFileMode)
}
