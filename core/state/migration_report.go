// File migration_report.go — отчёт миграции v6 → v7 (SPEC 118 Т7).
//
// Все потери миграции собираются в ОДИН список предупреждений и показываются
// пользователю (диалог отчёта — презентер конфигуратора); молчаливых потерь
// нет. Отчёт живёт только в памяти загруженного State — на диск не пишется:
// после сохранения v7 миграции больше не существует, и хранить отчёт негде
// и незачем.
package state

import (
	"fmt"

	"singbox-launcher/internal/debuglog"
)

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
