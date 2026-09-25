package subscription

import (
	"errors"
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// Xray-вход: что осталось ПОСЛЕ перевода разбора элемента на движок реестра
// (SPEC 133).
//
// Конвертеры протоколов (vless, vmess, trojan, shadowsocks, hysteria v1 и
// hysteria2) отсюда СНЯТЫ вместе с их выемками адреса, TLS и транспорта:
// элемент разбирает движок по секциям `mappers.xray`, и какая схема его
// забирает, решает `detect` реестра, а не список имён в коде. Это и был тот
// скрытый диспетчер диалекта, ради снятия которого затевалась кампания.
//
// Тексты причин отбраковки («empty user id — … subscription may be expired»)
// уехали туда же: они объявлены у обязательных записей секций атрибутом
// `desc_en`, и движок подставляет их в отказ `required`. Причина — знание о
// диалекте, и место ей в данных.
//
// Здесь живёт ровно то, что относится к ЭЛЕМЕНТУ КАК ЗАПИСИ ДОКУМЕНТА, а не
// к его содержимому: служебные протоколы, класс ошибки «протокол не
// поддержан» и запасное имя тега.

// xrayServiceProtocols — служебные Xray-протоколы, не являющиеся узлами.
// Собственный набор, не пересекающийся с sing-box (direct/block/dns).
var xrayServiceProtocols = map[string]struct{}{
	"freedom": {}, "blackhole": {}, "dns": {}, "loopback": {},
}

// IsXrayServiceProtocol сообщает, служебный ли это протокол.
func IsXrayServiceProtocol(p string) bool {
	_, ok := xrayServiceProtocols[strings.ToLower(strings.TrimSpace(p))]
	return ok
}

// xrayUnsupportedProtocolError — «этот протокол лаунчер не умеет».
//
// Отдельный тип, а не текст ошибки, потому что вызывающий обязан РАЗЛИЧАТЬ два
// класса отбраковки, и различать их подстрокой значило бы поставить диагностику
// в зависимость от формулировки. Класс «протокол не поддержан» — свойство
// лаунчера, чинить его пользователю нечем; класс «поддерживаемый протокол,
// битый элемент» (пустой id, нет vnext, кривой порт) — свойство ПОДПИСКИ, и
// пользователь обязан увидеть настоящую причину. До этого разделения любая
// ошибка конверсии превращалась в «unsupported protocol "vless" skipped» —
// сообщение, которое врало и уводило от протухшей подписки.
type xrayUnsupportedProtocolError struct {
	Protocol string
}

func (e *xrayUnsupportedProtocolError) Error() string {
	return fmt.Sprintf("unsupported protocol %q", e.Protocol)
}

// xrayUnsupportedProtocol возвращает имя протокола, если ошибка именно этого
// класса; иначе пусто.
func xrayUnsupportedProtocol(err error) (string, bool) {
	var e *xrayUnsupportedProtocolError
	if errors.As(err, &e) {
		return e.Protocol, true
	}
	return "", false
}

// xrayNodeFromOutbound конвертирует любой поддерживаемый Xray outbound в узел.
//
// Возвращает (nil, nil) для служебных протоколов — это не ошибка, они просто
// не узлы. Для неподдерживаемых возвращает *xrayUnsupportedProtocolError, чтобы
// вызывающий записал протокол в список пропущенных, а не потерял его молча (C1);
// для поддерживаемого протокола с битым содержимым — обычную ошибку с настоящей
// причиной.
func xrayNodeFromOutbound(ob map[string]interface{}, label string) (*configtypes.ParsedNode, error) {
	return xrayNodeFromOutboundInDoc(ob, nil, label)
}

// xrayNodeFromOutboundInDoc — то же для элемента с соседями по документу
// (массив outbounds): записи реестра с `deref` читают соседа, на которого
// ссылается элемент (служебный freedom с fragment по dialerProxy).
func xrayNodeFromOutboundInDoc(ob map[string]interface{}, doc []interface{}, label string) (*configtypes.ParsedNode, error) {
	// Разбор ведёт ДВИЖОК реестра (xray_element_engine.go): какая схема
	// забирает элемент, решает `detect` секции по полю `protocol`.
	node, err, handled := parseXrayElementByEngine(ob, doc, label)
	if handled {
		return node, err
	}
	// Реестр не собрался — разбирать нечем. Причина общая и на весь процесс,
	// а не свойство этого элемента.
	return nil, fmt.Errorf("registry unavailable")
}

// xrayTagOrDefault возвращает тег outbound'а либо запасное имя по схеме.
func xrayTagOrDefault(ob map[string]interface{}, fallback string) string {
	if tag := xrayMapString(ob, "tag"); tag != "" {
		return tag
	}
	return fallback
}
