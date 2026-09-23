package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"singbox-launcher/internal/constants"
)

// MigrationResult — что произошло при старте.
type MigrationResult struct {
	Migrated     bool   // копирование выполнено
	Source       string // откуда (App/bin), пусто если источника нет
	DataHadState bool   // в Data уже был state.json — мигрировать нечего
	Report       CopyReport
}

// MigrateLegacyData переносит унаследованные данные рядом с бинарём в DataDir
// (SPEC 135 §3.4).
//
// Условие: l.Mode ∈ {ModeSystem, ModeEnv}; App != Data; в
// Data/bin/wizard_states/state.json нет; в App/bin/wizard_states/state.json
// есть. Иначе — возврат без действий (Source и DataHadState заполнены), без
// ошибки.
//
// Порядок: MkdirAll(Data) → CopyTree(App/bin, Data/bin) → маркер
// Data/.migrated_from последним шагом. Обрыв до продвижения копии оставляет
// Data без state.json, и следующий старт повторяет миграцию. Логи не
// копируются: они вне bin/. Источник не трогается.
//
// log — куда писать одну итоговую строку (nil допустим).
func MigrateLegacyData(l Layout, log func(format string, args ...interface{})) (MigrationResult, error) {
	var res MigrationResult

	srcBin := l.App.Bin()
	dstBin := l.Data.Bin()
	if exists(stateFile(dstBin)) {
		res.DataHadState = true
	}
	if exists(stateFile(srcBin)) {
		res.Source = srcBin
	}

	if l.Mode != ModeSystem && l.Mode != ModeEnv {
		return res, nil
	}
	if res.DataHadState || res.Source == "" || sameDir(string(l.App), string(l.Data)) {
		return res, nil
	}

	if err := os.MkdirAll(string(l.Data), 0o755); err != nil {
		return res, fmt.Errorf("migration: create %s: %w", l.Data, err)
	}
	rep, err := CopyTree(srcBin, dstBin)
	res.Report = rep
	if err != nil {
		return res, fmt.Errorf("migration: %w", err)
	}

	marker := filepath.Join(string(l.Data), constants.MigratedFromMarkerFileName)
	if err := os.WriteFile(marker, []byte(res.Source+"\n"), 0o644); err != nil {
		return res, fmt.Errorf("migration: write %s: %w", marker, err)
	}
	res.Migrated = true

	if log != nil {
		line := fmt.Sprintf("migration: %s -> %s: files=%d dirs=%d bytes=%d skipped=%d",
			srcBin, dstBin, rep.Files, rep.Dirs, rep.Bytes, rep.Skipped)
		if len(rep.SkippedExamples) > 0 {
			line += " examples: " + strings.Join(rep.SkippedExamples, "; ")
		}
		log("%s", line)
	}
	return res, nil
}

func stateFile(bin string) string {
	return filepath.Join(bin, constants.WizardStatesDirName, constants.WizardStateFileName)
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// sameDir — один и тот же каталог: по очищенному пути либо, если оба
// существуют, по идентичности файла (симлинки, регистр на macOS/Windows).
func sameDir(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ai, bi)
}
