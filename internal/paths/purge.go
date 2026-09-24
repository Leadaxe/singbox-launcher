package paths

import (
	"encoding/json"
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
	PurgeNoteMovedAway    = "moved-away data"
	PurgeNoteUnusedSystem = "unused system data folder"
	// PurgeNoteSystemDataHasState — системный DataDir при Portable/Legacy, в
	// котором лежит state.json: это могут быть настоящие данные, скрытые
	// маркером (HiddenSystemData), поэтому по умолчанию не отмечен.
	PurgeNoteSystemDataHasState = "system data folder with settings - kept by default"
	PurgeNoteOldLogs            = "old logs next to the program"
	PurgeNotePreMigration       = "pre-migration data in the bundle"
	PurgeNotePreMigrationApp    = "pre-migration data next to the program"
	// PurgeNoteNeedsAdmin — пометка элемента с NeedsAdmin в тексте плана.
	PurgeNoteNeedsAdmin = "requires administrator rights"
)

// shippedBinNames — поставляемое внутри <App>/bin, которое очистка не
// трогает, даже когда удаляет сам <App>/bin (portable-данные, источник
// миграции): шаблон с маркером, локали, ядро со спутниками. Папку с
// программой пользователь удаляет сам (§4.3).
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

// alwaysShippedBinNames — поставляемое всегда, в любом архиве: локали.
// Когда <App>/bin — данные (portable, legacy), а маркера шаблона
// (wizard_template.version) рядом нет, остальное из shippedBinNames
// (шаблон, ядро, спутники) скачал сам лаунчер: маркер кладут только полные
// архивы и установщик. Тогда это данные и удаляются вместе с ними.
var alwaysShippedBinNames = []string{
	"locale",
}

// PurgeItem — один удаляемый каталог.
type PurgeItem struct {
	Kind      PurgeKind
	Path      string
	Bytes     int64 // сумма размеров файлов (best effort)
	FileCount int
	Note      string // для leftover: почему лишнее
	Selected  bool   // по умолчанию true (кроме системной папки с state.json)
	// Files — если непусто, удаляются только эти пути (файлы или каталоги)
	// внутри Path, а сам Path не трогается: режим Env, где DataDir и LogDir
	// выбрал пользователь и там может лежать чужое.
	Files []string
	// NeedsAdmin — остаток лежит под AppDir, а процесс не проходит пробу
	// записи AppDir (Program Files без прав, SPEC 139 §6 п. 6): элемент снят и
	// недоступен — это пропуск, а не неудачная попытка удаления.
	NeedsAdmin bool
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
//	          legacy) — только <App>/bin без поставляемого (без маркера
//	          шаблона поставляемыми считаются только локали, см.
//	          alwaysShippedBinNames);
//	logs      LogDir (отдельным элементом, даже если лежит внутри DataDir);
//	leftover  <App>/bin.moved-*; остаток переезда из settings.json
//	          (storage_leftover); системный DataDir при Mode portable/legacy;
//	          <App>/logs при Mode system/env; источник миграции из
//	          <Data>/.migrated_from.
//
// В режиме Env (DataDir/LogDir выбрал пользователь, там может лежать
// чужое) data — только <Data>/bin и <Data>/.migrated_from, logs — только
// файлы логов лаунчера (PurgeItem.Files), каталоги не удаляются.
//
// Не включает AppDir, portable.txt (решение Д) и поставляемые файлы.
// В план попадают только существующие пути. Все элементы отмечены, кроме
// системного DataDir с state.json при portable/legacy
// (PurgeNoteSystemDataHasState): это могут быть скрытые маркером данные.
func BuildPurgePlan(l Layout, exe string, env func(string) string, goos string, probe func(string) bool) PurgePlan {
	app := string(l.App)
	bundle := IsAppBundle(exe, goos)

	var p PurgePlan
	keepNames := shippedBinNames
	if app != "" && sameDir(string(l.Data), app) && !fileExists(filepath.Join(l.App.Bin(), constants.WizardTemplateVersionFileName)) {
		keepNames = alwaysShippedBinNames
	}
	for _, name := range keepNames {
		p.keep = append(p.keep, filepath.Join(l.App.Bin(), name))
	}

	// add — элемент плана; индекс или -1, если не добавлен.
	add := func(kind PurgeKind, path, note string) int {
		if path == "" || !isDir(path) {
			return -1
		}
		if app != "" && sameDir(path, app) {
			return -1 // AppDir целиком не удаляется никогда
		}
		for _, it := range p.Items {
			if sameDir(it.Path, path) {
				return -1
			}
		}
		p.Items = append(p.Items, PurgeItem{Kind: kind, Path: path, Note: note, Selected: true})
		return len(p.Items) - 1
	}
	// addFiles — элемент, удаляющий только существующие из names внутри dir;
	// ничего не существует — элемента нет (пустой Files значил бы «весь
	// каталог»).
	addFiles := func(kind PurgeKind, dir string, names []string) {
		var files []string
		for _, n := range names {
			if f := filepath.Join(dir, n); pathExists(f) {
				files = append(files, f)
			}
		}
		if len(files) == 0 {
			return
		}
		if i := add(kind, dir, ""); i >= 0 {
			p.Items[i].Files = files
		}
	}

	switch {
	case app != "" && sameDir(string(l.Data), app):
		add(PurgeData, l.App.Bin(), "")
	case l.Mode == ModeEnv:
		addFiles(PurgeData, string(l.Data), []string{constants.BinDirName, constants.MigratedFromMarkerFileName})
	default:
		add(PurgeData, string(l.Data), "")
	}
	if app != "" && !sameDir(string(l.Data), app) && isUnder(app, string(l.Data)) {
		p.keep = append(p.keep, app)
	}
	if l.Mode == ModeEnv {
		addFiles(PurgeLogs, string(l.Logs), launcherLogNames())
	} else {
		add(PurgeLogs, string(l.Logs), "")
	}

	// Остаток переезда, записанный переключателем Portable в settings.json.
	// Текущие данные не могут быть остатком: settings.json переезжает вместе
	// с данными, и старая запись могла указать на нынешнее место.
	leftover := storageLeftover(l.Data)
	if leftover != "" && (sameDir(leftover, string(l.Data)) || sameDir(leftover, l.Data.Bin()) ||
		isUnder(l.Data.Bin(), leftover) || isUnder(leftover, l.Data.Bin())) {
		leftover = ""
	}
	add(PurgeLeftover, leftover, PurgeNoteMovedAway)

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
			if i := add(PurgeLeftover, string(sys.Data), PurgeNoteUnusedSystem); i >= 0 {
				// state.json там — возможно, настоящие данные, скрытые
				// маркером (HiddenSystemData). Исключение — он внутри
				// записанного остатка переезда: это известная старая копия.
				st := stateFile(sys.Data.Bin())
				if exists(st) && (leftover == "" || !isUnder(st, leftover)) {
					p.Items[i].Selected = false
					p.Items[i].Note = PurgeNoteSystemDataHasState
				}
			}
		}
	}

	if (l.Mode == ModeSystem || l.Mode == ModeEnv) && app != "" {
		oldLogs := filepath.Join(app, constants.LogsDirName)
		if !sameDir(oldLogs, string(l.Logs)) {
			add(PurgeLeftover, oldLogs, PurgeNoteOldLogs)
		}
	}

	migrationIdx := -1
	if src := migratedFrom(l.Data); src != "" && !sameDir(src, l.Data.Bin()) {
		note := PurgeNotePreMigrationApp
		if bundle {
			note = PurgeNotePreMigration
		}
		migrationIdx = add(PurgeLeftover, src, note)
	}

	p.markNeedsAdmin(l, migrationIdx, probe)

	for i := range p.Items {
		it := &p.Items[i]
		if len(it.Files) == 0 {
			it.FileCount, it.Bytes = measureTree(it.Path, p.skipFor(i))
			continue
		}
		for _, f := range it.Files {
			n, b := measureTree(f, p.skipFor(i))
			it.FileCount += n
			it.Bytes += b
		}
	}
	return p
}

// markNeedsAdmin снимает остатки под AppDir, которые процесс удалить не
// может (SPEC 139 §6 п. 6): обычная проба записи AppDir, не предикат §7 —
// повышенный экземпляр в Program Files удалить может. Если снят источник
// миграции, маркер <Data>/.migrated_from сохраняется: по нему та же команда
// из консоли администратора найдёт остаток и доделает очистку.
func (p *PurgePlan) markNeedsAdmin(l Layout, migrationIdx int, probe func(string) bool) {
	app := string(l.App)
	if app == "" {
		return
	}
	var writable *bool
	for i := range p.Items {
		it := &p.Items[i]
		if it.Kind != PurgeLeftover || !isUnder(it.Path, app) {
			continue
		}
		if writable == nil {
			w := probe(app)
			writable = &w
		}
		if *writable {
			return
		}
		it.Selected = false
		it.NeedsAdmin = true
	}
	if migrationIdx < 0 || !p.Items[migrationIdx].NeedsAdmin {
		return
	}
	marker := filepath.Join(string(l.Data), constants.MigratedFromMarkerFileName)
	p.keep = append(p.keep, marker)
	// Элемент с Files (режим Env) удаляет файлы поштучно, мимо keep: маркер
	// убирается из списка. Пустой Files значил бы «весь каталог» — такой
	// элемент выпадает из плана.
	items := p.Items[:0]
	for _, it := range p.Items {
		if len(it.Files) > 0 {
			var files []string
			for _, f := range it.Files {
				if filepath.Clean(f) != marker {
					files = append(files, f)
				}
			}
			if len(files) == 0 {
				continue
			}
			it.Files = files
		}
		items = append(items, it)
	}
	p.Items = items
}

// launcherLogNames — имена файлов логов лаунчера в LogDir: четыре лога с
// ротированными .old, crash.log, native-stderr.log.
func launcherLogNames() []string {
	var names []string
	for _, n := range []string{constants.MainLogFileName, constants.ChildLogFileName, constants.ParserLogFileName, constants.APILogFileName} {
		names = append(names, n, n+".old")
	}
	return append(names, constants.CrashLogFileName, constants.NativeStderrLogFileName)
}

// settingsFileName — файл настроек в <Data>/bin (internal/locale.LoadSettings).
const settingsFileName = "settings.json"

// storageLeftover — поле storage_leftover из <Data>/bin/settings.json: что
// переключатель Portable не смог стереть на старом месте. "" — нет записи
// или путь не абсолютный. Пакет-лист locale не импортирует — читаем одно поле.
func storageLeftover(d DataDir) string {
	b, err := os.ReadFile(filepath.Join(d.Bin(), settingsFileName))
	if err != nil {
		return ""
	}
	var s struct {
		StorageLeftover string `json:"storage_leftover"`
	}
	if json.Unmarshal(b, &s) != nil || s.StorageLeftover == "" || !filepath.IsAbs(s.StorageLeftover) {
		return ""
	}
	return filepath.Clean(s.StorageLeftover)
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
			if len(it.Files) == 0 {
				errs[i] = removeTree(it.Path, p.skipFor(i))
			} else {
				for _, f := range it.Files {
					errs[i] = append(errs[i], removeTree(f, p.skipFor(i))...)
				}
			}
			done = append(done, i)
		}
	}
	for _, i := range done {
		path := p.Items[i].Path
		if files := p.Items[i].Files; len(files) > 0 {
			for _, f := range files {
				pruneEmptyDirs(f, p.skipFor(i)) // сам Path не трогается
			}
		} else {
			pruneEmptyDirs(path, p.skipFor(i))
		}
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
		fmt.Fprintf(&b, "[%s] %s (%d files, %s)", it.Kind, it.Path, it.FileCount, FormatBytes(it.Bytes))
		if len(it.Files) > 0 {
			b.WriteString(" only: " + strings.Join(it.FileNames(), ", "))
		}
		if it.Note != "" {
			b.WriteString(" - " + it.Note)
		}
		if it.NeedsAdmin {
			b.WriteString(" - " + PurgeNoteNeedsAdmin)
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

// FileNames — имена из Files относительно Path (для подписи «только это»).
func (it PurgeItem) FileNames() []string {
	names := make([]string, 0, len(it.Files))
	for _, f := range it.Files {
		if rel, err := filepath.Rel(it.Path, f); err == nil {
			names = append(names, filepath.ToSlash(rel))
		} else {
			names = append(names, f)
		}
	}
	return names
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

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
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
