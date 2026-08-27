package lxdclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Config описывает подключение к демону.
type Config struct {
	// Addr — host:port управляющего канала.
	Addr string
	// ServerFingerprint — пин серверного сертификата (lowercase hex). Пусто =
	// plain h2c без TLS (dev-режим демона на loopback). Пин обязателен для
	// любого TLS-канала: bootstrap без пина лаунчеру больше не нужен —
	// приглашение (с отпечатком внутри) приходит из вывода установки службы
	// либо из `sing-box lxd client add`, а не через operator-маршрут.
	ServerFingerprint string
	// Identity — клиентская пара для mTLS. Обязателна для всех admin/gRPC
	// вызовов при TLS, кроме Enroll (там достаточно кода).
	Identity *Identity
	// Secret — Bearer-секрет. Нужен ТОЛЬКО для plain-h2c демона (dev): в
	// mTLS-режиме доверенный клиентский сертификат — полный мандат, секретом
	// владеет демон, и лаунчер его не знает. Слать заголовок при mTLS
	// безвредно (демон его игнорирует).
	Secret string
	// LogKey — под каким ключом писать журнал обмена (обычно ID записи
	// реестра). Пусто = не журналировать: клиента к демону строят и разовые
	// сценарии (проба канала, enroll), которым журнал ни к чему.
	LogKey string
}

// TLSEnabled — включён ли TLS-канал (установлен пин сервера).
func (c Config) TLSEnabled() bool { return c.ServerFingerprint != "" }

// AddrString возвращает адрес управляющего канала (для логов/статуса).
func (c *Client) AddrString() string { return c.cfg.Addr }

// Client — REST-клиент admin-плоскости + фабрика gRPC-соединений.
type Client struct {
	cfg    Config
	httpc  *http.Client
	logKey string
}

const restTimeout = 30 * time.Second

// New создаёт клиента. Все REST-вызовы идут с общим таймаутом restTimeout;
// стримы gRPC живут на собственном соединении без таймаута.
func New(cfg Config) *Client {
	transport := &http.Transport{}
	if cfg.TLSEnabled() {
		transport.TLSClientConfig = cfg.tlsConfig()
	}
	return &Client{
		cfg:    cfg,
		httpc:  &http.Client{Timeout: restTimeout, Transport: transport},
		logKey: cfg.LogKey,
	}
}

// tlsConfig строит клиентский TLS: свой сертификат + пин сервера по
// отпечатку. Серверный сертификат демона самоподписан и без SAN, поэтому
// штатная верификация невозможна by design — доверие держится на пине
// (InsecureSkipVerify + VerifyPeerCertificate), как в модели 057 форка.
func (c Config) tlsConfig() *tls.Config {
	conf := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}
	if c.ServerFingerprint != "" {
		conf.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("lxdclient: server presented no certificate")
			}
			got := FingerprintOf(rawCerts[0])
			if got != strings.ToLower(c.ServerFingerprint) {
				return fmt.Errorf("lxdclient: server fingerprint does not match the pinned one (got %s…)", got[:12])
			}
			return nil
		}
	}
	if c.Identity != nil {
		conf.Certificates = []tls.Certificate{c.Identity.TLSCert}
	}
	return conf
}

func (c *Client) baseURL() string {
	scheme := "http"
	if c.cfg.TLSEnabled() {
		scheme = "https"
	}
	return scheme + "://" + c.cfg.Addr
}

// --- REST admin plane ---------------------------------------------------

// StatusInfo — ответ GET /admin/status.
type StatusInfo struct {
	Status           string `json:"status"` // idle | started | fatal
	ActiveSHA        string `json:"active_sha256"`
	LastGoodSHA      string `json:"last_good_sha256"`
	LastError        string `json:"last_error"`
	InterruptedApply bool   `json:"interrupted_apply"`
}

// ApplyError — типизированная ошибка применения конфига.
type ApplyError struct {
	StatusCode int
	RolledBack bool
	Message    string
}

func (e *ApplyError) Error() string {
	if e.RolledBack {
		return fmt.Sprintf("config rejected, daemon rolled back to last-good: %s", e.Message)
	}
	return e.Message
}

// Rejected — конфиг не прошёл валидацию (422): работающий инстанс не тронут.
func (e *ApplyError) Rejected() bool { return e.StatusCode == http.StatusUnprocessableEntity }

// do — единственная точка выхода REST-плоскости наружу, поэтому журнал
// обмена ведётся здесь: любой новый вызов попадает в него сам, без правки
// в каждом методе.
//
// Пишется только конверт (метод, путь, код, длительность, ошибка). Тела не
// журналируются by design: через /admin/apply едет конфиг с приватными
// ключами узлов, а журнал открывается кнопкой и копируется в тикет.
func (c *Client) do(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	return c.doCtx(context.Background(), method, path, body, contentType)
}

// doCtx — то же, но с собственным сроком вызова. Нужен там, где общий
// restTimeout заведомо велик: у рабочих вызовов (apply, ресурсы) тридцать
// секунд — запас на медленный канал, а у справочных запросов ровно наоборот
// цена ожидания выше цены ответа (SPEC 113-E M6: пикер интерфейсов).
//
// Дедлайн задаётся контекстом, а не подменой c.httpc.Timeout: клиент один на
// вызов, но Transport в нём общий, и трогать поле http.Client параллельным
// вызовам нельзя.
func (c *Client) doCtx(ctx context.Context, method, path string, body io.Reader, contentType string) (*http.Response, error) {
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL()+path, body)
	if err != nil {
		c.record(method+" "+path, started, 0, err)
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.cfg.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Secret)
	}
	resp, err := c.httpc.Do(req)
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	c.record(method+" "+path, started, status, err)
	return resp, err
}

func decodeError(resp *http.Response) string {
	var payload struct {
		Error string `json:"error"`
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if json.Unmarshal(data, &payload) == nil && payload.Error != "" {
		return payload.Error
	}
	return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
}

// Status возвращает состояние демона.
func (c *Client) Status() (StatusInfo, error) {
	resp, err := c.do(http.MethodGet, "/admin/status", nil, "")
	if err != nil {
		return StatusInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return StatusInfo{}, fmt.Errorf("lxdclient: status: %s", decodeError(resp))
	}
	var info StatusInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&info); err != nil {
		return StatusInfo{}, fmt.Errorf("lxdclient: status decode: %w", err)
	}
	return info, nil
}

// Apply отправляет конфиг демону: валидация сабпроцессом → подмена инстанса
// → last-good, с автооткатом на провале старта. 422 → ApplyError.Rejected().
func (c *Client) Apply(config []byte) error {
	resp, err := c.do(http.MethodPost, "/admin/apply", bytes.NewReader(config), "application/json")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var payload struct {
		Error      string `json:"error"`
		RolledBack bool   `json:"rolled_back"`
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	message := strings.TrimSpace(string(data))
	if json.Unmarshal(data, &payload) == nil && payload.Error != "" {
		message = payload.Error
	}
	return &ApplyError{StatusCode: resp.StatusCode, RolledBack: payload.RolledBack, Message: message}
}

// Start поднимает ядро из last-good (без нового конфига).
func (c *Client) Start() error { return c.simplePost("/admin/start") }

// Stop гасит ядро; демон и канал остаются жить.
func (c *Client) Stop() error { return c.simplePost("/admin/stop") }

// Rollback откатывает на last-good.
func (c *Client) Rollback() error { return c.simplePost("/admin/rollback") }

func (c *Client) simplePost(path string) error {
	resp, err := c.do(http.MethodPost, path, nil, "")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lxdclient: %s: %s", path, decodeError(resp))
	}
	return nil
}

// ActiveConfig возвращает активный конфиг демона (GET /admin/config).
func (c *Client) ActiveConfig() ([]byte, error) {
	resp, err := c.do(http.MethodGet, "/admin/config", nil, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lxdclient: config: %s", decodeError(resp))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

// --- Ресурсы (.srs и прочие файлы, на которые ссылается конфиг) ----------
//
// Второй канал полезной нагрузки admin-плоскости (SPEC 063 форка ядра).
// Конфиг сплошь и рядом ссылается на внешние файлы — скомпилированные
// rule-set'ы `type: local, format: binary`. До появления этого стора класть
// их на машину было некуда, и лаунчер эмитил путь СВОЕЙ файловой системы:
// на удалённой машине такого пути нет, ядро не находило набор и не
// поднималось. `type: remote` не годится — источник правды по ресурсам
// оператор, а не публичный HTTP-хост.
//
// Адресация по имени: путь всегда `<state_dir>/resources/<name>`, содержимое
// перезаливается под тем же именем. sha256 в каждом ответе — по нему клиент
// решает, гнать ли байты вообще.

// Resource — метаданные одного файла в сторе.
type Resource struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	// Path — АБСОЛЮТНЫЙ путь на машине демона. Копируется в конфиг как есть:
	// собирать его самим нельзя (относительный путь резолвится от cwd ядра, и
	// конфиг ломается при другом cwd).
	Path string `json:"path"`
}

// ClientInfo — запись справочника устройств локальной сети машины.
//
// Пустое поле — это состояние, а не ошибка: у проводного клиента нет ssid, а
// вне Linux три из пяти провайдеров молчат вовсе (эндпоинт всё равно отвечает
// — арендами и метками).
type ClientInfo struct {
	Name  string `json:"name"`
	MAC   string `json:"mac"`
	SSID  string `json:"ssid"`
	Iface string `json:"iface"`
	Port  string `json:"port"`
	// Source — какие провайдеры заполнили запись («lease+arp+wireless»).
	// Нужен при разборе «почему устройство потеряло имя»: одинокий «label»
	// означает, что аренда DHCP истекла.
	Source string `json:"source"`
}

// ClientsInfo — справочник «адрес → устройство» (GET /admin/clients-info).
//
// Именно СПРАВОЧНИК, а не поле соединения: имена меняются в масштабе часов, и
// демон отдаёт таблицу целиком, чтобы клиент сам соединил её с исходными
// адресами. Поток соединений при этом не трогается.
func (c *Client) ClientsInfo() (map[string]ClientInfo, error) {
	resp, err := c.do(http.MethodGet, "/admin/clients-info", nil, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lxdclient: clients-info: %s", decodeError(resp))
	}
	var out struct {
		Clients map[string]ClientInfo `json:"clients"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("lxdclient: clients-info: parse: %w", err)
	}
	return out.Clients, nil
}

// SetClientLabel задаёт собственное имя устройства по IP или MAC.
//
// Метка переживает перезапуск демона и имеет приоритет над провайдерами.
// Метка на MAC следует за устройством между адресами, но телефоны
// рандомизируют MAC при переподключении — для них устойчивее метка на IP
// вместе с резервированием DHCP.
func (c *Client) SetClientLabel(key, name string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("lxdclient: client label key is empty")
	}
	body, err := json.Marshal(struct {
		Name string `json:"name"`
	}{Name: name})
	if err != nil {
		return err
	}
	resp, err := c.do(http.MethodPut, "/admin/clients-info/labels/"+url.PathEscape(key),
		bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("lxdclient: set client label: %s", decodeError(resp))
	}
	return nil
}

// DeleteClientLabel снимает собственное имя устройства.
func (c *Client) DeleteClientLabel(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("lxdclient: client label key is empty")
	}
	resp, err := c.do(http.MethodDelete, "/admin/clients-info/labels/"+url.PathEscape(key), nil, "")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("lxdclient: delete client label: %s", decodeError(resp))
	}
	return nil
}

// Resources возвращает список залитых ресурсов с хешами (GET /admin/resources).
func (c *Client) Resources() ([]Resource, error) {
	resp, err := c.do(http.MethodGet, "/admin/resources", nil, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lxdclient: resources: %s", decodeError(resp))
	}
	// Список приходит обёрнутым: {"resources": [...]}. PUT и Stat, наоборот,
	// отдают объект напрямую — форма ответа у демона разная, и путать их
	// нельзя.
	var out struct {
		Resources []Resource `json:"resources"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("lxdclient: resources: parse: %w", err)
	}
	return out.Resources, nil
}

// PutResource заливает (или перезаписывает) ресурс под именем name.
//
// 409 означает, что на имя ссылается активный или last-good конфиг: стор
// именованный, и подмена файла под живой ссылкой сделала бы откат ядра
// дырявым — текст конфига откатился бы, а набор под именем остался новым.
// Порядок «сначала залить, потом сослаться» поэтому не пожелание, а
// требование демона.
func (c *Client) PutResource(name string, body []byte) (Resource, error) {
	if strings.TrimSpace(name) == "" {
		return Resource{}, fmt.Errorf("lxdclient: resource name is empty")
	}
	resp, err := c.do(http.MethodPut, "/admin/resources/"+url.PathEscape(name),
		bytes.NewReader(body), "application/octet-stream")
	if err != nil {
		return Resource{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Resource{}, &ResourceError{
			StatusCode: resp.StatusCode,
			Name:       name,
			Message:    decodeError(resp),
		}
	}
	var out Resource
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return Resource{}, fmt.Errorf("lxdclient: resource %q: parse: %w", name, err)
	}
	return out, nil
}

// ResourceContent скачивает сырые байты ресурса
// (GET /admin/resources/{name}/content).
//
// Нужен, чтобы забрать файл, положенный на машину мимо лаунчера: без этого
// про такой ресурс известно только имя и хеш, а что внутри — нет.
func (c *Client) ResourceContent(name string) ([]byte, error) {
	resp, err := c.do(http.MethodGet, "/admin/resources/"+url.PathEscape(name)+"/content", nil, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &ResourceError{StatusCode: resp.StatusCode, Name: name, Message: decodeError(resp)}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// DeleteResource удаляет ресурс (DELETE /admin/resources/{name}).
//
// 409, как и у PutResource, означает «на имя ссылается активный или last-good
// конфиг»: демон не даёт удалить файл из-под работающего ядра.
func (c *Client) DeleteResource(name string) error {
	resp, err := c.do(http.MethodDelete, "/admin/resources/"+url.PathEscape(name), nil, "")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &ResourceError{StatusCode: resp.StatusCode, Name: name, Message: decodeError(resp)}
	}
	return nil
}

// ResourceError — отказ операции над ресурсом. StatusCode 409 = имя занято
// активным/last-good конфигом (см. PutResource).
type ResourceError struct {
	StatusCode int
	Name       string
	Message    string
}

func (e *ResourceError) Error() string {
	return fmt.Sprintf("resource %q: HTTP %d: %s", e.Name, e.StatusCode, e.Message)
}

// InUse — ресурс занят живой ссылкой из конфига.
func (e *ResourceError) InUse() bool { return e.StatusCode == http.StatusConflict }

// --- Сопряжение ---------------------------------------------------------

// Enroll регистрирует клиентский сертификат по одноразовому коду. Вызывается
// на клиенте, собранном из приглашения (пин уже установлен), до появления
// доверия — маршрут /admin/enroll защищён только кодом.
func (c *Client) Enroll(code, name string) error {
	if c.cfg.Identity == nil {
		return fmt.Errorf("lxdclient: enroll requires a client keypair")
	}
	body, err := json.Marshal(map[string]string{
		"code":     code,
		"name":     name,
		"cert_pem": string(c.cfg.Identity.CertPEM),
	})
	if err != nil {
		return err
	}
	resp, err := c.do(http.MethodPost, "/admin/enroll", bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lxdclient: enroll: %s", decodeError(resp))
	}
	return nil
}

// Do выполняет произвольный запрос admin-плоскости и возвращает статус, тело
// и Content-Type ответа (raw REST passthrough, SPEC 100 §3.7).
//
// Ошибка возвращается только на транспортном отказе (машина недоступна,
// таймаут, пин не сошёлся); HTTP-статусы, включая 4xx/5xx, доезжают до
// вызывающего как данные — passthrough не интерпретирует ответ демона.
func (c *Client) Do(method, path string, body []byte, contentType string) (int, []byte, string, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	resp, err := c.do(method, path, reader, contentType)
	if err != nil {
		return 0, nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return 0, nil, "", fmt.Errorf("lxdclient: raw %s %s: read body: %w", method, path, err)
	}
	return resp.StatusCode, data, resp.Header.Get("Content-Type"), nil
}

// InfoData — паспорт демона (GET /admin/info): версия, домашний каталог,
// адрес, отпечаток. Лаунчер берёт пути демона отсюда, а не хардкодит их.
type InfoData struct {
	Version       string `json:"version"`
	StateDir      string `json:"state_dir"`
	Listen        string `json:"listen"`
	TLS           bool   `json:"tls"`
	Fingerprint   string `json:"fingerprint"`
	PID           int    `json:"pid"`
	UptimeSeconds int    `json:"uptime_seconds"`
	LogPath       string `json:"log_path"`
}

// Info возвращает паспорт демона.
func (c *Client) Info() (InfoData, error) {
	resp, err := c.do(http.MethodGet, "/admin/info", nil, "")
	if err != nil {
		return InfoData{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return InfoData{}, fmt.Errorf("lxdclient: info: %s", decodeError(resp))
	}
	var info InfoData
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&info); err != nil {
		return InfoData{}, fmt.Errorf("lxdclient: info decode: %w", err)
	}
	return info, nil
}

// --- gRPC plane ---------------------------------------------------------

// DialGRPC создаёт gRPC-соединение к плоскости daemon.StartedService: mTLS с
// пином (или plain h2c без TLS) + Bearer-метаданные на каждый вызов.
func (c *Client) DialGRPC() (*grpc.ClientConn, error) {
	transportCredentials := insecure.NewCredentials()
	if c.cfg.TLSEnabled() {
		transportCredentials = credentials.NewTLS(c.cfg.tlsConfig())
	}
	return grpc.NewClient(c.cfg.Addr,
		grpc.WithTransportCredentials(transportCredentials),
		grpc.WithUnaryInterceptor(c.authUnaryInterceptor()),
		grpc.WithStreamInterceptor(c.authStreamInterceptor()),
	)
}

func (c *Client) authUnaryInterceptor() grpc.UnaryClientInterceptor {
	secret := c.cfg.Secret
	return func(ctx context.Context, method string, req, reply any, conn *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if secret != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+secret)
		}
		return invoker(ctx, method, req, reply, conn, opts...)
	}
}

func (c *Client) authStreamInterceptor() grpc.StreamClientInterceptor {
	secret := c.cfg.Secret
	return func(ctx context.Context, desc *grpc.StreamDesc, conn *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if secret != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+secret)
		}
		return streamer(ctx, desc, conn, method, opts...)
	}
}

// --- Определение канала (follow-the-daemon) ------------------------------

// ChannelKind — результат пробы управляющего канала демона.
type ChannelKind int

const (
	// ChannelUnknown — адрес не отвечает (демон не запущен / порт закрыт).
	ChannelUnknown ChannelKind = iota
	// ChannelPlain — на адресе живёт plain-h2c демон (HTTP без TLS).
	ChannelPlain
	// ChannelTLS — на адресе живёт TLS-демон (нужны пин и сопряжение).
	ChannelTLS
)

// DetectChannel определяет, в каком режиме канала работает демон на addr:
// шлёт plain-HTTP GET /admin/info. Живой plain-демон отвечает валидным HTTP
// (200/401 — не важно: важен сам HTTP-ответ); TLS-listener на plain-запрос
// обрывает соединение без HTTP-ответа. Проба только ЧИТАЕТ режим — никакие
// учётные данные в ней не участвуют, поэтому она безопасна на любом адресе.
//
// Используется лаунчером, чтобы СЛЕДОВАТЬ за демоном: демон явно владеет
// своим режимом (daemon.json), клиент лишь обнаруживает его и подстраивает
// свой профиль (сброс пина при переходе демона на plain — только loopback;
// подсказка о сопряжении при переходе на TLS).
func DetectChannel(addr string) ChannelKind {
	httpc := &http.Client{Timeout: 3 * time.Second}
	resp, err := httpc.Get("http://" + addr + "/admin/info")
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		// net/http-сервер за tls.Listener (наш демон — он самый) отвечает на
		// plain-запрос НЕ обрывом, а честным 400 с этой сигнатурой — самый
		// надёжный признак TLS-канала.
		if resp.StatusCode == http.StatusBadRequest && strings.Contains(string(body), "HTTP request to an HTTPS server") {
			return ChannelTLS
		}
		return ChannelPlain
	}
	// Ошибка plain-запроса: разделяем «порт молчит» и «на порту не-HTTP».
	// Голый TLS-listener без net/http-обёртки рвёт соединение на мусорном
	// ClientHello-ожидании → клиент видит EOF/reset, но не refused.
	//
	// Проверка типизированная, а не по подстроке: текст ошибки зависит от ОС.
	// Windows на закрытый порт пишет «No connection could be made because the
	// target machine actively refused it», где подстроки "connection refused"
	// нет — и мёртвый порт уезжал в ChannelTLS. syscall.ECONNREFUSED един для
	// всех платформ (Go мапит на него WSAECONNREFUSED), а неудача на этапе
	// dial (OpError.Op == "dial") ловит остальные варианты «не достучались»:
	// хост не разрезолвился, сеть недоступна.
	var dnsErr *net.DNSError
	var opErr *net.OpError
	var netErr net.Error
	switch {
	case errors.Is(err, syscall.ECONNREFUSED),
		errors.As(err, &dnsErr),
		errors.As(err, &netErr) && netErr.Timeout(),
		errors.As(err, &opErr) && opErr.Op == "dial":
		return ChannelUnknown
	}
	return ChannelTLS
}
