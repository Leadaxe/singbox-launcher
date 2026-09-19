// File node_enabled.go — единый сеттер «включить/выключить узел» и хранение
// причины, по которой узел выключило ПРИЛОЖЕНИЕ (SPEC 132, CANON §9.4).
//
// # Где живёт причина
//
// Новых полей нет — решение владельца. Узел выключается тем же флажком
// `enabled=false`, что и рукой человека, а причина едет записью в УЖЕ
// существующем списке `Node.Warnings`:
//
//	{code: "core_rejected", params: {reason: <дословный текст ядра>}}
//
// Отсюда и правило чтения: «выключило приложение» = у выключенного узла есть
// запись `core_rejected`; её отсутствие при `enabled=false` = «выключил
// человек».
//
// # Ловушка, ради которой файл существует
//
// `Node.Warnings` — данные ПРОИЗВОДНЫЕ: их пересчитывает по телу узла
// санитайзер реестра, и пересчёт замещает список ЦЕЛИКОМ (правило Л5,
// SPEC 131 §3.5). Мест пересчёта шесть — миграция при загрузке state, правка
// тела, regen из исходника, форма AmneziaWG, вставка записи, миграция masque —
// и каждое из них молча стёрло бы причину отключения, которая производной НЕ
// является: её поставило ядро, а не разбор тела.
//
// Поэтому пересчёт идёт не присваиванием, а через ReplaceDerivedWarnings:
// одно место-правило, которое переносит авторитетные записи в новый список.
// Добавляя новый код уровня «факт, а не разбор тела», его достаточно внести в
// authoritativeWarningCodes — и все шесть мест начнут его беречь.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package state

import (
	"bytes"
	"encoding/json"
	"strings"
)

// WarnCoreRejected — код записи «ядро не приняло этот узел»
// (contract/registry/warnings.json, severity error, params: reason).
//
// Значение дублирует реестр строкой: core/state — leaf-пакет модели и о
// реестре не знает. Сверку кода с реестром держит
// TestRegistryWarningCodesHaveAProducer на стороне config/subscription.
const WarnCoreRejected = "core_rejected"

// authoritativeWarningCodes — коды, которые ставит НЕ разбор тела, а
// свершившийся факт. Пересчёт warnings их не стирает и не пересчитывает:
// пересчёт отвечает на вопрос «что правила сняли с этого тела», а эти записи
// отвечают на другой — «что с узлом случилось».
//
// Список, а не одиночный код: следующий такой факт (вердикт удалённой
// машины, отказ реального старта) встанет сюда же, и шесть мест пересчёта
// править снова не придётся.
var authoritativeWarningCodes = []string{WarnCoreRejected}

func isAuthoritativeWarning(code string) bool {
	for _, c := range authoritativeWarningCodes {
		if c == code {
			return true
		}
	}
	return false
}

// hasDerivedWarnings — по списку узла видно, что производные коды УЖЕ
// считали.
//
// nil = не считали. Непустой список из одних авторитетных записей — тоже «не
// считали»: их поставил не разбор тела. Пустой НЕ-nil список = «считали, и
// чисто» (различие живёт только в памяти, см. комментарий у Node.Warnings).
func hasDerivedWarnings(warns []NodeWarning) bool {
	if warns == nil {
		return false
	}
	for _, w := range warns {
		if !isAuthoritativeWarning(w.Code) {
			return true
		}
	}
	// Пустой не-nil список: считали и чисто.
	return len(warns) == 0
}

// CoreRejectedReason — текст ядра, если узел выключило приложение; "" иначе.
//
// Пустая строка при наличии записи невозможна: причину ставит только
// SetCoreRejected, а он пустой текст не принимает.
func (n *Node) CoreRejectedReason() string {
	if n == nil {
		return ""
	}
	for i := range n.Warnings {
		if n.Warnings[i].Code == WarnCoreRejected {
			return n.Warnings[i].Params["reason"]
		}
	}
	return ""
}

// SetCoreRejected выключает узел и записывает причину ядра.
//
// Возвращает false, когда делать нечего: узла нет, текст пуст, или узел уже
// выключен ЭТОЙ ЖЕ причиной. Второе важно для цикла страховки: круг, на
// котором выключить нечего нового, обязан цикл прервать (CANON §9.5), и
// признак «ничего не изменилось» приходит отсюда.
//
// Узел kind=unsupported адресовать нечего: он и так не в конфиге, ядро его
// назвать не могло, и включение у него запрещено формой.
func (n *Node) SetCoreRejected(reason string) bool {
	if n == nil || n.IsUnsupported() {
		return false
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return false
	}
	if !n.Enabled && n.CoreRejectedReason() == reason {
		return false
	}
	n.Enabled = false
	// Дубль по коду замещается свежим: причина у узла ровно одна — та, что
	// ядро назвало последней. Позиция сохраняется, чтобы порядок списка не
	// плясал от повторного отказа.
	for i := range n.Warnings {
		if n.Warnings[i].Code == WarnCoreRejected {
			n.Warnings[i] = coreRejectedWarning(reason)
			return true
		}
	}
	// Первой: это вердикт уровня УЗЛА (severity error), а не деградация
	// отдельного поля, и в списке он идёт впереди разборных кодов.
	n.Warnings = append([]NodeWarning{coreRejectedWarning(reason)}, n.Warnings...)
	return true
}

func coreRejectedWarning(reason string) NodeWarning {
	return NodeWarning{
		Code:   WarnCoreRejected,
		Params: map[string]string{"reason": reason},
	}
}

// ClearCoreRejected снимает запись о вердикте ядра. Возвращает true, если
// запись была.
//
// Зовётся ТОЛЬКО из SetNodeEnabled при включении рукой: автоснятия нет — ни
// по смене тела узла, ни по смене версии ядра (CANON §9.4, осознанная цена
// простоты).
func (n *Node) ClearCoreRejected() bool {
	if n == nil || len(n.Warnings) == 0 {
		return false
	}
	out := n.Warnings[:0]
	removed := false
	for _, w := range n.Warnings {
		if w.Code == WarnCoreRejected {
			removed = true
			continue
		}
		out = append(out, w)
	}
	if !removed {
		return false
	}
	// nil, а не пустой срез: «не считали» — то состояние, в котором узел и
	// был бы, не будь у него нашей записи. Следующая загрузка досчитает коды
	// по телу (recountNodeWarnings), что для только что включённого узла
	// ровно то, что нужно.
	if len(out) == 0 {
		n.Warnings = nil
		return true
	}
	n.Warnings = out
	return true
}

// SetNodeEnabled — ЕДИНСТВЕННЫЙ способ переключить узел «рукой».
//
// Возвращает true, если что-то изменилось (флаг или причина), — вызывающему
// это нужно, чтобы не поднимать ревизию модели на холостом клике.
//
// Включение стирает вердикт ядра: человек сказал «пробуй снова», и следующая
// сборка проверит узел заново; откажет — страховка выключит его и повесит
// причину заново (CANON §9.4).
//
// Выключение РУКОЙ вердикт тоже стирает: выключенный человеком узел не должен
// продолжать объясняться чужим текстом, будто это решение ядра.
//
// kind=unsupported включить нельзя — формой (sources_v7.go normalizeNodeShape).
func (n *Node) SetNodeEnabled(enabled bool) bool {
	if n == nil {
		return false
	}
	if enabled && n.IsUnsupported() {
		return false
	}
	changed := n.ClearCoreRejected()
	if n.Enabled != enabled {
		n.Enabled = enabled
		changed = true
	}
	return changed
}

// ReplaceDerivedWarnings замещает ПРОИЗВОДНЫЕ коды узла новым набором,
// сохраняя авторитетные записи.
//
// Одно место-правило на все шесть точек пересчёта (шапка файла). Вызывать
// вместо `node.Warnings = fresh`.
//
// Авторитетные записи встают ПЕРВЫМИ и в прежнем относительном порядке:
// вердикт уровня узла читается раньше деградаций полей.
//
// fresh == nil означает «не считали» и сохраняет это различие: узел без
// авторитетных записей получает nil, а не пустой список (см. комментарий у
// Node.Warnings).
func (n *Node) ReplaceDerivedWarnings(fresh []NodeWarning) {
	if n == nil {
		return
	}
	var keep []NodeWarning
	for _, w := range n.Warnings {
		if isAuthoritativeWarning(w.Code) {
			keep = append(keep, w)
		}
	}
	if len(keep) == 0 {
		n.Warnings = fresh
		return
	}
	// Свежий набор мог принести код с тем же именем (его не поставит ни один
	// санитайзер, но полагаться на это молча нельзя): авторитетная запись
	// побеждает — она факт, а не пересчёт.
	out := make([]NodeWarning, 0, len(keep)+len(fresh))
	out = append(out, keep...)
	for _, w := range fresh {
		if isAuthoritativeWarning(w.Code) {
			continue
		}
		out = append(out, w)
	}
	n.Warnings = out
}

// CarryCoreVerdict переносит вердикт ядра со СТАРОГО узла на свежий при
// обновлении подписки (refreshMergedNode) — по правилу «вердикт привязан к
// ТЕЛУ» (решение владельца 18.09.2026, CANON §9.4).
//
//   - тело то же → вердикт держится: перепроверка дала бы тот же отказ, и
//     узел остаётся выключенным С объяснением. Без этого он остался бы
//     выключенным БЕЗ объяснения, то есть стал бы неотличим от выключенного
//     рукой;
//   - тело другое → вердикт недействителен: запись стирается, и узел
//     ВКЛЮЧАЕТСЯ обратно — провайдер починил запись, и следующая сборка
//     обязана проверить её заново.
//
// Включается только узел, выключенный СТРАХОВКОЙ. Выключенный человеком (нет
// записи `core_rejected`) сменой тела не оживает: его выключенность — решение
// человека, а не вердикт о теле.
//
// Вызывается ПОСЛЕ переноса Enabled: метод и правит этот флажок.
func (fresh *Node) CarryCoreVerdict(old *Node) {
	if fresh == nil || old == nil {
		return
	}
	reason := old.CoreRejectedReason()
	if reason == "" {
		return // выключал человек (или узел включён) — переносить нечего
	}
	if fresh.IsUnsupported() {
		// Запись сломалась у провайдера: её выключенность объясняет
		// собственный Reason, и вердикт о ПРЕЖНЕМ теле к ней неприменим.
		return
	}
	if !BodiesEquivalent(old.Body, fresh.Body) {
		// Тело другое — вердикт о прежнем теле недействителен.
		fresh.Enabled = true
		fresh.ClearCoreRejected()
		return
	}
	// То же тело: вердикт держится, свежие производные коды остаются.
	derived := fresh.Warnings
	fresh.Warnings = []NodeWarning{coreRejectedWarning(reason)}
	fresh.ReplaceDerivedWarnings(derived)
	fresh.Enabled = false
}

// RevalidateCoreVerdictAfterBodyChange снимает вердикт ядра, когда тело узла
// изменили ПРАВКОЙ (форма тела, regen из исходника, форма AmneziaWG).
//
// Та же норма, что у refetch: вердикт привязан к телу, и другое тело его
// отменяет — запись стирается, узел включается обратно, следующая сборка
// проверит его заново.
//
// before — тело ДО правки. Зовётся ПОСЛЕ того, как новое тело и производные
// коды уже записаны в узел (ReplaceDerivedWarnings вердикт бережёт — именно
// поэтому решение о снятии принимается отдельным шагом, а не пересчётом).
func (n *Node) RevalidateCoreVerdictAfterBodyChange(before []byte) {
	if n == nil || n.CoreRejectedReason() == "" {
		return
	}
	if BodiesEquivalent(before, n.Body) {
		return // тело то же — вердикт держится
	}
	n.Enabled = true
	n.ClearCoreRejected()
}

// BodiesEquivalent — семантическое сравнение тел узла: одинаковы ли они как
// ДАННЫЕ, независимо от порядка ключей и форматирования.
//
// Зеркалит `bodiesEquivalent` из core/config (node_materialize.go): core/state
// — leaf-пакет модели и core/config не импортирует (направление зависимости
// обратное). Правило одно и то же — канонизация через json.Marshal
// разобранного значения и сравнение байт.
//
// Оба тела пусты = одинаковы (узел без тела: auto, chain). Одно пустое —
// разные. Неразбираемое тело даёт «разные»: судить о равенстве того, чего мы
// не понимаем, нельзя, и fail-open здесь безопаснее — вердикт снимется, узел
// включится, и следующая сборка вынесет его заново.
func BodiesEquivalent(a, b []byte) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	var av, bv interface{}
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return false
	}
	aj, aerr := json.Marshal(av)
	bj, berr := json.Marshal(bv)
	if aerr != nil || berr != nil {
		return false
	}
	return bytes.Equal(aj, bj)
}
