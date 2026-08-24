// Package dialogs — диалоги визарда конфигурации.
//
// Файл add_server_dialog.go: форма ручного добавления прокси-сервера (SOCKS5 и
// HTTP/HTTPS). Мотив тот же, что у мобильного Add Server Wizard в LxBox: у этих
// двух схем полей раз-два, и набрать их в форме быстрее, чем вспоминать синтаксис
// share-URI — тем более что у HTTP-прокси схема нестандартная (proxy-http://,
// потому что голый http:// перехватывается как URL подписки).
//
// Форма НЕ создаёт узел сама: она собирает share-URI и отдаёт его в тот же
// onURI-колбэк, что и WARP-диалог, то есть в общий путь Add. Это осознанно —
// свой путь записи разошёлся бы с парсером при первом же изменении схемы
// (ловушка emitter-parser-pairing), а так URI проходит ровно те же стадии,
// что и вставленный руками.
package dialogs

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	addServerTagNoteText = "Shown as the server title in the Sources list. If empty, the host is used."
	addServerTLSNoteText = "Connect to the proxy over TLS (HTTPS proxy). Fine TLS settings — SNI, ALPN — can be adjusted later in the source editor."
)

// ShowAddServerDialog открывает форму ручного добавления сервера. onURI
// получает готовый share-URI в главном потоке Fyne — обычно applyAddedSources.
func ShowAddServerDialog(presenter *wizardpresentation.WizardPresenter, onURI func(string)) {
	guiState := presenter.GUIState()
	if guiState == nil || guiState.Window == nil || onURI == nil {
		return
	}
	win := guiState.Window

	form := newAddServerForm()

	d := dialog.NewCustomConfirm(
		locale.T("Add server"),
		locale.T("Add"),
		locale.T("Cancel"),
		form.container,
		func(ok bool) {
			if !ok {
				return
			}
			uri, err := form.buildURI()
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			onURI(uri)
		},
		win,
	)
	d.Resize(fyne.NewSize(520, 420))
	d.Show()
}

// addServerForm — состояние формы: общий набор полей на обе схемы плюс
// TLS-переключатель, осмысленный только для HTTP.
type addServerForm struct {
	container *fyne.Container

	proto    *widget.RadioGroup
	tag      *widget.Entry
	host     *widget.Entry
	port     *widget.Entry
	user     *widget.Entry
	pass     *widget.Entry
	tls      *widget.Check
	tlsRow   *fyne.Container
	portEdit bool // порт правил человек — не перебивать его сменой протокола
	http     bool // выбран HTTP; флаг, а не сравнение локализованных строк
}

// Подписи протоколов вынесены в переменные: RadioGroup сравнивает выбранное
// значение со строкой, и вычислять locale.T дважды — верный способ разойтись.
func newAddServerForm() *addServerForm {
	f := &addServerForm{}

	socksLabel := locale.T("SOCKS5")
	httpLabel := locale.T("HTTP")

	f.tag = widget.NewEntry()
	f.tag.SetPlaceHolder(locale.T("optional"))

	f.host = widget.NewEntry()
	f.host.SetText("127.0.0.1")

	f.port = numEntry("1080")
	f.port.OnChanged = func(string) { f.portEdit = true }

	f.user = widget.NewEntry()
	f.user.SetPlaceHolder(locale.T("optional"))

	f.pass = widget.NewPasswordEntry()
	f.pass.SetPlaceHolder(locale.T("optional"))

	f.tls = widget.NewCheck(locale.T("HTTPS (TLS to proxy)"), nil)
	tlsNote := widget.NewLabel(locale.T(addServerTLSNoteText))
	tlsNote.Wrapping = fyne.TextWrapWord
	f.tlsRow = container.NewVBox(f.tls, tlsNote)
	f.tlsRow.Hide()

	f.proto = widget.NewRadioGroup([]string{socksLabel, httpLabel}, nil)
	f.proto.Horizontal = true
	f.proto.SetSelected(socksLabel)
	f.proto.OnChanged = func(sel string) {
		f.http = sel == httpLabel
		if f.http {
			f.tlsRow.Show()
		} else {
			f.tlsRow.Hide()
		}
		f.applyDefaultPort()
	}
	// TLS меняет дефолтный порт так же, как смена протокола.
	f.tls.OnChanged = func(bool) { f.applyDefaultPort() }

	tagNote := widget.NewLabel(locale.T(addServerTagNoteText))
	tagNote.Wrapping = fyne.TextWrapWord

	f.container = container.NewVBox(
		labeledRow(locale.T("Protocol"), f.proto),
		widget.NewSeparator(),
		labeledRow(locale.T("Host"), f.host),
		labeledRow(locale.T("Port"), f.port),
		labeledRow(locale.T("Username"), f.user),
		labeledRow(locale.T("Password"), f.pass),
		f.tlsRow,
		widget.NewSeparator(),
		labeledRow(locale.T("Label"), f.tag),
		tagNote,
	)
	return f
}

// defaultPort — порт по умолчанию для текущей комбинации протокол+TLS.
func (f *addServerForm) defaultPort() string {
	if !f.http {
		return "1080"
	}
	if f.tls.Checked {
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
	f.portEdit = false
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

// buildURI собирает share-URI из полей формы. Ошибки — человеческие: пустой
// хост и негодный порт ловятся здесь, а не в глубине парсера.
func (f *addServerForm) buildURI() (string, error) {
	host := strings.TrimSpace(f.host.Text)
	if host == "" {
		return "", fmt.Errorf("%s", locale.T("Host required"))
	}

	port, err := strconv.Atoi(strings.TrimSpace(f.port.Text))
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("%s", locale.T("Port 1..65535"))
	}

	user := strings.TrimSpace(f.user.Text)
	pass := f.pass.Text // пароль не триммим: пробелы в нём легальны

	scheme := "socks5"
	if f.http {
		scheme = "proxy-http"
		if f.tls.Checked {
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
		Fragment: strings.TrimSpace(f.tag.Text),
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
