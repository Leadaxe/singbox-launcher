// File chain_generator.go — эмиссия источника-цепочки (SPEC 110).
//
// Цепочка — это МАРШРУТ через несколько позиций подряд, а не точка выбора
// между маршрутами. Поэтому она живёт источником рядом с подпиской и
// сервером, а для остального лаунчера выглядит УЗЛОМ: попадает в общий пул,
// отбирается фильтрами Направлений, переключается в Clash API.
//
// Механизм эмиссии — тот же, что у ручного `config_json`
// (`subscription/manual_config.go`): готовый объект уходит в конфиг как
// есть (`ParsedNode.EmitRaw`), лаунчер перештамповывает только тег.
// Отличие в том, что объект собирает форма, а не пишет пользователь.
//
// Ядро отвергает конфиг ЦЕЛИКОМ, встретив неизвестный тип outbound'а или
// цепочку, нарушающую его инварианты (`protocol/chain/chain.go:85-100`).
// Поэтому цепочка, не прошедшая ChainEmitError, не превращается в узел
// вовсе — отдать её ядру «пусть само разберётся» значило бы оставить
// пользователя без VPN, а не без одного маршрута.
package config

import (
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
func ChainEmitError(tag string, c *configtypes.SourceChain) string {
	if c == nil || len(c.Hops) == 0 {
		return "цепочка пуста: не задано ни одной позиции"
	}
	hops := c.Hops
	if len(hops) < 2 {
		return "в цепочке одна позиция: ядру нужно минимум две"
	}
	seen := make(map[string]bool, len(hops))
	for i, hop := range hops {
		if strings.TrimSpace(hop) == "" {
			return fmt.Sprintf("позиция %d пуста", i+1)
		}
		if hop == tag {
			return fmt.Sprintf("позиция %d ссылается на саму цепочку", i+1)
		}
		if seen[hop] {
			return fmt.Sprintf("позиция %d повторяет %q", i+1, hop)
		}
		seen[hop] = true
	}
	for typeName := range c.Rewrite {
		if strings.TrimSpace(typeName) == "" {
			return "rewrite: пустое имя типа outbound'а"
		}
	}
	for key := range c.Strip {
		if _, known := configtypes.ChainStripDefault[key]; !known {
			return fmt.Sprintf("strip: неизвестный ключ %q (можно: %s)",
				key, strings.Join(configtypes.ChainStripKeys, ", "))
		}
	}
	return ""
}

// ChainOutboundObject собирает sing-box outbound типа `chain`.
//
// Возвращает map, а не строку: цепочка эмитится узлом с EmitRaw, и объект
// уходит в конфиг через общий путь ручных нод (`generateRawNodeJSON`).
// Своя сериализация здесь означала бы вторую реализацию того же — с риском
// разойтись в мелочах вроде экранирования тега.
//
// Порядок ключей `strip` не важен для map, но важен для читаемости конфига,
// поэтому каталог обходится по своему порядку, а не по map range.
func ChainOutboundObject(tag string, c *configtypes.SourceChain) map[string]interface{} {
	if c == nil {
		return nil
	}
	// Ключ ядра — `outbounds`, наше поле — Hops (см. комментарий у типа).
	hops := make([]interface{}, 0, len(c.Hops))
	for _, h := range c.Hops {
		hops = append(hops, h)
	}
	ob := map[string]interface{}{
		"tag":       tag,
		"type":      configtypes.ChainOutboundType,
		"outbounds": hops,
	}
	if v := strings.TrimSpace(c.IdleTimeout); v != "" {
		ob["idle_timeout"] = v
	}
	if c.StripEvasion != nil {
		ob["strip_evasion"] = *c.StripEvasion
	}
	if len(c.Strip) > 0 {
		strip := make(map[string]interface{}, len(c.Strip))
		for _, key := range configtypes.ChainStripKeys {
			if val, ok := c.Strip[key]; ok {
				strip[key] = val
			}
		}
		if len(strip) > 0 {
			ob["strip"] = strip
		}
	}
	if len(c.Rewrite) > 0 {
		ob["rewrite"] = c.Rewrite
	}
	return ob
}
