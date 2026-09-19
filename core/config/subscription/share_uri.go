// Package subscription: сборка share-ссылки ДВИЖКОМ реестра (SPEC 133).
//
// Обратный ход идёт по ТЕМ ЖЕ секциям `mappers.uri`, что читают ссылку на
// вход: одна таблица в обе стороны. Рукописных эмиттеров (`shareuri_*.go`)
// не осталось — вместе с ними ушёл диспетчер по имени схемы, из-за которого
// эмиттер и парсер расходились молча (память проекта emitter-parser-pairing:
// новая схема без эмиссионной ветки урезалась до {tag,type,server,port}).
//
// Какая секция ведёт узел, решает РЕЕСТР по типу тела, а не switch в коде.
package subscription

import (
	"errors"
	"fmt"
	"strings"

	"singbox-launcher/core/config/linkmap"
	"singbox-launcher/core/config/registry"
)

// ErrShareURINotSupported возвращается для тел, которые ссылкой не
// выражаются: группы (selector/urltest), служебные исходы (direct/block/dns),
// схема без секции обратного хода, а также узел без адреса.
var ErrShareURINotSupported = errors.New("outbound cannot be encoded as share URI")

// ShareURIFromOutbound собирает ссылку из тела узла в форме config.json
// (та же форма, что отдаёт buildOutbound / GenerateNodeJSON).
func ShareURIFromOutbound(out map[string]interface{}) (string, error) {
	if out == nil {
		return "", fmt.Errorf("%w: nil outbound", ErrShareURINotSupported)
	}
	typ := strings.ToLower(strings.TrimSpace(mapGetString(out, "type")))
	if typ == "" {
		return "", fmt.Errorf("%w: outbound without type", ErrShareURINotSupported)
	}

	reg, err := registry.Get()
	if err != nil {
		return "", fmt.Errorf("%w: реестр не собран: %v", ErrShareURINotSupported, err)
	}
	// Схему выбирает РЕЕСТР по типу тела. Тип, которого реестр не знает
	// схемой (группы и служебные исходы), ссылкой не выражается — и это
	// свойство данных, а не список имён в коде.
	scheme, ok := reg.SchemeForSingboxType(typ)
	if !ok {
		return "", fmt.Errorf("%w: type %q", ErrShareURINotSupported, typ)
	}
	plans, err := linkmap.Planes()
	if err != nil {
		return "", fmt.Errorf("%w: реестр не собран: %v", ErrShareURINotSupported, err)
	}
	plan, ok := plans.Plan(scheme, "uri")
	if !ok {
		return "", fmt.Errorf("%w: у схемы %q нет секции uri", ErrShareURINotSupported, scheme)
	}

	// Служебные ключи документа полем узла не являются: `tag` — это МЕТКА
	// (едет во фрагмент либо в ключ формы-контейнера), `type` выбрал секцию.
	body := make(map[string]interface{}, len(out))
	for k, v := range out {
		if k == "tag" || k == "type" {
			continue
		}
		body[k] = v
	}

	uri, err := linkmap.Emit(plan, linkmap.EmitInput{
		Body:     body,
		Label:    mapGetString(out, "tag"),
		Kind:     shareURIKindOf(out),
		BodyType: typ,
	})
	if err != nil {
		if errors.Is(err, linkmap.ErrEmitNoSection) {
			return "", fmt.Errorf("%w: у схемы %q обратного хода нет", ErrShareURINotSupported, scheme)
		}
		return "", fmt.Errorf("%w: %v", ErrShareURINotSupported, err)
	}
	return uri, nil
}

// ShareURIFromWireGuardEndpoint — та же сборка для элемента `endpoints[]`.
//
// Отдельное имя сохранено потому, что вызывающие спрашивают именно про
// endpoint (`config.json` держит его в другом массиве), а различать уровни
// документа — их работа, не наша. Тело у обоих уровней одно.
func ShareURIFromWireGuardEndpoint(ep map[string]interface{}) (string, error) {
	if ep == nil {
		return "", fmt.Errorf("%w: nil endpoint", ErrShareURINotSupported)
	}
	return ShareURIFromOutbound(ep)
}

// shareURIKindOf восстанавливает РОД узла из его тела.
//
// Род объявляет ВХОД (`kind_when` секции), и у узла, сохранённого телом, его
// приходится выводить заново: обратный ход читает род через `emit.form_from`,
// потому что узел, с которого все признаки рода снялись, обязан всё равно
// выбрать своё написание схемы — иначе род теряется на круге
// (MAPPER_ENGINE.md, «Критерий для эмиссии»).
//
// Условия берутся из РЕЕСТРА (`kind_when`), а не из списка полей в коде:
// имена awg-ключей принадлежат секции, и второй их список разъехался бы с
// первым при первом же новом наборе.
func shareURIKindOf(out map[string]interface{}) string {
	typ := strings.ToLower(strings.TrimSpace(mapGetString(out, "type")))
	reg, err := registry.Get()
	if err != nil {
		return ""
	}
	scheme, ok := reg.SchemeForSingboxType(typ)
	if !ok {
		return ""
	}
	set, err := registry.LoadMappers()
	if err != nil {
		return ""
	}
	m, ok := set.Mapper(scheme, "uri")
	if !ok || len(m.KindWhen) == 0 {
		return ""
	}
	return linkmap.KindFromBody(m.KindWhen, out)
}
