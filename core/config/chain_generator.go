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
// приём, что у CoreBuildTagsProbe.
var ChainSupportProbe func() (supported bool, reason string)

// chainSupported — вердикт пробы с безопасным умолчанием.
func chainSupported() (bool, string) {
	if ChainSupportProbe == nil {
		return true, ""
	}
	return ChainSupportProbe()
}

// chainSupportedFor — вердикт для машины-цели сборки (SPEC 186). Теги сборки
// ядра чужой машины неизвестны, а локальная проба говорит о другом бинаре —
// для неё действует то же безопасное умолчание, что без пробы.
func chainSupportedFor(target *configtypes.BuildTarget) (bool, string) {
	if target != nil {
		return true, ""
	}
	return chainSupported()
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
		return "chain is empty: no positions set"
	}
	hops := c.Hops
	if len(hops) < 2 {
		return "chain has a single position: the core needs at least two"
	}
	seen := make(map[string]bool, len(hops))
	for i, hop := range hops {
		if strings.TrimSpace(hop) == "" {
			return fmt.Sprintf("position %d is empty", i+1)
		}
		if hop == tag {
			return fmt.Sprintf("position %d references the chain itself", i+1)
		}
		if seen[hop] {
			// Маркер нерезолвимости снимается: человеку показывают тег
			// позиции, а не служебный префикс сборки.
			return fmt.Sprintf("position %d repeats %q", i+1, chainHopDisplayTag(hop))
		}
		seen[hop] = true
	}
	for typeName := range c.Rewrite {
		if strings.TrimSpace(typeName) == "" {
			return "rewrite: empty outbound type name"
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
// `idle_timeout`, `strip_evasion`, `strip` не пишутся: ядро с SPEC 165 их
// отвергает загрузкой (SPEC 189), фильтры перенесены в lx.detour.strip.
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
	// Только true: false — дефолт ядра, и писать его значило бы
	// зафиксировать сегодняшнее умолчание навсегда.
	if c.InterruptExistConnections {
		ob["interrupt_exist_connections"] = true
	}
	if len(c.Rewrite) > 0 {
		ob["rewrite"] = c.Rewrite
	}
	return ob
}
