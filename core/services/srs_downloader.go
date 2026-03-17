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
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
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
	if err := os.MkdirAll(dir, platform.DefaultDirMode); err != nil {
		return fmt.Errorf("DownloadSRS: failed to create directory: %w", err)
	}

	tmpPath := destPath + ".tmp"
	destFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("DownloadSRS: failed to create file: %w", err)
	}

	// defer гарантирует закрытие файла и удаление tmp при любом выходе (включая панику)
	closed := false
	defer func() {
		if !closed {
			_ = destFile.Close()
		}
		if _, statErr := os.Stat(tmpPath); statErr == nil {
			_ = os.Remove(tmpPath)
		}
	}()

	written, err := io.Copy(destFile, resp.Body)
	if err != nil {
		return fmt.Errorf("DownloadSRS: write error: %w", err)
	}

	if err := destFile.Close(); err != nil {
		return fmt.Errorf("DownloadSRS: failed to close file: %w", err)
	}
	closed = true

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
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

// AllSRSDownloaded проверяет, что все remote SRS для правила (по текущим правилам отбора)
// скачаны локально. Используется для встроенных правил, основанных на шаблоне.
func AllSRSDownloaded(execDir string, ruleSets []json.RawMessage) bool {
	entries := GetSRSEntries(ruleSets)
	return AllSRSDownloadedForEntries(execDir, entries)
}

// AllSRSDownloadedForEntries проверяет, что для всех переданных SRS-энтри существуют локальные файлы.
// Используется и для встроенных, и для пользовательских SRS-правил.
func AllSRSDownloadedForEntries(execDir string, entries []SRSEntry) bool {
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

// GetSRSEntries извлекает все remote rule-set'ы (type == "remote") из ruleSets и возвращает
// их в виде списка (tag, URL) для дальнейшей работы (проверка наличия локальных файлов,
// скачивание и т.п.).
//
// Перед добавлением в результат URL проходят через normalizeSRSURL — там чинятся только
// GitHub blob-ссылки вида https://github.com/owner/repo/blob/branch/path/file.srs.
// Все остальные URL (включая локальные пути, нестандартные схемы и уже "сырые" ссылки)
// не трогаются и не отфильтровываются.
func GetSRSEntries(ruleSets []json.RawMessage) []SRSEntry {
	var result []SRSEntry
	for _, raw := range ruleSets {
		var item map[string]interface{}
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		typ, _ := item["type"].(string)
		rawURL, _ := item["url"].(string)
		tag, _ := item["tag"].(string)
		if typ != "remote" || tag == "" || rawURL == "" {
			continue
		}
		normalizedURL := normalizeSRSURL(rawURL)
		result = append(result, SRSEntry{Tag: tag, URL: normalizedURL})
	}
	return result
}

// normalizeSRSURL приводит URL SRS к удобному для скачивания виду.
//
// Единственная "умная" логика, заложенная сюда:
//   - если URL указывает на GitHub blob-страницу
//     (https://github.com/{owner}/{repo}/blob/{branch}/{path/to/file.srs}),
//     он конвертируется в raw-вариант:
//     https://raw.githubusercontent.com/{owner}/{repo}/{branch}/{path/to/file.srs}.
//
// Все остальные URL (включая:
//   - https://github.com/.../raw/...;
//   - https://raw.githubusercontent.com/...;
//   - любые другие https/http-хосты;
//   - локальные пути и нестандартные схемы, если они уже попали в конфиг)
// возвращаются без изменений. Это позволяет поддерживать как удалённые, так и локальные
// SRS-источники, не навязывая жёстких ограничений по схеме/хосту.
func normalizeSRSURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	host := strings.ToLower(parsed.Host)
	if host != "github.com" {
		return rawURL
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	// Ожидаемый формат blob-ссылки:
	//   /{owner}/{repo}/blob/{branch}/{path...}
	if len(parts) < 5 || parts[2] != "blob" {
		// Для github.com/.../raw/... и любых других вариантов ничего не меняем.
		return rawURL
	}

	owner := parts[0]
	repo := parts[1]
	branch := parts[3]
	filePath := strings.Join(parts[4:], "/")

	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, branch, filePath)
}

// DownloadSRSGroup скачивает по очереди все SRS из entries в bin/rule-sets/{tag}.srs.
// Возвращает первую ошибку; при отмене ctx возвращает ctx.Err().
func DownloadSRSGroup(ctx context.Context, execDir string, entries []SRSEntry) error {
	for _, e := range entries {
		destPath := RuleSRSPath(execDir, e.Tag)
		if err := DownloadSRS(ctx, e.URL, destPath); err != nil {
			return err
		}
	}
	return nil
}
