// File chain_body.go — тело узла-цепочки в каноне v7 (SPEC 118 W5).
//
// В модели v7 у цепочки, как и у сервера, есть `body` — готовый sing-box
// outbound без того, чем владеет модель. У сервера модель владеет тегом и
// маршрутом (`tag`/`detour`); у цепочки — тегом и ПОЗИЦИЯМИ: позиции это
// ссылки (NodeLink), их подставляет резолв сборки, и в теле им делать нечего.
//
// Остальное — `type`, `idle_timeout`, `strip_evasion`, `strip`, `rewrite` —
// настройки маршрута, которые пользователь задал и которые обязаны пережить
// сохранение. До SPEC 118 они жили отдельным полем состояния (`chain`);
// здесь они переезжают в общий дом тел, чтобы у узла не было двух форм
// хранения.
package configtypes

import (
	"bytes"
	"encoding/json"
	"strings"
)

// ChainBody — сериализованное тело цепочки: те же ключи, что у
// ChainOutboundObject, минус `tag` и `outbounds`. Порядок ключей фиксирован
// (type → idle_timeout → strip_evasion → strip → rewrite), чтобы
// Load→Save→Load был байт-в-байт.
func ChainBody(c *SourceChain) json.RawMessage {
	if c == nil {
		return nil
	}
	var b bytes.Buffer
	b.WriteString(`{"type":`)
	writeJSONString(&b, ChainOutboundType)
	if v := strings.TrimSpace(c.IdleTimeout); v != "" {
		b.WriteString(`,"idle_timeout":`)
		writeJSONString(&b, v)
	}
	if c.StripEvasion != nil {
		if *c.StripEvasion {
			b.WriteString(`,"strip_evasion":true`)
		} else {
			b.WriteString(`,"strip_evasion":false`)
		}
	}
	if len(c.Strip) > 0 {
		first := true
		for _, key := range ChainStripKeys {
			val, ok := c.Strip[key]
			if !ok {
				continue
			}
			if first {
				b.WriteString(`,"strip":{`)
				first = false
			} else {
				b.WriteString(`,`)
			}
			writeJSONString(&b, key)
			if val {
				b.WriteString(`:true`)
			} else {
				b.WriteString(`:false`)
			}
		}
		if !first {
			b.WriteString(`}`)
		}
	}
	if len(c.Rewrite) > 0 {
		if raw, err := json.Marshal(c.Rewrite); err == nil {
			b.WriteString(`,"rewrite":`)
			b.Write(raw)
		}
	}
	b.WriteString(`}`)
	return json.RawMessage(b.Bytes())
}

// ChainFromBody — обратная сторона: настройки маршрута из тела плюс позиции,
// которые модель хранит отдельно. Возвращает форму, которую понимает эмиттер
// цепочек (chain_generator.go).
//
// Тело нечитаемо или пусто → настройки берутся умолчаниями ядра: маршрут с
// позициями важнее настроек, и терять его целиком из-за испорченного тела
// нельзя.
func ChainFromBody(body json.RawMessage, hops []string) *SourceChain {
	out := &SourceChain{Hops: hops}
	if len(body) == 0 {
		return out
	}
	var raw struct {
		IdleTimeout  string                 `json:"idle_timeout"`
		StripEvasion *bool                  `json:"strip_evasion"`
		Strip        map[string]bool        `json:"strip"`
		Rewrite      map[string]interface{} `json:"rewrite"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return out
	}
	out.IdleTimeout = raw.IdleTimeout
	out.StripEvasion = raw.StripEvasion
	out.Strip = raw.Strip
	out.Rewrite = raw.Rewrite
	return out
}

// writeJSONString — строковый литерал JSON без HTML-экранирования (ключи
// каталога и значения времени его не требуют, а `&`/`<` в них ломали бы
// байт-сравнение с телами, записанными эмиттером).
func writeJSONString(b *bytes.Buffer, s string) {
	enc := json.NewEncoder(b)
	enc.SetEscapeHTML(false)
	// Encoder дописывает '\n' — срезаем его, оставляя сам литерал.
	_ = enc.Encode(s)
	raw := b.Bytes()
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		b.Truncate(b.Len() - 1)
	}
}
