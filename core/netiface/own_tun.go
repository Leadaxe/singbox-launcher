package netiface

import (
	"strings"
	"sync"
)

// Имя СОБСТВЕННОГО TUN — второй признак петли, помимо адреса из tunSubnets.
//
// SPEC 113-F: чужой туннель (системный wg0/awg1) — законный аплинк, и отличить
// его от нашего по префиксу имени нельзя: наш зовётся singbox-tun0, а чужой
// tun0 — оба ловятся одним и тем же "tun". Разделяет их только ТОЧНОЕ имя,
// которое ядро получило в конфиге (tun.interface_name).
//
// Почему регистрация, а не чтение конфига отсюда: netiface лежит под UI и под
// сборкой конфига, а конфиг читает core/config — прямой импорт замкнул бы
// зависимость в кольцо. Имя ставит тот, кто и так держит путь к config.json.
//
// Пустой реестр — рабочее состояние, а не сбой: имя ещё не прочитано, конфига
// нет вовсе (первый запуск), таргет чужой. Признак адреса при этом никуда не
// девается и продолжает ловить наш TUN самостоятельно.
var (
	ownTunMu    sync.RWMutex
	ownTunNames = map[string]struct{}{}
)

// SetOwnTunNames запоминает имена собственных TUN лаунчера/ядра.
//
// Замена целиком, а не добавление: конфиг пересобирают, и имя в нём меняется
// (singbox-tun0 → своё). Копить прежние значило бы вечно прятать из выбора
// интерфейс, который наш ядру больше не принадлежит.
//
// Пустые строки игнорируются: незаполненная переменная шаблона иначе прятала
// бы из списка интерфейс с пустым именем — а такого не бывает, зато сравнение
// с пустой строкой совпало бы с чем угодно после TrimSpace.
func SetOwnTunNames(names ...string) {
	next := make(map[string]struct{}, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		next[strings.ToLower(n)] = struct{}{}
	}
	ownTunMu.Lock()
	ownTunNames = next
	ownTunMu.Unlock()
}

// isOwnTunName — имя зарегистрировано как собственный TUN.
//
// Сравнение точное (без учёта регистра), а не по префиксу: префикс — это ровно
// то, что путает наш singbox-tun0 с чужим tun0.
func isOwnTunName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	ownTunMu.RLock()
	_, ok := ownTunNames[name]
	ownTunMu.RUnlock()
	return ok
}
