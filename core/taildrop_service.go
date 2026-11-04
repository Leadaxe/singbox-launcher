// File taildrop_service.go — служебные файлы Taildrop на этом компьютере
// (SPEC 176, 179): настройки из записи узла, разбор кадра входящих своего ядра,
// выполнение clipboard / open_url, отправка текста устройству.
//
// Только своё ядро: команда служебного файла — про буфер и браузер ЭТОГО
// компьютера. Текст буфера, ссылки, имена файлов и устройств в журнал не
// пишутся (SPEC 148 §3).
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
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

// taildropStateCache — состояние профилей по времени и размеру файла: настройки
// Taildrop читаются часто, а файл меняется редко.
type taildropStateCache struct {
	mu     sync.Mutex
	byPath map[string]taildropCachedState
}

type taildropCachedState struct {
	mod  time.Time
	size int64
	st   *state.State
}

// taildropProfile — состояние профиля (machineID "" — Local) только для чтения;
// nil, если его нет или оно не читается.
func (ac *AppController) taildropProfile(machineID string) *state.State {
	path := tailscaleExitStatePath(ac.FileService.Layout.Data, machineID)
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	c := &ac.taildropStates
	c.mu.Lock()
	defer c.mu.Unlock()
	if got, ok := c.byPath[path]; ok && got.mod.Equal(info.ModTime()) && got.size == info.Size() {
		return got.st
	}
	st, err := state.Load(path)
	if err != nil {
		debuglog.DebugLog("taildrop: profile state not read: %v", err)
		return nil
	}
	if c.byPath == nil {
		c.byPath = map[string]taildropCachedState{}
	}
	c.byPath[path] = taildropCachedState{mod: info.ModTime(), size: info.Size(), st: st}
	return st
}

// taildropNodeRecord — запись своего узла цели; nil у узла подписки и когда
// состояние недоступно: действуют умолчания (SERVICE_FILES §5).
func (ac *AppController) taildropNodeRecord(t TailscaleTarget, tag string) *state.Node {
	machineID := ""
	if t.Scope == services.ScopeRemote {
		machineID = t.MachineID
	}
	return findOwnTailscaleNode(ac.taildropProfile(machineID), tag)
}

// taildropAllowedBody — на узле разрешён Taildrop: явное `disable_taildrop: false`.
func taildropAllowedBody(body []byte) bool {
	var head struct {
		Disable *bool `json:"disable_taildrop"`
	}
	return json.Unmarshal(body, &head) == nil && head.Disable != nil && !*head.Disable
}

// taildropSenderIsOwn — отправитель своё устройство (SERVICE_FILES §5). Ядро
// пропускает и пира с правом file-send, поэтому sharee отсекается здесь. Нет
// senderID или пир не найден — не своё.
func (ac *AppController) taildropSenderIsOwn(t TailscaleTarget, tag, senderID string) bool {
	if senderID == "" {
		return false
	}
	st, ok := ac.TailscaleStatus(t, tag)
	if !ok {
		return false
	}
	for _, g := range st.UserGroups {
		for _, p := range g.Peers {
			if p.StableID == senderID {
				return !p.ShareeNode
			}
		}
	}
	return false
}

// TaildropDoubleCopyChoices — устройства для выбора в настройках узла: свои
// устройства работающего узла своего ядра. Sharee не показываются.
func (ac *AppController) TaildropDoubleCopyChoices(tag string) []state.TaildropPeer {
	st, ok := ac.TailscaleStatus(TailscaleTarget{Scope: services.ScopeLocal}, tag)
	if !ok {
		return nil
	}
	var out []state.TaildropPeer
	for _, g := range st.UserGroups {
		for _, p := range g.Peers {
			if p.StableID != "" && !p.ShareeNode {
				out = append(out, state.TaildropPeer{PeerID: p.StableID, PeerName: p.HostName})
			}
		}
	}
	return out
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
	record := ac.taildropNodeRecord(t, tag)
	commandsOff := record != nil && record.DisableTaildropCommands
	now := time.Now()
	clip, clipFrom, haveClip := "", "", false
	ran, skipped := 0, 0
	// Отказы показываем после разбора, по одному на устройство и вид команды:
	// без них буфер или ссылка «не доходят» без объяснений. Текст и адрес в
	// уведомление не идут.
	type taildropDenied struct{ title, body string }
	var denied []taildropDenied
	deny := func(title, body string) {
		d := taildropDenied{title, body}
		for _, seen := range denied {
			if seen == d {
				return
			}
		}
		denied = append(denied, d)
	}
	for _, it := range items {
		f := it.file
		if f.ModifiedAt.IsZero() || now.Sub(f.ModifiedAt) > rules.TTL {
			skipped += len(it.cmds)
			continue
		}
		own := ac.taildropSenderIsOwn(t, tag, f.SenderID)
		from := f.SenderName
		if from == "" {
			from = locale.T("unknown device")
		}
		for _, c := range it.cmds {
			switch c.Kind {
			case services.TaildropServiceClipboard:
				text, ok := c.ClipboardText()
				if !ok {
					skipped++
					continue
				}
				if commandsOff || !own {
					skipped++
					if commandsOff && own {
						deny(locale.T("Clipboard not accepted"), locale.Tf("%s sent text for the clipboard. To accept it, turn on Accept clipboard and links in the Taildrop settings of the node.", from))
					}
					continue
				}
				clip, clipFrom, haveClip = text, from, true
				ran++
			case services.TaildropServiceOpenURL:
				if !rules.OpenURLAllowed(c.URL) {
					skipped++
					continue
				}
				if commandsOff || !own {
					skipped++
					if commandsOff && own {
						deny(locale.T("Link not opened"), locale.Tf("%s sent a link. To accept it, turn on Accept clipboard and links in the Taildrop settings of the node.", from))
					}
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
	for _, d := range denied {
		taildropNotify(d.title, d.body)
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
