package tabs

import (
	"strconv"
	"strings"
	"testing"

	"singbox-launcher/core/backup"
)

// makeWarns — n однотипных предупреждений с различимыми деталями.
func makeWarns(n int) []backup.Warning {
	out := make([]backup.Warning, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, backup.Warning{
			Code:   backup.WarnBackupUnknownOutbound,
			Detail: "tag-" + strconv.Itoa(i),
		})
	}
	return out
}

// Отчёт после импорта не режет список: ради полного перечня потерь он и
// существует (обрезка на 12 строках прятала ровно то, что нужно прочитать).
func TestImportReportTextKeepsEveryWarning(t *testing.T) {
	warns := makeWarns(40)
	text := importReportText(&backup.ImportResult{AppliedSources: 2, AppliedRules: 7}, warns)

	for _, w := range warns {
		if !strings.Contains(text, w.Detail) {
			t.Fatalf("в отчёте нет предупреждения %q", w.Detail)
		}
	}
	if strings.Contains(text, "… +") {
		t.Error("отчёт обрезан, а не должен")
	}
	if got := strings.Count(text, "\n• "); got != len(warns) {
		t.Errorf("строк-пунктов %d, ожидалось %d", got, len(warns))
	}
}

// Шапка отчёта есть всегда, даже когда потерь нет.
func TestImportReportTextWithoutWarnings(t *testing.T) {
	text := importReportText(&backup.ImportResult{AppliedSources: 1, AppliedRules: 0}, nil)
	if !strings.Contains(text, "1") {
		t.Errorf("в шапке нет счётчика источников: %q", text)
	}
	if strings.Contains(text, "•") {
		t.Errorf("пунктов быть не должно: %q", text)
	}
}

// Превью ДО импорта, наоборот, режет — но отсылает за полным списком в отчёт,
// иначе «… +N» оставляет пользователя без продолжения.
func TestWarnLinesTruncatesAndPointsToReport(t *testing.T) {
	warns := makeWarns(backupSummaryWarnLimit + 5)
	text := warnLines(warns, backupSummaryWarnLimit)

	if got := strings.Count(text, "\n• "); got != backupSummaryWarnLimit {
		t.Errorf("показано %d пунктов, ожидалось %d", got, backupSummaryWarnLimit)
	}
	if !strings.Contains(text, "… +5") {
		t.Errorf("нет отметки об обрезке: %q", text)
	}
	if !strings.Contains(text, settingsBackupReportMoreText) {
		t.Errorf("нет отсылки к отчёту: %q", text)
	}
}

// Без обрезки лимит 0: тот же вызов обслуживает оба места.
func TestWarnLinesNoLimit(t *testing.T) {
	warns := makeWarns(30)
	text := warnLines(warns, 0)
	if strings.Contains(text, "… +") {
		t.Error("при limit=0 обрезки быть не должно")
	}
	if got := strings.Count(text, "\n• "); got != len(warns) {
		t.Errorf("строк-пунктов %d, ожидалось %d", got, len(warns))
	}
}

// Ниже лимита хвоста нет: «… +0» и отсылка в отчёт при полном списке — шум.
func TestWarnLinesUnderLimitHasNoTail(t *testing.T) {
	text := warnLines(makeWarns(3), backupSummaryWarnLimit)
	if strings.Contains(text, "… +") || strings.Contains(text, settingsBackupReportMoreText) {
		t.Errorf("лишний хвост при коротком списке: %q", text)
	}
}
