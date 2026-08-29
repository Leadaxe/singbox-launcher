// Package business — приём вставленного sing-box JSON как источника.
//
// Файл sources_json.go: ветка Add для JSON-документа. Поле Add исторически
// принимало только ссылки (подписка либо share-URI); человек с готовым
// outbound'ом на руках вставить его не мог — построчный классификатор молча
// выбрасывал весь документ. Здесь JSON выкусывается до цикла по строкам и
// превращается в Source(server) с ConfigJSON, минуя URI целиком.
package business

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/config/subscription"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
)

// carveSingboxJSON выкусывает вставленный sing-box JSON перед построчным
// разбором. Причина та же, что у WG-conf блоков выше: JSON — многострочный
// документ, и цикл по строкам его уничтожит (ни одна строка не является ни
// подпиской, ни share-URI, поэтому весь блок молча пропадал).
//
// Возвращает ноды и признак того, что вход был JSON'ом. Признак нужен, чтобы
// отличить «это не JSON» от «JSON, но пустой/битый»: в первом случае вход
// уходит дальше по обычному пути, во втором — пользователь должен увидеть
// ошибку, а не «no valid URLs to add».
func carveSingboxJSON(input string) (nodes []singboxJSONNode, isJSON bool, err error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || (trimmed[0] != '{' && trimmed[0] != '[') {
		return nil, false, nil
	}

	kind := subscription.ClassifySubscriptionBody(trimmed)
	switch kind {
	case subscription.BodyKindSingboxOutbound:
		// Одиночный outbound идёт через NodeFromManualConfigJSON: он, в отличие
		// от импорта тела, сохраняет неизвестные типы passthrough и кладёт
		// объект целиком в ConfigJSON — ровно то, что нужно ручной вставке.
		node, perr := subscription.NodeFromManualConfigJSON([]byte(trimmed))
		if perr != nil {
			return nil, true, perr
		}
		compact, cerr := compactJSON(trimmed)
		if cerr != nil {
			return nil, true, cerr
		}
		return []singboxJSONNode{{Label: node.Tag, ConfigJSON: compact}}, true, nil

	case subscription.BodyKindSingboxOutboundArray,
		subscription.BodyKindSingboxConfig,
		subscription.BodyKindSingboxConfigArray:
		return carveSingboxJSONMulti(trimmed, kind)
	}

	// '{'/'[' без sing-box признаков — не наш JSON (Xray-массив, wg-quick
	// INI и прочее). Отдаём обычному пути, он разберётся.
	return nil, false, nil
}

// carveSingboxJSONMulti разбирает многоузловые формы (массив outbound'ов,
// целый конфиг, массив конфигов). Каждый узел становится отдельным Source —
// так же, как каждый share-URI из вставленного списка.
func carveSingboxJSONMulti(body string, kind subscription.BodyKind) ([]singboxJSONNode, bool, error) {
	res, err := subscription.ParseSingboxBody(body, kind, nil)
	if err != nil {
		return nil, true, err
	}
	if len(res.Nodes) == 0 {
		if len(res.UnsupportedTypes) > 0 {
			return nil, true, fmt.Errorf("no supported outbounds: %s",
				strings.Join(res.UnsupportedTypes, ", "))
		}
		return nil, true, fmt.Errorf("no outbounds found")
	}

	nodes := make([]singboxJSONNode, 0, len(res.Nodes))
	for _, n := range res.Nodes {
		if n == nil || len(n.Outbound) == 0 {
			continue
		}
		raw, merr := json.Marshal(n.Outbound)
		if merr != nil {
			debuglog.WarnLog("Parser: skipping pasted outbound %q: %v", n.Tag, merr)
			continue
		}
		nodes = append(nodes, singboxJSONNode{Label: n.Tag, ConfigJSON: raw})
	}
	if len(nodes) == 0 {
		return nil, true, fmt.Errorf("no outbounds found")
	}
	return nodes, true, nil
}

// singboxJSONNode — узел, вынутый из вставленного JSON: подпись для списка
// источников и компактное тело, которое ляжет в Source.ConfigJSON.
type singboxJSONNode struct {
	Label      string
	ConfigJSON []byte
}

// compactJSON сжимает документ, сохраняя порядок полей автора (json.Compact
// не пересобирает объект, в отличие от Unmarshal→Marshal).
func compactJSON(s string) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// AppendManualConfigJSON добавляет один вручную отредактированный outbound как
// Source(server). Отдельный вход, а не AppendURLsToSources: там тело проходит
// повторный разбор, а здесь важно сохранить объект ровно таким, каким его
// набрал человек — включая поля, которых наш парсер не знает.
func AppendManualConfigJSON(ctx UIUpdater, body []byte, label string) error {
	node, err := subscription.NodeFromManualConfigJSON(body)
	if err != nil {
		return err
	}

	compact, err := compactJSON(string(body))
	if err != nil {
		return err
	}

	model := ctx.Model()
	if strings.TrimSpace(label) == "" {
		label = node.Tag
	}
	if strings.TrimSpace(label) == "" {
		label = fmt.Sprintf("server-%d", len(model.Sources)+1)
	}

	model.Sources = append(model.Sources, corestate.Source{
		Node:       corestate.Node{Kind: corestate.SourceKindServer, Enabled: true},
		ID:         corestate.MakeULID(),
		Label:      label,
		ConfigJSON: compact,
	})

	model.BumpRevision()
	model.PreviewNeedsParse = true
	InvalidatePreviewCache(model)
	ctx.RefreshOutboundsConfiguratorList()
	return nil
}

// RelabelLastSources переименовывает источники, добавленные последним вызовом
// Append*: форма даёт один тег на всё добавленное, а общий путь Add берёт
// метку из фрагмента ссылки. Применяется только когда добавился ровно один
// источник — на список ссылок один тег не натянешь, там метки уже свои.
func RelabelLastSources(ctx UIUpdater, before int, label string) {
	label = strings.TrimSpace(label)
	model := ctx.Model()
	if label == "" || before < 0 || len(model.Sources)-before != 1 {
		return
	}
	model.Sources[len(model.Sources)-1].Label = label
	model.BumpRevision()
	ctx.RefreshOutboundsConfiguratorList()
}
