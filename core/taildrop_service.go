// File taildrop_service.go — служебные файлы Taildrop на этом компьютере
// (SPEC 176): разрешения по устройствам, разбор кадра входящих своего ядра,
// выполнение clipboard / open_url, отправка текста устройству.
//
// Только своё ядро: команда служебного файла — про буфер и браузер ЭТОГО
// компьютера. Текст буфера, ссылки, имена файлов и устройств в журнал не
// пишутся (SPEC 148 §3).
package core

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// localTailscaleTags — источник списка tailscale-узлов своего ядра.
type localTailscaleTags interface {
	TailscaleTags() []string
}

// taildropServiceTags — узлы своего ядра, чьи служебные файлы разбираются:
// ядро работает и умеет Taildrop. Иначе — пусто.
func (ac *AppController) taildropServiceTags() []string {
	local := TailscaleTarget{Scope: services.ScopeLocal}
	if !ac.TaildropAvailable(local) || !ac.TailscaleCoreRunning(local) {
		return nil
	}
	src, ok := ac.Backend().(localTailscaleTags)
	if !ok {
		return nil
	}
	return src.TailscaleTags()
}

// TaildropDeviceName — имя устройства для разрешения по имени: DNS-имя без
// домена (ComputedName ядра — то, что приходит в senderName), иначе HostName.
func TaildropDeviceName(p services.TailscalePeer) string {
	if dns := strings.TrimSuffix(p.DNSName, "."); dns != "" {
		if i := strings.IndexByte(dns, '.'); i > 0 {
			return dns[:i]
		}
		return dns
	}
	return p.HostName
}

// taildropServicePermissionFor — разрешения отправителя файла: по senderID,
// а при пустом senderID (старое ядро) — по senderName.
func taildropServicePermissionFor(perms []locale.TaildropServicePermission, senderID, senderName string) locale.TaildropServicePermission {
	for _, p := range perms {
		switch {
		case senderID != "":
			if p.SenderID == senderID {
				return p
			}
		case senderName != "":
			if strings.EqualFold(p.SenderName, senderName) {
				return p
			}
		}
	}
	return locale.TaildropServicePermission{}
}

// TaildropServicePermission — разрешения устройства (галки в его меню).
func (ac *AppController) TaildropServicePermission(p services.TailscalePeer) (clipboard, openURL bool) {
	perms := locale.LoadSettings(ac.FileService.Layout.Data.Bin()).TaildropServicePermissions
	got := taildropServicePermissionFor(perms, p.StableID, TaildropDeviceName(p))
	return got.Clipboard, got.OpenURL
}

// SetTaildropServicePermission ставит или снимает разрешение вида kind
// (services.TaildropServiceClipboard / TaildropServiceOpenURL) устройству.
// Запись без единого разрешения удаляется.
func (ac *AppController) SetTaildropServicePermission(p services.TailscalePeer, kind string, on bool) error {
	if p.StableID == "" {
		return errors.New("device has no stable ID")
	}
	binDir := ac.FileService.Layout.Data.Bin()
	st := locale.LoadSettings(binDir)
	entry := locale.TaildropServicePermission{SenderID: p.StableID}
	kept := st.TaildropServicePermissions[:0]
	for _, e := range st.TaildropServicePermissions {
		if e.SenderID == p.StableID {
			entry = e
			continue
		}
		kept = append(kept, e)
	}
	entry.SenderName = TaildropDeviceName(p)
	switch kind {
	case services.TaildropServiceClipboard:
		entry.Clipboard = on
	case services.TaildropServiceOpenURL:
		entry.OpenURL = on
	default:
		return errors.New("unknown permission kind")
	}
	if entry.Clipboard || entry.OpenURL {
		kept = append(kept, entry)
	}
	st.TaildropServicePermissions = kept
	return locale.SaveSettings(binDir, st)
}

// TaildropSendText отправляет устройству служебный файл с одной командой.
func (ac *AppController) TaildropSendText(ctx context.Context, t TailscaleTarget, tag, peerStableID string, cmd services.TaildropServiceCommand) error {
	rules, err := services.TaildropServiceRulesFromRegistry()
	if err != nil {
		return err
	}
	name, body, err := rules.BuildTaildropService(cmd, time.Now())
	if err != nil {
		return err
	}
	err = ac.TaildropSend(ctx, t, tag, peerStableID,
		[]services.TaildropOutgoing{{Name: name, Size: int64(len(body)), Body: bytes.NewReader(body)}}, nil)
	if err != nil {
		debuglog.WarnLog("taildrop %s: service file not sent", tag)
		return err
	}
	debuglog.InfoLog("taildrop %s: service file sent (%s)", tag, cmd.Kind)
	return nil
}

// taildropServiceItem — служебный файл кадра, разобранный.
type taildropServiceItem struct {
	file services.TaildropFile
	cmds []services.TaildropServiceCommand
}

// readTaildropService — тело кандидата в память и разбор. ok=false —
// обычный файл; err — не удалось скачать.
func (ac *AppController) readTaildropService(ctx context.Context, t TailscaleTarget, tag string, rules services.TaildropServiceRules, f services.TaildropFile) ([]services.TaildropServiceCommand, bool, error) {
	var buf bytes.Buffer
	if _, err := ac.TaildropDownload(ctx, t, tag, f.Name, &buf); err != nil {
		return nil, false, err
	}
	cmds, ok := rules.ParseTaildropService(buf.Bytes())
	return cmds, ok, nil
}

// runTaildropServices выполняет служебные файлы кадра и удаляет их
// (SERVICE_FILES §6). В буфер идёт последний clipboard по времени приёма.
func (ac *AppController) runTaildropServices(t TailscaleTarget, tag string, rules services.TaildropServiceRules, items []taildropServiceItem) {
	if len(items) == 0 {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].file, items[j].file
		if !a.ModifiedAt.Equal(b.ModifiedAt) {
			return a.ModifiedAt.Before(b.ModifiedAt)
		}
		return a.Name < b.Name
	})
	perms := locale.LoadSettings(ac.FileService.Layout.Data.Bin()).TaildropServicePermissions
	now := time.Now()
	clip, clipFrom, haveClip := "", "", false
	ran, skipped := 0, 0
	for _, it := range items {
		f := it.file
		if f.ModifiedAt.IsZero() || now.Sub(f.ModifiedAt) > rules.TTL {
			skipped += len(it.cmds)
			continue
		}
		perm := taildropServicePermissionFor(perms, f.SenderID, f.SenderName)
		from := f.SenderName
		if from == "" {
			from = locale.T("unknown device")
		}
		for _, c := range it.cmds {
			switch c.Kind {
			case services.TaildropServiceClipboard:
				text, ok := c.ClipboardText()
				if !ok || !perm.Clipboard {
					skipped++
					continue
				}
				clip, clipFrom, haveClip = text, from, true
				ran++
			case services.TaildropServiceOpenURL:
				if !perm.OpenURL || !rules.OpenURLAllowed(c.URL) {
					skipped++
					continue
				}
				if err := platform.OpenURL(strings.TrimSpace(c.URL)); err != nil {
					debuglog.WarnLog("taildrop %s: link from a service file not opened: %v", tag, err)
					continue
				}
				taildropNotify(locale.T("Link opened"), locale.Tf("Link from %s opened in the browser.", from))
				ran++
			default:
				skipped++
			}
		}
	}
	if haveClip {
		fyne.Do(func() {
			if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
				app.Clipboard().SetContent(clip)
			}
		})
		taildropNotify(locale.T("Clipboard updated"), locale.Tf("Text from %s copied to the clipboard.", clipFrom))
	}
	// Удаление — всегда, даже если ни одна команда не выполнена.
	for _, it := range items {
		if err := ac.TaildropDelete(t, tag, it.file.Name); err != nil {
			debuglog.WarnLog("taildrop %s: service file not removed from inbox", tag)
		}
	}
	debuglog.InfoLog("taildrop %s: %d service file(s): %d command(s) run, %d skipped", tag, len(items), ran, skipped)
}

// taildropNotify — системное уведомление: свой путь macOS, иначе Fyne.
func taildropNotify(title, body string) {
	if platform.SendNotification(title, body) {
		return
	}
	if app := fyne.CurrentApp(); app != nil {
		app.SendNotification(&fyne.Notification{Title: title, Content: body})
	}
}
