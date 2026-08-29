package tabs

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// buildOverviewTab — read-only сводка по source: identity, status, headers,
// quota. Содержимое pere-render'ится при `refreshOverviewTab` (вызывается
// при открытии вкладки и после Refresh-кнопки).
//
// Возвращает (rootCanvas, refresh).
func buildOverviewTab(presenter *wizardpresentation.WizardPresenter, sourceIndex int) (fyne.CanvasObject, func()) {
	body := container.NewVBox()
	scroll := container.NewVScroll(body)
	scroll.SetMinSize(fyne.NewSize(0, sourceEditSettingsScrollMinH))
	// Scrollbar gutter справа — чтобы контент не прижимался к скролл-баре.
	gutter := components.NewScrollGutter()
	rootWithGutter := container.NewBorder(nil, nil, nil, gutter, scroll)

	refresh := func() {
		t0 := time.Now()
		defer func() {
			debuglog.DebugLog("buildOverviewTab: refresh took %v", time.Since(t0))
		}()
		body.Objects = body.Objects[:0]
		m := presenter.Model()
		if m == nil || sourceIndex >= len(m.Sources) {
			body.Add(widget.NewLabel(locale.T("No meta yet — press Refresh to fetch this subscription.")))
			body.Refresh()
			return
		}
		src := m.Sources[sourceIndex]

		// === Identity ===
		body.Add(sectionHeader(locale.T("Status")))
		// Тип — по виду источника. Раньше их было два и подпись сводилась к
		// «подписка или сервер»; в v7 видов пять, и назвать цепочку
		// подпиской значило бы соврать в первой же строке обзора.
		typeLabel := sourceKindLabel(src.Kind)
		body.Add(kvRow(locale.T("Type"), typeLabel))
		body.Add(kvRow(locale.T("ID"), src.ID))
		if src.URL != "" {
			body.Add(kvRow(locale.T("URL"), src.URL))
		}
		if src.Origin != nil && src.Origin.Raw != "" {
			body.Add(kvRow(locale.T("Origin"), src.Origin.Raw))
		}
		// Тег и подпись — разные строки: раньше их роль делило одно поле,
		// и по обзору нельзя было понять, на что сошлётся правило.
		if tag := src.Tag; tag != "" {
			body.Add(kvRow(locale.T("Node tag"), tag))
		}
		if src.Name != "" {
			body.Add(kvRow(locale.T("Name"), src.Name))
		}
		if src.Label != "" {
			body.Add(kvRow(locale.T("Label"), src.Label))
		}
		body.Add(kvRow(locale.T("Enabled"), boolStr(src.Enabled)))

		// Диагностика fetch'а есть только у подписки: внешним владельцем
		// состава больше никто не обладает. Остальным видам показываем их
		// состав и запись хранилища — и НЕ предлагаем «нажать Refresh»:
		// обновлять папку или цепочку неоткуда, и такая подпись отправляла
		// бы пользователя искать кнопку, которой нет.
		if src.Kind != corestate.SourceKindSubscription {
			body.Add(widget.NewSeparator())
			lbl := widget.NewLabel(locale.T("Server source — meta is not collected (only fetched per subscription)."))
			lbl.Importance = widget.LowImportance
			lbl.Wrapping = fyne.TextWrapWord
			body.Add(lbl)
			if len(src.Nodes) > 0 {
				body.Add(widget.NewSeparator())
				body.Add(sectionHeader(locale.Tf("Nodes: %d", len(src.Nodes))))
				body.Add(nodeOriginList(src.Nodes))
			}
			appendStorageRecordSection(body, src)
			body.Refresh()
			return
		}

		meta := diagOf(&src)
		if meta == nil {
			body.Add(widget.NewSeparator())
			lbl := widget.NewLabel(locale.T("No meta yet — press Refresh to fetch this subscription."))
			lbl.Importance = widget.LowImportance
			lbl.Wrapping = fyne.TextWrapWord
			body.Add(lbl)
			appendStorageRecordSection(body, src)
			body.Refresh()
			return
		}

		// === Status (fetch history) ===
		body.Add(kvRow(locale.T("Last status"), formatStatusBadge(meta)))
		if at := meta.lastAttemptAt(); at != "" {
			body.Add(kvRow(locale.T("Last fetched"),
				fmt.Sprintf("%s (%s)", at, formatLastFetched(meta))))
		}
		if st := src.UpdateStatus; st != nil {
			if st.HTTPStatusCode > 0 {
				body.Add(kvRow(locale.T("HTTP status"), fmt.Sprintf("%d", st.HTTPStatusCode)))
			}
			if st.RawBodyBytes > 0 {
				body.Add(kvRow(locale.T("Body size"), humanizeBytes(st.RawBodyBytes)))
			}
		}
		if meta.nodesCount() > 0 {
			body.Add(kvRow(locale.T("Nodes fetched"), formatNodesCount(meta, 0)))
		}
		if c := meta.errorCount(); c > 0 {
			body.Add(kvRow(locale.T("Error count"), fmt.Sprintf("%d", c)))
		}
		if msg := meta.lastErrorMsg(); msg != "" {
			body.Add(kvRow(locale.T("Last error"), msg))
		}

		// === Headers ===
		hdr := src.Meta
		if hdr == nil {
			hdr = &corestate.SubMeta{}
		}
		hasHeaders := hdr.ProfileTitle != "" || hdr.ProfileUpdateIntervalHours > 0 ||
			hdr.SupportURL != "" || hdr.ProfileWebPageURL != "" || hdr.ContentDispositionFilename != ""
		if hasHeaders {
			body.Add(widget.NewSeparator())
			body.Add(sectionHeader(locale.T("Subscription metadata (HTTP headers)")))
			if hdr.ProfileTitle != "" {
				body.Add(kvRow(locale.T("Profile title"), hdr.ProfileTitle))
			}
			if hdr.ProfileUpdateIntervalHours > 0 {
				body.Add(kvRow(locale.T("Update interval"),
					fmt.Sprintf("%dh", hdr.ProfileUpdateIntervalHours)))
			}
			if hdr.SupportURL != "" {
				body.Add(kvRow(locale.T("Support URL"), hdr.SupportURL))
			}
			if hdr.ProfileWebPageURL != "" {
				body.Add(kvRow(locale.T("Web page"), hdr.ProfileWebPageURL))
			}
			if hdr.ContentDispositionFilename != "" {
				body.Add(kvRow(locale.T("Content-Disposition filename"), hdr.ContentDispositionFilename))
			}
		}

		// === Сообщение провайдера (announce) ===
		//
		// Заголовок `announce` мы разбирали и раньше, но показывали только в
		// диалоге ошибки и значком в списке: подписка, которая ФЕТЧИТСЯ
		// успешно и при этом отдаёт «⚠️ Произошла ошибка при получении
		// подписки», выглядела здоровой, а её сообщение не показывалось нигде.
		//
		// Показывается КАК ДАННЫЕ: чужой текст, без интерпретации и без
		// превращения в наш вывод, обрезанный до вменяемой длины (провайдер не
		// обязан быть краток). Отдельной секцией, а не строкой в блоке
		// заголовков: это обращение к пользователю, а не техническое поле.
		if msg := providerAnnounceText(meta); msg != "" {
			body.Add(widget.NewSeparator())
			body.Add(sectionHeader(locale.T("Provider message")))
			body.Add(kvRow(locale.T("Announcement"), msg))
			if a := meta.providerAnnounce(); a != nil && a.URL != "" {
				body.Add(kvRow(locale.T("Announcement URL"), a.URL))
			}
		}

		// === Quota ===
		if ui := meta.userInfo(); ui != nil && (ui.TotalBytes > 0 || ui.ExpireUnix > 0) {
			body.Add(widget.NewSeparator())
			body.Add(sectionHeader(locale.T("Traffic quota")))
			if ui.TotalBytes > 0 {
				used := ui.UploadBytes + ui.DownloadBytes
				remaining := ui.TotalBytes - used
				if remaining < 0 {
					remaining = 0
				}
				body.Add(kvRow(locale.TN(1, "Used"), humanizeBytes(used)))
				body.Add(kvRow(locale.T("Total"), humanizeBytes(ui.TotalBytes)))
				body.Add(kvRow(locale.T("Remaining"), humanizeBytes(remaining)))
				if pct := quotaPercentage(meta); pct > 0 {
					bar := widget.NewProgressBar()
					bar.SetValue(pct)
					body.Add(bar)
				}
			}
			if ui.ExpireUnix > 0 {
				expireAt := time.Unix(ui.ExpireUnix, 0)
				body.Add(kvRow(locale.T("Expires"),
					fmt.Sprintf("%s (%s)", expireAt.Format("2006-01-02 15:04"), formatExpire(meta))))
			}
		}

		// === Состав: узлы и их происхождение ===
		//
		// SPEC 118 Т8: блок «raw body» умер вместе с кэшем тел. Тела теперь
		// материализованы поузлово, и честный ответ на «что за подписка» —
		// её СОСТАВ, а не байты ответа сервера.
		body.Add(widget.NewSeparator())
		body.Add(sectionHeader(locale.Tf("Nodes: %d", len(src.Nodes))))
		if len(src.Nodes) > 0 {
			body.Add(nodeOriginList(src.Nodes))
		}

		appendStorageRecordSection(body, src)

		body.Refresh()
	}

	// Lazy: НЕ вызываем refresh() здесь. Overview по дефолту неактивный таб
	// (Settings — первый в NewAppTabs), а refresh() читает состав узлов +
	// DecodeSubscriptionContent для подписки с 1 MB Xray JSON body — это
	// ~10 сек на открытии окна. Refresh вызывается из tabs.OnSelected когда
	// юзер реально кликает Overview. До этого таб показывает пустой VBox.
	return rootWithGutter, refresh
}

// appendStorageRecordSection — блок «как источник записан в state.json»:
// каноническая запись v7 (SPEC 118 Т8) — ровно то, что уедет на диск, со
// всеми материализованными узлами, их origin и отметками включённости.
// Раньше этот снапшот был вкладкой JSON; переехал сюда, когда вкладка JSON
// стала показывать распакованный sing-box outbound.
func appendStorageRecordSection(body *fyne.Container, src corestate.Source) {
	body.Add(widget.NewSeparator())
	body.Add(sectionHeader(locale.T("Storage record (state.json)")))

	text := ""
	if b, err := json.MarshalIndent(src, "", "  "); err != nil {
		text = err.Error()
	} else {
		text = string(b)
	}

	// MultiLineEntry без Disable() — тот же приём, что у raw body выше:
	// disabled-текст на macOS рендерится цветом фона. Ввод откатывается.
	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapOff
	entry.SetText(text)
	entry.OnChanged = func(s string) {
		if s != text {
			entry.SetText(text)
		}
	}
	entryScroll := container.NewVScroll(container.NewStack(
		canvas.NewRectangle(transparentColor()),
		entry,
	))
	entryScroll.SetMinSize(fyne.NewSize(0, 240))
	body.Add(entryScroll)
}

// sectionHeader — bold-section-header label.
func sectionHeader(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.TextStyle = fyne.TextStyle{Bold: true}
	return l
}

// kvRow — label "Key: Value" с соответствующим стилем.
// providerAnnounceText — сообщение провайдера из метаданных источника; пусто,
// если провайдер ничего не присылал.
//
// Обрезка и схлопывание переносов — в state.AnnounceMessage: правило одно на
// все поверхности (Overview, Preview, пометка Sources, отчёт «Итога»), и
// разъехаться им нельзя.
func providerAnnounceText(meta *sourceDiag) string {
	if meta == nil {
		return ""
	}
	return meta.providerAnnounce().AnnounceMessage()
}

// nodeOriginList — список «сырой тег → origin.raw» узлов источника.
//
// Origin — то, из чего узел собран (share-URI, строка sing-box JSON): по нему
// пользователь узнаёт узел, даже когда финальный тег ушёл под тег-политику.
func nodeOriginList(nodes []corestate.Node) fyne.CanvasObject {
	var b strings.Builder
	for i := range nodes {
		n := &nodes[i]
		mark := ""
		if !n.Enabled {
			mark = " (off)"
		}
		raw := ""
		if n.Origin != nil {
			raw = n.Origin.Raw
		}
		b.WriteString(n.Tag)
		b.WriteString(mark)
		if raw != "" {
			b.WriteString("\n    ")
			b.WriteString(raw)
		}
		b.WriteString("\n")
	}
	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapOff
	text := b.String()
	entry.SetText(text)
	entry.OnChanged = func(s string) {
		if s != text {
			entry.SetText(text)
		}
	}
	scroll := container.NewVScroll(container.NewStack(
		canvas.NewRectangle(transparentColor()),
		entry,
	))
	scroll.SetMinSize(fyne.NewSize(0, 240))
	return scroll
}

func kvRow(key, value string) fyne.CanvasObject {
	if value == "" {
		value = "—"
	}
	keyLabel := widget.NewLabel(key + ":")
	keyLabel.Importance = widget.LowImportance
	valueLabel := widget.NewLabel(value)
	valueLabel.Wrapping = fyne.TextWrapBreak
	return container.NewBorder(nil, nil, keyLabel, nil, valueLabel)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// sourceKindLabel — подпись вида источника для обзора.
//
// Подписи взяты существующие (они уже живут в списке источников и в
// подсказках) — новых для W6 не заводится: правило ui-visuals-approve-first.
func sourceKindLabel(kind corestate.SourceKind) string {
	switch kind {
	case corestate.SourceKindServer:
		return locale.T("Server")
	case corestate.SourceKindChain:
		return locale.T("chain")
	case corestate.SourceKindAuto:
		return locale.T("group")
	case corestate.SourceKindFolder:
		return locale.T("Group")
	default:
		return locale.T("Subscription")
	}
}
