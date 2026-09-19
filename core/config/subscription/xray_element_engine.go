// Package subscription: разбор ЭЛЕМЕНТА Xray-конфига ДВИЖКОМ реестра
// (SPEC 133).
//
// Пара к node_parser_engine.go, и роль та же: какая схема ведёт элемент,
// решает не код, а РЕЕСТР — своим `detect` по полю `protocol`. Имён схем в
// этом файле нет по той же причине, по какой их нет в самом движке.
//
// Что осталось ЗА пределами этого файла и почему: какой элемент становится
// узлом, какой хопом цепочки, а какой группой-балансером, решает уровень
// ДОКУМЕНТА (xray_json_array.go, xray_balancer.go). Это не свойство таблицы
// узла: она описывает ОДИН элемент и про соседей по массиву не знает.
package subscription

import (
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/linkmap"
	"singbox-launcher/core/config/registry"
)

// xrayElementKind — вид источника, чьи секции ведут элемент Xray-конфига.
//
// Имя живёт ЗДЕСЬ, у вызывающего, а не в движке: движок берёт вид
// параметром, и назови он его сам, вернулся бы скрытый диспетчер диалекта
// (страж TestNoSchemeNamesInEngine). Этот файл знает, какой ДОКУМЕНТ читает,
// и называть его вид ему законно.
const xrayElementKind = "xray"

// parseXrayElementByEngine разбирает outbound Xray-конфига секцией реестра.
//
// Контракт возврата тот же, что у прежнего рукописного конвертера, потому
// что вызывающий различает три исхода и путать их нельзя:
//
//   - (node, nil, true) — узел собран;
//   - (nil, *xrayUnsupportedProtocolError, true) — протокол не поддержан:
//     запись едет в список пропущенных, а не теряется молча (C1);
//   - (nil, err, true) — поддерживаемый протокол, битый элемент: причина
//     доезжает до пользователя как есть;
//   - (nil, nil, false) — реестр не собрался, разбор этого элемента движком
//     невозможен. Отдельным значением, а не ошибкой: это отказ СБОРКИ, а не
//     свойство элемента, и объявлять подписку протухшей из-за него нельзя.
func parseXrayElementByEngine(ob map[string]interface{}, label string) (*configtypes.ParsedNode, error, bool) {
	plans, err := linkmap.Planes()
	if err != nil {
		return nil, nil, false
	}
	reg, regErr := registry.Get()
	if regErr != nil {
		return nil, nil, false
	}

	protocol := strings.ToLower(strings.TrimSpace(xrayMapString(ob, "protocol")))
	if protocol == "" {
		return nil, fmt.Errorf("missing protocol"), true
	}
	// Служебные протоколы (freedom/blackhole/dns) узлами не становятся — и
	// это НЕ отказ: они законная часть конфига. Решает это реестр своим
	// набором, а не ветка здесь.
	if IsXrayServiceProtocol(protocol) {
		return nil, nil, true
	}

	scheme, plan, ok := linkmap.SelectElementSection(plans, xrayElementKind, ob)
	if !ok {
		// Ни одна секция не опознала элемент. Для вызывающего это тот же
		// исход, что и прежде: протокол не поддержан. Сюда же попадает
		// hysteria с версией вне {1, 2} — прежний конвертер ронял её
		// собственной ошибкой, теперь причина общая (DELTAS D133-28).
		return nil, &xrayUnsupportedProtocolError{Protocol: protocol}, true
	}

	bodyType := reg.SingboxType(scheme)
	res, execErr := linkmap.ParseElement(plan, ob, bodyType, nil)
	if execErr != nil {
		return nil, execErr, true
	}

	// Санитайзер реестра ЗДЕСЬ НЕ ЗОВЁТСЯ — и это не упущение, а тот же
	// порядок, что у входа ссылки. Судья значений стоит СТАДИЕЙ НИЖЕ, в
	// единственной точке рождения тела (materializeBody), и коды оттуда
	// приезжают с путём и значением. Позови его ещё и здесь, узел получил
	// бы КАЖДУЮ деградацию дважды: первый раз голым кодом, второй — с
	// параметрами (корпус xray/vless_fp_junk ловит это сразу).
	//
	// Прежний конвертер звал у hysteria собственный ранний проход
	// (SanitizeSingboxOutboundMap), но правил значений в нём давно нет —
	// они ушли в реестр волной W2d, и функция возвращает пустой список.
	body := res.Body

	node := &configtypes.ParsedNode{
		Tag:      xrayTagOrDefault(ob, scheme),
		Scheme:   scheme,
		Label:    label,
		Outbound: body,
	}
	applyEngineBody(node, plan, body)
	// «Главный секрет» узла — общая выемка входа sing-box
	// (singboxCredentialFromMap), а не своя копия: поле у обоих входов одно
	// и то же, и второй список полей разъехался бы с первым на первой же
	// схеме. Сама выемка — рукописный switch по схеме, и снимется она
	// вместе с переводом входа `singbox` на движок; заводить здесь ВТОРУЮ
	// такую же ради того, чтобы не трогать чужую, значит удвоить работу.
	node.UUID = singboxCredentialFromMap(body, scheme)
	if flow, _ := body["flow"].(string); flow != "" {
		node.Flow = flow
	}

	body["type"] = bodyType
	body["tag"] = node.Tag

	// Коды движка становятся деградациями узла. Коды санитайзера к ним
	// припишет конвейер (mergeWarnings) — порядок «сперва разбор, затем
	// судья значений» нормативен (CANON §6, Л14).
	for _, n := range res.Notes {
		if n.Path != "" {
			node.AddSourceWarning(n.Code, n.Path, n.Params)
			continue
		}
		if len(n.Params) > 0 {
			node.AddWarningWithParams(n.Code, n.Params)
			continue
		}
		node.AddWarning(n.Code)
	}
	return node, nil, true
}
