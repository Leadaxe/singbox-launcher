//go:build darwin

package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/lxdclient"
	"singbox-launcher/internal/platform"
)

// Классификатор состояния launchd-службы демона (SPEC 136).
//
// Ядро lx.11+ на `lxd --service=install` копирует себя в root-owned каталог
// службы и переписывает plist на копию (SPEC 100 форка). Лаунчер ничего не
// копирует: он только читает plist, проверяет цепочку владения копии и
// сверяет sha256 копии с ядром лаунчера и с тем, что отвечает работающий
// демон. Всё читается без root: plist и сайдкар 0644, копия 0755.
//
// Раскладка (решение владельца 24.09.2026, ядро lx.12): копия — плоский
// файл /Library/PrivilegedHelperTools/sing-box-lxd, сайдкар —
// <копия>.install.json; каталога службы нет. Цепочка владения: /Library →
// PrivilegedHelperTools → файл. Ранние сборки lx.11 клали копию в
// /Library/PrivilegedHelperTools/<label>/sing-box или в плоский
// /Library/PrivilegedHelperTools/<label> — это legacy: убрать `sudo rm -rf`.

const (
	// daemonServiceChainRoot — верх цепочки владения копии: от него вниз до
	// файла каждое звено обязано быть root-owned без g/o-записи.
	daemonServiceChainRoot = "/Library"
	// daemonServiceHelperToolsDir — каталог привилегированных помощников
	// macOS (root:wheel 1755); копия лежит в нём плоским файлом
	// platform.PrivilegedCopyName (`sing-box-lxd` — так же зовётся процесс).
	daemonServiceHelperToolsDir = "/Library/PrivilegedHelperTools"
	// daemonServiceLegacyCopyPath — раскладка ранних сборок lx.11: каталог
	// <label>/ с sing-box внутри или плоский файл <label>. Не используется;
	// лаунчер советует её удалить.
	daemonServiceLegacyCopyPath = daemonServiceHelperToolsDir + "/" + daemonLaunchdLabel
	// daemonServiceSidecarSuffix — сайдкар установки рядом с копией,
	// <копия>.install.json (root:wheel 0644):
	// {source, sha256, version, installed_at, plist_path, label}.
	daemonServiceSidecarSuffix = ".install.json"
	// daemonHashCacheCap — потолок кэша sha256: файлов в игре два-три, потолок
	// лишь не даёт кэшу расти от череды заменённых ядер.
	daemonHashCacheCap = 16
	// launchctlTool / launchctlTimeout — чтение состояния службы у launchd
	// (`launchctl print system/<label>`, без sudo); exit 113 — службы в
	// домене нет (не загружена).
	launchctlTool           = "/bin/launchctl"
	launchctlTimeout        = 2 * time.Second
	launchctlNotFoundStatus = 113
	// launchdRunningState — `state = running` в выводе launchctl print.
	launchdRunningState = "running"
	// launchdNotLoaded — LaunchdState, когда launchd службу не знает.
	launchdNotLoaded = "not loaded"
)

// daemonServiceCorePath — каноническая root-owned копия ядра службы.
func daemonServiceCorePath() string {
	return filepath.Join(daemonServiceHelperToolsDir, platform.PrivilegedCopyName)
}

// isLegacyCopyPath — path — копия ранней раскладки lx.11 (legacy — сам
// файл или что-то внутри каталога <label>/).
func isLegacyCopyPath(path, legacy string) bool {
	if legacy == "" {
		return false
	}
	path = filepath.Clean(path)
	legacy = filepath.Clean(legacy)
	return path == legacy || strings.HasPrefix(path, legacy+string(filepath.Separator))
}

// legacyCopyRemoveCommand — команда удаления остатков ранней раскладки:
// сам путь (файл или каталог) и сайдкар плоского варианта.
func legacyCopyRemoveCommand(legacy string) string {
	return "sudo rm -rf " + shellQuote(legacy) + " " + shellQuote(daemonServiceSidecarPath(legacy))
}

// daemonServiceSidecarPath — сайдкар установки копии corePath.
func daemonServiceSidecarPath(corePath string) string {
	return corePath + daemonServiceSidecarSuffix
}

// DaemonServiceState — вердикт классификатора службы (SPEC 136 §4).
type DaemonServiceState string

const (
	// DaemonServiceNotInstalled — plist службы нет.
	DaemonServiceNotInstalled DaemonServiceState = "not_installed"
	// DaemonServiceUnsafe — root запускает файл, который может подменить
	// пользователь: plist не на каноническую копию, либо цепочка владения
	// копии нарушена, либо plist не разобрался.
	DaemonServiceUnsafe DaemonServiceState = "unsafe"
	// DaemonServiceStale — копия безопасна, но это не ядро лаунчера (sha
	// разные) или её нет вовсе.
	DaemonServiceStale DaemonServiceState = "stale"
	// DaemonServiceNotRunning — на диске всё в порядке (plist на безопасную
	// копию, sha совпал), но launchd службу не держит: не загружена или
	// state ≠ running. Лечится загрузкой plist (bootstrap), не
	// переустановкой. Пара к вердикту NOT RUNNING (exit 5) `lxd
	// --service=status` ядра.
	DaemonServiceNotRunning DaemonServiceState = "not_running"
	// DaemonServiceProcessStale — файл совпал, но работающий демон запущен из
	// другого образа (не перезапущен после обновления копии).
	DaemonServiceProcessStale DaemonServiceState = "process_stale"
	// DaemonServiceOK — служба запускает актуальную root-owned копию.
	DaemonServiceOK DaemonServiceState = "ok"
)

// DaemonServiceCheck — вердикт и его основания. Detail — английская причина
// для лога и Debug API; UI собирает свой текст из полей.
type DaemonServiceCheck struct {
	State DaemonServiceState
	// ServicePath — ProgramArguments[0] из plist ("" — plist не разобрался).
	ServicePath string
	Detail      string
	// CopyMissing — Stale потому, что копии нет (каталоги целы).
	CopyMissing bool
	// CopySHA256 / LauncherSHA256 — hex sha256 копии и ядра лаунчера; пусто,
	// если не считались.
	CopySHA256     string
	LauncherSHA256 string
	// CopyVersion — версия из сайдкара install.json, LauncherVersion —
	// `sing-box version` ядра лаунчера. Только для показа; вердикт по версии —
	// лишь запасной путь ProcessStale (ядро без executable_sha256).
	CopyVersion     string
	LauncherVersion string
	// RunningSHA256 / RunningVersion — что отвечает работающий демон
	// (/admin/info); пусто, если не спрашивали или поля нет.
	RunningSHA256  string
	RunningVersion string
	// LaunchdState — что launchd говорит о службе: значение `state = …` или
	// «not loaded»; пусто, если не спрашивали или спросить не удалось.
	LaunchdState string
}

// NeedsInstall — состояние лечится командой «Install or update service».
func (c DaemonServiceCheck) NeedsInstall() bool {
	switch c.State {
	case DaemonServiceUnsafe, DaemonServiceStale, DaemonServiceProcessStale:
		return true
	}
	return false
}

// NeedsBootstrap — служба установлена верно, но не запущена: лечится
// `launchctl bootstrap` (DaemonBootstrapCommand), а не install.
func (c DaemonServiceCheck) NeedsBootstrap() bool {
	return c.State == DaemonServiceNotRunning
}

// CopyUsable — plist указывает на каноническую копию, её цепочка владения
// цела и файл на месте: Uninstall и `lxd client add` можно звать через неё.
func (c DaemonServiceCheck) CopyUsable() bool {
	return c.State != DaemonServiceNotInstalled && c.State != DaemonServiceUnsafe && !c.CopyMissing
}

// daemonServiceLayout — где классификатор ищет службу. Прод —
// systemDaemonServiceLayout; тест строит свою раскладку во временном
// каталоге от собственного uid.
type daemonServiceLayout struct {
	PlistPath string
	CorePath  string
	// LegacyPath — копия ранней раскладки lx.11 (<label>/ или <label>).
	LegacyPath string
	ChainRoot  string
	OwnerUID   uint32
}

func systemDaemonServiceLayout() daemonServiceLayout {
	return daemonServiceLayout{
		PlistPath:  daemonSystemPlistPath(),
		CorePath:   daemonServiceCorePath(),
		LegacyPath: daemonServiceLegacyCopyPath,
		ChainRoot:  daemonServiceChainRoot,
		OwnerUID:   0,
	}
}

// errDaemonCopyMissing — звено цепочки или сама копия отсутствует, а всё,
// что выше, root-owned: создать недостающее пользователь не может, значит
// это не дыра, а служба без бинаря (Stale).
var errDaemonCopyMissing = errors.New("root-owned copy is missing")

// inspectDaemonServiceDefinition — первый шаг классификатора без хэшей:
// NotInstalled / Unsafe / Stale (копии нет) / OK (определение службы
// безопасно). Дёшев: чтение plist и Lstat цепочки.
func inspectDaemonServiceDefinition(l daemonServiceLayout) DaemonServiceCheck {
	if _, err := os.Lstat(l.PlistPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DaemonServiceCheck{State: DaemonServiceNotInstalled}
		}
		return DaemonServiceCheck{State: DaemonServiceUnsafe,
			Detail: fmt.Sprintf("cannot stat the service plist %s: %v", l.PlistPath, err)}
	}
	servicePath, err := readPlistProgramPath(l.PlistPath)
	if err != nil {
		return DaemonServiceCheck{State: DaemonServiceUnsafe,
			Detail: fmt.Sprintf("cannot read ProgramArguments[0] from %s: %v", l.PlistPath, err)}
	}
	c := DaemonServiceCheck{State: DaemonServiceOK, ServicePath: servicePath}
	if filepath.Clean(servicePath) != filepath.Clean(l.CorePath) {
		c.State = DaemonServiceUnsafe
		c.Detail = fmt.Sprintf("the service runs %s, not the root-owned copy %s", servicePath, l.CorePath)
		if isLegacyCopyPath(servicePath, l.LegacyPath) {
			c.Detail = fmt.Sprintf("the service runs %s, a copy in the legacy layout of early lx.11 builds: run Install or update service, then remove it (%s)",
				servicePath, legacyCopyRemoveCommand(l.LegacyPath))
		}
		return c
	}
	if err := checkRootOwnedChain(l.CorePath, l.ChainRoot, l.OwnerUID); err != nil {
		if errors.Is(err, errDaemonCopyMissing) {
			c.State = DaemonServiceStale
			c.CopyMissing = true
		} else {
			c.State = DaemonServiceUnsafe
		}
		c.Detail = err.Error()
	}
	return c
}

// checkRootOwnedChain проверяет каждое звено от root вниз до file по Lstat:
// не симлинк, владелец ownerUID, без записи для группы и остальных;
// каталоги — каталоги, file — обычный файл. Отсутствующее звено под целым
// родителем — errDaemonCopyMissing.
func checkRootOwnedChain(file, root string, ownerUID uint32) error {
	file = filepath.Clean(file)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, filepath.Dir(file))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is outside %s", file, root)
	}
	chain := []string{root}
	if rel != "." {
		cur := root
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			cur = filepath.Join(cur, part)
			chain = append(chain, cur)
		}
	}
	for _, dir := range chain {
		if err := checkRootOwnedEntry(dir, ownerUID, true); err != nil {
			return err
		}
	}
	return checkRootOwnedEntry(file, ownerUID, false)
}

func checkRootOwnedEntry(path string, ownerUID uint32, wantDir bool) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", errDaemonCopyMissing, path)
		}
		return fmt.Errorf("%s: %v", path, err)
	}
	mode := fi.Mode()
	switch {
	case mode&os.ModeSymlink != 0:
		return fmt.Errorf("%s is a symlink", path)
	case wantDir && !fi.IsDir():
		return fmt.Errorf("%s is not a directory", path)
	case !wantDir && fi.IsDir():
		return fmt.Errorf("%s is a directory, not the copy file: remove it (sudo rm -rf %s) and run the command again",
			path, shellQuote(path))
	case !wantDir && !mode.IsRegular():
		return fmt.Errorf("%s is not a regular file", path)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%s: no owner information", path)
	}
	if st.Uid != ownerUID {
		return fmt.Errorf("%s is owned by uid %d, want %d", path, st.Uid, ownerUID)
	}
	if mode.Perm()&0o022 != 0 {
		return fmt.Errorf("%s is writable by group or others (%04o)", path, mode.Perm())
	}
	return nil
}

// compareDaemonServiceFiles — второй шаг: sha256 копии против ядра лаунчера
// (после EvalSymlinks: в DataDir бывает ссылка на dev-сборку). Работает
// только поверх OK; не посчитался любой из хэшей — вердикт не выносится.
func compareDaemonServiceFiles(c *DaemonServiceCheck, corePath, launcherCore string, hashes *fileHashCache) {
	if c.State != DaemonServiceOK {
		return
	}
	copySum, err := hashes.sum(corePath)
	if err != nil {
		debuglog.DebugLog("daemon service: hash %s: %v", corePath, err)
		return
	}
	c.CopySHA256 = copySum
	if launcherCore == "" {
		return
	}
	resolved, err := filepath.EvalSymlinks(launcherCore)
	if err != nil {
		debuglog.DebugLog("daemon service: launcher core %s: %v", launcherCore, err)
		return
	}
	launcherSum, err := hashes.sum(resolved)
	if err != nil {
		debuglog.DebugLog("daemon service: hash %s: %v", resolved, err)
		return
	}
	c.LauncherSHA256 = launcherSum
	if launcherSum != copySum {
		c.State = DaemonServiceStale
		c.Detail = fmt.Sprintf("the root-owned copy (sha256 %s) is not the launcher core %s (sha256 %s)",
			shortSHA(copySum), resolved, shortSHA(launcherSum))
	}
}

// launchdJob — что launchd знает о службе. Known=false — спросить не
// удалось (нет launchctl, таймаут, непонятный вывод): вердикт не
// выносится. Loaded=false — службы в домене system нет.
type launchdJob struct {
	Known  bool
	Loaded bool
	State  string
}

// queryLaunchdJob — `launchctl print system/<label>` без sudo, с таймаутом;
// кэша нет: состояние меняется от любой команды пользователя.
func queryLaunchdJob(label string) launchdJob {
	ctx, cancel := context.WithTimeout(context.Background(), launchctlTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, launchctlTool, "print", "system/"+label).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == launchctlNotFoundStatus {
			return launchdJob{Known: true}
		}
		debuglog.DebugLog("daemon service: launchctl print system/%s: %v", label, err)
		return launchdJob{}
	}
	state := parseLaunchctlPrintState(out)
	if state == "" {
		debuglog.DebugLog("daemon service: launchctl print system/%s: no state line", label)
		return launchdJob{}
	}
	return launchdJob{Known: true, Loaded: true, State: state}
}

// parseLaunchctlPrintState — `state = …` самой службы: строка верхнего
// уровня блока (один таб), а не вложенных (endpoints и т.п.).
func parseLaunchctlPrintState(out []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "\tstate = ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "\tstate = "))
		}
	}
	return ""
}

// compareDaemonServiceLaunchd — шаг после файлов: служба на месте, но
// launchd её не держит — NotRunning. Работает только поверх OK; launchd не
// ответил — вердикт не выносится.
func compareDaemonServiceLaunchd(c *DaemonServiceCheck, job launchdJob) {
	if !job.Known {
		return
	}
	c.LaunchdState = job.State
	if !job.Loaded {
		c.LaunchdState = launchdNotLoaded
	}
	if c.State != DaemonServiceOK {
		return
	}
	switch {
	case !job.Loaded:
		c.State = DaemonServiceNotRunning
		c.Detail = "the service is not loaded in launchd"
	case job.State != launchdRunningState:
		c.State = DaemonServiceNotRunning
		c.Detail = fmt.Sprintf("launchd reports the service state %q", job.State)
	}
}

// compareDaemonServiceProcess — третий шаг: паспорт работающего демона.
// executable/executable_sha256 появились в lx.11; у старого ядра их нет, и
// запасной путь — версия (пустая и "unknown" вердикта не дают). Пустой
// executable_sha256 — «неизвестно» и у lx.11: хэш считается в фоне после
// старта демона, и до готовности поле пустое. ProcessStale по нему не
// выносится — судит версия.
func compareDaemonServiceProcess(c *DaemonServiceCheck, info lxdclient.InfoData, corePath string) {
	c.RunningSHA256 = info.ExecutableSHA256
	c.RunningVersion = info.Version
	if c.State != DaemonServiceOK {
		return
	}
	if info.Executable != "" && filepath.Clean(info.Executable) != filepath.Clean(corePath) {
		c.State = DaemonServiceProcessStale
		c.Detail = fmt.Sprintf("the running daemon was started from %s, the service runs %s", info.Executable, corePath)
		return
	}
	if info.ExecutableSHA256 != "" { // "" — ядро до lx.11 или хэш ещё считается
		if c.CopySHA256 != "" && !strings.EqualFold(info.ExecutableSHA256, c.CopySHA256) {
			c.State = DaemonServiceProcessStale
			c.Detail = fmt.Sprintf("the running daemon (sha256 %s) is not the service binary (sha256 %s)",
				shortSHA(info.ExecutableSHA256), shortSHA(c.CopySHA256))
		}
		return
	}
	if versionComparable(info.Version) && versionComparable(c.LauncherVersion) && info.Version != c.LauncherVersion {
		c.State = DaemonServiceProcessStale
		c.Detail = fmt.Sprintf("the running daemon reports version %s, the launcher core is %s", info.Version, c.LauncherVersion)
	}
}

// versionComparable — версия, по которой можно судить: dev-сборка
// репортит "unknown".
func versionComparable(v string) bool {
	return v != "" && v != "unknown"
}

// shortSHA — первые 12 hex-символов для лога и UI.
func shortSHA(sum string) string {
	if len(sum) > 12 {
		return sum[:12]
	}
	return sum
}

// readDaemonServiceSidecarVersion — версия из сайдкара копии corePath
// (<копия>.install.json); "" если сайдкара нет (ядро до lx.11) или он не
// разобрался. Только показ.
func readDaemonServiceSidecarVersion(corePath string) string {
	sidecarPath := daemonServiceSidecarPath(corePath)
	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		return ""
	}
	var sidecar struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &sidecar); err != nil {
		debuglog.DebugLog("daemon service: %s: %v", sidecarPath, err)
		return ""
	}
	return sidecar.Version
}

// fileHashKey — идентичность содержимого файла без чтения: замена файла
// (новый inode) или запись в него (size/mtime) меняет ключ.
type fileHashKey struct {
	dev   uint64
	ino   uint64
	size  int64
	mtime int64
}

// fileHashCache — sha256 файлов по (dev, inode, size, mtime). Классификатор
// зовут на каждом открытии окна и перед каждым apply, а ядро весит десятки
// мегабайт.
type fileHashCache struct {
	mu       sync.Mutex
	sums     map[fileHashKey]string
	computed int // сколько раз файл читался целиком (для теста)
}

// daemonServiceHashes — кэш процесса для классификатора службы.
var daemonServiceHashes fileHashCache

func statHashKey(path string) (fileHashKey, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return fileHashKey{}, err
	}
	if !fi.Mode().IsRegular() {
		return fileHashKey{}, fmt.Errorf("%s is not a regular file", path)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fileHashKey{}, fmt.Errorf("%s: no inode information", path)
	}
	return fileHashKey{dev: uint64(st.Dev), ino: st.Ino, size: fi.Size(), mtime: fi.ModTime().UnixNano()}, nil
}

// sum возвращает hex sha256 файла, из кэша при неизменном ключе. Файл,
// изменившийся во время чтения, не кешируется.
func (h *fileHashCache) sum(path string) (string, error) {
	key, err := statHashKey(path)
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	cached, ok := h.sums[key]
	h.mu.Unlock()
	if ok {
		return cached, nil
	}
	sum, err := sha256File(path)
	if err != nil {
		return "", err
	}
	after, statErr := statHashKey(path)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.computed++
	if statErr != nil || after != key {
		return sum, nil
	}
	if h.sums == nil || len(h.sums) >= daemonHashCacheCap {
		h.sums = make(map[fileHashKey]string)
	}
	h.sums[key] = sum
	return sum, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
