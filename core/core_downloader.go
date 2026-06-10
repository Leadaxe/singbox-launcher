package core

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// coreReleaseRepo returns the GitHub "owner/repo" the core is downloaded from.
// Since SPEC 072 (Variant A, live from fork v1.13.13-lx.5) the sing-box-lx fork
// builds every platform — including the Windows 7 (windows/386)
// `legacy-windows-7` asset — so there is no per-platform split anymore (no
// upstream/SourceForge legacy path).
func coreReleaseRepo() string {
	return coreReleaseRepoFor(runtime.GOOS, runtime.GOARCH)
}

// coreReleaseRepoFor is the pure form of coreReleaseRepo, kept as a seam so the
// repo selection stays unit-testable. All platforms resolve to the fork.
func coreReleaseRepoFor(_, _ string) string {
	return constants.SingboxCoreRepo
}

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

// DownloadCore downloads and installs sing-box.
// Per SPEC 046, the launcher pins constants.RequiredCoreVersion (the sing-box-lx
// fork tag) for every platform, including Windows 7 (windows/386).
//
// Callers always pass "" — the explicit-version path is kept only for tests
// and forced reinstall flows that target a specific tag.
func (ac *AppController) DownloadCore(ctx context.Context, version string, progressChan chan DownloadProgress) {
	defer close(progressChan)

	if version == "" {
		version = constants.RequiredCoreVersion
	}

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
	if err := os.MkdirAll(tempDir, platform.DefaultDirMode); err != nil {
		progressChan <- DownloadProgress{Progress: 0, Message: fmt.Sprintf("Failed to create temp dir: %v", err), Status: "error", Error: fmt.Errorf("DownloadCore: failed to create temp dir: %w", err)}
		return
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			debuglog.WarnLog("DownloadCore: failed to remove temp dir %s: %v", tempDir, err)
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
	if err := ac.installBinary(binaryPath, ac.FileService.SingboxBundledPath); err != nil {
		progressChan <- DownloadProgress{Progress: 0, Message: fmt.Sprintf("Installation failed: %v", err), Status: "error", Error: fmt.Errorf("DownloadCore: %w", err)}
		return
	}

	// 7. Done!
	progressChan <- DownloadProgress{Progress: 100, Message: fmt.Sprintf("sing-box v%s installed successfully!", version), Status: "done"}
}

// getReleaseInfo gets release information from the fork's GitHub releases.
func (ac *AppController) getReleaseInfo(ctx context.Context, version string) (*ReleaseInfo, error) {
	return ac.getReleaseInfoFromGitHub(ctx, version)
}

// getReleaseInfoFromGitHub gets release information from GitHub. `version`
// must be non-empty (DownloadCore guarantees this since SPEC 046 — there is
// no longer a /releases/latest path).
func (ac *AppController) getReleaseInfoFromGitHub(ctx context.Context, version string) (*ReleaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/v%s", coreReleaseRepo(), version)

	// Используем универсальный HTTP клиент
	client := CreateHTTPClient(NetworkRequestTimeout)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("getReleaseInfoFromGitHub: failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "singbox-launcher/1.0")

	resp, err := client.Do(req)
	defer func() {
		if resp != nil {
			debuglog.RunAndLog("getReleaseInfoFromGitHub: close response body", resp.Body.Close)
		}
	}()
	if err != nil {
		// Check error type
		if IsNetworkError(err) {
			return nil, fmt.Errorf("getReleaseInfoFromGitHub: network error: %s", GetNetworkErrorMessage(err))
		}
		return nil, fmt.Errorf("getReleaseInfoFromGitHub: request failed: %w", err)
	}

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

// SingboxAssetSuffix returns the asset filename suffix for current platform (e.g. "windows-amd64.zip").
// Used for UI hints when user downloads manually.
func SingboxAssetSuffix() string {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "windows-amd64.zip"
		}
		if runtime.GOARCH == "arm64" {
			return "windows-arm64.zip"
		}
		if runtime.GOARCH == "386" {
			return "windows-386-legacy-windows-7.zip"
		}
		return ""
	case "linux":
		if runtime.GOARCH == "amd64" {
			return "linux-amd64.tar.gz"
		}
		if runtime.GOARCH == "arm64" {
			return "linux-arm64.tar.gz"
		}
		if runtime.GOARCH == "arm" {
			return "linux-armv7.tar.gz"
		}
		return ""
	case "darwin":
		if runtime.GOARCH == "amd64" {
			return "darwin-amd64.tar.gz"
		}
		if runtime.GOARCH == "arm64" {
			return "darwin-arm64.tar.gz"
		}
		return ""
	default:
		return ""
	}
}

// findPlatformAsset finds the correct asset for current platform
func (ac *AppController) findPlatformAsset(assets []Asset) (*Asset, error) {
	platformPattern := SingboxAssetSuffix()
	if platformPattern == "" {
		return nil, fmt.Errorf("findPlatformAsset: unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	for i := range assets {
		if strings.Contains(assets[i].Name, platformPattern) {
			return &assets[i], nil
		}
	}

	return nil, fmt.Errorf("findPlatformAsset: asset not found for platform %s/%s", runtime.GOOS, runtime.GOARCH)
}

// downloadFile downloads a file with progress tracking (with a GitHub mirror fallback)
func (ac *AppController) downloadFile(ctx context.Context, url, destPath string, progressChan chan DownloadProgress) error {
	// Try to download from original URL
	err := ac.downloadFileFromURL(ctx, url, destPath, progressChan)
	if err == nil {
		return nil
	}

	debuglog.InfoLog("downloadFile: failed to download from original URL, trying mirrors...")

	// If that didn't work, try GitHub mirrors
	mirrors := []string{
		strings.Replace(url, "https://github.com/", "https://ghproxy.com/https://github.com/", 1),
	}

	for _, mirrorURL := range mirrors {
		debuglog.DebugLog("downloadFile: trying mirror: %s", mirrorURL)
		err := ac.downloadFileFromURL(ctx, mirrorURL, destPath, progressChan)
		if err == nil {
			return nil
		}
		debuglog.DebugLog("downloadFile: mirror failed: %v", err)
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
	defer func() {
		if resp != nil {
			debuglog.RunAndLog(fmt.Sprintf("downloadFileFromURL: close response body %s", url), resp.Body.Close)
		}
	}()
	if err != nil {
		// Check error type
		if IsNetworkError(err) {
			return fmt.Errorf("downloadFileFromURL: network error: %s", GetNetworkErrorMessage(err))
		}
		return fmt.Errorf("downloadFileFromURL: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloadFileFromURL: HTTP %d", resp.StatusCode)
	}

	// Hard upper bound on the downloaded archive. Legitimate sing-box
	// release archives are < 20 MB; capping at 100 MB protects us from a
	// compromised or misconfigured mirror feeding gigabytes onto disk and
	// filling the user's drive before failing.
	const maxDownloadSize = 100 * 1024 * 1024
	if resp.ContentLength > maxDownloadSize {
		return fmt.Errorf("downloadFileFromURL: advertised size %d bytes exceeds %d-byte cap", resp.ContentLength, maxDownloadSize)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("downloadFileFromURL: failed to create file: %w", err)
	}
	defer debuglog.RunAndLog(fmt.Sprintf("downloadFileFromURL: close file %s", destPath), file.Close)

	totalSize := resp.ContentLength
	var downloaded int64

	// Idle timeout: if no data received for 1 minute, abort (avoids hanging on stalled connections).
	const idleTimeout = 1 * time.Minute
	lastRead := time.Now()
	var lastReadMu sync.Mutex
	var stallAbort bool
	var stallMu sync.Mutex
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				lastReadMu.Lock()
				t := lastRead
				lastReadMu.Unlock()
				if time.Since(t) > idleTimeout {
					stallMu.Lock()
					stallAbort = true
					stallMu.Unlock()
					_ = resp.Body.Close()
					return
				}
			}
		}
	}()

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
			lastReadMu.Lock()
			lastRead = time.Now()
			lastReadMu.Unlock()
			written, writeErr := file.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("downloadFileFromURL: write failed: %w", writeErr)
			}
			downloaded += int64(written)
			// Runtime cap for servers that lie about Content-Length (chunked /
			// absent). Matches the pre-flight check above.
			if downloaded > maxDownloadSize {
				return fmt.Errorf("downloadFileFromURL: download exceeded %d-byte cap after %d bytes", maxDownloadSize, downloaded)
			}

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
			stallMu.Lock()
			aborted := stallAbort
			stallMu.Unlock()
			if aborted {
				return fmt.Errorf("downloadFileFromURL: no data received for 1 minute (connection stalled)")
			}
			return fmt.Errorf("downloadFileFromURL: read failed: %w", err)
		}
	}

	return nil
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
	defer debuglog.RunAndLog(fmt.Sprintf("extractZip: close zip reader %s", archivePath), r.Close)

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
				debuglog.RunAndLog(fmt.Sprintf("extractZip: close zip entry %s after create error", f.Name), rc.Close)
				return "", fmt.Errorf("extractZip: failed to create output file: %w", err)
			}

			_, err = io.Copy(outFile, rc)
			debuglog.RunAndLog(fmt.Sprintf("extractZip: close output file %s", binaryPath), outFile.Close)
			debuglog.RunAndLog(fmt.Sprintf("extractZip: close zip entry %s", f.Name), rc.Close)

			if err != nil {
				return "", fmt.Errorf("extractZip: failed to copy file: %w", err)
			}

			if err := platform.ChmodExecutable(binaryPath); err != nil {
				debuglog.WarnLog("extractZip: failed to chmod %s: %v", binaryPath, err)
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
	defer debuglog.RunAndLog(fmt.Sprintf("extractTarGz: close archive %s", archivePath), file.Close)

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("extractTarGz: failed to create gzip reader: %w", err)
	}
	defer debuglog.RunAndLog(fmt.Sprintf("extractTarGz: close gzip reader %s", archivePath), gzr.Close)

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
			debuglog.RunAndLog(fmt.Sprintf("extractTarGz: close output file %s", binaryPath), outFile.Close)

			if err != nil {
				return "", fmt.Errorf("extractTarGz: failed to copy file: %w", err)
			}

			if err := platform.ChmodExecutable(binaryPath); err != nil {
				debuglog.WarnLog("extractTarGz: failed to chmod %s: %v", binaryPath, err)
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
	if err := os.MkdirAll(binDir, platform.DefaultDirMode); err != nil {
		return fmt.Errorf("installBinary: failed to create bin directory: %w", err)
	}

	// If old binary exists, rename it
	if _, err := os.Stat(destPath); err == nil {
		oldPath := destPath + ".old"
		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			debuglog.WarnLog("installBinary: failed to remove old backup %s: %v", oldPath, err)
		}
		if err := os.Rename(destPath, oldPath); err != nil {
			debuglog.WarnLog("Warning: failed to rename old binary: %v", err)
		}
	}

	// Copy new binary
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("installBinary: failed to open source file: %w", err)
	}
	defer debuglog.RunAndLog(fmt.Sprintf("installBinary: close source file %s", sourcePath), sourceFile.Close)

	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("installBinary: failed to create destination file: %w", err)
	}
	defer debuglog.RunAndLog(fmt.Sprintf("installBinary: close destination file %s", destPath), destFile.Close)

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("installBinary: failed to copy file: %w", err)
	}

	if err := platform.ChmodExecutable(destPath); err != nil {
		debuglog.WarnLog("installBinary: failed to chmod %s: %v", destPath, err)
	}

	// Remove old backup
	oldPath := destPath + ".old"
	if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
		debuglog.WarnLog("installBinary: failed to remove backup %s: %v", oldPath, err)
	}

	debuglog.InfoLog("installBinary: binary installed successfully to %s", destPath)
	return nil
}
