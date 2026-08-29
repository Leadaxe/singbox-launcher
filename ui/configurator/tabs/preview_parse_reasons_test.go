// File preview_parse_reasons_test.go — вкладка Preview обязана объяснять
// пустоту (SPEC 115), и объясняет она её ИЗ СОСТОЯНИЯ (SPEC 118 Т3/Т8).
//
// Раньше вкладка разбирала тело своим кодом — второй реализацией разбора
// рядом с боевой; SPEC 118 снёс раздельный разбор целиком: тело разбирается
// один раз, при fetch, и per-record деградации остаются в
// `updateStatus.warnings`. Проверяем, что этот канал доносит причины до
// формы — иначе вкладка снова стала бы тупиком «0 server(s)» без повода.
package tabs

import (
	"strings"
	"testing"

	corestate "singbox-launcher/core/state"
)

func TestFetchWarningTextsCarryReasons(t *testing.T) {
	st := &corestate.SubUpdateStatus{Warnings: []corestate.FetchWarning{
		{Kind: "bad_record", Tag: "🇳🇱 Нидерланды",
			Message: "vless outbound rejected: empty user id — the server returned a placeholder, subscription may be expired"},
		{Kind: "skip", Count: 12, Message: "records filtered by skip"},
		// Вид без текста: остаться без строки он не вправе — тогда потеря
		// снова стала бы молчаливой.
		{Kind: "lost_group_member"},
	}}

	got := fetchWarningTexts(st)
	if len(got) != 3 {
		t.Fatalf("строк = %d, ожидали 3 — каждая деградация обязана быть названа: %v", len(got), got)
	}
	joined := strings.Join(got, " | ")
	if !strings.Contains(joined, "empty user id") {
		t.Errorf("настоящая причина отбраковки потеряна: %q", joined)
	}
	if !strings.Contains(joined, "🇳🇱 Нидерланды") {
		t.Errorf("адресация узлом потеряна — непонятно, ЧТО отбраковано: %q", joined)
	}
	if !strings.Contains(got[2], "lost_group_member") {
		t.Errorf("деградация без текста осталась без строки: %q", got[2])
	}
}

// Пустая диагностика причин не даёт, и блок причин не строится: пустая рамка
// над списком серверов — шум.
func TestFetchWarningTextsEmpty(t *testing.T) {
	if got := fetchWarningTexts(nil); len(got) != 0 {
		t.Fatalf("nil-диагностика дала причины: %v", got)
	}
	if got := fetchWarningTexts(&corestate.SubUpdateStatus{}); len(got) != 0 {
		t.Fatalf("пустая диагностика дала причины: %v", got)
	}
	if block := previewParseReasonsBlock(nil); block != nil {
		t.Error("блок причин построен без причин")
	}
}
