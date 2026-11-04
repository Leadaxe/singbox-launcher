// File taildrop_double_copy.go — двойное копирование (SPEC 177): текст,
// скопированный дважды подряд (Cmd+C / Ctrl+C два раза), уходит служебным
// файлом clipboard на устройство, выбранное в настройках узла (SPEC 179).
//
// Жест — действие человека (SERVICE_FILES §7 п.4). Окна показа нет, поэтому
// содержимое с пометкой чувствительности ОС (пароль из менеджера паролей)
// не отправляется никогда (§7 п.3). Текст в журнал не пишется.
package core

import (
	"context"
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

const (
	// taildropDoubleCopyWindow — два копирования одного текста не дальше
	// этого интервала — жест.
	taildropDoubleCopyWindow = time.Second
	// taildropDoubleCopyPoll — опрос счётчика буфера, пока жест включён.
	taildropDoubleCopyPoll = 150 * time.Millisecond
	// taildropDoubleCopyIdle — опрос, пока жест выключен или ядро стоит.
	taildropDoubleCopyIdle = 2 * time.Second
)

// TaildropDoubleCopySupported — умеет ли платформа видеть копирования.
func TaildropDoubleCopySupported() bool {
	_, ok := platform.ClipboardChangeCount()
	return ok
}

// taildropDoubleCopyTarget — куда уходит жест: узел своего ядра и его устройство.
type taildropDoubleCopyTarget struct {
	Tag string
	state.TaildropPeer
}

// taildropDoubleCopyTargets — узлы с разрешённым Taildrop и выбранным устройством.
func (ac *AppController) taildropDoubleCopyTargets() []taildropDoubleCopyTarget {
	var out []taildropDoubleCopyTarget
	for _, own := range ownTailscaleNodes(ac.taildropProfile("")) {
		peer := own.node.TaildropDoubleCopy
		if peer == nil || peer.PeerID == "" || !taildropAllowedBody(own.node.Body) {
			continue
		}
		out = append(out, taildropDoubleCopyTarget{Tag: own.tag, TaildropPeer: *peer})
	}
	return out
}

// startTaildropDoubleCopy — цикл жеста: пока есть выбранные устройства и
// своё ядро с Taildrop работает, следит за счётчиком буфера.
func (ac *AppController) startTaildropDoubleCopy() {
	if !TaildropDoubleCopySupported() {
		return
	}
	var (
		lastCount uint64
		haveCount bool
		lastText  string
		lastAt    time.Time
	)
	for {
		wait := taildropDoubleCopyIdle
		targets := ac.taildropDoubleCopyTargets()
		local := TailscaleTarget{Scope: services.ScopeLocal}
		if len(targets) > 0 && ac.TaildropAvailable(local) && ac.TailscaleCoreRunning(local) {
			wait = taildropDoubleCopyPoll
			count, _ := platform.ClipboardChangeCount()
			if haveCount && count != lastCount {
				now := time.Now()
				delta := count - lastCount
				text, concealed := readClipboardForGesture()
				switch {
				case concealed || text == "":
					lastText = ""
				case delta >= 2 || (text == lastText && now.Sub(lastAt) <= taildropDoubleCopyWindow):
					// Оба копирования между опросами — счётчик прыгнул на 2+.
					lastText = ""
					go ac.sendTaildropDoubleCopy(targets, text)
				default:
					lastText, lastAt = text, now
				}
			}
			lastCount, haveCount = count, true
		} else {
			haveCount = false
			lastText = ""
		}
		select {
		case <-ac.ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// readClipboardForGesture — текст буфера и пометка чувствительности.
func readClipboardForGesture() (string, bool) {
	if platform.ClipboardConcealed() {
		return "", true
	}
	app := fyne.CurrentApp()
	if app == nil {
		return "", false
	}
	var text string
	fyne.DoAndWait(func() {
		if cb := app.Clipboard(); cb != nil {
			text = cb.Content()
		}
	})
	return text, false
}

// sendTaildropDoubleCopy — текст на выбранное устройство каждого узла, если оно
// сейчас в сети и принимает файлы; итог — уведомлением.
func (ac *AppController) sendTaildropDoubleCopy(targets []taildropDoubleCopyTarget, text string) {
	local := TailscaleTarget{Scope: services.ScopeLocal}
	for _, t := range targets {
		name := t.PeerName
		online := false
		if st, ok := ac.TailscaleStatus(local, t.Tag); ok {
			for _, g := range st.UserGroups {
				for _, p := range g.Peers {
					if p.StableID == t.PeerID {
						name = p.HostName
						online = p.Online && p.CanReceiveFiles
					}
				}
			}
		}
		if !online {
			taildropNotify(locale.T("Not sent"), locale.Tf("%s is offline or does not accept files.", name))
			continue
		}
		ctx, cancel := context.WithTimeout(ac.ctx, 30*time.Second)
		err := ac.TaildropSendText(ctx, local, t.Tag, t.PeerID,
			services.TaildropServiceCommand{Kind: services.TaildropServiceClipboard, Text: text})
		cancel()
		if err != nil {
			debuglog.WarnLog("taildrop %s: double copy not sent", t.Tag)
			taildropNotify(locale.T("Not sent"), locale.Tf("Clipboard to %s: %v", name, err))
			continue
		}
		taildropNotify(locale.T("Clipboard sent"), locale.Tf("Copied text sent to %s.", name))
	}
}
