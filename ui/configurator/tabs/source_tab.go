// Package tabs содержит UI компоненты для табов визарда конфигурации.
//
// Файл source_tab.go содержит функции, создающие UI табов визарда:
//   - Вкладка Sources: ввод URL, проверка, список источников; объединённый превью серверов — в отдельном окне
//   - Вкладка Outbounds and ParserConfig: редактор ParserConfig JSON и вход в конфигуратор outbounds
//
// Каждый таб визарда имеет свою отдельную ответственность и логику UI.
//
// Используется в:
//   - configurator.go - при создании окна конфигуратора вызывается CreateSourcesTab(presenter)
//
// Взаимодействует с:
//   - presenter - все действия пользователя (нажатия кнопок, ввод текста) обрабатываются через методы presenter
//   - business - AppendURLsToSources по кнопке Add; список источников из model.Sources (canonical v5)
package tabs

import (
	"fmt"
	"os"
	"strings"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/textnorm"
	"singbox-launcher/ui/components"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizarddialogs "singbox-launcher/ui/configurator/dialogs"
	"singbox-launcher/ui/configurator/outbounds_configurator"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
	wizardutils "singbox-launcher/ui/configurator/utils"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	sourceHintText = "Supports subscription URLs (http/https) or direct links (vless://, vmess://, trojan://, ss://, hysteria2://, ssh://, wireguard://). For multiple links, use a new line for each."
)

// CreateSourcesTab creates the Sources tab UI (URLs, URL status and preview).
func CreateSourcesTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	guiState := presenter.GUIState()
	const directLinksDocURL = "https://github.com/Leadaxe/singbox-launcher/blob/6beb136b9082823699c6509d32e62f212fd7ff90/docs/ParserConfig.md#%D1%84%D0%BE%D1%80%D0%BC%D0%B0%D1%82%D1%8B-uri-%D0%B4%D0%BB%D1%8F-%D0%BF%D1%80%D1%8F%D0%BC%D1%8B%D1%85-%D1%81%D1%81%D1%8B%D0%BB%D0%BE%D0%BA"

	// Section 1: Subscription URL or Direct Links
	urlLabel := widget.NewLabel(locale.T("Subscription URL, Direct Links or sing-box JSON:"))
	urlLabel.Importance = widget.MediumImportance

	guiState.SourceURLEntry = widget.NewMultiLineEntry()
	guiState.SourceURLEntry.SetPlaceHolder(locale.T("https://your-subscription-url-here"))
	guiState.SourceURLEntry.Wrapping = fyne.TextWrapOff
	// No automatic application: URLs are applied only when the user clicks Add.
	guiState.SourceURLEntry.OnChanged = func(value string) {
		if guiState.SourceURLsProgrammatic {
			return
		}
		presenter.Model().PreviewNeedsParse = true
		presenter.MarkAsChanged()
	}

	hintLabel := widget.NewLabel(locale.T(sourceHintText))
	hintLabel.Wrapping = fyne.TextWrapWord
	wireguardHelpButton := widget.NewButton("?", func() {
		if err := platform.OpenURL(directLinksDocURL); err != nil {
			dialog.ShowError(fmt.Errorf("failed to open docs: %w", err), guiState.Window)
		}
	})
	wireguardHelpButton.Importance = widget.LowImportance
	// Keep help button compact (single-symbol width) and pinned to the right.
	helpButtonCompact := container.NewGridWrap(fyne.NewSize(24, 24), wireguardHelpButton)
	hintRow := container.NewBorder(nil, nil, nil, helpButtonCompact, hintLabel)
	// applyAddedSources runs the shared Add path: parse `text` (URI links /
	// vpn:// / [Interface]/[Peer] conf) into sources, refresh UI, clear the
	// field. Used by both the Add button and Add-from-file (SPEC 079).
	applyAddedSources := func(text string) {
		presenter.MergeGUIToModel()
		if err := wizardbusiness.AppendURLsToSources(presenter, strings.TrimSpace(text)); err != nil {
			debuglog.ErrorLog("source_tab: Add error: %v", err)
			return
		}
		m := presenter.Model()
		m.PreviewNeedsParse = true
		presenter.RefreshOutboundsConfiguratorList()
		if guiState.RefreshSourcesList != nil {
			guiState.RefreshSourcesList()
		}
		presenter.MarkAsChanged()
		// Clear the URL field after adding so the user can enter the next URL
		guiState.SourceURLsProgrammatic = true
		guiState.SourceURLEntry.SetText("")
		guiState.SourceURLsProgrammatic = false
	}

	addURLButton := widget.NewButton(locale.T("Add"), func() {
		applyAddedSources(guiState.SourceURLEntry.Text)
	})

	// fyneFileOpen — in-app file dialog, used as the fallback when no native
	// dialog is available (Linux without zenity/kdialog).
	fyneFileOpen := func() {
		fileDialog := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, guiState.Window)
				return
			}
			if rc == nil {
				return // cancelled
			}
			defer func() { _ = rc.Close() }()
			text, rerr := wizardbusiness.ReadSourceFileText(rc)
			if rerr != nil {
				dialog.ShowError(rerr, guiState.Window)
				return
			}
			if text != "" {
				applyAddedSources(text)
			}
		}, guiState.Window)
		fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".conf", ".vpn", ".txt"}))
		fileDialog.Show()
	}

	// SPEC 079/082: WG/AWG configs are often shared as files (.conf with
	// [Interface]/[Peer], or .vpn with a vpn:// link). Open a native system file
	// dialog (SPEC 082); fall back to the in-app one where there's no native
	// dialog. The picked file's text goes through the same path as the Add field.
	addFromFileAction := func() {
		path, ok, err := platform.PickOpenFile(locale.T("Select a config file (.conf / .vpn / .txt)"), []string{"conf", "vpn", "txt"})
		if err == platform.ErrNativeDialogUnavailable {
			fyneFileOpen()
			return
		}
		if err != nil {
			dialog.ShowError(err, guiState.Window)
			return
		}
		if !ok {
			return // cancelled
		}
		f, oerr := os.Open(path)
		if oerr != nil {
			dialog.ShowError(oerr, guiState.Window)
			return
		}
		defer func() { _ = f.Close() }()
		text, rerr := wizardbusiness.ReadSourceFileText(f)
		if rerr != nil {
			dialog.ShowError(rerr, guiState.Window)
			return
		}
		if text != "" {
			applyAddedSources(text)
		}
	}

	// «Free community servers» — picker (LxBox-style): клик подставляет URL
	// из bin/get_free.json в поле SourceURLEntry, ничего не сохраняет в
	// state.json и не мутирует модель. Юзер сам нажимает Add.
	getFreeVPNAction := func() {
		wizarddialogs.ShowGetFreeVPNDialog(presenter)
	}

	// SPEC 084.1: «Add WARP» — генератор Cloudflare WARP. Регистрирует аккаунт и
	// отдаёт готовый wireguard://-URI в тот же Add-путь, что и ручная вставка.
	addWarpAction := func() {
		wizarddialogs.ShowAddWarpDialog(presenter, applyAddedSources)
	}

	// «Add server» — ручная форма: SOCKS5/HTTP по полям либо Source (любой
	// текст, который понимает Add). У этих схем полей мало, а у HTTP-прокси
	// ещё и нестандартный префикс (proxy-http://), который человеку негде
	// подсмотреть. Форма собирает вход и отдаёт в тот же путь Add; вручную
	// отредактированный JSON идёт своей веткой, чтобы сохраниться побайтово.
	addServerAction := func() {
		wizarddialogs.ShowAddServerDialog(presenter, func(res wizarddialogs.AddServerResult) {
			presenter.MergeGUIToModel()
			before := len(presenter.Model().Sources)

			if len(res.ConfigJSON) > 0 {
				if err := wizardbusiness.AppendManualConfigJSON(presenter, res.ConfigJSON, res.Label); err != nil {
					dialog.ShowError(err, guiState.Window)
					return
				}
			} else {
				if err := wizardbusiness.AppendURLsToSources(presenter, strings.TrimSpace(res.Text)); err != nil {
					dialog.ShowError(err, guiState.Window)
					return
				}
				wizardbusiness.RelabelLastSources(presenter, before, res.Label)
			}

			m := presenter.Model()
			m.PreviewNeedsParse = true
			presenter.RefreshOutboundsConfiguratorList()
			if guiState.RefreshSourcesList != nil {
				guiState.RefreshSourcesList()
			}
			presenter.MarkAsChanged()
		})
	}

	// SPEC 110: цепочка хопов — источник, а не Направление: она описывает
	// МАРШРУТ, а точка выбора между маршрутами это Направление. Создаётся
	// пустой и настраивается в своём окне: позиции ссылаются на узлы и
	// Направления, которых при создании может ещё не быть.
	addChainAction := func() {
		presenter.MergeGUIToModel()
		m := presenter.Model()
		m.Sources = append(m.Sources, corestate.Source{
			Node: corestate.Node{Kind: corestate.SourceKindChain, Enabled: true},
			ID:   corestate.MakeULID(),
			// Выданное имя — ТЕГ узла: на него сошлются фильтры и позиции.
			// Подпись остаётся пустой, и список показывает тег, пока
			// пользователь не задаст своё отображаемое имя.
			NodeTag: wizardbusiness.NextChainLabel(m.Sources),
			Chain:   &configtypes.SourceChain{},
		})
		m.BumpRevision()
		m.PreviewNeedsParse = true
		wizardbusiness.InvalidatePreviewCache(m)
		presenter.RefreshOutboundsConfiguratorList()
		if guiState.RefreshSourcesList != nil {
			guiState.RefreshSourcesList()
		}
		presenter.MarkAsChanged()
		// Сразу открываем окно правки: пустая цепочка бесполезна — без
		// позиций она даже в конфиг не пойдёт. У подписки иначе (URL уже
		// введён, настраивать нечего), а здесь создание и настройка — одно
		// действие, и оставить пользователя перед пустой строкой значит
		// заставить искать, где её заполняют.
		idx := len(m.Sources) - 1
		showSourceEditWindow(presenter, guiState, guiState.Window, idx, m.Sources[idx].Label)
	}

	// Limit width and height of URL input field (3 lines)
	// Wrap MultiLineEntry in Scroll container to show scrollbars; right gutter for scrollbar strip
	urlURIGutter := components.NewScrollGutter()
	urlEntryScrollInner := container.NewBorder(nil, nil, nil, urlURIGutter, guiState.SourceURLEntry)
	urlEntryScroll := container.NewScroll(urlEntryScrollInner)
	urlEntryScroll.Direction = container.ScrollBoth
	// Create dummy Rectangle to set size (height 3 lines, width limited)
	urlEntrySizeRect := canvas.NewRectangle(color.Transparent)
	urlEntrySizeRect.SetMinSize(fyne.NewSize(0, 60)) // Width 900px, height ~3 lines (approx 20px per line)
	// Wrap in Max container with Rectangle to fix size
	// Scroll container will be limited by this size and show scrollbars when content doesn't fit
	urlEntryWithSize := container.NewStack(
		urlEntrySizeRect,
		urlEntryScroll,
	)

	// Header row: label on the left; the three add-source actions (Add WARP,
	// Add from file, Free community servers) hidden behind a compact ⋮ overflow
	// menu (same pattern as ui/traffic/toolbar.go) so the header stays clean.
	var overflowBtn *widget.Button
	overflowBtn = widget.NewButtonWithIcon("", theme.MoreVerticalIcon(), func() {
		menu := fyne.NewMenu("",
			fyne.NewMenuItem(locale.T("Add server"), addServerAction),
			fyne.NewMenuItem(locale.T("Add hop chain"), addChainAction),
			fyne.NewMenuItem(locale.T("Add WARP"), addWarpAction),
			fyne.NewMenuItem(locale.T("Add from file"), addFromFileAction),
			fyne.NewMenuItem(locale.T("Free community servers"), getFreeVPNAction),
		)
		pop := widget.NewPopUpMenu(menu, guiState.Window.Canvas())
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(overflowBtn)
		pop.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+overflowBtn.MinSize().Height))
	})
	// Header: только label (⋮ переехал в строку ввода, вплотную к Add).
	urlHeader := container.NewHBox(urlLabel)

	// URL field: [entry .......] [Add] [⋮]. Add и overflow-меню стоят рядом справа
	// от поля — ⋮ сразу за кнопкой Add, а не в дальнем углу заголовка.
	addCluster := container.NewHBox(
		container.NewCenter(addURLButton),
		container.NewCenter(overflowBtn),
	)
	urlEntryRow := container.NewBorder(
		nil, nil,
		nil,
		addCluster,
		urlEntryWithSize,
	)

	urlContainer := container.NewVBox(
		urlHeader,   // Header with Get free VPN
		urlEntryRow, // Input field + Add button on the right
		hintRow,     // Hint + docs button
	)

	// Section 2: Sources list (based on ParserConfig.ParserConfig.Proxies)
	sourcesLabel := widget.NewLabel(locale.T("Sources"))
	sourcesLabel.Importance = widget.MediumImportance

	sourcesBox := container.NewVBox()

	// SPEC 115 §3: какой источник подсвечен переходом из отчёта «Итога» и
	// какая строка его несёт. Живут в замыкании вкладки, а не в модели:
	// подсветка — состояние ЭТОГО экрана, оно не переживает пересоздание
	// вкладки и не имеет смысла ни для сборки, ни для сохранения.
	revealedSourceID := ""
	var revealedRow fyne.CanvasObject

	// SPEC 109: перетаскивание вместо ↑/↓ — тот же механизм, что на Rules,
	// DNS и Направлениях. Порядок источников — обычный порядок слайса
	// model.Sources; на конфиг он не влияет (узлы собираются из всех
	// включённых подписок), но определяет вид списка и нумерацию
	// префиксов по умолчанию.
	dragGroup := fynewidget.NewDragReorderGroup(func(from, to int) {
		m := presenter.Model()
		if m == nil || from < 0 || from >= len(m.Sources) || to < 0 || to >= len(m.Sources) || from == to {
			return
		}
		// Перестановка через копию: срез с обрезанным cap (m.Sources[:from:from])
		// не даёт append переписать хвост исходного слайса, на который могли
		// остаться ссылки в уже построенных строках.
		moved := m.Sources[from]
		rest := append(m.Sources[:from:from], m.Sources[from+1:]...)
		out := make([]corestate.Source, 0, len(m.Sources))
		out = append(out, rest[:to]...)
		out = append(out, moved)
		out = append(out, rest[to:]...)
		m.Sources = out
		applySourceMutation(presenter, guiState)
	})

	refreshSourcesList := func() {
		sourcesBox.Objects = sourcesBox.Objects[:0]
		// Ссылка на подсвеченную строку живёт ровно один набор строк:
		// пересборка списка делает прежний виджет мусором, и прокрутка к
		// нему увезла бы список в никуда.
		revealedRow = nil
		// Группа перетаскивания живёт вместе со строками (контракт
		// DragReorderGroup): без сброса запись удалённой строки со старшим
		// индексом оставалась бы навсегда — перетаскивание в конец списка
		// «не срабатывало», а полоса могла разрешиться в чужой индекс.
		dragGroup.Reset()
		m := presenter.Model()
		if len(m.Sources) == 0 {
			emptyGutter := components.NewScrollGutter()
			sourcesBox.Add(container.NewHBox(widget.NewLabel(locale.T("No sources defined in ParserConfig.")), layout.NewSpacer(), emptyGutter))
			sourcesBox.Refresh()
			return
		}

		for i := range m.Sources {
			// IIFE so each row's closures capture the correct index (avoids loop variable capture bug)
			func(sourceIndex int) {
				var row *fynewidget.HoverRow
				rowGetter := func() *fynewidget.HoverRow { return row }

				srcPtr := &m.Sources[sourceIndex]
				src := *srcPtr

				isSubscription := src.Kind == corestate.SourceTypeSubscription
				meta := src.Meta
				sourceID := src.ID

				// Label / tooltip data из v5 Source (canonical).
				// SPEC 052 phase 8: для подписки приоритет — profile_title (читабельно
				// для человека), URL уходит в tooltip + Edit-окно. Для server —
				// label или URI fragment.
				label := ""
				if isSubscription {
					if meta != nil && strings.TrimSpace(meta.ProfileTitle) != "" {
						label = strings.TrimSpace(meta.ProfileTitle)
					} else {
						label = src.URL
					}
				} else {
					// Подпись, а при её отсутствии — тег узла: у server и
					// chain тег и есть то имя, под которым источник знают
					// правила, и показывать вместо него «Source N» значит
					// прятать единственный опознавательный признак.
					label = src.Label
					if label == "" {
						label = src.NodeTag
					}
					if label == "" {
						label = src.URI
					}
					if label == "" {
						// Fallback: первый node tag из preview (если есть).
						if m.PreviewNodesBySource != nil &&
							sourceIndex < len(m.PreviewNodesBySource) &&
							len(m.PreviewNodesBySource[sourceIndex]) > 0 {
							first := m.PreviewNodesBySource[sourceIndex][0]
							if first.Tag != "" {
								label = first.Tag
							} else if first.Label != "" {
								label = first.Label
							}
						}
					}
					if label == "" {
						label = locale.Tf("Source %d", sourceIndex+1)
					}
				}
				// Счётчик узлов: подписка на полсотни серверов, у которой
				// половина выключена галками, выглядит в списке так же,
				// как полная. Показываем «сколько пойдёт в конфиг», а при
				// расхождении — и сколько всего.
				if c, ok := m.SourceNodeCounts[sourceIndex]; ok && c.Total > 0 {
					if c.Enabled == c.Total {
						label += "  " + locale.Tf("· %d nodes", c.Total)
					} else {
						label += "  " + locale.Tf("· %d of %d nodes", c.Enabled, c.Total)
					}
				}

				// SPEC 110: цепочку видно по строке — иначе она неотличима
				// от сервера, а ведёт себя иначе. Если ядро её не умеет,
				// вместо метки идёт предупреждение: узел в конфиг не
				// попадёт, и узнать об этом по факту пропавшего маршрута —
				// худший из способов.
				if src.Kind == corestate.SourceTypeChain {
					if supported, _ := config.ChainSupportedByCore(); supported {
						label += "  " + locale.Tf("[chain: %d]",
							len(src.Chain.HopsOrNil()))
					} else {
						label += "  " + locale.T("[chain] ⚠️ core has no chain support")
					}
				}
				label = wizardutils.TruncateStringEllipsis(label, wizardutils.MaxLabelRunes, "...")
				shortLabel := label

				// SPEC 112-B часть B: источник, выпавший из конфига
				// fail-closed (detour-хоп не разрешился), обязан быть виден
				// ЗДЕСЬ. Раньше исключение уходило только в лог, а строка
				// продолжала показывать галку и «N nodes» — парадокс Proton
				// NL: настройка на месте, трафика нет. Пометка живёт, пока
				// живёт причина: реестр целиком переписывается каждой
				// сборкой, и чистая сборка её снимает.
				exclusionReason := config.ExcludedSourceReason(sourceID)
				// SPEC 115 §3: у источника, который в конфиг попал, но
				// потерял часть узлов на последнем рубеже, — МЯГКАЯ пометка,
				// не ⚠-исключение. Разница содержательная: исключённый
				// источник не работает вовсе, а этот работает урезанным, и
				// показать второе как первое значило бы объявить потерянным
				// то, что живо.
				//
				// До первой сборки и после правки модели реестр пуст, и обе
				// пометки молчат — раскраска не имеет права врать о
				// конфигурации, которую никто не собирал.
				droppedNodes, droppedReason := config.DroppedNodesForSource(sourceID)
				// SPEC 115: источник, не давший конфигу ни одного узла
				// (не фетчнулся, или разобрался в ноль). Раньше это жило
				// одним WARN в логе — «source returned zero nodes» — и
				// пользователь не видел НИЧЕГО: строка показывала галку и
				// счётчик узлов от прошлой удачной сборки. Пометка та же
				// ⚠, что у исключения, но причина принципиально другая:
				// чинить надо саму подписку, а не ссылку на узел.
				parseFailedReason := config.ParseFailedSourceReason(sourceID)

				fullURL := src.URL
				var tagPrefix, tagPostfix, tagMask string
				if src.TagPolicy != nil {
					tagPrefix = src.TagPolicy.Prefix
					tagPostfix = src.TagPolicy.Postfix
					tagMask = src.TagPolicy.Mask
				}

				localTags := make([]string, 0, len(src.Outbounds))
				for _, ob := range src.Outbounds {
					if ob.Tag != "" {
						localTags = append(localTags, ob.Tag)
					}
				}

				tooltipLines := []string{
					fmt.Sprintf("URL: %s", fullURL),
					fmt.Sprintf("tag_prefix: %s", tagPrefix),
					fmt.Sprintf("tag_postfix: %s", tagPostfix),
					fmt.Sprintf("tag_mask: %s", tagMask),
					fmt.Sprintf("local outbounds: %d", len(localTags)),
				}
				if len(localTags) > 0 {
					tooltipLines = append(tooltipLines, "tags: "+strings.Join(localTags, ", "))
				}
				if metaTip := metaTooltip(meta); metaTip != "" {
					tooltipLines = append(tooltipLines, "—— meta ——", metaTip)
				}
				tooltipText := strings.Join(tooltipLines, "\n")

				copyText := fullURL
				if copyText == "" {
					copyText = src.URI
				}
				sourceLabel := ttwidget.NewLabel(shortLabel)
				sourceLabel.Wrapping = fyne.TextWrapOff
				sourceLabel.Truncation = fyne.TextTruncateEllipsis

				// SPEC 052 phase 8: type indicator убран как визуальный шум — тип
				// и так читается из URL (https://) vs URI (vless://, wireguard://);
				// в Edit-окне есть Overview tab с явным "Type: Subscription/Server".
				var leftBlock fyne.CanvasObject
				var prefixLabel *ttwidget.Label
				if pfx := strings.TrimSpace(tagPrefix); pfx != "" {
					pfxShow := wizardutils.TruncateStringEllipsis(pfx, 24, "...")
					prefixLabel = ttwidget.NewLabel(pfxShow)
					prefixLabel.Importance = widget.MediumImportance
					if pfxShow != pfx {
						prefixLabel.SetToolTip(pfx)
					}
					leftBlock = prefixLabel
				}
				_ = tagPostfix
				var rowCenter fyne.CanvasObject = container.NewBorder(nil, nil, leftBlock, nil, sourceLabel)

				// Enable/disable toggle — persists to Source.Enabled.
				// Dim the label importance so disabled rows are visibly inactive.
				enableCheck := widget.NewCheck("", nil)
				enableCheck.SetChecked(srcPtr.Enabled)
				if !srcPtr.Enabled {
					sourceLabel.Importance = widget.LowImportance
					if prefixLabel != nil {
						prefixLabel.Importance = widget.LowImportance
					}
				}
				enableCheck.OnChanged = func(enabled bool) {
					m := presenter.Model()
					if sourceIndex >= len(m.Sources) {
						return
					}
					m.Sources[sourceIndex].Enabled = enabled
					// Shared mutation chain (marks dirty, re-derives, refreshes
					// outbound options + list). The MarkAsChanged rationale and
					// the previously-missing RefreshOutboundOptions live there.
					applySourceMutation(presenter, guiState)
				}

				copyBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.ContentCopyIcon(), func() {
					if copyText == "" {
						return
					}
					if guiState.Window != nil {
						fyne.CurrentApp().Clipboard().SetContent(copyText)
						dialogs.ShowAutoHideInfo(fyne.CurrentApp(), guiState.Window, locale.T("Copied"), locale.T("Source copied to clipboard."))
					}
				}, rowGetter)
				copyBtn.Importance = widget.LowImportance
				sourceLabel.SetToolTip(tooltipText)
				fynewidget.SetToolTipSafe(copyBtn, tooltipText)

				editBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.DocumentCreateIcon(), func() {
					presenter.MergeGUIToModel()
					m := presenter.Model()
					if m == nil || sourceIndex >= len(m.Sources) {
						return
					}
					showSourceEditWindow(presenter, guiState, guiState.Window, sourceIndex, shortLabel)
				}, rowGetter)
				editBtn.Importance = widget.LowImportance
				fynewidget.SetToolTipSafe(editBtn, locale.T("Edit"))

				delBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.DeleteIcon(), func() {
					// Confirm before removing — deletion drops the source (and its
					// nodes) from the config; matches the Rules-tab delete UX.
					dialog.ShowConfirm(
						locale.T("Confirmation"),
						locale.Tf("Delete \"%s\"? The source and its nodes are removed from the configuration.", shortLabel),
						func(ok bool) {
							if !ok {
								return
							}
							m := presenter.Model()
							if sourceIndex >= len(m.Sources) {
								return
							}
							m.Sources = append(m.Sources[:sourceIndex], m.Sources[sourceIndex+1:]...)
							applySourceMutation(presenter, guiState)
						},
						guiState.Window,
					)
				}, rowGetter)
				delBtn.Importance = widget.LowImportance
				fynewidget.SetToolTipSafe(delBtn, locale.T("Del"))

				// SPEC 052 phase 8: статус из subtitle (⚠ при err); badge на главной
				// строке убран как избыточный. Refresh-icon только для подписок.
				var refreshBtn *fynewidget.HoverForwardButton
				if isSubscription && sourceID != "" {
					refreshBtn = fynewidget.NewHoverForwardButtonWithIcon("", theme.ViewRefreshIcon(), func() {
						refreshOneSourceFromUI(presenter, guiState, sourceID)
					}, rowGetter)
					refreshBtn.Importance = widget.LowImportance
					fynewidget.SetToolTipSafe(refreshBtn, locale.T("Fetch this subscription now"))
				}

				// SPEC 061 Phase 3: ⚠ / 📢 icon-button — persistent affordance to
				// open the source-error dialog when meta carries an error or a
				// provider announce. Placed to the LEFT of copy/edit so the
				// row's edit/delete cluster keeps a stable visual position.
				var noticeBtn *fynewidget.HoverForwardButton
				if isSubscription && meta != nil && (meta.LastStatus == "err" || (meta.ProviderAnnounce != nil && !meta.ProviderAnnounce.IsEmpty())) {
					icon := theme.WarningIcon()
					tooltipKey := "Subscription update failed — click for details" // l10n-key
					if meta.LastStatus != "err" {
						// Success-with-notice path: provider sent content + announce.
						// Use info-styled icon. We don't have an info-theme icon
						// in our minimal set, fall back to QuestionIcon (📢-ish).
						icon = theme.QuestionIcon()
						tooltipKey = "Provider sent a notice — click to read" // l10n-key
					}
					srcLabel := shortLabel
					metaCopy := meta // capture by value for closure (meta is *SubscriptionMeta, stable)
					noticeBtn = fynewidget.NewHoverForwardButtonWithIcon("", icon, func() {
						wizarddialogs.ShowSourceErrorDialog(guiState.Window, srcLabel, metaCopy)
					}, rowGetter)
					noticeBtn.Importance = widget.LowImportance
					fynewidget.SetToolTipSafe(noticeBtn, locale.T(tooltipKey))
				}

				// Порядок — обычный порядок слайса model.Sources; сохраняется
				// в state.connections.sources на Save (и подписки, и прямые
				// серверы живут в одном слайсе).
				dragHandle := fynewidget.NewDragHandle(dragGroup, sourceIndex, rowGetter)

				rowGutter := components.NewScrollGutter()
				rightControlsItems := []fyne.CanvasObject{}
				if noticeBtn != nil {
					rightControlsItems = append(rightControlsItems, noticeBtn)
				}
				// SPEC 069 feature: provider support / web-page link — small inline
				// icon in the info panel (TG plane / link), tooltip = URL, click opens.
				// No extra row height; nil for sources without a support URL.
				if supportBtn := supportLinkButton(meta, rowGetter); supportBtn != nil {
					rightControlsItems = append(rightControlsItems, supportBtn)
				}
				rightControlsItems = append(rightControlsItems, copyBtn, editBtn)
				if refreshBtn != nil {
					rightControlsItems = append(rightControlsItems, refreshBtn)
				}
				rightControlsItems = append(rightControlsItems, delBtn)
				// Pack the action icons tightly (tightHBox with a negative gap),
				// then keep the scroll gutter separated at the right edge with the
				// normal HBox padding so it still reserves the scrollbar strip.
				rightControls := container.NewHBox(
					container.New(tightHBox{spacing: rowIconGap}, rightControlsItems...),
					rowGutter,
				)
				// Guideline (Rules tab): ручка перетаскивания идёт ЛЕВЕЕ галки
				// включения, кнопки действий остаются справа.
				leftLead := container.NewHBox(dragHandle, fynewidget.CheckLeadingWrap(enableCheck))
				titleRow := container.NewBorder(nil, nil, leftLead, rightControls, rowCenter)

				// Subtitle row: meta inline (nodes / interval / fetched / quota / expires).
				// tightVBox — custom layout без theme.Padding между title/subtitle
				// (стандартный VBox / Border даёт ~12px воздуха, slишком много).
				//
				// Отступ дополнительных строк равен ширине ведущего кластера
				// (ручка + галка) — тогда они начинаются ровно под заголовком,
				// который в titleRow тоже стоит за leftLead. Хардкод тут уже
				// ломался при смене состава кластера.
				leftPad := func() fyne.CanvasObject {
					pad := canvas.NewRectangle(color.Transparent)
					pad.SetMinSize(fyne.NewSize(leftLead.MinSize().Width, 0))
					return pad
				}
				lines := []fyne.CanvasObject{titleRow}
				if isSubscription {
					if subtitle := formatSourceSubtitle(meta, srcPtr.Update, m.Defaults.Reload); subtitle != "" {
						subtitleText := canvas.NewText(subtitle, theme.Color(theme.ColorNamePlaceHolder))
						subtitleText.TextSize = theme.CaptionTextSize()
						lines = append(lines, container.NewBorder(nil, nil, leftPad(), nil, subtitleText))
					}
				}
				if exclusionReason != "" {
					// Wrapping обязателен: Label без него отдаёт всю строку как
					// min-width и раздувает окно визарда на весь экран
					// (fyne-ловушка). Причина бывает длинной — имя источника
					// плюс имя ненайденного узла.
					warn := widget.NewLabel(locale.Tf("⚠ Excluded from the config: %s", exclusionReason))
					warn.Wrapping = fyne.TextWrapWord
					warn.Importance = widget.WarningImportance
					warn.TextStyle = fyne.TextStyle{Italic: true}
					lines = append(lines, container.NewBorder(nil, nil, leftPad(), nil, warn))
				} else if parseFailedReason != "" {
					// После исключения, но ДО «снято N»: источник без узлов
					// снятых узлов не имеет, а исключение (ссылка не
					// разрешилась) ближе к корню, если случилось и то и другое.
					empty := widget.NewLabel(locale.Tf("⚠ No nodes from this source: %s", parseFailedReason))
					empty.Wrapping = fyne.TextWrapWord
					empty.Importance = widget.WarningImportance
					empty.TextStyle = fyne.TextStyle{Italic: true}
					lines = append(lines, container.NewBorder(nil, nil, leftPad(), nil, empty))
				} else if droppedNodes > 0 {
					// else if: источник, выпавший целиком, узлов уже не имеет —
					// вторая строка про «снято N» рядом с «исключён» была бы
					// про один и тот же факт дважды.
					dropped := widget.NewLabel(locale.Tf("⚠ %d node(s) dropped: %s", droppedNodes, droppedReason))
					dropped.Wrapping = fyne.TextWrapWord
					dropped.Importance = widget.MediumImportance
					dropped.TextStyle = fyne.TextStyle{Italic: true}
					lines = append(lines, container.NewBorder(nil, nil, leftPad(), nil, dropped))
				}
				var rowInner fyne.CanvasObject = titleRow
				if len(lines) > 1 {
					rowInner = container.New(tightVBox{}, lines...)
				}

				// Подсветка строки — половина перехода «показать источник» из
				// отчёта «Итога» (SPEC 115 §3): прокрутки мало, в списке из
				// сорока подписок глаз без выделения не находит нужную.
				rowSourceID := sourceID
				row = fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{
					IsSelected: func() bool {
						return rowSourceID != "" && rowSourceID == revealedSourceID
					},
				})
				if rowSourceID != "" && rowSourceID == revealedSourceID {
					revealedRow = row
				}
				// Регистрируем КАЖДУЮ строку: вычисление точки вставки
				// просматривает полосы всех строк, не только перетаскиваемой.
				dragGroup.Register(sourceIndex, row)
				row.WireTooltipLabelHover(sourceLabel)
				if prefixLabel != nil {
					row.WireTooltipLabelHover(prefixLabel)
				}
				sourcesBox.Add(row)
			}(i)
		}

		sourcesBox.Refresh()
	}

	// Ensure sources list is initialized from current model state
	refreshSourcesList()
	guiState.RefreshSourcesList = refreshSourcesList

	sourcesScroll := container.NewVScroll(sourcesBox)
	sourcesScroll.SetMinSize(fyne.NewSize(0, 80))

	// SPEC 115 §3: переход «показать источник» из отчёта «Итога».
	//
	// Список пересобирается целиком (строки — не переиспользуемые ячейки
	// widget.List), поэтому подсветка ставится ДО пересборки, а прокрутка —
	// после: только тогда на руках свежий виджет строки.
	guiState.RevealSource = func(sourceID string) {
		revealedSourceID = strings.TrimSpace(sourceID)
		refreshSourcesList()
		if revealedRow == nil {
			// Источник могли удалить между сборкой и кликом по строке
			// отчёта — законный исход: вкладку показали, прокручивать не к
			// чему.
			return
		}
		sourcesScroll.ScrollToOffset(fyne.NewPos(0, rowOffsetInBox(sourcesBox, revealedRow)))
	}

	previewAllBtn := widget.NewButton(locale.T("Preview all servers…"), func() {
		showSourcePreviewAllWindow(presenter)
	})
	sourcesHeader := container.NewHBox(
		sourcesLabel,
		layout.NewSpacer(),
		previewAllBtn,
	)

	// Без ведущего разделителя: AppTabs уже рисует свой divider под строкой
	// вкладок (container/apptabs.go), и собственная линия первым элементом
	// давала две полоски подряд. Разделитель между URL и списком остаётся —
	// он делит блоки, а не дублирует рамку вкладки.
	topBlock := container.NewVBox(
		urlContainer,
		widget.NewSeparator(),
		sourcesHeader,
	)

	tabScrollGutter := components.NewScrollGutter()

	// Sources list fills remaining tab height (preview all servers moved to a separate window).
	body := container.NewBorder(
		topBlock,
		nil,
		nil,
		tabScrollGutter,
		sourcesScroll,
	)

	return body
}

// applySourceMutation is the single refresh chain every Sources-list mutation
// runs after editing model.Sources (перетаскивание, enable toggle, delete):
// mark dirty → bump revision → invalidate preview cache →
// refresh configurator list → refresh outbound options → rebuild the list.
//
// MarkAsChanged is called explicitly (and first) on purpose: the refresh
// chain below does not touch widgets whose OnChanged marks the state dirty,
// so without this the mutation would be silently lost on close and the Save
// button wouldn't light up.
//
// Keeping all source mutations on this one helper is deliberate — the chain
// drifted before (the enable toggle used to skip RefreshOutboundOptions, so a
// disabled source's outbounds lingered in the rule selectors). Add new source
// mutations here, not as a fresh inline copy.
func applySourceMutation(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState) {
	m := presenter.Model()
	if m == nil {
		return
	}
	// MarkAsChanged — первым, намеренно (см. комментарий к функции выше).
	presenter.MarkAsChanged()
	m.BumpRevision()
	m.PreviewNeedsParse = true
	wizardbusiness.InvalidatePreviewCache(m)
	presenter.RefreshOutboundsConfiguratorList()
	presenter.RefreshOutboundOptions()
	if guiState != nil && guiState.RefreshSourcesList != nil {
		guiState.RefreshSourcesList()
	}
	// Пользователь СМОТРИТ на вкладку (мутация делается с неё) — счётчики
	// пересчитываются сразу в фоне, а не при следующем заходе на вкладку:
	// иначе тумблер одного источника гасил цифры у всех строк до конца
	// визита.
	go func() {
		if !wizardbusiness.EnsureSourceNodeCounts(m) {
			return
		}
		fyne.Do(func() {
			if guiState != nil && guiState.RefreshSourcesList != nil {
				guiState.RefreshSourcesList()
			}
		})
	}()
}

// showSourcePreviewAllWindow opens a window with the combined server list from all sources (uses View window slot).
func showSourcePreviewAllWindow(presenter *wizardpresentation.WizardPresenter) {
	if presenter == nil {
		return
	}
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

	win := app.NewWindow(locale.T("Servers from all sources"))
	presenter.SetViewWindow(win)
	win.SetOnClosed(func() {
		presenter.ClearViewWindow()
		presenter.UpdateChildOverlay()
	})

	var previewNodes []*config.ParsedNode
	previewStatusLabel := widget.NewLabel(locale.T("Click Refresh to load servers from all sources."))
	previewStatusLabel.Wrapping = fyne.TextWrapOff
	previewStatusScroll := container.NewHScroll(previewStatusLabel)
	previewList := widget.NewList(
		func() int { return len(previewNodes) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id int, o fyne.CanvasObject) {
			if id < len(previewNodes) {
				o.(*widget.Label).SetText(nodeDisplayLine(previewNodes[id]))
			}
		},
	)

	refreshPreview := func() {
		m := presenter.Model()
		if len(m.Sources) == 0 {
			previewNodes = nil
			previewList.Refresh()
			previewStatusLabel.SetText(locale.T("No sources. Add URLs and click Refresh."))
			return
		}
		previewStatusLabel.SetText(locale.T("Loading..."))

		go func() {
			mm := m
			errorCount, err := wizardbusiness.RebuildPreviewCache(mm)
			presenter.UpdateUI(func() {
				if err != nil {
					previewNodes = nil
					previewList.Refresh()
					previewStatusLabel.SetText(locale.Tf("Error: %s", err.Error()))
					return
				}
				previewNodes = mm.PreviewNodes
				previewList.Refresh()
				sourcesCount := len(mm.Sources)
				status := locale.Tf("%d server(s) from %d source(s)", len(previewNodes), sourcesCount)
				if errorCount > 0 {
					status += locale.Tf("  ⚠️ %d error(s)", errorCount)
				}
				previewStatusLabel.SetText(status)
			})
		}()
	}

	refreshBtn := widget.NewButton(locale.T("🔄 Refresh from sources"), refreshPreview)
	closeBtn := widget.NewButton(locale.T("Close"), func() { win.Close() })
	topRow := container.NewBorder(nil, nil, nil, refreshBtn, previewStatusScroll)
	listStrip := components.NewScrollGutter()
	previewScroll := container.NewScroll(previewList)
	previewScroll.Direction = container.ScrollVerticalOnly
	listRow := container.NewBorder(nil, nil, nil, listStrip, previewScroll)
	bottomRow := container.NewHBox(layout.NewSpacer(), closeBtn)

	minList := canvas.NewRectangle(color.Transparent)
	minList.SetMinSize(fyne.NewSize(0, 320))
	listFill := container.NewStack(minList, listRow)

	content := container.NewBorder(
		container.NewVBox(topRow, widget.NewSeparator()),
		bottomRow,
		nil, nil,
		listFill,
	)

	win.SetContent(content)
	win.Resize(fyne.NewSize(560, 520))
	win.CenterOnScreen()
	refreshPreview()
	win.Show()
	presenter.UpdateChildOverlay()
}

// nodeDisplayLine returns a short one-line description for a parsed node (for list display).
// textnorm.NormalizeProxyDisplay repairs UTF-8 and maps ❯/»/› to ASCII " > " for Fyne on macOS.
func nodeDisplayLine(node *config.ParsedNode) string {
	if node == nil {
		return ""
	}
	// Ветки Label здесь нет намеренно: у разобранного узла Tag не бывает
	// пустым — парсер подставляет `scheme-server-port` (generateDefaultTag),
	// когда имени в подписке не оказалось. Фолбэк на Label был недостижим и
	// создавал впечатление, будто это два взаимозаменяемых имени.
	var s string
	switch {
	case node.Tag != "":
		s = node.Tag
	case node.Server != "":
		return fmt.Sprintf("%s:%d", node.Server, node.Port)
	default:
		s = node.Scheme
	}
	return textnorm.NormalizeProxyDisplay(s)
}

// CreateDirectionsTab — вкладка «Направления» (SPEC 104).
//
// Направление — именованная точка выбора, на которую ссылаются правила.
// Прежний редактор ParserConfig JSON отсюда убран: подписки правятся на
// вкладке Sources, интервал обновления — в defaults, а сырой JSON остался
// там, где он и нужен, — внутри окна одного направления.
func CreateDirectionsTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	guiState := presenter.GUIState()

	onConfiguratorApply := func() {
		m := presenter.Model()
		// SPEC 117: конфигуратор мутирует canonical
		// (model.GlobalOutbounds / model.Sources[i].Outbounds) напрямую —
		// копировать назад больше нечего. Здесь остаются только производные
		// эффекты правки: протухание превью и обновление зависимых списков.
		m.BumpRevision()
		m.PreviewNeedsParse = true
		wizardbusiness.InvalidatePreviewCache(m)
		presenter.RefreshOutboundsConfiguratorList()
		presenter.RefreshOutboundOptions()
		if guiState.RefreshSourcesList != nil {
			guiState.RefreshSourcesList()
		}
		// Мутации списка (Edit/Add/Delete, ↑/↓) обязаны помечать состояние
		// изменённым явно: refresh-цепочка выше OnChanged-виджеты не трогает.
		presenter.MarkAsChanged()
	}

	configuratorContent, refreshOutboundsConfigurator := outbounds_configurator.NewConfiguratorContent(guiState.Window, presenter, onConfiguratorApply)
	guiState.RefreshOutboundsConfiguratorList = refreshOutboundsConfigurator

	// Ведущий разделитель убран — см. topBlock выше: строку под вкладками
	// рисует сам AppTabs. Замыкающий оставлен: он отбивает список от низа
	// окна.
	content := container.NewVBox(
		configuratorContent,
		widget.NewSeparator(),
	)

	scrollContainer := container.NewScroll(content)
	// Adaptive min-height: a fixed 620px forced the whole wizard window taller
	// than small laptop screens (Big Sur 1280×800), pushing nav buttons under
	// the Dock. Scale with window height; the fallback (used before the window
	// is measured at first layout) is kept small enough to fit a 600px window.
	scrollContainer.SetMinSize(adaptiveScrollSize(guiState, 0.62, 440))

	return scrollContainer
}

// refreshOneSourceFromUI — SPEC 052 phase 7: per-source Refresh button click handler.
//
// Использует RefreshSourceInPlace (in-memory path) вместо
// RefreshSingleSubscription (state.json path), чтобы Refresh работал на cold
// start — когда state.json ещё нет, потому что пользователь не нажимал Save.
// Каноничный Source хранится в model; refresh fetch'ит, пишет .raw на диск,
// и обновляет Meta в нашей snapshot-копии. На UI thread snapshot ассайнится
// обратно в model. State.json не трогается — он запишется при следующем Save
// пользователем (теперь уже со свежей Meta).
//
// Race protection: snapshot источника берётся на UI thread (включая deep-copy
// Meta), goroutine мутирует свою копию, на UI thread snapshot переезжает в
// model. Параллельный Add нового source не разваливается — slice может
// reallocate'нуться, мы по ID находим место заново.
func refreshOneSourceFromUI(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	sourceID string,
) {
	// UI thread: snapshot Source из model. Deep-copy Meta — иначе goroutine
	// мутирует общий объект (refreshOneSubscriptionSource на failure-path
	// дёргает src.Meta.X = ... через pointer).
	m := presenter.Model()
	var snapshot corestate.Source
	found := false
	for i := range m.Sources {
		if m.Sources[i].ID == sourceID {
			snapshot = m.Sources[i]
			if snapshot.Meta != nil {
				metaCopy := *snapshot.Meta
				snapshot.Meta = &metaCopy
			}
			found = true
			break
		}
	}
	if !found {
		return
	}

	configService := presenter.ConfigServiceAdapter()
	go func() {
		_, err := configService.RefreshSourceInPlace(&snapshot)
		presenter.UpdateUI(func() {
			if err != nil {
				if guiState != nil && guiState.Window != nil && fyne.CurrentApp() != nil {
					dialogs.ShowAutoHideInfo(fyne.CurrentApp(), guiState.Window,
						locale.T("Refresh"),
						locale.Tf("Refresh failed: %s", err.Error()))
				}
				return
			}
			// Snapshot обратно в model. Slice мог reallocate'нуться (Add /
			// Del между snapshot-таймом и сейчас), поэтому ищем по ID заново.
			m := presenter.Model()
			for i := range m.Sources {
				if m.Sources[i].ID == sourceID {
					m.Sources[i] = snapshot
					break
				}
			}
			// Обновление меняет СОСТАВ узлов — кэш превью и счётчики на
			// строках обязаны пересчитаться, иначе кандидаты позиций
			// цепочки и «50 nodes» живут телом до обновления.
			wizardbusiness.InvalidatePreviewCache(m)
			if guiState != nil && guiState.RefreshSourcesList != nil {
				guiState.RefreshSourcesList()
			}
			if guiState != nil && guiState.Window != nil && fyne.CurrentApp() != nil {
				dialogs.ShowAutoHideInfo(fyne.CurrentApp(), guiState.Window,
					locale.T("Refresh"),
					locale.T("Refreshed successfully"))
			}
			// Mark dirty: model.Sources[].Meta изменился, при следующем Save
			// эти изменения уедут в state.json. Это пользовательский edit-ish
			// — даём ему dirty marker, чтобы Save-кнопка светилась.
			presenter.MarkAsChanged()
		})
	}()
}

// rowOffsetInBox — вертикальное смещение строки внутри вертикального
// контейнера, посчитанное по МИНИМАЛЬНЫМ размерам предыдущих строк, а не по
// Position().Y самой строки.
//
// Position().Y здесь не годится: список пересобирается целиком, и прокрутка
// зовётся сразу за пересборкой — layout к этому моменту ещё не отработал, у
// свежесозданных виджетов Position нулевая, и «прокрутка к источнику»
// молча уезжала в начало списка. Ждать layout'а вторым fyne.Do — гонка с
// неизвестным числом кадров: разложиться контейнер может и позже.
//
// VBox выкладывает детей подряд с одним отступом между ними и берёт высоту
// каждого из MinSize — те же слагаемые, что суммируются здесь. MinSize
// считается по требованию, до всякого layout'а, поэтому ответ верен уже в
// момент пересборки. Невидимые дети пропускаются: VBox им места не отводит.
func rowOffsetInBox(box *fyne.Container, row fyne.CanvasObject) float32 {
	if box == nil || row == nil {
		return 0
	}
	pad := theme.Padding()
	var y float32
	for _, o := range box.Objects {
		if o == row {
			return y
		}
		if o == nil || !o.Visible() {
			continue
		}
		y += o.MinSize().Height + pad
	}
	// Строки нет в контейнере — прокручивать не к чему; ноль честнее
	// произвольной точки.
	return 0
}
