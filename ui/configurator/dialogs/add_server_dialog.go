// Package dialogs — диалоги визарда конфигурации.
//
// Файл add_server_dialog.go: форма ручного добавления источника. Две вкладки —
// «Параметры» (SOCKS5/HTTP формы либо Source: произвольный текст) и «JSON»
// (во что это превратится в config.json).
//
// Мотив форм тот же, что у мобильного Add Server Wizard в LxBox: у SOCKS5 и
// HTTP полей раз-два, и набрать их быстрее, чем вспоминать синтаксис share-URI
// — тем более что у HTTP-прокси схема нестандартная (proxy-http://, потому что
// голый http:// перехватывается как URL подписки).
//
// Формы НЕ создают узел сами: они собирают share-URI и отдают его в тот же
// onURI-колбэк, что и WARP-диалог, то есть в общий путь Add. Это осознанно —
// свой путь записи разошёлся бы с парсером при первом же изменении схемы
// (ловушка emitter-parser-pairing), а так вход проходит ровно те же стадии,
// что и вставленный руками.
//
// Вкладка JSON — не имитация: превью считается через config.EmitNodeJSONs, ту
// же точку эмиссии, что и реальная сборка (WYSIWYG, как во вкладке JSON окна
// Source). Правка JSON побеждает: тронул руками — поля больше не
// перезаписывают текст, и в конфиг уходит именно он.
package dialogs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/locale"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	addServerTagNoteText    = "Shown as the server title in the Sources list. If empty, the host or the link fragment is used."
	addServerTLSNoteText    = "Connect to the proxy over TLS (HTTPS proxy). Fine TLS settings — SNI, ALPN — can be adjusted later in the source editor."
	addServerSourceNoteText = "Paste anything: share-URI (one per line), a sing-box outbound or config JSON, or [Interface]/[Peer] WireGuard conf."
	addServerJSONHintText   = "Preview of what this source unpacks into. Edit it and your version wins — the fields above stop overwriting it."
	addServerJSONDirtyText  = "Edited by hand — the fields no longer overwrite this JSON."
)

// AddServerResult — что форма отдала наружу. Ровно одно из полей непусто.
type AddServerResult struct {
	// Text — вход для общего пути Add: share-URI из формы либо содержимое
	// вкладки Source (URI-список, JSON, INI — всё, что понимает Add).
	Text string
	// ConfigJSON — отредактированный вручную outbound. Заполняется только
	// когда человек правил вкладку JSON: тогда его версия и есть истина.
	ConfigJSON []byte
	// Label — тег из верхнего поля; для ConfigJSON становится Label источника.
	Label string
}

// ShowAddServerDialog открывает форму ручного добавления источника. onResult
// получает результат в главном потоке Fyne.
func ShowAddServerDialog(presenter *wizardpresentation.WizardPresenter, onResult func(AddServerResult)) {
	guiState := presenter.GUIState()
	if guiState == nil || guiState.Window == nil || onResult == nil {
		return
	}
	win := guiState.Window

	f := newAddServerForm()

	d := dialog.NewCustomConfirm(
		locale.T("Add server"),
		locale.T("Add"),
		locale.T("Cancel"),
		f.container,
		func(ok bool) {
			if !ok {
				return
			}
			res, err := f.result()
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			onResult(res)
		},
		win,
	)
	d.Resize(fyne.NewSize(640, 560))
	d.Show()
}

// addServerForm — состояние формы.
type addServerForm struct {
	container *fyne.Container

	tag *widget.Entry

	// Вкладка «Параметры».
	proto  *widget.RadioGroup
	host   *widget.Entry
	port   *widget.Entry
	user   *widget.Entry
	pass   *widget.Entry
	tls    *widget.Check
	tlsRow *fyne.Container
	// fieldsBox — блок полей SOCKS5/HTTP; прячется при выборе Source.
	fieldsBox *fyne.Container
	// source — многострочный ввод варианта Source.
	source    *widget.Entry
	sourceBox *fyne.Container

	// Вкладка «JSON».
	jsonView   *widget.Entry
	jsonStatus *widget.Label

	mode     addServerMode
	tlsOn    bool
	portEdit bool // порт правил человек — не перебивать его сменой протокола
	// jsonDirty — JSON тронут руками; поля больше его не перезаписывают.
	jsonDirty bool
	// syncing — идёт программная запись в jsonView, OnChanged не считать правкой.
	syncing bool
}

// addServerMode — что выбрано в «Параметрах».
type addServerMode int

const (
	modeSocks addServerMode = iota
	modeHTTP
	modeSource
)

func newAddServerForm() *addServerForm {
	f := &addServerForm{}

	// Тег — общий для всех вариантов, поэтому стоит над вкладками.
	f.tag = widget.NewEntry()
	f.tag.SetPlaceHolder(locale.T("optional"))
	f.tag.OnChanged = func(string) { f.refreshJSON() }
	tagNote := widget.NewLabel(locale.T(addServerTagNoteText))
	tagNote.Wrapping = fyne.TextWrapWord

	f.buildParamsTab()
	f.buildJSONTab()

	tabs := container.NewAppTabs(
		container.NewTabItem(locale.T("Parameters"), f.paramsContent()),
		container.NewTabItem(locale.T("JSON"), f.jsonContent()),
	)
	tabs.OnSelected = func(ti *container.TabItem) {
		if ti.Text == locale.T("JSON") {
			f.refreshJSON()
		}
	}

	f.container = container.NewBorder(
		container.NewVBox(
			labeledRow(locale.T("Tag"), f.tag),
			tagNote,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		tabs,
	)
	f.refreshJSON()
	return f
}

// buildParamsTab собирает виджеты вкладки «Параметры».
func (f *addServerForm) buildParamsTab() {
	socksLabel := locale.T("SOCKS5")
	httpLabel := locale.T("HTTP")
	sourceLabel := locale.T("Source")

	f.host = widget.NewEntry()
	f.host.SetText("127.0.0.1")
	f.host.OnChanged = func(string) { f.refreshJSON() }

	f.port = numEntry("1080")
	f.port.OnChanged = func(string) {
		f.portEdit = true
		f.refreshJSON()
	}

	f.user = widget.NewEntry()
	f.user.SetPlaceHolder(locale.T("optional"))
	f.user.OnChanged = func(string) { f.refreshJSON() }

	f.pass = widget.NewPasswordEntry()
	f.pass.SetPlaceHolder(locale.T("optional"))
	f.pass.OnChanged = func(string) { f.refreshJSON() }

	f.tls = widget.NewCheck(locale.T("HTTPS (TLS to proxy)"), nil)
	tlsNote := widget.NewLabel(locale.T(addServerTLSNoteText))
	tlsNote.Wrapping = fyne.TextWrapWord
	f.tlsRow = container.NewVBox(f.tls, tlsNote)
	f.tlsRow.Hide()
	f.tls.OnChanged = func(on bool) {
		f.tlsOn = on
		f.applyDefaultPort()
		f.refreshJSON()
	}

	f.fieldsBox = container.NewVBox(
		labeledRow(locale.T("Host"), f.host),
		labeledRow(locale.T("Port"), f.port),
		labeledRow(locale.T("Username"), f.user),
		labeledRow(locale.T("Password"), f.pass),
		f.tlsRow,
	)

	// Source: многострочный ввод чего угодно, что понимает общий путь Add.
	f.source = widget.NewMultiLineEntry()
	f.source.Wrapping = fyne.TextWrapOff
	f.source.SetPlaceHolder("vless://…\n{\"type\":\"vless\",…}\n[Interface]…")
	f.source.OnChanged = func(string) { f.refreshJSON() }
	sourceNote := widget.NewLabel(locale.T(addServerSourceNoteText))
	sourceNote.Wrapping = fyne.TextWrapWord
	f.sourceBox = container.NewBorder(
		nil, sourceNote, nil, nil,
		container.NewScroll(f.source),
	)
	f.sourceBox.Hide()

	f.proto = widget.NewRadioGroup([]string{socksLabel, httpLabel, sourceLabel}, nil)
	f.proto.Horizontal = true
	f.proto.SetSelected(socksLabel)
	f.proto.OnChanged = func(sel string) {
		switch sel {
		case httpLabel:
			f.mode = modeHTTP
		case sourceLabel:
			f.mode = modeSource
		default:
			f.mode = modeSocks
		}
		f.applyModeVisibility()
		f.applyDefaultPort()
		f.refreshJSON()
	}
}

// applyModeVisibility показывает блок, отвечающий выбранному варианту.
func (f *addServerForm) applyModeVisibility() {
	if f.mode == modeSource {
		f.fieldsBox.Hide()
		f.sourceBox.Show()
		return
	}
	f.sourceBox.Hide()
	f.fieldsBox.Show()
	if f.mode == modeHTTP {
		f.tlsRow.Show()
	} else {
		f.tlsRow.Hide()
	}
}

func (f *addServerForm) paramsContent() fyne.CanvasObject {
	return container.NewBorder(
		container.NewVBox(f.proto, widget.NewSeparator()),
		nil, nil, nil,
		container.NewVBox(f.fieldsBox, f.sourceBox),
	)
}

// buildJSONTab собирает виджеты вкладки «JSON».
func (f *addServerForm) buildJSONTab() {
	f.jsonView = widget.NewMultiLineEntry()
	f.jsonView.Wrapping = fyne.TextWrapOff
	f.jsonView.OnChanged = func(string) {
		// Программная синхронизация — не правка человека.
		if f.syncing {
			return
		}
		if !f.jsonDirty {
			f.jsonDirty = true
			f.jsonStatus.SetText(locale.T(addServerJSONDirtyText))
		}
	}
	f.jsonStatus = widget.NewLabel(locale.T(addServerJSONHintText))
	f.jsonStatus.Wrapping = fyne.TextWrapWord
}

func (f *addServerForm) jsonContent() fyne.CanvasObject {
	// Кнопка возврата к автогенерации: без неё ручная правка — дорога в один
	// конец, и человеку пришлось бы закрывать диалог, чтобы начать заново.
	reset := widget.NewButton(locale.T("Rebuild from fields"), func() {
		f.jsonDirty = false
		f.refreshJSON()
	})
	return container.NewBorder(
		nil,
		container.NewVBox(f.jsonStatus, reset),
		nil, nil,
		container.NewScroll(f.jsonView),
	)
}

// defaultPort — порт по умолчанию для текущей комбинации протокол+TLS.
func (f *addServerForm) defaultPort() string {
	if f.mode != modeHTTP {
		return "1080"
	}
	if f.tlsOn {
		return "443"
	}
	return "8080"
}

// applyDefaultPort подставляет дефолтный порт, пока человек не ввёл свой:
// молча затирать набранное значение сменой радиокнопки — худший вид сюрприза.
func (f *addServerForm) applyDefaultPort() {
	if f.portEdit && strings.TrimSpace(f.port.Text) != "" && !f.portIsSomeDefault() {
		return
	}
	f.port.SetText(f.defaultPort())
	f.portEdit = false
}

// portIsSomeDefault — в поле стоит один из наших дефолтов, а не ручной ввод.
func (f *addServerForm) portIsSomeDefault() bool {
	switch strings.TrimSpace(f.port.Text) {
	case "1080", "8080", "443":
		return true
	}
	return false
}

// refreshJSON пересчитывает превью. Ручную правку не трогает — она победила.
func (f *addServerForm) refreshJSON() {
	if f.jsonDirty || f.jsonView == nil {
		return
	}
	text, status := f.previewJSON()
	f.syncing = true
	f.jsonView.SetText(text)
	f.syncing = false
	f.jsonStatus.SetText(status)
}

// previewJSON строит превью через ту же эмиссию, что и реальная сборка.
func (f *addServerForm) previewJSON() (string, string) {
	input, err := f.rawInput()
	if err != nil {
		return "", err.Error()
	}
	if strings.TrimSpace(input) == "" {
		return "", locale.T(addServerJSONHintText)
	}

	nodes := parseAddServerInput(input)
	if len(nodes) == 0 {
		return "", locale.T("Nothing recognized yet.")
	}

	docs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		outs, ep, eerr := config.EmitNodeJSONs(n)
		if eerr != nil {
			continue
		}
		if ep != "" {
			docs = append(docs, indentJSON(stripEmitted(ep)))
			continue
		}
		for _, o := range outs {
			docs = append(docs, indentJSON(stripEmitted(o)))
		}
	}
	if len(docs) == 0 {
		return "", locale.T("Nothing recognized yet.")
	}
	return strings.Join(docs, ",\n"), locale.Tf("Unpacked nodes: %d", len(docs))
}

// parseAddServerInput разбирает вход превью: сначала как sing-box JSON, потом
// построчно как share-URI. Ошибки глотаются — это превью, оно обновляется на
// каждый символ и не должно кричать на недонабранную строку.
func parseAddServerInput(input string) []*config.ParsedNode {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil
	}

	if trimmed[0] == '{' || trimmed[0] == '[' {
		kind := subscription.ClassifySubscriptionBody(trimmed)
		switch kind {
		case subscription.BodyKindSingboxOutbound:
			if n, err := subscription.NodeFromManualConfigJSON([]byte(trimmed)); err == nil {
				return []*config.ParsedNode{n}
			}
			return nil
		case subscription.BodyKindSingboxOutboundArray,
			subscription.BodyKindSingboxConfig,
			subscription.BodyKindSingboxConfigArray:
			if res, err := subscription.ParseSingboxBody(trimmed, kind, nil); err == nil {
				return res.Nodes
			}
			return nil
		}
	}

	// WG-conf и share-URI: тот же порядок, что у общего пути Add.
	rest, blocks := subscription.ExtractWGConfBlocks(input)
	nodes := make([]*config.ParsedNode, 0, 4)
	for _, b := range blocks {
		uri, err := subscription.ConvertWGConfText(b)
		if err != nil {
			continue
		}
		if n, err := subscription.ParseNode(uri, nil); err == nil {
			nodes = append(nodes, n)
		}
	}
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !subscription.IsDirectLink(line) {
			continue
		}
		if n, err := subscription.ParseNode(line, nil); err == nil {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// rawInput — вход для текущего варианта: собранный URI либо текст Source.
func (f *addServerForm) rawInput() (string, error) {
	if f.mode == modeSource {
		return f.source.Text, nil
	}
	return f.buildURI()
}

// result — что уходит наружу по кнопке Add.
func (f *addServerForm) result() (AddServerResult, error) {
	label := strings.TrimSpace(f.tag.Text)

	// Ручная правка JSON побеждает: в конфиг уходит она, а не поля.
	if f.jsonDirty {
		return manualJSONResult(f.jsonView.Text, label)
	}

	if f.mode == modeSource {
		text := strings.TrimSpace(f.source.Text)
		if text == "" {
			return AddServerResult{}, fmt.Errorf("%s", locale.T("Input is empty"))
		}
		return AddServerResult{Text: text, Label: label}, nil
	}

	uri, err := f.buildURI()
	if err != nil {
		return AddServerResult{}, err
	}
	return AddServerResult{Text: uri, Label: label}, nil
}

// manualJSONResult решает, чем стал отредактированный вручную JSON.
//
// Одиночный объект уходит как ConfigJSON — так он сохраняется побайтово,
// включая поля, которых наш парсер не знает. Всё прочее (массив outbound'ов,
// целый конфиг) отдаётся общему пути Add: он это уже умеет.
func manualJSONResult(raw, label string) (AddServerResult, error) {
	body := strings.TrimSpace(raw)
	if body == "" {
		return AddServerResult{}, fmt.Errorf("%s", locale.T("JSON is empty"))
	}
	if strings.HasPrefix(body, "{") {
		kind := subscription.ClassifySubscriptionBody(body)
		// Целый конфиг ({"outbounds":[…]}) — законная многоузловая форма,
		// её разберёт общий путь Add. А одиночный объект, не признанный
		// outbound'ом, это чаще всего забытый "type": отвергаем здесь, иначе
		// он упадёт ниже по течению с куда менее внятным сообщением.
		if kind != subscription.BodyKindSingboxConfig {
			if _, err := subscription.NodeFromManualConfigJSON([]byte(body)); err != nil {
				return AddServerResult{}, err
			}
			var buf bytes.Buffer
			if err := json.Compact(&buf, []byte(body)); err != nil {
				return AddServerResult{}, err
			}
			return AddServerResult{ConfigJSON: buf.Bytes(), Label: label}, nil
		}
	}
	return AddServerResult{Text: body, Label: label}, nil
}

// buildURI собирает share-URI из полей формы.
func (f *addServerForm) buildURI() (string, error) {
	return buildProxyURI(proxyURIInput{
		Mode: f.mode,
		TLS:  f.tlsOn,
		Host: f.host.Text,
		Port: f.port.Text,
		User: f.user.Text,
		Pass: f.pass.Text,
		Tag:  f.tag.Text,
	})
}

// proxyURIInput — вход сборки URI, отвязанный от виджетов: логика схемы и
// валидации тестируется без запуска Fyne.
type proxyURIInput struct {
	Mode addServerMode
	TLS  bool
	Host string
	Port string
	User string
	Pass string
	Tag  string
}

// buildProxyURI собирает share-URI. Ошибки — человеческие: пустой хост и
// негодный порт ловятся здесь, а не в глубине парсера.
func buildProxyURI(in proxyURIInput) (string, error) {
	host := strings.TrimSpace(in.Host)
	if host == "" {
		return "", fmt.Errorf("%s", locale.T("Host required"))
	}

	port, err := strconv.Atoi(strings.TrimSpace(in.Port))
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("%s", locale.T("Port 1..65535"))
	}

	user := strings.TrimSpace(in.User)
	pass := in.Pass // пароль не триммим: пробелы в нём легальны

	scheme := "socks5"
	if in.Mode == modeHTTP {
		scheme = "proxy-http"
		if in.TLS {
			scheme = "proxy-https"
		}
	}

	var userinfo *url.Userinfo
	if user != "" || pass != "" {
		userinfo = url.UserPassword(user, pass)
	}

	u := &url.URL{
		Scheme:   scheme,
		User:     userinfo,
		Host:     joinHostPort(host, port),
		Fragment: strings.TrimSpace(in.Tag),
	}
	return u.String(), nil
}

// joinHostPort — как net.JoinHostPort, но IPv6 берётся в скобки только если
// их ещё нет (человек мог ввести адрес уже в скобках).
func joinHostPort(host string, port int) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return fmt.Sprintf("[%s]:%d", host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// stripEmitted снимает с эмитированной строки то, что делает её фрагментом
// конфига: строки-комментарии, ведущие табы и хвостовую запятую.
func stripEmitted(s string) string {
	lines := strings.Split(s, "\n")
	kept := lines[:0]
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "//") {
			continue
		}
		kept = append(kept, ln)
	}
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	return strings.TrimSuffix(out, ",")
}

// indentJSON приводит фрагмент к pretty-виду, сохраняя порядок полей эмиттера
// (json.Indent работает на токенах, в отличие от Unmarshal→MarshalIndent).
func indentJSON(s string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(s), "", "  "); err != nil {
		return s
	}
	return buf.String()
}
