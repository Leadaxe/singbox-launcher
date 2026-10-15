// File openconnect_status.go — состояние endpoint'ов openconnect у
// работающего ядра (SPEC 167 §2.5): доменные типы, конвертер pb → домен,
// кеш стрима SubscribeOpenConnectStatus и разовые вызовы Submit/Cancel.
//
// Живёт в services по той же причине, что tailscale_status.go: транспорт
// удалённой машины (этот пакет) держит свой стрим и кеш, а core
// импортирует services — обратная зависимость дала бы цикл. Конвертер и
// RPC-обёртки ОДНИ на оба пути: локальный демон и роутер обязаны вести себя
// одинаково.
package services

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	daemonpb "singbox-launcher/internal/daemonpb"
)

// Состояния endpoint'а словами ядра (строки, enum в proto нет).
const (
	OpenConnectStateAuthPending = "auth-pending"
	OpenConnectStateConnected   = "connected"
	OpenConnectStateError       = "error"
)

// OpenConnectFormChoice — вариант поля-выбора формы ядра.
type OpenConnectFormChoice struct {
	Value string
	Label string
}

// OpenConnectFormField — поле формы второго фактора как его отдал шлюз.
type OpenConnectFormField struct {
	SubmissionKey string
	Name          string
	Label         string
	Kind          string
	Value         string
	Options       []OpenConnectFormChoice
}

// Key — ключ значения в ответе: SubmissionKey, иначе Name.
func (f OpenConnectFormField) Key() string {
	if f.SubmissionKey != "" {
		return f.SubmissionKey
	}
	return f.Name
}

// OpenConnectBrowserRequest — запрос входа через браузер (SSO).
type OpenConnectBrowserRequest struct {
	URL                 string
	FinalURL            string
	CookieNames         []string
	HeaderNames         []string
	CallbackURLPrefixes []string
	EarlyCookieNames    []string
}

// OpenConnectChallenge — висящий челлендж ядра: ровно одно из Form/Browser.
type OpenConnectChallenge struct {
	ID      string
	Banner  string
	Message string
	Error   string
	// Form — поля формы; nil, если челлендж браузерный.
	Form []OpenConnectFormField
	// IsForm отличает пустую форму от браузерного челленджа.
	IsForm  bool
	Browser *OpenConnectBrowserRequest
}

// OpenConnectStatus — состояние одного endpoint'а openconnect.
type OpenConnectStatus struct {
	Tag       string
	State     string
	StateText string
	Error     string
	// Challenge — nil, если ядро ничего не ждёт.
	Challenge *OpenConnectChallenge
}

// PendingChallengeID — id висящего челленджа или "".
func (s OpenConnectStatus) PendingChallengeID() string {
	if s.Challenge == nil {
		return ""
	}
	return s.Challenge.ID
}

// OpenConnectStatusesFromPB — снимок стрима по тегам.
func OpenConnectStatusesFromPB(upd *daemonpb.OpenConnectStatusUpdate) map[string]OpenConnectStatus {
	out := make(map[string]OpenConnectStatus, len(upd.GetEndpoints()))
	for _, ep := range upd.GetEndpoints() {
		tag := ep.GetEndpointTag()
		if tag == "" {
			continue
		}
		out[tag] = OpenConnectStatus{
			Tag:       tag,
			State:     ep.GetState(),
			StateText: ep.GetStateText(),
			Error:     ep.GetError(),
			Challenge: openConnectChallengeFromPB(ep.GetAuthChallenge()),
		}
	}
	return out
}

func openConnectChallengeFromPB(ch *daemonpb.OpenConnectAuthChallenge) *OpenConnectChallenge {
	if ch == nil || ch.GetId() == "" {
		return nil
	}
	out := &OpenConnectChallenge{
		ID:      ch.GetId(),
		Banner:  ch.GetBanner(),
		Message: ch.GetMessage(),
		Error:   ch.GetError(),
	}
	if form := ch.GetForm(); form != nil {
		out.IsForm = true
		for _, f := range form.GetFields() {
			field := OpenConnectFormField{
				SubmissionKey: f.GetSubmissionKey(),
				Name:          f.GetName(),
				Label:         f.GetLabel(),
				Kind:          f.GetKind(),
				Value:         f.GetValue(),
			}
			for _, o := range f.GetOptions() {
				field.Options = append(field.Options, OpenConnectFormChoice{Value: o.GetValue(), Label: o.GetLabel()})
			}
			out.Form = append(out.Form, field)
		}
	}
	if br := ch.GetBrowser(); br != nil {
		out.Browser = &OpenConnectBrowserRequest{
			URL:                 br.GetUrl(),
			FinalURL:            br.GetFinalURL(),
			CookieNames:         br.GetCookieNames(),
			HeaderNames:         br.GetHeaderNames(),
			CallbackURLPrefixes: br.GetCallbackURLPrefixes(),
			EarlyCookieNames:    br.GetEarlyCookieNames(),
		}
	}
	return out
}

// OpenConnectNotify — колбэк кеша на новый снимок: newChallenges — статусы
// с челленджем, id которого в прежнем снимке у этого тега не было; changed —
// изменилось ли что-то видимое в строке узла (состояние, ошибка, челлендж).
// Зовётся с горутины стрима, вне блокировки кеша.
type OpenConnectNotify func(newChallenges []OpenConnectStatus, changed bool)

// OpenConnectStatusCache — кеш последнего снимка, общий для обоих
// gRPC-путей (локальный демон и транспорт машины). Правила те же, что у
// TailscaleStatusCache: обрыв стрима снимок не чистит, только живость.
type OpenConnectStatusCache struct {
	mu          sync.Mutex
	byTag       map[string]OpenConnectStatus
	live        bool
	unsupported bool
	notify      OpenConnectNotify
}

// Apply — новый снимок целиком заменяет прежний.
func (c *OpenConnectStatusCache) Apply(upd *daemonpb.OpenConnectStatusUpdate) {
	snap := OpenConnectStatusesFromPB(upd)
	c.mu.Lock()
	prev := c.byTag
	c.byTag = snap
	c.live = true
	cb := c.notify
	c.mu.Unlock()

	var fresh []OpenConnectStatus
	changed := len(prev) != len(snap)
	for tag, st := range snap {
		old, had := prev[tag]
		if id := st.PendingChallengeID(); id != "" && (!had || old.PendingChallengeID() != id) {
			fresh = append(fresh, st)
		}
		if !had || old.State != st.State || old.Error != st.Error || old.PendingChallengeID() != st.PendingChallengeID() {
			changed = true
		}
	}
	if cb != nil && (changed || len(fresh) > 0) {
		cb(fresh, changed)
	}
}

// MarkDead — стрим оборвался; снимок остаётся, живость снимается.
func (c *OpenConnectStatusCache) MarkDead() {
	c.mu.Lock()
	c.live = false
	c.mu.Unlock()
}

// MarkUnsupported — ядро без RPC openconnect (Unimplemented): переподписка
// не нужна, стрим не поднимается до смены инстанса бэкенда/транспорта.
func (c *OpenConnectStatusCache) MarkUnsupported() {
	c.mu.Lock()
	c.unsupported = true
	c.live = false
	c.mu.Unlock()
}

// Unsupported — получен ли Unimplemented.
func (c *OpenConnectStatusCache) Unsupported() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.unsupported
}

// Get — статус по тегу. ok=false — тега в снимке нет.
func (c *OpenConnectStatusCache) Get(tag string) (OpenConnectStatus, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, ok := c.byTag[tag]
	return st, ok
}

// Pending — все статусы с висящим челленджем (показ при подписке UI).
func (c *OpenConnectStatusCache) Pending() []OpenConnectStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []OpenConnectStatus
	for _, st := range c.byTag {
		if st.PendingChallengeID() != "" {
			out = append(out, st)
		}
	}
	return out
}

// Live — пришёл ли хотя бы один кадр и не оборван ли стрим.
func (c *OpenConnectStatusCache) Live() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.live
}

// SetNotify — колбэк на новые снимки (перезаписывает прежний).
func (c *OpenConnectStatusCache) SetNotify(fn OpenConnectNotify) {
	c.mu.Lock()
	c.notify = fn
	c.mu.Unlock()
}

// IsUnimplementedRPC — ядро без реализации RPC: повтором не лечится.
func IsUnimplementedRPC(err error) bool {
	return status.Code(err) == codes.Unimplemented
}

// OpenConnectSubmitFormRPC — ответ формы на челлендж.
func OpenConnectSubmitFormRPC(ctx context.Context, client daemonpb.StartedServiceClient, tag, challengeID string, values map[string]string) error {
	_, err := client.SubmitOpenConnectAuthResponse(ctx, &daemonpb.OpenConnectAuthResponseSubmission{
		EndpointTag: tag,
		ChallengeID: challengeID,
		Response: &daemonpb.OpenConnectAuthResponseSubmission_Form{
			Form: &daemonpb.OpenConnectAuthFormResponse{Values: values},
		},
	})
	if err != nil {
		return fmt.Errorf("SubmitOpenConnectAuthResponse: %w", err)
	}
	return nil
}

// OpenConnectSubmitBrowserRPC — URL редиректа SSO на челлендж браузера.
func OpenConnectSubmitBrowserRPC(ctx context.Context, client daemonpb.StartedServiceClient, tag, challengeID, finalURL string) error {
	_, err := client.SubmitOpenConnectAuthResponse(ctx, &daemonpb.OpenConnectAuthResponseSubmission{
		EndpointTag: tag,
		ChallengeID: challengeID,
		Response: &daemonpb.OpenConnectAuthResponseSubmission_Browser{
			Browser: &daemonpb.OpenConnectBrowserResult{FinalURL: finalURL},
		},
	})
	if err != nil {
		return fmt.Errorf("SubmitOpenConnectAuthResponse: %w", err)
	}
	return nil
}

// OpenConnectCancelRPC — отмена челленджа: ядро начинает новый логин
// (новый код от шлюза) через 1 с с бэкоффом.
func OpenConnectCancelRPC(ctx context.Context, client daemonpb.StartedServiceClient, tag, challengeID string) error {
	_, err := client.CancelOpenConnectAuthChallenge(ctx, &daemonpb.OpenConnectAuthChallengeCancel{
		EndpointTag: tag,
		ChallengeID: challengeID,
	})
	if err != nil {
		return fmt.Errorf("CancelOpenConnectAuthChallenge: %w", err)
	}
	return nil
}
