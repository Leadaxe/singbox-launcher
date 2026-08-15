package ui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// SPEC 095 I1–I4 — окно «Info»: всё, что известно об узле.
//
// Открывается из контекстного меню строки. Отдельной кнопки в строке нет:
// она и так плотная, а Info нужен изредка.
//
// Окно отдельное (Application.NewWindow), а не модальный диалог: форма
// высокая, и Fyne раздувает такой попап на весь экран.

// showNodeInfoWindow открывает окно с полной информацией об узле.
//
// cfgPath — config.json той области, из которой открыли строку (см.
// effectiveNodeConfigPath): для узла удалённой машины это её собранный
// конфиг, локальный описывает другое ядро.
func showNodeInfoWindow(ac *core.AppController, proxy api.ProxyInfo, cfgPath string) {
	if ac == nil || ac.FileService == nil || ac.UIService == nil {
		return
	}

	nodes := wizardbusiness.LoadConfigNodes(cfgPath)
	node := nodes.Lookup(proxy.Name)

	win := fyne.CurrentApp().NewWindow(
		locale.Tf("servers.node_info_title", proxy.DisplayOrName()))

	body := container.NewVBox()

	// Шапка: то, что видно в списке.
	body.Add(sectionHeader(locale.T("servers.node_info_section_general")))
	body.Add(infoRow(locale.T("servers.node_info_tag"), proxy.Name))
	if display := proxy.DisplayOrName(); display != proxy.Name {
		body.Add(infoRow(locale.T("servers.node_info_display_name"), display))
	}
	body.Add(infoRow(locale.T("servers.node_info_delay"), formatDelay(proxy.Delay)))

	if node == nil {
		// Узла нет в конфиге: гонка перегенерации либо служебный outbound.
		body.Add(widget.NewLabel(locale.T("servers.node_info_not_in_config")))
		finishNodeInfoWindow(win, body)
		return
	}

	body.Add(infoRow(locale.T("servers.node_info_type"), node.Type))
	body.Add(infoRow(locale.T("servers.node_info_kind"), node.Kind))

	if node.Server != "" {
		body.Add(infoRow(locale.T("servers.node_info_server"),
			fmt.Sprintf("%s:%d", node.Server, node.ServerPort)))
	}
	if node.Transport != "" {
		body.Add(infoRow(locale.T("servers.node_info_transport"), node.Transport))
	}
	if node.Security != "" {
		body.Add(infoRow(locale.T("servers.node_info_security"), node.Security))
	}
	if node.Detour != "" {
		body.Add(infoRow(locale.T("servers.node_info_detour"), node.Detour))
	}

	// Группа: состав и параметры проверки.
	if node.IsGroup() {
		body.Add(widget.NewSeparator())
		body.Add(sectionHeader(locale.Tf("servers.node_info_section_group", len(node.GroupMembers))))

		for _, row := range groupModeRows(node) {
			body.Add(row)
		}
		if url, ok := node.Raw["url"].(string); ok && url != "" {
			body.Add(infoRow(locale.T("servers.node_info_test_url"), url))
		}
		if interval, ok := node.Raw["interval"].(string); ok && interval != "" {
			body.Add(infoRow(locale.T("servers.node_info_test_interval"), interval))
		}
		if def, ok := node.Raw["default"].(string); ok && def != "" {
			body.Add(infoRow(locale.T("servers.node_info_default"), def))
		}
		// SPEC 097: активный участник группы помечается маркером — без него
		// окно показывало N равнозначных строк, и понять, через кого реально
		// идёт трафик, было нельзя.
		//
		// Выбор спрашиваем у ядра ОТДЕЛЬНЫМ запросом по тегу ЭТОЙ группы:
		// ProxyInfo.Now заполнен только у той группы, которую грузит вкладка,
		// а окно открывают для любой, в том числе вложенной. Запрос уходит в
		// горутину — сеть, и для удалённой машины это RTT до роутера.
		memberRows := make(map[string]*widget.Label, len(node.GroupMembers))
		for _, member := range node.GroupMembers {
			row := memberRow(member, nodes)
			memberRows[member] = row
			body.Add(row)
		}
		// Перерисовка метки: снимаем её со старого участника и ставим новому.
		// Идемпотентна — приходит на каждый кадр подписки, включая повтор
		// того же значения.
		markSelected := func(selected string) {
			for tag, row := range memberRows {
				want := tag == selected
				has := strings.Contains(row.Text, "   ● ")
				if want == has {
					continue
				}
				if want {
					row.SetText(strings.Replace(row.Text, "   · ", "   ● ", 1))
					row.TextStyle = fyne.TextStyle{Bold: true}
				} else {
					row.SetText(strings.Replace(row.Text, "   ● ", "   · ", 1))
					row.TextStyle = fyne.TextStyle{}
				}
				row.Refresh()
			}
		}

		// Живая подписка: ядро пушит смену выбора по событию, поэтому окно
		// отражает перевыбор само. Разовый снимок «замёрз» бы — у least_test
		// перевыбор случается по результатам url-теста в любой момент.
		if sub, ok := EffectiveProxyTransport(ac).(groupSelectionSource); ok {
			if cancel, err := sub.SubscribeGroupSelection(proxy.Name, func(selected string) {
				fyne.Do(func() { markSelected(selected) })
			}); err == nil {
				win.SetOnClosed(cancel) // иначе стрим и горутина переживут окно
			} else {
				debuglog.WarnLog("node info: subscribe group selection: %v", err)
			}
		} else {
			// Транспорт без подписки (Clash HTTP) — разовый снимок.
			go func(group string) {
				_, now, err := EffectiveProxyTransport(ac).GroupProxies(group)
				if err != nil || strings.TrimSpace(now) == "" {
					return
				}
				fyne.Do(func() { markSelected(now) })
			}(proxy.Name)
		}

		// Живой пул балансировщика — только urltest-группы и только в
		// daemon-режиме (lx-RPC GetPool доступен по gRPC-каналу). Секция
		// наполняется асинхронно после показа окна: statics выше — из
		// конфига, а тут — текущее состояние работающего ядра.
		// Секция пула показывается ТОЛЬКО если у группы он реально есть.
		//
		// Режим (least_test | round_robin) по gRPC не отдаётся: в Group его
		// нет, type у обоих "urltest". Из локального config.json читать
		// нельзя — для удалённой машины он описывает чужое ядро. Поэтому
		// различаем по контракту ядра: непустые слоты GetPool = round_robin,
		// пустые = у группы нет балансировщика.
		//
		// Отсюда порядок: сперва спрашиваем пул, и лишь при непустом ответе
		// СОЗДАЁМ секцию. Пустой ответ (в т.ч. Unimplemented без
		// with_lx_command) не рисует ничего — про пул не говорим там, где
		// балансировки нет.
		if node.Type == "urltest" && ac.DaemonPoolAvailable() {
			poolBox := container.NewVBox()
			body.Add(poolBox)
			go func(group string) {
				slots, err := ac.DaemonPoolSlots(group)
				fyne.Do(func() {
					switch {
					case err != nil || len(slots) == 0:
						// Ошибка или пустой пул — группа не балансируемая
						// (или RPC недоступен). Секцию не создаём вовсе.
						return
					default:
						poolBox.Add(widget.NewSeparator())
						poolBox.Add(sectionHeader(locale.T("servers.node_info_section_pool")))
						active := strings.TrimSpace(proxy.Now)
						if active == "" {
							active, _ = node.Raw["now"].(string)
						}
						for _, slot := range slots {
							delay := "—"
							if slot.Delay > 0 {
								delay = fmt.Sprintf("%d ms", slot.Delay)
							}
							marker := " "
							if active != "" && slot.Tag == active {
								marker = "●"
							}
							row := widget.NewLabel(fmt.Sprintf("  %s #%d  %s  ·  %s", marker, slot.Slot, slot.Tag, delay))
							row.Truncation = fyne.TextTruncateEllipsis
							if marker == "●" {
								row.TextStyle = fyne.TextStyle{Bold: true}
							}
							poolBox.Add(row)
						}
					}
					poolBox.Refresh()
				})
			}(proxy.Name)
		}
	}

	// TLS-подробности отдельной секцией: их много и они длинные.
	if tlsRows := tlsInfoRows(node); len(tlsRows) > 0 {
		body.Add(widget.NewSeparator())
		body.Add(sectionHeader(locale.T("servers.node_info_section_tls")))
		for _, row := range tlsRows {
			body.Add(row)
		}
	}

	// Транспорт с параметрами (path/host/service_name).
	if trRows := transportInfoRows(node); len(trRows) > 0 {
		body.Add(widget.NewSeparator())
		body.Add(sectionHeader(locale.T("servers.node_info_section_transport")))
		for _, row := range trRows {
			body.Add(row)
		}
	}

	// JSON — отдельной вкладкой: он длинный и на общей странице оттеснял бы
	// разобранные поля вниз, ради которых окно и открывают.
	jsonText := prettyNodeJSON(node.Raw)
	jsonEntry := widget.NewMultiLineEntry()
	jsonEntry.SetText(jsonText)
	jsonEntry.Wrapping = fyne.TextWrapOff

	jsonTab := container.NewBorder(
		nil,
		widget.NewButton(locale.T("servers.node_info_copy_json"), func() {
			setClipboard(jsonText)
		}),
		nil, nil,
		jsonEntry,
	)

	tabs := container.NewAppTabs(
		container.NewTabItem(locale.T("servers.node_info_tab_details"), withScrollGutter(body)),
		container.NewTabItem(locale.T("servers.node_info_section_json"), jsonTab),
	)

	win.SetContent(tabs)
	win.Resize(fyne.NewSize(620, 680))
	win.CenterOnScreen()
	win.Show()
}

// withScrollGutter оборачивает содержимое в скролл с отступом под полосу
// прокрутки, чтобы она не наезжала на значения полей.
func withScrollGutter(body fyne.CanvasObject) fyne.CanvasObject {
	gutter := canvas.NewRectangle(color.Transparent)
	gutter.SetMinSize(fyne.NewSize(nodeInfoScrollbarGutter, 0))
	return container.NewScroll(container.NewBorder(nil, nil, nil, gutter, body))
}

// finishNodeInfoWindow показывает окно без вкладок — используется, когда узла
// нет в конфиге и показывать в JSON-вкладке нечего.
func finishNodeInfoWindow(win fyne.Window, body *fyne.Container) {
	win.SetContent(withScrollGutter(body))
	win.Resize(fyne.NewSize(620, 400))
	win.CenterOnScreen()
	win.Show()
}

// groupModeRows разворачивает параметры отбора и балансировки группы.
//
// Раньше окно показывало только url/interval, и всё, что относится к
// round_robin, оставалось невидимым: режим, окно допуска, липкость сессий.
// Эти поля определяют, как именно расходится трафик, — без них «Info» о
// группе ничего толком не сообщает.
func groupModeRows(node *wizardbusiness.ConfigNode) []*fyne.Container {
	var rows []*fyne.Container

	// mode — расширение форка (SPEC 088). Отсутствие означает умолчание
	// urltest: один самый быстрый узел по замерам.
	if node.Type == "urltest" {
		mode, _ := node.Raw["mode"].(string)
		if strings.TrimSpace(mode) == "" {
			mode = "least_test"
		}
		rows = append(rows, infoRow(locale.T("servers.node_info_group_mode"), mode))
	}

	// tolerance — окно, в пределах которого узлы считаются равными по
	// скорости: без него least_test дёргал бы выбор на каждой миллисекунде.
	if tolerance, ok := jsonNumberString(node.Raw["tolerance"]); ok {
		rows = append(rows, infoRow(locale.T("servers.node_info_tolerance"), tolerance+" ms"))
	}
	if idle, ok := node.Raw["idle_timeout"].(string); ok && idle != "" {
		rows = append(rows, infoRow(locale.T("servers.node_info_idle_timeout"), idle))
	}

	balancer, ok := node.Raw["balancer"].(map[string]interface{})
	if !ok {
		return rows
	}

	// pool — сколько узлов держать в раздаче; 0 означает «весь состав».
	if pool, ok := jsonNumberString(balancer["pool"]); ok {
		rows = append(rows, infoRow(locale.T("servers.node_info_pool"), pool))
	}
	if poolTolerance, ok := jsonNumberString(balancer["pool_tolerance"]); ok {
		rows = append(rows, infoRow(locale.T("servers.node_info_pool_tolerance"), poolTolerance+" ms"))
	}

	// sticky_hash — по каким признакам соединение закрепляется за узлом.
	// Сентинел ["none"] означает «липкость выключена» (SPEC 210).
	if raw, ok := balancer["sticky_hash"].([]interface{}); ok {
		keys := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok {
				keys = append(keys, s)
			}
		}
		value := strings.Join(keys, ", ")
		if len(keys) == 1 && keys[0] == "none" {
			value = locale.T("servers.node_info_sticky_off")
		}
		if value != "" {
			rows = append(rows, infoRow(locale.T("servers.node_info_sticky_hash"), value))
		}
	}
	if ttl, ok := balancer["sticky_ttl"].(string); ok && ttl != "" {
		rows = append(rows, infoRow(locale.T("servers.node_info_sticky_ttl"), ttl))
	}

	return rows
}

// jsonNumberString приводит числовое поле JSON к строке.
//
// После json.Unmarshal целые приезжают как float64 — печатать их «%v» дало бы
// «50» либо «5e+01» в зависимости от величины.
func jsonNumberString(v interface{}) (string, bool) {
	switch x := v.(type) {
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case int:
		return strconv.Itoa(x), true
	case int64:
		return strconv.FormatInt(x, 10), true
	case json.Number:
		return x.String(), true
	default:
		return "", false
	}
}

// sectionHeader — заголовок секции.
func sectionHeader(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.TextStyle.Bold = true
	return l
}

// nodeInfoKeyColumnWidth — ширина колонки с названиями полей.
//
// Фиксированная: без неё каждая строка сама решает, сколько занять под ключ, и
// колонка значений разъезжается — «Tag» и «REALITY public key» дают разный
// отступ, читать невозможно.
const nodeInfoKeyColumnWidth = 168

// nodeInfoScrollbarGutter — отступ справа под полосу прокрутки.
//
// В проекте принято резервировать место под скроллбар, чтобы он не наезжал на
// содержимое (см. components.ScrollbarGutterWidth).
const nodeInfoScrollbarGutter = 5

// infoRow — строка «ключ: значение».
//
// Значение в Entry, а не Label: его можно выделить и скопировать, а длинное
// значение не растягивает окно (Entry сжимается, Label — нет).
func infoRow(key, value string) *fyne.Container {
	keyLabel := widget.NewLabel(key)
	keyLabel.TextStyle.Bold = true
	keyLabel.Truncation = fyne.TextTruncateEllipsis

	// Распорка задаёт колонке ключей одинаковую ширину во всех строках.
	keySpacer := canvas.NewRectangle(color.Transparent)
	keySpacer.SetMinSize(fyne.NewSize(nodeInfoKeyColumnWidth, 0))
	keyCell := container.NewStack(keySpacer, keyLabel)

	valueEntry := widget.NewEntry()
	valueEntry.SetText(value)
	valueEntry.Wrapping = fyne.TextWrapOff

	return container.NewBorder(nil, nil, keyCell, nil, valueEntry)
}

// memberRow — строка члена группы с его собственным подзаголовком.
func memberRow(tag string, nodes *wizardbusiness.ConfigNodes) *widget.Label {
	line := "   · " + tag
	if member := nodes.Lookup(tag); member != nil {
		if parts := member.SubtitleParts(); len(parts) > 0 {
			line += "  (" + strings.Join(parts, "·") + ")"
		}
	} else {
		// Висячая ссылка: ядро такую группу не поднимет.
		line += "  ⚠"
	}
	l := widget.NewLabel(line)
	l.Truncation = fyne.TextTruncateEllipsis
	return l
}

// tlsInfoRows разворачивает блок tls в строки.
func tlsInfoRows(node *wizardbusiness.ConfigNode) []*fyne.Container {
	tls, ok := node.Raw["tls"].(map[string]interface{})
	if !ok {
		return nil
	}

	var rows []*fyne.Container
	if sni, ok := tls["server_name"].(string); ok && sni != "" {
		rows = append(rows, infoRow(locale.T("servers.node_info_sni"), sni))
	}
	if alpn, ok := tls["alpn"].([]interface{}); ok && len(alpn) > 0 {
		parts := make([]string, 0, len(alpn))
		for _, a := range alpn {
			if s, ok := a.(string); ok {
				parts = append(parts, s)
			}
		}
		rows = append(rows, infoRow("ALPN", strings.Join(parts, ", ")))
	}
	if insecure, ok := tls["insecure"].(bool); ok && insecure {
		rows = append(rows, infoRow(locale.T("servers.node_info_insecure"), "true"))
	}
	if utls, ok := tls["utls"].(map[string]interface{}); ok {
		if fp, ok := utls["fingerprint"].(string); ok && fp != "" {
			rows = append(rows, infoRow(locale.T("servers.node_info_fingerprint"), fp))
		}
	}
	if reality, ok := tls["reality"].(map[string]interface{}); ok {
		if pk, ok := reality["public_key"].(string); ok && pk != "" {
			rows = append(rows, infoRow(locale.T("servers.node_info_reality_key"), pk))
		}
		if sid, ok := reality["short_id"].(string); ok && sid != "" {
			rows = append(rows, infoRow(locale.T("servers.node_info_reality_short_id"), sid))
		}
	}
	return rows
}

// transportInfoRows разворачивает блок transport в строки.
func transportInfoRows(node *wizardbusiness.ConfigNode) []*fyne.Container {
	tr, ok := node.Raw["transport"].(map[string]interface{})
	if !ok {
		return nil
	}

	var rows []*fyne.Container
	if path, ok := tr["path"].(string); ok && path != "" {
		rows = append(rows, infoRow(locale.T("servers.node_info_path"), path))
	}
	if headers, ok := tr["headers"].(map[string]interface{}); ok {
		if host, ok := headers["Host"].(string); ok && host != "" {
			rows = append(rows, infoRow("Host", host))
		}
	}
	if sn, ok := tr["service_name"].(string); ok && sn != "" {
		rows = append(rows, infoRow(locale.T("servers.node_info_service_name"), sn))
	}
	if med, ok := tr["max_early_data"].(float64); ok && med > 0 {
		rows = append(rows, infoRow(locale.T("servers.node_info_early_data"),
			fmt.Sprintf("%d", int(med))))
	}
	return rows
}

// prettyNodeJSON форматирует объект узла с отсортированными ключами.
func prettyNodeJSON(raw map[string]interface{}) string {
	if raw == nil {
		return "{}"
	}
	// json.MarshalIndent сортирует ключи map сам — порядок стабилен между
	// открытиями окна, и глазу не приходится заново искать поле.
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		// Падать нельзя: окно — диагностический инструмент, и оно обязано
		// открыться даже на странном узле.
		keys := make([]string, 0, len(raw))
		for k := range raw {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return fmt.Sprintf("// %v\n%s", err, strings.Join(keys, "\n"))
	}
	return string(data)
}

// formatDelay человекочитаемо описывает последний замер.
func formatDelay(delay int64) string {
	switch {
	case delay > 0:
		return fmt.Sprintf("%d ms", delay)
	case delay == -1:
		return locale.T("servers.ping_button_error")
	default:
		return "—"
	}
}

// groupSelectionSource — транспорт, умеющий пушить смену выбранного узла
// группы (gRPC SubscribeGroups). Clash-транспорт его не реализует — там
// остаётся разовый снимок.
type groupSelectionSource interface {
	SubscribeGroupSelection(group string, onSelected func(string)) (func(), error)
}
