package traffic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ClashConn is the subset of the Clash /connections JSON object we care
// about. The full response carries more (chain stats, byte/sec totals) but
// for the profiler the per-connection record is enough.
type ClashConn struct {
	ID          string        `json:"id"`
	Metadata    ClashConnMeta `json:"metadata"`
	Upload      int64         `json:"upload"`
	Download    int64         `json:"download"`
	Start       time.Time     `json:"start"`
	Chains      []string      `json:"chains"`
	Rule        string        `json:"rule"`
	RulePayload string        `json:"rulePayload"`
}

// ClashConnMeta — the relevant fields of metadata. Port comes as a string
// in the Clash schema, so we parse on demand.
type ClashConnMeta struct {
	// SourceAddr — адрес КЛИЕНТА (ip:port), инициировавшего соединение.
	//
	// На роутере это единственный способ различить, кто ходит: процесса там
	// нет вовсе (find_process имеет смысл только для трафика самой машины), а
	// клиенты — устройства в локальной сети.
	SourceAddr      string
	Network         string `json:"network"`
	Type            string `json:"type"`
	Host            string `json:"host"`
	DestinationIP   string `json:"destinationIP"`
	DestinationPort string `json:"destinationPort"`
	ProcessPath     string `json:"processPath"`
	Process         string `json:"process"`
}

// PortInt parses the string port field. Returns 0 on failure (the UI handles
// 0 ports gracefully — shows as `:0` which is enough signal that we don't
// know).
func (m ClashConnMeta) PortInt() int {
	if m.DestinationPort == "" {
		return 0
	}
	n, err := strconv.Atoi(m.DestinationPort)
	if err != nil {
		return 0
	}
	return n
}

// ClashConnSnapshot is the polled response. We only need `connections`;
// the totals are visible in the UI elsewhere already (Core dashboard).
type ClashConnSnapshot struct {
	Connections []ClashConn `json:"connections"`
}

// ClashConfigProvider returns the latest Clash API base URL + token. The
// poller calls it once per poll so config reloads (user re-saves wizard)
// take effect without a profiler restart.
type ClashConfigProvider func() (baseURL, token string, enabled bool)

// ConnDelta is what the poller diff'er emits each cycle. Subscribers (the
// profiler) consume the channel and translate each item into 0..N
// TrafficEvents.
type ConnDelta struct {
	Opened []ClashConn           // new connection ids
	Closed []ClashConnClosed     // ids that disappeared since last snapshot
	Bytes  []ClashConnBytesDelta // ids present in both with non-zero up/down delta
	At     time.Time
}

// ClashConnClosed is an open-then-closed connection, carrying the last-seen
// snapshot plus the duration computed from `start`.
type ClashConnClosed struct {
	Conn     ClashConn
	Duration time.Duration
}

// ClashConnBytesDelta captures byte counters delta for one already-tracked
// connection. Total bytes since open live in Conn.Upload / Conn.Download.
type ClashConnBytesDelta struct {
	Conn      ClashConn
	UpDelta   int64
	DownDelta int64
}

// SnapshotFunc returns the current set of open connections keyed by id. ok=false
// means "no source available right now" (core not running / API disabled) —
// the poller resets its diff state so a later start doesn't report every old id
// as "just closed". This is the pluggable seam that lets the poller draw from
// Clash HTTP (classic) OR gRPC SubscribeConnections (daemon) without the
// profiler knowing which. See SetSnapshotFunc.
type SnapshotFunc func(ctx context.Context) (conns map[string]ClashConn, ok bool)

// ConnPoller polls a connection source at 1s cadence and emits ConnDeltas.
// One instance per app lifetime; lives inside TrafficProfiler. The source is
// either the built-in Clash /connections HTTP fetch (default) or an injected
// SnapshotFunc (daemon mode gRPC).
type ConnPoller struct {
	cfg      ClashConfigProvider
	httpc    *http.Client
	interval time.Duration

	// snapshotFn, when non-nil, replaces the Clash HTTP fetch (daemon mode).
	// Guarded by srcMu so it can be swapped when the backend mode changes.
	srcMu      sync.Mutex
	snapshotFn SnapshotFunc

	// state for diff.
	//
	// prevMu защищает prev: пишет его goroutine поллера, а читает UI-поток
	// через Current() — агрегат по клиентам считается из этого снимка.
	prevMu sync.Mutex
	prev   map[string]ClashConn

	out chan ConnDelta
}

// NewConnPoller creates a poller. `cfg` is called every poll. Pass a shared
// http.Client (we reuse api.getHTTPClient() in production) — passing nil
// causes the poller to construct a private one with sane timeouts.
func NewConnPoller(cfg ClashConfigProvider, httpc *http.Client) *ConnPoller {
	if httpc == nil {
		httpc = &http.Client{Timeout: 5 * time.Second}
	}
	return &ConnPoller{
		cfg:      cfg,
		httpc:    httpc,
		interval: time.Second,
		prev:     make(map[string]ClashConn),
		out:      make(chan ConnDelta, 16),
	}
}

// SetSnapshotFunc swaps the connection source at runtime. Non-nil → the poller
// draws snapshots from fn (daemon mode gRPC); nil → back to the Clash HTTP
// fetch (classic). Thread-safe; called when the backend mode changes.
func (p *ConnPoller) SetSnapshotFunc(fn SnapshotFunc) {
	p.srcMu.Lock()
	p.snapshotFn = fn
	p.srcMu.Unlock()
}

func (p *ConnPoller) currentSnapshotFn() SnapshotFunc {
	p.srcMu.Lock()
	defer p.srcMu.Unlock()
	return p.snapshotFn
}

// Out returns the channel that emits one ConnDelta per poll. Buffered so a
// slow consumer doesn't block the poll loop for long, but if it falls more
// than 16 cycles behind we drop deltas (logged via SetWarn).
func (p *ConnPoller) Out() <-chan ConnDelta { return p.out }

// warnFn is set via SetWarn so callers can route log warnings without
// taking a hard dep on debuglog from this package (and break tests).
var pollerWarnFn = func(format string, args ...any) {}

// SetPollerWarn registers a logger for poll-level warnings (drop, fetch
// error). Profiler wires this to debuglog.WarnLog.
func SetPollerWarn(fn func(format string, args ...any)) {
	if fn != nil {
		pollerWarnFn = fn
	}
}

// Run blocks until ctx is cancelled, polling and emitting diffs. Errors
// from the HTTP fetch are logged-and-skipped — a temporarily down sing-box
// is normal, the poller resumes on the next tick.
func (p *ConnPoller) Run(ctx context.Context) {
	defer close(p.out)
	tick := time.NewTicker(p.interval)
	defer tick.Stop()

	// One immediate pull so the UI doesn't wait `interval` before the first
	// open events show up.
	p.pollOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			p.pollOnce(ctx)
		}
	}
}

func (p *ConnPoller) pollOnce(ctx context.Context) {
	// Daemon mode: an injected snapshot source (gRPC SubscribeConnections)
	// replaces the Clash HTTP fetch. Classic keeps the HTTP path unchanged.
	if fn := p.currentSnapshotFn(); fn != nil {
		curr, ok := fn(ctx)
		if !ok {
			if p.snapshotLen() > 0 {
				p.setPrev(make(map[string]ClashConn))
			}
			return
		}
		p.emit(p.diff(curr, time.Now()), curr)
		return
	}

	baseURL, token, enabled := p.cfg()
	if !enabled || baseURL == "" {
		// sing-box not running or Clash API disabled — reset state so a
		// fresh start later doesn't think all old ids "just closed".
		if p.snapshotLen() > 0 {
			p.setPrev(make(map[string]ClashConn))
		}
		return
	}
	snap, err := fetchSnapshot(ctx, p.httpc, baseURL, token)
	if err != nil {
		pollerWarnFn("traffic poller: fetch /connections failed: %v", err)
		return
	}
	now := time.Now()
	curr := make(map[string]ClashConn, len(snap.Connections))
	for _, c := range snap.Connections {
		curr[c.ID] = c
	}
	p.emit(p.diff(curr, now), curr)
}

// emit records the new snapshot as prev and pushes the delta to the out
// channel (non-blocking; drops on backlog). Shared by the Clash HTTP path and
// the injected-source (daemon gRPC) path.
func (p *ConnPoller) emit(delta ConnDelta, curr map[string]ClashConn) {
	p.setPrev(curr)
	select {
	case p.out <- delta:
	default:
		// Drop rather than block — a stuck UI thread shouldn't wedge the
		// poller. We're at 16 deltas backlog; that's 16s of data.
		pollerWarnFn("traffic poller: out chan full, dropping delta (%d open / %d closed / %d byte updates)",
			len(delta.Opened), len(delta.Closed), len(delta.Bytes))
	}
}

func (p *ConnPoller) diff(curr map[string]ClashConn, now time.Time) ConnDelta {
	d := ConnDelta{At: now}
	// Снимок под мьютексом: тот же prev читает UI-поток через Current().
	prev := p.Current()
	for id, c := range curr {
		old, was := prev[id]
		if !was {
			d.Opened = append(d.Opened, c)
			continue
		}
		if c.Upload != old.Upload || c.Download != old.Download {
			d.Bytes = append(d.Bytes, ClashConnBytesDelta{
				Conn:      c,
				UpDelta:   c.Upload - old.Upload,
				DownDelta: c.Download - old.Download,
			})
		}
	}
	for id, old := range prev {
		if _, still := curr[id]; still {
			continue
		}
		dur := time.Duration(0)
		if !old.Start.IsZero() {
			dur = now.Sub(old.Start)
		}
		d.Closed = append(d.Closed, ClashConnClosed{Conn: old, Duration: dur})
	}
	return d
}

// fetchSnapshot performs the actual GET /connections. Kept out of the
// struct so it's trivially mockable from tests.
func fetchSnapshot(ctx context.Context, httpc *http.Client, baseURL, token string) (ClashConnSnapshot, error) {
	url := strings.TrimRight(baseURL, "/") + "/connections"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ClashConnSnapshot{}, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return ClashConnSnapshot{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return ClashConnSnapshot{}, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var snap ClashConnSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return ClashConnSnapshot{}, fmt.Errorf("decode: %w", err)
	}
	return snap, nil
}

// Current возвращает копию последнего снимка соединений.
//
// Нужен агрегату по клиентам: считать байты по кольцевому буферу событий
// нельзя — там открытия и закрытия, а не текущие итоги, и цифры разошлись бы
// с реальностью на первом же длинном соединении.
func (c *ConnPoller) Current() map[string]ClashConn {
	c.prevMu.Lock()
	defer c.prevMu.Unlock()
	out := make(map[string]ClashConn, len(c.prev))
	for k, v := range c.prev {
		out[k] = v
	}
	return out
}

func (c *ConnPoller) setPrev(m map[string]ClashConn) {
	c.prevMu.Lock()
	c.prev = m
	c.prevMu.Unlock()
}

func (c *ConnPoller) snapshotLen() int {
	c.prevMu.Lock()
	defer c.prevMu.Unlock()
	return len(c.prev)
}
