// File parse_failure_reasons.go — компактный список причин отбраковки узлов
// при разборе ОДНОГО источника.
//
// Зачем отдельный тип, а не []string: причин у большой подписки может быть
// пятьсот, и все они — одна и та же фраза, повторённая на каждый узел
// («empty user id — the server returned a placeholder…»). Пользователю нужна
// не стенограмма, а ответ на вопрос «почему источник пуст»; для этого хватает
// первых нескольких РАЗНЫХ причин. Ограничение стоит на входе, а не на выходе:
// иначе тот же список на 500 строк всё равно накапливался бы в памяти на
// каждой сборке.
//
// SPEC 113-A не затрагивается: это исключительно ВИДИМОСТЬ. Решение «0 узлов =
// недостоверный разбор, состояние не меняем» принимается там же, где и раньше,
// и от наличия причин не зависит.
package subscription

import (
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// MaxParseFailureReasons — сколько РАЗНЫХ причин доезжает до пользователя.
//
// Три: первая называет самую частую поломку (протухший креденшл), вторая-третья
// показывают, что источник сломан не одним способом. Четвёртая уже ничего не
// добавляет к решению «чинить подписку», а строка в UI становится нечитаемой.
const MaxParseFailureReasons = 3

// ParseFailureReasons — накопитель причин с дедупом и потолком.
//
// Нулевое значение готово к работе: собирать причины должен уметь любой разбор,
// не заводя ничего заранее.
type ParseFailureReasons struct {
	reasons []string
	seen    map[string]struct{}
	// truncated — были ли РАЗНЫЕ причины сверх потолка. Нужен, чтобы не
	// выдавать урезанный список за полный.
	truncated bool
}

// Add кладёт причину, если она новая и потолок не выбран.
func (r *ParseFailureReasons) Add(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return
	}
	if r.seen == nil {
		r.seen = make(map[string]struct{}, MaxParseFailureReasons)
	}
	if _, dup := r.seen[reason]; dup {
		return
	}
	r.seen[reason] = struct{}{}
	if len(r.reasons) >= MaxParseFailureReasons {
		r.truncated = true
		return
	}
	r.reasons = append(r.reasons, reason)
}

// AddAll вливает причины другого накопителя (разбор вложен: элемент → источник).
func (r *ParseFailureReasons) AddAll(other []string) {
	for _, reason := range other {
		r.Add(reason)
	}
}

// List — накопленные причины. Копия: читатель не должен уметь испортить
// накопитель следующему источнику.
func (r *ParseFailureReasons) List() []string {
	if len(r.reasons) == 0 {
		return nil
	}
	return append([]string(nil), r.reasons...)
}

// Truncated — были ли отброшены причины сверх потолка.
func (r *ParseFailureReasons) Truncated() bool { return r.truncated }

// Empty — не собрано ни одной причины.
func (r *ParseFailureReasons) Empty() bool { return len(r.reasons) == 0 }

// RecordParseFailures — package-level hook: разбор источника отдаёт наверх
// причины отбраковки СРАЗУ, не дожидаясь, пока их вернёт результат.
//
// Тот же приём, что у NodeIdentityFunc, и по той же
// причине: единственный потребитель — генератор из пакета config, а он зовёт
// разбор через функцию-параметр `loadNodesFunc`, чья сигнатура отдаёт только
// `([]*ParsedNode, error)`. Расширять её значило бы переписать полтора десятка
// точек вызова (в основном тесты) ради значения, которое нужно ровно одному
// вызывающему; хук ставится на время сборки и снимается после.
//
// nil (тесты разбора, точечные прогоны) — причины просто никуда не уезжают.
// Разбор от этого не меняется: хук ничего не решает, только сообщает.
var RecordParseFailures func(source configtypes.ProxySource, reasons []string)

// reportParseFailures зовёт хук, если он установлен и причины есть.
func reportParseFailures(source configtypes.ProxySource, reasons []string) {
	if RecordParseFailures == nil || len(reasons) == 0 {
		return
	}
	RecordParseFailures(source, reasons)
}
