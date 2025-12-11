package core

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"singbox-launcher/internal/platform"
)

// ReleaseInfo содержит информацию о релизе GitHub
type ReleaseInfo struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset содержит информацию об asset релиза
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// DownloadProgress содержит информацию о прогрессе скачивания
type DownloadProgress struct {
	Progress int // 0-100
	Message  string
	Status   string // "downloading", "extracting", "done", "error"
	Error    error
}

// DownloadCore downloads and installs sing-box
func (ac *AppController) DownloadCore(ctx context.Context, version string, progressChan chan DownloadProgress) {
	defer close(progressChan)

	// 1. Get release information
	progressChan <- DownloadProgress{Progress: 5, Message: "Getting release information...", Status: "downloading"}
	release, err := ac.getReleaseInfo(ctx, version)
	if err != nil {
		progressChan <- DownloadProgress{Progress: 0, Message: fmt.Sprintf("Failed to get release info: %v", err), Status: "error", Error: err}
		return
	}

	// 2. Находим правильный asset для платформы
	progressChan <- DownloadProgress{Progress: 10, Message: "Finding platform asset...", Status: "downloading"}
	asset, err := ac.findPlatformAsset(release.Assets)
	if err != nil {
		progressChan <- DownloadProgress{Progress: 0, Message: fmt.Sprintf("Failed to find platform asset: %v", err), Status: "error", Error: err}
		return
	}

	// 3. Создаем временную директорию
	tempDir := filepath.Join(ac.ExecDir, "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		progressChan <- DownloadProgress{Progress: 0, Message: fmt.Sprintf("Failed to create temp dir: %v", err), Status: "error", Error: err}
		return
	}
	defer os.RemoveAll(tempDir) // Удаляем временную директорию после завершения

	// 4. Download archive
	archivePath := filepath.Join(tempDir, asset.Name)
	progressChan <- DownloadProgress{Progress: 15, Message: fmt.Sprintf("Downloading %s...", asset.Name), Status: "downloading"}
	if err := ac.downloadFile(ctx, asset.BrowserDownloadURL, archivePath, progressChan); err != nil {
		progressChan <- DownloadProgress{Progress: 0, Message: fmt.Sprintf("Download failed: %v", err), Status: "error", Error: err}
		return
	}

	// 5. Распаковываем архив
	progressChan <- DownloadProgress{Progress: 80, Message: "Extracting archive...", Status: "extracting"}
	binaryPath, err := ac.extractArchive(archivePath, tempDir)
	if err != nil {
		progressChan <- DownloadProgress{Progress: 0, Message: fmt.Sprintf("Extraction failed: %v", err), Status: "error", Error: err}
		return
	}

	// 6. Копируем бинарник в целевую директорию
	progressChan <- DownloadProgress{Progress: 90, Message: "Installing binary...", Status: "extracting"}
	if err := ac.installBinary(binaryPath, ac.SingboxPath); err != nil {
		progressChan <- DownloadProgress{Progress: 0, Message: fmt.Sprintf("Installation failed: %v", err), Status: "error", Error: err}
		return
	}

	// 7. Готово!
	progressChan <- DownloadProgress{Progress: 100, Message: fmt.Sprintf("sing-box v%s installed successfully!", version), Status: "done"}
}

// getReleaseInfo gets release information from GitHub (with SourceForge fallback)
func (ac *AppController) getReleaseInfo(ctx context.Context, version string) (*ReleaseInfo, error) {
	// Try GitHub API first
	release, err := ac.getReleaseInfoFromGitHub(ctx, version)
	if err == nil {
		return release, nil
	}

	log.Printf("GitHub failed, trying SourceForge...")

	// If GitHub doesn't work, try SourceForge
	return ac.getReleaseInfoFromSourceForge(ctx, version)
}

// getReleaseInfoFromGitHub gets release information from GitHub
func (ac *AppController) getReleaseInfoFromGitHub(ctx context.Context, version string) (*ReleaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/SagerNet/sing-box/releases/tags/v%s", version)
	if version == "" {
		url = "https://api.github.com/repos/SagerNet/sing-box/releases/latest"
	}

	// Используем универсальный HTTP клиент
	client := createHTTPClient(NetworkRequestTimeout)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "singbox-launcher/1.0")

	resp, err := client.Do(req)
	if err != nil {
		// Проверяем тип ошибки
		if IsNetworkError(err) {
			return nil, fmt.Errorf("network error: %s", GetNetworkErrorMessage(err))
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var release ReleaseInfo
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &release, nil
}

// getReleaseInfoFromSourceForge creates ReleaseInfo based on SourceForge (builds direct links)
func (ac *AppController) getReleaseInfoFromSourceForge(ctx context.Context, version string) (*ReleaseInfo, error) {
	if version == "" {
		// Если версия не указана, пытаемся получить последнюю с GitHub
		// Если не получилось, используем фиксированную версию
		latest, err := ac.GetLatestCoreVersion()
		if err != nil {
			log.Printf("Failed to get latest version, using fallback: %v", err)
			version = FallbackVersion
		} else {
			version = latest
		}
	}

	// Строим список assets на основе известных платформ
	assets := ac.buildSourceForgeAssets(version)

	return &ReleaseInfo{
		TagName: fmt.Sprintf("v%s", version),
		Assets:  assets,
	}, nil
}

// buildSourceForgeAssets строит список assets для SourceForge
func (ac *AppController) buildSourceForgeAssets(version string) []Asset {
	var assets []Asset

	// Определяем нужный файл для текущей платформы
	var fileName string
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "amd64" {
			fileName = fmt.Sprintf("sing-box-%s-windows-amd64.zip", version)
		} else if runtime.GOARCH == "arm64" {
			fileName = fmt.Sprintf("sing-box-%s-windows-arm64.zip", version)
		}
	case "linux":
		if runtime.GOARCH == "amd64" {
			fileName = fmt.Sprintf("sing-box-%s-linux-amd64.tar.gz", version)
		} else if runtime.GOARCH == "arm64" {
			fileName = fmt.Sprintf("sing-box-%s-linux-arm64.tar.gz", version)
		} else if runtime.GOARCH == "arm" {
			fileName = fmt.Sprintf("sing-box-%s-linux-armv7.tar.gz", version)
		}
	case "darwin":
		if runtime.GOARCH == "amd64" {
			fileName = fmt.Sprintf("sing-box-%s-darwin-amd64.tar.gz", version)
		} else if runtime.GOARCH == "arm64" {
			fileName = fmt.Sprintf("sing-box-%s-darwin-arm64.tar.gz", version)
		}
	}

	if fileName == "" {
		return assets
	}

	// Строим прямую ссылку на SourceForge
	downloadURL := fmt.Sprintf("https://sourceforge.net/projects/sing-box.mirror/files/v%s/%s/download", version, fileName)

	assets = append(assets, Asset{
		Name:               fileName,
		BrowserDownloadURL: downloadURL,
		Size:               0, // Размер неизвестен заранее
	})

	return assets
}

// findPlatformAsset находит правильный asset для текущей платформы
func (ac *AppController) findPlatformAsset(assets []Asset) (*Asset, error) {
	var platformPattern string

	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "amd64" {
			platformPattern = "windows-amd64.zip"
		} else if runtime.GOARCH == "arm64" {
			platformPattern = "windows-arm64.zip"
		} else {
			return nil, fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
		}
	case "linux":
		if runtime.GOARCH == "amd64" {
			platformPattern = "linux-amd64.tar.gz"
		} else if runtime.GOARCH == "arm64" {
			platformPattern = "linux-arm64.tar.gz"
		} else if runtime.GOARCH == "arm" {
			platformPattern = "linux-armv7.tar.gz"
		} else {
			return nil, fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
		}
	case "darwin":
		if runtime.GOARCH == "amd64" {
			platformPattern = "darwin-amd64.tar.gz"
		} else if runtime.GOARCH == "arm64" {
			platformPattern = "darwin-arm64.tar.gz"
		} else {
			return nil, fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
		}
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	for i := range assets {
		if strings.Contains(assets[i].Name, platformPattern) {
			return &assets[i], nil
		}
	}

	return nil, fmt.Errorf("asset not found for platform %s/%s", runtime.GOOS, runtime.GOARCH)
}

// downloadFile downloads a file with progress tracking (with SourceForge fallback)
func (ac *AppController) downloadFile(ctx context.Context, url, destPath string, progressChan chan DownloadProgress) error {
	// Try to download from original URL
	err := ac.downloadFileFromURL(ctx, url, destPath, progressChan)
	if err == nil {
		return nil
	}

	log.Printf("Failed to download from original URL, trying mirrors...")

	// Если не получилось, пробуем зеркала GitHub
	mirrors := []string{
		strings.Replace(url, "https://github.com/", "https://ghproxy.com/https://github.com/", 1),
	}

	for _, mirrorURL := range mirrors {
		log.Printf("Trying mirror: %s", mirrorURL)
		err := ac.downloadFileFromURL(ctx, mirrorURL, destPath, progressChan)
		if err == nil {
			return nil
		}
		log.Printf("Mirror failed: %v", err)
	}

	// Если все зеркала GitHub не работают, пробуем SourceForge
	if strings.Contains(url, "github.com") {
		log.Printf("Trying SourceForge...")
		// Извлекаем версию и имя файла из URL
		version, fileName := ac.extractVersionAndFileName(url)
		if version != "" && fileName != "" {
			sourceForgeURL := fmt.Sprintf("https://sourceforge.net/projects/sing-box.mirror/files/v%s/%s/download", version, fileName)
			err := ac.downloadFileFromURL(ctx, sourceForgeURL, destPath, progressChan)
			if err == nil {
				return nil
			}
			log.Printf("SourceForge failed: %v", err)
		}
	}

	return fmt.Errorf("all download sources failed, last error: %w", err)
}

// downloadFileFromURL downloads a file from a specific URL
func (ac *AppController) downloadFileFromURL(ctx context.Context, url, destPath string, progressChan chan DownloadProgress) error {
	// Use parent context timeout or create one with default timeout
	downloadTimeout := 5 * time.Minute
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, downloadTimeout)
		defer cancel()
	}

	// Use client with large timeout for download
	client := createHTTPClient(downloadTimeout)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "singbox-launcher/1.0")

	resp, err := client.Do(req)
	if err != nil {
		// Проверяем тип ошибки
		if IsNetworkError(err) {
			return fmt.Errorf("network error: %s", GetNetworkErrorMessage(err))
		}
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	totalSize := resp.ContentLength
	var downloaded int64

	// Download with progress tracking
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("download cancelled: %w", ctx.Err())
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			written, writeErr := file.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("write failed: %w", writeErr)
			}
			downloaded += int64(written)

			// Update progress (15-80%)
			if totalSize > 0 {
				progress := 15 + int(float64(downloaded)/float64(totalSize)*65)
				progressChan <- DownloadProgress{
					Progress: progress,
					Message:  "Downloading...",
					Status:   "downloading",
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read failed: %w", err)
		}
	}

	return nil
}

// extractVersionAndFileName извлекает версию и имя файла из GitHub URL
func (ac *AppController) extractVersionAndFileName(url string) (string, string) {
	// Формат GitHub URL: https://github.com/SagerNet/sing-box/releases/download/v1.12.12/sing-box-1.12.12-windows-amd64.zip
	parts := strings.Split(url, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, "v") && len(part) > 1 {
			version := strings.TrimPrefix(part, "v")
			if i+1 < len(parts) {
				fileName := parts[i+1]
				return version, fileName
			}
		}
	}
	return "", ""
}

// extractArchive распаковывает архив и возвращает путь к бинарнику
func (ac *AppController) extractArchive(archivePath, destDir string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return ac.extractZip(archivePath, destDir)
	} else if strings.HasSuffix(archivePath, ".tar.gz") {
		return ac.extractTarGz(archivePath, destDir)
	}
	return "", fmt.Errorf("unsupported archive format")
}

// extractZip распаковывает ZIP архив (Windows)
func (ac *AppController) extractZip(archivePath, destDir string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	singboxName := platform.GetExecutableNames()
	var binaryPath string

	for _, f := range r.File {
		// Ищем sing-box.exe в архиве
		if strings.HasSuffix(f.Name, singboxName) {
			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("failed to open file in zip: %w", err)
			}

			binaryPath = filepath.Join(destDir, filepath.Base(f.Name))
			outFile, err := os.Create(binaryPath)
			if err != nil {
				rc.Close()
				return "", fmt.Errorf("failed to create output file: %w", err)
			}

			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()

			if err != nil {
				return "", fmt.Errorf("failed to copy file: %w", err)
			}

			// Устанавливаем права на выполнение (для Unix-подобных систем)
			if runtime.GOOS != "windows" {
				os.Chmod(binaryPath, 0755)
			}

			return binaryPath, nil
		}
	}

	return "", fmt.Errorf("sing-box binary not found in archive")
}

// extractTarGz распаковывает tar.gz архив (Linux/macOS)
func (ac *AppController) extractTarGz(archivePath, destDir string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	singboxName := platform.GetExecutableNames()
	var binaryPath string

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read tar: %w", err)
		}

		// Ищем sing-box в архиве
		if strings.HasSuffix(header.Name, singboxName) || strings.HasSuffix(header.Name, "sing-box") {
			binaryPath = filepath.Join(destDir, filepath.Base(header.Name))
			outFile, err := os.Create(binaryPath)
			if err != nil {
				return "", fmt.Errorf("failed to create output file: %w", err)
			}

			_, err = io.Copy(outFile, tr)
			outFile.Close()

			if err != nil {
				return "", fmt.Errorf("failed to copy file: %w", err)
			}

			// Устанавливаем права на выполнение
			os.Chmod(binaryPath, 0755)

			return binaryPath, nil
		}
	}

	return "", fmt.Errorf("sing-box binary not found in archive")
}

// installBinary копирует бинарник в целевую директорию
func (ac *AppController) installBinary(sourcePath, destPath string) error {
	// Создаем директорию bin если её нет
	binDir := filepath.Dir(destPath)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// If old binary exists, rename it
	if _, err := os.Stat(destPath); err == nil {
		oldPath := destPath + ".old"
		os.Remove(oldPath) // Remove old backup if exists
		if err := os.Rename(destPath, oldPath); err != nil {
			log.Printf("Warning: failed to rename old binary: %v", err)
		}
	}

	// Copy new binary
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Set execute permissions (for Unix)
	if runtime.GOOS != "windows" {
		os.Chmod(destPath, 0755)
	}

	// Remove old backup
	oldPath := destPath + ".old"
	os.Remove(oldPath)

	log.Printf("Binary installed successfully to %s", destPath)
	return nil
}
