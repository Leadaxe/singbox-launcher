package ui

import (
	"fmt"
	"time"

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
// Одна значащая дробь до гигабайта и две после: 3.87 ГБ и 3.9 ГБ на диске
// роутера — это разница в 30 МБ, которую и хотят видеть.
func hostBytes(n uint64) string {
	const unit = 1024
	switch {
	case n >= unit*unit*unit*unit:
		return fmt.Sprintf("%.2f ТБ", float64(n)/(unit*unit*unit*unit))
	case n >= unit*unit*unit:
		return fmt.Sprintf("%.2f ГБ", float64(n)/(unit*unit*unit))
	case n >= unit*unit:
		return fmt.Sprintf("%.1f МБ", float64(n)/(unit*unit))
	case n >= unit:
		return fmt.Sprintf("%.0f КБ", float64(n)/unit)
	default:
		return fmt.Sprintf("%d Б", n)
	}
}

// hostRate — скорость в секунду; nil (первый замер) даёт прочерк.
func hostRate(v *float64) string {
	if v == nil {
		return hostDash
	}
	return hostBytes(uint64(*v)) + "/с"
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
		return fmt.Sprintf("%d сут %02d:%02d", days, hours, mins)
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
	return fmt.Sprintf("окно замера %.1f с", sec)
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
	return fmt.Sprintf("%.1f%% · свободно %s", m.UsedPercent, hostBytes(m.AvailableBytes))
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
		return "ro · state"
	case m.ReadOnly:
		return "ro"
	case m.HoldsStateDir:
		return "state"
	default:
		return ""
	}
}

// hostIfaceRates — «↓ 11.4 МБ/с   ↑ 1.2 МБ/с» либо прочерки до второго замера.
func hostIfaceRates(i lxdclient.HostInterface) string {
	return fmt.Sprintf("↓ %s   ↑ %s", hostRate(i.RxBytesPerSecond), hostRate(i.TxBytesPerSecond))
}

// hostIfaceTotals — сырые счётчики. Они переживают рестарты и разрывы, и
// именно по ним строят график; скорость рядом — для чтения глазами.
func hostIfaceTotals(i lxdclient.HostInterface) string {
	return fmt.Sprintf("↓ %s   ↑ %s", hostBytes(i.RxBytes), hostBytes(i.TxBytes))
}

// hostIfaceErrors — ошибки и дропы; пустая строка, когда всё чисто.
//
// Пустая, а не «0 / 0»: ноль ошибок — норма, и печатать его в каждой строке
// значит приучить глаз пролистывать колонку, в которой однажды появится
// ненулевое число.
func hostIfaceErrors(i lxdclient.HostInterface) string {
	errs := i.RxErrors + i.TxErrors
	drops := i.RxDropped + i.TxDropped
	if errs == 0 && drops == 0 {
		return ""
	}
	return fmt.Sprintf("ошибки %d · дропы %d", errs, drops)
}

// hostCPUSummary — заголовок блока CPU.
//
// До второго замера процентов нет, и вместо них честно говорится, чего ждём:
// пустая полоска с «0%» читалась бы как «простаивает».
func hostCPUSummary(c lxdclient.HostCPU) string {
	if c.UsagePercent == nil {
		return "ждём второй замер…"
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
	return fmt.Sprintf("load  %.2f  %.2f  %.2f", *c.Load1, *c.Load5, *c.Load15)
}

// hostMemoryDetail — расшифровка памяти под полоской.
//
// Процент демон считает от available, а не от free, поэтому «занято» здесь
// тоже от available: роутер держит почти всю память в page cache, и цифра от
// free кричала бы «занято» при реально свободных 120 МБ.
func hostMemoryDetail(m lxdclient.HostMemory) string {
	used := m.TotalBytes - m.AvailableBytes
	s := fmt.Sprintf("занято %s из %s · доступно %s",
		hostBytes(used), hostBytes(m.TotalBytes), hostBytes(m.AvailableBytes))
	if m.CachedBytes != nil {
		s += " · кеш " + hostBytes(*m.CachedBytes)
	}
	return s
}

// hostSwapText — swap либо явное «выключен».
//
// Ноль общего размера — это не «swap пуст», а «swap не настроен»; на роутере
// это норма, и показывать «0 Б / 0 Б» значит заставлять читателя гадать.
func hostSwapText(m lxdclient.HostMemory) string {
	if m.SwapTotalBytes == 0 {
		return "swap выключен"
	}
	used := m.SwapTotalBytes - m.SwapFreeBytes
	return fmt.Sprintf("swap %s из %s", hostBytes(used), hostBytes(m.SwapTotalBytes))
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
		line += "↑ " + up
	}
	return line
}
