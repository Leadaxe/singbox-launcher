// File migration_report_dialog.go — показ отчёта миграции состояния v6 → v7
// (SPEC 118 Т7): все потери миграции — в одном списке, показанном
// пользователю ОДИН раз; молчаливых потерь нет. Лог получает каждую строку
// независимо от показа (state/migration_report.go пишет при сборе).
//
// SPEC 118 W6 (хвост W2): показывать только отчёт, приехавший в памяти
// загруженного состояния, было недостаточно. Мигрирует ПЕРВЫЙ, кто откроет
// состояние, а на старте лаунчера это фоновая загрузка без единого окна:
// к тому моменту, когда пользователь откроет конфигуратор, файл уже
// переписан в v7 и миграции для него не существует. Поэтому Load кладёт
// отчёт в `bin/migration_report.txt`, а этот файл показывает при первом
// открытии конфигуратора и удаляет — иначе тот же диалог возвращался бы
// на каждое открытие.
package configurator

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	corestate "singbox-launcher/core/state"
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

// maybeShowMigrationReport показывает отчёт миграции, если он есть и ещё не
// показывался этим процессом.
//
// Источников два, и они не исключают друг друга: отчёт из памяти (миграция
// случилась прямо на этой загрузке) и отчёт с диска (миграция случилась
// раньше — фоновой загрузкой на старте лаунчера или загрузкой профиля другой
// машины). Оба идут в один диалог: пользователю это одно событие —
// «состояние переехало на новый формат».
func maybeShowMigrationReport(win fyne.Window, stateFile *wizardmodels.WizardStateFile, binDir string) {
	if win == nil {
		return
	}

	body := migrationReportBody(stateFile, binDir)
	if body == "" {
		return
	}

	sum := sha256.Sum256([]byte(body))
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
	b.WriteString("\n\n")
	b.WriteString(body)

	// Ловушка min-width Fyne: длинные строки без Wrapping раздувают диалог
	// на весь экран — поэтому label с переносом внутри скролла с явным
	// минимальным размером.
	label := widget.NewLabel(b.String())
	label.Wrapping = fyne.TextWrapWord
	scroll := container.NewScroll(label)
	scroll.SetMinSize(fyne.NewSize(560, 320))

	dialogs.ShowCustom(win, locale.T("Configuration migration report"), locale.T("OK"), scroll)

	// Показали — файл больше не новость. Строки уже в логе навсегда.
	corestate.ClearMigrationReport(binDir)
}

// migrationReportBody — текст отчёта из обоих источников; пусто, если
// показывать нечего.
//
// Вынесена ради проверяемости: составление текста — вся содержательная часть
// решения «что показать», а диалог поверх неё требует Fyne-окна.
func migrationReportBody(stateFile *wizardmodels.WizardStateFile, binDir string) string {
	var parts []string
	if stateFile != nil && stateFile.Migration.HasWarnings() {
		rep := stateFile.Migration
		var b strings.Builder
		for _, w := range rep.Warnings {
			b.WriteString("• ")
			b.WriteString(w)
			b.WriteString("\n")
		}
		if rep.BackupPath != "" {
			b.WriteString(locale.T("A backup of the original file was saved next to it:"))
			b.WriteString(" ")
			b.WriteString(rep.BackupPath)
			b.WriteString("\n")
		}
		parts = append(parts, strings.TrimRight(b.String(), "\n"))
	}
	if saved := corestate.ReadMigrationReport(binDir); saved != "" {
		parts = append(parts, saved)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
