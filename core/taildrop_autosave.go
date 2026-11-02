// File taildrop_autosave.go — сохранение файлов Taildrop в ~/Downloads
// (SPEC 175): по кнопке Save и автоматически у узлов с галкой «Save to
// Downloads automatically» (§3.4).
//
// Автосохранение держит подписку на входящие узла всё время, пока ядро его
// машины работает, — не только при открытом окне. Галка — настройка этого
// компьютера (settings.json), в бэкап не идёт. Имена файлов и устройств в
// журнал не пишутся (SPEC 148 §3).
package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// TaildropSaveToDownloads скачивает файл входящих в ~/Downloads под
// свободным именем и убирает его из входящих; возвращает путь. Недокачанный
// файл удаляется. Не удалось убрать из входящих — файл всё равно сохранён.
func (ac *AppController) TaildropSaveToDownloads(ctx context.Context, t TailscaleTarget, tag, name string) (string, error) {
	dir, err := DownloadsDir()
	if err != nil {
		return "", err
	}
	out, path, err := createFreeFile(dir, name)
	if err != nil {
		return "", err
	}
	_, err = ac.TaildropDownload(ctx, t, tag, name, out)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path)
		debuglog.WarnLog("taildrop %s: file not saved", tag)
		return "", err
	}
	if err := ac.TaildropDelete(t, tag, name); err != nil {
		debuglog.WarnLog("taildrop %s: saved file not removed from inbox", tag)
	}
	return path, nil
}

// createFreeFile создаёт файл в dir: имя как пришло, при занятом —
// «name (2).ext», «name (3).ext»… Имя — только последний сегмент: путь
// приходит от чужого устройства.
func createFreeFile(dir, name string) (*os.File, string, error) {
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if base == "." || base == ".." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		base = "taildrop-file"
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; i <= 1000; i++ {
		candidate := base
		if i > 1 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		path := filepath.Join(dir, candidate)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return f, path, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("no free file name for %s in %s", base, dir)
}

func taildropAutoSaveEntry(t TailscaleTarget, tag string) locale.TaildropAutoSaveEntry {
	e := locale.TaildropAutoSaveEntry{Tag: tag}
	if t.Scope == services.ScopeRemote {
		e.MachineID = t.MachineID
	}
	return e
}

// TaildropAutoSaveEnabled — стоит ли галка автосохранения у узла.
func (ac *AppController) TaildropAutoSaveEnabled(t TailscaleTarget, tag string) bool {
	want := taildropAutoSaveEntry(t, tag)
	for _, e := range locale.LoadSettings(ac.FileService.Layout.Data.Bin()).TaildropAutoSave {
		if e == want {
			return true
		}
	}
	return false
}

// SetTaildropAutoSave ставит или снимает галку и сразу применяет её.
func (ac *AppController) SetTaildropAutoSave(t TailscaleTarget, tag string, on bool) error {
	binDir := ac.FileService.Layout.Data.Bin()
	st := locale.LoadSettings(binDir)
	want := taildropAutoSaveEntry(t, tag)
	kept := st.TaildropAutoSave[:0]
	for _, e := range st.TaildropAutoSave {
		if e != want {
			kept = append(kept, e)
		}
	}
	if on {
		kept = append(kept, want)
	}
	st.TaildropAutoSave = kept
	if err := locale.SaveSettings(binDir, st); err != nil {
		return err
	}
	ac.taildropAutoSave.kick()
	return nil
}

// taildropAutoSaver — подписки автосохранения по узлам.
type taildropAutoSaver struct {
	mu     sync.Mutex
	active map[locale.TaildropAutoSaveEntry]*taildropAutoSaveSub
	wake   chan struct{}
}

// taildropAutoSaveSub — поднятая подписка; указатель отличает её от
// следующей подписки того же узла.
type taildropAutoSaveSub struct {
	cancel context.CancelFunc
}

func (s *taildropAutoSaver) kick() {
	if s.wake == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// taildropAutoSaveTick — как часто сверяются галки с работающими ядрами.
const taildropAutoSaveTick = 5 * time.Second

// startTaildropAutoSave — цикл сверки: у узла с галкой и работающим ядром
// с Taildrop подписка поднята, у остальных — снята.
func (ac *AppController) startTaildropAutoSave() {
	s := &ac.taildropAutoSave
	s.mu.Lock()
	s.active = map[locale.TaildropAutoSaveEntry]*taildropAutoSaveSub{}
	s.wake = make(chan struct{}, 1)
	s.mu.Unlock()
	t := time.NewTicker(taildropAutoSaveTick)
	defer t.Stop()
	for {
		ac.reconcileTaildropAutoSave()
		select {
		case <-ac.ctx.Done():
			return
		case <-t.C:
		case <-s.wake:
		}
	}
}

func (ac *AppController) reconcileTaildropAutoSave() {
	s := &ac.taildropAutoSave
	want := map[locale.TaildropAutoSaveEntry]bool{}
	for _, e := range locale.LoadSettings(ac.FileService.Layout.Data.Bin()).TaildropAutoSave {
		target := TailscaleTarget{Scope: services.ScopeLocal}
		if e.MachineID != "" {
			target = TailscaleTarget{Scope: services.ScopeRemote, MachineID: e.MachineID}
		}
		if ac.TaildropAvailable(target) && ac.TailscaleCoreRunning(target) {
			want[e] = true
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for e, sub := range s.active {
		if !want[e] {
			sub.cancel()
			delete(s.active, e)
		}
	}
	for e := range want {
		if _, ok := s.active[e]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(ac.ctx)
		sub := &taildropAutoSaveSub{cancel: cancel}
		s.active[e] = sub
		go ac.runTaildropAutoSave(ctx, e, sub)
	}
}

// runTaildropAutoSave — подписка одного узла: принятые файлы по одному
// уходят в ~/Downloads. Обрыв подписки — снятие из active, следующая сверка
// поднимет её заново.
func (ac *AppController) runTaildropAutoSave(ctx context.Context, e locale.TaildropAutoSaveEntry, sub *taildropAutoSaveSub) {
	target := TailscaleTarget{Scope: services.ScopeLocal}
	if e.MachineID != "" {
		target = TailscaleTarget{Scope: services.ScopeRemote, MachineID: e.MachineID}
	}
	frames := make(chan services.TaildropInbox, 1)
	go func() {
		// handled — файлы, уже взятые в работу: не удалось убрать из входящих
		// — второй раз не качаем. Чистится по составу последнего кадра.
		handled := map[string]bool{}
		for {
			var in services.TaildropInbox
			select {
			case <-ctx.Done():
				return
			case in = <-frames:
			}
			present := map[string]bool{}
			for _, f := range in.Files {
				present[f.Name] = true
				if handled[f.Name] || ctx.Err() != nil {
					continue
				}
				handled[f.Name] = true
				path, err := ac.TaildropSaveToDownloads(ctx, target, e.Tag, f.Name)
				if err != nil {
					continue
				}
				debuglog.InfoLog("taildrop %s: received file saved to Downloads (%d bytes)", e.Tag, f.Size)
				platform.SendNotification(locale.T("File received"), locale.Tf("%s saved to Downloads.", filepath.Base(path)))
			}
			for name := range handled {
				if !present[name] {
					delete(handled, name)
				}
			}
		}
	}()
	err := ac.TaildropSubscribe(ctx, target, e.Tag, func(in services.TaildropInbox) {
		// Кадр новее заменяет недоработанный старый: важен состав входящих.
		select {
		case <-frames:
		default:
		}
		frames <- in
	})
	if err != nil {
		debuglog.DebugLog("taildrop %s: auto-save subscription ended: %v", e.Tag, err)
	}
	s := &ac.taildropAutoSave
	sub.cancel()
	s.mu.Lock()
	if s.active[e] == sub {
		delete(s.active, e)
	}
	s.mu.Unlock()
}
