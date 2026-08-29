// File migration_report_dialog.go — показ отчёта миграции состояния v6 → v7
// (SPEC 118 Т7): все потери миграции — в одном списке, показанном
// пользователю ОДИН раз; молчаливых потерь нет. Лог получает каждую строку
// независимо от показа (state/migration_report.go пишет при сборе).
package configurator

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// shownMigrationReports — отпечатки уже показанных отчётов (процесс живёт
// дольше, чем миграция: до первого Save файл остаётся старой схемы, и каждое
// открытие конфигуратора мигрирует заново — второй раз тем же списком
// пользователя не дёргаем).
var (
	shownMigrationReportsMu sync.Mutex
	shownMigrationReports   = map[string]bool{}
)

// maybeShowMigrationReport показывает отчёт миграции загруженного состояния,
// если он есть и ещё не показывался этим процессом.
func maybeShowMigrationReport(win fyne.Window, stateFile *wizardmodels.WizardStateFile) {
	if win == nil || stateFile == nil || !stateFile.Migration.HasWarnings() {
		return
	}
	rep := stateFile.Migration

	sum := sha256.Sum256([]byte(strings.Join(rep.Warnings, "\n")))
	key := hex.EncodeToString(sum[:])
	shownMigrationReportsMu.Lock()
	already := shownMigrationReports[key]
	shownMigrationReports[key] = true
	shownMigrationReportsMu.Unlock()
	if already {
		return
	}

	var b strings.Builder
	b.WriteString(locale.T("The saved configuration was migrated to the new storage format. Some settings could not be carried over unchanged:"))
	b.WriteString("\n")
	for _, w := range rep.Warnings {
		b.WriteString("\n• ")
		b.WriteString(w)
	}
	if rep.BackupPath != "" {
		b.WriteString("\n\n")
		b.WriteString(locale.T("A backup of the original file was saved next to it:"))
		b.WriteString(" ")
		b.WriteString(rep.BackupPath)
	}

	// Ловушка min-width Fyne: длинные строки без Wrapping раздувают диалог
	// на весь экран — поэтому label с переносом внутри скролла с явным
	// минимальным размером.
	label := widget.NewLabel(b.String())
	label.Wrapping = fyne.TextWrapWord
	scroll := container.NewScroll(label)
	scroll.SetMinSize(fyne.NewSize(560, 320))

	dialogs.ShowCustom(win, locale.T("Configuration migration report"), locale.T("OK"), scroll)
}
