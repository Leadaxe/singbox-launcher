// File chain_generator.go — эмиссия Направления-цепочки (SPEC 110, фаза 1).
//
// Цепочка снаружи — обычный outbound: её кладут в правило, в селектор, в
// urltest. Поэтому она живёт Направлением (SPEC 104) с Type == "chain", а не
// отдельной сущностью, и проходит те же три прохода генератора. Отличий два,
// и оба из-за того, что у цепочки нет состава:
//
//   - фильтры к ней не применяются: позиции заданы явно, а не отобраны по
//     маске, и «поймать узлы фильтром» тут нечего;
//   - зависимости на проходе 2 строятся по Hops, а не по AddOutbounds.
//
// Ядро отвергает конфиг ЦЕЛИКОМ, встретив неизвестный тип outbound'а или
// цепочку, нарушающую его инварианты (`protocol/chain/chain.go:85-100`).
// Поэтому эмиттер молчаливо ничего не выпускает: запись, не прошедшую
// ChainEmitError, генератор пропускает, а не отдаёт ядру «пусть само
// разберётся» — разбор кончился бы отсутствием VPN.
package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// ChainSupportProbe — hook уровня приложения (core.AppController): знает ли
// установленное ядро тип `chain` и почему нет.
//
// nil (тесты парсера, standalone) → считаем, что знает: деградировать на
// догадке нельзя, а `sing-box check` остаётся последним рубежом. Тот же
// приём, что у NaiveSupportProbe.
var ChainSupportProbe func() (supported bool, reason string)

// chainSupported — вердикт пробы с безопасным умолчанием.
func chainSupported() (bool, string) {
	if ChainSupportProbe == nil {
		return true, ""
	}
	return ChainSupportProbe()
}

// ChainSupportedByCore — вердикт пробы для UI: форма показывает
// предупреждение до того, как пользователь настроит маршрут, который не
// попадёт в конфиг. Та же функция, что зовёт эмиттер, — иначе форма и
// сборка разошлись бы в том, что считают поддержкой.
func ChainSupportedByCore() (bool, string) { return chainSupported() }

// ChainEmitError — почему цепочку нельзя выпустить в конфиг. Пусто = можно.
//
// Проверяются ровно инварианты ядра, и в тех же словах: сообщение уезжает
// пользователю в список Направлений, а сверять его с чужим текстом ошибки
// ядра тому, кто читает лог, невозможно.
func ChainEmitError(d configtypes.Direction) string {
	if d.Chain == nil || len(d.Chain.Hops) == 0 {
		return "цепочка пуста: не задано ни одной позиции"
	}
	hops := d.Chain.Hops
	if len(hops) < 2 {
		return "в цепочке одна позиция: ядру нужно минимум две"
	}
	seen := make(map[string]bool, len(hops))
	for i, tag := range hops {
		if strings.TrimSpace(tag) == "" {
			return fmt.Sprintf("позиция %d пуста", i+1)
		}
		if tag == d.Tag {
			return fmt.Sprintf("позиция %d ссылается на саму цепочку", i+1)
		}
		if seen[tag] {
			return fmt.Sprintf("позиция %d повторяет %q", i+1, tag)
		}
		seen[tag] = true
	}
	for typeName := range d.Chain.Rewrite {
		if strings.TrimSpace(typeName) == "" {
			return "rewrite: пустое имя типа outbound'а"
		}
	}
	for key := range d.Chain.Strip {
		if _, known := configtypes.ChainStripDefault[key]; !known {
			return fmt.Sprintf("strip: неизвестный ключ %q (можно: %s)",
				key, strings.Join(configtypes.ChainStripKeys, ", "))
		}
	}
	return ""
}

// GenerateChainJSON собирает outbound типа `chain` одной строкой с хвостовой
// запятой — в том же виде, что отдают остальные эмиттеры прохода 3.
//
// validTags — теги, дошедшие до конфига (проход 2). Позиция, которой в
// конфиге нет, — это ссылка в никуда, на которой ядро не стартует; такую
// цепочку не выпускаем целиком, а не выбрасываем позицию: маршрут без
// одного хопа — это другой маршрут, и молча подменять его нельзя.
//
// Пустая строка = запись не выпущена; причина ушла в reason.
func GenerateChainJSON(d configtypes.Direction, validTags map[string]bool) (string, string) {
	if err := ChainEmitError(d); err != "" {
		return "", err
	}
	if validTags != nil {
		for i, tag := range d.Chain.Hops {
			if !validTags[tag] {
				return "", fmt.Sprintf("позиция %d (%q) не дошла до конфига", i+1, tag)
			}
		}
	}

	c := d.Chain
	var parts []string
	parts = append(parts, fmt.Sprintf(`"tag":%s`, marshalJSONString(d.Tag)))
	parts = append(parts, fmt.Sprintf(`"type":%s`, marshalJSONString(configtypes.DirectionTypeChain)))

	// Ключ ядра — `outbounds`, наше поле — Hops (см. комментарий у типа).
	hopsJSON, _ := json.Marshal(c.Hops)
	parts = append(parts, fmt.Sprintf(`"outbounds":%s`, string(hopsJSON)))

	if v := strings.TrimSpace(c.IdleTimeout); v != "" {
		parts = append(parts, fmt.Sprintf(`"idle_timeout":%s`, marshalJSONString(v)))
	}
	if c.StripEvasion != nil {
		parts = append(parts, fmt.Sprintf(`"strip_evasion":%v`, *c.StripEvasion))
	}
	if len(c.Strip) > 0 {
		// Порядок каталога, а не map range: конфиг обязан собираться
		// байт-в-байт одинаково, иначе golden-фикстуры мигают.
		var stripParts []string
		for _, key := range configtypes.ChainStripKeys {
			if val, ok := c.Strip[key]; ok {
				stripParts = append(stripParts, fmt.Sprintf(`%s:%v`, marshalJSONString(key), val))
			}
		}
		if len(stripParts) > 0 {
			parts = append(parts, fmt.Sprintf(`"strip":{%s}`, strings.Join(stripParts, ",")))
		}
	}
	if len(c.Rewrite) > 0 {
		rewriteJSON, err := json.Marshal(c.Rewrite)
		if err == nil {
			parts = append(parts, fmt.Sprintf(`"rewrite":%s`, string(rewriteJSON)))
		}
	}

	jsonStr := "{" + strings.Join(parts, ",") + "}"

	note := d.Label
	if note == "" {
		note = d.Comment
	}
	result := ""
	if note != "" {
		result = fmt.Sprintf("\t// %s\n", sanitizeOutboundLineComment(note))
	}
	result += fmt.Sprintf("\t%s,", jsonStr)
	return result, ""
}
