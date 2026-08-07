package subscription

import (
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// SPEC 094 фаза C, §322 — балансировщик Xray-элемента → узел-группа.
//
// Провайдеры отдают «🚀Авто | Лучший сервер» одним элементом с 15 узлами
// внутри и `routing.balancers[0]`, который эти узлы объединяет. Это ПУЛ
// балансировщика, а не 15 самостоятельных серверов: показывать их отдельными
// строками с техническими тегами (`… proxy-45-195-228-192-direct`) —
// раскрывать пользователю внутреннюю кухню провайдера.
//
// Правильное поведение (как в LxBox §322): элемент с балансировщиком даёт
// узел-группу с чистым именем из remarks, а его члены — узлы с
// различителями, которые группа и объединяет.

// xrayBalancerDefaultURL — цель проверки, если провайдер её не задал.
const xrayBalancerDefaultURL = "https://www.gstatic.com/generate_204"

// xrayBalancerDefaultInterval — период проверки по умолчанию.
const xrayBalancerDefaultInterval = "3m"

// xrayBalancerFromElement строит узел-группу из routing.balancers[0].
//
// Возвращает nil, если балансировщика нет или его состав пуст: пустой urltest
// роняет старт ядра.
//
// memberTags — итоговые теги узлов этого элемента в порядке появления.
// Состав берётся по ним, а не по `selector`: селектор Xray матчит теги
// ПРЕФИКСОМ в границах своего конфига, и тот же тег `proxy` встречается ещё в
// трёх десятках соседних элементов — общий матчинг растащил бы в пул всё
// подряд.
func xrayBalancerFromElement(
	root map[string]interface{},
	remarks string,
	elemIndex int,
	memberTags []string,
) *configtypes.ParsedNode {
	balancer, ok := xrayFirstBalancer(root)
	if !ok {
		return nil
	}
	if len(memberTags) == 0 {
		debuglog.DebugLog("Parser: Xray element %d: balancer has no members — skipped", elemIndex)
		return nil
	}

	label := strings.TrimSpace(remarks)
	if label == "" {
		label = strings.TrimSpace(xrayMapString(balancer, "tag"))
	}
	if label == "" {
		label = "auto"
	}

	members := make([]interface{}, 0, len(memberTags))
	for _, tag := range memberTags {
		members = append(members, tag)
	}

	outbound := map[string]interface{}{
		"tag":                       label,
		"type":                      "urltest",
		configtypes.GroupMembersKey: members,
		"url":                       xrayBalancerURL(root),
		"interval":                  xrayBalancerInterval(root),
	}

	debuglog.DebugLog("Parser: Xray element %d: balancer %q → group node with %d member(s)",
		elemIndex, label, len(memberTags))

	return &configtypes.ParsedNode{
		Tag:    label,
		Scheme: configtypes.SchemeGroup,
		// Группа не соединение: server/port у неё нет.
		Label:       label,
		Outbound:    outbound,
		SourceIndex: configtypes.UnsetSourceIndex,
	}
}

// xrayElementHasBalancer сообщает, есть ли у элемента балансировщик.
func xrayElementHasBalancer(root map[string]interface{}) bool {
	_, ok := xrayFirstBalancer(root)
	return ok
}

// xrayFirstBalancer возвращает routing.balancers[0].
//
// Схема допускает несколько балансировщиков; на практике провайдеры пишут
// один. Берём первый, остальные игнорируем — как в LxBox.
func xrayFirstBalancer(root map[string]interface{}) (map[string]interface{}, bool) {
	routing, ok := root["routing"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	balancers, ok := routing["balancers"].([]interface{})
	if !ok || len(balancers) == 0 {
		return nil, false
	}
	b, ok := balancers[0].(map[string]interface{})
	if !ok {
		return nil, false
	}
	return b, true
}

// xrayBalancerURL достаёт цель проверки из burstObservatory.pingConfig.
func xrayBalancerURL(root map[string]interface{}) string {
	if ping, ok := xrayPingConfig(root); ok {
		if dest := strings.TrimSpace(xrayMapString(ping, "destination")); dest != "" {
			return dest
		}
	}
	return xrayBalancerDefaultURL
}

// xrayBalancerInterval достаёт период проверки из burstObservatory.pingConfig.
//
// Go-длительности Xray («2m», «30s») совпадают по формату с тем, что ждёт
// sing-box, поэтому переносятся как есть. Голое число трактуется как секунды.
func xrayBalancerInterval(root map[string]interface{}) string {
	ping, ok := xrayPingConfig(root)
	if !ok {
		return xrayBalancerDefaultInterval
	}
	raw := strings.TrimSpace(xrayMapString(ping, "interval"))
	if raw == "" {
		return xrayBalancerDefaultInterval
	}
	if _, err := strconv.Atoi(raw); err == nil {
		return raw + "s"
	}
	return raw
}

// xrayPingConfig возвращает burstObservatory.pingConfig.
func xrayPingConfig(root map[string]interface{}) (map[string]interface{}, bool) {
	obs, ok := root["burstObservatory"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	ping, ok := obs["pingConfig"].(map[string]interface{})
	return ping, ok
}
