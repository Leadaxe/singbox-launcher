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

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// ReleaseInfo contains information about GitHub release
type ReleaseInfo struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset contains information about release asset
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// DownloadProgress contains information about download progress
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

	// 2. Find correct asset for platform
	progressChan <- DownloadProgress{Progress: 10, Message: "Finding platform asset...", Status: "downloading"}
	asset, err := ac.findPlatformAsset(release.Assets)
	if err != nil {
		progressChan <- DownloadProgress{Progress: 0, Message: fmt.Sprintf("Failed to find platform asset: %v", err), Status: "error", Error: fmt.Errorf("DownloadCore: %w", err)}
		return
	}

	// 3. Create temporary directory
	tempDir := filepath.Join(ac.FileService.ExecDir, "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		progressChan <- DownloadProgress{Progress: 0, Message: fmt.Sprintf("Failed to create temp dir: %v", err), Status: "error", Error: fmt.Errorf("DownloadCore: failed to create temp dir: %w", err)}
		return
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			log.Printf("DownloadCore: failed to remove temp dir %s: %v", tempDir, err)
		}
	}()

	// 4. Download archive
	archivePath := filepath.Join(tempDir, asset.Name)
	progressChan <- DownloadProgress{Progress: 15, Message: fmt.Sprintf("Downloading %s...", asset.Name), Status: "downloading"}
	if err := ac.downloadFile(ctx, asset.BrowserDownloadURL, archivePath, progressChan); err != nil {
		progressChan <- DownloadProgress{Progress: 0, Message: fmt.Sprintf("Download failed: %v", err), Status: "error", Error: fmt.Errorf("DownloadCore: %w", err)}
		return
	}

	// 5. Extract archive
	progressChan <- DownloadProgress{Progress: 80, Message: "Extracting archive...", Status: "extracting"}
	binaryPath, err := ac.extractArchive(archivePath, tempDir)
	if err != nil {
		progressChan <- DownloadProgress{Progress: 0, Message: fmt.Sprintf("Extraction failed: %v", err), Status: "error", Error: fmt.Errorf("DownloadCore: %w", err)}
		return
	}

	// 6. Copy binary to target directory
	progressChan <- DownloadProgress{Progress: 90, Message: "Installing binary...", Status: "extracting"}
	if err := ac.installBinary(binaryPath, ac.FileService.SingboxPath); err != nil {
		progressChan <- DownloadProgress{Progress: 0, Message: fmt.Sprintf("Installation failed: %v", err), Status: "error", Error: fmt.Errorf("DownloadCore: %w", err)}
		return
	}

	// 7. Done!
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
	client := CreateHTTPClient(NetworkRequestTimeout)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("getReleaseInfoFromGitHub: failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "singbox-launcher/1.0")

	resp, err := client.Do(req)
	if err != nil {
		// Check error type
		if IsNetworkError(err) {
			return nil, fmt.Errorf("getReleaseInfoFromGitHub: network error: %s", GetNetworkErrorMessage(err))
		}
		return nil, fmt.Errorf("getReleaseInfoFromGitHub: request failed: %w", err)
	}
	defer debuglog.CloseWithLog("getReleaseInfoFromGitHub: response body", resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getReleaseInfoFromGitHub: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("getReleaseInfoFromGitHub: failed to read response: %w", err)
	}

	var release ReleaseInfo
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("getReleaseInfoFromGitHub: failed to parse response: %w", err)
	}

	return &release, nil
}

// getReleaseInfoFromSourceForge creates ReleaseInfo based on SourceForge (builds direct links)
func (ac *AppController) getReleaseInfoFromSourceForge(ctx context.Context, version string) (*ReleaseInfo, error) {
	if version == "" {
		// If version is not specified, try to get latest from GitHub
		// If that fails, use fixed version
		latest, err := ac.GetLatestCoreVersion()
		if err != nil {
			log.Printf("getReleaseInfoFromSourceForge: failed to get latest version, using fallback: %v", err)
			version = FallbackVersion
		} else {
			version = latest
		}
	}

	// Build list of assets based on known platforms
	assets := ac.buildSourceForgeAssets(version)

	return &ReleaseInfo{
		TagName: fmt.Sprintf("v%s", version),
		Assets:  assets,
	}, nil
}

// buildSourceForgeAssets builds list of assets for SourceForge
func (ac *AppController) buildSourceForgeAssets(version string) []Asset {
	var assets []Asset

	// Determine required file for current platform
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

	// Build direct link to SourceForge
	downloadURL := fmt.Sprintf("https://sourceforge.net/projects/sing-box.mirror/files/v%s/%s/download", version, fileName)

	assets = append(assets, Asset{
		Name:               fileName,
		BrowserDownloadURL: downloadURL,
		Size:               0, // Size is unknown in advance
	})

	return assets
}

// findPlatformAsset finds the correct asset for current platform
func (ac *AppController) findPlatformAsset(assets []Asset) (*Asset, error) {
	var platformPattern string

	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "amd64" {
			platformPattern = "windows-amd64.zip"
		} else if runtime.GOARCH == "arm64" {
			platformPattern = "windows-arm64.zip"
		} else {
			return nil, fmt.Errorf("findPlatformAsset: unsupported architecture: %s", runtime.GOARCH)
		}
	case "linux":
		if runtime.GOARCH == "amd64" {
			platformPattern = "linux-amd64.tar.gz"
		} else if runtime.GOARCH == "arm64" {
			platformPattern = "linux-arm64.tar.gz"
		} else if runtime.GOARCH == "arm" {
			platformPattern = "linux-armv7.tar.gz"
		} else {
			return nil, fmt.Errorf("findPlatformAsset: unsupported architecture: %s", runtime.GOARCH)
		}
	case "darwin":
		if runtime.GOARCH == "amd64" {
			platformPattern = "darwin-amd64.tar.gz"
		} else if runtime.GOARCH == "arm64" {
			platformPattern = "darwin-arm64.tar.gz"
		} else {
			return nil, fmt.Errorf("findPlatformAsset: unsupported architecture: %s", runtime.GOARCH)
		}
	default:
		return nil, fmt.Errorf("findPlatformAsset: unsupported platform: %s", runtime.GOOS)
	}

	for i := range assets {
		if strings.Contains(assets[i].Name, platformPattern) {
			return &assets[i], nil
		}
	}

	return nil, fmt.Errorf("findPlatformAsset: asset not found for platform %s/%s", runtime.GOOS, runtime.GOARCH)
}

// downloadFile downloads a file with progress tracking (with SourceForge fallback)
func (ac *AppController) downloadFile(ctx context.Context, url, destPath string, progressChan chan DownloadProgress) error {
	// Try to download from original URL
	err := ac.downloadFileFromURL(ctx, url, destPath, progressChan)
	if err == nil {
		return nil
	}

	log.Printf("downloadFile: failed to download from original URL, trying mirrors...")

	// If that didn't work, try GitHub mirrors
	mirrors := []string{
		strings.Replace(url, "https://github.com/", "https://ghproxy.com/https://github.com/", 1),
	}

	for _, mirrorURL := range mirrors {
		log.Printf("downloadFile: trying mirror: %s", mirrorURL)
		err := ac.downloadFileFromURL(ctx, mirrorURL, destPath, progressChan)
		if err == nil {
			return nil
		}
		log.Printf("downloadFile: mirror failed: %v", err)
	}

	// If all GitHub mirrors don't work, try SourceForge
	if strings.Contains(url, "github.com") {
		log.Printf("downloadFile: trying SourceForge...")
		// Extract version and file name from URL
		version, fileName := ac.extractVersionAndFileName(url)
		if version != "" && fileName != "" {
			sourceForgeURL := fmt.Sprintf("https://sourceforge.net/projects/sing-box.mirror/files/v%s/%s/download", version, fileName)
			err := ac.downloadFileFromURL(ctx, sourceForgeURL, destPath, progressChan)
			if err == nil {
				return nil
			}
			log.Printf("downloadFile: SourceForge failed: %v", err)
		}
	}

	return fmt.Errorf("downloadFile: all download sources failed, last error: %w", err)
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
	client := CreateHTTPClient(downloadTimeout)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("downloadFileFromURL: failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "singbox-launcher/1.0")

	resp, err := client.Do(req)
	if err != nil {
		// Check error type
		if IsNetworkError(err) {
			return fmt.Errorf("downloadFileFromURL: network error: %s", GetNetworkErrorMessage(err))
		}
		return fmt.Errorf("downloadFileFromURL: request failed: %w", err)
	}
	defer debuglog.CloseWithLog(fmt.Sprintf("downloadFileFromURL: response body %s", url), resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloadFileFromURL: HTTP %d", resp.StatusCode)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("downloadFileFromURL: failed to create file: %w", err)
	}
	defer debuglog.CloseWithLog(fmt.Sprintf("downloadFileFromURL: file %s", destPath), file)

	totalSize := resp.ContentLength
	var downloaded int64

	// Download with progress tracking
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("downloadFileFromURL: download cancelled: %w", ctx.Err())
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			written, writeErr := file.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("downloadFileFromURL: write failed: %w", writeErr)
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
			return fmt.Errorf("downloadFileFromURL: read failed: %w", err)
		}
	}

	return nil
}

// extractVersionAndFileName extracts version and file name from GitHub URL
func (ac *AppController) extractVersionAndFileName(url string) (string, string) {
	// GitHub URL format: https://github.com/SagerNet/sing-box/releases/download/v1.12.12/sing-box-1.12.12-windows-amd64.zip
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

// extractArchive extracts archive and returns path to binary
func (ac *AppController) extractArchive(archivePath, destDir string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return ac.extractZip(archivePath, destDir)
	} else if strings.HasSuffix(archivePath, ".tar.gz") {
		return ac.extractTarGz(archivePath, destDir)
	}
	return "", fmt.Errorf("extractArchive: unsupported archive format")
}

// extractZip extracts ZIP archive (Windows)
func (ac *AppController) extractZip(archivePath, destDir string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("extractZip: failed to open zip: %w", err)
	}
	defer debuglog.CloseWithLog(fmt.Sprintf("extractZip: zip reader %s", archivePath), r)

	singboxName := platform.GetExecutableNames()
	var binaryPath string

	for _, f := range r.File {
		// Search for sing-box.exe in archive
		if strings.HasSuffix(f.Name, singboxName) {
			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("extractZip: failed to open file in zip: %w", err)
			}

			binaryPath = filepath.Join(destDir, filepath.Base(f.Name))
			outFile, err := os.Create(binaryPath)
			if err != nil {
				debuglog.CloseWithLog(fmt.Sprintf("extractZip: zip entry %s after create error", f.Name), rc)
				return "", fmt.Errorf("extractZip: failed to create output file: %w", err)
			}

			_, err = io.Copy(outFile, rc)
			debuglog.CloseWithLog(fmt.Sprintf("extractZip: output file %s", binaryPath), outFile)
			debuglog.CloseWithLog(fmt.Sprintf("extractZip: zip entry %s", f.Name), rc)

			if err != nil {
				return "", fmt.Errorf("extractZip: failed to copy file: %w", err)
			}

			// Set execute permissions (for Unix-like systems)
			if runtime.GOOS != "windows" {
				if err := os.Chmod(binaryPath, 0755); err != nil {
					log.Printf("extractZip: failed to chmod %s: %v", binaryPath, err)
				}
			}

			return binaryPath, nil
		}
	}

	return "", fmt.Errorf("extractZip: sing-box binary not found in archive")
}

// extractTarGz extracts tar.gz archive (Linux/macOS)
func (ac *AppController) extractTarGz(archivePath, destDir string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("extractTarGz: failed to open archive: %w", err)
	}
	defer debuglog.CloseWithLog(fmt.Sprintf("extractTarGz: archive %s", archivePath), file)

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("extractTarGz: failed to create gzip reader: %w", err)
	}
	defer debuglog.CloseWithLog(fmt.Sprintf("extractTarGz: gzip reader %s", archivePath), gzr)

	tr := tar.NewReader(gzr)
	singboxName := platform.GetExecutableNames()
	var binaryPath string

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("extractTarGz: failed to read tar: %w", err)
		}

		// Search for sing-box in archive
		if strings.HasSuffix(header.Name, singboxName) || strings.HasSuffix(header.Name, "sing-box") {
			binaryPath = filepath.Join(destDir, filepath.Base(header.Name))
			outFile, err := os.Create(binaryPath)
			if err != nil {
				return "", fmt.Errorf("extractTarGz: failed to create output file: %w", err)
			}

			_, err = io.Copy(outFile, tr)
			debuglog.CloseWithLog(fmt.Sprintf("extractTarGz: output file %s", binaryPath), outFile)

			if err != nil {
				return "", fmt.Errorf("extractTarGz: failed to copy file: %w", err)
			}

			// Set execute permissions
			if err := os.Chmod(binaryPath, 0755); err != nil {
				log.Printf("extractTarGz: failed to chmod %s: %v", binaryPath, err)
			}

			return binaryPath, nil
		}
	}

	return "", fmt.Errorf("extractTarGz: sing-box binary not found in archive")
}

// installBinary copies binary to target directory
func (ac *AppController) installBinary(sourcePath, destPath string) error {
	// Create bin directory if it doesn't exist
	binDir := filepath.Dir(destPath)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("installBinary: failed to create bin directory: %w", err)
	}

	// If old binary exists, rename it
	if _, err := os.Stat(destPath); err == nil {
		oldPath := destPath + ".old"
		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			log.Printf("installBinary: failed to remove old backup %s: %v", oldPath, err)
		}
		if err := os.Rename(destPath, oldPath); err != nil {
			log.Printf("Warning: failed to rename old binary: %v", err)
		}
	}

	// Copy new binary
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("installBinary: failed to open source file: %w", err)
	}
	defer debuglog.CloseWithLog(fmt.Sprintf("installBinary: source file %s", sourcePath), sourceFile)

	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("installBinary: failed to create destination file: %w", err)
	}
	defer debuglog.CloseWithLog(fmt.Sprintf("installBinary: destination file %s", destPath), destFile)

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("installBinary: failed to copy file: %w", err)
	}

	// Set execute permissions (for Unix)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(destPath, 0755); err != nil {
			log.Printf("installBinary: failed to chmod %s: %v", destPath, err)
		}
	}

	// Remove old backup
	oldPath := destPath + ".old"
	if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
		log.Printf("installBinary: failed to remove backup %s: %v", oldPath, err)
	}

	log.Printf("installBinary: binary installed successfully to %s", destPath)
	return nil
}
