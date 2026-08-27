//go:build darwin

package netiface

import (
	"os/exec"
	"strings"
	"sync"
)

// friendlyNames кэширует разбор `networksetup -listnetworkserviceorder`.
// Вызов внешнего процесса на каждый интерфейс в списке означал бы N запусков
// подпроцесса при каждой отрисовке вкладки настроек.
//
// SPEC 113-E (M6): загрузка НИКОГДА не блокирует вызывающего. `networksetup` —
// подпроцесс, и первый его запуск занимал заметное время прямо в потоке
// отрисовки вкладки Settings. Пока ответа нет, List() отдаёт интерфейсы без
// человеческих имён (системное имя всё равно первое в подписи и достаточно для
// выбора), а Warm() поднимает кэш заранее — из горутины.
var (
	friendlyMu      sync.Mutex
	friendlyCache   map[string]string
	friendlyLoaded  bool
	friendlyLoading bool
	// friendlyOnLoaded — кого разбудить, когда имена приехали. Ставит тот, кто
	// умеет перерисовать поле; вызывается из фоновой горутины.
	friendlyOnLoaded func()
)

// SetFriendlyNamesLoadedHook сообщает, кому перерисовать подписи интерфейсов,
// когда `networksetup` наконец ответил.
//
// Колбэк приходит из фоновой горутины: уводить его на UI-поток — забота
// вызывающего, этот пакет о потоках отрисовки ничего не знает.
func SetFriendlyNamesLoadedHook(f func()) {
	friendlyMu.Lock()
	defer friendlyMu.Unlock()
	friendlyOnLoaded = f
}

// Warm поднимает кэш человеческих имён в фоне. Идемпотентна: пока загрузка
// идёт, повторные вызовы ничего не запускают.
func Warm() {
	friendlyMu.Lock()
	if friendlyLoaded || friendlyLoading {
		friendlyMu.Unlock()
		return
	}
	friendlyLoading = true
	friendlyMu.Unlock()

	go func() {
		loaded := loadFriendlyNames()
		friendlyMu.Lock()
		friendlyCache = loaded
		friendlyLoaded = true
		friendlyLoading = false
		notify := friendlyOnLoaded
		friendlyMu.Unlock()
		if notify != nil {
			notify()
		}
	}()
}

func loadFriendlyNames() map[string]string {
	out := map[string]string{}
	raw, err := exec.Command("networksetup", "-listnetworkserviceorder").Output()
	if err != nil {
		return out
	}
	// Строки вида: «(Hardware Port: Wi-Fi, Device: en0)».
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "(Hardware Port:") {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(line, "(Hardware Port:"), ")")
		parts := strings.SplitN(body, ", Device:", 2)
		if len(parts) != 2 {
			continue
		}
		port, dev := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if dev != "" && port != "" {
			out[dev] = port
		}
	}
	return out
}

// friendlyName отдаёт человеческое имя сервиса, если оно уже прочитано, и ""
// пока нет — заодно заводя чтение. Ждать здесь нельзя: функция зовётся из
// List(), а List() зовут из потока отрисовки.
func friendlyName(dev string) string {
	friendlyMu.Lock()
	if friendlyLoaded {
		name := friendlyCache[dev]
		friendlyMu.Unlock()
		return name
	}
	friendlyMu.Unlock()
	Warm()
	return ""
}
