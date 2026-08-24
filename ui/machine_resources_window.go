package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
)

// Окно управления ресурсами машины — файлами, на которые ссылается её конфиг
// (SPEC 063: `.srs` rule-set'ы, geo-базы).
//
// Зачем отдельный экран: до него состояние ресурсов было невидимым. Deploy
// заливает недостающее молча, а расхождение содержимого не всплывает вовсе —
// конфиг ссылается на ИМЯ, и что под этим именем лежит на той стороне, знает
// только демон. Здесь обе стороны показаны рядом, с хешами.

// OpenMachineResourcesWindow открывает окно ресурсов машины.
func OpenMachineResourcesWindow(ac *core.AppController, d services.RemoteDaemon) {
	if ac == nil || ac.UIService == nil || ac.UIService.Application == nil {
		return
	}
	win := ac.UIService.Application.NewWindow(locale.Tf("%s — resources", d.Name))
	registry := services.NewRemoteRegistry(ac.FileService.ExecDir)

	list := container.NewVBox()
	summary := widget.NewLabel("")
	// Путь стора на машине: именно он уезжает в rule_set[].path, и при разборе
	// «почему ядро не нашло набор» это первое, что нужно увидеть.
	pathLabel := widget.NewLabel(d.ResourceDir())

	var reload func()
	reload = func() {
		list.RemoveAll()
		list.Add(widget.NewLabel(locale.T("Reading the machine's resource store…")))
		list.Refresh()
		go func() {
			entries, err := registry.ResourceOverview(d.ID)
			fyne.Do(func() {
				list.RemoveAll()
				if err != nil {
					debuglog.WarnLog("resources: overview %q: %v", d.ID, err)
					msg := widget.NewLabel(locale.Tf("Could not read the resource store: %v", err))
					msg.Wrapping = fyne.TextWrapWord
					list.Add(msg)
					list.Refresh()
					return
				}
				if len(entries) == 0 {
					hint := widget.NewLabel(locale.T("No resources: nothing stored on the machine and no rule-set files in this machine's profile."))
					hint.Wrapping = fyne.TextWrapWord
					list.Add(hint)
					list.Refresh()
					summary.SetText("")
					return
				}
				var nLocal, nServer, nOrphan int
				for _, e := range entries {
					if e.LocalSHA != "" {
						nLocal++
					}
					if e.ServerSHA != "" {
						nServer++
					}
					if e.State == services.ResourceOrphan {
						nOrphan++
					}
					list.Add(resourceRow(ac, registry, d, e, reload))
				}
				summary.SetText(locale.Tf("%d local · %d on server · %d orphan", nLocal, nServer, nOrphan))
				list.Refresh()
			})
		}()
	}

	refreshBtn := widget.NewButtonWithIcon(locale.T("Refresh"), theme.ViewRefreshIcon(), reload)

	// Залить всё недостающее одной кнопкой: типичный случай после настройки
	// нового правила — несколько наборов сразу, и жать по одному незачем.
	uploadAllBtn := widget.NewButton(locale.T("Upload all missing"), func() {
		go func() {
			entries, err := registry.ResourceOverview(d.ID)
			if err != nil {
				fyne.Do(func() { dialog.ShowError(err, win) })
				return
			}
			var failed error
			for _, e := range entries {
				if e.State != services.ResourceMissing && e.State != services.ResourceDiffers {
					continue
				}
				if err := registry.UploadResource(d.ID, e.Name); err != nil {
					failed = err
					break
				}
			}
			fyne.Do(func() {
				if failed != nil {
					dialog.ShowError(failed, win)
				}
				reload()
			})
		}()
	})
	uploadAllBtn.Importance = widget.HighImportance

	closeBtn := widget.NewButton(locale.T("Close"), func() { win.Close() })

	header := container.NewBorder(nil, nil, pathLabel, refreshBtn)
	footer := container.NewBorder(nil, nil, summary,
		container.NewHBox(uploadAllBtn, closeBtn))

	body := container.NewBorder(
		container.NewVBox(header, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), footer),
		nil, nil,
		components.WrapInScrollWithGutter(list),
	)
	win.SetContent(container.NewPadded(body))
	win.Resize(fyne.NewSize(680, 480))
	win.CenterOnScreen()
	win.Show()
	reload()
}

// resourceRow — строка одного ресурса: имя, размеры, состояние и действия.
func resourceRow(ac *core.AppController, registry *services.RemoteRegistry,
	d services.RemoteDaemon, e services.ResourceEntry, reload func()) fyne.CanvasObject {
	name := widget.NewLabelWithStyle(e.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	var stateText string
	switch e.State {
	case services.ResourceMatch:
		stateText = locale.T("✓ match")
	case services.ResourceMissing:
		stateText = locale.T("✗ not on server")
	case services.ResourceDiffers:
		stateText = locale.T("≠ differs")
	case services.ResourceOrphan:
		stateText = locale.T("⚠ only on server")
	}
	meta := widget.NewLabel(fmt.Sprintf("%s   %s   %s",
		sizeOrDash(e.LocalSize, e.LocalSHA != ""),
		sizeOrDash(e.ServerSize, e.ServerSHA != ""),
		stateText))

	run := func(action func() error) {
		go func() {
			err := action()
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(err, ac.UIService.MainWindow)
				}
				reload()
			})
		}()
	}

	buttons := container.NewHBox()
	// Залить: когда файла на сервере нет или он другой.
	if e.State == services.ResourceMissing || e.State == services.ResourceDiffers {
		up := widget.NewButtonWithIcon(locale.T("Upload"), theme.MoveUpIcon(), func() {
			run(func() error { return registry.UploadResource(d.ID, e.Name) })
		})
		if e.InUse {
			// Демон откажет 409: на имя ссылается работающий конфиг.
			// Показать нажимаемую кнопку значило бы обещать невозможное.
			up.Disable()
		}
		buttons.Add(up)
	}
	// Скачать: когда файла нет локально или он расходится.
	if e.State == services.ResourceOrphan || e.State == services.ResourceDiffers {
		down := widget.NewButtonWithIcon(locale.T("Download"), theme.MoveDownIcon(), func() {
			run(func() error { return registry.DownloadResource(d.ID, e.Name) })
		})
		buttons.Add(down)
	}
	// Удалить на сервере: всё, что там есть.
	if e.ServerSHA != "" {
		del := ttwidget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
			dialog.ShowConfirm(locale.T("Delete resource"),
				locale.Tf("Delete %s from the machine? The local copy stays.", e.Name),
				func(ok bool) {
					if ok {
						run(func() error { return registry.DeleteRemoteResource(d.ID, e.Name) })
					}
				}, ac.UIService.MainWindow)
		})
		if e.InUse {
			del.SetToolTip(locale.T("The running config references this file — deploy a config without it first"))
			del.Disable()
		} else {
			del.SetToolTip(locale.T("Delete from the machine"))
		}
		del.Importance = widget.LowImportance
		buttons.Add(del)
	}

	return container.NewVBox(
		container.NewBorder(nil, nil, name, buttons),
		meta,
		widget.NewSeparator(),
	)
}

// sizeOrDash — размер в человеческом виде либо «—», если стороны нет.
func sizeOrDash(size int64, present bool) string {
	if !present {
		return "—"
	}
	switch {
	case size >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(size)/(1<<20))
	case size >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(size)/(1<<10))
	default:
		return fmt.Sprintf("%d B", size)
	}
}
