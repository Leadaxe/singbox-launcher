package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// Ядро под платформу удалённой машины (SPEC 161, вкладка Core окна Service):
// скачать архив релиза форка под GOOS/GOARCH машины, сверить его sha256 с
// SHA256SUMS релиза, распаковать бинарь в ~/Downloads. Дальше пользователь
// заливает его на машину по рецепту core_upload. Ядро лаунчера не трогается.
// Файл без build-тега: машину обслуживают с любой платформы лаунчера.

// targetCoreSumsFile — файл сумм в каждом релизе форка: строки
// `<sha256 архива>␠␠<имя ассета>` (по архивам, не по бинарям).
const targetCoreSumsFile = "SHA256SUMS"

// ErrNoTargetAsset — форк не публикует сборку под платформу машины
// (например, linux/386): шаги загрузки остаются с плейсхолдером.
var ErrNoTargetAsset = errors.New("no release asset for this platform")

// TargetCoreDownload — итог скачивания (или проверки уже скачанного) ядра.
type TargetCoreDownload struct {
	// Path — распакованный бинарь в ~/Downloads; SidecarPath — <Path>.sha256
	// (формат sha256sum, пишется только после сверки с SHA256SUMS).
	Path, SidecarPath string
	// Asset — имя архива релиза; ArchiveSHA — его sha256; BinarySHA — sha256
	// бинаря (его же печатает шаг core_check на машине).
	Asset, ArchiveSHA, BinarySHA string
	// SumsVerified — архив сверен с SHA256SUMS релиза.
	SumsVerified bool
	// SumsMissing — SHA256SUMS не скачался или ассета в нём нет: sha256
	// бинаря надо сверить руками (шаг core_check).
	SumsMissing bool
}

// TargetCoreFileName — имя бинаря в ~/Downloads: версия и платформа в имени,
// чтобы ядра разных машин и версий не затирали друг друга.
func TargetCoreFileName(version, goos, goarch string) string {
	name := fmt.Sprintf("sing-box-%s-%s-%s", version, goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// DownloadsDir — ~/Downloads (создаётся при отсутствии).
func DownloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("downloads dir: %w", err)
	}
	dir := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("downloads dir: %w", err)
	}
	return dir, nil
}

// CheckTargetCore — ядро уже скачано и проверено: файл и сайдкар есть, sha256
// файла совпадает с сайдкаром. Без сети; читает файл целиком — звать из
// горутины.
func CheckTargetCore(version, goos, goarch string) (TargetCoreDownload, bool) {
	var out TargetCoreDownload
	dir, err := DownloadsDir()
	if err != nil {
		return out, false
	}
	out.Path = filepath.Join(dir, TargetCoreFileName(version, goos, goarch))
	out.SidecarPath = out.Path + ".sha256"
	if suffix := SingboxAssetSuffixFor(goos, goarch); suffix != "" {
		out.Asset = fmt.Sprintf("sing-box-%s-%s", version, suffix)
	}
	raw, err := os.ReadFile(out.SidecarPath)
	if err != nil {
		return out, false
	}
	want, _, _ := strings.Cut(strings.TrimSpace(string(raw)), " ")
	have, err := sha256File(out.Path)
	if err != nil || !strings.EqualFold(have, want) {
		return out, false
	}
	out.BinarySHA = have
	out.SumsVerified = true
	return out, true
}

// DownloadCoreForTarget скачивает ядро version под goos/goarch машины в
// ~/Downloads: архив (оригинал → зеркала, прогресс 15–75) → SHA256SUMS
// (best-effort) → сверка sha архива (несовпадение — ошибка, файл не
// кладётся) → распаковка → копия в ~/Downloads, chmod 0755 → сайдкар.
// Канал progress закрывает сама функция, как DownloadCore.
func (ac *AppController) DownloadCoreForTarget(ctx context.Context, version, goos, goarch string, progress chan DownloadProgress) (TargetCoreDownload, error) {
	defer close(progress)
	out, err := ac.downloadCoreForTarget(ctx, version, goos, goarch, progress)
	if err != nil {
		debuglog.WarnLog("DownloadCoreForTarget %s %s/%s: %v", version, goos, goarch, err)
		progress <- DownloadProgress{Message: err.Error(), Status: "error", Error: err}
		return out, err
	}
	progress <- DownloadProgress{Progress: 100, Message: out.Path, Status: "done"}
	return out, nil
}

func (ac *AppController) downloadCoreForTarget(ctx context.Context, version, goos, goarch string, progress chan DownloadProgress) (TargetCoreDownload, error) {
	var out TargetCoreDownload
	asset, err := directAssetNameFor(version, goos, goarch)
	if err != nil {
		return out, fmt.Errorf("%s/%s: %w", goos, goarch, ErrNoTargetAsset)
	}
	out.Asset = asset
	dir, err := DownloadsDir()
	if err != nil {
		return out, err
	}
	out.Path = filepath.Join(dir, TargetCoreFileName(version, goos, goarch))
	out.SidecarPath = out.Path + ".sha256"

	// Свой каталог в системном temp, а не temp лаунчера: DownloadCore по
	// завершении сносит тот целиком и задел бы параллельное скачивание.
	tempDir, err := os.MkdirTemp("", "singbox-target-core-")
	if err != nil {
		return out, fmt.Errorf("temp dir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			debuglog.WarnLog("DownloadCoreForTarget: remove temp dir %s: %v", tempDir, err)
		}
	}()

	progress <- DownloadProgress{Progress: 5, Message: "Getting release checksums...", Status: "downloading"}
	wantSHA := ac.fetchReleaseSum(ctx, version, asset, filepath.Join(tempDir, targetCoreSumsFile))
	out.SumsMissing = wantSHA == ""

	archive := filepath.Join(tempDir, asset)
	progress <- DownloadProgress{Progress: 15, Message: "Downloading " + asset + "...", Status: "downloading"}
	if err := ac.downloadFile(ctx, releaseDownloadURL(version, asset), archive, progress); err != nil {
		return out, err
	}
	if out.ArchiveSHA, err = sha256File(archive); err != nil {
		return out, err
	}
	if !out.SumsMissing {
		if !strings.EqualFold(out.ArchiveSHA, wantSHA) {
			return out, fmt.Errorf("%s: sha256 %s does not match %s (%s)", asset, out.ArchiveSHA, targetCoreSumsFile, wantSHA)
		}
		out.SumsVerified = true
	}

	progress <- DownloadProgress{Progress: 80, Message: "Extracting archive...", Status: "extracting"}
	binName := "sing-box"
	if goos == "windows" {
		binName += ".exe"
	}
	binary, _, err := ac.extractArchiveNamed(archive, tempDir, binName)
	if err != nil {
		return out, err
	}

	progress <- DownloadProgress{Progress: 90, Message: "Saving to Downloads...", Status: "extracting"}
	// Старый сайдкар — долой до замены файла: оборвись копия посередине, он
	// подтверждал бы недописанный бинарь.
	if err := os.Remove(out.SidecarPath); err != nil && !os.IsNotExist(err) {
		return out, fmt.Errorf("remove %s: %w", out.SidecarPath, err)
	}
	if err := copyFileAtomic(binary, out.Path, 0o755); err != nil {
		return out, err
	}
	if out.BinarySHA, err = sha256File(out.Path); err != nil {
		return out, err
	}
	// Сайдкар — только за сверенный архив: CheckTargetCore по нему ставит
	// «✓ sha256 matches», без SHA256SUMS это было бы неправдой.
	if out.SumsVerified {
		line := out.BinarySHA + "  " + filepath.Base(out.Path) + "\n"
		if err := os.WriteFile(out.SidecarPath, []byte(line), 0o644); err != nil {
			return out, fmt.Errorf("write %s: %w", out.SidecarPath, err)
		}
	}
	return out, nil
}

// fetchReleaseSum — sha256 ассета из SHA256SUMS релиза; "" — файла нет, он
// не скачался или ассета в нём нет (best-effort: ядро всё равно скачается,
// sha бинаря покажется для ручной сверки).
func (ac *AppController) fetchReleaseSum(ctx context.Context, version, asset, dest string) string {
	// downloadFile шлёт прогресс в канал — свой, сливаемый: полоса скачивания
	// архива не должна прыгать от крошечного файла сумм.
	sink := make(chan DownloadProgress, 10)
	drained := make(chan struct{})
	go func() {
		for range sink {
		}
		close(drained)
	}()
	err := ac.downloadFile(ctx, releaseDownloadURL(version, targetCoreSumsFile), dest, sink)
	close(sink)
	<-drained
	if err != nil {
		debuglog.WarnLog("DownloadCoreForTarget: %s unavailable: %v", targetCoreSumsFile, err)
		return ""
	}
	f, err := os.Open(dest)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return fields[0]
		}
	}
	debuglog.WarnLog("DownloadCoreForTarget: %s has no line for %s", targetCoreSumsFile, asset)
	return ""
}

// copyFileAtomic копирует src в dst через dst.tmp + rename: недописанный
// файл под итоговым именем не появляется.
func copyFileAtomic(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	outFile, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}
	if _, err := io.Copy(outFile, in); err != nil {
		outFile.Close()
		os.Remove(tmp)
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := outFile.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	// OpenFile не меняет права уже существующего tmp, а umask режет новые.
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("chmod %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", dst, err)
	}
	return nil
}
