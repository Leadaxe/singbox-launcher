package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"singbox-launcher/internal/constants"
)

// MovedBinPrefix — префикс остатка AppDir/bin, который не удалось стереть
// при выключении Portable (SPEC 135 §4.2): AppDir/bin.moved-<ГГГГММДД-ЧЧММСС>.
// Очистка (§4.3) находит такие каталоги по этому префиксу.
const MovedBinPrefix = constants.BinDirName + ".moved-"

// movedStampLayout — формат метки времени в имени остатка.
const movedStampLayout = "20060102-150405"

// portableMarkerContent — содержимое portable.txt. Resolve его не читает
// (§3.2), строка только поясняет файл тому, кто откроет его руками.
const portableMarkerContent = "portable\n"

// ErrEnvLayout — раскладку задают переменные окружения, переключать нечего.
var ErrEnvLayout = errors.New("paths are set by environment variables")

// ErrTargetHasData — при выключении Portable в системном DataDir уже есть
// state.json, а в AppDir/bin его нет: переезжать нечему, а копия поверх
// смешала бы поставляемое с настоящими данными. Вызывающий вместо переезда
// удаляет portable.txt (RemovePortableMarker) и перезапускается — это
// нормальный исход, не ошибка для пользователя.
var ErrTargetHasData = errors.New("the data folder already holds settings; remove portable.txt instead of moving")

// SwitchReport — итог переключения Portable.
type SwitchReport struct {
	From, To string     // каталоги bin: откуда и куда переехали данные
	Copy     CopyReport // итог копирования
	// Leftover — что не удалось удалить на старом месте (для выключения —
	// AppDir/bin.moved-<метка> или AppDir/bin, для включения — DataDir/bin);
	// "" если старое место стёрто начисто. Это не ошибка: остаток попадает
	// в очистку §4.3.
	Leftover string
}

// Summary — итог одной строкой для лога:
// «portable switch: <from> -> <to>: files=… dirs=… bytes=… skipped=… leftover=…[ examples: …]».
func (r SwitchReport) Summary() string {
	c := r.Copy
	leftover := r.Leftover
	if leftover == "" {
		leftover = "none"
	}
	line := fmt.Sprintf("portable switch: %s -> %s: files=%d dirs=%d bytes=%d skipped=%d leftover=%s",
		r.From, r.To, c.Files, c.Dirs, c.Bytes, c.Skipped, leftover)
	if len(c.SkippedExamples) > 0 {
		line += " examples: " + strings.Join(c.SkippedExamples, "; ")
	}
	return line
}

// SystemDefault — раскладка по правилу 4 (§3.1) без учёта переменных
// окружения, маркера и унаследованных данных: куда уедут данные при
// выключении Portable. Ошибка, если у платформы нет системного дефолта
// (голый бинарь macOS, Windows без LOCALAPPDATA и USERPROFILE) — выключать
// Portable там некуда.
func SystemDefault(exe string, env func(string) string, goos string, probe func(string) bool) (Layout, error) {
	app := AppDir(filepath.Dir(exe))
	l, err := platformDefault(app, IsAppBundle(exe, goos), env, goos, probe)
	if err != nil {
		return Layout{}, err
	}
	if l.Mode != ModeSystem {
		return Layout{}, fmt.Errorf("no system data directory on this platform")
	}
	return l, nil
}

// SwitchToPortable — включение Portable (SPEC 135 §4.2 «Включить»):
//  1. CopyTree(DataDir/bin → AppDir/bin): поставляемое в AppDir/bin
//     сливается, скачанное перекрывает его, как в цепочке §3.3;
//  2. AppDir/portable.txt — только после удачного копирования;
//  3. старое стирается (решение Б): DataDir/bin и DataDir/.migrated_from.
//     DataDir/logs не трогается — логи держит текущий процесс. Что не
//     удалилось — Leftover, не ошибка: правило 2 сильнее правила 4.
//
// Сбой на шагах 1–2 оставляет обе раскладки рабочими: копировщик стирает
// свой временный каталог, маркер без полной копии не пишется. Если
// копировщик пропустил state.json — ошибка до записи маркера
// (ErrStateNotCopied, AppDir/bin не тронут); если пропустил
// что-то другое — маркер пишется, но источник не стирается (Leftover = From):
// пропущенное осталось только там.
func SwitchToPortable(l Layout) (SwitchReport, error) {
	var rep SwitchReport
	switch l.Mode {
	case ModeEnv:
		return rep, ErrEnvLayout
	case ModePortable, ModeLegacy:
		return rep, fmt.Errorf("portable mode is already on")
	case ModeSystem:
	default:
		return rep, fmt.Errorf("unknown layout mode %q", l.Mode)
	}
	if sameDir(string(l.App), string(l.Data)) {
		return rep, fmt.Errorf("data directory is the program directory already")
	}

	rep.From = l.Data.Bin()
	rep.To = l.App.Bin()
	if exists(rep.From) {
		c, err := CopyTree(rep.From, rep.To)
		rep.Copy = c
		if err != nil {
			return rep, err
		}
	} else if err := os.MkdirAll(rep.To, 0o755); err != nil {
		// Данных ещё нет (свежая установка): переезжать нечему, но
		// AppDir/bin должен быть, как у любой portable-раскладки.
		return rep, fmt.Errorf("create %s: %w", rep.To, err)
	}

	marker := filepath.Join(string(l.App), constants.PortableMarkerFileName)
	if err := os.WriteFile(marker, []byte(portableMarkerContent), 0o644); err != nil {
		return rep, fmt.Errorf("write %s: %w", marker, err)
	}

	if rep.Copy.Skipped > 0 {
		rep.Leftover = rep.From
		return rep, nil
	}
	if err := os.RemoveAll(rep.From); err != nil || exists(rep.From) {
		rep.Leftover = rep.From
	}
	_ = os.Remove(filepath.Join(string(l.Data), constants.MigratedFromMarkerFileName))
	return rep, nil
}

// SwitchToSystem — выключение Portable (SPEC 135 §4.2 «Выключить»); target —
// SystemDefault(...):
//  1. CopyTree(AppDir/bin → target.Data/bin);
//  2. удаляется AppDir/portable.txt (если был);
//  3. AppDir/bin уходит целиком (решение Б), иначе правило 3 вернёт Legacy:
//     сначала переименование в AppDir/bin.moved-<метка> и стирание его, что
//     не стёрлось — Leftover. Если переименовать нельзя (Windows держит
//     открытый файл), первым удаляется state.json — единственное, что
//     включает правило 3, — затем остальное; остаток — Leftover.
//
// AppDir/logs не трогается: логи держит текущий процесс.
//
// В target.Data уже есть state.json, а в AppDir/bin нет — ErrTargetHasData
// без каких-либо изменений (переезжать нечему, см. HiddenSystemData).
// Копировщик пропустил state.json — ErrStateNotCopied до удаления маркера,
// обе раскладки целы. Пропустил что-то другое — источник не стирается, а только
// переименовывается в bin.moved-<метка> (Leftover): пропущенное осталось
// только там, а правило 3 гасить всё равно нужно.
//
// Порядок выбран так, что любой сбой оставляет рабочую раскладку: до
// удаления маркера данные на старом месте целы (Portable как был), после —
// state.json либо цел (Legacy с полными данными), либо уже удалён вместе с
// переездом (System). Ошибка возвращается, только когда state.json в
// AppDir/bin удалить не удалось: тогда следующий старт снова Legacy.
func SwitchToSystem(l Layout, target Layout) (SwitchReport, error) {
	var rep SwitchReport
	switch l.Mode {
	case ModeEnv:
		return rep, ErrEnvLayout
	case ModeSystem:
		return rep, fmt.Errorf("portable mode is already off")
	case ModePortable, ModeLegacy:
	default:
		return rep, fmt.Errorf("unknown layout mode %q", l.Mode)
	}
	if target.Mode != ModeSystem || target.Data == "" {
		return rep, fmt.Errorf("target layout is not a system layout")
	}
	if sameDir(string(l.App), string(target.Data)) {
		return rep, fmt.Errorf("system data directory is the program directory")
	}

	rep.From = l.App.Bin()
	rep.To = target.Data.Bin()
	if exists(stateFile(rep.To)) && !exists(stateFile(rep.From)) {
		return rep, ErrTargetHasData
	}
	if err := os.MkdirAll(string(target.Data), 0o755); err != nil {
		return rep, fmt.Errorf("create %s: %w", target.Data, err)
	}
	if exists(rep.From) {
		c, err := CopyTree(rep.From, rep.To)
		rep.Copy = c
		if err != nil {
			return rep, err
		}
	} else if err := os.MkdirAll(rep.To, 0o755); err != nil {
		return rep, fmt.Errorf("create %s: %w", rep.To, err)
	}

	marker := filepath.Join(string(l.App), constants.PortableMarkerFileName)
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return rep, fmt.Errorf("remove %s: %w", marker, err)
	}

	if !exists(rep.From) {
		return rep, nil
	}
	moved := movedBinPath(l.App)
	if err := os.Rename(rep.From, moved); err == nil {
		if rep.Copy.Skipped > 0 {
			rep.Leftover = moved // пропущенное живёт только здесь — не стирать
			return rep, nil
		}
		if err := os.RemoveAll(moved); err != nil || exists(moved) {
			rep.Leftover = moved
		}
		return rep, nil
	}

	// Переименовать нельзя: гасим правило 3 точечно, остальное — как выйдет.
	state := stateFile(rep.From)
	if err := os.Remove(state); err != nil && !os.IsNotExist(err) {
		return rep, fmt.Errorf("remove %s: %w", state, err)
	}
	if rep.Copy.Skipped > 0 {
		rep.Leftover = rep.From
		return rep, nil
	}
	if err := os.RemoveAll(rep.From); err != nil || exists(rep.From) {
		rep.Leftover = rep.From
	}
	return rep, nil
}

// movedBinPath — свободное имя AppDir/bin.moved-<метка>[-N]: остаток
// прошлого переезда в ту же секунду не должен сорвать переименование.
func movedBinPath(app AppDir) string {
	base := filepath.Join(string(app), MovedBinPrefix+time.Now().Format(movedStampLayout))
	p := base
	for i := 2; pathExists(p) && i < 100; i++ {
		p = fmt.Sprintf("%s-%d", base, i)
	}
	return p
}

// RemovePortableMarker удаляет AppDir/portable.txt (отсутствие — не ошибка):
// выход из Portable без переезда, когда данные уже лежат в системном DataDir
// (ErrTargetHasData, HiddenSystemData). Раскладку применит перезапуск.
func RemovePortableMarker(app AppDir) error {
	marker := filepath.Join(string(app), constants.PortableMarkerFileName)
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", marker, err)
	}
	return nil
}

// HiddenSystemData — данные в системном DataDir, скрытые маркером: режим
// Portable/Legacy, в AppDir/bin нет state.json, а в SystemDefault(...).Data/bin
// он есть. Типичный путь: Portable выключили (данные уехали в системный
// каталог), затем распаковали новый zip с portable.txt поверх папки
// программы. Возвращает системный DataDir.
func HiddenSystemData(l Layout, exe string, env func(string) string, goos string, probe func(string) bool) (dataDir string, found bool) {
	if l.Mode != ModePortable && l.Mode != ModeLegacy {
		return "", false
	}
	if exists(stateFile(l.App.Bin())) {
		return "", false
	}
	sys, err := SystemDefault(exe, env, goos, probe)
	if err != nil || sameDir(string(sys.Data), string(l.App)) {
		return "", false
	}
	if !exists(stateFile(sys.Data.Bin())) {
		return "", false
	}
	return string(sys.Data), true
}
