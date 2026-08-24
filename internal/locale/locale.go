// Package locale provides a simple i18n layer for the application.
//
// SPEC 111: the English text at the call site IS the translation key
// (natural keys). English is the base language and lives in the code;
// translations come from external JSON catalogs in bin/locale/. A miss at
// any level (no catalog, no key, no form) degrades into the key itself —
// correct English — with the arguments substituted.
package locale

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
)

const displayNameKey = "_display_name"

// RemoteLanguages lists language codes available for download from GitHub.
// Order is used for download; all matching bin/locale/*.json are loaded at startup.
// Пока поддерживаются только языки, которые ведутся вручную: английский
// (живёт в коде — ключ и есть текст) и русский. Восемь машинных переводов
// сняты — каждая новая
// строка требовала правки во всех десяти файлах, а качество их всё равно
// никто не проверял. Вернуть язык = добавить код сюда и файл в релиз.
var RemoteLanguages = []string{
	"ru",
}

var (
	mu       sync.RWMutex
	lang     = "en"
	catalogs map[string]map[string]Entry
)

// CreateHTTPClientFunc allows injecting a shared HTTP client factory from core.
// If not set, locale downloads use a local default client with timeout.
var CreateHTTPClientFunc func(timeout time.Duration) *http.Client

func init() {
	// Английский — базовый язык: он живёт в коде (ключ = текст), каталог не
	// нужен. Псевдозапись держит "en" в списке языков и даёт имя селектору.
	catalogs = map[string]map[string]Entry{
		"en": {displayNameKey: {Value: Value{Text: "English"}}},
	}
}

// pick returns the rendering value for an entry: form 0 is the root value,
// form N>0 is special["N"] falling back to the root when absent.
func pick(e Entry, form int) Value {
	if form > 0 {
		if sp, ok := e.Special[strconv.Itoa(form)]; ok && !sp.Value.IsZero() {
			return sp.Value
		}
	}
	return e.Value
}

// textFor resolves key in language l as a plain string; ok is false when
// the language has no usable text for the key (missing entry, or a plural
// entry used where a plain string is expected — the checker's domain).
func textFor(l, key string, form int) (string, bool) {
	msgs, ok := catalogs[l]
	if !ok {
		return "", false
	}
	e, ok := msgs[key]
	if !ok {
		return "", false
	}
	v := pick(e, form)
	if v.Text == "" {
		return "", false
	}
	return v.Text, true
}

// pluralFor resolves key in language l as a plural template for count n.
// A plain-string value is accepted gracefully (a language may translate a
// plural key with a single form).
func pluralFor(l, key string, form, n int) (string, bool) {
	msgs, ok := catalogs[l]
	if !ok {
		return "", false
	}
	e, ok := msgs[key]
	if !ok {
		return "", false
	}
	v := pick(e, form)
	if len(v.Forms) > 0 {
		r := resolverFor(l)
		if t, ok := v.Forms[r.Resolve(n)]; ok && t != "" {
			return t, true
		}
		if t, ok := v.Forms["other"]; ok && t != "" {
			return t, true
		}
		return "", false
	}
	if v.Text != "" {
		return v.Text, true
	}
	return "", false
}

// T returns the translated string for the given key.
// Fallback order: current language → English catalog → the key itself.
func T(key string) string {
	return TN(0, key)
}

// TN returns the translation of key using special form N (0 = the root
// value). Used at call sites where one English text must be translated
// differently depending on context.
func TN(form int, key string) string {
	mu.RLock()
	defer mu.RUnlock()
	for _, l := range langChainLocked() {
		if t, ok := textFor(l, key, form); ok {
			return t
		}
	}
	return key
}

// Tf returns a formatted translated string (fmt.Sprintf with the translated template).
func Tf(key string, args ...any) string {
	return fmt.Sprintf(T(key), args...)
}

// TfN is Tf with a special form index (see TN).
func TfN(form int, key string, args ...any) string {
	return fmt.Sprintf(TN(form, key), args...)
}

// Plural returns the plural-aware translation of key for count n; n is
// always the first substituted argument, extra follow it.
// The set of required forms is dictated by the language's PluralResolver.
func Plural(key string, n int, extra ...any) string {
	return PluralN(0, key, n, extra...)
}

// PluralN is Plural with a special form index (see TN).
func PluralN(form int, key string, n int, extra ...any) string {
	args := append([]any{n}, extra...)
	mu.RLock()
	defer mu.RUnlock()
	for _, l := range langChainLocked() {
		if t, ok := pluralFor(l, key, form, n); ok {
			return fmt.Sprintf(t, args...)
		}
	}
	return fmt.Sprintf(key, args...)
}

// langChainLocked is langChain for callers already holding mu.
func langChainLocked() []string {
	if lang == "en" {
		return []string{"en"}
	}
	return []string{lang, "en"}
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
		if e, ok := msgs[displayNameKey]; ok && e.Value.Text != "" {
			return e.Value.Text
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
		if e, ok := msgs[displayNameKey]; ok && e.Value.Text == name {
			return code
		}
	}
	return ""
}

// LoadExternalLocales scans localeDir for *.json files and loads them as additional languages.
// Language code is derived from filename (e.g. "ru.json" → "ru").
// A file named en.json would shadow the builtin English pseudo-catalog.
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
		m, skipped, err := parseCatalog(data)
		if err != nil {
			debuglog.WarnLog("locale: failed to parse %s: %v", entry.Name(), err)
			continue
		}
		if len(skipped) > 0 {
			debuglog.WarnLog("locale: %s has %d malformed entries (skipped): %v",
				entry.Name(), len(skipped), skipped)
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

	client := createLocaleHTTPClient(30 * time.Second)
	resp, err := client.Do(req)
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

	// Validate before saving: a top-level parse error or a file with no
	// usable entries must never replace a working catalog on disk.
	m, skipped, err := parseCatalog(data)
	if err != nil {
		return fmt.Errorf("invalid JSON for %s: %w", langCode, err)
	}
	if len(m) == 0 {
		return fmt.Errorf("catalog %s has no usable entries", langCode)
	}
	if len(skipped) > 0 {
		debuglog.WarnLog("locale: downloaded %s has %d malformed entries (skipped): %v",
			langCode, len(skipped), skipped)
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

func createLocaleHTTPClient(timeout time.Duration) *http.Client {
	if CreateHTTPClientFunc != nil {
		return CreateHTTPClientFunc(timeout)
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
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
