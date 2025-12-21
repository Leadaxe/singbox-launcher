package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/platform"
)

// GetInstalledCoreVersion получает установленную версию sing-box
func (ac *AppController) GetInstalledCoreVersion() (string, error) {
	// Проверяем существование бинарника
	if _, err := os.Stat(ac.FileService.SingboxPath); os.IsNotExist(err) {
		return "", fmt.Errorf("sing-box not found at %s", ac.FileService.SingboxPath)
	}

	// Запускаем sing-box version
	cmd := exec.Command(ac.FileService.SingboxPath, "version")
	platform.PrepareCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("GetInstalledCoreVersion: command failed: %v, output: %q", err, string(output))
		return "", fmt.Errorf("failed to get version: %w", err)
	}

	// Парсим вывод - формат: "sing-box version 1.12.12"
	outputStr := strings.TrimSpace(string(output))
	log.Printf("GetInstalledCoreVersion: raw output: %q", outputStr)

	// Ищем версию после "sing-box version" до конца строки
	versionRegex := regexp.MustCompile(`sing-box version\s+(\S+)`)
	matches := versionRegex.FindStringSubmatch(outputStr)
	if len(matches) > 1 {
		version := matches[1]
		log.Printf("GetInstalledCoreVersion: found version: %s", version)
		return version, nil
	}

	log.Printf("GetInstalledCoreVersion: unable to parse version from output: %q", outputStr)
	return "", fmt.Errorf("unable to parse version from output: %s", outputStr)
}

// GetCoreBinaryPath возвращает путь к бинарнику sing-box для отображения
func (ac *AppController) GetCoreBinaryPath() string {
	singboxName := platform.GetExecutableNames()
	// Для отображения убираем полный путь, оставляем только bin/sing-box.exe или bin/sing-box
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	relPath, err := filepath.Rel(ac.FileService.ExecDir, binDir)
	if err != nil {
		// Если не удалось получить относительный путь, возвращаем просто имя
		return singboxName
	}
	return filepath.Join(relPath, singboxName)
}

// CoreVersionInfo содержит информацию о версии sing-box
type CoreVersionInfo struct {
	InstalledVersion string
	LatestVersion    string
	UpdateAvailable  bool
	Error            string
}

// FallbackVersion - фиксированная версия для использования, если не удается получить последнюю
const FallbackVersion = "1.12.12"

// GetLatestCoreVersion получает последнюю версию sing-box (с fallback на фиксированную версию)
func (ac *AppController) GetLatestCoreVersion() (string, error) {
	sources := []struct {
		name string
		url  string
	}{
		{"GitHub API", "https://api.github.com/repos/SagerNet/sing-box/releases/latest"},
		{"GitHub Mirror (ghproxy)", "https://ghproxy.com/https://api.github.com/repos/SagerNet/sing-box/releases/latest"},
	}

	for _, source := range sources {
		log.Printf("Trying to get latest version from %s...", source.name)
		version, err := ac.getLatestVersionFromURL(source.url)
		if err == nil {
			log.Printf("Successfully got latest version %s from %s", version, source.name)
			return version, nil
		}
		log.Printf("Failed to get latest version from %s: %v", source.name, err)
	}

	// Если GitHub недоступен, используем фиксированную версию для скачивания с SourceForge
	log.Printf("All GitHub sources failed, using fallback version %s from SourceForge", FallbackVersion)
	return FallbackVersion, nil
}

// GetLatestLauncherVersion получает последнюю версию приложения из GitHub
func (ac *AppController) GetLatestLauncherVersion() (string, error) {
	sources := []struct {
		name string
		url  string
	}{
		{"GitHub API", "https://api.github.com/repos/Leadaxe/singbox-launcher/releases/latest"},
		{"GitHub Mirror (ghproxy)", "https://ghproxy.com/https://api.github.com/repos/Leadaxe/singbox-launcher/releases/latest"},
	}

	for _, source := range sources {
		log.Printf("Trying to get latest launcher version from %s...", source.name)
		// Сохраняем префикс "v" для launcher версии
		version, err := ac.getLatestVersionFromURLWithPrefix(source.url, true)
		if err == nil {
			log.Printf("Successfully got latest launcher version %s from %s", version, source.name)
			return version, nil
		}
		log.Printf("Failed to get latest launcher version from %s: %v", source.name, err)
	}

	return "", fmt.Errorf("failed to get latest launcher version from all sources")
}

// GetCachedLauncherVersion возвращает закешированную версию launcher (если есть)
func (ac *AppController) GetCachedLauncherVersion() string {
	if ac.StateService != nil {
		return ac.StateService.GetCachedLauncherVersion()
	}
	return ""
}

// SetCachedLauncherVersion сохраняет версию launcher в кеш
func (ac *AppController) SetCachedLauncherVersion(version string) {
	if ac.StateService != nil {
		ac.StateService.SetCachedLauncherVersion(version)
	}
}

// CheckLauncherVersionOnStartup выполняет разовую проверку версии launcher при старте
func (ac *AppController) CheckLauncherVersionOnStartup() {
	// Проверяем, не идет ли уже проверка
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

		// Пытаемся получить последнюю версию
		latest, err := ac.GetLatestLauncherVersion()
		if err != nil {
			log.Printf("CheckLauncherVersionOnStartup: Failed to get latest launcher version: %v", err)
			return
		}

		// Сохраняем в кеш
		ac.SetCachedLauncherVersion(latest)
		log.Printf("CheckLauncherVersionOnStartup: Successfully cached launcher version %s", latest)
	}()
}

// ShouldCheckVersion проверяет, нужно ли проверять версию
// Если версия успешно получена (не FallbackVersion), проверки прекращаются до перезапуска приложения
func (ac *AppController) ShouldCheckVersion() bool {
	if ac.StateService == nil {
		return true
	}

	cachedVersion := ac.StateService.GetCachedVersion()
	cacheTime := ac.StateService.GetCachedVersionTime()

	// Если кеш пустой или время не установлено - нужно проверить
	if cachedVersion == "" || cacheTime.IsZero() {
		return true
	}

	// Если версия успешно получена (не FallbackVersion), прекращаем проверки до перезапуска
	// Это означает, что версия была получена из GitHub, а не fallback
	if cachedVersion != FallbackVersion {
		return false // Версия успешно получена, не проверяем до перезапуска
	}

	// Если это FallbackVersion, проверяем периодически (каждые 24 часа)
	// чтобы попытаться получить реальную версию
	timeSinceCheck := time.Since(cacheTime)
	if timeSinceCheck >= 24*time.Hour {
		return true
	}

	// Иначе не нужно проверять
	return false
}

// GetCachedVersion возвращает закешированную версию (если есть)
func (ac *AppController) GetCachedVersion() string {
	if ac.StateService != nil {
		return ac.StateService.GetCachedVersion()
	}
	return ""
}

// SetCachedVersion сохраняет версию в кеш
func (ac *AppController) SetCachedVersion(version string) {
	if ac.StateService != nil {
		ac.StateService.SetCachedVersion(version)
	}
}

// CheckVersionInBackground запускает фоновую проверку версии с логикой повторных попыток
func (ac *AppController) CheckVersionInBackground() {
	// Сначала проверяем, нужно ли вообще проверять версию (если уже есть в кеше и не прошло 24 часа)
	// Это нужно делать ДО блокировки мьютекса, чтобы избежать deadlock
	if !ac.ShouldCheckVersion() {
		return
	}

	// Теперь проверяем, не идет ли уже проверка (атомарно)
	if ac.StateService == nil {
		return
	}
	if ac.StateService.IsVersionCheckInProgress() {
		return // Уже запущена проверка
	}
	// Устанавливаем флаг
	ac.StateService.SetVersionCheckInProgress(true)

	go func() {
		defer func() {
			if ac.StateService != nil {
				ac.StateService.SetVersionCheckInProgress(false)
			}
		}()

		// Логика повторных попыток
		maxQuickAttempts := 10
		maxTotalAttempts := 30 // Максимальное количество попыток всего
		quickInterval := 1 * time.Minute
		slowInterval := 5 * time.Minute
		verySlowInterval := 30 * time.Minute // Очень медленный интервал после многих попыток

		attemptCount := 0
		for attemptCount < maxTotalAttempts {
			// Проверяем кеш перед каждой попыткой - возможно версия уже получена другой горутиной
			if !ac.ShouldCheckVersion() {
				log.Println("CheckVersionInBackground: Version already cached, stopping")
				return
			}

			// Определяем интервал в зависимости от количества попыток
			var interval time.Duration
			if attemptCount < maxQuickAttempts {
				interval = quickInterval
			} else if attemptCount < 20 {
				interval = slowInterval
			} else {
				interval = verySlowInterval
			}

			// Ждем перед попыткой (кроме первой)
			if attemptCount > 0 {
				select {
				case <-ac.ctx.Done():
					log.Println("CheckVersionInBackground: Stopped (context cancelled)")
					return
				case <-time.After(interval):
					// Continue
				}
			}

			// Еще раз проверяем кеш после ожидания - возможно версия была получена во время ожидания
			if !ac.ShouldCheckVersion() {
				log.Println("CheckVersionInBackground: Version already cached during wait, stopping")
				return
			}

			attemptCount++
			log.Printf("CheckVersionInBackground: Attempt %d/%d to get latest version", attemptCount, maxTotalAttempts)

			// Пытаемся получить версию
			version, err := ac.GetLatestCoreVersion()
			if err == nil {
				// Успех - сохраняем в кеш (даже если это FallbackVersion) и выходим
				// Это предотвращает бесконечные проверки
				ac.SetCachedVersion(version)
				if version == FallbackVersion {
					log.Printf("CheckVersionInBackground: Using fallback version %s (GitHub unavailable), will retry in 24h", version)
				} else {
					log.Printf("CheckVersionInBackground: Successfully cached version %s, checks stopped until app restart", version)
				}
				return
			}

			if err != nil {
				log.Printf("CheckVersionInBackground: Attempt %d failed: %v", attemptCount, err)
			}
		}

		// Если достигли максимального количества попыток, сохраняем FallbackVersion
		log.Printf("CheckVersionInBackground: Reached max attempts (%d), using fallback version %s", maxTotalAttempts, FallbackVersion)
		ac.SetCachedVersion(FallbackVersion)
	}()
}

// getLatestVersionFromURL получает последнюю версию по конкретному URL
func (ac *AppController) getLatestVersionFromURL(url string) (string, error) {
	return ac.getLatestVersionFromURLWithPrefix(url, false)
}

// getLatestVersionFromURLWithPrefix получает последнюю версию по конкретному URL
// keepPrefix: если true, сохраняет префикс "v" в версии
func (ac *AppController) getLatestVersionFromURLWithPrefix(url string, keepPrefix bool) (string, error) {
	// Создаем контекст с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), NetworkRequestTimeout)
	defer cancel()

	// Используем универсальный HTTP клиент
	client := CreateHTTPClient(NetworkRequestTimeout)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "singbox-launcher/1.0")

	resp, err := client.Do(req)
	if err != nil {
		// Проверяем тип ошибки
		if IsNetworkError(err) {
			return "", fmt.Errorf("network error: %s", GetNetworkErrorMessage(err))
		}
		return "", fmt.Errorf("check failed: %w", err)
	}
	defer resp.Body.Close()

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

	// Убираем префикс "v" если нужно (для sing-box убираем, для launcher сохраняем)
	version := release.TagName
	if !keepPrefix {
		version = strings.TrimPrefix(version, "v")
	}
	return version, nil
}

// CheckForUpdates проверяет наличие обновлений и показывает результат пользователю.
// Запускает синхронную проверку версии и отображает диалог с информацией.
func (ac *AppController) CheckForUpdates() {
	// Показываем информационное сообщение о начале проверки
	if ac.UIService != nil && ac.UIService.MainWindow != nil {
		dialogs.ShowInfo(ac.UIService.MainWindow, "Checking for Updates", "Checking for updates...")
	}

	// Запускаем проверку версии в фоне
	go func() {
		// Сбрасываем кеш, чтобы принудительно проверить версию
		if ac.StateService != nil {
			ac.StateService.SetCachedVersion("")
		}

		// Пытаемся получить последнюю версию
		latest, err := ac.GetLatestCoreVersion()
		if err != nil {
			log.Printf("CheckForUpdates: Failed to get latest version: %v", err)
			fyne.Do(func() {
				if ac.UIService != nil && ac.UIService.MainWindow != nil {
					dialogs.ShowError(ac.UIService.MainWindow, fmt.Errorf("Failed to check for updates: %v", err))
				}
			})
			return
		}

		// Сохраняем версию в кеш перед получением информации
		ac.SetCachedVersion(latest)

		// Получаем информацию о версиях
		info := ac.GetCoreVersionInfo()
		if info.Error != "" {
			fyne.Do(func() {
				if ac.UIService != nil && ac.UIService.MainWindow != nil {
					dialogs.ShowError(ac.UIService.MainWindow, fmt.Errorf("Error checking version: %s", info.Error))
				}
			})
			return
		}

		// Формируем сообщение для пользователя
		var message string
		if info.UpdateAvailable {
			message = fmt.Sprintf("Update available!\n\nInstalled: %s\nLatest: %s\n\nYou can download the update from the Core tab.",
				info.InstalledVersion, info.LatestVersion)
			fyne.Do(func() {
				if ac.UIService != nil && ac.UIService.MainWindow != nil {
					dialogs.ShowInfo(ac.UIService.MainWindow, "Update Available", message)
				}
			})
		} else {
			message = fmt.Sprintf("You are using the latest version.\n\nInstalled: %s\nLatest: %s",
				info.InstalledVersion, info.LatestVersion)
			fyne.Do(func() {
				if ac.UIService != nil && ac.UIService.MainWindow != nil {
					dialogs.ShowInfo(ac.UIService.MainWindow, "No Updates", message)
				}
			})
		}
	}()
}

// GetCoreVersionInfo получает полную информацию о версии
// Использует кешированную версию, если она доступна и валидна
func (ac *AppController) GetCoreVersionInfo() CoreVersionInfo {
	info := CoreVersionInfo{}

	// Получаем установленную версию
	installed, err := ac.GetInstalledCoreVersion()
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.InstalledVersion = installed

	// Используем кешированную версию, если она есть
	latest := ac.GetCachedVersion()
	if latest == "" {
		// Если кеша нет, пытаемся получить версию (но не блокируем UI)
		// В этом случае просто не показываем информацию об обновлении
		info.LatestVersion = ""
		return info
	}
	info.LatestVersion = latest

	// Сравниваем версии
	info.UpdateAvailable = CompareVersions(installed, latest) < 0

	return info
}

// CompareVersions сравнивает две версии (формат X.Y.Z)
// Возвращает: -1 если v1 < v2, 0 если v1 == v2, 1 если v1 > v2
func CompareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

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
