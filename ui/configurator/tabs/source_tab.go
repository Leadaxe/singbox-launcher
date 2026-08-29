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
		wizarddialogs.ShowAddWarpDialog(presenter, nil, applyAddedSources)
	}

	// «Add server» — ручная форма: SOCKS5/HTTP по полям либо Source (любой
	// текст, который понимает Add). У этих схем полей мало, а у HTTP-прокси
	// ещё и нестандартный префикс (proxy-http://), который человеку негде
	// подсмотреть. Форма собирает вход и отдаёт в тот же путь Add; вручную
	// отредактированный JSON идёт своей веткой, чтобы сохраниться побайтово.
	addServerAction := func() {
		wizarddialogs.ShowAddServerDialog(presenter, nil, func(res wizarddialogs.AddServerResult) {
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
			// Выданное имя — ТЕГ узла: на него сошлются фильтры и позиции.
			// Подпись остаётся пустой, и список показывает тег, пока
			// пользователь не задаст своё отображаемое имя.
			Node: corestate.Node{Kind: corestate.SourceKindChain, Enabled: true},
			ID:   corestate.MakeULID(),
		})
		m.Sources[len(m.Sources)-1].Tag = wizardbusiness.NextChainLabel(m.Sources)
		m.BumpRevision()
		m.PreviewNeedsParse = true
		wizardbusiness.InvalidateNodePool(m)
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

	// SPEC 116 этап 3 (сценарий С1): «Add folder» — пустой контейнер под
	// собственные узлы пользователя. Папка создаётся ПУСТОЙ и окно правки
	// НЕ открывается: в отличие от цепочки, пустая папка — законное
	// состояние (её наполняют потом, узел за узлом), и открывать форму
	// значило бы требовать настройки там, где настраивать нечего.
	//
	// Конструктор — существующий corestate.NewFolderSource: он же минтит
	// ULID, а ULID у папки единственная идентификация (на него смотрит
	// NodeLink.FolderID). Своего создания папки не заводить.
	addFolderAction := func() {
		presenter.MergeGUIToModel()
		m := presenter.Model()
		if m == nil {
			return
		}
		m.Sources = append(m.Sources, corestate.NewFolderSource(wizardbusiness.NextFolderName(m.Sources)))
		applySourceMutation(presenter, guiState)
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
			// SPEC 116 §O6: «Add folder» первым пунктом — папка это
			// контейнер, в который потом кладут всё остальное из этого же
			// меню; порядок читается как «сначала куда, потом что».
			fyne.NewMenuItem(locale.T("Add folder"), addFolderAction),
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

		// Дефолт интервала обновления — настройки приложения (SPEC 118 Т1);
		// читается один раз на перерисовку списка, а не на строку.
		defaultReload := locale.LoadSettings(platform.GetBinDir(m.ExecDir)).DefaultSubscriptionReload

		for i := range m.Sources {
			// IIFE so each row's closures capture the correct index (avoids loop variable capture bug)
			func(sourceIndex int) {
				var row *fynewidget.HoverRow
				rowGetter := func() *fynewidget.HoverRow { return row }

				srcPtr := &m.Sources[sourceIndex]
				src := *srcPtr

				isSubscription := src.Kind == corestate.SourceKindSubscription
				isFolder := src.Kind == corestate.SourceKindFolder
				meta := diagOf(&src)
				sourceID := src.ID

				// Label / tooltip data из v5 Source (canonical).
				// SPEC 052 phase 8: для подписки приоритет — profile_title (читабельно
				// для человека), URL уходит в tooltip + Edit-окно. Для server —
				// label или URI fragment.
				label := ""
				if isFolder {
					// SPEC 116 §O5 (вердикт А): строку папки НЕ декорируем —
					// ни метки, ни иконки. Отличие от подписки читается само:
					// у папки нет URL в подстроке и нет кнопки обновления.
					//
					// Имя папки живёт в Source.Name (не в Label — это
					// контейнер, а не узел); своего имени пользователь мог
					// ещё не дать только у папок из чужого состояния.
					label = strings.TrimSpace(src.Name)
					if label == "" {
						label = locale.Tf("Source %d", sourceIndex+1)
					}
				} else if isSubscription {
					if t := strings.TrimSpace(meta.profileTitle()); t != "" {
						label = t
					} else if src.Name != "" {
						label = src.Name
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
						label = src.Tag
					}
					if label == "" && src.Origin != nil {
						label = src.Origin.Raw
					}
					if label == "" {
						// Fallback: первый node tag из preview (если есть).
						if m.NodePoolBySource != nil &&
							sourceIndex < len(m.NodePoolBySource) &&
							len(m.NodePoolBySource[sourceIndex]) > 0 {
							first := m.NodePoolBySource[sourceIndex][0]
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
				} else if isFolder {
					// SPEC 116 A1: у пустой папки счётчик показывается явным
					// «0 nodes», а не пропадает. Пустота папки — это её
					// нормальное начальное состояние (только что создали,
					// ещё не наполнили), и молчание строки читалось бы как
					// «счётчик ещё не посчитан».
					label += "  " + locale.Tf("· %d nodes", 0)
				}

				// SPEC 110: цепочку видно по строке — иначе она неотличима
				// от сервера, а ведёт себя иначе. Если ядро её не умеет,
				// вместо метки идёт предупреждение: узел в конфиг не
				// попадёт, и узнать об этом по факту пропавшего маршрута —
				// худший из способов.
				if src.Kind == corestate.SourceKindChain {
					if supported, _ := config.ChainSupportedByCore(); supported {
						label += "  " + locale.Tf("[chain: %d]", len(src.Hops))
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
				// SPEC 116 A1: ПУСТАЯ ПАПКА — не сбой. У подписки ноль узлов
				// значит «что-то не так с провайдером»; у папки это воля
				// пользователя: он её создал и ещё не наполнил. Сборка
				// считает контейнер без узлов не давшим ни одного (общее
				// правило генератора), но объявлять папку сломанной здесь
				// нельзя — чинить в ней нечего.
				if isFolder && len(src.Nodes) == 0 {
					parseFailedReason = ""
				}
				// SPEC 116 W12 фикс 3: деградации ЭМИССИИ этого источника
				// (выпавший член Auto, нерезолвнутая позиция цепочки, снятое
				// умолчание, столкновение тегов). Раньше они уходили в отчёт
				// «Итога» без адресата, и строка списка про них молчала —
				// починить их можно только зная, у кого именно.
				emitWarnings := config.EmitWarningsForSource(sourceID)

				fullURL := src.URL
				var tagPrefix, tagPostfix string
				if src.TagPolicy != nil {
					tagPrefix = src.TagPolicy.Prefix
					tagPostfix = src.TagPolicy.Postfix
				}

				tooltipLines := []string{
					fmt.Sprintf("URL: %s", fullURL),
					fmt.Sprintf("tag_prefix: %s", tagPrefix),
					fmt.Sprintf("tag_postfix: %s", tagPostfix),
				}
				if src.Replace != nil {
					tooltipLines = append(tooltipLines,
						fmt.Sprintf("replace: %s (%s)", src.Replace.Tag, src.Replace.Mode))
				}
				if metaTip := metaTooltip(meta); metaTip != "" {
					tooltipLines = append(tooltipLines, "—— meta ——", metaTip)
				}
				tooltipText := strings.Join(tooltipLines, "\n")

				copyText := fullURL
				if copyText == "" && src.Origin != nil {
					copyText = src.Origin.Raw
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

				folderHasNodes := isFolder && len(src.Nodes) > 0
				delBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.DeleteIcon(), func() {
					// SPEC 116 сценарий С7: у НЕПУСТОЙ папки удаление — не
					// «да/нет», а выбор судьбы её узлов: снести вместе с
					// папкой либо вынести в корень. Обычное подтверждение
					// здесь предлагало бы ровно один исход и молча уносило
					// десяток настроенных узлов.
					if folderHasNodes {
						showFolderDeleteDialog(presenter, guiState, sourceID, shortLabel, len(src.Nodes))
						return
					}
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
				if isSubscription && meta != nil && (meta.lastStatus() == "err" || (meta.providerAnnounce() != nil && !meta.providerAnnounce().IsEmpty())) {
					icon := theme.WarningIcon()
					tooltipKey := "Subscription update failed — click for details" // l10n-key
					if meta.lastStatus() != "err" {
						// Success-with-notice path: provider sent content + announce.
						// Use info-styled icon. We don't have an info-theme icon
						// in our minimal set, fall back to QuestionIcon (📢-ish).
						icon = theme.QuestionIcon()
						tooltipKey = "Provider sent a notice — click to read" // l10n-key
					}
					srcLabel := shortLabel
					// Снимок обеих половин диагностики — диалог рисуется
					// вне владельца модели, разделять указатели нельзя.
					diagCopy := &wizarddialogs.SourceDiag{}
					if src.Meta != nil {
						diagCopy.Meta = *src.Meta
					}
					if src.UpdateStatus != nil {
						diagCopy.Status = *src.UpdateStatus
					}
					noticeBtn = fynewidget.NewHoverForwardButtonWithIcon("", icon, func() {
						wizarddialogs.ShowSourceErrorDialog(guiState.Window, srcLabel, diagCopy)
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
					if subtitle := formatSourceSubtitle(meta, srcPtr.Update, defaultReload); subtitle != "" {
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
				// Эмиссионные деградации — ОТДЕЛЬНОЙ строкой, а не в общей
				// цепочке else if: источник, потерявший члена группы, при этом
				// вполне может быть и урезанным на последнем рубеже, и это
				// разные факты. Мягкая пометка: узлы источник дал, работает он
				// частично.
				for _, w := range emitWarnings {
					// Причина уже переведена движком — здесь только знак.
					emitLabel := widget.NewLabel("⚠ " + w)
					emitLabel.Wrapping = fyne.TextWrapWord
					emitLabel.Importance = widget.MediumImportance
					emitLabel.TextStyle = fyne.TextStyle{Italic: true}
					lines = append(lines, container.NewBorder(nil, nil, leftPad(), nil, emitLabel))
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
	wizardbusiness.InvalidateNodePool(m)
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

// showFolderDeleteDialog — удаление НЕПУСТОЙ папки (SPEC 116 сценарий С7).
//
// Два исхода, а не «да/нет»: узлы в папке — собственность пользователя, и
// молча унести их вместе с контейнером нельзя.
//
//   - «Delete with nodes» — папка и её узлы уходят из конфигурации;
//   - «Move nodes to root» — каждый узел становится верхним Source
//     (business.ExtractFolderNodesToRoot, волна W2), после чего опустевшая
//     папка удаляется.
//
// Ссылки НА вынесенные узлы (detour, хопы, члены Auto) переписывает реестр
// W2 — они не рвутся, и сообщать о них нечего. А вот ФИНАЛЬНЫЙ тег узла у
// папки с тег-политикой меняется (в корне политики нет), и ручной выбор в
// селекторах живого ядра по нему протухает — про это предупреждает
// существующий showStaleSelectionDialog, своего диалога не заводим.
//
// Три кнопки не влезают в dialog.ShowConfirm, поэтому окно собрано
// NewCustomWithoutButtons — тем же приёмом, что диалог WARP.
//
// Ловушка Fyne (fyne-label-minwidth-trap): текст обязан быть Wrapping, иначе
// имя папки в одну строку задаёт окну min-width и раздувает диалог.
func showFolderDeleteDialog(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	folderID string,
	folderLabel string,
	nodeCount int,
) {
	if presenter == nil || guiState == nil || guiState.Window == nil || folderID == "" {
		return
	}

	body := widget.NewLabel(locale.Tf(
		"Folder %q holds %d node(s). Delete them together with the folder, or move them to the root of the sources list?",
		folderLabel, nodeCount))
	body.Wrapping = fyne.TextWrapWord

	var d *dialog.CustomDialog

	// Позиция папки ищется по ULID НА КЛИКЕ, а не берётся индексом из строки
	// списка: пока висел диалог, порядок Sources мог поменяться (фоновый
	// fetch, перетаскивание, второе окно), и удаление по устаревшему индексу
	// снесло бы чужой источник. ULID — единственная идентификация папки.
	folderIndex := func() int {
		m := presenter.Model()
		if m == nil {
			return -1
		}
		for i := range m.Sources {
			if m.Sources[i].ID == folderID && m.Sources[i].Kind == corestate.SourceKindFolder {
				return i
			}
		}
		return -1
	}

	deleteBtn := widget.NewButton(locale.T("Delete with nodes"), func() {
		if d != nil {
			d.Hide()
		}
		m := presenter.Model()
		idx := folderIndex()
		if m == nil || idx < 0 {
			return
		}
		m.Sources = append(m.Sources[:idx], m.Sources[idx+1:]...)
		applySourceMutation(presenter, guiState)
	})
	deleteBtn.Importance = widget.DangerImportance

	extractBtn := widget.NewButton(locale.T("Move nodes to root"), func() {
		if d != nil {
			d.Hide()
		}
		m := presenter.Model()
		idx := folderIndex()
		if m == nil || idx < 0 {
			return
		}
		// Политика читается ДО выноса: после него папки в модели уже нет.
		hadTagPolicy := m.Sources[idx].TagPolicy != nil && !m.Sources[idx].TagPolicy.IsZero()
		// Порядок обязателен: сначала вынести (функция W2 адресует папку по
		// индексу и вставляет узлы сразу ЗА ней), потом удалить опустевшую —
		// иначе вставлять было бы некуда и позиция узлов в списке уехала бы
		// в конец, за все подписки.
		wizardbusiness.ExtractFolderNodesToRoot(m, idx)
		m.Sources = append(m.Sources[:idx], m.Sources[idx+1:]...)
		applySourceMutation(presenter, guiState)
		if hadTagPolicy {
			showStaleSelectionDialog(guiState.Window, staleSelectionScope{NodesRenamed: true})
		}
	})

	cancelBtn := widget.NewButton(locale.T("Cancel"), func() {
		if d != nil {
			d.Hide()
		}
	})

	content := container.NewVBox(
		body,
		container.NewHBox(layout.NewSpacer(), cancelBtn, extractBtn, deleteBtn),
	)
	d = dialog.NewCustomWithoutButtons(locale.T("Delete folder"), content, guiState.Window)
	d.Resize(fyne.NewSize(520, 200))
	d.Show()
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
			errorCount, err := wizardbusiness.RebuildNodePool(mm)
			presenter.UpdateUI(func() {
				if err != nil {
					previewNodes = nil
					previewList.Refresh()
					previewStatusLabel.SetText(locale.Tf("Error: %s", err.Error()))
					return
				}
				previewNodes = mm.NodePool
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
		// (model.GlobalOutbounds) напрямую —
		// копировать назад больше нечего. Здесь остаются только производные
		// эффекты правки: протухание превью и обновление зависимых списков.
		m.BumpRevision()
		m.PreviewNeedsParse = true
		wizardbusiness.InvalidateNodePool(m)
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
	// SPEC 118 W6 (хвост ревью W3): ревизия модели на момент снятия снимка.
	// Пока горутина качает, пользователь вправе править ту же запись —
	// запись снимка целиком откатила бы его правки (см. ApplyFetchSnapshot).
	revAtStart := m.Revision

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
			// Del между snapshot-таймом и сейчас) — поиск по ID; а если
			// модель успели ПРАВИТЬ, заносятся только поля результата
			// fetch'а, а не снимок целиком (SPEC 118 W6, хвост ревью W3).
			m := presenter.Model()
			if !wizardbusiness.ApplyFetchSnapshot(m, &snapshot, revAtStart) {
				return
			}
			// Обновление меняет СОСТАВ узлов — кэш превью и счётчики на
			// строках обязаны пересчитаться, иначе кандидаты позиций
			// цепочки и «50 nodes» живут телом до обновления.
			wizardbusiness.InvalidateNodePool(m)
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
			// SPEC 118 W3: fetch-merge наполнил канонические nodes[] —
			// это мутация модели, производные конвейеры обязаны увидеть
			// новую ревизию («конфиг устарел», не автозапуск сборки).
			m.BumpRevision()
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
