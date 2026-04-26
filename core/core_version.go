package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// GetInstalledCoreVersion получает установленную версию sing-box.
// После первой успешной проверки в этой сессии возвращает закешированное
// значение без повторного запуска `sing-box version`.
func (ac *AppController) GetInstalledCoreVersion() (string, error) {
	ac.installedCoreVersionCacheMu.Lock()
	defer ac.installedCoreVersionCacheMu.Unlock()
	if ac.installedCoreVersionCache != "" {
		return ac.installedCoreVersionCache, nil
	}

	if _, err := os.Stat(ac.FileService.SingboxPath); os.IsNotExist(err) {
		return "", fmt.Errorf("sing-box not found at %s", ac.FileService.SingboxPath)
	}

	cmd := exec.Command(ac.FileService.SingboxPath, "version")
	platform.PrepareCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		debuglog.WarnLog("GetInstalledCoreVersion: command failed: %v, output: %q", err, string(output))
		return "", fmt.Errorf("failed to get version: %w", err)
	}

	outputStr := strings.TrimSpace(string(output))
	versionRegex := regexp.MustCompile(`sing-box version\s+(\S+)`)
	matches := versionRegex.FindStringSubmatch(outputStr)
	if len(matches) > 1 {
		ac.installedCoreVersionCache = matches[1]
		return ac.installedCoreVersionCache, nil
	}

	debuglog.WarnLog("GetInstalledCoreVersion: unable to parse version from output: %q", outputStr)
	return "", fmt.Errorf("unable to parse version from output: %s", outputStr)
}

// GetCoreBinaryPath возвращает путь к бинарнику sing-box для отображения.
func (ac *AppController) GetCoreBinaryPath() string {
	p := ac.FileService.SingboxPath
	rel, err := filepath.Rel(ac.FileService.ExecDir, p)
	if err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return p
}

// GetLatestLauncherVersion получает последнюю версию лаунчера из GitHub.
// (Sing-box версия не проверяется — она пиннится через constants.RequiredCoreVersion;
// см. SPEC 046.)
func (ac *AppController) GetLatestLauncherVersion() (string, error) {
	sources := []struct {
		name string
		url  string
	}{
		{"GitHub API", "https://api.github.com/repos/Leadaxe/singbox-launcher/releases/latest"},
		{"GitHub Mirror (ghproxy)", "https://ghproxy.com/https://api.github.com/repos/Leadaxe/singbox-launcher/releases/latest"},
	}

	for _, source := range sources {
		debuglog.DebugLog("Trying to get latest launcher version from %s...", source.name)
		// Сохраняем префикс "v" для launcher версии (releases tagged как v0.8.x).
		version, err := ac.getLatestVersionFromURLWithPrefix(source.url, true)
		if err == nil {
			debuglog.InfoLog("Successfully got latest launcher version %s from %s", version, source.name)
			return version, nil
		}
		debuglog.DebugLog("Failed to get latest launcher version from %s: %v", source.name, err)
	}

	return "", fmt.Errorf("failed to get latest launcher version from all sources")
}

// GetCachedLauncherVersion возвращает закешированную версию лаунчера (если есть).
func (ac *AppController) GetCachedLauncherVersion() string {
	if ac.StateService != nil {
		return ac.StateService.GetCachedLauncherVersion()
	}
	return ""
}

// SetCachedLauncherVersion сохраняет версию лаунчера в кеш.
func (ac *AppController) SetCachedLauncherVersion(version string) {
	if ac.StateService != nil {
		ac.StateService.SetCachedLauncherVersion(version)
	}
}

// CheckLauncherVersionOnStartup выполняет разовую проверку версии лаунчера при старте.
// Проверка всегда выполняется и сохраняет результат в кеш. Попап с обновлением
// показывается при первом отображении окна (через OnWindowShown).
func (ac *AppController) CheckLauncherVersionOnStartup() {
	if ac.StateService == nil {
		return
	}
	if ac.StateService.IsLauncherVersionCheckInProgress() {
		return
	}
	ac.StateService.SetLauncherVersionCheckInProgress(true)

	go func() {
		defer func() {
			if ac.StateService != nil {
				ac.StateService.SetLauncherVersionCheckInProgress(false)
			}
		}()

		latest, err := ac.GetLatestLauncherVersion()
		if err != nil {
			debuglog.WarnLog("CheckLauncherVersionOnStartup: Failed to get latest launcher version: %v", err)
			return
		}

		ac.SetCachedLauncherVersion(latest)
		debuglog.InfoLog("CheckLauncherVersionOnStartup: Successfully cached launcher version %s", latest)
	}()
}

// getLatestVersionFromURLWithPrefix получает последнюю версию по конкретному URL.
// keepPrefix: если true, сохраняет префикс "v" в версии (для launcher releases
// — они отдаются в формате `v0.8.x`).
func (ac *AppController) getLatestVersionFromURLWithPrefix(url string, keepPrefix bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), NetworkRequestTimeout)
	defer cancel()

	client := CreateHTTPClient(NetworkRequestTimeout)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "singbox-launcher/1.0")

	resp, err := client.Do(req)
	defer func() {
		if resp != nil {
			debuglog.RunAndLog("getLatestVersionFromURLWithPrefix: close response body", resp.Body.Close)
		}
	}()
	if err != nil {
		if IsNetworkError(err) {
			return "", fmt.Errorf("network error: %s", GetNetworkErrorMessage(err))
		}
		return "", fmt.Errorf("check failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("check failed: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}

	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	version := release.TagName
	if !keepPrefix {
		version = strings.TrimPrefix(version, "v")
	}
	return version, nil
}

// CompareVersions сравнивает две версии (формат X.Y.Z или X.Y.Z-N-hash или X.Y.Z-dev.branch-hash).
// Возвращает: -1 если v1 < v2, 0 если v1 == v2, 1 если v1 > v2.
func CompareVersions(v1, v2 string) int {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	base1, hasSuffix1 := extractBaseVersion(v1)
	base2, hasSuffix2 := extractBaseVersion(v2)

	baseCompare := compareBaseVersions(base1, base2)
	if baseCompare != 0 {
		return baseCompare
	}

	// Если базовые версии равны — версия с суффиксом (коммиты после тега
	// или dev) считается новее. v0.7.1-96-gc1343cc > v0.7.1.
	if hasSuffix1 && !hasSuffix2 {
		return 1
	}
	if !hasSuffix1 && hasSuffix2 {
		return -1
	}

	return 0
}

// extractBaseVersion извлекает базовую версию и проверяет наличие суффикса.
// Форматы: "0.7.1", "0.7.1-96-gc1343cc", "0.7.1-dev.branch-hash".
func extractBaseVersion(version string) (base string, hasSuffix bool) {
	idx := strings.Index(version, "-")
	if idx == -1 {
		return version, false
	}
	return version[:idx], true
}

// compareBaseVersions сравнивает базовые версии (формат X.Y.Z).
func compareBaseVersions(base1, base2 string) int {
	parts1 := strings.Split(base1, ".")
	parts2 := strings.Split(base2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var num1, num2 int
		if i < len(parts1) {
			_, _ = fmt.Sscanf(parts1[i], "%d", &num1)
		}
		if i < len(parts2) {
			_, _ = fmt.Sscanf(parts2[i], "%d", &num2)
		}

		if num1 < num2 {
			return -1
		}
		if num1 > num2 {
			return 1
		}
	}

	return 0
}

// ShowUpdatePopupIfAvailable проверяет наличие обновления лаунчера и показывает
// попап. Сравнение всегда против `constants.AppVersion` (запущенный лаунчер) и
// закешированной из GitHub `GetCachedLauncherVersion`. Sing-box версия здесь
// не участвует — она pinned через `RequiredCoreVersion` (см. SPEC 046).
func (ac *AppController) ShowUpdatePopupIfAvailable() {
	if ac.isUpdatePopupShown() {
		debuglog.DebugLog("ShowUpdatePopupIfAvailable: Update popup already shown, skipping")
		return
	}

	currentVersion := constants.AppVersion
	currentVersionClean := strings.TrimPrefix(currentVersion, "v")

	latestVersion := ac.GetCachedLauncherVersion()
	if latestVersion == "" {
		debuglog.DebugLog("ShowUpdatePopupIfAvailable: No cached version available, skipping popup")
		return
	}
	latestVersionClean := strings.TrimPrefix(latestVersion, "v")

	if CompareVersions(currentVersionClean, latestVersionClean) >= 0 {
		debuglog.DebugLog("ShowUpdatePopupIfAvailable: No update available (current: %s, latest: %s)", currentVersion, latestVersion)
		return
	}

	debuglog.InfoLog("ShowUpdatePopupIfAvailable: Update available (current: %s, latest: %s), triggering popup callback", currentVersion, latestVersion)
	if ac.UIService != nil && ac.UIService.ShowUpdatePopupFunc != nil {
		ac.UIService.ShowUpdatePopupFunc(currentVersion, latestVersion)
	} else {
		debuglog.WarnLog("ShowUpdatePopupIfAvailable: ShowUpdatePopupFunc callback not set")
	}
}
