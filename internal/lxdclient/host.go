package lxdclient

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Телеметрия ХОСТА машины: GET /admin/host и GET /admin/host/interfaces
// (SPEC 068 форка, docs-lx/lxd-daemon-ru.md §10b).
//
// Отличие от /admin/status и /admin/memory: те описывают ядро и процесс
// демона, а эти две ручки — саму машину. Когда роутер начинает тормозить,
// вопрос «упёрлось ли железо и во что» по процессу не решается: демон может
// быть в полном порядке при забитом overlay и ядре в термотротлинге.
//
// Форма ответа одинакова на всех платформах, а недоступное поле приезжает как
// null. Поэтому здесь ВЕЗДЕ указатели на то, что демон вправе не измерить:
// разница между «ноль» и «не знаем» в этих числах смысловая. Клиент проверяет
// nil, а не разбирает поле OS.

// HostInfo — снимок машины целиком.
type HostInfo struct {
	Model         string `json:"model"`
	OS            string `json:"os"`
	Kernel        string `json:"kernel"`
	Arch          string `json:"arch"`
	UptimeSeconds int64  `json:"uptime_seconds"`

	CPU     HostCPU      `json:"cpu"`
	Memory  HostMemory   `json:"memory"`
	Thermal *HostThermal `json:"thermal"`
	Disk    HostDisk     `json:"disk"`
	FD      HostFD       `json:"fd"`

	UpdatedUnix int64 `json:"updated_unix"`
}

// HostCPU — загрузка процессора и load average.
//
// UsagePercent и PerCorePercent — дельта между двумя чтениями счётчиков, и до
// второго замера их нет вовсе. Именно nil, а не 0: ноль читался бы как
// «простаивает», а это другое утверждение. IntervalSeconds говорит, за какое
// окно посчитан процент — 12.4% за пять секунд и за час значат разное.
type HostCPU struct {
	Cores           int       `json:"cores"`
	UsagePercent    *float64  `json:"usage_percent"`
	PerCorePercent  []float64 `json:"per_core_percent"`
	IntervalSeconds float64   `json:"interval_seconds"`
	// Load* достаются от ядра готовыми, поэтому есть уже в первом ответе —
	// единственные числа этой секции, которым второй замер не нужен.
	Load1  *float64 `json:"load_1"`
	Load5  *float64 `json:"load_5"`
	Load15 *float64 `json:"load_15"`
}

// HostMemory — память МАШИНЫ.
//
// UsedPercent демон считает от AvailableBytes, а не от FreeBytes: роутер
// держит почти всю память в page cache, и процент от free кричал бы «занято»
// при реально свободных 120 МБ.
type HostMemory struct {
	TotalBytes     uint64   `json:"total_bytes"`
	AvailableBytes uint64   `json:"available_bytes"`
	FreeBytes      uint64   `json:"free_bytes"`
	BuffersBytes   *uint64  `json:"buffers_bytes"`
	CachedBytes    *uint64  `json:"cached_bytes"`
	UsedPercent    *float64 `json:"used_percent"`
	SwapTotalBytes uint64   `json:"swap_total_bytes"`
	SwapFreeBytes  uint64   `json:"swap_free_bytes"`
}

// HostThermal — все датчики платы разом плюс максимум для одного индикатора.
//
// В HostInfo это указатель целиком: «датчиков нет» (виртуалка, контейнер,
// macOS без CGO) приезжает как null, а не пустым массивом — пустой массив
// читался бы как «датчики есть и промолчали».
type HostThermal struct {
	Zones      []HostThermalZone `json:"zones"`
	MaxCelsius float64           `json:"max_celsius"`
}

// HostThermalZone — один датчик.
type HostThermalZone struct {
	Name    string  `json:"name"`
	Celsius float64 `json:"celsius"`
}

// HostDisk — точки монтирования машины.
type HostDisk struct {
	Mounts       []HostMount `json:"mounts"`
	StateDirPath string      `json:"state_dir_path"`
	// MaxUsedPercent игнорирует read-only ФС: squashfs-корень на OpenWrt вечно
	// занят на 100%, а всегда красный индикатор перестают замечать. Полную
	// картину сохраняет флаг ReadOnly у каждой точки.
	MaxUsedPercent *float64 `json:"max_used_percent"`
}

// HostMount — одна файловая система.
type HostMount struct {
	Path           string  `json:"path"`
	FSType         string  `json:"fstype"`
	TotalBytes     uint64  `json:"total_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedPercent    float64 `json:"used_percent"`
	ReadOnly       bool    `json:"read_only"`
	// HoldsStateDir помечает ФС, на которой лежит state-dir: именно её
	// переполнение ломает apply. Стоит не более чем у одной точки.
	HoldsStateDir bool `json:"holds_state_dir,omitempty"`
}

// HostFD — дескрипторы в двух уровнях: свои у демона и системные.
//
// Оба упираются в потолок с общим симптомом «новые соединения не
// открываются», но это разные баги: демон на своём лимите при полупустой
// системе — не то же, что кончились системные.
type HostFD struct {
	Open        *int `json:"open"`
	Limit       *int `json:"limit"`
	SystemOpen  *int `json:"system_open"`
	SystemLimit *int `json:"system_limit"`
}

// HostInterfaces — ответ GET /admin/host/interfaces.
type HostInterfaces struct {
	Interfaces []HostInterface `json:"interfaces"`
	// IntervalSeconds общий на весь ответ: интерфейсы снимаются одним
	// проходом, окно дельты у них одно на всех.
	IntervalSeconds float64 `json:"interval_seconds"`
	UpdatedUnix     int64   `json:"updated_unix"`
}

// HostInterface — один интерфейс: сырые счётчики И производные скорости.
//
// Оба сразу намеренно: счётчик переживает рестарты и разрывы и годится под
// график, скорость удобна глазу, но на разрывах врёт.
type HostInterface struct {
	Name string `json:"name"`
	Up   bool   `json:"up"`
	// Mac пуст у туннеля, Addresses пусты у радиоинтерфейса в мосте — оба
	// состояния законные, а не ошибка.
	Mac       string   `json:"mac"`
	Addresses []string `json:"addresses"`
	MTU       int      `json:"mtu"`

	RxBytes   uint64 `json:"rx_bytes"`
	TxBytes   uint64 `json:"tx_bytes"`
	RxPackets uint64 `json:"rx_packets"`
	TxPackets uint64 `json:"tx_packets"`
	RxErrors  uint64 `json:"rx_errors"`
	TxErrors  uint64 `json:"tx_errors"`
	RxDropped uint64 `json:"rx_dropped"`
	TxDropped uint64 `json:"tx_dropped"`

	RxBytesPerSecond *float64 `json:"rx_bytes_per_second"`
	TxBytesPerSecond *float64 `json:"tx_bytes_per_second"`
}

// ErrHostUnsupported — демон машины старше телеметрии хоста (нет ручки).
//
// Отдельная ошибка, а не просто текст: 404 здесь значит «машину видно, версия
// старая» — сообщение про обновление демона, а не про обрыв связи.
var ErrHostUnsupported = fmt.Errorf("lxdclient: host telemetry not supported by this daemon")

// Host возвращает снимок машины (GET /admin/host).
//
// Проценты появятся со ВТОРОГО вызова: демон считает их как дельту между
// замерами и до накопления пина честно отдаёт null.
func (c *Client) Host() (HostInfo, error) {
	var out HostInfo
	if err := c.getHostJSON("/admin/host", &out); err != nil {
		return HostInfo{}, err
	}
	return out, nil
}

// HostInterfaces возвращает все интерфейсы машины (GET /admin/host/interfaces).
//
// Отдаются ВСЕ, включая lo и лежачие: «wan лёг» — ровно то, что нужно
// увидеть. Фильтрация — задача UI.
func (c *Client) HostInterfaces() (HostInterfaces, error) {
	var out HostInterfaces
	if err := c.getHostJSON("/admin/host/interfaces", &out); err != nil {
		return HostInterfaces{}, err
	}
	return out, nil
}

// getHostJSON — общий GET+разбор для обеих ручек телеметрии.
func (c *Client) getHostJSON(path string, dst any) error {
	resp, err := c.do(http.MethodGet, path, nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrHostUnsupported
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lxdclient: host %s: %s", path, decodeError(resp))
	}
	// Потолок с запасом: интерфейсов на роутере десятки, а точек монтирования
	// единицы — мегабайта хватает с многократным запасом.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(dst); err != nil {
		return fmt.Errorf("lxdclient: host %s: parse: %w", path, err)
	}
	return nil
}
