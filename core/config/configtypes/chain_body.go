// File chain_body.go — тело узла-цепочки в каноне v7 (SPEC 118 W5).
//
// В модели v7 у цепочки, как и у сервера, есть `body` — готовый sing-box
// outbound без того, чем владеет модель. У сервера модель владеет тегом и
// маршрутом (`tag`/`detour`); у цепочки — тегом и ПОЗИЦИЯМИ: позиции это
// ссылки (NodeLink), их подставляет резолв сборки, и в теле им делать нечего.
//
// Остальное — `type`, `interrupt_exist_connections`, `rewrite` —
// настройки маршрута, которые пользователь задал и которые обязаны пережить
// сохранение. До SPEC 118 они жили отдельным полем состояния (`chain`);
// здесь они переезжают в общий дом тел, чтобы у узла не было двух форм
// хранения.
package configtypes

import (
	"bytes"
	"encoding/json"
)

// ChainBody — сериализованное тело цепочки: те же ключи, что у
// ChainOutboundObject, минус `tag` и `outbounds`. Порядок ключей фиксирован
// (type → interrupt_exist_connections → rewrite), чтобы Load→Save→Load был
// байт-в-байт. `idle_timeout`, `strip_evasion`, `strip` не пишутся: ядро с
// SPEC 165 отвергает их загрузкой (SPEC 189).
func ChainBody(c *SourceChain) json.RawMessage {
	if c == nil {
		return nil
	}
	var b bytes.Buffer
	b.WriteString(`{"type":`)
	writeJSONString(&b, ChainOutboundType)
	if c.InterruptExistConnections {
		b.WriteString(`,"interrupt_exist_connections":true`)
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
	// `idle_timeout`, `strip_evasion`, `strip` из тел, сохранённых до SPEC 189,
	// не читаются: ядро с SPEC 165 их отвергает, а старому ядру хватает его
	// умолчаний (простой 5 мин, каталог фильтров). Следующее сохранение
	// пишет тело уже без них.
	var raw struct {
		InterruptExistConnections bool                   `json:"interrupt_exist_connections"`
		Rewrite                   map[string]interface{} `json:"rewrite"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return out
	}
	out.InterruptExistConnections = raw.InterruptExistConnections
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
