// File json_body_rejects.go — позиционный сток отбраковок JSON-веток разбора
// тела (SPEC 116 W11, дефект найден в W13).
//
// Зачем он есть. Модель Unsupported (features/sources.md) требует: КАЖДАЯ
// запись тела, не ставшая узлом, материализуется узлом `kind=unsupported` на
// СВОЕЙ позиции — с исходником и причиной. URI-ветка это делала с самого W11
// (`bodyParseState.reject`), а JSON-ветки — нет: Xray-ветка выбрасывала
// неподдержанный протокол внутри себя одним WARN'ом («unsupported protocol
// "hysteria" skipped»), sing-box-импорт — одним `UnsupportedTypes`. Наружу не
// выходило НИЧЕГО, что материализация (`fetch_materialize.go`) могла бы
// поставить в `nodes[]`, и на Xray-теле unsupported-узлы не появлялись вовсе.
//
// Почему отдельный тип, а не `ParseFailureReasons`. Тот — компактный список
// РАЗНЫХ причин с потолком в 3 штуки, адресат которого «почему источник пуст»;
// он дедуплицирует и обязан НЕ содержать класс «протокол не поддержан» (см.
// xray_reject_reasons_test.go). Здесь задача обратная: нужна КАЖДАЯ запись,
// поштучно, со своим исходником и своим местом в составе. Слить их в один
// список значило бы либо потерять записи на дедупе, либо испортить текст
// строки «источник пуст».
//
// Позиция. JSON-ветки отдают наверх готовый СПИСОК узлов, а не поток; поэтому
// отбраковка помнит, сколько узлов ветка выпустила до неё (`AfterNodes`).
// `ParseSubscriptionBody` проходит этот список в том же порядке и вызывает
// `st.reject` ровно в тот момент, когда счётчик совпал, — так позиция
// пересчитывается в общую нумерацию ПРИНЯТЫХ записей (`RejectedBodyRecord.After`)
// без второй модели позиций.
package subscription

import (
	"encoding/json"
	"fmt"
	"strings"
)

// jsonRejectedRecord — одна неразобранная запись JSON-тела с её местом.
type jsonRejectedRecord struct {
	// AfterNodes — сколько узлов ветка выпустила ДО этой записи (в том самом
	// списке, который она возвращает наверх).
	AfterNodes int
	// Reason — почему запись не стала узлом (текст парсера, английский: он же
	// едет в диагностику).
	Reason string
	// OriginRaw — исходник записи. У JSON-веток это канонический маршал самого
	// элемента — единственная форма «как пришло», которая у них есть
	// пофрагментно.
	OriginRaw string
}

// jsonRejectSink — накопитель отбраковок одной JSON-ветки.
//
// Нулевое значение готово к работе; nil-приёмник молча игнорирует запись —
// черновые проходы (computeXrayServerOwners) разбирают те же элементы второй
// раз, и их отбраковки были бы дублями боевого прохода.
type jsonRejectSink struct {
	records []jsonRejectedRecord
}

// add запоминает отбраковку на позиции afterNodes.
//
// Запись без исходника не запоминается по тому же правилу, что и в
// `bodyParseState.reject`: «unsupported без origin» — форма, которую модель не
// держит, и показать пользователю было бы нечего.
func (s *jsonRejectSink) add(afterNodes int, reason, originRaw string) {
	if s == nil {
		return
	}
	if strings.TrimSpace(originRaw) == "" || strings.TrimSpace(reason) == "" {
		return
	}
	s.records = append(s.records, jsonRejectedRecord{
		AfterNodes: afterNodes,
		Reason:     reason,
		OriginRaw:  originRaw,
	})
}

// list — накопленные отбраковки (копия: читатель не портит накопитель).
func (s *jsonRejectSink) list() []jsonRejectedRecord {
	if s == nil || len(s.records) == 0 {
		return nil
	}
	return append([]jsonRejectedRecord(nil), s.records...)
}

// shift сдвигает позиции всех записей накопителя на base узлов вправо.
//
// Нужен при склейке: элемент Xray-массива нумерует свои отбраковки от нуля
// («после k-го узла ЭТОГО элемента»), а наверху они встают в общий список,
// перед которым уже стоят узлы прежних элементов.
func shiftJSONRejects(records []jsonRejectedRecord, base int) []jsonRejectedRecord {
	out := make([]jsonRejectedRecord, 0, len(records))
	for _, r := range records {
		r.AfterNodes += base
		out = append(out, r)
	}
	return out
}

// newJSONRejectFlusher переводит позиции отбраковок JSON-ветки в общую
// нумерацию принятых записей.
//
// Ветка считала позиции по СВОЕМУ списку узлов («после k-го узла ветки»), а
// `bodyParseState` — по ПРИНЯТЫМ записям («после k-й принятой»). Числа
// расходятся: дедуп по подписи и кап отбрасывают часть узлов ветки уже здесь,
// в `st.accept`. Поэтому позиция не пересчитывается арифметикой, а
// проигрывается: вызывающий отдаёт свой список узлов ветке по одному и после
// каждого зовёт возвращённую функцию с номером пройденного узла; та сдаёт
// накопителю все отбраковки, чьё место наступило, — и `st.reject` берёт
// текущее число принятых сам.
//
// Возвращаемая функция вызывается с монотонно растущим processed; отбраковки
// с позицией за концом списка сдаются на последнем вызове, чтобы запись,
// стоявшая в теле последней, не потерялась.
func newJSONRejectFlusher(st *bodyParseState, records []jsonRejectedRecord) func(processed int) {
	if st == nil || len(records) == 0 {
		return func(int) {}
	}
	next := 0
	return func(processed int) {
		for next < len(records) && records[next].AfterNodes <= processed {
			r := records[next]
			next++
			st.warn(fmt.Sprintf("record rejected: %s", r.Reason))
			st.reject(r.Reason, OriginKindJSON, r.OriginRaw)
		}
	}
}

// marshalRawJSONElement — исходник записи JSON-тела для показа пользователю.
//
// Канонический маршал разобранного элемента (encoding/json сортирует ключи
// карт — стабильно между запусками), тем же правилом, что и
// `marshalNodeOriginJSON` у принятых узлов: пофрагментного «сырого тела
// провайдера» у JSON-веток не существует, а этого достаточно, чтобы запись
// узнать и починить.
func marshalRawJSONElement(v interface{}) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
