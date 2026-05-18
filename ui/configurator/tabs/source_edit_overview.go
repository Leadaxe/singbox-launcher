package tabs

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core/config/subscription"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
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
		t0 := time.Now()
		defer func() {
			debuglog.DebugLog("buildOverviewTab: refresh took %v", time.Since(t0))
		}()
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
			rawPath := filepath.Join(subsDir, src.ID+".raw")
			// Partial read: вместо `os.ReadFile` всего файла (~1 MB для Xray JSON)
			// + DecodeSubscriptionContent на нём (`json.Valid` + `Unmarshal` —
			// ~100ms на MB) читаем только prefix `rawBodyMaxDisplay + 1`. Это:
			//  - быстро (< 1ms даже на медленном диске);
			//  - достаточно для рендера в Entry;
			//  - +1 байт позволяет понять truncated (если файл больше).
			// totalSize берём через os.Stat — для отображения "of N bytes".
			tRead := time.Now()
			display, totalSize, ok := readRawBodyPartial(rawPath, rawBodyMaxDisplay+1)
			debuglog.DebugLog("buildOverviewTab: readRawBodyPartial took %v (size=%d, ok=%v)", time.Since(tRead), len(display), ok)
			if ok && len(display) > 0 {
				body.Add(widget.NewSeparator())

				// DecodeSubscriptionContent ТОЛЬКО если хвост base64 — попытка
				// декодировать обрезанную base64 даст garbage. Для JSON / plain
				// URI body показываем как есть.
				// Эвристика: если первый non-whitespace == '[' или содержит '://' —
				// уже decoded, не трогаем. Иначе пробуем base64 (только prefix).
				trimmed := strings.TrimLeft(string(display), " \t\n\r")
				looksDecoded := strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") ||
					strings.Contains(trimmed[:minInt(len(trimmed), 1024)], "://")
				if !looksDecoded {
					if decoded, derr := subscription.DecodeSubscriptionContent(display); derr == nil && len(decoded) > 0 {
						display = decoded
					}
				}

				truncatedNote := ""
				if totalSize > rawBodyMaxDisplay {
					if len(display) > rawBodyMaxDisplay {
						display = display[:rawBodyMaxDisplay]
					}
					truncatedNote = locale.Tf("wizard.source.raw_body_truncated", rawBodyMaxDisplay, totalSize)
				}

				// Header: title + icon-кнопки сразу справа от него (inline HBox).
				// Кнопки показываем всегда — путь полезен и когда body не truncated
				// (юзер может захотеть открыть в внешнем editor'е).
				// ttwidget.NewButtonWithIcon — поддерживает SetToolTip (обычный
				// widget.Button его не поддерживает, поэтому setTooltip был no-op).
				openBtn := ttwidget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
					openInFileManager(subsDir)
				})
				openBtn.Importance = widget.LowImportance
				openBtn.SetToolTip(locale.T("wizard.source.raw_open_folder") + "\n" + subsDir)
				copyBtn := ttwidget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
					if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
						app.Clipboard().SetContent(rawPath)
					}
				})
				copyBtn.Importance = widget.LowImportance
				copyBtn.SetToolTip(locale.T("wizard.source.raw_copy_path") + "\n" + rawPath)
				headerRow := container.NewHBox(
					sectionHeader(locale.T("wizard.source.raw_section_body")),
					openBtn, copyBtn,
				)
				body.Add(headerRow)

				if truncatedNote != "" {
					tr := widget.NewLabel(truncatedNote)
					tr.Importance = widget.LowImportance
					body.Add(tr)
				}

				// MultiLineEntry без Disable() — на macOS Fyne disabled-text
				// рендерится цветом фона (невидимо). Оставляем editable
				// на ввод, но без OnChanged — мутации игнорятся.
				//
				// TextWrapBreak (не Off): compact JSON подписки (типа Xray)
				// идут одной длинной строкой без переводов, без wrap'а уходят
				// далеко вправо за viewport — юзер видит чёрное пустое поле.
				// Break wrap'ает по любому символу (JSON без пробелов
				// нормально не break'ается по слову).
				// Pre-wrap: компактный JSON / base64 одной строкой Fyne wrap'ает
				// посимвольно (TextWrapBreak без виртуализации — 9+ сек на 4 KB).
				// Вставляем \n каждые wrapEvery символов вручную и снимаем
				// Fyne-wrap (TextWrapOff) — мгновенно.
				displayStr := wrapLongLines(string(display), 100)
				tEntry := time.Now()
				bodyEntry := widget.NewMultiLineEntry()
				bodyEntry.Wrapping = fyne.TextWrapOff
				bodyEntry.SetText(displayStr)
				debuglog.DebugLog("buildOverviewTab: bodyEntry.SetText(%d bytes, pre-wrapped %d lines) took %v", len(displayStr), strings.Count(displayStr, "\n")+1, time.Since(tEntry))
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

	// Lazy: НЕ вызываем refresh() здесь. Overview по дефолту неактивный таб
	// (Settings — первый в NewAppTabs), а refresh() тянет ReadRawBody +
	// DecodeSubscriptionContent для подписки с 1 MB Xray JSON body — это
	// ~10 сек на открытии окна. Refresh вызывается из tabs.OnSelected когда
	// юзер реально кликает Overview. До этого таб показывает пустой VBox.
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

// readRawBodyPartial читает первые `maxBytes` байт файла + возвращает total
// size через os.Stat. Не тащит всю мегабайтную body в память.
// Возвращает (prefix, totalSize, true) на успех; (nil, 0, false) если файл
// не существует / нечитаем.
func readRawBodyPartial(path string, maxBytes int) ([]byte, int, bool) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, 0, false
	}
	total := int(stat.Size())
	if total == 0 {
		return nil, 0, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, total, false
	}
	defer f.Close()
	readN := maxBytes
	if readN > total {
		readN = total
	}
	buf := make([]byte, readN)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, total, false
	}
	return buf[:n], total, true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// wrapLongLines вставляет '\n' каждые `every` символов в строки, длиннее
// этого порога. Строки с уже-имеющимися переводами оставляет как есть.
//
// Цель: убрать стоимость Fyne text-wrap (TextWrapBreak без виртуализации) —
// сами заранее разбиваем длинные строки на короткие. SetText на pre-wrapped
// тексте с TextWrapOff летает.
//
// Используется для компактных JSON / base64 raw bodies подписок.
func wrapLongLines(s string, every int) string {
	if every <= 0 || len(s) < every {
		return s
	}
	// Если уже есть переводы строк и средняя строка короче порога — не трогаем.
	if nl := strings.Count(s, "\n"); nl > 0 && len(s)/(nl+1) < every {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(s)/every + 16)
	for _, line := range strings.SplitAfter(s, "\n") {
		if len(line) <= every {
			b.WriteString(line)
			continue
		}
		// Разбиваем длинную строку (без переводов) на куски.
		for i := 0; i < len(line); i += every {
			end := i + every
			if end > len(line) {
				end = len(line)
			}
			b.WriteString(line[i:end])
			// Не дублируем \n если он уже на конце последнего куска.
			if end < len(line) || (end == len(line) && !strings.HasSuffix(line, "\n")) {
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

// openInFileManager открывает path в системном file-manager'е (Finder/Explorer/xdg-open).
// Best-effort: ошибки игнорируются (logged debuglog'ом нет смысла).
func openInFileManager(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("explorer", path)
	default: // linux, *bsd
		cmd = exec.Command("xdg-open", path)
	}
	_ = cmd.Start()
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// guard: strings used somewhere
var _ = strings.Builder{}
