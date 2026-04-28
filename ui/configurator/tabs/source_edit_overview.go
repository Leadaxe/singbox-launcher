package tabs

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config/subscription"
	corestate "singbox-launcher/core/state"
	v5 "singbox-launcher/core/state/v5"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
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
	gutter := canvas.NewRectangle(transparentColor())
	gutter.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))
	rootWithGutter := container.NewBorder(nil, nil, nil, gutter, scroll)

	refresh := func() {
		body.Objects = body.Objects[:0]
		m := presenter.Model()
		if m == nil || sourceIndex >= len(m.Sources) {
			body.Add(widget.NewLabel(locale.T("wizard.source.overview_no_meta")))
			body.Refresh()
			return
		}
		src := m.Sources[sourceIndex]

		// === Identity ===
		body.Add(sectionHeader(locale.T("wizard.source.overview_section_status")))
		typeLabel := locale.T("wizard.source.type_subscription_label")
		if src.Type == corestate.SourceTypeServer {
			typeLabel = locale.T("wizard.source.type_server_label")
		}
		body.Add(kvRow(locale.T("wizard.source.overview_field_type"), typeLabel))
		body.Add(kvRow(locale.T("wizard.source.overview_field_id"), src.ID))
		if src.URL != "" {
			body.Add(kvRow(locale.T("wizard.source.overview_field_url"), src.URL))
		}
		if src.URI != "" {
			body.Add(kvRow(locale.T("wizard.source.overview_field_uri"), src.URI))
		}
		if src.Label != "" {
			body.Add(kvRow(locale.T("wizard.source.overview_field_label"), src.Label))
		}
		body.Add(kvRow(locale.T("wizard.source.overview_field_enabled"), boolStr(src.Enabled)))
		if src.ExcludeFromGlobal {
			body.Add(kvRow(locale.T("wizard.source.overview_field_excluded"), "true"))
		}

		if src.Type == corestate.SourceTypeServer {
			body.Add(widget.NewSeparator())
			lbl := widget.NewLabel(locale.T("wizard.source.overview_server_no_meta"))
			lbl.Importance = widget.LowImportance
			lbl.Wrapping = fyne.TextWrapWord
			body.Add(lbl)
			body.Refresh()
			return
		}

		meta := src.Meta
		if meta == nil {
			body.Add(widget.NewSeparator())
			lbl := widget.NewLabel(locale.T("wizard.source.overview_no_meta"))
			lbl.Importance = widget.LowImportance
			lbl.Wrapping = fyne.TextWrapWord
			body.Add(lbl)
			body.Refresh()
			return
		}

		// === Status (fetch history) ===
		body.Add(kvRow(locale.T("wizard.source.overview_field_status"), formatStatusBadge(meta)))
		if meta.LastFetchedAt != "" {
			body.Add(kvRow(locale.T("wizard.source.overview_field_fetched"),
				fmt.Sprintf("%s (%s)", meta.LastFetchedAt, formatLastFetched(meta))))
		}
		if meta.HTTPStatusCode > 0 {
			body.Add(kvRow(locale.T("wizard.source.overview_field_http"), fmt.Sprintf("%d", meta.HTTPStatusCode)))
		}
		if meta.RawBodyBytes > 0 {
			body.Add(kvRow(locale.T("wizard.source.overview_field_size"), humanizeBytes(meta.RawBodyBytes)))
		}
		if meta.NodesCountFetched > 0 {
			body.Add(kvRow(locale.T("wizard.source.overview_field_nodes"), formatNodesCount(meta, 0)))
		}
		if meta.ErrorCount > 0 {
			body.Add(kvRow(locale.T("wizard.source.overview_field_errors"), fmt.Sprintf("%d", meta.ErrorCount)))
		}
		if meta.LastErrorMsg != "" {
			body.Add(kvRow(locale.T("wizard.source.overview_field_last_error"), meta.LastErrorMsg))
		}

		// === Headers ===
		hasHeaders := meta.ProfileTitle != "" || meta.ProfileUpdateIntervalHours > 0 ||
			meta.SupportURL != "" || meta.ProfileWebPageURL != "" || meta.ContentDispositionFilename != ""
		if hasHeaders {
			body.Add(widget.NewSeparator())
			body.Add(sectionHeader(locale.T("wizard.source.overview_section_headers")))
			if meta.ProfileTitle != "" {
				body.Add(kvRow(locale.T("wizard.source.overview_field_title"), meta.ProfileTitle))
			}
			if meta.ProfileUpdateIntervalHours > 0 {
				body.Add(kvRow(locale.T("wizard.source.overview_field_update_interval"),
					fmt.Sprintf("%dh", meta.ProfileUpdateIntervalHours)))
			}
			if meta.SupportURL != "" {
				body.Add(kvRow(locale.T("wizard.source.overview_field_support"), meta.SupportURL))
			}
			if meta.ProfileWebPageURL != "" {
				body.Add(kvRow(locale.T("wizard.source.overview_field_web"), meta.ProfileWebPageURL))
			}
			if meta.ContentDispositionFilename != "" {
				body.Add(kvRow(locale.T("wizard.source.overview_field_filename"), meta.ContentDispositionFilename))
			}
		}

		// === Quota ===
		if ui := meta.UserInfo; ui != nil && (ui.TotalBytes > 0 || ui.ExpireUnix > 0) {
			body.Add(widget.NewSeparator())
			body.Add(sectionHeader(locale.T("wizard.source.overview_section_quota")))
			if ui.TotalBytes > 0 {
				used := ui.UploadBytes + ui.DownloadBytes
				remaining := ui.TotalBytes - used
				if remaining < 0 {
					remaining = 0
				}
				body.Add(kvRow(locale.T("wizard.source.overview_field_used"), humanizeBytes(used)))
				body.Add(kvRow(locale.T("wizard.source.overview_field_total"), humanizeBytes(ui.TotalBytes)))
				body.Add(kvRow(locale.T("wizard.source.overview_field_remaining"), humanizeBytes(remaining)))
				if pct := quotaPercentage(meta); pct > 0 {
					bar := widget.NewProgressBar()
					bar.SetValue(pct)
					body.Add(bar)
				}
			}
			if ui.ExpireUnix > 0 {
				expireAt := time.Unix(ui.ExpireUnix, 0)
				body.Add(kvRow(locale.T("wizard.source.overview_field_expires"),
					fmt.Sprintf("%s (%s)", expireAt.Format("2006-01-02 15:04"), formatExpire(meta))))
			}
		}

		// === Raw body (slили из Raw tab) ===
		execDir := m.ExecDir
		if execDir != "" {
			subsDir := platform.GetSubscriptionsDir(execDir)
			if raw, rerr := v5.ReadRawBody(subsDir, src.ID); rerr == nil && len(raw) > 0 {
				body.Add(widget.NewSeparator())
				body.Add(sectionHeader(locale.T("wizard.source.raw_section_body")))

				// Раскодируем base64-обёрнутые subscription'ы (Liberty etc.) —
				// без этого MultiLineEntry с TextWrapOff уносит body за экран
				// одной длинной строкой. На plain-text bodies (BL/WL) с
				// #-comments DecodeSubscriptionContent — identity, без изменений.
				display := raw
				if decoded, derr := subscription.DecodeSubscriptionContent(raw); derr == nil && len(decoded) > 0 {
					display = decoded
				}

				if len(display) > rawBodyMaxDisplay {
					display = display[:rawBodyMaxDisplay]
					truncated := widget.NewLabel(locale.Tf("wizard.source.raw_body_truncated", rawBodyMaxDisplay, len(raw)))
					truncated.Importance = widget.LowImportance
					body.Add(truncated)
				}

				// MultiLineEntry без Disable() — на macOS Fyne disabled-text
				// рендерится цветом фона (невидимо). Оставляем editable
				// на ввод, но без OnChanged — мутации игнорятся.
				bodyEntry := widget.NewMultiLineEntry()
				bodyEntry.Wrapping = fyne.TextWrapOff
				bodyEntry.SetText(string(display))
				bodyEntry.OnChanged = func(s string) {
					if s != string(display) {
						bodyEntry.SetText(string(display))
					}
				}
				bodyEntryScroll := container.NewVScroll(container.NewMax(
					canvas.NewRectangle(transparentColor()),
					bodyEntry,
				))
				bodyEntryScroll.SetMinSize(fyne.NewSize(0, 240))
				body.Add(bodyEntryScroll)
			}
		}

		body.Refresh()
	}

	refresh()
	return rootWithGutter, refresh
}

// sectionHeader — bold-section-header label.
func sectionHeader(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.TextStyle = fyne.TextStyle{Bold: true}
	return l
}

// kvRow — label "Key: Value" с соответствующим стилем.
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

// guard: strings used somewhere
var _ = strings.Builder{}
