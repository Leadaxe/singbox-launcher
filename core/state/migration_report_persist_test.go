// File migration_report_persist_test.go — отчёт миграции переживает
// дистанцию между миграцией и открытием конфигуратора (SPEC 118 W6, хвост W2).
//
// Мигрирует ПЕРВЫЙ, кто откроет состояние, а на старте лаунчера это фоновая
// загрузка без единого окна: к моменту, когда пользователь откроет
// конфигуратор, файл уже переписан v7, и миграции для него не существует.
// Отчёт в памяти State до этого момента не доживает — поэтому он ложится в
// `bin/migration_report.txt`.
package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationReportPersistedAndCleared(t *testing.T) {
	binDir := t.TempDir()

	rep := &MigrationReport{
		FromVersion: 6,
		Warnings: []string{
			"подписка «nl»: маска тегов упразднена, шаблон потерян",
			"источник 3: локальное Направление с фильтрами не переносится",
		},
		BackupPath: filepath.Join(binDir, "state.json.v6.bak"),
	}

	PersistMigrationReport(binDir, rep, "/bin/wizard_states/state.json")

	got := ReadMigrationReport(binDir)
	if got == "" {
		t.Fatal("отчёт не сохранён — на старте без окна он пропал бы вместе с миграцией")
	}
	for _, w := range rep.Warnings {
		if !strings.Contains(got, w) {
			t.Errorf("предупреждение потеряно при сохранении: %q", w)
		}
	}
	if !strings.Contains(got, "/bin/wizard_states/state.json") {
		t.Error("отчёт не называет, ЧЬЁ состояние мигрировало")
	}
	if !strings.Contains(got, rep.BackupPath) {
		t.Error("путь бэкап-копии потерян — страховка необратимого шага 8 не найдётся")
	}

	// Показали — файл больше не новость: иначе тот же диалог возвращался бы
	// на каждое открытие конфигуратора.
	ClearMigrationReport(binDir)
	if ReadMigrationReport(binDir) != "" {
		t.Error("отчёт пережил очистку — диалог вернётся на следующем открытии")
	}
	// Повторная очистка не ошибка: файла уже нет, и это нормальный исход.
	ClearMigrationReport(binDir)
}

// Отчёты нескольких профилей за один запуск ДОПИСЫВАЮТСЯ: лаунчер мигрирует
// локальное состояние и состояния удалённых машин, и перезапись показала бы
// только последнее.
func TestMigrationReportAppendsProfiles(t *testing.T) {
	binDir := t.TempDir()

	PersistMigrationReport(binDir, &MigrationReport{
		FromVersion: 6, Warnings: []string{"локальная потеря"}}, "/local/state.json")
	PersistMigrationReport(binDir, &MigrationReport{
		FromVersion: 5, Warnings: []string{"потеря машины"}}, "/remote/m1/state.json")

	got := ReadMigrationReport(binDir)
	if !strings.Contains(got, "локальная потеря") || !strings.Contains(got, "потеря машины") {
		t.Fatalf("второй профиль затёр первый:\n%s", got)
	}
	if !strings.Contains(got, "/local/state.json") || !strings.Contains(got, "/remote/m1/state.json") {
		t.Errorf("профили не различимы в отчёте:\n%s", got)
	}
}

// Отчёт без потерь файла не создаёт: пустой диалог «всё перенеслось» —
// сообщение ни о чём.
func TestMigrationReportSilentWithoutWarnings(t *testing.T) {
	binDir := t.TempDir()

	PersistMigrationReport(binDir, nil, "/local/state.json")
	PersistMigrationReport(binDir, &MigrationReport{FromVersion: 6}, "/local/state.json")

	if _, err := os.Stat(MigrationReportPath(binDir)); !os.IsNotExist(err) {
		t.Error("файл отчёта создан без единого предупреждения")
	}
	if ReadMigrationReport(binDir) != "" {
		t.Error("пустой отчёт прочитан как непустой")
	}
}

// Без каталога bin писать некуда — и это не ошибка (Parse из байтов путей не
// знает вовсе).
func TestMigrationReportWithoutBinDir(t *testing.T) {
	PersistMigrationReport("", &MigrationReport{FromVersion: 6, Warnings: []string{"x"}}, "/s.json")
	if ReadMigrationReport("") != "" {
		t.Error("отчёт прочитан из пустого каталога")
	}
	if MigrationReportPath("") != "" {
		t.Error("путь отчёта построен без каталога")
	}
}
