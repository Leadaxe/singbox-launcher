// File source_edit_raw_body.go — «сырой ответ подписки» по требованию и
// строка счёта состава для Overview (SPEC 116 W11).
//
// # Почему скачивание, а не показ сохранённого
//
// Тела подписки в состоянии нет и не будет: SPEC 118 снёс raw-кэш вместе с
// повторным разбором, узлы материализованы поузлово. Значит показать «что
// прислал провайдер» можно только одним честным способом — спросить его
// снова. Это диагностика, а не обновление: скачанное НИКУДА не пишется,
// nodes[] не трогаются, merge не зовётся. Пользователь смотрит байты и
// закрывает окно.
//
// # Почему галки декодирования, а не автоматика
//
// Провайдеры отдают тело в разных обёртках (base64 поверх URI-списка,
// percent-encoding внутри строк, минифицированный JSON), и «умный» показ,
// решающий за пользователя, ровно в спорном случае и врёт. Здесь показ
// буквальный, а каждое преобразование — отдельная галка, которую видно.
package tabs

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config/subscription"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
)

// sourceNodesHeader — строка счёта состава.
//
// Неразобранные записи (SPEC 116 W11) названы ОТДЕЛЬНЫМ слагаемым:
// «Nodes: 38 + 5 unsupported». Сложить их в одно число нельзя — 43 узла
// пользователь искал бы в конфиге, а их там 38.
func sourceNodesHeader(nodes []corestate.Node) string {
	supported, unsupported := 0, 0
	for i := range nodes {
		if nodes[i].IsUnsupported() {
			unsupported++
			continue
		}
		supported++
	}
	head := locale.Tf("Nodes: %d", supported)
	if unsupported > 0 {
		head += locale.Tf(" + %d unsupported", unsupported)
	}
	return head
}

// appendRawBodySection добавляет блок «скачать и показать сырой ответ».
//
// url пуст (не подписка / URL не задан) — блока нет: кнопка, которой некуда
// ходить, обещала бы несуществующее.
// userAgent — UA ИСТОЧНИКА: провайдеры ветвят выдачу по нему, и без него
// диагностика показывала бы не то тело, которое приедет на fetch.
func appendRawBodySection(body *fyne.Container, subURL string, id subscription.SourceIdentity) {
	subURL = strings.TrimSpace(subURL)
	if subURL == "" {
		return
	}

	body.Add(widget.NewSeparator())
	body.Add(sectionHeader(locale.T("Raw response")))

	status := widget.NewLabel(locale.T("Not fetched — press Reload to download the body as it is now."))
	status.Wrapping = fyne.TextWrapWord
	status.Importance = widget.LowImportance

	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapOff
	// Read-only тем же приёмом, что и прочие показы Overview: Disable() на
	// macOS красит текст цветом фона.
	shown := ""
	setShown := func(s string) {
		shown = s
		entry.SetText(s)
	}
	entry.OnChanged = func(s string) {
		if s != shown {
			entry.SetText(shown)
		}
	}
	entryScroll := container.NewVScroll(container.NewStack(
		canvas.NewRectangle(transparentColor()),
		entry,
	))
	entryScroll.SetMinSize(fyne.NewSize(0, 240))

	// raw — то, что реально приехало; вид пересобирается из него при каждой
	// смене галок, а не накапливается поверх предыдущего вида.
	var raw []byte
	var urldecode, unbase64, pretty *widget.Check
	render := func() {
		if raw == nil {
			return
		}
		setShown(renderRawBodyView(raw, unbase64.Checked, urldecode.Checked, pretty.Checked))
	}
	onToggle := func(bool) { render() }
	unbase64 = widget.NewCheck(locale.T("base64"), onToggle)
	urldecode = widget.NewCheck(locale.T("urldecode"), onToggle)
	pretty = widget.NewCheck(locale.T("pretty-print"), onToggle)

	copyBtn := widget.NewButton(locale.T("Copy"), func() {
		fynewidget.SetClipboard(shown)
	})
	copyBtn.Disable()

	var reloadBtn *widget.Button
	reloadBtn = widget.NewButton(locale.T("Reload"), func() {
		reloadBtn.Disable()
		status.SetText(locale.T("Loading..."))
		go func() {
			res, err := subscription.FetchSubscriptionWithMetaFor(subURL, id)
			fyne.Do(func() {
				reloadBtn.Enable()
				if err != nil {
					// Ошибка показывается здесь и НИКУДА не пишется: это
					// разовый запрос диагностики, а счётчики ошибок подписки
					// ведёт fetch-конвейер, и подмешивать в них ручной
					// просмотр значило бы портить её историю.
					status.SetText(locale.Tf("Fetch failed: %s", err.Error()))
					return
				}
				// RawBody — байты ДО декодирования: «как есть» это именно они.
				raw = res.RawBody
				if len(raw) == 0 {
					raw = res.Body
				}
				status.SetText(locale.Tf("HTTP %d · %s", res.HTTPStatus, humanizeBytes(int64(len(raw)))))
				copyBtn.Enable()
				render()
			})
		}()
	})

	body.Add(container.NewHBox(reloadBtn, unbase64, urldecode, pretty, copyBtn))
	body.Add(status)
	body.Add(entryScroll)
}

// renderRawBodyView — как показать скачанные байты.
//
// Порядок преобразований — тот же, в котором обёртки накладывались
// провайдером: сначала снять base64 (внешняя обёртка тела), потом
// percent-encoding (живёт уже внутри строк), и только потом форматировать
// JSON. Неудача любого шага — не ошибка показа: он просто не применяется, а
// байты остаются те же. Так галка, поставленная не к тому телу, ничего не
// портит.
func renderRawBodyView(raw []byte, unbase64, urldecode, pretty bool) string {
	out := raw
	if unbase64 {
		if dec, err := subscription.DecodeSubscriptionContent(out); err == nil && len(dec) > 0 {
			out = dec
		}
	}
	text := string(out)
	if urldecode {
		if dec, err := url.QueryUnescape(text); err == nil {
			text = dec
		}
	}
	if pretty {
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(text), "", "  "); err == nil {
			text = buf.String()
		} else {
			// Не JSON — форматировать нечего; разбиваем URI-список по строкам
			// на случай тела, приехавшего одной длинной строкой.
			text = strings.ReplaceAll(text, "\r\n", "\n")
		}
	}
	return text
}
