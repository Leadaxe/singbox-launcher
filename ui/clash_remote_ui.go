// File clash_remote_ui.go — SPEC 064: UI помощники для remote-endpoint flow.
//
// Содержит:
//   - `newRemoteEndpointBadge` — текстовый badge "🏠 Local" / "🌐 host:port",
//     рендерится в шапке Servers tab. Автоматически обновляется при
//     SetRemoteOverride/ClearRemoteOverride через OnOverrideChanged.
//   - `showRemoteEndpointDialog` — модальное окно с Host/Port/Secret полями,
//     валидацией, reachability probe и кнопками Reset / Use Local / Connect.
//   - `probeRemoteEndpoint` — 3s GET /version против пред-активированного
//     endpoint'а; используется в Connect-flow.
//
// Storage / generation counter / EffectiveClashAPIConfig — в `clash_remote.go`.
package ui

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
)

// probeTimeout — TTL для reachability probe в Connect-flow. 3 секунды — баланс
// между «дать сети ответить» и «не подвешивать UI на минуту».
const probeTimeout = 3 * time.Second

// newRemoteEndpointBadge — text widget показывающий текущий endpoint-mode.
//
// Renders:
//   - "🏠 Local"     — default, override не active
//   - "🌐 host:port" — override active
//
// Регистрирует listener через OnOverrideChanged для авто-update'а; caller
// должен убедиться что viz-callback'и thread-safe (fyne.Do внутри listener'а).
func newRemoteEndpointBadge() *widget.Label {
	badge := widget.NewLabel("")
	refresh := func() {
		if ov, ok := GetRemoteOverride(); ok {
			badge.SetText(locale.Tf("servers.endpoint.badge_remote_format", ov.Host, ov.Port))
		} else {
			badge.SetText(locale.T("servers.endpoint.badge_local"))
		}
	}
	refresh()
	OnOverrideChanged(func() {
		fyne.Do(refresh)
	})
	return badge
}

// probeRemoteEndpoint — синхронный GET /version с per-call 3s timeout.
//
// Не использует глобальный api.TestAPIConnection (тот зашит на 20s + global
// httpClient с собственным IdleConnTimeout — нам нужен «быстрый probe»).
// Возвращает nil если endpoint живой и отвечает 200; иначе error с понятной
// причиной (timeout / connection refused / bad status).
func probeRemoteEndpoint(host string, port int, secret string) error {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	url := fmt.Sprintf("http://%s:%d/version", host, port)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	client := &http.Client{
		Timeout: probeTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: probeTimeout}).DialContext,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// showRemoteEndpointDialog — модальное окно для управления remote-override.
//
// Поля:
//   - Host  (string, normalized через NormalizeHost при Connect)
//   - Port  (int, 1..65535)
//   - Secret (string, password mode; empty allowed)
//
// Кнопки:
//   - Reset to local config — prefill из ac.APIService.GetClashAPIConfig()
//     если local config валиден; disabled иначе.
//   - Use Local — ClearRemoteOverride + close.
//   - Cancel — close без изменений.
//   - Connect — validation → 3s probe → confirm-on-fail → Set + close.
//
// Каллер должен передать `parent` для модальности и `onChanged` (опциональный
// callback после Set/Clear — обычно trigger таб-refresh; nil = noop).
func showRemoteEndpointDialog(ac *core.AppController, parent fyne.Window, onChanged func()) {
	if parent == nil {
		return
	}

	// Prefill: если override уже active — берём оттуда; иначе пустые поля.
	hostEntry := widget.NewEntry()
	portEntry := widget.NewEntry()
	secretEntry := widget.NewPasswordEntry()
	if ov, ok := GetRemoteOverride(); ok {
		hostEntry.SetText(ov.Host)
		portEntry.SetText(strconv.Itoa(ov.Port))
		secretEntry.SetText(ov.Secret)
	}

	hostEntry.SetPlaceHolder("192.168.10.1")
	portEntry.SetPlaceHolder("9090")
	secretEntry.SetPlaceHolder(locale.T("servers.endpoint.secret"))

	errorLabel := widget.NewLabel("")
	errorLabel.Wrapping = fyne.TextWrapWord
	errorLabel.Hide()

	setError := func(msg string) {
		if msg == "" {
			errorLabel.Hide()
			return
		}
		errorLabel.SetText(msg)
		errorLabel.Show()
	}

	noteLabel := widget.NewLabel(locale.T("servers.endpoint.note_http_only"))
	noteLabel.Wrapping = fyne.TextWrapWord

	form := container.NewVBox(
		widget.NewLabel(locale.T("servers.endpoint.host")),
		hostEntry,
		widget.NewLabel(locale.T("servers.endpoint.port")),
		portEntry,
		widget.NewLabel(locale.T("servers.endpoint.secret")),
		secretEntry,
		noteLabel,
		errorLabel,
	)

	// Reset to local config: тянем live из APIService (он держит local config
	// в RAM; ReloadClashAPIConfig пересинкается с config.json). Disabled если
	// local config недоступен (cold start, нет sing-box config).
	resetBtn := widget.NewButton(locale.T("servers.endpoint.reset_to_local"), nil)
	resetBtn.OnTapped = func() {
		if ac == nil || ac.APIService == nil {
			return
		}
		baseURL, token, enabled := ac.APIService.GetClashAPIConfig()
		if !enabled || baseURL == "" {
			setError(locale.T("servers.endpoint.no_local_config"))
			return
		}
		// baseURL имеет формат "http://host:port" — парсим обратно.
		host, portStr, ok := splitHostPort(baseURL)
		if !ok {
			setError(locale.T("servers.endpoint.no_local_config"))
			return
		}
		hostEntry.SetText(host)
		portEntry.SetText(portStr)
		secretEntry.SetText(token)
		setError("")
	}
	if ac == nil || ac.APIService == nil {
		resetBtn.Disable()
	} else if _, _, enabled := ac.APIService.GetClashAPIConfig(); !enabled {
		resetBtn.Disable()
		resetBtn.SetText(locale.T("servers.endpoint.reset_to_local") + " (" + locale.T("servers.endpoint.no_local_config") + ")")
	}

	useLocalBtn := widget.NewButton(locale.T("servers.endpoint.use_local"), nil)
	cancelBtn := widget.NewButton(locale.T("servers.endpoint.cancel"), nil)
	connectBtn := widget.NewButton(locale.T("servers.endpoint.connect"), nil)
	connectBtn.Importance = widget.HighImportance

	buttonsRow := container.NewBorder(
		nil, nil,
		resetBtn,
		container.NewHBox(useLocalBtn, cancelBtn, connectBtn),
	)

	content := container.NewVBox(form, buttonsRow)

	// buttonsRow уже внутри content (Cancel/Connect/Use Local/Reset).
	// NewCustom: buttons=nil + dismissText="" → не добавляем дополнительные кнопки.
	dlg := dialogs.NewCustom(locale.T("servers.endpoint.dialog.title"), content, nil, "", parent)
	dlg.Resize(fyne.NewSize(440, 360))

	useLocalBtn.OnTapped = func() {
		ClearRemoteOverride()
		dlg.Hide()
		if onChanged != nil {
			onChanged()
		}
	}
	cancelBtn.OnTapped = func() {
		dlg.Hide()
	}
	connectBtn.OnTapped = func() {
		setError("")

		host, err := NormalizeHost(hostEntry.Text)
		if err != nil {
			setError(err.Error())
			return
		}
		portRaw := hostEntry.Text // for error formatting only — reuse below
		_ = portRaw

		portInt, err := strconv.Atoi(secretEntry.Text)
		_ = portInt
		// real port parse:
		portN, perr := strconv.Atoi(portEntry.Text)
		if perr != nil || portN < 1 || portN > 65535 {
			setError(locale.T("servers.endpoint.invalid_port"))
			return
		}
		secret := secretEntry.Text

		// Probe в background, чтобы UI не подвисал. Disable Connect на время.
		connectBtn.Disable()
		connectBtn.SetText(locale.T("servers.endpoint.connecting"))
		go func() {
			probeErr := probeRemoteEndpoint(host, portN, secret)
			fyne.Do(func() {
				connectBtn.Enable()
				connectBtn.SetText(locale.T("servers.endpoint.connect"))
				if probeErr != nil {
					// Confirm-modal: «Endpoint unreachable. Connect anyway?»
					dialogs.ShowConfirm(
						parent,
						locale.T("servers.endpoint.unreachable_title"),
						locale.Tf("servers.endpoint.unreachable_body", probeErr),
						func(yes bool) {
							if !yes {
								return
							}
							SetRemoteOverride(RemoteOverride{Host: host, Port: portN, Secret: secret})
							dlg.Hide()
							if onChanged != nil {
								onChanged()
							}
						},
					)
					return
				}
				SetRemoteOverride(RemoteOverride{Host: host, Port: portN, Secret: secret})
				dlg.Hide()
				if onChanged != nil {
					onChanged()
				}
			})
		}()
	}

	dlg.Show()
}

// splitHostPort — разбирает строку вида "http://host:port" обратно на (host, "port", true).
// Возвращает ok=false если URL не парсится или нет порта.
func splitHostPort(baseURL string) (host, portStr string, ok bool) {
	const prefix = "http://"
	if len(baseURL) < len(prefix) || baseURL[:len(prefix)] != prefix {
		return "", "", false
	}
	rest := baseURL[len(prefix):]
	// Поскольку local config всегда даёт "http://host:port" (LoadClashAPIConfig),
	// trailing path/query не ожидаются; но защитимся.
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' {
			rest = rest[:i]
			break
		}
	}
	for i := 0; i < len(rest); i++ {
		if rest[i] == ':' {
			return rest[:i], rest[i+1:], true
		}
	}
	return "", "", false
}

// Ensure api package referenced for future probe extensions (TestAPIConnection
// is intentionally NOT used here — see probeRemoteEndpoint comment).
var _ = api.TestAPIConnection
