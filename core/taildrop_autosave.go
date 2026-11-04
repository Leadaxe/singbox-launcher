// File taildrop_autosave.go — сохранение файлов Taildrop в ~/Downloads
// (SPEC 175): по кнопке Save и автоматически, если узел этого не выключил. Тот же фоновый поток разбирает служебные
// файлы своего ядра и уведомляет о принятых (SPEC 176).
//
// Подписка на входящие узла живёт всё время, пока ядро его может обслуживать, —
// не только при открытом окне. Настройка автосохранения — поле записи узла
// (SPEC 179). Имена файлов и устройств в журнал не пишутся (SPEC 148 §3).
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

// taildropAutoSaver — фоновые подписки на входящие узлов своего ядра:
// служебные файлы, автосохранение и уведомление о принятых (SPEC 176, 179).
// Одна подписка на финальный тег. Узлы удалённых машин не подписываются:
// их файлы забирают кнопкой Save в окне узла.
type taildropAutoSaver struct {
	mu     sync.Mutex
	active map[string]*taildropAutoSaveSub
}

// taildropAutoSaveSub — поднятая подписка; указатель отличает её от
// следующей подписки того же узла.
type taildropAutoSaveSub struct {
	cancel context.CancelFunc
	// autoSave — обычные файлы в ~/Downloads (галка узла).
	autoSave bool
}

// taildropAutoSaveTick — как часто сверяются узлы работающего ядра.
const taildropAutoSaveTick = 5 * time.Second

// startTaildropAutoSave — цикл сверки: у узла своего ядра с Taildrop подписка
// поднята, у остальных — снята.
func (ac *AppController) startTaildropAutoSave() {
	s := &ac.taildropAutoSave
	s.mu.Lock()
	s.active = map[string]*taildropAutoSaveSub{}
	s.mu.Unlock()
	t := time.NewTicker(taildropAutoSaveTick)
	defer t.Stop()
	for {
		ac.reconcileTaildropAutoSave()
		select {
		case <-ac.ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (ac *AppController) reconcileTaildropAutoSave() {
	s := &ac.taildropAutoSave
	want := map[string]bool{}
	local := TailscaleTarget{Scope: services.ScopeLocal}
	for _, tag := range ac.taildropServiceTags() {
		record := ac.taildropNodeRecord(local, tag)
		want[tag] = record == nil || !record.DisableTaildropAutosave
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for tag, sub := range s.active {
		if autoSave, ok := want[tag]; !ok || autoSave != sub.autoSave {
			sub.cancel()
			delete(s.active, tag)
		}
	}
	for tag, autoSave := range want {
		if _, ok := s.active[tag]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(ac.ctx)
		sub := &taildropAutoSaveSub{cancel: cancel, autoSave: autoSave}
		s.active[tag] = sub
		go ac.runTaildropAutoSave(ctx, tag, sub)
	}
}

// taildropWatchState — что подписка уже знает о файлах входящих. Чистится
// по составу последнего кадра.
type taildropWatchState struct {
	// handled — файлы, уже взятые в работу (сохранён, разобран, показан в
	// уведомлении): второй раз не трогаем, даже если не ушёл из входящих.
	handled map[string]bool
	// regular — кандидаты в служебные, оказавшиеся обычными файлами.
	regular map[string]bool
	// seen — первый кадр подписки обработан: файлы, лежавшие до неё, не
	// объявляются уведомлением «принят».
	seen bool
}

// runTaildropAutoSave — подписка одного узла. Обрыв — снятие из active,
// следующая сверка поднимет её заново.
func (ac *AppController) runTaildropAutoSave(ctx context.Context, tag string, sub *taildropAutoSaveSub) {
	target := TailscaleTarget{Scope: services.ScopeLocal}
	frames := make(chan services.TaildropInbox, 1)
	go func() {
		st := &taildropWatchState{handled: map[string]bool{}, regular: map[string]bool{}}
		for {
			var in services.TaildropInbox
			select {
			case <-ctx.Done():
				return
			case in = <-frames:
			}
			ac.handleTaildropFrame(ctx, target, tag, sub.autoSave, in, st)
		}
	}()
	err := ac.TaildropSubscribe(ctx, target, tag, func(in services.TaildropInbox) {
		// Кадр новее заменяет недоработанный старый: важен состав входящих.
		select {
		case <-frames:
		default:
		}
		frames <- in
	})
	if err != nil {
		debuglog.DebugLog("taildrop %s: inbox subscription ended: %v", tag, err)
	}
	s := &ac.taildropAutoSave
	sub.cancel()
	s.mu.Lock()
	if s.active[tag] == sub {
		delete(s.active, tag)
	}
	s.mu.Unlock()
}

// handleTaildropFrame — один кадр входящих: сначала служебные файлы (§6
// SERVICE_FILES: до автосохранения и уведомления), затем обычные —
// в Downloads или уведомлением.
func (ac *AppController) handleTaildropFrame(ctx context.Context, t TailscaleTarget, tag string, autoSave bool, in services.TaildropInbox, st *taildropWatchState) {
	present := map[string]bool{}
	for _, f := range in.Files {
		present[f.Name] = true
	}
	for name := range st.handled {
		if !present[name] {
			delete(st.handled, name)
		}
	}
	for name := range st.regular {
		if !present[name] {
			delete(st.regular, name)
		}
	}
	first := !st.seen
	st.seen = true

	rules, rulesErr := services.TaildropServiceRulesFromRegistry()
	var items []taildropServiceItem
	var regular []services.TaildropFile
	for _, f := range in.Files {
		if st.handled[f.Name] || ctx.Err() != nil {
			continue
		}
		if rulesErr == nil && !st.regular[f.Name] && rules.TaildropServiceCandidate(f.Name, f.Size) {
			cmds, ok, err := ac.readTaildropService(ctx, t, tag, rules, f)
			if err != nil {
				// Не скачался — следующий кадр попробует снова.
				debuglog.DebugLog("taildrop %s: service candidate not read: %v", tag, err)
				continue
			}
			if ok {
				st.handled[f.Name] = true
				items = append(items, taildropServiceItem{file: f, cmds: cmds})
				continue
			}
			st.regular[f.Name] = true
		}
		regular = append(regular, f)
	}
	ac.runTaildropServices(t, tag, rules, items)

	for _, f := range regular {
		if ctx.Err() != nil {
			return
		}
		st.handled[f.Name] = true
		if autoSave {
			path, err := ac.TaildropSaveToDownloads(ctx, t, tag, f.Name)
			if err != nil {
				continue
			}
			debuglog.InfoLog("taildrop %s: received file saved to Downloads (%d bytes)", tag, f.Size)
			taildropNotify(locale.T("File received"), locale.Tf("%s saved to Downloads.", filepath.Base(path)))
			continue
		}
		if !first {
			from := f.SenderName
			if from == "" {
				from = locale.T("unknown device")
			}
			taildropNotify(locale.T("File received"), locale.Tf("%s from %s. Open the node window to save it.", f.Name, from))
		}
	}
}
