// File direction_options.go — опции Направлений (addOutbounds) на сборке
// (NODE_LINK.md §8, решение владельца 15.09.2026).
//
// Направление на узлы не ссылается: узлы в него набирает фильтр, а в опциях
// стоят только ОБЪЯВЛЕННЫЕ корневые имена — теги Направлений и их `-auto`,
// теги свёрток и их `-auto`, системные и шаблонные теги. Форма другого и не
// пишет; узел или чужое имя в опциях появляется только сырым JSON, чужим
// файлом или ручной правкой состояния.
//
// Сборка уже сохранённый маршрут молча не меняет:
//
//   - опция, совпавшая с финальным тегом узла, остаётся в составе, как и
//     раньше, но пользователь узнаёт, что так узел в Направление не кладут;
//   - опция, которой нет вовсе, из группы выпадает (ядро отвергло бы весь
//     конфиг), и теперь об этом говорит предупреждение, а не молчание
//     санитайзера графа.
package config

import (
	"strings"

	"singbox-launcher/internal/locale"
)

// Фразы опций Направлений: ключ локали — английский текст (bin/locale/ru.json).
const (
	emitDirectionNodeOptionText    = "Direction %q: node %q is listed as an option — a node cannot be added to a Direction directly, use a filter"
	emitDirectionUnknownOptionText = "Direction %q: option %q was not found — it is left out of the group"
)

// directionDeclaredTags — теги ВСЕХ Направлений входа, включая выключенные, и
// их `-auto`. Снимается ДО PrepareDirections: выключенное Направление в
// конфиг не едет, но опцией чужого Направления остаётся объявленным именем, и
// назвать его «не найденным» было бы неправдой.
func directionDeclaredTags(directions []Direction) []string {
	var out []string
	for i := range directions {
		tag := strings.TrimSpace(directions[i].Tag)
		if tag == "" {
			continue
		}
		out = append(out, tag)
		if directions[i].Auto != nil {
			out = append(out, tag+twinSuffix)
		}
	}
	return out
}

// directionOptionWarnings — предупреждения об опциях Направлений, которые не
// являются объявленными корневыми именами.
//
// declared — объявленные имена (allRootLinkTargets плюс выключенные
// Направления); nodeTags — финальные теги узлов этой сборки. Адресат —
// Направление: чинят его опции.
func directionOptionWarnings(directions []Direction, declared, nodeTags map[string]bool) []EmissionWarning {
	var out []EmissionWarning
	for i := range directions {
		d := &directions[i]
		if strings.TrimSpace(d.TwinOf) != "" || strings.TrimSpace(d.Tag) == "" {
			continue // твин — производная родителя, опции у него свои не бывают
		}
		seen := map[string]bool{}
		for _, raw := range d.AddOutbounds {
			tag := strings.TrimSpace(raw)
			if tag == "" || seen[tag] || declared[tag] || tag == d.TwinTag {
				continue
			}
			seen[tag] = true
			w := EmissionWarning{DirectionTag: d.Tag}
			if nodeTags[tag] {
				w.Text = locale.Tf(emitDirectionNodeOptionText, d.Tag, tag)
			} else {
				w.Text = locale.Tf(emitDirectionUnknownOptionText, d.Tag, tag)
			}
			out = append(out, w)
		}
	}
	return out
}
