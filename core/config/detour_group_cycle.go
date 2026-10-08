// File detour_group_cycle.go — цикл «узел ходит через группу, в которую сам
// входит» (SPEC 077 follow-up).
//
// sanitizeNodeDetours ловит циклы ТОЛЬКО между узлами: там видна вся
// картина сразу — у каждого узла ровно одно ребро detour, и обход даёт
// ответ. Цикл через группу так не увидеть: состав группы считается позже,
// на проходе 2, когда фильтры уже применены.
//
// Между тем это ровно тот же класс ошибки, и он не гипотетический:
//
//	Proton.detour = "vpn ②"        — узел ходит через группу
//	"vpn ②".outbounds ∋ Proton     — и сам входит в её состав
//
// Ядро при старте разворачивает зависимости, упирается в кольцо и
// отвергает конфиг ЦЕЛИКОМ:
//
//	dependency[Proton] not found for outbound[proxy-out-auto]
//
// То есть пользователь остаётся без VPN, а сообщение показывает не на тот
// узел, который он трогал, а на группу, куда цикл дотянулся транзитивно, —
// найти причину по нему почти невозможно.
//
// Лечение: узел выпадает из СОСТАВА группы, но остаётся в конфиге и
// продолжает ходить своим переходом. Снять detour было бы хуже:
// пользователь задал его осознанно, и тихо отправить трафик напрямую
// значит нарушить ровно то, о чём он просил — SPEC 113-B запрещает это на
// всех уровнях. Кольцо рвётся по членству, а не по переходу; сам переход
// тут остаётся рабочим, поэтому носитель не выбрасывается (недоступная
// цель detour — другой случай, он разбирается fail-closed отдельно).
package config

import (
	"strconv"
	"strings"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
)

// emitDetourThroughGroupText — запасной текст записи, когда реестр не
// прочитался. Здесь, а не в emission_warning.go: чекер локали резолвит
// константы в пределах файла использования.
const emitDetourThroughGroupText = "%d node(s) left out of group %q: they dial through this very group (%s)"

// nodeDetourTarget — цель detour узла. Пусто, если её нет.
func nodeDetourTarget(n *ParsedNode) string {
	if n == nil || n.Outbound == nil {
		return ""
	}
	d, _ := n.Outbound["detour"].(string)
	return strings.TrimSpace(d)
}

// dropNodesDetouringThroughGroup убирает из отобранного состава узлы,
// которые ходят через эту же группу.
//
// Вторым значением возвращает выброшенные узлы: пользователь задал detour и
// вправе знать, почему узел не появился в группе, — а чтобы сказать ему это
// в отчёте с адресом, нужен не только тег, но и источник узла.
func dropNodesDetouringThroughGroup(nodes []*ParsedNode, groupTag string) ([]*ParsedNode, []*ParsedNode) {
	if groupTag == "" {
		return nodes, nil
	}
	// Сначала считаем, есть ли что выбрасывать: у подавляющего большинства
	// групп таких узлов нет, и вход возвращается как есть.
	cyclic := 0
	for _, n := range nodes {
		if nodeDetourTarget(n) == groupTag {
			cyclic++
		}
	}
	if cyclic == 0 {
		return nodes, nil
	}
	out := make([]*ParsedNode, 0, len(nodes))
	dropped := make([]*ParsedNode, 0, cyclic)
	for _, n := range nodes {
		if nodeDetourTarget(n) == groupTag {
			debuglog.WarnLog("detour: node %q dials through %q and therefore is not among its members "+
				"(otherwise the core will not start: dependency ring)", n.Tag, groupTag)
			dropped = append(dropped, n)
			continue
		}
		out = append(out, n)
	}
	return out, dropped
}

// detourCycleWarnings — узлы, не взятые в состав группы, через которую ходят,
// в форме предупреждения эмиссии с кодом node_detour_through_group.
//
// До этой записи выброс жил одним WARN в логе, а в Мастере выглядел как
// «фильтр не сработал»: пользователь ставит у Направления фильтр, видит в
// нём половину узлов и идёт чинить регулярку — притом что регулярка цела, а
// узлы ходят через само Направление (detour подписки). Адресат — источник
// узла: чинить это у источника (снять его detour на эту группу) или у
// Направления (не брать такие узлы), и строка Sources встаёт с пометкой у
// виновника, как у остальных деградаций эмиссии.
//
// Одна запись на пару (источник, группа), а не на узел: detour чаще всего
// задан у источника целиком, и подписка на полсотни узлов дала бы полсотни
// одинаковых строк — шум, а не сообщение. Узлы без источника (ручной
// outbound шаблона) собираются в свою запись без адреса.
func detourCycleWarnings(cycles []DetourCycle, proxies []ProxySource) []EmissionWarning {
	if len(cycles) == 0 {
		return nil
	}
	type key struct {
		source int
		group  string
	}
	order := make([]key, 0, 4)
	tags := make(map[key][]string, 4)
	for _, c := range cycles {
		k := key{source: c.SourceIndex, group: c.Group}
		if _, seen := tags[k]; !seen {
			order = append(order, k)
		}
		tags[k] = append(tags[k], c.Node)
	}
	out := make([]EmissionWarning, 0, len(order))
	for _, k := range order {
		list := tags[k]
		params := map[string]string{
			"group": k.group,
			"count": strconv.Itoa(len(list)),
			"tags":  detourMissingNodeList(list),
		}
		w := EmissionWarning{
			Text: registryWarningText(codeNodeDetourThroughGroup, params,
				locale.Tf(emitDetourThroughGroupText, len(list), k.group, detourMissingNodeList(list))),
			DirectionTag: k.group,
			Code:         codeNodeDetourThroughGroup,
			Params:       params,
		}
		if k.source != UnsetSourceIndex && k.source >= 0 && k.source < len(proxies) {
			ps := proxies[k.source]
			w.SourceID = strings.TrimSpace(ps.ID)
			w.SourceLabel = sourceDisplayName(ps, k.source)
		}
		out = append(out, w)
	}
	return out
}
