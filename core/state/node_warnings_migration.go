// File node_warnings_migration.go — разовый пересчёт кодов деградации на
// узлах, сохранённых до конвейера (SPEC 131 W2c §3.5, ловушка Л3).
//
// # Зачем
//
// Коды узла (`Node.Warnings`) — производные данные: они отвечают на вопрос
// «что у этого тела сняли правила реестра» и считаются заново из origin.raw
// любым повторным разбором. У узлов, лежащих в state со времён до SPEC 131,
// их нет вовсе — а значит, нет и ⚠ в списке: человек видит узел, у которого
// молча срезали обфускацию, и повода заподозрить это у него нет.
//
// # Почему по телу, а не по origin.raw
//
// Пересбор из origin.raw — это ПЕРЕразбор: он даёт другое тело (правила
// парсера с тех пор менялись), то есть меняет узел, а не объясняет его.
// Здесь задача обратная — объяснить то тело, которое у узла УЖЕ есть.
// Поэтому санитайзер запускается по телу, а origin не читается вовсе.
//
// # Когда переписывается тело
//
// Только если санитайзер что-то снял или привёл (решение ТЗ §3.5, Л3).
// Тело, прошедшее правила чисто, остаётся байт в байт: иначе апгрейд
// переписал бы `state.json` целиком, и у всех узлов разом «изменилось тело»
// — ровно тот массовый шум, которого избегает вся кампания. Каждая
// перезапись — строка WARN в лог с тегом и кодами.
package state

import (
	"strings"

	"singbox-launcher/internal/debuglog"
)

// recountNodeWarnings проходит узлы состояния и досчитывает коды там, где их
// не считали ни разу (`Warnings == nil` при непустом `Body`).
//
// Возвращает число узлов, у которых ПЕРЕПИСАНО ТЕЛО, — именно оно решает,
// надо ли сохранять файл. Узел, у которого появились только коды, файла не
// требует: коды производные, и следующая загрузка посчитает их заново.
func recountNodeWarnings(s *State) int {
	if s == nil || migrationHooks.SanitizeBody == nil {
		return 0
	}
	rewritten := 0
	for i := range s.Sources {
		rewritten += recountOneNode(&s.Sources[i].Node)
		for j := range s.Sources[i].Nodes {
			rewritten += recountOneNode(&s.Sources[i].Nodes[j])
		}
	}
	return rewritten
}

func recountOneNode(node *Node) int {
	if node == nil || node.Kind != SourceKindServer {
		return 0
	}
	// nil = «не считали», пустой список = «считали, чисто». Узел, у которого
	// коды уже есть, не трогаем: их поставил разбор, и он знает о входе
	// больше, чем мы знаем о теле.
	if node.Warnings != nil || len(node.Body) == 0 {
		return 0
	}
	res, err := migrationHooks.SanitizeBody(SanitizeBodyRequest{Body: node.Body})
	if err != nil || res == nil {
		if err != nil {
			debuglog.WarnLog("state: node %q: warnings not recounted: %v", node.Tag, err)
		}
		return 0
	}
	// Пустой срез, а не nil: узел «посчитан и чист». Различие живёт в
	// памяти — omitempty сотрёт его из файла, и следующая загрузка посчитает
	// заново по тому же телу с тем же результатом (цена ошибки нулевая,
	// см. комментарий у Node.Warnings).
	if res.Warnings == nil {
		res.Warnings = []NodeWarning{}
	}
	node.Warnings = res.Warnings

	if len(res.Body) == 0 {
		return 0 // санитайзер ничего не снял — тело остаётся байт в байт
	}
	node.Body = res.Body
	debuglog.WarnLog("state: node %q: body rewritten by the registry sanitizer (%s)",
		node.Tag, warningCodesLabel(res.Warnings))
	return 1
}

// warningCodesLabel — коды строкой для WARN-записи.
func warningCodesLabel(warns []NodeWarning) string {
	if len(warns) == 0 {
		return "no codes"
	}
	parts := make([]string, 0, len(warns))
	for _, w := range warns {
		if w.Path != "" {
			parts = append(parts, w.Code+" @ "+w.Path)
			continue
		}
		parts = append(parts, w.Code)
	}
	return strings.Join(parts, ", ")
}
