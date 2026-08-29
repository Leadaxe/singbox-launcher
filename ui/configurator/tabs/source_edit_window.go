package tabs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fynetooltip "github.com/dweymouth/fyne-tooltip"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	sourceDetourHintText         = "Dial this source's nodes through another outbound to build a proxy chain. Group tags chain through a selector; » entries chain through one concrete server, tracked by that server and its node tag — editing the server's settings or tag prefix keeps the link, renaming its node tag clears it."
	sourcePreviewNoCacheHintText = "Disabled subscriptions are not auto-fetched, so this one has no nodes yet. Click below to fetch it once without enabling it."
	sourceTagVarsHintText        = "Variables (work in both fields): {$tag} original node tag · {$server} address · {$port} port · {$scheme} protocol (also {$protocol}) · {$label} name from the link's #fragment · {$comment} comment · {$num} node number from 1"
)

// Min heights for Source Edit dialog tab bodies (child window; do not use main window canvas before Show).
const (
	sourceEditSettingsScrollMinH float32 = 260
	sourceEditJSONScrollMinH     float32 = 380
)

// previewNodeCap — сколько узлов вкладка JSON выводит текстом.
//
// SPEC 118 W6: вкладка Preview больше не ограничена — она рисует
// widget.List, а тот виртуализирует прокрутку сам, и обрезка списка узлов
// подписки скрывала бы от пользователя половину его собственных галок. А вот
// вкладка JSON выводит ОДИН текст в MultiLineEntry, у которого виртуализации
// нет (fyne-io/fyne#2935): полтысячи outbound'ов там — секунды на
// перерисовку кадра. Обрезка честная — статус-строка называет её вслух.
const previewNodeCap = 200

// previewParseReasonsBlock — блок причин отбраковки для вкладки Preview;
// nil, если причин нет (показывать нечего, и пустая рамка только шумит).
//
// Wrapping обязателен у каждой строки: Label без него отдаёт всю строку как
// min-width и раздувает окно источника на весь экран (fyne-ловушка), а причины
// тут длинные по построению — «empty user id — the server returned a
// placeholder, subscription may be expired».
func previewParseReasonsBlock(reasons []string) fyne.CanvasObject {
	if len(reasons) == 0 {
		return nil
	}
	items := make([]fyne.CanvasObject, 0, len(reasons)+1)
	head := widget.NewLabel(locale.T("Why nodes were rejected:"))
	head.Wrapping = fyne.TextWrapWord
	head.Importance = widget.WarningImportance
	head.TextStyle = fyne.TextStyle{Bold: true}
	items = append(items, head)
	for _, reason := range reasons {
		lbl := widget.NewLabel("• " + reason)
		lbl.Wrapping = fyne.TextWrapWord
		lbl.Importance = widget.MediumImportance
		lbl.TextStyle = fyne.TextStyle{Italic: true}
		items = append(items, lbl)
	}
	return container.NewVBox(items...)
}

// setNodeEnabled включает или выключает узел источника (SPEC 118 Т2).
//
// Отметка живёт ОДНИМ полем — `node.enabled`, по сырому тегу узла
// (идентичность в рамках источника). Прежней карты выключенных с временами и
// TTL больше нет: узел, исчезнувший из подписки, исчезает вместе со своей
// отметкой на первом же достоверном merge.
//
// SPEC 117: правит canonical state.Source (в окне — его рабочую deep-copy;
// до модели отметки доезжают на Save вместе со всей копией).
func setNodeEnabled(src *wizardmodels.Source, rawTag string, enabled bool) {
	if src == nil || rawTag == "" {
		return
	}
	for i := range src.Nodes {
		if src.Nodes[i].Tag == rawTag {
			src.Nodes[i].Enabled = enabled
		}
	}
	// Узловой источник (server/chain/auto): узел один, и он сам источник.
	if len(src.Nodes) == 0 && src.Tag == rawTag {
		src.Enabled = enabled
	}
}

// nodeEnabledInSource — включён ли узел с таким сырым тегом (SPEC 118 Т2:
// отметка живёт полем node.enabled, отдельной карты выключенных нет).
func nodeEnabledInSource(src *wizardmodels.Source, rawTag string) bool {
	if src == nil || rawTag == "" {
		return true
	}
	for i := range src.Nodes {
		if src.Nodes[i].Tag == rawTag {
			return src.Nodes[i].Enabled
		}
	}
	if src.Tag == rawTag {
		return src.Enabled
	}
	return true
}

// sourceOriginURI — исходная строка узла (share-URI либо текст sing-box
// JSON), из которой собрано его тело. В модели v7 её дом — origin.raw.
func sourceOriginURI(p *wizardmodels.Source) string {
	if p == nil || p.Origin == nil {
		return ""
	}
	return p.Origin.Raw
}

// setSourceOriginURI — правка происхождения узла в рабочем буфере.
//
// Тело здесь НЕ пересобирается: материализация идёт на Save
// (materializeScratchServer), одним разбором на всю правку, а не на каждое
// нажатие клавиши.
func setSourceOriginURI(p *wizardmodels.Source, uri string) {
	if p == nil {
		return
	}
	if uri == "" {
		if p.Origin != nil && p.Origin.Kind == wizardmodels.OriginKindURI {
			p.Origin = nil
		}
		return
	}
	p.Origin = &wizardmodels.Origin{Kind: wizardmodels.OriginKindURI, Raw: uri}
}

// cloneSource — явная deep-copy state.Source для рабочего буфера окна
// источника (SPEC 117, риск Р4).
//
// Поверхностная копия разделила бы Nodes/Replace/Meta/Skip с моделью:
// правка в форме утекала бы в модель до Save и переживала Cancel.
// Копируются все ссылочные поля; вложенные значения Options/Filters/Patch
// (map[string]interface{}) копируются на верхнем уровне — глубже форма их
// не мутирует, только заменяет целиком.
//
// go1.20 (риск Р9): без slices./maps. — ручные append/копии карт.
// cloneCanonicalNode — глубокая копия канонического Node v7 (SPEC 118):
// Origin/Body/Detour/Hops/Group — ссылочные, буфер формы обязан владеть
// своими экземплярами. Без slices./maps. (go1.20-гард win7-сборки).
func cloneCanonicalNode(n wizardmodels.Node) wizardmodels.Node {
	c := n
	if n.Origin != nil {
		o := *n.Origin
		c.Origin = &o
	}
	if n.Body != nil {
		c.Body = append(json.RawMessage(nil), n.Body...)
	}
	if n.Detour != nil {
		d := *n.Detour
		c.Detour = &d
	}
	if n.Hops != nil {
		c.Hops = append([]wizardmodels.NodeLink(nil), n.Hops...)
	}
	if n.Group != nil {
		g := *n.Group
		g.Members = append([]wizardmodels.NodeLink(nil), n.Group.Members...)
		// Хвост ревью W1: Strategy глубоко — *TemplateInt
		// (Tolerance/PoolTolerance) не должны разделяться указателями с
		// моделью, даже пока TemplateInt replace-not-mutate.
		g.Strategy = *n.Group.Strategy.Clone()
		c.Group = &g
	}
	return c
}

func cloneSource(src *wizardmodels.Source) wizardmodels.Source {
	if src == nil {
		return wizardmodels.Source{}
	}
	c := *src
	if src.Skip != nil {
		c.Skip = make([]map[string]string, len(src.Skip))
		for i, m := range src.Skip {
			if m == nil {
				continue
			}
			mm := make(map[string]string, len(m))
			for k, v := range m {
				mm[k] = v
			}
			c.Skip[i] = mm
		}
	}
	if src.TagPolicy != nil {
		t := *src.TagPolicy
		c.TagPolicy = &t
	}
	if src.Update != nil {
		u := *src.Update
		if src.Update.AutoRefresh != nil {
			b := *src.Update.AutoRefresh
			u.AutoRefresh = &b
		}
		c.Update = &u
	}
	if src.Meta != nil {
		m := *src.Meta
		if src.Meta.UserInfo != nil {
			ui := *src.Meta.UserInfo
			m.UserInfo = &ui
		}
		if src.Meta.ProviderAnnounce != nil {
			pa := *src.Meta.ProviderAnnounce
			m.ProviderAnnounce = &pa
		}
		c.Meta = &m
	}
	// SPEC 118 (W1): канонические поля v7 — глубокая копия, иначе правки
	// буфера формы утекали бы в модель до Save (тот же контракт, что у
	// остальных полей выше).
	c.Node = cloneCanonicalNode(src.Node)
	if src.Nodes != nil {
		c.Nodes = make([]wizardmodels.Node, len(src.Nodes))
		for i := range src.Nodes {
			c.Nodes[i] = cloneCanonicalNode(src.Nodes[i])
		}
	}
	if src.Replace != nil {
		r := *src.Replace
		// Хвост ревью W1: deep-copy *TemplateInt внутри стратегии, а не
		// копия структуры с разделяемыми указателями.
		r.Strategy = src.Replace.Strategy.Clone()
		c.Replace = &r
	}
	if src.UpdateStatus != nil {
		us := *src.UpdateStatus
		us.Warnings = append([]wizardmodels.FetchWarning(nil), src.UpdateStatus.Warnings...)
		c.UpdateStatus = &us
	}
	// SPEC 118 W3: одноразовые отметки выключения (вердикт O2) — свой слайс.
	if src.PendingDisabled != nil {
		c.PendingDisabled = append([]string(nil), src.PendingDisabled...)
	}
	return c
}

// cloneDirection — копия Направления с собственными ссылочными полями
// верхнего уровня (для cloneSource; форма Направления целиком не правит,
// но общий backing-слайс с моделью недопустим — риск Р4).
func cloneDirection(d *configtypes.Direction) configtypes.Direction {
	if d == nil {
		return configtypes.Direction{}
	}
	c := *d
	c.AddOutbounds = append([]string(nil), d.AddOutbounds...)
	if d.Options != nil {
		c.Options = make(map[string]interface{}, len(d.Options))
		for k, v := range d.Options {
			c.Options[k] = v
		}
	}
	if d.Filters != nil {
		c.Filters = make(map[string]interface{}, len(d.Filters))
		for k, v := range d.Filters {
			c.Filters[k] = v
		}
	}
	if d.PreferredDefault != nil {
		c.PreferredDefault = make(map[string]interface{}, len(d.PreferredDefault))
		for k, v := range d.PreferredDefault {
			c.PreferredDefault[k] = v
		}
	}
	if d.Auto != nil {
		a := *d.Auto
		c.Auto = &a
	}
	if d.Updates != nil {
		c.Updates = make([]configtypes.OutboundUpdate, len(d.Updates))
		for i := range d.Updates {
			u := d.Updates[i]
			if u.Patch != nil {
				p := make(map[string]interface{}, len(u.Patch))
				for k, v := range u.Patch {
					p[k] = v
				}
				u.Patch = p
			}
			c.Updates[i] = u
		}
	}
	return c
}

// mergeEditedSourceIntoModel — data-часть Save окна источника: рабочая
// deep-copy записывается в canonical `m.Sources[sourceIndex]` ЦЕЛИКОМ,
// полевого маппинга (бывший applyProxyEditToSource) больше нет.
//
// SPEC 118 W3 (фикс ревью, блокер 2): runtime-поля fetch'а берутся из ЖИВОЙ
// записи модели, а не из снимка окна — one-shot fetch вкладки Preview и
// фоновый fetch могли обновить их, пока окно открыто, и запись снимка
// целиком молча откатывала бы свежие nodes[]/updateStatus. Поверх живых
// полей применяются ТОЛЬКО оконные правки включённости — по тегам из
// журнала тумблеров (enabledEdits), а не слепым снимком состава:
// так побеждают и свежий fetch, и то, что пользователь реально нажал.
//
// Вынесена из applySourceEditToModel, чтобы контракт Save был проверяем
// тестом без presenter'а/Fyne (сама applySourceEditToModel лишь добавляет
// пересчёт превью и перерисовку списков).
func mergeEditedSourceIntoModel(
	m *wizardmodels.WizardModel,
	sourceIndex int,
	edited *wizardmodels.Source,
	enabledEdits map[string]bool,
) {
	if m == nil || edited == nil || sourceIndex < 0 || sourceIndex >= len(m.Sources) {
		return
	}
	live := &m.Sources[sourceIndex]
	// Runtime-поля источника формой не правятся (кроме журналируемых
	// тумблеров ниже) — переносим их из актуальной записи.
	edited.Meta = live.Meta
	edited.Update = live.Update
	edited.MaxNodes = live.MaxNodes
	edited.Nodes = live.Nodes
	edited.UpdateStatus = live.UpdateStatus
	edited.PendingDisabled = live.PendingDisabled
	// Оконные тумблеры — поверх живых полей: правка из окна доезжает даже
	// до узлов, родившихся фоновым fetch'ем после открытия.
	for tag, enabled := range enabledEdits {
		setNodeEnabled(edited, tag, enabled)
	}
	m.Sources[sourceIndex] = *edited
}

// applySourceEditToModel — путь Save окна источника (SPEC 117, Т4).
// До этого вызова ни одна правка формы модели не касается — Cancel
// закрывает окно без следов.
func applySourceEditToModel(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	m *wizardmodels.WizardModel,
	sourceIndex int,
	edited *wizardmodels.Source,
	enabledEdits map[string]bool,
) {
	if m == nil || edited == nil || sourceIndex < 0 || sourceIndex >= len(m.Sources) {
		return
	}
	mergeEditedSourceIntoModel(m, sourceIndex, edited, enabledEdits)
	m.BumpRevision()
	m.PreviewNeedsParse = true
	wizardbusiness.InvalidateNodePool(m)
	presenter.RefreshOutboundsConfiguratorList()
	presenter.ScheduleRefreshOutboundOptionsDebounced()
	presenter.MarkAsChanged()
	if guiState.RefreshSourcesList != nil {
		guiState.RefreshSourcesList()
	}
}

// showSourceEditWindow opens Settings | Preview | JSON for one proxy source (SPEC 026).
func showSourceEditWindow(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	parent fyne.Window,
	sourceIndex int,
	shortLabel string,
) {
	if presenter == nil {
		return
	}
	// One modal child workflow: finish Outbound Edit or another Source Edit (View slot) first.
	if w := presenter.OpenOutboundEditWindow(); w != nil {
		w.RequestFocus()
		return
	}
	if w := presenter.OpenViewWindow(); w != nil {
		w.RequestFocus()
		return
	}
	presenter.MergeGUIToModel()

	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	m := presenter.Model()
	if m == nil {
		return
	}
	if sourceIndex < 0 || sourceIndex >= len(m.Sources) {
		return
	}

	// Window title — берём полный URL/Label без обрезки; OS title-bar
	// сам ellipsis'ит до доступной ширины (избегаем двойного "...").
	mm := presenter.Model()
	fullTitleSrc := shortLabel
	if mm != nil && sourceIndex < len(mm.Sources) {
		s := mm.Sources[sourceIndex]
		switch s.Kind {
		case wizardmodels.SourceKindSubscription:
			if s.Meta != nil && strings.TrimSpace(s.Meta.ProfileTitle) != "" {
				fullTitleSrc = s.Meta.ProfileTitle
			} else if s.URL != "" {
				fullTitleSrc = s.URL
			}
		case wizardmodels.SourceKindServer:
			// Подпись, а без неё — тег: у server-источника тег и есть то
			// имя, под которым его знают правила.
			originRaw := ""
			if s.Origin != nil {
				originRaw = s.Origin.Raw
			}
			if name := firstNonEmpty(s.Label, s.Tag, originRaw); name != "" {
				fullTitleSrc = name
			}
		}
	}
	title := locale.Tf("Source — %s", fullTitleSrc)
	win := app.NewWindow(title)
	presenter.SetViewWindow(win)
	win.SetOnClosed(func() {
		fynetooltip.DestroyWindowToolTipLayer(win.Canvas())
		presenter.ClearViewWindow()
		presenter.UpdateChildOverlay()
	})

	// SPEC 117 (Т4): рабочий буфер окна — deep-copy canonical state.Source.
	// Widget'ы мутируют копию; Save записывает её в m.Sources[sourceIndex]
	// целиком (applySourceEditToModel); Cancel закрывает окно без следов.
	scratch := cloneSource(&m.Sources[sourceIndex])
	// SPEC 118 W3 (фикс ревью, блокер 2): журнал тумблеров включённости,
	// сделанных В ЭТОМ окне (identity → последнее выбранное состояние).
	// На Save он применяется поверх runtime-полей живой записи модели —
	// снимок scratch их не переносит (см. mergeEditedSourceIntoModel).
	enabledEdits := make(map[string]bool)
	// SPEC 112-A: имя узла на момент открытия формы. Переименование меняет
	// ИДЕНТИЧНОСТЬ узла, а резолв ссылок на сборке строгий — значит на
	// сохранении надо сбросить ссылки на прежнее имя и сказать об этом
	// пользователю (иначе ссылающиеся источники выпали бы fail-closed молча).
	// Сравнивается именно исходное значение, а не то, что было в поле секунду
	// назад: nodeTagEntry.OnChanged срабатывает на каждый символ.
	nodeTagAtOpen := strings.TrimSpace(m.Sources[sourceIndex].NodeTagOrLabel())
	sourceIDAtOpen := m.Sources[sourceIndex].ID
	nodeIdentityOwner := m.Sources[sourceIndex].Kind == wizardmodels.SourceKindServer ||
		m.Sources[sourceIndex].Kind == wizardmodels.SourceKindChain
	// SPEC 118 Т8: форма тегов на момент открытия. Правка тег-политики или
	// тега замены переименовывает то, чем адресован ручной выбор в кэше
	// живого ядра (cache.db) — переписать его лаунчер не может и обязан
	// предупредить (features/directions.md §10).
	tagShapeAtOpen := tagShapeOf(&m.Sources[sourceIndex])
	// srcRef — рабочая копия, если источник ещё существует в модели (список
	// могли перестроить, пока окно открыто); nil — форме больше некого править.
	srcRef := func() *wizardmodels.Source {
		mm := presenter.Model()
		if mm == nil || sourceIndex >= len(mm.Sources) {
			return nil
		}
		return &scratch
	}

	prefixEntry := widget.NewEntry()
	prefixEntry.SetPlaceHolder(locale.T("prefix"))

	// SPEC 052 phase 8: URL/URI/Label/Postfix editors живут в Settings tab.
	// URL/Prefix/Postfix — только у подписки; URI/Label — только у server.
	// Все мутации идут через scratch + Source.
	//
	// SPEC 118 W6: поля маски тегов здесь больше нет — маска упразднена
	// классом (тег узла хранится полем Node.Tag, а политика контейнера это
	// ровно префикс с постфиксом). Отключённое поле держать было незачем:
	// показывать управление, которое ничем не управляет, — хуже, чем не
	// показывать его вовсе.
	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder("https://example.com/sub") // l10n-exempt: sample URL

	uriEntry := widget.NewEntry()
	uriEntry.SetPlaceHolder("vless://uuid@host:443?...#tokyo") // l10n-exempt: sample URI

	labelEntry := widget.NewEntry()
	labelEntry.SetPlaceHolder(locale.T("human-readable label"))

	// Тег узла — отдельным полем от подписи: на него ссылаются фильтры
	// Направлений, позиции цепочек и правила, поэтому переименование в
	// списке его менять не должно (прежде Label работал и подписью, и
	// тегом сразу — «force tag = label»).
	nodeTagEntry := widget.NewEntry()
	nodeTagEntry.SetPlaceHolder(locale.T("node tag (referenced by rules and filters)"))

	postfixEntry := widget.NewEntry()
	postfixEntry.SetPlaceHolder(locale.T("postfix"))

	// SPEC 108: одна галка вместо прежних четырёх (`Local auto`,
	// `Local select`, `Exclude from global`, `Expose tags`), из восьми
	// комбинаций которых осмысленной была ровно одна. Галка отвечает на
	// «сворачивать ли»; чем именно заменить узлы — на вкладке «Группа».
	var afterSync func()

	foldCheck := ttwidget.NewCheck(locale.T("Fold this subscription into a group"), nil)
	foldCheck.SetToolTip(locale.T("Its nodes are replaced by a single entry in the directions list. Pick what to fold into on the Group tab."))

	// Вкладка «Группа»: во что именно сворачивать. Отдельно от галки
	// намеренно (S2) — галка отвечает на «сворачивать ли», а расклад с его
	// настройками автогруппы в один чекбокс не помещается.
	var applyFoldFromForm func()
	// Пересборка набора вкладок: вкладка «Группа» появляется и исчезает
	// вместе с галкой. Объявлена заранее — сами вкладки строятся ниже.
	var syncFoldTabVisible func()
	foldTabBody := newReplaceTab(presenter.Model(), func() { applyFoldFromForm() })

	// SPEC 110: форма цепочки. Существует только у источника-цепочки, где
	// заменяет собой всё остальное: ни URL, ни URI, ни свёртки у неё нет.
	isChainSource := m.Sources[sourceIndex].Kind == wizardmodels.SourceKindChain
	var chainTabBody *chainForm
	if isChainSource {
		selfTag := m.Sources[sourceIndex].NodeTagOrLabel()
		cands := collectChainHopCandidates(presenter.Model(), selfTag)
		reality, detoured, nodeTypes := chainNodeFlags(presenter.Model())
		unsupported := ""
		if supported, reason := config.ChainSupportedByCore(); !supported {
			unsupported = reason
		}
		chainTabBody = newChainForm(nil, cands, reality, detoured, nodeTypes, unsupported, func() {
			if p := srcRef(); p != nil {
				applyChainFormToSource(p, chainTabBody.Collect(), chainTabBody.CollectLinks())
				// Форма правит ТЕГ узла цепочки — он живёт в Node.Tag копии.
				// Подпись (Label) при этом не трогается — на тег ссылаются
				// фильтры и позиции других цепочек. Пустое поле применяется
				// на Save (см. saveBtn): посимвольный OnChanged не должен
				// стирать тег на полпути ввода.
				if tag := chainTabBody.Tag(); tag != "" {
					p.Tag = tag
				}
			}
			// SPEC 117: правка буферизуется в копии — модель не менялась,
			// помечать состояние изменённым будет Save.
		})
		// Content() создаёт виджеты, Load() их наполняет — порядок
		// обязателен: загрузка в несозданную форму молча потеряла бы
		// настройки.
		chainTabBody.built = chainTabBody.Content()
		chainTabBody.nodesKnown = chainNodesKnown(presenter.Model())
		chainTabBody.SetReferencedBy(chainReferencedBy(presenter.Model()))
		chainTabBody.SetTag(scratch.NodeTagOrLabel())
		// Загружаем из РАБОЧЕЙ копии: форма и дальше живёт на буфере, а не
		// на записи модели (Cancel — без следов, риск Р4).
		chainTabBody.Load(chainFormSettings(&scratch), chainFormHops(&scratch))
		// Владелец диалогов формы — это окно, а не главное: пикер позиции
		// иначе всплыл бы за окном правки.
		chainTabBody.parent = win
		// Кэш узлов мог быть ещё не разобран — тогда позиции показаны как
		// «загружается». Досчитываем в фоне: окно не должно ждать разбора
		// всех подписок, но и оставлять пользователя перед списком без
		// видов нельзя.
		if !chainTabBody.nodesKnown {
			go func() {
				mm := presenter.Model()
				if _, err := wizardbusiness.RebuildNodePool(mm); err != nil {
					return
				}
				cands := collectChainHopCandidates(mm, selfTag)
				reality, detoured, types := chainNodeFlags(mm)
				fyne.Do(func() {
					chainTabBody.SetCandidates(cands, reality, detoured, types, chainNodesKnown(mm))
				})
			}()
		}
	}

	applyFoldFromForm = func() {
		p := srcRef()
		if p == nil {
			return
		}
		if !foldCheck.Checked {
			p.Replace = nil
		} else {
			p.Replace = foldTabBody.Collect(defaultReplaceTag(p, sourceIndex))
		}
		foldTabBody.updateTagsHint()
		if syncFoldTabVisible != nil {
			syncFoldTabVisible()
		}
		// SPEC 117: правка буферизуется в копии до Save — модель не трогаем,
		// обновляем только зависимые вкладки самого окна.
		if afterSync != nil {
			afterSync()
		}
	}

	// syncFoldFormFromModel — форма из рабочей копии. Обработчик галки
	// ставится ПОСЛЕ SetChecked: иначе programmatic установка вызвала бы
	// OnChanged, тот — запись в копию и перестроение формы (ловушка SPEC 104).
	syncFoldFormFromModel := func(p *wizardmodels.Source) {
		if p == nil {
			return
		}
		foldCheck.OnChanged = nil
		foldCheck.SetChecked(p.Replace != nil)
		foldCheck.OnChanged = func(bool) { applyFoldFromForm() }
		foldTabBody.Load(p.Replace, defaultReplaceTag(p, sourceIndex))
	}

	// SPEC 077 + SPEC 101 (ключ — SPEC 112): detour (proxy-chain) picker —
	// works for both server and subscription. A group option sets
	// scratch.DetourTag (stamped as-is); a "» node" option sets
	// scratch.DetourNodeTag (looked up by that tag at generation time).
	// The two are mutually exclusive.
	detourNone := locale.T("(none — direct)")
	detourSelect := widget.NewSelect(nil, nil)
	detourHint := widget.NewLabel(locale.T(sourceDetourHintText))
	detourHint.Wrapping = fyne.TextWrapWord
	detourChoices := map[string]wizardbusiness.DetourChoice{}
	detourOnChanged := func(sel string) {
		p := srcRef()
		if p == nil {
			return
		}
		// zero value для "" / detourNone → detour снимается.
		choice := detourChoices[sel]
		// SPEC 118: в состояние едет ОДНА ссылка (NodeLink) — либо корневой
		// финальный тег, либо пара «id папки + сырой тег узла». Финальный
		// конфиговый тег узла папки здесь не хранится: он вычисляется на
		// сборке и протух бы от правки тег-политики папки.
		if choice.Link == nil {
			p.Detour = nil
		} else {
			link := *choice.Link
			p.Detour = &link
		}
		// SPEC 117: выбор буферизуется в копии до Save.
	}
	detourSelect.OnChanged = detourOnChanged
	refreshDetourOptions := func() {
		opts, sel, choices := wizardbusiness.DetourOptionsWithNodes(presenter.Model(), srcRef(), detourNone)
		detourChoices = choices
		detourSelect.OnChanged = nil // avoid feedback while repopulating
		detourSelect.Options = opts
		detourSelect.SetSelected(sel)
		detourSelect.OnChanged = detourOnChanged
		detourSelect.Refresh()
	}

	// tagSpec* — правка prefix/postfix рабочей копии. TagPolicy живёт
	// указателем и создаётся лениво; оба поля пустые → nil (та же
	// нормализация, что раньше делал полевой маппинг на Save).
	//
	// SPEC 118 W5: маски тегов больше нет — тег узла хранится полем
	// (Node.Tag), а политика контейнера это ровно префикс с постфиксом.
	setTagSpecField := func(set func(*wizardmodels.TagPolicy)) {
		p := srcRef()
		if p == nil {
			return
		}
		if p.TagPolicy == nil {
			p.TagPolicy = &wizardmodels.TagPolicy{}
		}
		set(p.TagPolicy)
		if p.TagPolicy.Prefix == "" && p.TagPolicy.Postfix == "" {
			p.TagPolicy = nil
		}
	}
	tagSpecOf := func(p *wizardmodels.Source) wizardmodels.TagPolicy {
		if p == nil || p.TagPolicy == nil {
			return wizardmodels.TagPolicy{}
		}
		return *p.TagPolicy
	}

	syncFormFromModel := func() {
		p := srcRef()
		if p == nil {
			return
		}
		urlEntry.SetText(p.URL)
		ts := tagSpecOf(p)
		prefixEntry.SetText(ts.Prefix)
		postfixEntry.SetText(ts.Postfix)
		// URI / Label / тег узла — для server-type; всё из рабочей копии.
		uriEntry.SetText(sourceOriginURI(p))
		labelEntry.SetText(p.Label)
		nodeTagEntry.SetText(p.NodeTagOrLabel())
		syncFoldFormFromModel(p)
		refreshDetourOptions()
		if afterSync != nil {
			afterSync()
		}
	}

	urlEntry.OnChanged = func(s string) {
		p := srcRef()
		if p == nil {
			return
		}
		p.URL = strings.TrimSpace(s)
	}

	// URI-поле правит ПРОИСХОЖДЕНИЕ узла: тело пересобирается из него на
	// Save (см. saveBtn) — тем же путём, что fetch и миграция. Посимвольная
	// материализация была бы разбором на каждое нажатие клавиши.
	uriEntry.OnChanged = func(s string) {
		p := srcRef()
		if p == nil {
			return
		}
		setSourceOriginURI(p, strings.TrimSpace(s))
	}

	labelEntry.OnChanged = func(s string) {
		p := srcRef()
		if p == nil {
			return
		}
		// Только подпись: тег живёт в NodeTag и правится своим полем.
		// Прежде эта же строка переписывала маску тегов, и переименование
		// источника молча уводило тег из-под ссылающихся на него правил.
		// SPEC 117: как и остальные поля, подпись буферизуется до Save —
		// раньше она правила модель посимвольно и не откатывалась Cancel'ом.
		p.Label = strings.TrimSpace(s)
	}

	nodeTagEntry.OnChanged = func(s string) {
		p := srcRef()
		// Поле именует УЗЕЛ и есть только у источников-владельцев
		// идентичности (server/chain); у подписки NodeTag не используется,
		// и programmatic SetText не должен его затрагивать.
		if p == nil || (p.Kind != wizardmodels.SourceKindServer && p.Kind != wizardmodels.SourceKindChain) {
			return
		}
		// SPEC 113-E: правка тега БУФЕРИЗУЕТСЯ в рабочей копии, как и
		// остальные поля формы, и доезжает до модели только на Save.
		//
		// Идентичность узла = его тег (SPEC 112), поэтому посимвольная запись
		// в модель означала бы смену идентичности БЕЗ сопутствующего сброса
		// ссылок — тот делается только на Save; Cancel обязан не оставлять
		// следов.
		p.Tag = strings.TrimSpace(s)
	}

	prefixEntry.OnChanged = func(s string) {
		p := srcRef()
		if p == nil {
			return
		}
		setTagSpecField(func(t *wizardmodels.TagPolicy) { t.Prefix = strings.TrimSpace(s) })
		// Тег ЗАМЕНЫ теперь явный (replace.tag) и от префикса не зависит:
		// обновляем только подсказку, чтобы она не отставала от формы.
		if p.Replace != nil {
			foldTabBody.updateTagsHint()
		}
	}

	postfixEntry.OnChanged = func(s string) {
		setTagSpecField(func(t *wizardmodels.TagPolicy) { t.Postfix = strings.TrimSpace(s) })
	}

	// SPEC 052 phase 8: Settings tab type-conditional. Subscription и server
	// показывают разные блоки полей.
	settingsContent := container.NewVBox()
	rebuildSettingsLayout := func() {
		settingsContent.Objects = settingsContent.Objects[:0]
		mm := presenter.Model()
		isServer := mm != nil && sourceIndex < len(mm.Sources) && mm.Sources[sourceIndex].Kind == wizardmodels.SourceKindServer

		if isServer {
			// Server: URI + тег узла + Label + Detour.
			settingsContent.Add(widget.NewLabel(locale.T("Server URI")))
			settingsContent.Add(uriEntry)
			// Ручной config_json переопределяет URI — без пометки правка URI
			// «молча не работает» и путает.
			if scratch.Origin != nil && scratch.Origin.Kind == wizardmodels.OriginKindJSON {
				manualNote := widget.NewLabel(locale.T("A manual config_json is set — the URI above is ignored at build time (see the JSON tab)."))
				manualNote.Wrapping = fyne.TextWrapWord
				manualNote.Importance = widget.LowImportance
				settingsContent.Add(manualNote)
			}
			settingsContent.Add(widget.NewLabel(locale.T("Node tag")))
			settingsContent.Add(nodeTagEntry)
			settingsContent.Add(widget.NewLabel(locale.T("Label")))
			settingsContent.Add(labelEntry)
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(widget.NewLabel(locale.T("Detour server (chain)")))
			settingsContent.Add(detourSelect)
			settingsContent.Add(detourHint)
		} else {
			// Subscription: URL + Tag prefix/postfix + свёртка + Detour.
			settingsContent.Add(widget.NewLabel(locale.T("Subscription URL")))
			settingsContent.Add(urlEntry)
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(widget.NewLabel(locale.T("Tag prefix")))
			settingsContent.Add(prefixEntry)
			settingsContent.Add(widget.NewLabel(locale.T("Tag postfix")))
			settingsContent.Add(postfixEntry)
			// Список переменных — прямо под полями, а не за иконкой «?» и
			// не в доках: их семь, они конкретны, и без них поля префикса —
			// пустое приглашение угадывать. Подсказка одна на оба поля:
			// переменные работают в обоих.
			tagVarsHint := widget.NewLabel(locale.T(sourceTagVarsHintText))
			tagVarsHint.Wrapping = fyne.TextWrapWord
			tagVarsHint.Importance = widget.LowImportance
			settingsContent.Add(tagVarsHint)
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(foldCheck)
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(widget.NewLabel(locale.T("Detour server (chain)")))
			settingsContent.Add(detourSelect)
			settingsContent.Add(detourHint)
		}
		settingsContent.Refresh()
	}
	rebuildSettingsLayout()
	settingsScroll := container.NewVScroll(settingsContent)
	settingsScroll.SetMinSize(fyne.NewSize(0, sourceEditSettingsScrollMinH))
	settingsGutter := components.NewScrollGutter()
	settingsWithGutter := container.NewBorder(nil, nil, nil, settingsGutter, settingsScroll)

	previewStatus := widget.NewLabel(locale.T("Loading..."))
	previewStatus.Wrapping = fyne.TextWrapOff
	previewStatusScroll := container.NewHScroll(previewStatus)
	previewListHost := container.NewStack()
	previewGutter := components.NewScrollGutter()
	previewBox := container.NewBorder(previewStatusScroll, nil, nil, previewGutter, previewListHost)

	previewRefreshSeq := 0
	// fetchInProgress: предохранитель от двойного клика "Fetch now" пока
	// goroutine ещё качает. Гард читается только на UI-thread (set перед
	// go-фоновой fetch'ей, clear на UI-thread в callback).
	fetchInProgress := false
	var refreshPreviewTab func()
	// triggerOneShotFetch — клик по "Fetch now" в Preview tab когда нет .raw кэша.
	// Тот же поток что refreshOneSourceFromUI в Sources tab: snapshot источника,
	// RefreshSourceInPlace в горутине, на UI-thread мигрируем обновлённый snapshot
	// (включая обновлённую Meta) обратно в model + вызываем refreshPreviewTab
	// чтобы новый .raw был прочитан и preview отрендерился.
	triggerOneShotFetch := func() {
		if fetchInProgress {
			return
		}
		fetchInProgress = true
		previewStatus.SetText(locale.T("Loading..."))
		previewListHost.Objects = nil
		previewListHost.Add(layout.NewSpacer())
		previewListHost.Refresh()
		m := presenter.Model()
		if m == nil || sourceIndex >= len(m.Sources) {
			fetchInProgress = false
			return
		}
		// Snapshot источника на UI-thread (deep-copy Meta — иначе goroutine
		// мутирует общий объект через pointer).
		snapshot := m.Sources[sourceIndex]
		if snapshot.Meta != nil {
			metaCopy := *snapshot.Meta
			snapshot.Meta = &metaCopy
		}
		// SPEC 118 W3 (фикс ревью): горутина читает Skip и гоняет merge по
		// Nodes/PendingDisabled — глубокие копии, иначе merge
		// мутировал бы backing-массивы, разделяемые с моделью и scratch'ем
		// на UI-thread. Руками, без slices./maps. — go1.20 (win7-сборка).
		if snapshot.Skip != nil {
			sk := make([]map[string]string, len(snapshot.Skip))
			for i, mp := range snapshot.Skip {
				if mp == nil {
					continue
				}
				mm := make(map[string]string, len(mp))
				for k, v := range mp {
					mm[k] = v
				}
				sk[i] = mm
			}
			snapshot.Skip = sk
		}
		if snapshot.Nodes != nil {
			nn := make([]wizardmodels.Node, len(snapshot.Nodes))
			for i := range snapshot.Nodes {
				nn[i] = cloneCanonicalNode(snapshot.Nodes[i])
			}
			snapshot.Nodes = nn
		}
		if snapshot.PendingDisabled != nil {
			snapshot.PendingDisabled = append([]string(nil), snapshot.PendingDisabled...)
		}
		// SPEC 118 W6 (хвост ревью W3): ревизия модели на момент снятия
		// снимка. Окно источника правит МОДЕЛЬ на Save, а Save доступен всё
		// время полёта fetch'а — запись снимка целиком откатила бы его.
		revAtStart := m.Revision
		configService := presenter.ConfigServiceAdapter()
		go func() {
			_, fetchErr := configService.RefreshSourceInPlace(&snapshot)
			fyne.Do(func() {
				fetchInProgress = false
				if fetchErr != nil {
					previewStatus.SetText(locale.Tf("Local outbounds: %d · Servers: load failed — %s", 0, fetchErr.Error()))
					previewListHost.Objects = nil
					previewListHost.Refresh()
					return
				}
				// Snapshot обратно в model: поиск по ID (slice мог
				// реаллокнуться), а при изменившейся ревизии — только поля
				// результата fetch'а поверх живой записи.
				m := presenter.Model()
				if !wizardbusiness.ApplyFetchSnapshot(m, &snapshot, revAtStart) {
					return
				}
				// Состав узлов сменился — без инвалидции кэш превью (и
				// счётчики, и кандидаты позиций цепочек) оставались бы от
				// старого тела до случайной другой мутации.
				wizardbusiness.InvalidateNodePool(m)
				presenter.MarkAsChanged()
				// SPEC 118 W3: fetch-merge наполнил канонические nodes[] —
				// мутация модели, ревизия обязана вырасти.
				m.BumpRevision()
				if refreshPreviewTab != nil {
					refreshPreviewTab()
				}
			})
		}()
	}
	refreshPreviewTab = func() {
		previewRefreshSeq++
		seq := previewRefreshSeq
		previewStatus.SetText(locale.T("Loading..."))
		previewListHost.Objects = nil
		previewListHost.Add(layout.NewSpacer())
		previewListHost.Refresh()
		go func() {
			model := presenter.Model()
			var nodes []*config.ParsedNode
			var err error
			// parseReasons — компактные причины отбраковки от ТОГО ЖЕ разбора,
			// что дал nodes (SPEC 115). Показываются и при нуле узлов, и при
			// частичной отбраковке: «половина подписки протухла» — тоже ответ,
			// которого у пользователя раньше не было. До этого вкладка Preview
			// у протухшей подписки писала «0 server(s)» и «No servers found.» —
			// то есть повторяла факт пустоты и молчала о причине.
			var parseReasons []string
			// needsFetch — true когда нет .raw кэша для subscription: UI должен
			// показать кнопку "Fetch now" вместо просто текста ошибки.
			needsFetch := false

			// SPEC 118 Т8: превью читает МАТЕРИАЛИЗОВАННЫЕ узлы — те же,
			// из которых собирается конфиг. Разбора тут больше нет: тела
			// разобраны один раз (fetch либо миграция) и лежат в состоянии.
			//
			// Подписка без единого узла = «её ещё ни разу не обновляли»
			// (или последний ответ был недостоверен): показываем affordance
			// одноразового fetch'а, а не пустой список.
			if model != nil && sourceIndex < len(model.Sources) {
				src := model.Sources[sourceIndex]
				switch src.Kind {
				case wizardmodels.SourceKindFolder, wizardmodels.SourceKindSubscription:
					if len(src.Nodes) == 0 {
						needsFetch = src.Kind == wizardmodels.SourceKindSubscription
					} else {
						emitted := config.EmitCanonicalSource(src.ToProxySourceV4(), sourceIndex, map[string]int{})
						nodes = emitted.Nodes
						parseReasons = append(parseReasons, emitted.Warnings...)
					}
					// SPEC 118 Т3/Т8: причины отбраковки записей ЖИВУТ В СОСТОЯНИИ —
					// их записал fetch, когда разбирал тело. Разбирать тело
					// заново, чтобы их узнать, вкладке больше нечем (и не нужно):
					// у неё готовые узлы, а «почему их столько» знает только тот,
					// кто их считал.
					parseReasons = append(parseReasons, fetchWarningTexts(src.UpdateStatus)...)
					// Сообщение провайдера — ПЕРВОЙ причиной: он объясняет,
					// почему состав такой, а наши причины — что мы в нём
					// увидели. Чужой текст, показывается как данные.
					if msg := providerAnnounceText(diagOf(&src)); msg != "" {
						parseReasons = append(
							[]string{locale.Tf("provider says: %s", msg)},
							parseReasons...)
					}
				default:
					emitted := config.EmitCanonicalSource(src.ToProxySourceV4(), sourceIndex, map[string]int{})
					nodes = emitted.Nodes
					parseReasons = append(parseReasons, emitted.Warnings...)
					if len(nodes) == 0 && len(emitted.Warnings) == 0 {
						err = fmt.Errorf("%s", locale.T("node has no body — set a URI or JSON"))
					}
				}
			}
			fyne.Do(func() {
				if seq != previewRefreshSeq {
					return
				}
				previewListHost.Objects = nil
				if needsFetch {
					previewStatus.SetText(locale.T("Subscription has not been fetched yet"))
					hint := widget.NewLabel(locale.T(sourcePreviewNoCacheHintText))
					hint.Wrapping = fyne.TextWrapWord
					hint.Importance = widget.LowImportance
					fetchBtn := widget.NewButtonWithIcon(
						locale.T("Fetch now"),
						nil,
						triggerOneShotFetch,
					)
					fetchBtn.Importance = widget.HighImportance
					// Сборка: hint вверху, кнопка под ним, остальное место — spacer.
					previewListHost.Add(container.NewVBox(
						hint,
						container.NewHBox(fetchBtn, layout.NewSpacer()),
						layout.NewSpacer(),
					))
					previewListHost.Refresh()
					return
				}
				if err != nil {
					previewStatus.SetText(locale.Tf("Local outbounds: %d · Servers: load failed — %s", 0, err.Error()))
				} else {
					previewStatus.SetText(locale.Tf("%d server(s) from %d source(s)", len(nodes), 1))
				}
				// SPEC 115: причины отбраковки — под счётчиком, ДО списка.
				// Строка «0 server(s) from 1 source(s)» отвечает на вопрос
				// «сколько», а пользователю нужен ответ «почему»; без него
				// вкладка Preview у протухшей подписки была тупиком.
				reasonsBlock := previewParseReasonsBlock(parseReasons)
				if err == nil {
					if len(nodes) == 0 {
						lbl := widget.NewLabel(locale.T("No servers found."))
						lbl.Importance = widget.LowImportance
						// Spacer below pushes label to top instead of centering blank space.
						items := []fyne.CanvasObject{lbl}
						if reasonsBlock != nil {
							items = append(items, reasonsBlock)
						}
						items = append(items, layout.NewSpacer())
						previewListHost.Add(container.NewVBox(items...))
					} else {
						nn := nodes
						// SPEC 094 D4: у каждой ноды переключатель «включена».
						// Отметка живёт по идентичности узла — сырому тегу
						// источника (SPEC 112), поэтому переживает смену
						// сервера под тем же именем и правку tag_prefix.
						identities := make([]string, len(nn))
						for i, n := range nn {
							identities[i] = config.NodeIdentity(n)
						}
						// widget.List сам виртуализирует scroll — не оборачиваем в
						// NewScroll/NewVScroll (двойной scroll + ограничивающий
						// MinSize 280px не давал списку расти на всю высоту).
						// Truncation=Ellipsis для длинных тегов → "...".
						srvList := widget.NewList(
							func() int { return len(nn) },
							func() fyne.CanvasObject {
								check := widget.NewCheck("", nil)

								// Имя и подзаголовок — как в списке серверов:
								// у провайдера часто по два сервера на страну
								// под одним именем, и различает их только
								// «протокол·транспорт·security».
								name := canvas.NewText("", theme.Color(theme.ColorNameForeground))
								name.TextSize = previewNameTextSize
								// БЕЗ Italic: у эмодзи нет курсивного глифа, Fyne
								// берёт его из emoji-шрифта с другой базовой
								// линией — значки режима групп повисали выше
								// наклонного текста.
								sub := canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder))
								sub.TextSize = previewSubtitleTextSize

								titleBox := container.New(
									previewTightVBox{gap: previewTitleSubtitleGap}, name, sub)
								row := container.NewBorder(nil, nil, check, nil, titleBox)

								// Правый клик по строке — контекстное меню с
								// «Info»: разобранные поля и JSON, который
								// уйдёт в конфиг. Обработчик ставится в
								// updateItem, где известен узел.
								return fynewidget.NewSecondaryTapWrap(row)
							},
							func(id int, o fyne.CanvasObject) {
								wrap, ok := o.(*fynewidget.SecondaryTapWrap)
								if !ok {
									return
								}
								node := nn[id]
								wrap.OnSecondary = func(pe *fyne.PointEvent) {
									showPreviewNodeContextMenu(win, node, pe)
								}

								row, ok := wrap.Content.(*fyne.Container)
								if !ok || len(row.Objects) < 2 {
									return
								}
								titleBox, _ := row.Objects[0].(*fyne.Container)
								check, _ := row.Objects[1].(*widget.Check)
								if titleBox == nil || check == nil || len(titleBox.Objects) < 2 {
									return
								}
								name, _ := titleBox.Objects[0].(*canvas.Text)
								sub, _ := titleBox.Objects[1].(*canvas.Text)
								if name == nil || sub == nil {
									return
								}

								name.Text = nodeDisplayLine(nn[id])
								name.Color = theme.Color(theme.ColorNameForeground)
								name.Refresh()

								sub.Text = previewNodeSubtitle(nn[id])
								sub.Color = theme.Color(theme.ColorNamePlaceHolder)
								sub.Refresh()

								identity := identities[id]
								// Узел без идентичности выключать нельзя:
								// отметку не к чему привязать, и она поехала бы
								// на соседа при следующем обновлении.
								if identity == "" {
									check.OnChanged = nil
									check.SetChecked(true)
									check.Disable()
									return
								}
								check.Enable()
								check.OnChanged = nil
								check.SetChecked(nodeEnabledInSource(&scratch, identity))
								check.OnChanged = func(enabled bool) {
									setNodeEnabled(&scratch, identity, enabled)
									// Журнал правок окна: на Save тумблер
									// применится поверх живой записи модели
									// (блокер 2 ревью W3).
									enabledEdits[identity] = enabled
								}
							},
						)
						// Частичная отбраковка: узлы есть, но часть элементов
						// отвергнута. Причины идут НАД списком — иначе их не
						// видно у подписки на две сотни серверов.
						if reasonsBlock != nil {
							previewListHost.Add(container.NewBorder(reasonsBlock, nil, nil, nil, srvList))
						} else {
							previewListHost.Add(srvList)
						}
					}
				}
				previewListHost.Refresh()
			})
		}()
	}

	// JSON tab — как источник распакуется в sing-box (та же точка эмиссии,
	// что у реальной сборки: config.EmitNodeJSONs). Снапшот записи хранилища,
	// который жил здесь раньше, переехал в Overview (блок Storage record).
	//
	//   - server: редактируемый outbound. Apply сохраняет ручной config_json
	//     (переопределяет URI — для протоколов без парсера/конвертера JSON
	//     собирается руками); Reset возвращает генерацию из URI.
	//   - subscription: read-only распаковка кэшированного body — правки
	//     перезатёр бы первый же сетевой refresh.
	isServerSource := m.Sources[sourceIndex].Kind == wizardmodels.SourceKindServer

	jsonEntry := widget.NewMultiLineEntry()
	jsonEntry.Wrapping = fyne.TextWrapOff
	// Read-only для подписки: OnChanged откатывает любой ввод к последнему
	// установленному тексту (Disable() нельзя — на macOS disabled-текст
	// рендерится цветом фона, см. Overview raw body).
	lastSetJSON := ""
	setJSONText := func(s string) {
		lastSetJSON = s
		jsonEntry.SetText(s)
	}
	if !isServerSource {
		jsonEntry.OnChanged = func(s string) {
			if s != lastSetJSON {
				jsonEntry.SetText(lastSetJSON)
			}
		}
	}
	jsonScroll := container.NewVScroll(container.NewStack(
		canvas.NewRectangle(color.Transparent),
		jsonEntry,
	))
	jsonScroll.SetMinSize(fyne.NewSize(0, sourceEditJSONScrollMinH))

	jsonStatus := widget.NewLabel("")
	jsonStatus.Wrapping = fyne.TextWrapWord

	// doRefreshJSONTab — безусловный рендер (Apply/Reset). refreshJSONTab —
	// обёртка для tabs.OnSelected/afterSync: она не затирает незаApplied
	// ручные правки в server-режиме (текст отличается от последнего
	// установленного → пользователь редактирует).
	var doRefreshJSONTab func()

	jsonApplyBtn := widget.NewButton(locale.T("Apply JSON"), func() {
		text := strings.TrimSpace(jsonEntry.Text)
		if text == "" {
			dialog.ShowError(errors.New(locale.T("JSON is empty.")), win)
			return
		}
		// Общая проверка на объект с непустым `type` — до ветвления: ядро
		// не принимает outbound без типа НИ у сервера, ни у цепочки, и
		// сказать это одной внятной строкой лучше, чем двумя разными из
		// глубины каждой ветки.
		var ob map[string]interface{}
		if err := json.Unmarshal([]byte(text), &ob); err != nil {
			dialog.ShowError(errors.New(locale.Tf("Invalid JSON: %s", err.Error())), win)
			return
		}
		if t, _ := ob["type"].(string); strings.TrimSpace(t) == "" {
			dialog.ShowError(errors.New(locale.T("The outbound object must have a non-empty \"type\" field.")), win)
			return
		}
		// SPEC 110: у цепочки правится СВОЙ объект, а не ConfigJSON —
		// последнего у неё нет, и правка ушла бы в никуда. Обратно
		// разбираем только то, чем цепочка является: позиции и настройки
		// звеньев; чужие ключи молча не принимаем — иначе пользователь
		// решил бы, что вписал рабочую настройку.
		if isChainSource {
			var parsed struct {
				Outbounds    []string               `json:"outbounds"`
				IdleTimeout  string                 `json:"idle_timeout"`
				StripEvasion *bool                  `json:"strip_evasion"`
				Strip        map[string]bool        `json:"strip"`
				Rewrite      map[string]interface{} `json:"rewrite"`
			}
			if err := json.Unmarshal([]byte(text), &parsed); err != nil {
				dialog.ShowError(errors.New(locale.Tf("Invalid JSON: %s", err.Error())), win)
				return
			}
			c := &configtypes.SourceChain{
				Hops:         parsed.Outbounds,
				IdleTimeout:  parsed.IdleTimeout,
				StripEvasion: parsed.StripEvasion,
				Strip:        parsed.Strip,
				Rewrite:      parsed.Rewrite,
			}
			// Отвергаем правку, на которой ядро не стартует: показать
			// ошибку здесь дешевле, чем дать сохранить и обнаружить, что
			// VPN не поднимается.
			if reason := config.ChainEmitError(scratch.NodeTagOrLabel(), c); reason != "" {
				dialog.ShowError(errors.New(reason), win)
				return
			}
			// Позиции из JSON приезжают финальными ТЕГАМИ — форма ядра другой
			// адресации не знает. Разворачиваем их в ссылки по кандидатам:
			// тег узла папки иначе лёг бы корневой ссылкой в никуда.
			jsonHops := chainLinksFromTags(chainTabBody, parsed.Outbounds)
			applyChainFormToSource(&scratch, c, jsonHops)
			if chainTabBody != nil {
				// Форма и JSON — два вида одного объекта: список позиций
				// обязан показать то, что сейчас вписали.
				chainTabBody.Load(c, jsonHops)
			}
			doRefreshJSONTab()
			// SPEC 117: правка буферизуется в копии — модель тронет Save.
			return
		}
		// SPEC 118 Т8: вкладка JSON server-узла — ПРЯМОЙ редактор тела.
		// Разбор, проверка и материализация — в applyServerBodyJSON: одна
		// точка на весь путь «текст → тело», и ошибка в ней означает ОТКАТ
		// (узел остаётся прежним), а не полупринятую правку.
		if err := applyServerBodyJSON(&scratch, text); err != nil {
			dialog.ShowError(errors.New(locale.Tf("Invalid JSON: %s", err.Error())), win)
			return
		}
		doRefreshJSONTab()
	})
	// «Regen from raw» (SPEC 118 Т8): тело пересоздаётся из origin.raw.
	// Неразбираемый raw = ошибка и ОТКАТ — узел не портится.
	jsonResetBtn := widget.NewButton(locale.T("Regen from raw"), func() {
		raw := sourceOriginURI(&scratch)
		if raw == "" {
			return
		}
		dialog.ShowConfirm(
			locale.T("Regen from raw"),
			locale.T("Rebuild the outbound from the original URI/JSON, discarding manual edits?"),
			func(ok bool) {
				if !ok {
					return
				}
				if err := regenServerBodyFromRaw(&scratch); err != nil {
					// Откат: тело остаётся прежним, узел не испорчен.
					dialog.ShowError(errors.New(locale.Tf("URI does not unpack: %s. You can write the outbound JSON by hand and press Apply.", err.Error())), win)
					return
				}
				rebuildSettingsLayout()
				doRefreshJSONTab()
			}, win)
	})
	jsonResetBtn.Disable()

	// refreshServerJSONTab — синхронный рендер для server-source (без сети).
	// refreshChainJSONTab — вкладка JSON у цепочки (SPEC 110).
	//
	// Показывает ОБЪЕКТ, который уедет в конфиг, а не тело подписки: у
	// цепочки нет ни URL, ни кэшированного body, и общая ветка подписок
	// показывала ей «Subscription has not been fetched yet» — текст про
	// сущность, которой здесь нет.
	//
	// Вкладка редактируемая, потому что `rewrite` правится только тут:
	// это произвольный merge-patch по типам протоколов, и урезанная форма
	// молча теряла бы ключи, которых не знает.
	refreshChainJSONTab := func() {
		// Позиции берём с формы: только она умеет развернуть ссылки в
		// финальные теги, которых ждёт эмиттер цепочек.
		var c *configtypes.SourceChain
		if chainTabBody != nil {
			c = chainTabBody.Collect()
		}
		if c == nil {
			c = chainFormSettings(&scratch)
		}
		if c == nil {
			c = &configtypes.SourceChain{}
		}
		tag := scratch.NodeTagOrLabel()
		if tag == "" {
			tag = locale.T("unnamed")
		}
		ob := config.ChainOutboundObject(tag, c)
		raw, err := json.MarshalIndent(ob, "", "  ")
		if err != nil {
			jsonStatus.SetText(locale.Tf("URI does not unpack: %s. You can write the outbound JSON by hand and press Apply.", err.Error()))
			return
		}
		setJSONText(string(raw))
		// Причина, по которой объект не поедет в конфиг, важнее самого
		// объекта: без неё пользователь смотрит на валидный JSON и не
		// понимает, почему маршрут не работает.
		if reason := config.ChainEmitError(tag, c); reason != "" {
			jsonStatus.SetText("⚠️ " + reason)
		} else {
			jsonStatus.SetText(locale.T("Chain is ready to build"))
		}
		jsonResetBtn.Enable()
	}

	refreshServerJSONTab := func() {
		// SPEC 118 Т8: вкладка показывает ТЕЛО узла — ровно то, что уедет в
		// config.json (плюс tag/detour, которые штампует сборка). Разбора
		// здесь нет: тело материализовано и лежит в состоянии.
		if len(scratch.Body) > 0 {
			text := string(scratch.Body)
			var buf bytes.Buffer
			if err := json.Indent(&buf, scratch.Body, "", "  "); err == nil {
				text = buf.String()
			}
			setJSONText(text)
			jsonStatus.SetText(locale.T("The outbound as it will reach the config."))
			if sourceOriginURI(&scratch) != "" {
				jsonResetBtn.Enable()
			} else {
				jsonResetBtn.Disable()
			}
			return
		}
		jsonResetBtn.Disable()
		setJSONText("")
		if raw := sourceOriginURI(&scratch); raw == "" {
			jsonStatus.SetText(locale.T("No URI set. Paste a sing-box outbound object below and press Apply."))
		} else {
			jsonStatus.SetText(locale.Tf("URI does not unpack: %s. You can write the outbound JSON by hand and press Apply.", raw))
			jsonResetBtn.Enable()
		}
	}

	doRefreshJSONTab = func() {
		if isChainSource {
			refreshChainJSONTab()
			return
		}
		if isServerSource {
			refreshServerJSONTab()
			return
		}
		// Подписка: тела уже материализованы — рендерим их синхронно и
		// read-only (SPEC 118 Т8: узлы подписки несвободны).
		model := presenter.Model()
		if model == nil || sourceIndex >= len(model.Sources) {
			setJSONText("")
			jsonStatus.SetText("")
			return
		}
		src := model.Sources[sourceIndex]
		if len(src.Nodes) == 0 {
			setJSONText("")
			jsonStatus.SetText(locale.T("Subscription has not been fetched yet"))
			return
		}
		emitted := config.EmitCanonicalSource(src.ToProxySourceV4(), sourceIndex, map[string]int{})
		text, status := renderUnpackedNodes(emitted.Nodes)
		setJSONText(text)
		jsonStatus.SetText(status)
	}

	refreshJSONTab := func() {
		if (isServerSource || isChainSource) && jsonEntry.Text != lastSetJSON {
			return // незаApplied ручные правки — не затирать автообновлением
		}
		doRefreshJSONTab()
	}

	jsonHintKey := "Read-only: the sing-box outbounds this subscription is built from. Nodes of a subscription are not free-form — they are refreshed from the provider." // l10n-key
	switch {
	case isChainSource:
		jsonHintKey = "The chain object exactly as it will reach the config. This is also where rewrite is edited — per-protocol overrides of node options; everything else is easier to change on the Chain tab." // l10n-key
	case isServerSource:
		jsonHintKey = "The sing-box outbound this node is built from — exactly what the build writes to config.json. Edit and press Apply to store it; Regen from raw rebuilds it from the original URI/JSON. Tag and detour are restamped by the launcher at build time." // l10n-key
	}
	jsonHint := widget.NewLabel(locale.T(jsonHintKey))
	jsonHint.Wrapping = fyne.TextWrapWord
	jsonGutter := components.NewScrollGutter()
	jsonScrollWithGutter := container.NewBorder(nil, nil, nil, jsonGutter, jsonScroll)
	var jsonCol *fyne.Container
	if isServerSource || isChainSource {
		jsonButtonsRow := container.NewHBox(jsonApplyBtn, jsonResetBtn, layout.NewSpacer())
		jsonCol = container.NewVBox(jsonHint, jsonStatus, jsonScrollWithGutter, jsonButtonsRow)
	} else {
		jsonCol = container.NewVBox(jsonHint, jsonStatus, jsonScrollWithGutter)
	}

	// SPEC 052 phase 8: Overview-tab включает raw body section (раньше был
	// отдельный Raw tab — слили чтобы не дублировать read-only inspection).
	overviewContent, refreshOverviewTab := buildOverviewTab(presenter, sourceIndex)

	settingsTab := container.NewTabItem(locale.T("Settings"), settingsWithGutter)
	previewTab := container.NewTabItem(locale.TN(1, "Preview"), previewBox)
	overviewTab := container.NewTabItem(locale.T("Overview"), overviewContent)
	jsonTab := container.NewTabItem(locale.T("JSON"), jsonCol)
	// Вкладка «Группа» — только у подписок и только при включённой свёртке
	// (как «Автовыбор» у Направления, SPEC 104): показывать настройки того,
	// чего нет, — значит предлагать настроить выключённое.
	foldTab := container.NewTabItem(locale.T("Group"), container.NewVScroll(foldTabBody.content))
	var tabs *container.AppTabs
	if isChainSource && chainTabBody != nil {
		// У цепочки вкладка позиций — главная и первая: остальное окно
		// (подписка, URI, свёртка) к ней не относится.
		chainTab := container.NewTabItem(locale.T("Chain"),
			container.NewVScroll(chainTabBody.built))
		tabs = container.NewAppTabs(chainTab, jsonTab)
	} else {
		tabs = container.NewAppTabs(settingsTab, previewTab, overviewTab, jsonTab)
	}
	syncFoldTabVisible = func() {
		p := srcRef()
		show := !isServerSource && !isChainSource && p != nil && p.Replace != nil
		hasTab := false
		for _, ti := range tabs.Items {
			if ti == foldTab {
				hasTab = true
				break
			}
		}
		switch {
		case show && !hasTab:
			// Сразу после Settings: расклад — продолжение галки, а не
			// довесок в конец за JSON.
			tabs.Items = append([]*container.TabItem{settingsTab, foldTab}, tabs.Items[1:]...)
			tabs.Refresh()
		case !show && hasTab:
			tabs.Remove(foldTab)
		}
	}
	syncFoldTabVisible()
	afterSync = func() {
		if tabs.Selected() == overviewTab {
			refreshOverviewTab()
		}
		if tabs.Selected() == previewTab {
			refreshPreviewTab()
		}
		if tabs.Selected() == jsonTab {
			refreshJSONTab()
		}
	}
	tabs.OnSelected = func(ti *container.TabItem) {
		switch ti {
		case overviewTab:
			refreshOverviewTab()
		case previewTab:
			refreshPreviewTab()
		case jsonTab:
			refreshJSONTab()
		}
	}

	cancelBtn := widget.NewButton(locale.T("Cancel"), func() {
		win.Close()
	})
	saveBtn := widget.NewButton(locale.T("Save"), func() {
		// Свёртка собирается ЗДЕСЬ, а не только по событиям отдельных
		// виджетов: у Interval/URL/Tolerance/липкости обработчиков нет, и
		// правка «зайти на Группу, поменять интервал, Save» молча терялась —
		// applyFoldFromForm срабатывал лишь на галку и селект режима.
		if p := srcRef(); p != nil && foldCheck.Checked {
			p.Replace = foldTabBody.Collect(defaultReplaceTag(p, sourceIndex))
		}
		// Тег цепочки финализируется с формы: пустое поле = «тега нет»
		// (откат на подпись через NodeTagOrLabel) — посимвольный обработчик
		// его намеренно не стирал. Пустой состав — всё же цепочка: тип
		// обязан остаться консистентным, а сборка сама объяснит, чего не
		// хватает (прежний контракт applyProxyEditToSource).
		if isChainSource {
			if chainTabBody != nil {
				scratch.Tag = strings.TrimSpace(chainTabBody.Tag())
				applyChainFormToSource(&scratch, chainTabBody.Collect(), chainTabBody.CollectLinks())
			}
		}
		// SPEC 117 (Т4): вся правка одной записью — копия в m.Sources[i];
		// runtime-поля fetch'а берутся из живой записи, оконные тумблеры —
		// журналом enabledEdits (блокер 2 ревью W3).
		applySourceEditToModel(presenter, guiState, presenter.Model(), sourceIndex, &scratch, enabledEdits)
		// SPEC 112-A: узел переименован — его прежней идентичности больше нет,
		// и ссылки на неё обязаны погаснуть здесь, а не молча провалиться на
		// следующей сборке. Порядок важен: сначала запись формы (тег уже
		// новый), потом сброс ссылок на СТАРОЕ имя.
		affected := resetRefsAfterNodeRename(presenter, guiState,
			nodeIdentityOwner, sourceIndex, sourceIDAtOpen, nodeTagAtOpen)
		// SPEC 118 Т8: сменились финальные теги — ручной выбор в селекторах
		// живого ядра адресован прежними именами и собьётся на умолчание.
		stale := staleSelectionAfterEdit(tagShapeAtOpen, tagShapeOf(&scratch))
		win.Close()
		if len(affected) > 0 {
			// Владелец окна — родительское, а не win: то закрывается прямо
			// сейчас, и диалог на нём умер бы вместе с ним, не показавшись.
			showDetourRefsResetDialog(parent, nodeTagAtOpen, affected)
		}
		if !stale.Empty() {
			showStaleSelectionDialog(parent, stale)
		}
	})
	buttonsRow := container.NewHBox(layout.NewSpacer(), cancelBtn, saveBtn)
	root := container.NewBorder(nil, buttonsRow, nil, nil, tabs)

	// fynetooltip layer обязателен для каждого окна — без него
	// ttwidget tooltips не работают в этом окне (fyne-tooltip пишет в логи
	// "no tool tip layer created for current overlay"). Главное окно и
	// Configurator wizard окно делают то же самое.
	win.SetContent(fynetooltip.AddWindowToolTipLayer(root, win.Canvas()))
	win.Resize(fyne.NewSize(880, 600))
	win.CenterOnScreen()
	syncFormFromModel()
	win.Show()
	presenter.UpdateChildOverlay()
}

// applyClearedNodeTag (SPEC 113-E) упразднён вместе со scratch-паттерном
// (SPEC 117): рабочая копия несёт Node.Tag напрямую, и очистка поля — такая
// же буферизованная правка, как любая другая; Save применяет её присваиванием
// копии целиком.

// resetRefsAfterNodeRename гасит detour-ссылки на узел, чьё имя только что
// сменилось, и возвращает имена задетых источников (SPEC 112-A).
//
// Тег узла — единственная идентичность узла (SPEC 112), поэтому его
// переименование = появление ДРУГОГО узла. Резолв ссылок на сборке строгий и
// такую ссылку не разрешит; чинить её подстановкой узла с новым именем нельзя
// (пользователь его хопом не выбирал), поэтому единственный честный исход —
// сбросить ссылку здесь и сказать об этом.
//
// Возвращает nil, когда сбрасывать нечего: имя не менялось, источник не
// именует узел (подписка — там имён много, и переименования узла в форме нет)
// или на прежнее имя никто не ссылался.
func resetRefsAfterNodeRename(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	nodeIdentityOwner bool,
	sourceIndex int,
	sourceIDAtOpen string,
	nodeTagAtOpen string,
) []string {
	if !nodeIdentityOwner || nodeTagAtOpen == "" {
		return nil
	}
	m := presenter.Model()
	if m == nil || sourceIndex < 0 || sourceIndex >= len(m.Sources) {
		return nil
	}
	if strings.TrimSpace(m.Sources[sourceIndex].NodeTagOrLabel()) == nodeTagAtOpen {
		return nil // имя на месте — идентичность не менялась
	}

	affected := wizardbusiness.ResetDetourNodeRefs(m, sourceIDAtOpen, nodeTagAtOpen)
	if len(affected) == 0 {
		return nil
	}
	m.BumpRevision()
	m.PreviewNeedsParse = true
	wizardbusiness.InvalidateNodePool(m)
	presenter.RefreshOutboundsConfiguratorList()
	presenter.ScheduleRefreshOutboundOptionsDebounced()
	presenter.MarkAsChanged()
	if guiState != nil && guiState.RefreshSourcesList != nil {
		guiState.RefreshSourcesList()
	}
	return affected
}

// showDetourRefsResetDialog сообщает, чьи ссылки погасли из-за переименования.
//
// Окно информирующее: сброс уже применён вместе с сохранением формы, отменять
// в нём нечего.
//
// Ловушка Fyne (fyne-label-minwidth-trap): Label без Wrapping задаёт окну
// min-width своей строкой в одну линию — список из десятка имён растянул бы
// диалог на весь экран. Отсюда Wrapping и явный Resize у содержимого.
func showDetourRefsResetDialog(parent fyne.Window, nodeTag string, affected []string) {
	if parent == nil || len(affected) == 0 {
		return
	}
	body := widget.NewLabel(locale.Tf(
		"Node %q was renamed, so its identity changed. Detour links to it have been cleared in: %s",
		nodeTag, strings.Join(affected, ", ")))
	body.Wrapping = fyne.TextWrapWord

	content := container.NewVScroll(body)
	content.SetMinSize(fyne.NewSize(460, 120))
	dialog.ShowCustom(locale.T("Detour links cleared"), locale.T("OK"), content, parent)
}

// showStaleSelectionDialog предупреждает о протухании ручного выбора в
// селекторах живого ядра (SPEC 118 Т8, features/directions.md §10).
//
// Тот же информирующий диалог, что у сброса ссылок при переименовании узла:
// правка уже сохранена, отменять в нём нечего, а выбор в cache.db — не наша
// собственность (переписать его лаунчер не может; у Remote-машины он вообще
// на другой машине).
//
// Ловушка Fyne (fyne-label-minwidth-trap): Label без Wrapping задаёт окну
// min-width своей строкой в одну линию.
func showStaleSelectionDialog(parent fyne.Window, stale staleSelectionScope) {
	if parent == nil || stale.Empty() {
		return
	}
	var lines []string
	if stale.NodesRenamed {
		lines = append(lines, locale.T("Tag prefix/postfix changed — every node of this source is renamed in the config."))
	}
	if len(stale.GroupTags) > 0 {
		lines = append(lines, locale.Tf("Group tags changed: %s.", strings.Join(stale.GroupTags, ", ")))
	}
	lines = append(lines, locale.T("A manual pick in a selector is remembered by the running core (cache.db) by tag, and the launcher cannot rewrite it: the affected selectors fall back to their default until you pick again. On a remote machine this applies to its own core."))

	body := widget.NewLabel(strings.Join(lines, "\n\n"))
	body.Wrapping = fyne.TextWrapWord

	content := container.NewVScroll(body)
	content.SetMinSize(fyne.NewSize(460, 160))
	dialog.ShowCustom(locale.T("Selector choices will reset"), locale.T("OK"), content, parent)
}

// firstNonEmpty — первая непустая строка из перечисленных.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
