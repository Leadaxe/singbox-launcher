package build

import (
	"fmt"
	"strings"

	"singbox-launcher/core/netiface"
	"singbox-launcher/core/template"
)

// bindInterfaceVar — имя переменной шаблона, несущей route.default_interface.
const bindInterfaceVar = "bind_interface"

// warnBindInterface проверяет, что выбранный исходящий интерфейс существует
// на машине, для которой собирается конфиг, и что у него есть адрес.
//
// Проверка осмысленна только для локального таргета: интерфейсы удалённой
// машины отсюда не видны, и сверка с локальными дала бы ложную тревогу на
// каждом remote-конфиге.
//
// Не ошибка, а warning: пользователь мог собирать конфиг с вынутым кабелем,
// намереваясь воткнуть его позже, и запрещать это — значит ломать валидный
// сценарий ради подозрения.
func warnBindInterface(vars map[string]string, target template.TargetSpec, res *Result) {
	name := strings.TrimSpace(vars[bindInterfaceVar])
	if name == "" || target.Normalized().IsRemote() {
		return
	}
	for _, ifc := range netiface.ListOrEmpty() {
		if strings.EqualFold(ifc.Name, name) {
			// SPEC 113-F: чужой туннель (системный WireGuard/AmneziaWG) —
			// ВАЛИДНЫЙ выбор, и warning ему не полагается: ядро через него
			// выйдет, а лишнее предупреждение в отчёте сборки заставляло бы
			// искать поломку там, где всё сделано намеренно. Единственное, что
			// остаётся проверить, — поднят ли он, ровно как у любого другого.
			if !ifc.Up {
				res.Validation.Warnings = append(res.Validation.Warnings, fmt.Sprintf(
					"outbound interface %q is down — sing-box will have no uplink until it comes up", name))
			}
			return
		}
	}
	// Интерфейс есть, но не прошёл фильтр пригодности. Причину называем по
	// факту: действия у них разные, а прежняя формулировка «нет IP-адреса»
	// была для собственного TUN прямой ложью — адрес у него есть.
	switch fit := netiface.Fitness(name); {
	case fit == netiface.UnfitTunnel:
		res.Validation.Warnings = append(res.Validation.Warnings, fmt.Sprintf(
			"outbound interface %q is the core's own tunnel or a tunnel without an address — sing-box cannot use it as an uplink", name))
		return
	case fit == netiface.UnfitLoopback:
		res.Validation.Warnings = append(res.Validation.Warnings, fmt.Sprintf(
			"outbound interface %q is loopback — no traffic will leave the machine through it", name))
		return
	case netiface.Exists(name):
		res.Validation.Warnings = append(res.Validation.Warnings, fmt.Sprintf(
			"outbound interface %q has no IP address — sing-box will have no uplink through it", name))
		return
	}
	res.Validation.Warnings = append(res.Validation.Warnings, fmt.Sprintf(
		"outbound interface %q does not exist on this machine — sing-box will fail to dial through it", name))
}
