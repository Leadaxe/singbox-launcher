package api

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/urlredact"

	"github.com/muhammadmuzzammil1998/jsonc"
)

// APIConfigState — вердикт разбора `experimental.clash_api` (SPEC 143).
//
// Раньше «нет secret» и «нет адреса» давали одну фатальную ошибку, и
// валидный конфиг ядра с пустым secret (sing-box отдаёт такой API без
// аутентификации) считался ошибкой конфигурации: лаунчер выключал весь API.
// Теперь это состояние, а не ошибка.
type APIConfigState string

const (
	// APIConfigOK — адрес есть, secret непустой: запросы идут с Bearer.
	APIConfigOK APIConfigState = "ok"
	// APIConfigNoAuth — адрес есть, secret пуст: API ядра без
	// аутентификации. Запросы идут БЕЗ заголовка Authorization.
	APIConfigNoAuth APIConfigState = "no_auth"
	// APIConfigMissing — секции `experimental.clash_api` или адреса нет:
	// API не настроен, это не ошибка.
	APIConfigMissing APIConfigState = "missing"
	// APIConfigInvalid — адрес есть, но не разобрался как host:port.
	APIConfigInvalid APIConfigState = "invalid"
	// APIConfigUnreadable — файл не прочитался или не распарсился.
	APIConfigUnreadable APIConfigState = "unreadable"
)

// Enabled — по этому состоянию решается, работать ли с API ядра.
func (s APIConfigState) Enabled() bool {
	return s == APIConfigOK || s == APIConfigNoAuth
}

// APIConfig — разобранная конфигурация Clash API.
type APIConfig struct {
	// State — вердикт разбора.
	State APIConfigState
	// BaseURL — http://host:port; пусто, если State не Enabled.
	BaseURL string
	// Token — secret как есть; пусто при APIConfigNoAuth. Наружу (лог,
	// UI, URL, аргументы процесса) не выводится — см. urlredact.
	Token string
	// Detail — английская причина для лога и UI.
	Detail string
}

// Enabled — API пригоден к использованию (с аутентификацией или без).
func (c APIConfig) Enabled() bool { return c.State.Enabled() }

// RequiresAuth — запросы должны нести заголовок Authorization.
func (c APIConfig) RequiresAuth() bool { return c.State == APIConfigOK }

// authHeader — значение заголовка Authorization, если он нужен.
//
// Пустой secret означает «без аутентификации»: заголовок не ставится вовсе.
// Отправлять `Bearer ` с пустым токеном нельзя — это и неверный запрос, и
// ложный след в логе сервера.
func (c APIConfig) authHeader() (string, bool) {
	if !c.RequiresAuth() {
		return "", false
	}
	return "Bearer " + c.Token, true
}

// ApplyAuthHeaders — единственная точка постановки аутентификации на
// запрос к Clash API. Пустой secret ⇒ заголовка нет.
func ApplyAuthHeaders(reqHeader interface{ Set(string, string) }, cfg APIConfig) {
	if h, ok := cfg.authHeader(); ok {
		reqHeader.Set("Authorization", h)
	}
}

// SetAuthHeader — то же по «сырому» token: пустой token означает API без
// аутентификации, и заголовок Authorization не ставится вовсе. Одна точка
// для всех запросов к ядру, чтобы `Bearer ` с пустым токеном не появился
// снова в новом вызове.
func SetAuthHeader(reqHeader interface{ Set(string, string) }, token string) {
	if token == "" {
		return
	}
	reqHeader.Set("Authorization", "Bearer "+token)
}

// LoadClashAPIConfig читает конфигурацию Clash API из config.json.
//
// Возвращает разобранное состояние. Ошибка возвращается только тогда, когда
// это действительно ошибка (файл не читается/не парсится или адрес невалиден);
// отсутствие секции и пустой secret — штатные состояния, а не ошибка.
func LoadClashAPIConfig(configPath string) (APIConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		// Cold-start logging: на свежей инсталляции config.json ещё не существует
		// (пользователь не нажал Save → не пересобирали через RunParser). Это не
		// ошибка приложения, а ожидаемое первое-запуск состояние. Логируем как
		// DEBUG, чтобы не пугать пользователя красным ERROR в логах. Все callers
		// обрабатывают возвращаемую error как «нет clash API» и продолжают работу.
		if os.IsNotExist(err) {
			debuglog.DebugLog("LoadClashAPIConfig: config.json not present yet (cold start): %v", err)
		} else {
			debuglog.ErrorLog("LoadClashAPIConfig: Failed to read config.json: %v", err)
		}
		return APIConfig{State: APIConfigUnreadable, Detail: "cannot read config.json"},
			fmt.Errorf("failed to read config.json: %w", err)
	}
	cleanData := jsonc.ToJSON(data)

	var jsonData map[string]interface{}
	if err := json.Unmarshal(cleanData, &jsonData); err != nil {
		debuglog.ErrorLog("LoadClashAPIConfig: Failed to parse JSON: %v", err)
		return APIConfig{State: APIConfigUnreadable, Detail: "cannot parse config.json"},
			fmt.Errorf("failed to parse JSON: %w", err)
	}

	exp, ok := jsonData["experimental"].(map[string]interface{})
	if !ok {
		return APIConfig{State: APIConfigMissing, Detail: "no 'experimental' section"}, nil
	}
	apiSec, ok := exp["clash_api"].(map[string]interface{})
	if !ok {
		return APIConfig{State: APIConfigMissing, Detail: "no 'clash_api' section"}, nil
	}

	host, _ := apiSec["external_controller"].(string)
	secret, _ := apiSec["secret"].(string)
	host = strings.TrimSpace(host)

	if host == "" {
		return APIConfig{State: APIConfigMissing, Detail: "'external_controller' is empty"}, nil
	}
	if !validControllerHost(host) {
		debuglog.WarnLog("LoadClashAPIConfig: 'external_controller' %q is not host:port", host)
		return APIConfig{State: APIConfigInvalid, Detail: "'external_controller' is not a valid host:port"}, nil
	}

	baseURL := "http://" + host
	if secret == "" {
		// Ядро поднимает API без аутентификации — это допустимо, но только на
		// петле. Наружу без secret выставлять нельзя: предупреждаем, не отключая.
		if !isLoopbackHost(host) {
			debuglog.WarnLog("LoadClashAPIConfig: Clash API on %s has no secret and is not bound to loopback", host)
			return APIConfig{
				BaseURL: baseURL,
				State:   APIConfigNoAuth,
				Detail:  "no secret and the controller is not on loopback",
			}, nil
		}
		debuglog.DebugLog("Clash API loaded from config: %s / no secret (unauthenticated loopback)", baseURL)
		return APIConfig{BaseURL: baseURL, State: APIConfigNoAuth, Detail: "no secret"}, nil
	}

	debuglog.DebugLog("Clash API loaded from config: %s / token=%s", baseURL, urlredact.RedactToken(secret))
	return APIConfig{BaseURL: baseURL, Token: secret, State: APIConfigOK}, nil
}

// validControllerHost — host:port, который примет http.Client.
func validControllerHost(host string) bool {
	if strings.ContainsAny(host, " \t/") {
		return false
	}
	_, port, err := net.SplitHostPort(host)
	if err != nil || port == "" {
		return false
	}
	return true
}

// isLoopbackHost — адрес указывает на петлю.
func isLoopbackHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
