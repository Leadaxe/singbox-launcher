//go:build darwin

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/lxdclient"
)

// Классификатор состояния launchd-службы демона (SPEC 136).
//
// Ядро lx.11+ на `lxd --service=install` копирует себя в root-owned каталог
// службы и переписывает plist на копию (SPEC 100 форка). Лаунчер ничего не
// копирует: он только читает plist, проверяет цепочку владения копии и
// сверяет sha256 копии с ядром лаунчера и с тем, что отвечает работающий
// демон. Всё читается без root: plist и сайдкар 0644, каталог и копия 0755.

const (
	// daemonServiceChainRoot — верх цепочки владения копии: от него вниз до
	// файла каждое звено обязано быть root-owned без g/o-записи.
	daemonServiceChainRoot = "/Library"
	// daemonServiceHelperDir — каталог службы (root:wheel 0755), зеркалит
	// раскладку ядра lx.11.
	daemonServiceHelperDir = "/Library/PrivilegedHelperTools/" + daemonLaunchdLabel
	// daemonServiceBinaryName — имя копии: то же `sing-box`, что у ядра
	// лаунчера (pgrep/ps и диагностика по имени процесса не ломаются).
	daemonServiceBinaryName = "sing-box"
	// daemonServiceSidecarName — сайдкар установки (root:wheel 0644):
	// {source, sha256, version, installed_at, plist_path, label}.
	daemonServiceSidecarName = "install.json"
	// daemonHashCacheCap — потолок кэша sha256: файлов в игре два-три, потолок
	// лишь не даёт кэшу расти от череды заменённых ядер.
	daemonHashCacheCap = 16
)

// daemonServiceCorePath — каноническая root-owned копия ядра службы.
func daemonServiceCorePath() string {
	return filepath.Join(daemonServiceHelperDir, daemonServiceBinaryName)
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
}

// NeedsInstall — состояние лечится командой «Install or update service».
func (c DaemonServiceCheck) NeedsInstall() bool {
	switch c.State {
	case DaemonServiceUnsafe, DaemonServiceStale, DaemonServiceProcessStale:
		return true
	}
	return false
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
	ChainRoot string
	OwnerUID  uint32
}

func systemDaemonServiceLayout() daemonServiceLayout {
	return daemonServiceLayout{
		PlistPath: daemonSystemPlistPath(),
		CorePath:  daemonServiceCorePath(),
		ChainRoot: daemonServiceChainRoot,
		OwnerUID:  0,
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

// compareDaemonServiceProcess — третий шаг: паспорт работающего демона.
// executable/executable_sha256 появились в lx.11; у старого ядра их нет, и
// запасной путь — версия (пустая и "unknown" вердикта не дают).
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
	if info.ExecutableSHA256 != "" {
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

// readDaemonServiceSidecarVersion — версия из install.json каталога службы;
// "" если сайдкара нет (ядро до lx.11) или он не разобрался. Только показ.
func readDaemonServiceSidecarVersion(helperDir string) string {
	data, err := os.ReadFile(filepath.Join(helperDir, daemonServiceSidecarName))
	if err != nil {
		return ""
	}
	var sidecar struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &sidecar); err != nil {
		debuglog.DebugLog("daemon service: %s: %v", daemonServiceSidecarName, err)
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
