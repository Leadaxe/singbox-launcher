package tabs

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/state"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// rawBodyMaxDisplay — лимит на отображение body в textarea (4 KB).
//
// Fyne widget.MultiLineEntry рендерит каждый символ через text-layout
// без виртуализации (см. https://github.com/fyne-io/fyne/issues/2935):
// уже на 64 KB Xray JSON (вложенные объекты, длинные строки) на macOS
// заметно лагает. 4 KB — это «инспектор первых нескольких нод» без
// freeze'ов.
//
// Покрытие:
//   - base64-decoded подписки на 10-15 URI строк (~4 KB) — целиком
//   - Xray JSON pretty-printed: первые 1-2 ноды (выглядит как структура,
//     но юзер быстро увидит truncated-label куда идти за полным)
//
// Для больших bodies полный raw доступен через filesystem path
// `bin/subscriptions/<id>.raw` — показываем подсказку в truncated label.
const rawBodyMaxDisplay = 4 * 1024

// buildRawTab — read-only HTTP response inspector для подписок:
//   - meta summary (status, fetched, size);
//   - parsed headers (профильный title, userinfo, support_url, ...);
//   - raw body content из bin/subscriptions/<id>.raw.
//
// Для server-source — заглушка: raw view не применима.
func buildRawTab(presenter *wizardpresentation.WizardPresenter, sourceIndex int) (fyne.CanvasObject, func()) {
	body := container.NewVBox()
	scroll := container.NewVScroll(body)
	scroll.SetMinSize(fyne.NewSize(0, sourceEditJSONScrollMinH))

	bodyEntry := widget.NewMultiLineEntry()
	bodyEntry.Wrapping = fyne.TextWrapOff
	bodyScroll := container.NewVScroll(container.NewMax(
		canvas.NewRectangle(transparentColor()),
		bodyEntry,
	))
	bodyScroll.SetMinSize(fyne.NewSize(0, 240))

	refresh := func() {
		body.Objects = body.Objects[:0]
		m := presenter.Model()
		if m == nil || sourceIndex >= len(m.Sources) {
			body.Add(widget.NewLabel(locale.T("wizard.source.raw_no_body")))
			body.Refresh()
			return
		}
		src := m.Sources[sourceIndex]
		if src.Type == corestate.SourceTypeServer {
			lbl := widget.NewLabel(locale.T("wizard.source.raw_server_unsupported"))
			lbl.Importance = widget.LowImportance
			lbl.Wrapping = fyne.TextWrapWord
			body.Add(lbl)
			body.Refresh()
			return
		}

		// Summary section
		body.Add(sectionHeader(locale.T("wizard.source.raw_section_summary")))
		if src.Meta != nil {
			body.Add(kvRow(locale.T("wizard.source.overview_field_status"), formatStatusBadge(src.Meta)))
			if src.Meta.LastFetchedAt != "" {
				body.Add(kvRow(locale.T("wizard.source.overview_field_fetched"), src.Meta.LastFetchedAt))
			}
			if src.Meta.HTTPStatusCode > 0 {
				body.Add(kvRow(locale.T("wizard.source.overview_field_http"), fmt.Sprintf("%d", src.Meta.HTTPStatusCode)))
			}
			if src.Meta.RawBodyBytes > 0 {
				body.Add(kvRow(locale.T("wizard.source.overview_field_size"), humanizeBytes(src.Meta.RawBodyBytes)))
			}
			if src.Meta.LastErrorMsg != "" {
				body.Add(kvRow(locale.T("wizard.source.overview_field_last_error"), src.Meta.LastErrorMsg))
			}
		}

		// Parsed headers section
		if src.Meta != nil {
			hasHeaders := src.Meta.ProfileTitle != "" || src.Meta.SupportURL != "" ||
				src.Meta.ProfileWebPageURL != "" || src.Meta.ContentDispositionFilename != "" ||
				src.Meta.UserInfo != nil
			if hasHeaders {
				body.Add(widget.NewSeparator())
				body.Add(sectionHeader(locale.T("wizard.source.raw_section_headers")))
				if src.Meta.ProfileTitle != "" {
					body.Add(kvRow("profile-title", src.Meta.ProfileTitle))
				}
				if src.Meta.ProfileUpdateIntervalHours > 0 {
					body.Add(kvRow("profile-update-interval", fmt.Sprintf("%d", src.Meta.ProfileUpdateIntervalHours)))
				}
				if src.Meta.SupportURL != "" {
					body.Add(kvRow("support-url", src.Meta.SupportURL))
				}
				if src.Meta.ProfileWebPageURL != "" {
					body.Add(kvRow("profile-web-page-url", src.Meta.ProfileWebPageURL))
				}
				if src.Meta.ContentDispositionFilename != "" {
					body.Add(kvRow("content-disposition (filename)", src.Meta.ContentDispositionFilename))
				}
				if ui := src.Meta.UserInfo; ui != nil {
					body.Add(kvRow("subscription-userinfo (upload)", humanizeBytes(ui.UploadBytes)))
					body.Add(kvRow("subscription-userinfo (download)", humanizeBytes(ui.DownloadBytes)))
					body.Add(kvRow("subscription-userinfo (total)", humanizeBytes(ui.TotalBytes)))
					if ui.ExpireUnix > 0 {
						body.Add(kvRow("subscription-userinfo (expire)",
							fmt.Sprintf("unix %d", ui.ExpireUnix)))
					}
				}
			}
		}

		// Raw body section
		body.Add(widget.NewSeparator())
		body.Add(sectionHeader(locale.T("wizard.source.raw_section_body")))

		execDir := m.ExecDir
		if execDir == "" {
			lbl := widget.NewLabel(locale.T("wizard.source.raw_no_body"))
			lbl.Importance = widget.LowImportance
			body.Add(lbl)
			body.Refresh()
			return
		}
		subsDir := platform.GetSubscriptionsDir(execDir)
		raw, rerr := state.ReadRawBody(subsDir, src.ID)
		if rerr != nil {
			lbl := widget.NewLabel(locale.T("wizard.source.raw_no_body"))
			lbl.Importance = widget.LowImportance
			body.Add(lbl)
			body.Refresh()
			return
		}

		display := raw
		if len(raw) > rawBodyMaxDisplay {
			display = raw[:rawBodyMaxDisplay]
			truncated := widget.NewLabel(locale.Tf("wizard.source.raw_body_truncated", rawBodyMaxDisplay, len(raw)))
			truncated.Importance = widget.LowImportance
			body.Add(truncated)
		}
		bodyEntry.SetText(string(display))
		body.Add(bodyScroll)
		body.Refresh()
	}

	refresh()
	return scroll, refresh
}
