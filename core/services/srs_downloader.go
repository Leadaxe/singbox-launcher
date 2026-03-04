// Package services содержит сервисы приложения.
//
// srs_downloader.go — скачивание rule-set (SRS) файлов по HTTP.
// Файлы сохраняются в bin/rule-sets/{tag}.srs для локального использования sing-box.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
)

// RuleSRSPath возвращает путь к локальному SRS файлу: {ExecDir}/bin/rule-sets/{tag}.srs
func RuleSRSPath(execDir string, tag string) string {
	return filepath.Join(execDir, constants.BinDirName, constants.RuleSetsDirName, tag+".srs")
}

// SRSFileExists проверяет наличие локального SRS файла
func SRSFileExists(execDir string, tag string) bool {
	path := RuleSRSPath(execDir, tag)
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// SRSDownloadTimeout — таймаут на скачивание одного SRS файла (60 сек по спецификации)
const SRSDownloadTimeout = 60 * time.Second

// DownloadSRS скачивает SRS файл по URL и сохраняет в destPath.
// При ctx.Done() прерывает загрузку; частичный файл удаляется.
func DownloadSRS(ctx context.Context, url string, destPath string) error {
	if url == "" || destPath == "" {
		return fmt.Errorf("DownloadSRS: url and destPath are required")
	}

	// Создаём контекст с таймаутом
	ctx, cancel := context.WithTimeout(ctx, SRSDownloadTimeout)
	defer cancel()

	client := &http.Client{
		Timeout: SRSDownloadTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("DownloadSRS: failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "singbox-launcher/1.0")

	resp, err := client.Do(req)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return fmt.Errorf("connection timeout")
		}
		return fmt.Errorf("DownloadSRS: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("DownloadSRS: HTTP %d", resp.StatusCode)
	}

	// Пишем во временный файл, затем переименовываем атомарно
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("DownloadSRS: failed to create directory: %w", err)
	}

	tmpPath := destPath + ".tmp"
	destFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("DownloadSRS: failed to create file: %w", err)
	}

	written, err := io.Copy(destFile, resp.Body)
	if err != nil {
		_ = destFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("DownloadSRS: write error: %w", err)
	}

	if err := destFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("DownloadSRS: failed to close file: %w", err)
	}

	if ctx.Err() != nil {
		_ = os.Remove(tmpPath)
		return ctx.Err()
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("DownloadSRS: failed to save file: %w", err)
	}

	debuglog.DebugLog("DownloadSRS: downloaded %d bytes to %s", written, destPath)
	return nil
}

// SRSEntry — один rule_set, требующий загрузки (tag + url)
type SRSEntry struct {
	Tag string
	URL string
}

// AllSRSDownloaded проверяет, что все remote SRS для правила скачаны локально.
func AllSRSDownloaded(execDir string, ruleSets []json.RawMessage) bool {
	entries := GetRemoteSRSEntries(ruleSets)
	if execDir == "" || len(entries) == 0 {
		return true
	}
	for _, e := range entries {
		if !SRSFileExists(execDir, e.Tag) {
			return false
		}
	}
	return true
}

// GetRemoteSRSEntries извлекает из RuleSets записи с type=remote и url содержащим raw.githubusercontent.com
func GetRemoteSRSEntries(ruleSets []json.RawMessage) []SRSEntry {
	var result []SRSEntry
	for _, raw := range ruleSets {
		var item map[string]interface{}
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		typ, _ := item["type"].(string)
		url, _ := item["url"].(string)
		tag, _ := item["tag"].(string)
		if typ != "remote" || tag == "" || url == "" {
			continue
		}
		if !strings.Contains(url, "raw.githubusercontent.com") {
			continue
		}
		result = append(result, SRSEntry{Tag: tag, URL: url})
	}
	return result
}
