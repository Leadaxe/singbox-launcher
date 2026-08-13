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
//
// Локальная машина. Для удалённой — RuleSRSPathFor: у каждой машины свой
// каталог .srs (SPEC 098 §2.3).
func RuleSRSPath(execDir string, tag string) string {
	return filepath.Join(execDir, constants.BinDirName, constants.RuleSetsDirName, tag+".srs")
}

// RuleSRSPathFor возвращает путь .srs в каталоге конкретной машины
// (SPEC 098 §2.3):
//
//	local          → bin/rule-sets/<tag>.srs
//	remote + <id>  → bin/wizard_states/remote/<id>/srs/<tag>.srs
//
// ВАЖНО: это путь на МАШИНЕ ЛАУНЧЕРА. Он попадает в config.json как
// rule_set[].path, поэтому для remote-конфига остаётся открытым вопросом
// SPEC 097 §6 — на самой удалённой машине такого пути нет. Пока такие наборы
// у remote следует оставлять type:remote; здесь функция отвечает только за
// то, ГДЕ лаунчер держит скачанный файл.
func RuleSRSPathFor(execDir, target, machineID, tag string) string {
	return filepath.Join(platform.GetRuleSetsDirFor(execDir, target, machineID), tag+".srs")
}

// SRSFileExists проверяет наличие локального SRS файла
func SRSFileExists(execDir string, tag string) bool {
	path := RuleSRSPath(execDir, tag)
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// SRSFileExistsFor — SRSFileExists в границах конкретной машины.
func SRSFileExistsFor(execDir, target, machineID, tag string) bool {
	info, err := os.Stat(RuleSRSPathFor(execDir, target, machineID, tag))
	return err == nil && !info.IsDir()
}

// SRSDownloadTimeout — таймаут на скачивание одного SRS файла (60 сек по спецификации)
const SRSDownloadTimeout = 60 * time.Second

// CreateHTTPClientFunc allows core package to inject a shared HTTP client factory.
// If not set, DownloadSRS uses a local fallback client.
var CreateHTTPClientFunc func(timeout time.Duration) *http.Client

func createSRSHTTPClient(timeout time.Duration) *http.Client {
	if CreateHTTPClientFunc != nil {
		return CreateHTTPClientFunc(timeout)
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:       http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		},
	}
}

// DownloadSRS скачивает SRS файл по URL и сохраняет в destPath.
// При ctx.Done() прерывает загрузку; частичный файл удаляется.
func DownloadSRS(ctx context.Context, url string, destPath string) error {
	if url == "" || destPath == "" {
		return fmt.Errorf("DownloadSRS: url and destPath are required")
	}

	// Создаём контекст с таймаутом
	ctx, cancel := context.WithTimeout(ctx, SRSDownloadTimeout)
	defer cancel()

	client := createSRSHTTPClient(SRSDownloadTimeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("DownloadSRS: failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "LxBox/1.0")

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

// AllSRSDownloadedForEntries проверяет, что для всех переданных SRS-энтри существуют локальные файлы.
// Используется и для встроенных, и для пользовательских SRS-правил.
func AllSRSDownloadedForEntries(execDir string, entries []SRSEntry) bool {
	return AllSRSDownloadedIn(execDir, "", entries)
}

// AllSRSDownloadedIn — проверка наличия в каталоге КОНКРЕТНОЙ машины
// (SPEC 098). srsDir пуст = локальная машина, bin/rule-sets/.
//
// Разделение обязательно: каталоги у профилей разные, и проверка по общему
// показывала бы «скачано» для машины, у которой файла нет, — правило молча
// выпадало бы из её конфига при сборке.
func AllSRSDownloadedIn(execDir, srsDir string, entries []SRSEntry) bool {
	if execDir == "" || len(entries) == 0 {
		return true
	}
	for _, e := range entries {
		path := RuleSRSPath(execDir, e.Tag)
		if srsDir != "" {
			path = filepath.Join(srsDir, e.Tag+".srs")
		}
		if info, err := os.Stat(path); err != nil || info.IsDir() {
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
//
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
// destDir (SPEC 098) — каталог .srs той машины, для которой качаем. Пусто =
// локальная, файлы ложатся в bin/rule-sets/ как раньше.
//
// Разделение обязательно: у каждой машины свой профиль, и складывать её
// наборы в общий каталог значило бы, что окно ресурсов машины их не видит, а
// GC одной машины удаляет файлы другой.
func DownloadSRSGroupTo(ctx context.Context, execDir, destDir string, entries []SRSEntry) error {
	for _, e := range entries {
		destPath := RuleSRSPath(execDir, e.Tag)
		if destDir != "" {
			destPath = filepath.Join(destDir, e.Tag+".srs")
		}
		if err := DownloadSRS(ctx, e.URL, destPath); err != nil {
			return fmt.Errorf("DownloadSRSGroup tag=%q url=%s: %w", e.Tag, e.URL, err)
		}
	}
	return nil
}

// DownloadSRSGroup — загрузка для ЛОКАЛЬНОЙ машины (bin/rule-sets/).
func DownloadSRSGroup(ctx context.Context, execDir string, entries []SRSEntry) error {
	return DownloadSRSGroupTo(ctx, execDir, "", entries)
}

// DeleteOrphanRuleSets удаляет файлы из bin/rule-sets/, чьих tags нет в knownTags.
// Каноничная папка launcher-managed (как bin/subscriptions/) — третьим файлам
// тут не место, удаляем всё что не в множестве, без exception'ов на расширение.
//
// Возвращает список удалённых имён файлов (для логов/метрик).
//
// Используется как orphan GC после Rebuild: build pipeline проходит по всем
// enabled rules, собирает live tags, эта функция чистит остальное.
// Multi-stage safety: caller должен передать union tags из ВСЕХ
// bin/wizard_states/*.json (см. collectAllStageRuleSetTags), иначе rebuild
// активного state'а сметёт .srs нужные другому (неактивному) stage'у.
//
// Локальная машина. Для удалённой — DeleteOrphanRuleSetsFor.
func DeleteOrphanRuleSets(execDir string, knownTags []string) ([]string, error) {
	return DeleteOrphanRuleSetsFor(execDir, constants.ConfigTargetLocal, "", knownTags)
}

// DeleteOrphanRuleSetsFor чистит каталог .srs КОНКРЕТНОЙ машины
// (SPEC 098 §3.1.10).
//
// Границы машины — не оптимизация, а корректность: knownTags считается по
// состояниям этой же машины (collectAllStageRuleSetTags с тем же
// target/machineID). Смешать одно с другим — значит либо удалить чужой живой
// файл, либо вечно держать свой мёртвый.
func DeleteOrphanRuleSetsFor(execDir, target, machineID string, knownTags []string) ([]string, error) {
	knownSet := make(map[string]struct{}, len(knownTags))
	for _, tag := range knownTags {
		knownSet[tag] = struct{}{}
	}

	dir := platform.GetRuleSetsDirFor(execDir, target, machineID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("DeleteOrphanRuleSets: readdir %s: %w", dir, err)
	}

	deleted := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Каноничный паттерн bin/rule-sets/<tag>.srs. Tag = name без .srs.
		// Файлы без .srs (мусор / .tmp от прерванной DownloadSRS) тоже
		// чистим — папка целиком launcher-managed.
		tag := strings.TrimSuffix(name, ".srs")
		if _, ok := knownSet[tag]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err == nil {
			deleted = append(deleted, name)
		}
	}
	return deleted, nil
}
