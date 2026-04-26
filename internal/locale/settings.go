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
	// AutoPingAfterConnectDisabled — выключить автопинг нод через 5с после старта VPN.
	// По умолчанию (отсутствует / false) — автопинг включён. Ручная «test» всегда работает.
	AutoPingAfterConnectDisabled bool `json:"auto_ping_after_connect_disabled,omitempty"`
	// DebugAPIEnabled — пользователь явно включил локальный HTTP debug-API
	// (127.0.0.1:9269 по умолчанию). Off by default.
	DebugAPIEnabled bool `json:"debug_api_enabled,omitempty"`
	// DebugAPIToken — Bearer-токен для debug-API. Генерируется при первом
	// включении, больше не меняется (кроме явной регенерации через UI).
	DebugAPIToken string `json:"debug_api_token,omitempty"`
	// DebugAPIPort — порт для debug-API; 0 / отсутствует означает DefaultPort.
	DebugAPIPort int `json:"debug_api_port,omitempty"`
	// LastTemplateLauncherVersion — версия лаунчера, которая в последний раз
	// успешно скачала bin/wizard_template.json. На старте сравнивается с
	// текущей AppVersion: если меньше → шаблон удаляется как протухший
	// (формат шаблона мог разойтись между версиями). См. SPEC 046.
	LastTemplateLauncherVersion string `json:"last_template_launcher_version,omitempty"`
}

// MarkTemplateInstalled persists the launcher version that just installed the
// template. Called after a successful template download — see SPEC 046.
//
// Failure to persist is non-fatal for the immediate operation but logged: if
// the version isn't recorded the next launcher upgrade will re-invalidate the
// template once more (cosmetic UX nuisance, not a correctness issue).
func MarkTemplateInstalled(binDir, appVersion string) error {
	s := LoadSettings(binDir)
	if s.LastTemplateLauncherVersion == appVersion {
		return nil
	}
	s.LastTemplateLauncherVersion = appVersion
	return SaveSettings(binDir, s)
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
//
// Writes are atomic: we stage to a sibling temp file then rename over the
// real one. Protects against power loss or a crash mid-write leaving the
// user with a zero-byte settings.json and losing language / ping / subs
// preferences on next launch.
func SaveSettings(binDir string, s Settings) error {
	path := filepath.Join(binDir, "settings.json")
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("locale: marshal settings: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, platform.DefaultFileMode); err != nil {
		return fmt.Errorf("locale: write temp settings: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("locale: rename settings: %w", err)
	}
	return nil
}
