package ui

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"singbox-launcher/core/services"
)

// localConfigWithGroups кладёт config.json с двумя selector-группами.
func localConfigWithGroups(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{
	  "route": {"final": "vpn"},
	  "outbounds": [
	    {"tag": "vpn", "type": "selector", "outbounds": ["a"]},
	    {"tag": "media", "type": "selector", "outbounds": ["a"]},
	    {"tag": "a", "type": "direct"}
	  ]
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("подготовка config.json: %v", err)
	}
	return path
}

// stubRemoteGroups подменяет источник групп машины на время теста.
func stubRemoteGroups(t *testing.T, f func() ([]string, bool, error)) {
	t.Helper()
	prev := remoteGroupsSource
	remoteGroupsSource = f
	t.Cleanup(func() { remoteGroupsSource = prev })
}

// SPEC 113-E (M5): у Local-панели источник групп ровно один — локальный
// config.json. Раньше RemoteDaemonGroups() спрашивали ДО проверки области, и
// при подключённой машине Local получала группы РОУТЕРА (повтор класса
// «remote-override глобальный»). Новый подписчик ConfigBuilt сделал эту ветку
// достижимой для локальных пересборок.
func TestCollectSelectorGroups_LocalIgnoresConnectedMachine(t *testing.T) {
	cfg := localConfigWithGroups(t)
	asked := false
	stubRemoteGroups(t, func() ([]string, bool, error) {
		asked = true
		return []string{"router-vpn", "router-media"}, true, nil
	})

	snap := collectSelectorGroups(services.ScopeLocal, cfg)

	if asked {
		t.Error("Local-панель спросила группы у машины — это чужое ядро")
	}
	if snap.clearAll {
		t.Error("Local-панель не должна очищать дропдаун")
	}
	if len(snap.options) != 2 || snap.options[0] != "vpn" || snap.options[1] != "media" {
		t.Fatalf("группы = %v, ожидались локальные [vpn media]", snap.options)
	}
	if snap.defaultGroup != "vpn" {
		t.Errorf("группа по умолчанию = %q, ожидалась vpn из route.final", snap.defaultGroup)
	}
}

// Remote-панель, наоборот, обязана брать группы ТОЛЬКО у машины: локальный
// config.json описывает другое ядро.
func TestCollectSelectorGroups_RemoteAsksMachine(t *testing.T) {
	cfg := localConfigWithGroups(t)
	stubRemoteGroups(t, func() ([]string, bool, error) {
		return []string{"router-vpn"}, true, nil
	})

	snap := collectSelectorGroups(services.ScopeRemote, cfg)
	if len(snap.options) != 1 || snap.options[0] != "router-vpn" {
		t.Fatalf("группы = %v, ожидались группы машины", snap.options)
	}
	if snap.defaultGroup != "router-vpn" {
		t.Errorf("группа по умолчанию = %q", snap.defaultGroup)
	}
}

// Машина недоступна: список пуст, но локальные группы подставлять нельзя —
// это выдало бы чужой список за список выбранной машины.
func TestCollectSelectorGroups_RemoteErrorDoesNotFallBackToLocal(t *testing.T) {
	cfg := localConfigWithGroups(t)
	stubRemoteGroups(t, func() ([]string, bool, error) {
		return nil, true, errTestUnreachable
	})

	snap := collectSelectorGroups(services.ScopeRemote, cfg)
	if len(snap.options) != 0 {
		t.Fatalf("группы = %v, ожидался пустой список", snap.options)
	}
	if snap.clearAll {
		t.Error("машина выбрана — дропдаун чистить не за что, просто нечего показать")
	}
}

// Remote без выбранной машины: собеседника нет, и дропдаун обязан опустеть.
func TestCollectSelectorGroups_RemoteWithoutMachineClears(t *testing.T) {
	cfg := localConfigWithGroups(t)
	stubRemoteGroups(t, func() ([]string, bool, error) { return nil, false, nil })

	snap := collectSelectorGroups(services.ScopeRemote, cfg)
	if !snap.clearAll {
		t.Fatal("без выбранной машины дропдаун обязан очиститься")
	}
	if len(snap.options) != 0 {
		t.Errorf("группы = %v, ожидался пустой список", snap.options)
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }

const errTestUnreachable = testErr("no route to host")

// SPEC 113-E (M4), регресс: Request не ждёт сбора.
//
// На старом коде сбор шёл в потоке вызова, а из пяти путей вызова четыре идут
// с UI-потока — недоступный роутер держал главный цикл на весь dial-дедлайн.
func TestSelectorReloader_RequestDoesNotBlock(t *testing.T) {
	release := make(chan struct{})
	r := &selectorReloader{
		collect: func() selectorGroupsSnapshot {
			<-release
			return selectorGroupsSnapshot{}
		},
	}
	defer close(release)

	done := make(chan struct{})
	go func() {
		r.Request()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Request ждал сбор данных — это и есть заморозка UI-потока")
	}
}

// Повторные вызовы во время сбора схлопываются: Reset, клик по ↻ и колбэк
// успешного теста связи приходят пачкой, и без схлопывания панель завела бы
// три параллельных запроса к машине.
func TestSelectorReloader_CoalescesWhileRunning(t *testing.T) {
	var calls int32
	gate := make(chan struct{})
	var applied int32
	done := make(chan struct{})
	var once sync.Once

	r := &selectorReloader{}
	r.collect = func() selectorGroupsSnapshot {
		if atomic.AddInt32(&calls, 1) == 1 {
			<-gate // держим первый сбор, пока копятся повторные запросы
		}
		return selectorGroupsSnapshot{options: []string{"vpn"}}
	}
	r.apply = func(selectorGroupsSnapshot) {
		if atomic.AddInt32(&applied, 2) >= 4 {
			once.Do(func() { close(done) })
		}
	}

	r.Request()
	// Ждём, пока первый сбор реально начался — иначе последующие запросы
	// стали бы отдельными прогонами, и тест проверял бы не то.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&calls) == 0 {
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < 4; i++ {
		r.Request()
	}
	close(gate)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("повторный прогон после схлопывания не состоялся")
	}
	// Пять запросов дали ровно два прогона: текущий и один догоняющий.
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("прогонов сбора = %d, ожидалось 2 (текущий + догоняющий)", got)
	}
}
