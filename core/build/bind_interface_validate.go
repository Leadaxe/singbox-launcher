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
			if !ifc.Up {
				res.Validation.Warnings = append(res.Validation.Warnings, fmt.Sprintf(
					"outbound interface %q is down — sing-box will have no uplink until it comes up", name))
			}
			return
		}
	}
	// Интерфейс есть, но не прошёл фильтр пригодности — почти всегда это
	// воткнутый кабель без адреса. Формулировка разделяет два случая, потому
	// что действия у них разные: получить адрес против исправить имя.
	if netiface.Exists(name) {
		res.Validation.Warnings = append(res.Validation.Warnings, fmt.Sprintf(
			"outbound interface %q has no IP address — sing-box will have no uplink through it", name))
		return
	}
	res.Validation.Warnings = append(res.Validation.Warnings, fmt.Sprintf(
		"outbound interface %q does not exist on this machine — sing-box will fail to dial through it", name))
}
