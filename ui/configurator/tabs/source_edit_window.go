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

// Min heights for Source Edit dialog tab bodies (child window; do not use main window canvas before Show).
const (
	sourceEditSettingsScrollMinH float32 = 260
	sourceEditJSONScrollMinH     float32 = 380
)

func showWizardTagConflictError(win fyne.Window) {
	dialog.ShowError(errors.New(locale.T("wizard.source.wizard_tag_conflict")), win)
}

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
		return capPreviewNodes(uniquifyPreviewTags(res.Nodes))
	}

	if subscription.IsXrayJSONArrayBody(bodyStr) {
		nodes, err := subscription.ParseNodesFromXrayJSONArray(bodyStr, skip)
		if err != nil {
			return nil
		}
		return capPreviewNodes(uniquifyPreviewTags(nodes))
	}
	tagCounts := make(map[string]int)
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
		node.Tag = subscription.MakeTagUnique(node.Tag, tagCounts, "ConfigWizard")
		out = append(out, node)
		if len(out) >= previewNodeCap {
			break
		}
	}
	return out
}

// uniquifyPreviewTags разводит одинаковые теги суффиксами «-2», «-3».
//
// Провайдер часто отдаёт по два сервера на страну под одним именем
// («🇳🇱-Нидерланды» дважды). В основном списке их разводит LoadNodesFromSource,
// а превью зовёт парсер напрямую — без этого пользователь видит две
// неразличимые строки и не понимает, какую из них выключает.
func uniquifyPreviewTags(nodes []*config.ParsedNode) []*config.ParsedNode {
	tagCounts := make(map[string]int, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
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
//   - subscription: URL/Skip/Outbounds/ExcludeFromGlobal/ExposeGroupTagsToGlobal/
//     Enabled (=!Disabled) + Tag из TagPrefix/Postfix/Mask;
//   - server: URI=Connections[0], Label=TagMask (corestate adapter ставит
//     `tag_mask=label` для server-source — тэг node будет точно label).
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
	if ps.Source != "" {
		// subscription
		src.Type = wizardmodels.SourceTypeSubscription
		src.URL = ps.Source
		src.URI = ""
		src.Skip = ps.Skip
		src.Outbounds = append([]configtypes.OutboundConfig(nil), ps.Outbounds...)
		src.ExcludeFromGlobal = ps.ExcludeFromGlobal
		src.ExposeGroupTagsToGlobal = ps.ExposeGroupTagsToGlobal
		src.DetourTag = ps.DetourTag             // SPEC 077
		src.DetourNodeHash = ps.DetourNodeHash   // SPEC 101
		src.DetourNodeLabel = ps.DetourNodeLabel // SPEC 101
		src.DisabledNodes = ps.DisabledNodes     // SPEC 094 D4
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
		// Если widget'ом задан tag_mask — это и есть label для server.
		if ps.TagMask != "" {
			src.Label = ps.TagMask
		}
		src.Enabled = !ps.Disabled
		src.ExcludeFromGlobal = ps.ExcludeFromGlobal
		src.DetourTag = ps.DetourTag             // SPEC 077
		src.DetourNodeHash = ps.DetourNodeHash   // SPEC 101
		src.DetourNodeLabel = ps.DetourNodeLabel // SPEC 101
		src.ConfigJSON = ps.ConfigJSON           // ручной outbound JSON (вкладка JSON)
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
			if s.Label != "" {
				fullTitleSrc = s.Label
			} else if s.URI != "" {
				fullTitleSrc = s.URI
			}
		}
	}
	title := locale.Tf("wizard.source.edit_title", fullTitleSrc)
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
	proxyRef := func() *config.ProxySource {
		mm := presenter.Model()
		if mm == nil || sourceIndex >= len(mm.Sources) {
			return nil
		}
		return &scratch
	}

	prefixEntry := widget.NewEntry()
	prefixEntry.SetPlaceHolder(locale.T("wizard.source.placeholder_prefix"))

	// SPEC 052 phase 8: URL/URI/Label/Postfix/Mask editors теперь доступны
	// в Settings tab. URL/Postfix/Mask показываются только для subscription;
	// URI/Label — только для server. Все мутации идут через scratch + Source.
	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder("https://example.com/sub")

	uriEntry := widget.NewEntry()
	uriEntry.SetPlaceHolder("vless://uuid@host:443?...#tokyo")

	labelEntry := widget.NewEntry()
	labelEntry.SetPlaceHolder(locale.T("wizard.source.placeholder_label"))

	postfixEntry := widget.NewEntry()
	postfixEntry.SetPlaceHolder(locale.T("wizard.source.placeholder_postfix"))

	maskEntry := widget.NewEntry()
	maskEntry.SetPlaceHolder(locale.T("wizard.source.placeholder_mask"))

	autoCheck := widget.NewCheck(locale.T("wizard.source.local_auto"), nil)
	selectCheck := widget.NewCheck(locale.T("wizard.source.local_select"), nil)
	excludeCheck := widget.NewCheck(locale.T("wizard.source.exclude_global"), nil)
	exposeCheck := widget.NewCheck(locale.T("wizard.source.expose_tags"), nil)
	hintLabel := widget.NewLabel("")
	hintLabel.Wrapping = fyne.TextWrapWord

	// SPEC 077 + SPEC 101: detour (proxy-chain) picker — works for both server
	// and subscription. A group option sets scratch.DetourTag (stamped as-is);
	// a "» node" option sets scratch.DetourNodeHash (resolved to the node's
	// final tag at generation time). The two are mutually exclusive.
	detourNone := locale.T("wizard.source.detour_none")
	detourSelect := widget.NewSelect(nil, nil)
	detourHint := widget.NewLabel(locale.T("wizard.source.detour_hint"))
	detourHint.Wrapping = fyne.TextWrapWord
	detourChoices := map[string]wizardbusiness.DetourChoice{}
	detourOnChanged := func(sel string) {
		p := proxyRef()
		if p == nil {
			return
		}
		choice := detourChoices[sel] // zero value for "" / detourNone → clears both
		p.DetourTag = choice.Tag
		p.DetourNodeHash = choice.NodeHash
		p.DetourNodeLabel = choice.NodeLabel
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

	var afterSync func()

	exposeOnChanged := func(v bool) {
		if exposeCheck.Disabled() {
			return
		}
		pp := proxyRef()
		if pp == nil {
			return
		}
		pp.ExposeGroupTagsToGlobal = v
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
		if afterSync != nil {
			afterSync()
		}
	}
	exposeCheck.OnChanged = exposeOnChanged

	refreshExposeAvailability := func() {
		p := proxyRef()
		if p == nil {
			return
		}
		has := wizardbusiness.ProxyHasLocalAuto(p) || wizardbusiness.ProxyHasLocalSelect(p)
		exposeCheck.OnChanged = nil
		if has {
			exposeCheck.Enable()
			exposeCheck.SetChecked(p.ExposeGroupTagsToGlobal)
		} else {
			exposeCheck.Disable()
			exposeCheck.SetChecked(false)
		}
		exposeCheck.OnChanged = exposeOnChanged
		tip := locale.T("wizard.source.expose_tags_tooltip")
		if has {
			tip = ""
		}
		fynewidget.SetToolTipSafe(exposeCheck, tip)
	}

	refreshExcludeHint := func() {
		p := proxyRef()
		if p == nil {
			return
		}
		if p.ExcludeFromGlobal && (!wizardbusiness.ProxyHasLocalAuto(p) || !wizardbusiness.ProxyHasLocalSelect(p)) {
			hintLabel.SetText(locale.T("wizard.source.exclude_hint"))
			hintLabel.Show()
		} else {
			hintLabel.SetText("")
			hintLabel.Hide()
		}
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
		}
		autoCheck.SetChecked(wizardbusiness.ProxyHasLocalAuto(p))
		selectCheck.SetChecked(wizardbusiness.ProxyHasLocalSelect(p))
		excludeCheck.SetChecked(p.ExcludeFromGlobal)
		refreshExposeAvailability()
		refreshExcludeHint()
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
		mm.Sources[sourceIndex].Label = strings.TrimSpace(s)
		// Для server-type: Label также используется как TagMask в derived
		// view (см. ToProxySourceV4) — синхронизируем scratch.
		if mm.Sources[sourceIndex].Type == wizardmodels.SourceTypeServer {
			scratch.TagMask = strings.TrimSpace(s)
		}
		mm.RefreshDerivedParserConfig()
		presenter.UpdateParserConfig(mm.ParserConfigJSON)
		presenter.MarkAsChanged()
		if guiState.RefreshSourcesList != nil {
			guiState.RefreshSourcesList()
		}
	}

	prefixEntry.OnChanged = func(s string) {
		p := proxyRef()
		if p == nil {
			return
		}
		p.TagPrefix = strings.TrimSpace(s)
		wizardbusiness.RenameWizardLocalOutboundTags(p, sourceIndex)
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

	autoCheck.OnChanged = func(on bool) {
		p := proxyRef()
		if p == nil {
			return
		}
		if on {
			if err := wizardbusiness.EnsureLocalAuto(p, sourceIndex); err != nil {
				autoCheck.SetChecked(false)
				showWizardTagConflictError(win)
				return
			}
		} else {
			wizardbusiness.RemoveWizardSelectOutbounds(p)
			wizardbusiness.RemoveWizardAutoOutbounds(p)
			wizardbusiness.SyncExposeFlagWhenNoLocalGroups(p)
		}
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
		syncFormFromModel()
	}

	selectCheck.OnChanged = func(on bool) {
		p := proxyRef()
		if p == nil {
			return
		}
		if on {
			if err := wizardbusiness.EnsureLocalSelect(p, sourceIndex); err != nil {
				selectCheck.SetChecked(false)
				showWizardTagConflictError(win)
				return
			}
		} else {
			wizardbusiness.RemoveWizardSelectOutbounds(p)
			wizardbusiness.SyncExposeFlagWhenNoLocalGroups(p)
		}
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
		syncFormFromModel()
	}

	excludeCheck.OnChanged = func(v bool) {
		p := proxyRef()
		if p == nil {
			return
		}
		p.ExcludeFromGlobal = v
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
		refreshExcludeHint()
		if afterSync != nil {
			afterSync()
		}
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
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_uri")))
			settingsContent.Add(uriEntry)
			// Ручной config_json переопределяет URI — без пометки правка URI
			// «молча не работает» и путает.
			if len(scratch.ConfigJSON) > 0 {
				manualNote := widget.NewLabel(locale.T("wizard.source.settings_manual_json_note"))
				manualNote.Wrapping = fyne.TextWrapWord
				manualNote.Importance = widget.LowImportance
				settingsContent.Add(manualNote)
			}
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_label_field")))
			settingsContent.Add(labelEntry)
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(excludeCheck)
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_detour")))
			settingsContent.Add(detourSelect)
			settingsContent.Add(detourHint)
		} else {
			// Subscription: URL + Tag prefix/postfix/mask + auto/select/exclude/expose + Detour.
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_url_edit")))
			settingsContent.Add(urlEntry)
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_prefix")))
			settingsContent.Add(prefixEntry)
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_postfix")))
			settingsContent.Add(postfixEntry)
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_mask")))
			settingsContent.Add(maskEntry)
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(autoCheck)
			settingsContent.Add(selectCheck)
			settingsContent.Add(excludeCheck)
			settingsContent.Add(exposeCheck)
			settingsContent.Add(hintLabel)
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_detour")))
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

	previewStatus := widget.NewLabel(locale.T("wizard.source.preview_loading"))
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
		previewStatus.SetText(locale.T("wizard.source.preview_loading"))
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
					previewStatus.SetText(locale.Tf("wizard.source.preview_status_err", 0, fetchErr.Error()))
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
		previewStatus.SetText(locale.T("wizard.source.preview_loading"))
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
							if src.Label != "" {
								node.Tag = src.Label
							}
							nodes = []*config.ParsedNode{node}
						}
					} else if src.URI != "" {
						node, perr := subscription.ParseNode(src.URI, nil)
						if perr != nil {
							err = perr
						} else if node != nil {
							node.Tag = src.Label
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
					previewStatus.SetText(locale.T("wizard.source.preview_no_cache"))
					hint := widget.NewLabel(locale.T("wizard.source.preview_no_cache_hint"))
					hint.Wrapping = fyne.TextWrapWord
					hint.Importance = widget.LowImportance
					fetchBtn := widget.NewButtonWithIcon(
						locale.T("wizard.source.preview_fetch_now"),
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
					previewStatus.SetText(locale.Tf("wizard.source.preview_status_err", 0, err.Error()))
				} else {
					previewStatus.SetText(locale.Tf("wizard.source.preview_servers", len(nodes), 1))
				}
				if err == nil {
					if len(nodes) == 0 {
						lbl := widget.NewLabel(locale.T("wizard.source.view_no_servers"))
						lbl.Importance = widget.LowImportance
						// Spacer below pushes label to top instead of centering blank space.
						previewListHost.Add(container.NewVBox(lbl, layout.NewSpacer()))
					} else {
						nn := nodes
						// SPEC 094 D4: у каждой ноды переключатель «включена».
						// Отметка живёт по хешу идентичности, поэтому переживает
						// переименование ноды провайдером и смену её позиции.
						hashes := make([]string, len(nn))
						for i, n := range nn {
							hashes[i] = config.NodeIdentityHash(n)
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

								hash := hashes[id]
								// Узел без вычислимого хеша выключать нельзя:
								// отметку не к чему привязать, и она поехала бы
								// на соседа при следующем обновлении.
								if hash == "" {
									check.OnChanged = nil
									check.SetChecked(true)
									check.Disable()
									return
								}
								check.Enable()
								check.OnChanged = nil
								_, off := scratch.DisabledNodes[hash]
								check.SetChecked(!off)
								check.OnChanged = func(enabled bool) {
									setNodeEnabled(&scratch, hash, enabled)
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

	jsonApplyBtn := widget.NewButton(locale.T("wizard.source.json_apply"), func() {
		text := strings.TrimSpace(jsonEntry.Text)
		if text == "" {
			dialog.ShowError(errors.New(locale.T("wizard.source.json_err_empty")), win)
			return
		}
		var ob map[string]interface{}
		if err := json.Unmarshal([]byte(text), &ob); err != nil {
			dialog.ShowError(errors.New(locale.Tf("wizard.source.json_err_invalid", err.Error())), win)
			return
		}
		if t, _ := ob["type"].(string); strings.TrimSpace(t) == "" {
			dialog.ShowError(errors.New(locale.T("wizard.source.json_err_no_type")), win)
			return
		}
		// Compact сохраняет порядок полей пользователя (в отличие от
		// Unmarshal→Marshal, который пересортировал бы ключи по алфавиту).
		var compact bytes.Buffer
		if err := json.Compact(&compact, []byte(text)); err != nil {
			dialog.ShowError(errors.New(locale.Tf("wizard.source.json_err_invalid", err.Error())), win)
			return
		}
		scratch.ConfigJSON = json.RawMessage(append([]byte(nil), compact.Bytes()...))
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
		rebuildSettingsLayout() // пометка «URI игнорируется» в Settings
		doRefreshJSONTab()
	})
	jsonResetBtn := widget.NewButton(locale.T("wizard.source.json_reset"), func() {
		if len(scratch.ConfigJSON) == 0 {
			return
		}
		dialog.ShowConfirm(
			locale.T("wizard.source.json_reset"),
			locale.T("wizard.source.json_reset_confirm"),
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
			jsonStatus.SetText(locale.T("wizard.source.json_status_manual"))
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
				jsonStatus.SetText(locale.T("wizard.source.json_status_no_uri"))
			default:
				reason := locale.T("wizard.source.view_no_servers")
				if _, perr := subscription.ParseNode(uri, nil); perr != nil {
					reason = perr.Error()
				}
				jsonStatus.SetText(locale.Tf("wizard.source.json_status_unparsed", reason))
			}
			setJSONText("")
			return
		}
		outJSONs, epJSON, eerr := config.EmitNodeJSONs(node)
		if eerr != nil {
			jsonStatus.SetText(locale.Tf("wizard.source.json_status_unparsed", eerr.Error()))
			setJSONText("")
			return
		}
		raw := epJSON
		if raw == "" && len(outJSONs) > 0 {
			raw = outJSONs[len(outJSONs)-1]
		}
		setJSONText(emittedToEditableJSON(raw))
		jsonStatus.SetText(locale.T("wizard.source.json_status_generated"))
	}

	jsonRefreshSeq := 0
	doRefreshJSONTab = func() {
		if isServerSource {
			refreshServerJSONTab()
			return
		}
		// Subscription: как Preview — декод кэшированного body в горутине.
		jsonRefreshSeq++
		seq := jsonRefreshSeq
		jsonStatus.SetText(locale.T("wizard.source.preview_loading"))
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
					status = locale.T("wizard.source.preview_no_cache")
				} else if decoded, decErr := subscription.DecodeSubscriptionContent(raw); decErr != nil {
					status = locale.Tf("wizard.source.json_status_unparsed", decErr.Error())
				} else {
					nodes := parsePreviewNodesFromBody(decoded, skip)
					// Выключенные ноды не эмитятся — как в реальной сборке.
					if len(disabled) > 0 {
						kept := nodes[:0]
						for _, n := range nodes {
							if h := config.NodeIdentityHash(n); h != "" {
								if _, off := disabled[h]; off {
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
		if isServerSource && jsonEntry.Text != lastSetJSON {
			return // незаApplied ручные правки — не затирать автообновлением
		}
		doRefreshJSONTab()
	}

	jsonHintKey := "wizard.source.json_hint_subscription"
	if isServerSource {
		jsonHintKey = "wizard.source.json_hint_server"
	}
	jsonHint := widget.NewLabel(locale.T(jsonHintKey))
	jsonHint.Wrapping = fyne.TextWrapWord
	jsonGutter := components.NewScrollGutter()
	jsonScrollWithGutter := container.NewBorder(nil, nil, nil, jsonGutter, jsonScroll)
	var jsonCol *fyne.Container
	if isServerSource {
		jsonButtonsRow := container.NewHBox(jsonApplyBtn, jsonResetBtn, layout.NewSpacer())
		jsonCol = container.NewVBox(jsonHint, jsonStatus, jsonScrollWithGutter, jsonButtonsRow)
	} else {
		jsonCol = container.NewVBox(jsonHint, jsonStatus, jsonScrollWithGutter)
	}

	// SPEC 052 phase 8: Overview-tab включает raw body section (раньше был
	// отдельный Raw tab — слили чтобы не дублировать read-only inspection).
	overviewContent, refreshOverviewTab := buildOverviewTab(presenter, sourceIndex)

	settingsTab := container.NewTabItem(locale.T("wizard.source.tab_settings"), settingsWithGutter)
	previewTab := container.NewTabItem(locale.T("wizard.source.tab_preview"), previewBox)
	overviewTab := container.NewTabItem(locale.T("wizard.source.tab_overview"), overviewContent)
	jsonTab := container.NewTabItem(locale.T("wizard.source.tab_json"), jsonCol)
	tabs := container.NewAppTabs(settingsTab, previewTab, overviewTab, jsonTab)
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

	cancelBtn := widget.NewButton(locale.T("wizard.outbound.button_cancel"), func() {
		win.Close()
	})
	saveBtn := widget.NewButton(locale.T("wizard.outbound.button_save"), func() {
		if err := serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win); err != nil {
			return
		}
		win.Close()
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
