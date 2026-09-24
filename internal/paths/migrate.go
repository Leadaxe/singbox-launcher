package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"singbox-launcher/internal/constants"
)

// MigrationResult — что произошло при старте.
type MigrationResult struct {
	Migrated     bool   // копирование выполнено
	Busy         bool   // другой экземпляр мигрирует прямо сейчас — пропущено
	Source       string // откуда (App/bin), пусто если источника нет
	Dest         string // куда (Data/bin); заполнено всегда
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
// Два экземпляра, стартовавшие одновременно, не мигрируют наперегонки:
// Data/.migrating.lock создаётся через O_EXCL; свежий чужой lock (моложе
// migrationLockTTL) — миграция в этом старте пропускается (Busy), старый —
// остаток упавшего процесса, сносится.
//
// log — куда писать одну итоговую строку (nil допустим).
func MigrateLegacyData(l Layout, log func(format string, args ...interface{})) (MigrationResult, error) {
	var res MigrationResult

	srcBin := l.App.Bin()
	dstBin := l.Data.Bin()
	res.Dest = dstBin
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
	release, busy, err := acquireMigrationLock(l.Data)
	if err != nil {
		return res, fmt.Errorf("migration: %w", err)
	}
	if busy {
		res.Busy = true
		if log != nil {
			log("migration: another instance is migrating, skipping")
		}
		return res, nil
	}
	defer release()
	// Пока ждали lock, другой экземпляр мог закончить.
	if exists(stateFile(dstBin)) {
		res.DataHadState = true
		return res, nil
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
		log("%s", res.Summary())
	}
	return res, nil
}

// Summary — итог миграции одной строкой для лога:
// «migration: <src> -> <dst>: files=… dirs=… bytes=… skipped=…[ examples: …]».
func (r MigrationResult) Summary() string {
	rep := r.Report
	line := fmt.Sprintf("migration: %s -> %s: files=%d dirs=%d bytes=%d skipped=%d",
		r.Source, r.Dest, rep.Files, rep.Dirs, rep.Bytes, rep.Skipped)
	if len(rep.SkippedExamples) > 0 {
		line += " examples: " + strings.Join(rep.SkippedExamples, "; ")
	}
	return line
}

// migrationLockName — lock-файл миграции в DataDir.
const migrationLockName = ".migrating.lock"

// migrationLockTTL — lock старше этого считается брошенным.
const migrationLockTTL = 10 * time.Minute

// acquireMigrationLock создаёт Data/.migrating.lock (O_EXCL). busy — свежий
// lock держит другой экземпляр; release удаляет свой lock.
func acquireMigrationLock(d DataDir) (release func(), busy bool, err error) {
	p := filepath.Join(string(d), migrationLockName)
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			return func() { _ = os.Remove(p) }, false, nil
		}
		if !os.IsExist(err) {
			return nil, false, fmt.Errorf("create %s: %w", p, err)
		}
		info, statErr := os.Stat(p)
		if statErr != nil {
			continue // lock исчез между попытками — ещё раз
		}
		if time.Since(info.ModTime()) < migrationLockTTL {
			return nil, true, nil
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return nil, false, fmt.Errorf("remove stale %s: %w", p, err)
		}
	}
	return nil, true, nil
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
