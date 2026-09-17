// File core_runtime_window.go — окно «что сейчас живёт в ядре».
//
// Заменило плоский диалог «Selector -> Active Outbound» (строки текста по
// одной на группу). Довод переделки: у ядра есть сущности, которых вкладка
// серверов не показывает вовсе — endpoint без роли выхода (tailscale с
// advertise_exit_node) ни в одну группу не входит, и для вкладки, которая
// умеет смотреть только на группы, его не существует. Узел работает, а
// наблюдать его негде.
//
// Окно — контейнер ВКЛАДОК по сущностям ядра. Вкладка сама знает, откуда
// берёт строки; окно про содержимое не знает ничего. Сегодня две вкладки
// (Направления, Endpoints), завтра — Inbounds или Tailnet, когда ядро
// начнёт отдавать их состояние; ни первые две, ни окно при этом не трогаются.
//
// Каждая строка узла несёт ⓘ → showNodeInfoWindow: это точка расширения.
// Когда форк отдаст статус tailnet (auth / IP / пиры / подтверждён ли анонс),
// он появится ВНУТРИ окна инфо tailscale-узла, а список останется прежним.
package ui

import (
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/textnorm"
	"singbox-launcher/ui/components"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// coreRuntimeDirection — одна строка вкладки Направлений: группа и то, что
// она выбрала прямо сейчас.
type coreRuntimeDirection struct {
	Group string
	// Active — выбранный узел; пусто = группа ничего не выбрала.
	Active api.ProxyInfo
	// Err — ошибка опроса группы; строка тогда показывает её вместо узла.
	Err error
}

// coreRuntimeEndpoint — одна строка вкладки Endpoints.
type coreRuntimeEndpoint struct {
	Node *wizardbusiness.ConfigNode
	// Live — снимок из ядра, если endpoint нашёлся в какой-нибудь группе
	// (тогда у него есть пинг). nil = ядро про него как о члене группы не
	// знает — это норма для tailscale без exit_node.
	Live *api.ProxyInfo
}

// coreRuntimeSnapshot — всё, что окно показывает; собирается в фоне одним
// проходом, чтобы вкладки не дёргали транспорт порознь.
type coreRuntimeSnapshot struct {
	Directions []coreRuntimeDirection
	Endpoints  []coreRuntimeEndpoint
}

// collectCoreRuntimeSnapshot опрашивает ядро и конфиг области scope.
//
// groups — список групп в порядке показа (тот же, что в выпадашке вкладки).
// Одного прохода по группам хватает на обе вкладки: Направления берут «now»,
// Endpoints — пинг тех endpoint'ов, что состоят в группах. Endpoint'ы вне
// групп приходят из config.json (ConfigNodes.Endpoints) — ядро их как
// членов групп не отдаёт.
func collectCoreRuntimeSnapshot(ac *core.AppController, scope services.ProxyScope, transport services.ProxyTransport, groups []string) coreRuntimeSnapshot {
	var snap coreRuntimeSnapshot

	// Живые снимки по тегу: endpoint, состоящий в группе, получит отсюда пинг.
	liveByTag := make(map[string]api.ProxyInfo)

	for _, g := range groups {
		proxies, now, err := transport.GroupProxies(g)
		dir := coreRuntimeDirection{Group: g, Err: err}
		if err == nil {
			for _, p := range proxies {
				liveByTag[p.Name] = p
			}
			if now != "" {
				if p, ok := liveByTag[now]; ok {
					dir.Active = p
				} else {
					// Выбран узел, которого нет в списке группы (вложенная
					// группа, direct-out): показываем хотя бы имя.
					dir.Active = api.ProxyInfo{Name: now, DisplayName: textnorm.NormalizeProxyDisplay(now)}
				}
			}
		}
		snap.Directions = append(snap.Directions, dir)
	}

	nodes := wizardbusiness.LoadConfigNodes(effectiveNodeConfigPath(ac, scope))
	for _, n := range nodes.Endpoints() {
		ep := coreRuntimeEndpoint{Node: n}
		if p, ok := liveByTag[n.Tag]; ok {
			pc := p
			ep.Live = &pc
		}
		snap.Endpoints = append(snap.Endpoints, ep)
	}
	sort.SliceStable(snap.Endpoints, func(i, j int) bool {
		return snap.Endpoints[i].Node.Tag < snap.Endpoints[j].Node.Tag
	})
	return snap
}

// showCoreRuntimeWindow открывает окно. Опрос идёт в фоне; окно показывается
// сразу с надписью «загрузка», чтобы клик по кнопке не выглядел потерянным.
//
// Application.NewWindow, а не модальный попап: содержимое растёт с числом
// групп и endpoint'ов, и попап Fyne тянулся бы до MinSize контента.
func showCoreRuntimeWindow(ac *core.AppController, scope services.ProxyScope, transport services.ProxyTransport, groups []string) {
	if ac == nil || ac.UIService == nil || ac.UIService.Application == nil {
		return
	}
	win := ac.UIService.Application.NewWindow(locale.T("Core runtime"))
	cfgPath := effectiveNodeConfigPath(ac, scope)

	dirBox := container.NewVBox(widget.NewLabel(locale.T("Loading…")))
	epBox := container.NewVBox(widget.NewLabel(locale.T("Loading…")))

	tabs := container.NewAppTabs(
		container.NewTabItem(locale.T("Directions"), components.WrapInScrollWithGutter(dirBox)),
		container.NewTabItem(locale.T("Endpoints"), components.WrapInScrollWithGutter(epBox)),
	)

	var reload func()
	reloadBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { reload() })
	reloadBtn.Importance = widget.LowImportance
	closeBtn := widget.NewButton(locale.T("Close"), func() { win.Close() })
	buttons := container.NewHBox(reloadBtn, layout.NewSpacer(), closeBtn)

	reload = func() {
		reloadBtn.Disable()
		go func() {
			snap := collectCoreRuntimeSnapshot(ac, scope, transport, groups)
			fyne.Do(func() {
				renderCoreRuntimeDirections(dirBox, ac, snap.Directions, cfgPath)
				renderCoreRuntimeEndpoints(epBox, ac, snap.Endpoints, cfgPath)
				reloadBtn.Enable()
			})
		}()
	}

	win.SetContent(container.NewBorder(nil, buttons, nil, nil, tabs))
	win.Resize(fyne.NewSize(560, 480))
	fynewidget.CenterOnScreen(win)
	win.Show()
	reload()
}

// renderCoreRuntimeDirections — вкладка Направлений: имя группы, под ним
// стрелка и активный узел строкой списка. Состав группы НЕ разворачивается:
// он и так на вкладке серверов, здесь нужен ответ «куда идёт трафик».
func renderCoreRuntimeDirections(box *fyne.Container, ac *core.AppController, dirs []coreRuntimeDirection, cfgPath string) {
	box.Objects = nil
	if len(dirs) == 0 {
		box.Add(widget.NewLabel(locale.T("No selector groups in the config.")))
		box.Refresh()
		return
	}
	for _, d := range dirs {
		head := widget.NewLabel(d.Group)
		head.TextStyle.Bold = true
		box.Add(head)

		switch {
		case d.Err != nil:
			errLbl := widget.NewLabel(coreRuntimeTreeBranch + locale.Tf("error: %v", d.Err))
			errLbl.Importance = widget.LowImportance
			errLbl.Wrapping = fyne.TextWrapWord
			box.Add(errLbl)
		case d.Active.Name == "":
			none := widget.NewLabel(coreRuntimeTreeBranch + locale.T("(no active outbound)"))
			none.Importance = widget.LowImportance
			box.Add(none)
		default:
			box.Add(coreRuntimeNodeRow(ac, d.Active, cfgPath, coreRuntimeTreeBranch))
		}
	}
	box.Refresh()
}

// renderCoreRuntimeEndpoints — вкладка Endpoints: все endpoint'ы конфига.
//
// Статус по типу: у endpoint'а, состоящего в группе (wireguard как выход),
// есть живой пинг; у остального — только факт присутствия в конфиге. Пинг
// tailscale-узлу без exit_node не рисуется намеренно: /delay через него
// ушёл бы в tailnet и вернул ошибку, а не задержку — число врало бы.
func renderCoreRuntimeEndpoints(box *fyne.Container, ac *core.AppController, eps []coreRuntimeEndpoint, cfgPath string) {
	box.Objects = nil
	if len(eps) == 0 {
		box.Add(widget.NewLabel(locale.T("No endpoints in the config.")))
		box.Refresh()
		return
	}
	for _, ep := range eps {
		info := api.ProxyInfo{Name: ep.Node.Tag, DisplayName: textnorm.NormalizeProxyDisplay(ep.Node.Tag), ClashType: ep.Node.Type}
		if ep.Live != nil {
			info = *ep.Live
		}
		row := coreRuntimeNodeRow(ac, info, cfgPath, "  ")
		// У tailscale в колонке вместо пинга — слово состояния из кеша стрима
		// (SPEC 130): «starting», «running», «needs login». Подробности — в ⓘ.
		if ep.Node.Type == configtypes.SchemeTailscale {
			if st, ok := ac.TailscaleStatus(ep.Node.Tag); ok {
				row = coreRuntimeNodeRowWithStatus(ac, info, cfgPath, "  ", tailscaleStateLabel(st.BackendState))
			}
		}
		box.Add(row)
	}
	box.Refresh()
}

// coreRuntimeTreeBranch — маркер «ветка дерева» перед активным узлом
// Направления, как в дереве файлов. Box-drawing, а не «→»: стрелка в
// шрифте Fyne есть, но с пробелом сразу после неё рисуется плиткой �
// (наблюдение владельца), а ветка под заголовком группы по смыслу и точнее.
const coreRuntimeTreeBranch = "└─ "

// coreRuntimeNodeRow — строка узла: имя с подзаголовком (тип·транспорт)
// слева, пинг и ⓘ справа. Оформление повторяет строку вкладки серверов —
// те же canvas.Text и tightVBoxLayout, — чтобы глаз не переучивался; ⓘ
// открывает то же окно инфо, что и там.
//
// Имя и подзаголовок — ОДИН блок, а не два виджета в VBox: у widget.Label
// свой внутренний отступ, и два Label'а разъезжались в три строки с ⓘ,
// повисшим между ними. prefix — маркер слева (ветка у Направления, отступ у
// Endpoint'а).
func coreRuntimeNodeRow(ac *core.AppController, p api.ProxyInfo, cfgPath, prefix string) fyne.CanvasObject {
	return coreRuntimeNodeRowWithStatus(ac, p, cfgPath, prefix, formatDelay(p.Delay))
}

// coreRuntimeNodeRowWithStatus — та же строка, но с ЯВНЫМ текстом колонки
// статуса вместо пинга. Нужна tailscale-узлу: пинг через него врал бы, а
// слово состояния из стрима — нет.
func coreRuntimeNodeRowWithStatus(ac *core.AppController, p api.ProxyInfo, cfgPath, prefix, status string) fyne.CanvasObject {
	name := canvas.NewText(prefix+p.DisplayOrName(), theme.Color(theme.ColorNameForeground))
	name.TextSize = serversNameTextSize
	name.TextStyle.Bold = true

	// Подзаголовок — из конфига, как на вкладке: Clash API отдаёт только тип.
	// Пустой всё равно занимает место: строки одной высоты читаются как
	// список, разной — как ошибка вёрстки.
	subText := ""
	if node := wizardbusiness.LoadConfigNodes(cfgPath).Lookup(p.Name); node != nil {
		subText = strings.Join(node.SubtitleParts(), "·")
	}
	sub := canvas.NewText(subText, theme.Color(theme.ColorNamePlaceHolder))
	sub.TextSize = serversSubtitleTextSize
	title := container.New(tightVBoxLayout{gap: serversTitleSubtitleGap}, name, sub)

	delay := widget.NewLabel(status)
	delay.Alignment = fyne.TextAlignTrailing

	infoBtn := widget.NewButtonWithIcon("", theme.InfoIcon(), func() {
		showNodeInfoWindow(ac, p, cfgPath)
	})
	infoBtn.Importance = widget.LowImportance

	// Правый кластер центрируется по вертикали относительно двухэтажного
	// заголовка — иначе ⓘ прижимается к верхнему краю строки.
	right := container.NewCenter(container.NewHBox(delay, infoBtn))
	return container.NewBorder(nil, nil, nil, right, title)
}
