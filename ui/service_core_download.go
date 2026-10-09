package ui

import (
	"context"
	"errors"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
)

// Шаг 1 вкладки Core для удалённой машины (SPEC 161 §5.3, PLAN §6.6): ядро
// под платформу МАШИНЫ в ~/Downloads со сверкой SHA256SUMS релиза. Ядро
// лаунчера не трогается. Состояние живёт в окне, а не во вкладке: вкладка
// перестраивается на каждом изменении снапшота, скачивание — нет.

const (
	serviceCoreVerifiedText    = "%s  ✓ sha256 matches SHA256SUMS"
	serviceCoreUnverifiedText  = "%s — sha256 %s. SHA256SUMS is unavailable: compare this sum with the one step 3 prints on the machine."
	serviceCoreNoAssetText     = "The core release has no build for %s/%s — build the core for this machine yourself."
	serviceCoreDownloadTimeout = 10 * time.Minute
)

// targetCoreDownload — кнопка, полоса и итог скачивания ядра под машину.
type targetCoreDownload struct {
	ac           *core.AppController
	win          fyne.Window
	goos, goarch string
	btn          *widget.Button
	bar          *widget.ProgressBar
	result       *widget.Label
	busy         bool
	// got — скачанное и проверенное ядро (Path пуст — ещё нет).
	got core.TargetCoreDownload
	// onReady — ядро появилось: окно перестраивает рецепты с его путём.
	onReady func()
}

// newTargetCoreDownload строит шаг и в фоне проверяет, не скачано ли ядро
// раньше (файл и сайдкар в ~/Downloads совпали — сразу ✓, без сети).
func newTargetCoreDownload(ac *core.AppController, win fyne.Window, goos, goarch string, onReady func()) *targetCoreDownload {
	d := &targetCoreDownload{ac: ac, win: win, goos: goos, goarch: goarch, onReady: onReady}
	d.bar = widget.NewProgressBar()
	d.bar.Hide()
	d.result = widget.NewLabel("")
	d.result.Wrapping = fyne.TextWrapWord
	d.result.Hide()
	d.btn = widget.NewButton(locale.Tf("Download %s", constants.RequiredCoreVersion), d.start)
	d.btn.Importance = widget.HighImportance

	if core.SingboxAssetSuffixFor(goos, goarch) == "" {
		d.btn.Disable()
		d.showResult(locale.Tf(serviceCoreNoAssetText, goos, goarch), widget.WarningImportance)
		return d
	}
	go func() {
		got, ok := core.CheckTargetCore(constants.RequiredCoreVersion, goos, goarch)
		if !ok {
			return
		}
		fyne.Do(func() {
			if d.busy || d.got.Path != "" {
				return
			}
			d.finish(got)
		})
	}()
	return d
}

// matches — шаг собран под эту платформу (машине могли поменять платформу).
func (d *targetCoreDownload) matches(goos, goarch string) bool {
	return d.goos == goos && d.goarch == goarch
}

// path — путь скачанного ядра для шага загрузки; "" — ещё нет.
func (d *targetCoreDownload) path() string { return d.got.Path }

func (d *targetCoreDownload) object() fyne.CanvasObject {
	return container.NewVBox(container.NewBorder(nil, nil, d.btn, nil, d.bar), d.result)
}

func (d *targetCoreDownload) start() {
	if d.busy {
		return
	}
	d.busy = true
	d.btn.Disable()
	d.bar.SetValue(0)
	d.bar.Show()
	d.result.Hide()

	progress := make(chan core.DownloadProgress, 10)
	go func() {
		for p := range progress {
			value := float64(p.Progress) / 100
			fyne.Do(func() { d.bar.SetValue(value) })
		}
	}()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), serviceCoreDownloadTimeout)
		defer cancel()
		got, err := d.ac.DownloadCoreForTarget(ctx, constants.RequiredCoreVersion, d.goos, d.goarch, progress)
		fyne.Do(func() {
			d.busy = false
			d.btn.Enable()
			d.bar.Hide()
			if err != nil {
				debuglog.WarnLog("service window: download core for %s/%s: %v", d.goos, d.goarch, err)
				if errors.Is(err, core.ErrNoTargetAsset) {
					d.showResult(locale.Tf(serviceCoreNoAssetText, d.goos, d.goarch), widget.WarningImportance)
					return
				}
				d.showResult(locale.Tf("Download failed: %s", downloadFailureReason(err)), widget.DangerImportance)
				return
			}
			d.finish(got)
		})
	}()
}

// finish — ядро на месте: итог и перестройка рецептов (путь в шаге 2).
func (d *targetCoreDownload) finish(got core.TargetCoreDownload) {
	d.got = got
	if got.SumsVerified {
		d.showResult(locale.Tf(serviceCoreVerifiedText, got.Path), widget.SuccessImportance)
	} else {
		d.showResult(locale.Tf(serviceCoreUnverifiedText, got.Path, got.BinarySHA), widget.WarningImportance)
	}
	if d.onReady != nil {
		d.onReady()
	}
}

func (d *targetCoreDownload) showResult(text string, importance widget.Importance) {
	d.result.Importance = importance
	d.result.SetText(text)
	d.result.Show()
}
