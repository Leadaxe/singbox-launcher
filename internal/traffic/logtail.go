package traffic

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
)

// LogTailer follows a single growing log file (sing-box.log) and emits
// parsed LogLines on Out(). It survives:
//
//   - rotation (file renamed by logrotate; we reopen on next read attempt)
//   - truncation (file size shrinks; we seek to 0 and continue)
//   - file-not-yet-present (poll for existence at startup; no error)
//
// The implementation uses fsnotify for prompt wake-ups but falls back to a
// short poll on platforms where fsnotify is flaky (Linux + bind mounts,
// macOS + APFS clones). Failures from fsnotify are non-fatal — the polling
// path keeps us correct, just less responsive.
type LogTailer struct {
	path      string
	pollEvery time.Duration

	out chan LogLine
}

// NewLogTailer creates a tailer for the given path. The file does not need
// to exist when this is called — the Run loop polls for it.
func NewLogTailer(path string) *LogTailer {
	return &LogTailer{
		path:      path,
		pollEvery: 500 * time.Millisecond,
		out:       make(chan LogLine, 128),
	}
}

// Out returns the parsed-line channel. Closed when Run returns.
func (t *LogTailer) Out() <-chan LogLine { return t.out }

// warnFn for the tailer. Replaceable via SetTailerWarn so we don't depend
// on debuglog from tests.
var tailerWarnFn = func(format string, args ...any) {}

// SetTailerWarn registers a warning logger; profiler wires this to debuglog.
func SetTailerWarn(fn func(format string, args ...any)) {
	if fn != nil {
		tailerWarnFn = fn
	}
}

// Run blocks until ctx is cancelled. Emits parsed LogLines on Out(). Lines
// not matched by any parser pattern are silently dropped — there are too
// many noise lines (startup banners, periodic stat dumps) to warn about.
func (t *LogTailer) Run(ctx context.Context) {
	defer close(t.out)

	watcher, _ := fsnotify.NewWatcher() // err is non-fatal
	if watcher != nil {
		defer func() { _ = watcher.Close() }()
	}

	var (
		f       *os.File
		reader  *bufio.Reader
		lastIno uint64
		lastSz  int64
	)

	openFile := func() {
		if f != nil {
			_ = f.Close()
			f = nil
		}
		nf, err := os.Open(t.path)
		if err != nil {
			return
		}
		// On rotation we want fresh content, not historical replay. But on
		// initial open we also start at EOF — backfill of *old* events is
		// the rolling buffer's job (which fills as soon as the file
		// produces new lines), not the tailer's.
		if _, err := nf.Seek(0, io.SeekEnd); err != nil {
			tailerWarnFn("traffic tailer: seek end failed: %v", err)
		}
		f = nf
		reader = bufio.NewReader(f)
		if st, err := nf.Stat(); err == nil {
			lastSz = st.Size()
			lastIno = inode(st)
		}
		if watcher != nil {
			_ = watcher.Add(t.path)
		}
	}

	// Initial open attempt — if the file isn't there yet we'll retry on
	// the poll tick.
	openFile()

	poll := time.NewTicker(t.pollEvery)
	defer poll.Stop()

	for {
		// Drain any new lines available right now.
		if reader != nil {
			t.drain(reader)
		}

		var watcherCh <-chan fsnotify.Event
		var errCh <-chan error
		if watcher != nil {
			watcherCh = watcher.Events
			errCh = watcher.Errors
		}

		select {
		case <-ctx.Done():
			if f != nil {
				_ = f.Close()
			}
			return

		case <-poll.C:
			// Rotation/truncation detector. If inode changed or file size
			// shrank, reopen.
			if t.path == "" {
				continue
			}
			st, err := os.Stat(t.path)
			if err != nil {
				// File gone — close and wait. logrotate `copytruncate` may
				// also have removed-then-recreated it.
				if f != nil {
					_ = f.Close()
					f, reader = nil, nil
				}
				continue
			}
			ino := inode(st)
			if f == nil {
				openFile()
				continue
			}
			if ino != lastIno || st.Size() < lastSz {
				openFile()
				continue
			}
			lastSz = st.Size()

		case ev, ok := <-watcherCh:
			if !ok {
				continue
			}
			if ev.Op&(fsnotify.Rename|fsnotify.Remove|fsnotify.Create) != 0 {
				openFile()
			}

		case err, ok := <-errCh:
			if !ok {
				continue
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				tailerWarnFn("traffic tailer: watcher: %v", err)
			}
		}
	}
}

// drain reads lines from r until EOF or a non-EOF error, parses each, and
// emits non-blockingly. Drops on full channel — same justification as the
// poller.
func (t *LogTailer) drain(r *bufio.Reader) {
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			ll, ok := ParseLogLine(line)
			if ok {
				// Use receive time if log line had no parseable timestamp.
				if ll.TS.IsZero() {
					ll.TS = time.Now()
				}
				select {
				case t.out <- ll:
				default:
					tailerWarnFn("traffic tailer: out chan full, dropping line")
				}
			}
		}
		if err != nil {
			return
		}
	}
}
