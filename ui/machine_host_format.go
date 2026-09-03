package ui

import (
	"fmt"
	"strings"
	"time"

	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/lxdclient"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	hostArchMismatchText = "⚠ This machine is recorded as %s, but the daemon reports %s. config.json is built for the recorded architecture — fix it in the machine settings."
	hostOsMismatchText   = "⚠ This machine is recorded as %s, but the daemon runs on %s. config.json is built for the recorded platform — fix it in the machine settings."
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

// hostUptime — «18 сут 04:12:37», с секундами.
//
// Демон отдаёт аптайм в секундах, и раз разряд у нас есть — показываем его:
// на свежеподнятой машине «00:00:37» отвечает на вопрос «оно только что
// перезагрузилось?», на который «00:00» ответить не может.
func hostUptime(sec int64) string {
	if sec <= 0 {
		return hostDash
	}
	d := time.Duration(sec) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	clock := fmt.Sprintf("%02d:%02d:%02d", hours, mins, secs)
	if days > 0 {
		return locale.Tf("%dd %s", days, clock)
	}
	return clock
}

// hostInterval — подпись окна усреднения под дельта-блоками.
//
// «34%» без окна — бессмысленное число: за пять секунд и за час это разные
// утверждения. Ноль означает, что процентов ещё нет вовсе.
func hostInterval(sec float64) string {
	if sec <= 0 {
		return ""
	}
	return locale.Tf("sampled over %.1f s", sec)
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

// hostMountFlags — пометки точки монтирования.
//
// read-only помечается явно, потому что такая ФС исключена из сводного
// максимума: squashfs-корень вечно занят на 100%, и без пометки строка
// выглядела бы как незамеченная авария. Звёздочка — раздел state-dir, тот
// самый, чьё переполнение ломает apply.
func hostMountFlags(m lxdclient.HostMount) string {
	switch {
	case m.ReadOnly && m.HoldsStateDir:
		return locale.T("🔒 ro · ★ state")
	case m.ReadOnly:
		return locale.T("🔒 ro")
	case m.HoldsStateDir:
		return locale.T("★ state")
	default:
		return ""
	}
}

// Интерфейс показывается КАРТОЧКОЙ в три строки, а не строкой таблицы: в семь
// колонок не влезала и половина того, что приезжает с машины — MAC, адреса и
// пакеты не показывались вовсе, а MTU ради ширины окна стоял без подписи.
// Поэтому величины ниже подписаны при себе («MTU 1500», «pkt 1.2M/948K»), а не
// опознаются по шапке колонки над ними.

// hostCount — счётчик штук в коротком виде: 1.2M, 948K, 512.
//
// Отдельно от hostBytes, потому что делитель другой: пакеты считаются
// десятичными тысячами, и «1.1M пакетов» от деления на 1024 было бы просто
// неверным числом. Одна дробная цифра — предел полезного: между 1.2M и 1.24M
// пакетов разницы для диагноза нет, а ширину строки вторая цифра съедает.
func hostCount(n uint64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fG", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// hostIfaceSpeeds — скорости первой строки карточки: «↓ 39 KB/s   ↑ 294 KB/s».
//
// У лежачего интерфейса скоростей нет по построению, и вместо двух прочерков
// стоит причина: «— down —» отвечает на вопрос «почему пусто» прямо там, где
// его задают. Стрелки здесь, в отличие от старой таблицы, законны: колонок с
// шапкой больше нет, и направление задать больше нечем.
//
// Пробел после ↑/↓ в этом шрифте рвал отрисовку в подписи ШАПКИ (жирный
// стиль); в обычном тексте карточки стрелка со следующим за ней пробелом
// рисуется. Разделитель между парой — три пробела, а не «·»: это две величины
// одного показателя, а не два разных пункта перечисления.
func hostIfaceSpeeds(i lxdclient.HostInterface) string {
	if !i.Up {
		return locale.T("— down —")
	}
	return "↓ " + hostRate(i.RxBytesPerSecond) + "   ↑ " + hostRate(i.TxBytesPerSecond)
}

// hostIfaceAddresses — вторая строка: все адреса через « · ».
//
// ВСЕ, включая link-local v6: карточка обещает показать всё, что приехало с
// машины, а «лишний» fe80:: — ровно тот адрес, по которому опознают
// интерфейс без конфигурации. Пусто — законное состояние (радиоинтерфейс в
// мосте), и о нём говорится словами, а не пустым местом.
func hostIfaceAddresses(i lxdclient.HostInterface) string {
	if len(i.Addresses) == 0 {
		return locale.T("(no address)")
	}
	return strings.Join(i.Addresses, " · ")
}

// hostIfaceMAC — хвост второй строки.
//
// Пустой MAC у туннеля — не ошибка, и подписи «MAC —» он не заслуживает:
// у tun-устройства канального адреса нет вовсе, и прочерк намекал бы, что его
// не смогли прочитать.
func hostIfaceMAC(i lxdclient.HostInterface) string {
	mac := strings.TrimSpace(i.Mac)
	if mac == "" {
		return ""
	}
	return locale.Tf("MAC %s", mac)
}

// hostIfaceTotals — третья строка: суммарные байты, пакеты, ошибки, потери.
//
// Всё в одной строке через « · », потому что это один ответ на один вопрос —
// «что этот интерфейс насчитал за свою жизнь». Ошибки и потери печатаются
// ВСЕГДА, даже нулями: в таблице их прятали, чтобы не пустела колонка, но в
// карточке пропавшая пара читается как «не измеряли». Приглушает их вызывающая
// сторона стилем, а не мы отсутствием текста.
func hostIfaceTotals(i lxdclient.HostInterface) string {
	return "Σ ↓" + hostBytes(i.RxBytes) + " ↑" + hostBytes(i.TxBytes) +
		"  ·  " + locale.Tf("pkt %s/%s", hostCount(i.RxPackets), hostCount(i.TxPackets)) +
		"  ·  " + locale.Tf("err %d/%d", i.RxErrors, i.TxErrors) +
		"  ·  " + locale.Tf("drop %d/%d", i.RxDropped, i.TxDropped)
}

// hostIfaceClean — нечего тревожиться: ни ошибок, ни потерь.
//
// Служит выбором стиля третьей строки: чистый интерфейс показывается
// приглушённо и не спорит за внимание с тем, у которого дропы растут.
func hostIfaceClean(i lxdclient.HostInterface) bool {
	return i.RxErrors == 0 && i.TxErrors == 0 && i.RxDropped == 0 && i.TxDropped == 0
}

// hostIfaceMTUText — правый край первой строки: «MTU 1500».
//
// С подписью, а не голым числом: в строке рядом со скоростями «1500» без слова
// читалось бы как ещё одна величина трафика.
//
// Неизвестный MTU (ноль — такого не бывает) не показывается вовсе, а не
// прочерком: подписанное «MTU —» утверждало бы, что значение пытались прочесть
// и не смогли, тогда как в карточке пустое место говорит ровно столько же.
func hostIfaceMTUText(i lxdclient.HostInterface) string {
	if i.MTU <= 0 {
		return ""
	}
	return locale.Tf("MTU %d", i.MTU)
}

// hostCPUSummary — заголовок блока CPU.
//
// До второго замера процентов нет, и вместо них честно говорится, чего ждём:
// пустая полоска с «0%» читалась бы как «простаивает».
func hostCPUSummary(c lxdclient.HostCPU) string {
	if c.UsagePercent == nil {
		return locale.T("waiting for a second sample…")
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
	return locale.Tf("load  %.2f  %.2f  %.2f", *c.Load1, *c.Load5, *c.Load15)
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
	s := locale.Tf("%s used of %s", hostBytes(used), hostBytes(m.TotalBytes))
	s += "\n" + locale.Tf("%s available", hostBytes(m.AvailableBytes))
	if m.CachedBytes != nil {
		s += "\n" + locale.Tf("cache %s", hostBytes(*m.CachedBytes))
	}
	return s
}

// hostSwapText — swap либо явное «выключен».
//
// Ноль общего размера — это не «swap пуст», а «swap не настроен»; на роутере
// это норма, и показывать «0 Б / 0 Б» значит заставлять читателя гадать.
func hostSwapText(m lxdclient.HostMemory) string {
	if m.SwapTotalBytes == 0 {
		return locale.T("swap disabled")
	}
	var used uint64
	if m.SwapTotalBytes > m.SwapFreeBytes {
		used = m.SwapTotalBytes - m.SwapFreeBytes
	}
	return locale.Tf("swap %s of %s", hostBytes(used), hostBytes(m.SwapTotalBytes))
}

// hostMachineLine — шапка: модель, ОС, ядро, архитектура, аптайм.
// hostMachineTitle — первая строка шапки: чем машина является.
//
// Модель с архитектурой в скобках: арх — свойство железа, а не отдельный
// пункт перечисления, и в скобках она читается как уточнение, а не как ещё
// одна сущность через разделитель.
func hostMachineTitle(h lxdclient.HostInfo) string {
	switch {
	case h.Model != "" && h.Arch != "":
		return fmt.Sprintf("%s (%s)", h.Model, h.Arch)
	case h.Model != "":
		return h.Model
	default:
		return h.Arch
	}
}

// hostOSBadge — короткая пометка дистрибутива для шапки: «openwrt», «debian».
//
// Берётся из os_id, а при собственном ID форка — из первого элемента
// os_id_like (SPEC 068): immortalwrt ставит свой ID и называет базу только
// там, и без запасного варианта такой форк остался бы неопознанным.
// Показывается только когда добавляет знание: если человеческая строка os и
// так начинается с этого слова, второй раз его писать незачем.
func hostOSBadge(h lxdclient.HostInfo) string {
	id := strings.TrimSpace(h.OSID)
	if id == "" {
		for _, like := range h.OSIDLike {
			if like = strings.TrimSpace(like); like != "" {
				id = like
				break
			}
		}
	}
	if id == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(h.OS)), strings.ToLower(id)) {
		return ""
	}
	return id
}

// hostArchMismatch сообщает, что записанная у машины архитектура не совпала с
// той, что демон прочитал с железа.
//
// Зачем: GOARCH машины оператор выбирает руками при добавлении, и до этой
// ручки проверить его было нечем — под него собирается config.json (SPEC 098
// §2.4), так что промах в выпадающем списке даёт конфиг для чужой платформы.
// Пустая строка — расхождения нет или сверять нечего.
func hostArchMismatch(want, actual string) string {
	want, actual = strings.TrimSpace(want), strings.TrimSpace(actual)
	if want == "" || actual == "" {
		return ""
	}
	if hostArchEqual(want, actual) {
		return ""
	}
	return locale.Tf(hostArchMismatchText, want, actual)
}

// hostOSMismatch — то же для ОС: записанный GOOS против os_family.
//
// Ошибка здесь грубее промаха в архитектуре и ловится тем же способом:
// os_family отдаётся из runtime.GOOS демона, то есть словарь с записанным
// GOOS общий, и нормализовать нечего.
func hostOSMismatch(want, actual string) string {
	want, actual = strings.TrimSpace(want), strings.TrimSpace(actual)
	if want == "" || actual == "" {
		return ""
	}
	if strings.EqualFold(want, actual) {
		return ""
	}
	return locale.Tf(hostOsMismatchText, want, actual)
}

// hostPlatformMismatch собирает предупреждения по обоим полям платформы.
//
// Обе строки сразу, а не первая попавшаяся: если оператор промахнулся в
// выпадающем списке, он мог промахнуться в обоих, и показать только ОС значит
// отправить его чинить платформу дважды.
func hostPlatformMismatch(wantOS, wantArch string, h lxdclient.HostInfo) string {
	parts := make([]string, 0, 2)
	if s := hostOSMismatch(wantOS, h.OSFamily); s != "" {
		parts = append(parts, s)
	}
	if s := hostArchMismatch(wantArch, h.Arch); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n")
}

// hostArchEqual сравнивает имя архитектуры Go с тем, что отдаёт ядро.
//
// Имена не совпадают по написанию: Go говорит amd64/arm64/386, uname —
// x86_64/aarch64/i686. Без нормализации сверка ругалась бы на каждой машине.
func hostArchEqual(goArch, kernelArch string) bool {
	norm := func(s string) string {
		switch strings.ToLower(s) {
		case "x86_64", "amd64":
			return "amd64"
		case "aarch64", "arm64", "armv8", "armv8l":
			return "arm64"
		case "i386", "i486", "i586", "i686", "386":
			return "386"
		case "armv7l", "armv6l", "arm":
			return "arm"
		default:
			return strings.ToLower(s)
		}
	}
	return norm(goArch) == norm(kernelArch)
}

// hostMachineUptime — правый край первой строки: давно ли машина живёт.
//
// Словом, а не стрелкой: «↑ 1d 09:50» в этом шрифте рвётся на тофу из-за
// пробела после стрелки, а вплотную читается как «выросло на».
func hostMachineUptime(h lxdclient.HostInfo) string {
	up := hostUptime(h.UptimeSeconds)
	if up == hostDash {
		return ""
	}
	return locale.Tf("up %s", up)
}

// hostMachineSoftware — вторая строка шапки: что на машине запущено.
//
// Прошивка и ядро отделены от модели, потому что отвечают на другой вопрос:
// первая строка говорит «что это за коробка», вторая — «что в ней сейчас
// стоит». Одной строкой они сливались в перечисление длиной в экран.
func hostMachineSoftware(h lxdclient.HostInfo) string {
	// Ядро в скобках после прошивки: версия ядра — уточнение к ней, а не
	// равноправный пункт перечисления. Через разделитель две версии подряд
	// читались как одна длинная строка непонятно чего.
	//
	// Подпись ядра берётся из os_family (SPEC 068), а не угадывается по виду
	// версии: «linux 6.6.119» до этого поля было домыслом клиента, и на
	// не-Linux хосте оно оказалось бы враньём.
	kernel := h.Kernel
	if kernel != "" && h.OSFamily != "" {
		kernel = h.OSFamily + " " + kernel
	}
	// Пометка дистрибутива — сразу за прошивкой: форк зовёт себя RouteRich, а
	// платформа под ним openwrt, и знать это нужно раньше, чем версию ядра.
	os := h.OS
	if badge := hostOSBadge(h); badge != "" && os != "" {
		os = fmt.Sprintf("%s [%s]", os, badge)
	}

	switch {
	case os != "" && kernel != "":
		return fmt.Sprintf("%s  (%s)", os, kernel)
	case os != "":
		return os
	case kernel != "":
		return kernel
	default:
		return ""
	}
}
