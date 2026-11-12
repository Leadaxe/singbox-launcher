// File xray_manual.go — узел из текста Xray, пришедшего источником ОДНОЙ
// записи (вставка в форму, вкладка Source/JSON, Regen из origin.raw вида
// `xray`; контракт 1.1.165, PARSING_PRINCIPLES §11).
//
// Разбор тот же, что у Xray-тела подписки: элементом там служит целый
// конфиг (так видны звенья `sockopt.dialerProxy`), поэтому любая форма
// входа сводится к массиву конфигов. Вторая реализация разбора Xray здесь
// не нужна и запрещена — расходились бы на первой же правке реестра.
package subscription

import (
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// Имена видов источника реестра для JSON Xray (source_kinds.json). Форма
// входа («голое тело / документ / массив») хранимым видом не является и
// видна за одну классификацию; здесь — единственное место, где по ней
// ветвится код.
const (
	sourceKindXrayOutbound      = "xray_outbound"
	sourceKindXrayConfig        = "xray_config"
	sourceKindXrayOutboundArray = "xray_outbound_array"
	sourceKindXrayConfigArray   = "xray_config_array"
)

// NodeFromXrayJSON — первый узел (не группа) из текста Xray любой формы:
// голый outbound, конфиг с `outbounds`, массив outbound'ов, массив конфигов.
// sourceKind — имя вида реестра (ClassifyOriginText); пусто — определяется
// здесь. Текст, который реестр Xray не считает, — ошибка.
func NodeFromXrayJSON(raw []byte, sourceKind string) (*configtypes.ParsedNode, error) {
	text := strings.TrimSpace(string(raw))
	if sourceKind == "" {
		_, sourceKind = ClassifyOriginText(text)
	}
	var arrayBody string
	switch sourceKind {
	case sourceKindXrayOutbound:
		arrayBody = `[{"outbounds":[` + text + `]}]`
	case sourceKindXrayConfig:
		arrayBody = XrayConfigToArray(text)
	case sourceKindXrayOutboundArray:
		arrayBody = `[{"outbounds":` + text + `}]`
	case sourceKindXrayConfigArray:
		arrayBody = text
	default:
		return nil, fmt.Errorf("not an Xray JSON source (registry kind %q)", sourceKind)
	}
	nodes, _, rejects, _, err := parseNodesFromXrayJSONArrayFull(arrayBody, nil)
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		if n != nil && n.Scheme != configtypes.SchemeGroup {
			return n, nil
		}
	}
	if len(rejects) > 0 {
		return nil, fmt.Errorf("xray: %s", rejects[0].Reason)
	}
	return nil, fmt.Errorf("xray: the document carries no node")
}
