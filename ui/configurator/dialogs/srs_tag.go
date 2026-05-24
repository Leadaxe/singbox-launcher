package dialogs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

// srsTagFromURL — content-addressed tag: <filename-without-srs>-<hash8(URL)>.
//
// Один и тот же URL всегда даёт один tag (стабильность через runs), разные
// URL с одинаковым filename — разные tags (collision impossible). Hash8 —
// первые 8 hex от SHA-256 от полной URL (~32 бита, коллизия ~1/4млрд).
//
// Replace-policy:
//
//	https://example.com/path/blocklist.srs → "blocklist-a3f5c2d1"
//	https://other.com/path/blocklist.srs   → "blocklist-7e9b1f04"
//	повтор первой URL                       → "blocklist-a3f5c2d1" (дедуп)
//
// Filename без `.srs` пустой (URL без слешей или без расширения) → fallback
// "srs", чтоб tag не получился "-<hash>" с ведущим тире. Невалидный URL
// → пустая строка (caller должен скипнуть entry).
//
// Извлечён из ShowAddRuleDialog в отдельную функцию для unit-тестирования
// (SPEC 045 фаза 9).
// SRSTagFromURL — публичная обёртка для использования из других пакетов
// (preset_ref UI tile, build pipeline). См. srsTagFromURL для деталей.
func SRSTagFromURL(urlStr string) string {
	return srsTagFromURL(urlStr)
}

func srsTagFromURL(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}
	path := u.Path
	if path == "" {
		path = urlStr
	}
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	filename := strings.TrimSuffix(path, ".srs")
	if filename == "" {
		filename = "srs"
	}
	sum := sha256.Sum256([]byte(urlStr))
	hash8 := hex.EncodeToString(sum[:4])
	return fmt.Sprintf("%s-%s", filename, hash8)
}
