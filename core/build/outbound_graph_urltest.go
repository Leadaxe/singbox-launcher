// File outbound_graph_urltest.go — правило 6 графового санитайзера: пара
// interval/idle_timeout у urltest-группы.
//
// Ядро строит urltest так (protocol/group/urltest.go, NewURLTestGroup):
// пустой interval → 3m, пустой idle_timeout → 30m, и затем
// `interval > idle_timeout` — фатальная ошибка «interval must be less or
// equal than idle_timeout». Проверка живёт в КОНСТРУКТОРЕ аутбаунда, а не в
// разборе схемы, поэтому `sing-box check` конфиг принимает и падает только
// `run` — тот же класс, что правило 4 про вложенную цепочку.
//
// Дыра открыта с двух сторон, и обе дают в конфиге interval без пары:
//   - импорт: провайдерский interval переносится как есть — из sing-box JSON
//     (singboxGroupOptionKeys) и из Xray-подписки, где balancer +
//     burstObservatory.pingConfig.interval конвертируются в urltest
//     (xray_balancer.go). idle_timeout провайдер почти никогда не пишет;
//   - собственные настройки: у шаблонного urltest_interval есть штатный
//     вариант «1h», а idle_timeout шаблон не задаёт вовсе — 1h > 30m ломает
//     старт без всякой подписки.
//
// Политика — поднять idle_timeout до interval, а НЕ урезать interval:
// interval это частота перепроверки ВСЕХ узлов группы, и молча заменить
// провайдерские 3h на 5m значило бы дать по серверу в 36 раз больше проб,
// чем он просил. idle_timeout же не про нагрузку: это простой, после
// которого группа перестаёт проверять себя фоном, и его удлинение стоит
// ровно одного лишнего пробинга у неактивной группы.
//
// Значение берётся у ядра дословно: ParseDuration ядра понимает суффикс «d»
// (sing/common/json/badoption/internal/my_time), которого нет у
// time.ParseDuration. Свой парсер здесь именно поэтому: «1d» — валидный для
// ядра интервал, и принять его за мусор значило бы задеградировать рабочую
// группу.
package build

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"singbox-launcher/internal/debuglog"
)

// coreDefaultURLTestIdleTimeout — значение, которое ядро подставит само, если
// idle_timeout в конфиге нет (constant.DefaultURLTestIdleTimeout).
const coreDefaultURLTestIdleTimeout = 30 * time.Minute

// sanitizeURLTestTimings приводит пару interval/idle_timeout одной группы к
// виду, который переживёт конструктор ядра. Возвращает true, если запись
// изменилась.
//
// Разбирается только то, что реально мешает: если interval невалиден как
// длительность, ядро отвергнет его само с внятным сообщением про сам ключ, и
// подменять такое значение здесь — значит прятать ошибку конфига.
func sanitizeURLTestTimings(e *graphEntry) bool {
	if e.typ() != "urltest" {
		return false
	}

	rawInterval, hasInterval := stringField(e.m, "interval")
	if !hasInterval {
		// Без interval ядро возьмёт 3m, и 3m ≤ любого дефолта — чинить нечего.
		return false
	}
	interval, err := parseCoreDuration(rawInterval)
	if err != nil || interval <= 0 {
		return false
	}

	rawIdle, hasIdle := stringField(e.m, "idle_timeout")
	if !hasIdle {
		if interval <= coreDefaultURLTestIdleTimeout {
			return false
		}
		// Провайдер (или наш собственный «1h») попросил редкую проверку, но
		// idle_timeout не назвал — ядро подставит 30m и упадёт. Пишем пару.
		e.m["idle_timeout"] = rawInterval
		e.dirty = true
		debuglog.WarnLog(
			"build: urltest %q: interval %s exceeds the core default idle_timeout %s — idle_timeout set to %s "+
				"(the interval is kept: shortening it would probe the provider more often than it asked)",
			e.tag, rawInterval, coreDefaultURLTestIdleTimeout, rawInterval)
		return true
	}

	idle, err := parseCoreDuration(rawIdle)
	if err != nil || idle <= 0 || interval <= idle {
		return false
	}
	// Заданы оба и пара не сходится: тянем вверх idle_timeout — см. шапку.
	e.m["idle_timeout"] = rawInterval
	e.dirty = true
	debuglog.WarnLog(
		"build: urltest %q: interval %s is greater than idle_timeout %s — idle_timeout raised to %s "+
			"(the core rejects interval > idle_timeout at startup)",
		e.tag, rawInterval, rawIdle, rawInterval)
	return true
}

// stringField достаёт строковое поле; отсутствующее, пустое и нестроковое
// трактуются одинаково — «не задано».
func stringField(m map[string]interface{}, key string) (string, bool) {
	v, ok := m[key].(string)
	if !ok {
		return "", false
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	return v, true
}

// parseCoreDuration — разбор длительности по правилам ЯДРА, а не stdlib.
//
// Отличие ровно одно и оно значимое: ядро знает суффикс «d» (24h), а
// time.ParseDuration на «1d» отдаёт ошибку. Всё остальное совпадает, поэтому
// дни разворачиваются в часы, а разбор отдаётся stdlib — держать здесь копию
// парсера ядра значило бы завести второе место, расходящееся с ним по
// крайним случаям (дроби, знаки, составные записи вида «1d12h»).
func parseCoreDuration(s string) (time.Duration, error) {
	expanded, err := expandDayUnits(s)
	if err != nil {
		return 0, err
	}
	return time.ParseDuration(expanded)
}

// expandDayUnits заменяет каждую группу «<число>d» на эквивалент в часах.
//
// «d» ловится только как ЕДИНИЦА — то есть суффикс числа, за которым не идёт
// другая буква: иначе «1ds» (мусор) молча стал бы валидным, а суффиксов на
// «d» у ядра больше нет.
func expandDayUnits(s string) (string, error) {
	var b strings.Builder
	num := 0 // длина числа, непосредственно предшествующего текущей позиции

	for i := 0; i < len(s); i++ {
		c := s[i]
		isDigit := (c >= '0' && c <= '9') || c == '.'

		if c == 'd' && num > 0 && (i+1 >= len(s) || !isUnitLetter(s[i+1])) {
			digits := s[i-num : i]
			days, err := strconv.ParseFloat(digits, 64)
			if err != nil {
				return "", errors.New("invalid duration " + s)
			}
			// Срезаем уже записанное число и пишем его же в часах.
			kept := b.String()[:b.Len()-num]
			b.Reset()
			b.WriteString(kept)
			b.WriteString(strconv.FormatFloat(days*24, 'f', -1, 64))
			b.WriteByte('h')
			num = 0
			continue
		}

		b.WriteByte(c)
		if isDigit {
			num++
		} else {
			num = 0
		}
	}
	return b.String(), nil
}

func isUnitLetter(c byte) bool {
	return c >= 'a' && c <= 'z'
}
