// File openconnect_sso_callback.go — временный loopback-слушатель для SSO
// openconnect в callback-режиме (SPEC 167 §2.5 п. 3; так работает SAML у
// FortiGate). Шлюз после входа в браузере редиректит на
// http://127.0.0.1:<port>/…; слушатель отдаёт короткую страницу и передаёт
// полный URL в канал, лаунчер шлёт его ядру как BrowserResult.finalURL.
//
// Не постоянный сервис: живёт, пока открыт диалог входа, и закрывается
// вместе с ним. Слушает только 127.0.0.1 — наружу порт не открывается.
// Удалённая машина — тот же путь: браузер и слушатель на этом компьютере,
// ядро машины получает только строку URL.
package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"singbox-launcher/internal/debuglog"
)

// DefaultSSOCallbackPort — порт по умолчанию, как у FortiClient.
const DefaultSSOCallbackPort = 8020

// SSOCallbackNodePortKey — ключ тела узла с портом слушателя, когда шлюз
// не называет порт в префиксе.
const SSOCallbackNodePortKey = "lx_sso_callback_port"

// SSOCallbackLoopbackOnly — все префиксы ведут на этот компьютер (и их
// больше нуля): только тогда callback-режим поддержан.
func SSOCallbackLoopbackOnly(prefixes []string) bool {
	if len(prefixes) == 0 {
		return false
	}
	for _, p := range prefixes {
		u, err := url.Parse(strings.TrimSpace(p))
		if err != nil || !strings.EqualFold(u.Scheme, "http") {
			return false
		}
		if !ssoLoopbackHost(u.Hostname()) {
			return false
		}
	}
	return true
}

// ssoLoopbackHost — имя, которое браузер приведёт на IPv4-слушатель
// 127.0.0.1 (localhost браузеры при отказе ::1 пробуют по IPv4).
func ssoLoopbackHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.To4() != nil && ip.IsLoopback()
}

// SSOCallbackPort — порт слушателя: из префикса, если шлюз его назвал;
// иначе nodePort (поле lx_sso_callback_port узла, 0 — не задано); иначе
// DefaultSSOCallbackPort.
func SSOCallbackPort(prefixes []string, nodePort int) int {
	for _, p := range prefixes {
		u, err := url.Parse(strings.TrimSpace(p))
		if err != nil {
			continue
		}
		if n, err := strconv.Atoi(u.Port()); err == nil && n > 0 && n <= 65535 {
			return n
		}
	}
	if nodePort > 0 && nodePort <= 65535 {
		return nodePort
	}
	return DefaultSSOCallbackPort
}

// SSOCallbackListener — слушатель на время одного диалога входа.
type SSOCallbackListener struct {
	port     int
	prefixes []string
	srv      *http.Server
	urls     chan string
	once     sync.Once
}

// ssoCallbackPage — ответ браузеру после редиректа.
const ssoCallbackPage = `<!doctype html><html><head><meta charset="utf-8"><title>Sign-in complete</title></head>` +
	`<body style="font-family:sans-serif;text-align:center;margin-top:20vh">` +
	`<h2>Sign-in complete</h2><p>You can close this window and return to the launcher.</p></body></html>`

// StartSSOCallbackListener поднимает слушатель на 127.0.0.1:port. prefixes —
// callbackURLPrefixes ядра: принимается только запрос, чей URL начинается
// с одного из них (favicon и прочие запросы браузера отсекаются); пустой
// список — принимается любой запрос, кроме /favicon.ico. Ошибка — порт
// занят или запрещён; текст называет порт.
func StartSSOCallbackListener(port int, prefixes []string) (*SSOCallbackListener, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	debuglog.DebugLog("openconnect sso: starting callback listener on %s", addr)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		debuglog.WarnLog("openconnect sso: listen on %s failed: %v", addr, err)
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	l := &SSOCallbackListener{
		port:     port,
		prefixes: prefixes,
		urls:     make(chan string, 1),
	}
	l.srv = &http.Server{
		Handler:           http.HandlerFunc(l.handle),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if serr := l.srv.Serve(ln); serr != nil && !errors.Is(serr, http.ErrServerClosed) {
			debuglog.WarnLog("openconnect sso: serve: %v", serr)
		}
	}()
	debuglog.InfoLog("openconnect sso: callback listener on %s", addr)
	return l, nil
}

// handle принимает редирект шлюза.
func (l *SSOCallbackListener) handle(w http.ResponseWriter, r *http.Request) {
	full, ok := l.match(r.RequestURI)
	if !ok {
		http.NotFound(w, r)
		return
	}
	select {
	case l.urls <- full:
		debuglog.DebugLog("openconnect sso: callback received")
	default:
		// Первый URL уже передан; повторный запрос (обновление вкладки)
		// получает ту же страницу.
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(ssoCallbackPage))
}

// match — полный URL запроса в форме, которую ждёт ядро. Форма по
// умолчанию — http://127.0.0.1:<port><RequestURI>; если префикс ядра назван
// через localhost, URL строится его хостом, чтобы префикс совпал.
func (l *SSOCallbackListener) match(requestURI string) (string, bool) {
	def := fmt.Sprintf("http://127.0.0.1:%d%s", l.port, requestURI)
	if len(l.prefixes) == 0 {
		return def, !strings.HasPrefix(requestURI, "/favicon.ico")
	}
	for _, p := range l.prefixes {
		p = strings.TrimSpace(p)
		u, err := url.Parse(p)
		if err != nil {
			continue
		}
		host := u.Host
		if u.Port() == "" {
			host = net.JoinHostPort(u.Hostname(), strconv.Itoa(l.port))
		}
		if strings.HasPrefix(def, p) {
			return def, true
		}
		if full := "http://" + host + requestURI; strings.HasPrefix(full, p) {
			return full, true
		}
	}
	return "", false
}

// Port — порт слушателя.
func (l *SSOCallbackListener) Port() int { return l.port }

// URLs — канал с полным URL редиректа (один раз).
func (l *SSOCallbackListener) URLs() <-chan string { return l.urls }

// Close гасит слушатель. Идемпотентен.
func (l *SSOCallbackListener) Close() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = l.srv.Shutdown(ctx)
		debuglog.DebugLog("openconnect sso: callback listener on port %d closed", l.port)
	})
}
