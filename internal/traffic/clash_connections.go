package traffic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ClashConn is the subset of the Clash /connections JSON object we care
// about. The full response carries more (chain stats, byte/sec totals) but
// for the profiler the per-connection record is enough.
type ClashConn struct {
	ID       string         `json:"id"`
	Metadata ClashConnMeta  `json:"metadata"`
	Upload   int64          `json:"upload"`
	Download int64          `json:"download"`
	Start    time.Time      `json:"start"`
	Chains   []string       `json:"chains"`
	Rule     string         `json:"rule"`
	RulePayload string      `json:"rulePayload"`
}

// ClashConnMeta — the relevant fields of metadata. Port comes as a string
// in the Clash schema, so we parse on demand.
type ClashConnMeta struct {
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
	Opened []ClashConn          // new connection ids
	Closed []ClashConnClosed    // ids that disappeared since last snapshot
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
	Conn       ClashConn
	UpDelta    int64
	DownDelta  int64
}

// ConnPoller polls Clash /connections at 1s cadence and emits ConnDeltas.
// One instance per app lifetime; lives inside TrafficProfiler.
type ConnPoller struct {
	cfg      ClashConfigProvider
	httpc    *http.Client
	interval time.Duration

	// state for diff
	prev map[string]ClashConn

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

// Out returns the channel that emits one ConnDelta per poll. Buffered so a
// slow consumer doesn't block the poll loop for long, but if it falls more
// than 16 cycles behind we drop deltas (logged via SetWarn).
func (p *ConnPoller) Out() <-chan ConnDelta { return p.out }

// SetInterval overrides the default 1s poll cadence. Mainly for tests.
func (p *ConnPoller) SetInterval(d time.Duration) {
	if d > 0 {
		p.interval = d
	}
}

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
	baseURL, token, enabled := p.cfg()
	if !enabled || baseURL == "" {
		// sing-box not running or Clash API disabled — reset state so a
		// fresh start later doesn't think all old ids "just closed".
		if len(p.prev) > 0 {
			p.prev = make(map[string]ClashConn)
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
	delta := p.diff(curr, now)
	p.prev = curr
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
	for id, c := range curr {
		old, was := p.prev[id]
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
	for id, old := range p.prev {
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
