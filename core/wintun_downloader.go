package core

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// WinTunVersion is the version of wintun.dll to download
const WinTunVersion = "0.14.1"

// WinTunDownloadURL is the URL for downloading wintun.dll
const WinTunDownloadURL = "https://www.wintun.net/builds/wintun-%s.zip"

// CheckWintunDLL checks for the presence of wintun.dll
func (ac *AppController) CheckWintunDLL() (bool, error) {
	if runtime.GOOS != "windows" {
		return true, nil // wintun is not needed on non-Windows systems
	}

	if _, err := os.Stat(ac.FileService.WintunPath); os.IsNotExist(err) {
		return false, nil
	}
	return true, nil
}

// DownloadWintunDLL downloads and installs wintun.dll
func (ac *AppController) DownloadWintunDLL(ctx context.Context, progressChan chan DownloadProgress) {
	defer close(progressChan)

	if runtime.GOOS != "windows" {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  "wintun.dll is only needed on Windows",
			Status:   "error",
			Error:    fmt.Errorf("wintun.dll is only needed on Windows"),
		}
		return
	}

	// 1. Create temporary directory
	tempDir := filepath.Join(ac.FileService.ExecDir, "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Failed to create temp dir: %v", err),
			Status:   "error",
			Error:    err,
		}
		return
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			log.Printf("DownloadWintunDLL: failed to remove temp dir %s: %v", tempDir, err)
		}
	}()

	// 2. Download ZIP archive
	zipURL := fmt.Sprintf(WinTunDownloadURL, WinTunVersion)
	zipPath := filepath.Join(tempDir, fmt.Sprintf("wintun-%s.zip", WinTunVersion))

	progressChan <- DownloadProgress{Progress: 10, Message: "Downloading wintun.dll...", Status: "downloading"}
	if err := ac.downloadFileFromURL(ctx, zipURL, zipPath, progressChan); err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Download failed: %v", err),
			Status:   "error",
			Error:    err,
		}
		return
	}

	// 3. Extract ZIP and extract wintun.dll
	progressChan <- DownloadProgress{Progress: 80, Message: "Extracting wintun.dll...", Status: "extracting"}

	// Determine architecture
	var archDir string
	if runtime.GOARCH == "amd64" {
		archDir = "amd64"
	} else if runtime.GOARCH == "arm64" {
		archDir = "arm64"
	} else {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Unsupported architecture: %s", runtime.GOARCH),
			Status:   "error",
			Error:    fmt.Errorf("DownloadWintunDLL: unsupported architecture: %s", runtime.GOARCH),
		}
		return
	}

	// Open ZIP
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Failed to open zip: %v", err),
			Status:   "error",
			Error:    fmt.Errorf("DownloadWintunDLL: failed to open zip: %w", err),
		}
		return
	}
	defer debuglog.CloseWithLog("DownloadWintunDLL: zip reader", r)

	// Search for wintun.dll in the correct directory
	var dllPath string
	targetPath := fmt.Sprintf("bin/%s/wintun.dll", archDir)

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, targetPath) || strings.Contains(f.Name, fmt.Sprintf("/%s/wintun.dll", archDir)) {
			rc, err := f.Open()
			if err != nil {
				continue
			}

			// Extract the file
			dllPath = filepath.Join(tempDir, "wintun.dll")
			outFile, err := os.Create(dllPath)
			if err != nil {
				debuglog.CloseWithLog(fmt.Sprintf("DownloadWintunDLL: zip entry %s after create error", f.Name), rc)
				continue
			}

			_, err = io.Copy(outFile, rc)
			debuglog.CloseWithLog(fmt.Sprintf("DownloadWintunDLL: output file %s", dllPath), outFile)
			debuglog.CloseWithLog("DownloadWintunDLL: zip entry", rc)

			if err != nil {
				continue
			}

			break
		}
	}

	if dllPath == "" {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  "wintun.dll not found in archive",
			Status:   "error",
			Error:    fmt.Errorf("DownloadWintunDLL: wintun.dll not found in archive"),
		}
		return
	}

	// 4. Copy wintun.dll to target directory
	progressChan <- DownloadProgress{Progress: 90, Message: "Installing wintun.dll...", Status: "extracting"}

	// Create bin directory if it doesn't exist
	binDir := filepath.Dir(ac.FileService.WintunPath)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Failed to create bin directory: %v", err),
			Status:   "error",
			Error:    fmt.Errorf("DownloadWintunDLL: failed to create bin directory: %w", err),
		}
		return
	}

	// Copy the file
	sourceFile, err := os.Open(dllPath)
	if err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Failed to open source file: %v", err),
			Status:   "error",
			Error:    fmt.Errorf("DownloadWintunDLL: failed to open source file: %w", err),
		}
		return
	}
	defer debuglog.CloseWithLog(fmt.Sprintf("DownloadWintunDLL: source file %s", dllPath), sourceFile)

	destFile, err := os.Create(ac.FileService.WintunPath)
	if err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Failed to create destination file: %v", err),
			Status:   "error",
			Error:    fmt.Errorf("DownloadWintunDLL: failed to create destination file: %w", err),
		}
		return
	}
	defer debuglog.CloseWithLog(fmt.Sprintf("DownloadWintunDLL: destination file %s", ac.FileService.WintunPath), destFile)

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Failed to copy file: %v", err),
			Status:   "error",
			Error:    fmt.Errorf("DownloadWintunDLL: failed to copy file: %w", err),
		}
		return
	}

	// 5. Done!
	progressChan <- DownloadProgress{
		Progress: 100,
		Message:  fmt.Sprintf("wintun.dll v%s installed successfully!", WinTunVersion),
		Status:   "done",
	}
}
