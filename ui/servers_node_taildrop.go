// File servers_node_taildrop.go — файлы Taildrop на вкладке Network окна
// узла Tailscale (SPEC 175): секция Files (входящие: Save, Delete, Cancel
// приёма, галка автосохранения) и отправка файла устройству из его меню.
//
// Входящие — подписка SubscribeTaildropInbox на время открытого окна, у ядра
// окна (core.TailscaleTarget). Save кладёт файл в ~/Downloads и убирает его
// из входящих (core.TaildropSaveToDownloads — тот же путь, что у
// автосохранения). Имена файлов и устройств в журнал не пишутся (SPEC 148 §3).
package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// taildropInboxState — последний кадр входящих; пишет поток подписки,
// читает цикл перерисовки вкладки.
type taildropInboxState struct {
	mu    sync.Mutex
	inbox services.TaildropInbox
	have  bool
}

func (s *taildropInboxState) set(in services.TaildropInbox, have bool) {
	s.mu.Lock()
	s.inbox, s.have = in, have
	s.mu.Unlock()
}

func (s *taildropInboxState) get() (services.TaildropInbox, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inbox, s.have
}

// taildropView — что секции Files нужно для отрисовки.
type taildropView struct {
	// available — у ядра окна есть Taildrop (gRPC-источник).
	available bool
	have      bool
	inbox     services.TaildropInbox
}

// key — ключ перерисовки секции.
func (v taildropView) key() string {
	return fmt.Sprintf("%v|%v|%+v", v.available, v.have, v.inbox)
}

// superviseTaildropInbox держит подписку на входящие до отмены ctx:
// обрыв (ядро перезапущено, демон недоступен) — переподписка с backoff.
func superviseTaildropInbox(ctx context.Context, ac *core.AppController, target core.TailscaleTarget, tag string, state *taildropInboxState) {
	backoff := time.Second
	lastErr := ""
	for ctx.Err() == nil {
		got := false
		err := ac.TaildropSubscribe(ctx, target, tag, func(in services.TaildropInbox) {
			got = true
			state.set(in, true)
		})
		state.set(services.TaildropInbox{}, false)
		if err != nil && err.Error() != lastErr {
			lastErr = err.Error()
			debuglog.DebugLog("node network: taildrop inbox %s: %v", tag, err)
		}
		if got {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

// buildTaildropSection — секция Files: принимаемые (Cancel) и принятые
// (Save, Delete).
func buildTaildropSection(cmd tailscaleCommands, win fyne.Window, box *fyne.Container, tag string, st services.TailscaleStatus, v taildropView) {
	if !v.available {
		return
	}
	note := func(text string) {
		l := widget.NewLabel(text)
		l.Wrapping = fyne.TextWrapWord
		l.Importance = widget.LowImportance
		box.Add(l)
	}
	box.Add(widget.NewSeparator())
	box.Add(sectionHeader(locale.T("Files")))
	if !st.CanShareFiles {
		note(locale.T("File sharing is off for this tailnet. Turn on Taildrop in the Tailscale admin console."))
		return
	}
	// Галка автосохранения (§3.4): применяется сразу, без Save; держит
	// подписку и при закрытом окне.
	auto := widget.NewCheck(locale.T("Save to Downloads automatically"), nil)
	auto.SetChecked(cmd.ac.TaildropAutoSaveEnabled(cmd.target, tag))
	auto.OnChanged = func(on bool) {
		if err := cmd.ac.SetTaildropAutoSave(cmd.target, tag, on); err != nil {
			debuglog.WarnLog("taildrop %s: auto-save setting not saved", tag)
			ShowError(win, err)
		}
	}
	box.Add(auto)
	if !v.have {
		box.Add(widget.NewProgressBarInfinite())
		return
	}
	if len(v.inbox.Files) == 0 && len(v.inbox.Receiving) == 0 {
		note(locale.T("No files."))
		return
	}
	for _, r := range v.inbox.Receiving {
		r := r
		progress := paths.FormatBytes(r.ReceivedBytes)
		if r.Size > 0 {
			progress += " / " + paths.FormatBytes(r.Size)
		}
		cancel := widget.NewButton(locale.T("Cancel"), func() {
			go func() {
				if err := cmd.ac.TaildropCancelReceiving(cmd.target, tag, r.SenderID, r.Name); err != nil {
					debuglog.WarnLog("taildrop %s: receiving not cancelled", tag)
					fyne.Do(func() { ShowError(win, err) })
				}
			}()
		})
		box.Add(taildropFileRow(r.Name, taildropFileDetails(progress, r.SenderName), cancel))
	}
	for _, f := range v.inbox.Files {
		f := f
		var save, del *widget.Button
		save = widget.NewButton(locale.T("Save"), func() {
			save.Disable()
			go func() {
				path, err := cmd.ac.TaildropSaveToDownloads(context.Background(), cmd.target, tag, f.Name)
				fyne.Do(func() {
					if err != nil {
						save.Enable()
						ShowError(win, err)
						return
					}
					showTaildropSaved(win, path)
				})
			}()
		})
		save.Importance = widget.HighImportance
		del = widget.NewButton(locale.T("Delete"), func() {
			dialog.ShowConfirm(locale.T("Delete file"),
				locale.Tf("Delete %s from the inbox?", f.Name), func(yes bool) {
					if !yes {
						return
					}
					del.Disable()
					go func() {
						if err := cmd.ac.TaildropDelete(cmd.target, tag, f.Name); err != nil {
							debuglog.WarnLog("taildrop %s: file not deleted", tag)
							fyne.Do(func() {
								del.Enable()
								ShowError(win, err)
							})
						}
					}()
				}, win)
		})
		box.Add(taildropFileRow(f.Name, taildropFileDetails(paths.FormatBytes(f.Size), f.SenderName), container.NewHBox(save, del)))
	}
}

// taildropFileDetails — вторая строка файла: размер (или прогресс) и
// отправитель.
func taildropFileDetails(size, sender string) string {
	if sender == "" {
		return size
	}
	return size + " · " + locale.Tf("from %s", sender)
}

// taildropFileRow — файл двумя строками (имя, детали) и кнопки справа.
func taildropFileRow(name, details string, actions fyne.CanvasObject) fyne.CanvasObject {
	title := widget.NewLabel(name)
	title.Truncation = fyne.TextTruncateEllipsis
	sub := widget.NewLabel(details)
	sub.Importance = widget.LowImportance
	sub.Truncation = fyne.TextTruncateEllipsis
	return container.NewBorder(nil, nil, nil, container.NewCenter(actions), container.NewVBox(title, sub))
}

// showTaildropSaved — куда лёг файл и кнопка открыть папку.
func showTaildropSaved(win fyne.Window, path string) {
	label := widget.NewLabel(path)
	label.Wrapping = fyne.TextWrapBreak
	label.Selectable = true
	d := dialog.NewCustomConfirm(locale.T("File saved"), locale.T("Show in folder"), locale.T("Close"), label, func(open bool) {
		if !open {
			return
		}
		if err := platform.OpenFolder(filepath.Dir(path)); err != nil {
			ShowError(win, err)
		}
	}, win)
	d.Resize(fyne.NewSize(420, 0))
	d.Show()
}

// canSendTaildrop — устройство принимает файл от этого узла (как у LxBox:
// в сети, canReceiveFiles, не расшаренное).
func canSendTaildrop(st services.TailscaleStatus, p services.TailscalePeer) bool {
	return st.CanShareFiles && p.Online && p.CanReceiveFiles && !p.ShareeNode
}

// sendTaildropFile — выбор файла и лист отправки с прогрессом и Cancel.
func sendTaildropFile(cmd tailscaleCommands, win fyne.Window, tag string, p services.TailscalePeer) {
	open := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil {
			ShowError(win, err)
			return
		}
		if rc == nil {
			return
		}
		size := int64(-1)
		if u := rc.URI(); u != nil && u.Scheme() == "file" {
			if fi, serr := os.Stat(u.Path()); serr == nil {
				size = fi.Size()
			}
		}
		name := rc.URI().Name()
		showTaildropSend(cmd, win, tag, p, name, size, rc)
	}, win)
	if dir, err := core.DownloadsDir(); err == nil {
		if lister, lerr := storage.ListerForURI(storage.NewFileURI(dir)); lerr == nil {
			open.SetLocation(lister)
		}
	}
	open.Show()
}

// showTaildropSend — лист отправки: прогресс, Cancel; по концу — итог.
func showTaildropSend(cmd tailscaleCommands, win fyne.Window, tag string, p services.TailscalePeer, name string, size int64, rc fyne.URIReadCloser) {
	ctx, cancel := context.WithCancel(context.Background())
	bar := widget.NewProgressBar()
	status := widget.NewLabel(locale.Tf("Sending %s to %s…", name, p.HostName))
	status.Wrapping = fyne.TextWrapWord
	var body fyne.CanvasObject = container.NewVBox(status, bar)
	if size < 0 {
		body = container.NewVBox(status, widget.NewProgressBarInfinite())
	} else if size > 0 {
		bar.Max = float64(size)
	}
	d := dialog.NewCustom(locale.T("Send file"), locale.T("Cancel"), body, win)
	d.SetOnClosed(cancel)
	d.Resize(fyne.NewSize(420, 0))
	d.Show()
	go func() {
		defer rc.Close()
		err := cmd.ac.TaildropSend(ctx, cmd.target, tag, p.StableID,
			[]services.TaildropOutgoing{{Name: name, Size: size, Body: rc}},
			func(_ int, sent int64) {
				if size > 0 {
					fyne.Do(func() { bar.SetValue(float64(sent)) })
				}
			})
		cancelled := ctx.Err() != nil
		cancel()
		fyne.Do(func() {
			d.Hide()
			switch {
			case cancelled:
			case err != nil:
				debuglog.WarnLog("taildrop %s: file not sent", tag)
				ShowError(win, err)
			default:
				dialog.ShowInformation(locale.T("Send file"), locale.Tf("%s sent to %s.", name, p.HostName), win)
			}
		})
	}()
}
