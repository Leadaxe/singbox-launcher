// File migration_report.go — отчёт миграции v6 → v7 (SPEC 118 Т7).
//
// Все потери миграции собираются в ОДИН список предупреждений и показываются
// пользователю (диалог отчёта — презентер конфигуратора); молчаливых потерь
// нет.
//
// SPEC 118 W6 (хвост W2): отчёт живёт не только в памяти. Миграция —
// одноразовое событие, и происходит она у ПЕРВОГО, кто откроет состояние:
// на старте лаунчера это фоновая загрузка без единого окна. К моменту, когда
// пользователь откроет конфигуратор, файл уже переписан v7 — миграции больше
// нет, и показывать нечего. Поэтому отчёт кладётся рядом, в
// `bin/migration_report.txt`, и конфигуратор показывает его при первом
// открытии, после чего файл удаляет.
package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// MigrationReportFileName — имя файла отчёта в bin/.
//
// Один на приложение, а не на профиль: пользователь читает его как «что
// случилось при обновлении», и три файла на три удалённые машины были бы
// тремя диалогами об одном и том же событии. Отчёты профилей дописываются
// в конец — каждый со своим заголовком-путём.
const MigrationReportFileName = "migration_report.txt"

// MigrationReportPath — путь отчёта в каталоге bin.
func MigrationReportPath(binDir string) string {
	if binDir == "" {
		return ""
	}
	return filepath.Join(binDir, MigrationReportFileName)
}

// MigrationReport — итог одной миграции старой схемы в v7.
type MigrationReport struct {
	// FromVersion — версия схемы исходного файла (2–6).
	FromVersion int
	// Warnings — поимённые потери и переименования; порядок = порядок шагов.
	Warnings []string
	// BackupPath — путь бэкап-копии исходного файла (пусто, если состояние
	// пришло из байтов без пути — Parse).
	BackupPath string
}

// add фиксирует предупреждение и дублирует его в лог: диалог показывается
// один раз, а лог остаётся навсегда.
func (r *MigrationReport) add(format string, args ...interface{}) {
	if r == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	r.Warnings = append(r.Warnings, msg)
	debuglog.WarnLog("state migration v%d→v7: %s", r.FromVersion, msg)
}

// HasWarnings — есть что показать пользователю.
func (r *MigrationReport) HasWarnings() bool {
	return r != nil && len(r.Warnings) > 0
}

// Text — отчёт одного состояния в виде текста файла.
//
// Формат намеренно простой: заголовок с путём состояния и версией, строки
// предупреждений маркером, путь бэкап-копии. Это текст для человека, а не
// формат для разбора — читать его будет пользователь в диалоге или в
// блокноте, и вводить схему ради этого не за чем.
func (r *MigrationReport) Text(statePath string) string {
	if !r.HasWarnings() {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("=== %s (schema v%d -> v%d) ===\n", statePath, r.FromVersion, SchemaVersion))
	for _, w := range r.Warnings {
		b.WriteString("- ")
		b.WriteString(w)
		b.WriteString("\n")
	}
	if r.BackupPath != "" {
		b.WriteString("backup: ")
		b.WriteString(r.BackupPath)
		b.WriteString("\n")
	}
	return b.String()
}

// PersistMigrationReport дописывает отчёт в `bin/migration_report.txt`.
//
// Дописывает, а не перезаписывает: за один запуск лаунчер мигрирует локальное
// состояние и состояния удалённых машин, и каждое следующее затирало бы
// предыдущее — пользователь увидел бы отчёт только по последнему профилю.
//
// Ошибка записи не фатальна: отчёт уже в логе (add пишет каждую строку), а
// падать из-за невозможности сохранить предупреждение — хуже самого
// предупреждения.
func PersistMigrationReport(binDir string, rep *MigrationReport, statePath string) {
	if binDir == "" || !rep.HasWarnings() {
		return
	}
	text := rep.Text(statePath)
	if text == "" {
		return
	}
	path := MigrationReportPath(binDir)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		debuglog.WarnLog("state: migration report not persisted (%s): %v", path, err)
		return
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(text); err != nil {
		debuglog.WarnLog("state: migration report write (%s): %v", path, err)
	}
}

// ReadMigrationReport — сохранённый отчёт; пусто, если файла нет.
func ReadMigrationReport(binDir string) string {
	path := MigrationReportPath(binDir)
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// ClearMigrationReport удаляет файл отчёта — зовётся ПОСЛЕ показа.
//
// Показанный отчёт больше не новость: оставленный файл дал бы тот же диалог
// на каждом открытии конфигуратора, а строки его навсегда остались в логе.
func ClearMigrationReport(binDir string) {
	path := MigrationReportPath(binDir)
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		debuglog.WarnLog("state: migration report not cleared (%s): %v", path, err)
	}
}
