package paths

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"singbox-launcher/internal/constants"
)

// PurgeKind — раздел плана очистки (SPEC 135 §4.3).
type PurgeKind string

const (
	PurgeData     PurgeKind = "data"
	PurgeLogs     PurgeKind = "logs"
	PurgeLeftover PurgeKind = "leftover"
)

// Пояснения к лишнему (PurgeItem.Note). Английский без locale: текст идёт
// в stdout флага -purge-data и в диалог как подпись к пути.
const (
	PurgeNoteMovedAway       = "moved-away data"
	PurgeNoteUnusedSystem    = "unused system data folder"
	PurgeNoteOldLogs         = "old logs next to the program"
	PurgeNotePreMigration    = "pre-migration data in the bundle"
	PurgeNotePreMigrationApp = "pre-migration data next to the program"
)

// shippedBinNames — поставляемое внутри <App>/bin, которое очистка не
// трогает, даже когда удаляет сам <App>/bin (portable-данные, источник
// миграции): шаблон с маркером, локали, ядро со спутниками. Папку с
// программой пользователь удаляет сам (§4.3); в portable скачанное ядро от
// поставляемого не отличить, поэтому ядро остаётся тоже.
var shippedBinNames = []string{
	constants.WizardTemplateFileName,
	constants.WizardTemplateVersionFileName,
	"locale",
	constants.SingBoxExecName,
	constants.SingBoxExecName + ".exe",
	constants.WinTunDLLName,
	"libcronet.dll",
	"libcronet.dylib",
	"libcronet.so",
}

// PurgeItem — один удаляемый каталог.
type PurgeItem struct {
	Kind     PurgeKind
	Path     string
	Bytes    int64 // сумма размеров файлов (best effort)
	Files    int
	Note     string // для leftover: почему лишнее
	Selected bool   // по умолчанию true
}

// PurgePlan — что удалит очистка.
type PurgePlan struct {
	Items []PurgeItem
	// keep — пути, которые не удаляются, даже если лежат внутри удаляемого
	// каталога: поставляемое в <App>/bin, а если App вложен в удаляемый
	// каталог (экзотика с SINGBOX_LAUNCHER_DATA_DIR) — App целиком.
	keep []string
}

// PurgeReport — итог ExecutePurge.
type PurgeReport struct {
	Removed []string
	Failed  map[string]string // путь элемента → ошибка
}

// BuildPurgePlan собирает, что удалять (SPEC 135 §4.3):
//
//	data      DataDir целиком; если DataDir совпадает с App (portable,
//	          legacy) — только <App>/bin без поставляемого;
//	logs      LogDir (отдельным элементом, даже если лежит внутри DataDir);
//	leftover  <App>/bin.moved-*; системный DataDir при Mode portable/legacy;
//	          <App>/logs при Mode system/env; источник миграции из
//	          <Data>/.migrated_from.
//
// Не включает AppDir, portable.txt (решение Д) и поставляемые файлы.
// В план попадают только существующие каталоги. Все элементы отмечены.
func BuildPurgePlan(l Layout, exe string, env func(string) string, goos string, probe func(string) bool) PurgePlan {
	app := string(l.App)
	bundle := IsAppBundle(exe, goos)

	var p PurgePlan
	for _, name := range shippedBinNames {
		p.keep = append(p.keep, filepath.Join(l.App.Bin(), name))
	}

	add := func(kind PurgeKind, path, note string) {
		if path == "" || !isDir(path) {
			return
		}
		if app != "" && sameDir(path, app) {
			return // AppDir целиком не удаляется никогда
		}
		for _, it := range p.Items {
			if sameDir(it.Path, path) {
				return
			}
		}
		p.Items = append(p.Items, PurgeItem{Kind: kind, Path: path, Note: note, Selected: true})
	}

	if app != "" && sameDir(string(l.Data), app) {
		add(PurgeData, l.App.Bin(), "")
	} else {
		add(PurgeData, string(l.Data), "")
		if app != "" && isUnder(app, string(l.Data)) {
			p.keep = append(p.keep, app)
		}
	}
	add(PurgeLogs, string(l.Logs), "")

	if app != "" {
		if entries, err := os.ReadDir(app); err == nil {
			for _, e := range entries {
				if e.IsDir() && strings.HasPrefix(e.Name(), MovedBinPrefix) {
					add(PurgeLeftover, filepath.Join(app, e.Name()), PurgeNoteMovedAway)
				}
			}
		}
	}

	if l.Mode == ModePortable || l.Mode == ModeLegacy {
		// Остаток включения Portable (§4.2 п.4, решение Б): системный DataDir,
		// который не удалось стереть. Нет системного дефолта — нет и остатка.
		if sys, err := SystemDefault(exe, env, goos, probe); err == nil && !sameDir(string(sys.Data), string(l.Data)) {
			add(PurgeLeftover, string(sys.Data), PurgeNoteUnusedSystem)
		}
	}

	if (l.Mode == ModeSystem || l.Mode == ModeEnv) && app != "" {
		oldLogs := filepath.Join(app, constants.LogsDirName)
		if !sameDir(oldLogs, string(l.Logs)) {
			add(PurgeLeftover, oldLogs, PurgeNoteOldLogs)
		}
	}

	if src := migratedFrom(l.Data); src != "" && !sameDir(src, l.Data.Bin()) {
		note := PurgeNotePreMigrationApp
		if bundle {
			note = PurgeNotePreMigration
		}
		add(PurgeLeftover, src, note)
	}

	for i := range p.Items {
		p.Items[i].Files, p.Items[i].Bytes = measureTree(p.Items[i].Path, p.skipFor(i))
	}
	return p
}

// skipFor — что обходить внутри элемента i: чужие элементы плана (у них
// свой размер и своя очередь удаления) и keep.
func (p PurgePlan) skipFor(i int) []string {
	skip := append([]string(nil), p.keep...)
	for j, it := range p.Items {
		if j != i {
			skip = append(skip, it.Path)
		}
	}
	return skip
}

// ExecutePurge удаляет отмеченные элементы в порядке data → leftover → logs
// (логи последними: вызывающий закрывает свои лог-файлы до вызова). Ошибки
// собираются по элементам и не прерывают остальное. Вложенные элементы
// плана и поставляемое (keep) внутри удаляемого каталога обходятся; после
// всех удалений опустевшие каталоги отмеченных элементов подчищаются.
func ExecutePurge(p PurgePlan) PurgeReport {
	rep := PurgeReport{Failed: map[string]string{}}
	order := []PurgeKind{PurgeData, PurgeLeftover, PurgeLogs}
	errs := make(map[int][]error)
	var done []int
	for _, kind := range order {
		for i, it := range p.Items {
			if it.Kind != kind || !it.Selected {
				continue
			}
			errs[i] = removeTree(it.Path, p.skipFor(i))
			done = append(done, i)
		}
	}
	for _, i := range done {
		path := p.Items[i].Path
		pruneEmptyDirs(path, p.skipFor(i))
		if len(errs[i]) == 0 {
			rep.Removed = append(rep.Removed, path)
			continue
		}
		msg := errs[i][0].Error()
		if n := len(errs[i]); n > 1 {
			msg = fmt.Sprintf("%s (and %d more)", msg, n-1)
		}
		rep.Failed[path] = msg
	}
	return rep
}

// Text — план текстом для stdout и диалога: строка на элемент
// «[kind] path (N files, X MB) - note», снятые с отметки — с «(skipped)».
func (p PurgePlan) Text() string {
	var b strings.Builder
	for _, it := range p.Items {
		fmt.Fprintf(&b, "[%s] %s (%d files, %s)", it.Kind, it.Path, it.Files, FormatBytes(it.Bytes))
		if it.Note != "" {
			b.WriteString(" - " + it.Note)
		}
		if !it.Selected {
			b.WriteString(" (skipped)")
		}
		b.WriteString("\n")
	}
	if len(p.Items) == 0 {
		b.WriteString("Nothing to remove.\n")
	}
	return b.String()
}

// Text — итог очистки для stdout.
func (r PurgeReport) Text() string {
	var b strings.Builder
	for _, path := range r.Removed {
		b.WriteString("Removed: " + path + "\n")
	}
	failed := make([]string, 0, len(r.Failed))
	for path := range r.Failed {
		failed = append(failed, path)
	}
	sort.Strings(failed)
	for _, path := range failed {
		b.WriteString("Failed: " + path + ": " + r.Failed[path] + "\n")
	}
	return b.String()
}

// FormatBytes — размер для плана: «0 B», «12.3 KB», «4.5 MB», «1.2 GB».
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	v := float64(n) / unit
	for _, suffix := range []string{"KB", "MB", "GB"} {
		if v < unit || suffix == "GB" {
			return fmt.Sprintf("%.1f %s", v, suffix)
		}
		v /= unit
	}
	return fmt.Sprintf("%d B", n)
}

// migratedFrom — путь источника миграции из <Data>/.migrated_from; "" если
// маркера нет или он пуст.
func migratedFrom(d DataDir) string {
	b, err := os.ReadFile(filepath.Join(string(d), constants.MigratedFromMarkerFileName))
	if err != nil {
		return ""
	}
	src := strings.TrimSpace(string(b))
	if src == "" || !filepath.IsAbs(src) {
		return ""
	}
	return filepath.Clean(src)
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// isUnder — p лежит строго внутри root (по очищенным путям).
func isUnder(p, root string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// skipped — путь совпадает с одним из skip.
func skipped(p string, skip []string) bool {
	for _, s := range skip {
		if filepath.Clean(s) == filepath.Clean(p) {
			return true
		}
	}
	return false
}

// holdsSkipped — внутри p лежит что-то из skip.
func holdsSkipped(p string, skip []string) bool {
	for _, s := range skip {
		if isUnder(s, p) {
			return true
		}
	}
	return false
}

// measureTree — число и суммарный размер обычных файлов под root, без
// поддеревьев из skip. Симлинки не разворачиваются. Ошибки обхода
// пропускаются: размер ориентировочный.
func measureTree(root string, skip []string) (files int, bytes int64) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if p != root && skipped(p, skip) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				files++
				bytes += info.Size()
			}
		}
		return nil
	})
	return files, bytes
}

// removeTree удаляет root, обходя пути из skip внутри него: если внутри
// ничего из skip нет — os.RemoveAll целиком, иначе спуск по записям.
// Возвращает все ошибки удаления.
func removeTree(root string, skip []string) []error {
	if !holdsSkipped(root, skip) {
		if err := os.RemoveAll(root); err != nil {
			return []error{err}
		}
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []error{err}
	}
	var errs []error
	for _, e := range entries {
		child := filepath.Join(root, e.Name())
		if skipped(child, skip) {
			continue
		}
		if e.IsDir() {
			errs = append(errs, removeTree(child, skip)...)
			continue
		}
		if err := os.Remove(child); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	return errs
}

// pruneEmptyDirs удаляет опустевшие каталоги снизу вверх, включая сам root.
// Файлы и пути из skip не трогает; непустые каталоги остаются.
func pruneEmptyDirs(root string, skip []string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		child := filepath.Join(root, e.Name())
		if e.IsDir() && !skipped(child, skip) {
			pruneEmptyDirs(child, skip)
		}
	}
	_ = os.Remove(root) // удалится, только если пуст
}
