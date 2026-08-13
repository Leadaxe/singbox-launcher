package ui

import (
	"fmt"
	"time"

	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/lxdclient"
)

// Форматирование телеметрии хоста (SPEC 068 форка, §10b документации демона).
//
// Вынесено из окна отдельно и без единого виджета: всё интересное здесь —
// решения про «нет данных», и их надо проверять тестами, а не глазами на
// живом роутере. Правило одно на весь файл: отсутствующее значение
// показывается прочерком, а НЕ нулём. Ноль в этих числах — утверждение
// («простаивает», «датчик показал 0 °C»), и подменять им «не знаем» значит
// врать ровно в тот момент, когда на экран смотрят ради диагноза.

// hostDash — то, чем показывается отсутствующее значение.
const hostDash = "—"

// hostBytes переводит байты в человеческий вид.
//
// Единицы латиницей и без перевода — так же, как в formatBytes профайлера:
// эти суффиксы в проекте не локализуются, и своя конвенция здесь развела бы
// два окна, которые смотрят рядом.
//
// Две значащие дробные цифры от гигабайта: 3.60 ГБ и 3.6 ГБ на диске роутера
// — это разница в десятки мегабайт, которую и хотят видеть.
func hostBytes(n uint64) string {
	const unit = 1024
	switch {
	case n >= unit*unit*unit*unit:
		return fmt.Sprintf("%.2f TB", float64(n)/(unit*unit*unit*unit))
	case n >= unit*unit*unit:
		return fmt.Sprintf("%.2f GB", float64(n)/(unit*unit*unit))
	case n >= unit*unit:
		return fmt.Sprintf("%.1f MB", float64(n)/(unit*unit))
	case n >= unit:
		return fmt.Sprintf("%.0f KB", float64(n)/unit)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// hostRate — скорость в секунду; nil (первый замер) даёт прочерк.
func hostRate(v *float64) string {
	if v == nil {
		return hostDash
	}
	return hostBytes(uint64(*v)) + "/s"
}

// hostPercent — процент; nil даёт прочерк.
func hostPercent(v *float64) string {
	if v == nil {
		return hostDash
	}
	return fmt.Sprintf("%.1f%%", *v)
}

// hostBarValue — заполнение полоски 0..1.
//
// При nil отдаётся 0 (полоска пустая), но подпись рядом обязана быть
// прочерком: пустая полоска сама по себе читается как «свободно», и только
// текст отличает «свободно» от «ещё не знаем».
func hostBarValue(v *float64) float64 {
	if v == nil {
		return 0
	}
	switch {
	case *v < 0:
		return 0
	case *v > 100:
		return 1
	default:
		return *v / 100
	}
}

// hostUptime — «18 сут 04:12», без секунд: аптайм роутера меряют днями, и
// бегущие секунды в нём только шумят.
func hostUptime(sec int64) string {
	if sec <= 0 {
		return hostDash
	}
	d := time.Duration(sec) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return locale.Tf("remote.host.uptime_days", days, hours, mins)
	}
	return fmt.Sprintf("%02d:%02d", hours, mins)
}

// hostInterval — подпись окна усреднения под дельта-блоками.
//
// «34%» без окна — бессмысленное число: за пять секунд и за час это разные
// утверждения. Ноль означает, что процентов ещё нет вовсе.
func hostInterval(sec float64) string {
	if sec <= 0 {
		return ""
	}
	return locale.Tf("remote.host.interval", sec)
}

// hostThermalText — максимум по датчикам одной строкой.
//
// Указатель nil — «датчиков нет» (виртуалка, контейнер, macOS без CGO), и это
// не то же самое, что 0 °C.
func hostThermalText(t *lxdclient.HostThermal) string {
	if t == nil {
		return hostDash
	}
	return fmt.Sprintf("%.1f °C", t.MaxCelsius)
}

// hostFDText — «412 / 8192»; неизвестный любой из двух даёт прочерк целиком.
//
// Именно целиком: «412 / —» выглядит как заполненность, про которую нечего
// сказать, тогда как смысл строки — насколько близко до потолка.
func hostFDText(open, limit *int) string {
	if open == nil || limit == nil {
		return hostDash
	}
	return fmt.Sprintf("%d / %d", *open, *limit)
}

// hostFDRatio — заполнение полоски дескрипторов 0..1.
func hostFDRatio(open, limit *int) float64 {
	if open == nil || limit == nil || *limit <= 0 {
		return 0
	}
	r := float64(*open) / float64(*limit)
	if r > 1 {
		return 1
	}
	return r
}

// hostMountUsed — «7.4% · свободно 3.33 ГБ» для одной точки монтирования.
func hostMountUsed(m lxdclient.HostMount) string {
	return locale.Tf("remote.host.mount_used", m.UsedPercent, hostBytes(m.AvailableBytes))
}

// hostMountFlags — пометки точки монтирования.
//
// read-only помечается явно, потому что такая ФС исключена из сводного
// максимума: squashfs-корень вечно занят на 100%, и без пометки строка
// выглядела бы как незамеченная авария. Звёздочка — раздел state-dir, тот
// самый, чьё переполнение ломает apply.
func hostMountFlags(m lxdclient.HostMount) string {
	switch {
	case m.ReadOnly && m.HoldsStateDir:
		return locale.T("remote.host.flag_ro_state")
	case m.ReadOnly:
		return locale.T("remote.host.flag_ro")
	case m.HoldsStateDir:
		return locale.T("remote.host.flag_state")
	default:
		return ""
	}
}

// Значения интерфейса отдаются БЕЗ стрелок: направление задаёт колонка, в
// шапке которой стрелка стоит один раз. Стрелка в каждой ячейке — это шум,
// который ещё и мешает выровнять числа по правому краю.
//
// Отдельно: пробел ПОСЛЕ ↑/↓ рвёт отрисовку в этом шрифте (в тексте вылезает
// тофу вместо стрелки), поэтому в шапке стрелка приклеена к слову вплотную.

// hostIfaceRxRate — входящая скорость; прочерк до второго замера.
func hostIfaceRxRate(i lxdclient.HostInterface) string { return hostRate(i.RxBytesPerSecond) }

// hostIfaceTxRate — исходящая скорость; прочерк до второго замера.
func hostIfaceTxRate(i lxdclient.HostInterface) string { return hostRate(i.TxBytesPerSecond) }

// hostIfaceRxTotal — сырой счётчик приёма. Он переживает рестарты и разрывы,
// и именно по нему строят график; скорость рядом — для чтения глазами.
func hostIfaceRxTotal(i lxdclient.HostInterface) string { return hostBytes(i.RxBytes) }

// hostIfaceTxTotal — сырой счётчик передачи.
func hostIfaceTxTotal(i lxdclient.HostInterface) string { return hostBytes(i.TxBytes) }

// hostIfaceErrors — «120 / 52210» для колонки «ошиб/дроп».
//
// Пусто, когда всё чисто: ноль ошибок — норма, и печатать его в каждой строке
// значит приучить глаз пролистывать колонку, в которой однажды появится
// ненулевое число.
func hostIfaceErrors(i lxdclient.HostInterface) string {
	errs := i.RxErrors + i.TxErrors
	drops := i.RxDropped + i.TxDropped
	if errs == 0 && drops == 0 {
		return ""
	}
	return fmt.Sprintf("%d / %d", errs, drops)
}

// hostIfaceMTU — MTU строкой; ноль показывается прочерком, а не «0»: MTU=0
// не бывает, это «не знаем».
func hostIfaceMTU(i lxdclient.HostInterface) string {
	if i.MTU <= 0 {
		return hostDash
	}
	return fmt.Sprintf("%d", i.MTU)
}

// hostCPUSummary — заголовок блока CPU.
//
// До второго замера процентов нет, и вместо них честно говорится, чего ждём:
// пустая полоска с «0%» читалась бы как «простаивает».
func hostCPUSummary(c lxdclient.HostCPU) string {
	if c.UsagePercent == nil {
		return locale.T("remote.host.awaiting_sample")
	}
	return hostPercent(c.UsagePercent)
}

// hostLoadText — три числа load average одной строкой.
//
// Достаются от ядра готовыми, поэтому есть уже в первом ответе — в отличие от
// процентов рядом.
func hostLoadText(c lxdclient.HostCPU) string {
	if c.Load1 == nil || c.Load5 == nil || c.Load15 == nil {
		return hostDash
	}
	return locale.Tf("remote.host.load", *c.Load1, *c.Load5, *c.Load15)
}

// hostMemoryDetail — расшифровка памяти под полоской.
//
// Процент демон считает от available, а не от free, поэтому «занято» здесь
// тоже от available: роутер держит почти всю память в page cache, и цифра от
// free кричала бы «занято» при реально свободных 120 МБ.
func hostMemoryDetail(m lxdclient.HostMemory) string {
	// AvailableBytes может превысить Total на кривом ответе; вычитание на
	// uint64 ушло бы в огромное число вместо нуля.
	var used uint64
	if m.TotalBytes > m.AvailableBytes {
		used = m.TotalBytes - m.AvailableBytes
	}
	// Построчно, а не одной длинной строкой: Label без переноса отдаёт
	// минимальной шириной весь свой текст, и «… · cache 168.1 MB» распирал
	// колонку шире окна.
	s := locale.Tf("remote.host.mem_used", hostBytes(used), hostBytes(m.TotalBytes))
	s += "\n" + locale.Tf("remote.host.mem_avail", hostBytes(m.AvailableBytes))
	if m.CachedBytes != nil {
		s += "\n" + locale.Tf("remote.host.mem_cache", hostBytes(*m.CachedBytes))
	}
	return s
}

// hostSwapText — swap либо явное «выключен».
//
// Ноль общего размера — это не «swap пуст», а «swap не настроен»; на роутере
// это норма, и показывать «0 Б / 0 Б» значит заставлять читателя гадать.
func hostSwapText(m lxdclient.HostMemory) string {
	if m.SwapTotalBytes == 0 {
		return locale.T("remote.host.swap_off")
	}
	var used uint64
	if m.SwapTotalBytes > m.SwapFreeBytes {
		used = m.SwapTotalBytes - m.SwapFreeBytes
	}
	return locale.Tf("remote.host.swap_on", hostBytes(used), hostBytes(m.SwapTotalBytes))
}

// hostMachineLine — шапка: модель, ОС, ядро, архитектура, аптайм.
func hostMachineLine(h lxdclient.HostInfo) string {
	parts := make([]string, 0, 5)
	for _, s := range []string{h.Model, h.OS, h.Kernel, h.Arch} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	line := ""
	for i, p := range parts {
		if i > 0 {
			line += " · "
		}
		line += p
	}
	if up := hostUptime(h.UptimeSeconds); up != hostDash {
		if line != "" {
			line += " · "
		}
		// Словом, а не стрелкой: «↑ 1d 09:50» в этом шрифте рвётся на тофу
		// из-за пробела после стрелки, а вплотную читается как «выросло на».
		line += locale.Tf("remote.host.uptime", up)
	}
	return line
}
