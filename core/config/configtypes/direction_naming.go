// Package configtypes: direction_naming.go — выдача тега и имени по умолчанию
// для нового Направления (SPEC 104).
package configtypes

import "strconv"

// DirectionTagPrefix — префикс автоматически выдаваемых тегов направлений.
//
// Общий с LxBox: тег — цель правил, и бэкап, приехавший с телефона, должен
// попадать в существующее направление, а не заводить рядом второе.
const DirectionTagPrefix = "vpn-"

// NextDirectionTag выдаёт свободный тег `vpn-N` среди занятых.
//
// Ищет первую свободную позицию, а не «максимум + 1»: после удаления
// среднего направления номера не должны уползать вверх — иначе `vpn-2`
// навсегда исчезает из списка, хотя тег свободен, а пользователь видит
// растущие числа при трёх живых направлениях.
//
// Потолка нет (решение D-4): лимит LxBox в 10 каналов — следствие его
// интерфейса, а не модели.
func NextDirectionTag(usedTags []string) string {
	used := make(map[string]bool, len(usedTags))
	for _, t := range usedTags {
		used[t] = true
	}
	for i := 1; ; i++ {
		tag := DirectionTagPrefix + strconv.Itoa(i)
		if !used[tag] {
			return tag
		}
	}
}

// DirectionNumber возвращает N из тега вида `vpn-N`; ok == false для любого
// другого тега (`proxy-out`, `ru VPN 🇷🇺`).
func DirectionNumber(tag string) (n int, ok bool) {
	rest, found := trimPrefix(tag, DirectionTagPrefix)
	if !found || rest == "" {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// trimPrefix — strings.TrimPrefix с признаком «префикс был».
//
// Локальная копия ради одного факта: `strings.TrimPrefix` не отличает
// «префикса не было» от «строка равна префиксу», а нам нужно и то и другое.
func trimPrefix(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || s[:len(prefix)] != prefix {
		return s, false
	}
	return s[len(prefix):], true
}
