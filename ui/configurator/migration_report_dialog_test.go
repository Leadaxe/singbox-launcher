// File migration_report_dialog_test.go — что именно показывается в отчёте
// миграции (SPEC 118 W6, хвост W2).
//
// Источников два и они не исключают друг друга: отчёт из памяти (миграция
// случилась прямо на этой загрузке) и отчёт с диска (миграция случилась
// раньше — фоновой загрузкой на старте или загрузкой профиля другой машины).
// Потерять любой значит потерять предупреждение — а молчаливых потерь
// миграция не допускает.
package configurator

import (
	"strings"
	"testing"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

func TestMigrationReportBodyMergesBothSources(t *testing.T) {
	binDir := t.TempDir()
	corestate.PersistMigrationReport(binDir, &corestate.MigrationReport{
		FromVersion: 6, Warnings: []string{"фоновая потеря на старте"}}, "/local/state.json")

	sf := &wizardmodels.WizardStateFile{Migration: &corestate.MigrationReport{
		FromVersion: 6,
		Warnings:    []string{"потеря этой загрузки"},
		BackupPath:  "/local/state.json.v6.bak",
	}}

	body := migrationReportBody(sf, binDir)
	if !strings.Contains(body, "потеря этой загрузки") {
		t.Error("отчёт из памяти потерян")
	}
	if !strings.Contains(body, "фоновая потеря на старте") {
		t.Error("отчёт с диска потерян — headless-миграция осталась немой")
	}
	if !strings.Contains(body, "/local/state.json.v6.bak") {
		t.Error("путь бэкап-копии потерян")
	}
}

// Показывать нечего — пусто: диалог «всё перенеслось» это сообщение ни о чём.
func TestMigrationReportBodyEmpty(t *testing.T) {
	binDir := t.TempDir()

	if body := migrationReportBody(nil, binDir); body != "" {
		t.Errorf("пустые источники дали тело: %q", body)
	}
	if body := migrationReportBody(&wizardmodels.WizardStateFile{}, binDir); body != "" {
		t.Errorf("состояние без миграции дало тело: %q", body)
	}
	// Отчёт без предупреждений — тоже нечего показывать.
	sf := &wizardmodels.WizardStateFile{Migration: &corestate.MigrationReport{FromVersion: 6}}
	if body := migrationReportBody(sf, binDir); body != "" {
		t.Errorf("отчёт без предупреждений дал тело: %q", body)
	}
}

// Каждый источник работает и в одиночку: на headless-старте память пуста, а
// при миграции прямо из мастера пуст диск.
func TestMigrationReportBodyEitherSourceAlone(t *testing.T) {
	t.Run("только диск", func(t *testing.T) {
		binDir := t.TempDir()
		corestate.PersistMigrationReport(binDir, &corestate.MigrationReport{
			FromVersion: 6, Warnings: []string{"только с диска"}}, "/s.json")
		if body := migrationReportBody(nil, binDir); !strings.Contains(body, "только с диска") {
			t.Errorf("отчёт с диска в одиночку не показан: %q", body)
		}
	})
	t.Run("только память", func(t *testing.T) {
		sf := &wizardmodels.WizardStateFile{Migration: &corestate.MigrationReport{
			FromVersion: 6, Warnings: []string{"только из памяти"}}}
		if body := migrationReportBody(sf, t.TempDir()); !strings.Contains(body, "только из памяти") {
			t.Errorf("отчёт из памяти в одиночку не показан: %q", body)
		}
	})
}
