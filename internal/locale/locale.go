// Package locale provides a simple i18n layer for the application.
// English translations are embedded (always available).
// Additional languages are loaded from external JSON files in bin/locale/.
package locale

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
)

//go:embed en.json
var enJSON []byte

const displayNameKey = "_display_name"

// RemoteLanguages lists language codes available for download from GitHub.
// Order is used for download; all matching bin/locale/*.json are loaded at startup.
var RemoteLanguages = []string{
	"ru", "de", "es", "fr", "it", "pt-BR", "zh", "ja", "ko", "tr",
}

var (
	mu       sync.RWMutex
	lang     = "en"
	catalogs map[string]map[string]string
)

func init() {
	catalogs = map[string]map[string]string{
		"en": mustParse(enJSON),
	}
}

func mustParse(data []byte) map[string]string {
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		panic(fmt.Sprintf("locale: failed to parse translations: %v", err))
	}
	return m
}

// T returns the translated string for the given key.
// Fallback order: current language → English → key itself.
func T(key string) string {
	mu.RLock()
	l := lang
	mu.RUnlock()

	if msgs, ok := catalogs[l]; ok {
		if val, ok := msgs[key]; ok {
			return val
		}
	}
	if l != "en" {
		if msgs, ok := catalogs["en"]; ok {
			if val, ok := msgs[key]; ok {
				return val
			}
		}
	}
	return key
}

// Tf returns a formatted translated string (fmt.Sprintf with the translated template).
func Tf(key string, args ...any) string {
	return fmt.Sprintf(T(key), args...)
}

// SetLang changes the current language. Ignored if the language is not available.
func SetLang(l string) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := catalogs[l]; ok {
		lang = l
	}
}

// GetLang returns the current language code.
func GetLang() string {
	mu.RLock()
	defer mu.RUnlock()
	return lang
}

// Languages returns sorted list of available (loaded) language codes.
func Languages() []string {
	mu.RLock()
	defer mu.RUnlock()
	codes := make([]string, 0, len(catalogs))
	for k := range catalogs {
		codes = append(codes, k)
	}
	sort.Strings(codes)
	return codes
}

// LangDisplayName returns the display name for a language code.
// Reads _display_name from the catalog; falls back to the code itself.
func LangDisplayName(code string) string {
	mu.RLock()
	defer mu.RUnlock()
	if msgs, ok := catalogs[code]; ok {
		if name, ok := msgs[displayNameKey]; ok {
			return name
		}
	}
	return code
}

// LangDisplayNames returns display names for all available languages, in the same order as Languages().
func LangDisplayNames() []string {
	codes := Languages()
	names := make([]string, len(codes))
	for i, c := range codes {
		names[i] = LangDisplayName(c)
	}
	return names
}

// LangCodeByDisplayName returns the language code for a display name (e.g. "English" → "en").
func LangCodeByDisplayName(name string) string {
	mu.RLock()
	defer mu.RUnlock()
	for code, msgs := range catalogs {
		if dn, ok := msgs[displayNameKey]; ok && dn == name {
			return code
		}
	}
	return ""
}

// LoadExternalLocales scans localeDir for *.json files and loads them as additional languages.
// Language code is derived from filename (e.g. "ru.json" → "ru").
// External files can override the embedded English catalog.
func LoadExternalLocales(localeDir string) {
	entries, err := os.ReadDir(localeDir)
	if err != nil {
		debuglog.DebugLog("locale: no external locale directory %s: %v", localeDir, err)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		code := strings.TrimSuffix(entry.Name(), ".json")
		data, err := os.ReadFile(filepath.Join(localeDir, entry.Name()))
		if err != nil {
			debuglog.WarnLog("locale: failed to read %s: %v", entry.Name(), err)
			continue
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			debuglog.WarnLog("locale: failed to parse %s: %v", entry.Name(), err)
			continue
		}
		catalogs[code] = m
		debuglog.InfoLog("locale: loaded external locale %q (%d keys)", code, len(m))
	}
}

// GetLocaleURL returns the GitHub raw URL for a given locale file.
func GetLocaleURL(langCode string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/Leadaxe/singbox-launcher/%s/bin/locale/%s.json",
		constants.GetMyBranch(), langCode)
}

// DownloadLocale downloads a single locale file from GitHub and saves it to localeDir.
func DownloadLocale(langCode, localeDir string) error {
	url := GetLocaleURL(langCode)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", langCode, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", langCode, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// Validate JSON before saving
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("invalid JSON for %s: %w", langCode, err)
	}

	if err := os.MkdirAll(localeDir, 0755); err != nil {
		return fmt.Errorf("create locale dir: %w", err)
	}

	path := filepath.Join(localeDir, langCode+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	// Load into catalogs immediately
	mu.Lock()
	catalogs[langCode] = m
	mu.Unlock()

	debuglog.InfoLog("locale: downloaded and loaded %q (%d keys)", langCode, len(m))
	return nil
}

// DownloadAllRemoteLocales downloads all known remote languages.
// Returns the number of successfully downloaded locales and the first error (if any).
func DownloadAllRemoteLocales(localeDir string) (int, error) {
	var firstErr error
	downloaded := 0
	for _, code := range RemoteLanguages {
		if err := DownloadLocale(code, localeDir); err != nil {
			debuglog.WarnLog("locale: failed to download %q: %v", code, err)
			if firstErr == nil {
				firstErr = err
			}
		} else {
			downloaded++
		}
	}
	return downloaded, firstErr
}

// GetLocaleDir returns the path to the locale directory under binDir.
func GetLocaleDir(binDir string) string {
	return filepath.Join(binDir, "locale")
}
