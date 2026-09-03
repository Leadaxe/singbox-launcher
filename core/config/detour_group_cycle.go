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
	"strings"

	"singbox-launcher/internal/debuglog"
)

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
// Вторым значением возвращает теги выброшенных: пользователь задал detour и
// вправе знать, почему узел не появился в группе.
func dropNodesDetouringThroughGroup(nodes []*ParsedNode, groupTag string) ([]*ParsedNode, []string) {
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
	dropped := make([]string, 0, cyclic)
	for _, n := range nodes {
		if nodeDetourTarget(n) == groupTag {
			debuglog.WarnLog("detour: node %q dials through %q and therefore is not among its members "+
				"(otherwise the core will not start: dependency ring)", n.Tag, groupTag)
			dropped = append(dropped, n.Tag)
			continue
		}
		out = append(out, n)
	}
	return out, dropped
}
