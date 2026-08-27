package tabs

import (
	"bytes"
	"encoding/json"
	"errors"
	"image/color"
	"strings"
	"time"

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
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/ui/components"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	sourceDetourHintText         = "Dial this source's nodes through another outbound to build a proxy chain. Group tags chain through a selector; » entries chain through one concrete server, tracked by that server and its node tag — editing the server's settings or tag prefix keeps the link, renaming its node tag clears it."
	sourcePreviewNoCacheHintText = "Disabled subscriptions are not auto-fetched, so there is no cached body to preview. Click below to fetch this subscription once without enabling it."
	sourceTagVarsHintText        = "Variables (work in all three fields): {$tag} original node tag · {$server} address · {$port} port · {$scheme} protocol (also {$protocol}) · {$label} name from the link's #fragment · {$comment} comment · {$num} node number from 1"
)

// Min heights for Source Edit dialog tab bodies (child window; do not use main window canvas before Show).
const (
	sourceEditSettingsScrollMinH float32 = 260
	sourceEditJSONScrollMinH     float32 = 380
)

// previewNodeCap bounds how many nodes the Preview tab parses/renders from a
// source body, keeping the preview responsive for large (CIDR) subscriptions.
const previewNodeCap = 200

// parsePreviewNodesFromBody — парсер decoded body для Preview tab.
//
// Dispatcher по формату body (так же как SPEC 054 для preview_nodes):
//   - Xray JSON array (Liberty VPN и подобные): ParseNodesFromXrayJSONArray
//     (body — `[{"outbounds":...},...]`, без \n-разделения)
//   - base64 / plain text-line (vless://...): построчно через ParseNode
//
// Без диспатча Xray JSON давал 0 нод (split по \n → 1 строка, не URI).
//
// SPEC 094: sing-box JSON (одиночный outbound, массив, целый конфиг, массив
// конфигов) разбирается той же веткой, что и в основном пайплайне — иначе
// превью показывало бы 0 нод для тела, которое импортируется успешно.
func parsePreviewNodesFromBody(body []byte, skip []map[string]string) []*config.ParsedNode {
	bodyStr := strings.TrimSpace(string(body))

	if kind := subscription.ClassifySubscriptionBody(bodyStr); kind.IsSingbox() {
		res, err := subscription.ParseSingboxBody(bodyStr, kind, skip)
		if err != nil || res == nil {
			return nil
		}
		return capPreviewNodes(uniquifyPreviewTags(subscription.DedupParsedNodes(res.Nodes)))
	}

	if subscription.IsXrayJSONArrayBody(bodyStr) {
		nodes, err := subscription.ParseNodesFromXrayJSONArray(bodyStr, skip)
		if err != nil {
			return nil
		}
		return capPreviewNodes(uniquifyPreviewTags(subscription.DedupParsedNodes(nodes)))
	}
	out := make([]*config.ParsedNode, 0)
	contentStr := strings.ReplaceAll(string(body), "\r\n", "\n")
	contentStr = strings.ReplaceAll(contentStr, "\r", "\n")
	for _, line := range strings.Split(contentStr, "\n") {
		line = subscription.NormalizeSubscriptionTextLine(line)
		if line == "" {
			continue
		}
		node, perr := subscription.ParseNode(line, skip)
		if perr != nil || node == nil {
			continue
		}
		out = append(out, node)
	}
	// Дедуп ДО стемпинга и уникализации (SPEC 094 D3, как в боевом
	// LoadNodesFromSource) — иначе дубль получил бы «X-2» и превью показало
	// бы 39 строк там, где сборка даст 8. Кап — ПОСЛЕ дедупа: 32 копии не
	// должны съедать лимит отрисовки.
	return capPreviewNodes(uniquifyPreviewTags(subscription.DedupParsedNodes(out)))
}

// uniquifyPreviewTags разводит одинаковые теги суффиксами «-2», «-3» и
// штампует узлам идентичность (SPEC 112).
//
// Провайдер часто отдаёт по два сервера на страну под одним именем
// («🇳🇱-Нидерланды» дважды). В основном списке их разводит LoadNodesFromSource,
// а превью зовёт парсер напрямую — без этого пользователь видит две
// неразличимые строки и не понимает, какую из них выключает.
//
// Ловушка «Preview ≡ parse»: идентичность здесь обязана совпасть с боевой.
// Совпадает потому, что превью тегов источника не применяет (сырой тег = тег
// узла) и уникализирует тем же алгоритмом, в том же порядке разбора; сам
// стемпинг идёт через ту же subscription.StampNodeIdentity.
func uniquifyPreviewTags(nodes []*config.ParsedNode) []*config.ParsedNode {
	tagCounts := make(map[string]int, len(nodes))
	idCounts := make(map[string]int, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		subscription.StampNodeIdentity(node, idCounts)
		node.Tag = subscription.MakeTagUnique(node.Tag, tagCounts, "ConfigWizard")
	}
	return nodes
}

// capPreviewNodes ограничивает превью, чтобы подписка на тысячи узлов не
// подвешивала окно отрисовкой.
func capPreviewNodes(nodes []*config.ParsedNode) []*config.ParsedNode {
	if len(nodes) > previewNodeCap {
		return nodes[:previewNodeCap]
	}
	return nodes
}

// applyProxyEditToSource — SPEC 052 phase 8: переносит изменения в widget'е
// edit-окна (которое мутирует *config.ProxySource scratch-буфер) обратно
// в canonical `model.Sources[sourceIndex]`.
//
// Маппинг ProxySource → Source:
//   - subscription: URL/Skip/Outbounds/Fold/Enabled (=!Disabled) + Tag из
//     TagPrefix/Postfix/Mask;
//   - server: URI=Connections[0], NodeTag=TagMask (тег узла; corestate
//     adapter отдаёт его обратно в TagMask через NodeTagOrLabel);
//   - chain:  Chain + NodeTag=TagMask.
//
// Label в обе стороны НЕ участвует: это подпись, она живёт только в
// canonical Source и в legacy-форме места не имеет — перенос её оттуда
// затирал бы правку пользователя пустым значением.
//
// setNodeEnabled ставит или снимает отметку «нода выключена» (SPEC 094 D4).
//
// Отметка хранится в ProxySource.DisabledNodes по хешу идентичности узла;
// значение — unix-время последнего подтверждения, по нему потом работает GC.
// Включение ноды удаляет ключ целиком, а не пишет false: пустая карта
// сериализуется как отсутствующее поле (omitempty), и state.json не растёт
// отметками «всё включено».
func setNodeEnabled(ps *config.ProxySource, hash string, enabled bool) {
	if ps == nil || hash == "" {
		return
	}
	if enabled {
		delete(ps.DisabledNodes, hash)
		if len(ps.DisabledNodes) == 0 {
			ps.DisabledNodes = nil
		}
		return
	}
	if ps.DisabledNodes == nil {
		ps.DisabledNodes = make(map[string]int64, 1)
	}
	ps.DisabledNodes[hash] = time.Now().Unix()
}

func applyProxyEditToSource(ps *config.ProxySource, src *wizardmodels.Source) {
	if ps == nil || src == nil {
		return
	}
	if ps.Chain != nil || src.Type == wizardmodels.SourceTypeChain {
		// SPEC 110: цепочка. Проверяется ПЕРВОЙ: у неё нет ни Source, ни
		// Connections, ни ConfigJSON — то есть в ветки ниже она не попадает
		// вовсе, и Save молча терял бы все позиции.
		//
		// Ключ ветки — и ТИП источника, не только ps.Chain: у цепочки с
		// нулём позиций Collect отдаёт nil, и scratch не попадал ни в одну
		// ветку — Save становился no-op, теряя даже переименование
		// (сценарий «создал цепочку, вписал имя, Save» — самый первый).
		src.Type = wizardmodels.SourceTypeChain
		src.Chain = ps.Chain
		if src.Chain == nil {
			// Пустой состав — всё же цепочка: тип обязан остаться
			// консистентным, а сборка сама объяснит, чего не хватает.
			src.Chain = &configtypes.SourceChain{}
		}
		src.URL = ""
		src.URI = ""
		src.Outbounds = nil
		src.Enabled = !ps.Disabled
		src.ExcludeFromGlobal = ps.ExcludeFromGlobal
		// TagMask несёт ТЕГ узла цепочки — он и едет в NodeTag.
		if ps.TagMask != "" {
			src.NodeTag = ps.TagMask
		}
	} else if ps.Source != "" {
		// subscription
		src.Type = wizardmodels.SourceTypeSubscription
		src.URL = ps.Source
		src.URI = ""
		src.Skip = ps.Skip
		src.Outbounds = append([]configtypes.Direction(nil), ps.Outbounds...)
		src.ExcludeFromGlobal = ps.ExcludeFromGlobal
		src.ExposeGroupTagsToGlobal = ps.ExposeGroupTagsToGlobal
		src.Fold = ps.Fold                             // SPEC 108
		src.DetourTag = ps.DetourTag                   // SPEC 077
		src.DetourNodeSourceID = ps.DetourNodeSourceID // SPEC 112-A
		src.DetourNodeTag = ps.DetourNodeTag           // SPEC 112
		src.DetourNodeHash = ps.DetourNodeHash         // legacy, мигрирует на сборке
		src.DetourNodeLabel = ps.DetourNodeLabel       // SPEC 101
		src.DisabledNodes = ps.DisabledNodes           // SPEC 094 D4
		src.Enabled = !ps.Disabled
		if ps.TagPrefix != "" || ps.TagPostfix != "" || ps.TagMask != "" {
			src.Tag = &wizardmodels.TagSpec{
				Prefix:  ps.TagPrefix,
				Postfix: ps.TagPostfix,
				Mask:    ps.TagMask,
			}
		} else {
			src.Tag = nil
		}
	} else if len(ps.Connections) > 0 || len(ps.ConfigJSON) > 0 {
		// server: URI и/или ручной config_json. Ветка обязана срабатывать и
		// без URI — протокол без парсера может существовать только как
		// ручной JSON, и иначе правка вкладки JSON не доехала бы до Source.
		src.Type = wizardmodels.SourceTypeServer
		if len(ps.Connections) > 0 {
			src.URI = ps.Connections[0]
		} else {
			src.URI = ""
		}
		src.URL = ""
		// Если widget'ом задан tag_mask — это тег узла server-источника.
		if ps.TagMask != "" {
			src.NodeTag = ps.TagMask
		}
		src.Enabled = !ps.Disabled
		src.ExcludeFromGlobal = ps.ExcludeFromGlobal
		src.DetourTag = ps.DetourTag                   // SPEC 077
		src.DetourNodeSourceID = ps.DetourNodeSourceID // SPEC 112-A
		src.DetourNodeTag = ps.DetourNodeTag           // SPEC 112
		src.DetourNodeHash = ps.DetourNodeHash         // legacy, мигрирует на сборке
		src.DetourNodeLabel = ps.DetourNodeLabel       // SPEC 101
		src.ConfigJSON = ps.ConfigJSON                 // ручной outbound JSON (вкладка JSON)
		src.Outbounds = nil
		src.Tag = nil
	}
}

func serializeParserAfterSourceEdit(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	m *wizardmodels.WizardModel,
	sourceIndex int,
	scratch *config.ProxySource,
	errParent fyne.Window,
) error {
	if scratch != nil && sourceIndex >= 0 && sourceIndex < len(m.Sources) {
		applyProxyEditToSource(scratch, &m.Sources[sourceIndex])
	}
	m.RefreshDerivedParserConfig()
	m.PreviewNeedsParse = true
	wizardbusiness.InvalidatePreviewCache(m)
	presenter.UpdateParserConfig(m.ParserConfigJSON)
	presenter.ScheduleRefreshOutboundOptionsDebounced()
	presenter.MarkAsChanged()
	if guiState.RefreshSourcesList != nil {
		guiState.RefreshSourcesList()
	}
	return nil
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
		switch s.Type {
		case wizardmodels.SourceTypeSubscription:
			if s.Meta != nil && strings.TrimSpace(s.Meta.ProfileTitle) != "" {
				fullTitleSrc = s.Meta.ProfileTitle
			} else if s.URL != "" {
				fullTitleSrc = s.URL
			}
		case wizardmodels.SourceTypeServer:
			// Подпись, а без неё — тег: у server-источника тег и есть то
			// имя, под которым его знают правила.
			if name := firstNonEmpty(s.Label, s.NodeTag, s.URI); name != "" {
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

	// SPEC 052 phase 8: scratch ProxySource — derived из Sources[i] на open;
	// widget'ы мутируют его in-place; на сохранение → applyProxyEditToSource
	// синхронизирует обратно в canonical Sources[i].
	scratch := m.Sources[sourceIndex].ToProxySourceV4()
	// SPEC 112-A: имя узла на момент открытия формы. Переименование меняет
	// ИДЕНТИЧНОСТЬ узла, а резолв ссылок на сборке строгий — значит на
	// сохранении надо сбросить ссылки на прежнее имя и сказать об этом
	// пользователю (иначе ссылающиеся источники выпали бы fail-closed молча).
	// Сравнивается именно исходное значение, а не то, что было в поле секунду
	// назад: nodeTagEntry.OnChanged срабатывает на каждый символ.
	nodeTagAtOpen := strings.TrimSpace(m.Sources[sourceIndex].NodeTagOrLabel())
	sourceIDAtOpen := m.Sources[sourceIndex].ID
	nodeIdentityOwner := m.Sources[sourceIndex].Type == wizardmodels.SourceTypeServer ||
		m.Sources[sourceIndex].Type == wizardmodels.SourceTypeChain
	proxyRef := func() *config.ProxySource {
		mm := presenter.Model()
		if mm == nil || sourceIndex >= len(mm.Sources) {
			return nil
		}
		return &scratch
	}

	prefixEntry := widget.NewEntry()
	prefixEntry.SetPlaceHolder(locale.T("prefix"))

	// SPEC 052 phase 8: URL/URI/Label/Postfix/Mask editors теперь доступны
	// в Settings tab. URL/Postfix/Mask показываются только для subscription;
	// URI/Label — только для server. Все мутации идут через scratch + Source.
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

	maskEntry := widget.NewEntry()
	maskEntry.SetPlaceHolder(locale.T("{$tag}-{$server}"))

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
	foldTabBody := newFoldTab(presenter.Model(), func() { applyFoldFromForm() })

	// SPEC 110: форма цепочки. Существует только у источника-цепочки, где
	// заменяет собой всё остальное: ни URL, ни URI, ни свёртки у неё нет.
	isChainSource := m.Sources[sourceIndex].Type == wizardmodels.SourceTypeChain
	var chainTabBody *chainForm
	if isChainSource {
		selfTag := m.Sources[sourceIndex].NodeTagOrLabel()
		cands := collectChainHopCandidates(presenter.Model(), getParserConfigForChain(presenter.Model()), selfTag)
		reality, detoured, nodeTypes := chainNodeFlags(presenter.Model())
		unsupported := ""
		if supported, reason := config.ChainSupportedByCore(); !supported {
			unsupported = reason
		}
		chainTabBody = newChainForm(nil, cands, reality, detoured, nodeTypes, unsupported, func() {
			if p := proxyRef(); p != nil {
				p.Chain = chainTabBody.Collect()
				// Форма правит ТЕГ узла цепочки; в ProxySource он живёт в
				// TagMask, откуда sync_to_connections вернёт его в
				// Source.NodeTag. Подпись (Label) при этом не трогается —
				// на тег ссылаются фильтры и позиции других цепочек.
				if tag := chainTabBody.Tag(); tag != "" {
					p.TagMask = tag
				}
			}
			presenter.MarkAsChanged()
		})
		// Content() создаёт виджеты, Load() их наполняет — порядок
		// обязателен: загрузка в несозданную форму молча потеряла бы
		// настройки.
		chainTabBody.built = chainTabBody.Content()
		chainTabBody.nodesKnown = chainNodesKnown(presenter.Model())
		chainTabBody.SetReferencedBy(chainReferencedBy(presenter.Model()))
		chainTabBody.SetTag(m.Sources[sourceIndex].NodeTagOrLabel())
		chainTabBody.Load(m.Sources[sourceIndex].Chain)
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
				if _, err := wizardbusiness.RebuildPreviewCache(mm); err != nil {
					return
				}
				cands := collectChainHopCandidates(mm, getParserConfigForChain(mm), selfTag)
				reality, detoured, types := chainNodeFlags(mm)
				fyne.Do(func() {
					chainTabBody.SetCandidates(cands, reality, detoured, types, chainNodesKnown(mm))
				})
			}()
		}
	}

	applyFoldFromForm = func() {
		p := proxyRef()
		if p == nil {
			return
		}
		if !foldCheck.Checked {
			p.Fold = nil
		} else {
			p.Fold = foldTabBody.Collect()
		}
		// Прежние флаги свёрткой выражены и больше не наши: оставленное
		// значение перебивало бы её на сборке.
		if p.Fold != nil {
			p.ExcludeFromGlobal = false
			p.ExposeGroupTagsToGlobal = false
		}
		foldTabBody.updateTagsHint(foldTabBody.selectedMode(), foldTagPrefix(p), sourceIndex)
		if syncFoldTabVisible != nil {
			syncFoldTabVisible()
		}
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
		if afterSync != nil {
			afterSync()
		}
	}

	// syncFoldFormFromModel — форма из модели. Обработчик галки ставится
	// ПОСЛЕ SetChecked: иначе programmatic установка вызвала бы OnChanged,
	// тот — запись в модель и перестроение формы (ловушка SPEC 104).
	syncFoldFormFromModel := func(p *config.ProxySource) {
		if p == nil {
			return
		}
		foldCheck.OnChanged = nil
		foldCheck.SetChecked(p.Fold != nil)
		foldCheck.OnChanged = func(bool) { applyFoldFromForm() }
		foldTabBody.Load(p.Fold, foldTagPrefix(p), sourceIndex)
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
		p := proxyRef()
		if p == nil {
			return
		}
		choice := detourChoices[sel] // zero value for "" / detourNone → clears both
		p.DetourTag = choice.Tag
		// SPEC 112-A: в состояние едет ссылка-ОБЪЕКТ (источник + identity-тег
		// узла), а не финальный конфиговый тег — тот вычисляется на сборке.
		p.DetourNodeSourceID = choice.NodeSourceID
		p.DetourNodeTag = choice.NodeTag
		p.DetourNodeLabel = choice.NodeLabel
		// Явный выбор гасит упразднённую ссылку: иначе миграция на сборке
		// перебила бы только что выбранный пользователем хоп.
		p.DetourNodeHash = ""
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
	}
	detourSelect.OnChanged = detourOnChanged
	refreshDetourOptions := func() {
		opts, sel, choices := wizardbusiness.DetourOptionsWithNodes(presenter.Model(), proxyRef(), detourNone)
		detourChoices = choices
		detourSelect.OnChanged = nil // avoid feedback while repopulating
		detourSelect.Options = opts
		detourSelect.SetSelected(sel)
		detourSelect.OnChanged = detourOnChanged
		detourSelect.Refresh()
	}

	syncFormFromModel := func() {
		p := proxyRef()
		if p == nil {
			return
		}
		urlEntry.SetText(p.Source)
		prefixEntry.SetText(p.TagPrefix)
		postfixEntry.SetText(p.TagPostfix)
		maskEntry.SetText(p.TagMask)
		// URI / Label — для server-type.
		uriText := ""
		if len(p.Connections) > 0 {
			uriText = p.Connections[0]
		}
		uriEntry.SetText(uriText)
		// Label берём из Source напрямую (scratch не имеет Label-поля).
		mm := presenter.Model()
		if mm != nil && sourceIndex < len(mm.Sources) {
			labelEntry.SetText(mm.Sources[sourceIndex].Label)
			nodeTagEntry.SetText(mm.Sources[sourceIndex].NodeTagOrLabel())
		}
		syncFoldFormFromModel(p)
		refreshDetourOptions()
		if afterSync != nil {
			afterSync()
		}
	}

	urlEntry.OnChanged = func(s string) {
		p := proxyRef()
		if p == nil {
			return
		}
		p.Source = strings.TrimSpace(s)
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
	}

	uriEntry.OnChanged = func(s string) {
		p := proxyRef()
		if p == nil {
			return
		}
		s = strings.TrimSpace(s)
		if s == "" {
			p.Connections = nil
		} else {
			p.Connections = []string{s}
		}
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
	}

	labelEntry.OnChanged = func(s string) {
		mm := presenter.Model()
		if mm == nil || sourceIndex >= len(mm.Sources) {
			return
		}
		// Только подпись: тег живёт в NodeTag и правится своим полем.
		// Прежде эта же строка переписывала TagMask, и переименование
		// источника молча уводило тег из-под ссылающихся на него правил.
		mm.Sources[sourceIndex].Label = strings.TrimSpace(s)
		mm.RefreshDerivedParserConfig()
		presenter.UpdateParserConfig(mm.ParserConfigJSON)
		presenter.MarkAsChanged()
		if guiState.RefreshSourcesList != nil {
			guiState.RefreshSourcesList()
		}
	}

	nodeTagEntry.OnChanged = func(s string) {
		mm := presenter.Model()
		if mm == nil || sourceIndex >= len(mm.Sources) {
			return
		}
		// SPEC 113-E: правка тега БУФЕРИЗУЕТСЯ в scratch, как и остальные поля
		// формы, и доезжает до модели только на Save (applyProxyEditToSource
		// переносит TagMask в NodeTag).
		//
		// Прежде она писалась в модель прямо здесь, посимвольно. Идентичность
		// узла = его тег (SPEC 112), поэтому такая запись означала смену
		// идентичности БЕЗ сопутствующего сброса ссылок — тот делается только
		// на Save. Cancel её не откатывал: окно закрывалось, а тег в модели
		// оставался новым, и ссылающиеся источники выпадали на следующей
		// сборке fail-closed — по правке, от которой пользователь отказался.
		scratch.TagMask = strings.TrimSpace(s)
	}

	prefixEntry.OnChanged = func(s string) {
		p := proxyRef()
		if p == nil {
			return
		}
		p.TagPrefix = strings.TrimSpace(s)
		// SPEC 108: переименовывать группы вручную больше не нужно — они
		// не хранятся, а разворачиваются на сборке из текущего префикса
		// (config.PrepareSourceFolds). Обновляем только подсказку с тегами.
		if p.Fold != nil {
			foldTabBody.updateTagsHint(foldTabBody.selectedMode(), foldTagPrefix(p), sourceIndex)
		}
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
		syncFormFromModel()
	}

	postfixEntry.OnChanged = func(s string) {
		p := proxyRef()
		if p == nil {
			return
		}
		p.TagPostfix = strings.TrimSpace(s)
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
	}

	maskEntry.OnChanged = func(s string) {
		p := proxyRef()
		if p == nil {
			return
		}
		p.TagMask = strings.TrimSpace(s)
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
	}

	// SPEC 052 phase 8: Settings tab type-conditional. Subscription и server
	// показывают разные блоки полей.
	settingsContent := container.NewVBox()
	rebuildSettingsLayout := func() {
		settingsContent.Objects = settingsContent.Objects[:0]
		mm := presenter.Model()
		isServer := mm != nil && sourceIndex < len(mm.Sources) && mm.Sources[sourceIndex].Type == wizardmodels.SourceTypeServer

		if isServer {
			// Server: URI + Label + ExcludeFromGlobal + Detour.
			settingsContent.Add(widget.NewLabel(locale.T("Server URI")))
			settingsContent.Add(uriEntry)
			// Ручной config_json переопределяет URI — без пометки правка URI
			// «молча не работает» и путает.
			if len(scratch.ConfigJSON) > 0 {
				manualNote := widget.NewLabel(locale.T("A manual config_json is set — the URI above is ignored at build time (see the JSON tab)."))
				manualNote.Wrapping = fyne.TextWrapWord
				manualNote.Importance = widget.LowImportance
				settingsContent.Add(manualNote)
			}
			settingsContent.Add(widget.NewLabel(locale.T("Node tag")))
			settingsContent.Add(nodeTagEntry)
			settingsContent.Add(widget.NewLabel(locale.T("Label")))
			settingsContent.Add(labelEntry)
			// SPEC 108: `Exclude from global` из формы убран вместе с
			// остальными тремя галками. Поле в состоянии остаётся и
			// читается — им описаны конфиги, где узлы шли мимо общего
			// списка без всяких групп, — но выставлять его больше нечем:
			// свёртка всегда создаёт группу, а у server-source узел один и
			// сворачивать нечего.
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(widget.NewLabel(locale.T("Detour server (chain)")))
			settingsContent.Add(detourSelect)
			settingsContent.Add(detourHint)
		} else {
			// Subscription: URL + Tag prefix/postfix/mask + свёртка + Detour.
			settingsContent.Add(widget.NewLabel(locale.T("Subscription URL")))
			settingsContent.Add(urlEntry)
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(widget.NewLabel(locale.T("Tag prefix")))
			settingsContent.Add(prefixEntry)
			settingsContent.Add(widget.NewLabel(locale.T("Tag postfix")))
			settingsContent.Add(postfixEntry)
			settingsContent.Add(widget.NewLabel(locale.T("Tag mask (overrides prefix/postfix)")))
			settingsContent.Add(maskEntry)
			// Список переменных — прямо под полями, а не за иконкой «?» и
			// не в доках: их семь, они конкретны, и без них поле маски —
			// пустое приглашение угадывать. Подсказка одна на три поля:
			// переменные работают во всех (replaceTagVariables зовётся и
			// для prefix/postfix, и для mask).
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
		sourceID := snapshot.ID
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
				// Snapshot обратно в model (slice мог реаллокнуться — ищем по ID).
				m := presenter.Model()
				for i := range m.Sources {
					if m.Sources[i].ID == sourceID {
						m.Sources[i] = snapshot
						break
					}
				}
				// Состав узлов сменился — без инвалидции кэш превью (и
				// счётчики, и кандидаты позиций цепочек) оставались бы от
				// старого тела до случайной другой мутации.
				wizardbusiness.InvalidatePreviewCache(m)
				presenter.MarkAsChanged()
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
			// needsFetch — true когда нет .raw кэша для subscription: UI должен
			// показать кнопку "Fetch now" вместо просто текста ошибки.
			needsFetch := false

			// SPEC 052 phase 8 preview pipeline:
			//   - server-source: parse URI напрямую (без сети);
			//   - subscription с .raw на диске: декодим cached body;
			//   - subscription без .raw: показываем "Fetch now" affordance
			//     (disabled-источники в обычном parser pipeline никогда не
			//     дёргают сеть, поэтому .raw для них может не существовать —
			//     одноразовый manual fetch снимает блок без необходимости
			//     включать источник).
			if model != nil && sourceIndex < len(model.Sources) {
				src := model.Sources[sourceIndex]
				switch src.Type {
				case wizardmodels.SourceTypeServer:
					if len(src.ConfigJSON) > 0 {
						// Ручной config_json приоритетнее URI — как в сборке.
						node, perr := subscription.NodeFromManualConfigJSON(src.ConfigJSON)
						if perr != nil {
							err = perr
						} else if node != nil {
							if tag := src.NodeTagOrLabel(); tag != "" {
								node.Tag = tag
							}
							nodes = []*config.ParsedNode{node}
						}
					} else if src.URI != "" {
						node, perr := subscription.ParseNode(src.URI, nil)
						if perr != nil {
							err = perr
						} else if node != nil {
							node.Tag = src.NodeTagOrLabel()
							nodes = []*config.ParsedNode{node}
						}
					}
				case wizardmodels.SourceTypeSubscription:
					subsDir := platform.GetSubscriptionsDir(model.ExecDir)
					if raw, rerr := state.ReadRawBody(subsDir, src.ID); rerr == nil && len(raw) > 0 {
						decoded, decErr := subscription.DecodeSubscriptionContent(raw)
						if decErr != nil {
							err = decErr
						} else {
							pp := proxyRef()
							skip := []map[string]string(nil)
							if pp != nil {
								skip = pp.Skip
							}
							nodes = parsePreviewNodesFromBody(decoded, skip)
						}
					} else {
						// Нет кэша — UI даст affordance для one-shot fetch'а.
						needsFetch = true
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
				if err == nil {
					if len(nodes) == 0 {
						lbl := widget.NewLabel(locale.T("No servers found."))
						lbl.Importance = widget.LowImportance
						// Spacer below pushes label to top instead of centering blank space.
						previewListHost.Add(container.NewVBox(lbl, layout.NewSpacer()))
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
								_, off := scratch.DisabledNodes[identity]
								check.SetChecked(!off)
								check.OnChanged = func(enabled bool) {
									setNodeEnabled(&scratch, identity, enabled)
								}
							},
						)
						previewListHost.Add(srvList)
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
	isServerSource := m.Sources[sourceIndex].Type == wizardmodels.SourceTypeServer

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
			if reason := config.ChainEmitError(scratch.TagMask, c); reason != "" {
				dialog.ShowError(errors.New(reason), win)
				return
			}
			scratch.Chain = c
			if chainTabBody != nil {
				// Форма и JSON — два вида одного объекта: список позиций
				// обязан показать то, что сейчас вписали.
				chainTabBody.Load(c)
			}
			doRefreshJSONTab()
			presenter.MarkAsChanged()
			return
		}
		// Compact сохраняет порядок полей пользователя (в отличие от
		// Unmarshal→Marshal, который пересортировал бы ключи по алфавиту).
		var compact bytes.Buffer
		if err := json.Compact(&compact, []byte(text)); err != nil {
			dialog.ShowError(errors.New(locale.Tf("Invalid JSON: %s", err.Error())), win)
			return
		}
		scratch.ConfigJSON = json.RawMessage(append([]byte(nil), compact.Bytes()...))
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
		rebuildSettingsLayout() // пометка «URI игнорируется» в Settings
		doRefreshJSONTab()
	})
	jsonResetBtn := widget.NewButton(locale.T("Reset to URI"), func() {
		if len(scratch.ConfigJSON) == 0 {
			return
		}
		dialog.ShowConfirm(
			locale.T("Reset to URI"),
			locale.T("Delete the manual config_json and generate the outbound from the URI again?"),
			func(ok bool) {
				if !ok {
					return
				}
				scratch.ConfigJSON = nil
				_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
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
		c := scratch.Chain
		if c == nil {
			c = &configtypes.SourceChain{}
		}
		tag := scratch.TagMask
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
		// Ручной config_json показывается как есть (порядок полей автора);
		// tag/detour перештампуются при сборке.
		if len(scratch.ConfigJSON) > 0 {
			text := string(scratch.ConfigJSON)
			var buf bytes.Buffer
			if err := json.Indent(&buf, scratch.ConfigJSON, "", "  "); err == nil {
				text = buf.String()
			}
			setJSONText(text)
			jsonStatus.SetText(locale.T("Manual config_json — the URI is ignored at build time."))
			jsonResetBtn.Enable()
			return
		}
		jsonResetBtn.Disable()
		// Генерация из URI — ровно тот же путь, что и сборка конфига.
		ps := scratch
		ps.Disabled = false
		var node *config.ParsedNode
		if res, _ := subscription.LoadNodesFromSourceEx(ps, map[string]int{}, nil, 0, 1); res != nil && len(res.Nodes) > 0 {
			node = res.Nodes[0]
		}
		if node == nil {
			// LoadNodesFromSourceEx ошибки только логирует — причину для
			// пользователя добываем прямым ParseNode.
			uri := ""
			if len(scratch.Connections) > 0 {
				uri = subscription.NormalizeSubscriptionTextLine(scratch.Connections[0])
			}
			switch {
			case uri == "":
				jsonStatus.SetText(locale.T("No URI set. Paste a sing-box outbound object below and press Apply."))
			default:
				reason := locale.T("No servers found.")
				if _, perr := subscription.ParseNode(uri, nil); perr != nil {
					reason = perr.Error()
				}
				jsonStatus.SetText(locale.Tf("URI does not unpack: %s. You can write the outbound JSON by hand and press Apply.", reason))
			}
			setJSONText("")
			return
		}
		outJSONs, epJSON, eerr := config.EmitNodeJSONs(node)
		if eerr != nil {
			jsonStatus.SetText(locale.Tf("URI does not unpack: %s. You can write the outbound JSON by hand and press Apply.", eerr.Error()))
			setJSONText("")
			return
		}
		raw := epJSON
		if raw == "" && len(outJSONs) > 0 {
			raw = outJSONs[len(outJSONs)-1]
		}
		setJSONText(emittedToEditableJSON(raw))
		jsonStatus.SetText(locale.T("Generated from URI."))
	}

	jsonRefreshSeq := 0
	doRefreshJSONTab = func() {
		if isChainSource {
			refreshChainJSONTab()
			return
		}
		if isServerSource {
			refreshServerJSONTab()
			return
		}
		// Subscription: как Preview — декод кэшированного body в горутине.
		jsonRefreshSeq++
		seq := jsonRefreshSeq
		jsonStatus.SetText(locale.T("Loading..."))
		// Снапшоты на UI-thread: горутина не должна читать scratch, который
		// мутируют обработчики виджетов.
		detourTag := scratch.DetourTag
		disabled := make(map[string]int64, len(scratch.DisabledNodes))
		for h, ts := range scratch.DisabledNodes {
			disabled[h] = ts
		}
		skip := append([]map[string]string(nil), scratch.Skip...)
		go func() {
			model := presenter.Model()
			text := ""
			status := ""
			if model != nil && sourceIndex < len(model.Sources) {
				src := model.Sources[sourceIndex]
				subsDir := platform.GetSubscriptionsDir(model.ExecDir)
				if raw, rerr := state.ReadRawBody(subsDir, src.ID); rerr != nil || len(raw) == 0 {
					status = locale.T("Subscription has not been fetched yet")
				} else if decoded, decErr := subscription.DecodeSubscriptionContent(raw); decErr != nil {
					status = locale.Tf("URI does not unpack: %s. You can write the outbound JSON by hand and press Apply.", decErr.Error())
				} else {
					nodes := parsePreviewNodesFromBody(decoded, skip)
					// Выключенные ноды не эмитятся — как в реальной сборке.
					if len(disabled) > 0 {
						kept := nodes[:0]
						for _, n := range nodes {
							if id := config.NodeIdentity(n); id != "" {
								if _, off := disabled[id]; off {
									continue
								}
							}
							kept = append(kept, n)
						}
						nodes = kept
					}
					subscription.ApplySourceDetour(nodes, detourTag)
					text, status = renderUnpackedNodes(nodes)
				}
			}
			fyne.Do(func() {
				if seq != jsonRefreshSeq {
					return
				}
				setJSONText(text)
				jsonStatus.SetText(status)
			})
		}()
	}

	refreshJSONTab := func() {
		if (isServerSource || isChainSource) && jsonEntry.Text != lastSetJSON {
			return // незаApplied ручные правки — не затирать автообновлением
		}
		doRefreshJSONTab()
	}

	jsonHintKey := "Read-only: how the cached subscription body unpacks into sing-box outbounds. Tags are shown before the source prefix/mask is applied; the list is rebuilt from the network on every refresh." // l10n-key
	switch {
	case isChainSource:
		jsonHintKey = "The chain object exactly as it will reach the config. This is also where rewrite is edited — per-protocol overrides of node options; everything else is easier to change on the Chain tab." // l10n-key
	case isServerSource:
		jsonHintKey = "The sing-box outbound this source unpacks into — exactly what the build writes to config.json. Edit and press Apply to save a manual config_json (it overrides the URI); Reset returns to URI-generated. Tag and detour are restamped by the launcher at build time." // l10n-key
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
		p := proxyRef()
		show := !isServerSource && !isChainSource && p != nil && p.Fold != nil
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
		if p := proxyRef(); p != nil && foldCheck.Checked {
			p.Fold = foldTabBody.Collect()
			p.ExcludeFromGlobal = false
			p.ExposeGroupTagsToGlobal = false
		}
		if err := serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win); err != nil {
			return
		}
		applyClearedNodeTag(presenter.Model(), nodeIdentityOwner, sourceIndex, scratch.TagMask)
		// SPEC 112-A: узел переименован — его прежней идентичности больше нет,
		// и ссылки на неё обязаны погаснуть здесь, а не молча провалиться на
		// следующей сборке. Порядок важен: сначала запись формы (тег уже
		// новый), потом сброс ссылок на СТАРОЕ имя.
		affected := resetRefsAfterNodeRename(presenter, guiState,
			nodeIdentityOwner, sourceIndex, sourceIDAtOpen, nodeTagAtOpen)
		win.Close()
		if len(affected) > 0 {
			// Владелец окна — родительское, а не win: то закрывается прямо
			// сейчас, и диалог на нём умер бы вместе с ним, не показавшись.
			showDetourRefsResetDialog(parent, nodeTagAtOpen, affected)
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

// applyClearedNodeTag доносит до модели ОЧИЩЕННОЕ поле тега (SPEC 113-E).
//
// Правка тега буферизуется в scratch.TagMask и доезжает через
// applyProxyEditToSource — но тот пустую маску намеренно игнорирует: у
// подписки «маски нет» и «сотри тег» это разные вещи, и различить их там
// нечем. У источников, ИМЕНУЮЩИХ узел (server, chain), пустое поле значит
// именно «тега нет»: дальше NodeTagOrLabel сам откатится на подпись, как и до
// разделения ролей.
func applyClearedNodeTag(m *wizardmodels.WizardModel, nodeIdentityOwner bool, sourceIndex int, tagMask string) {
	if !nodeIdentityOwner || strings.TrimSpace(tagMask) != "" {
		return
	}
	if m == nil || sourceIndex < 0 || sourceIndex >= len(m.Sources) {
		return
	}
	m.Sources[sourceIndex].NodeTag = ""
}

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
	m.RefreshDerivedParserConfig()
	m.PreviewNeedsParse = true
	wizardbusiness.InvalidatePreviewCache(m)
	presenter.UpdateParserConfig(m.ParserConfigJSON)
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

// firstNonEmpty — первая непустая строка из перечисленных.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
