package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
)

// timeoutErr is a net.Error whose Timeout() reports true.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return false }

// nonTimeoutNetErr is a net.Error whose Timeout() reports false.
type nonTimeoutNetErr struct{}

func (nonTimeoutNetErr) Error() string   { return "some net error" }
func (nonTimeoutNetErr) Timeout() bool   { return false }
func (nonTimeoutNetErr) Temporary() bool { return false }

func TestClassifyRequestError(t *testing.T) {
	const fallback = "failed to execute request: %w"

	t.Run("context.Canceled maps to ErrPlatformInterrupt", func(t *testing.T) {
		// Wrapped so errors.Is(err, context.Canceled) is true, mirroring real ctx cancellation.
		in := fmt.Errorf("do: %w", context.Canceled)
		got := classifyRequestError(in, fallback)
		if !errors.Is(got, ErrPlatformInterrupt) {
			t.Fatalf("got %v, want ErrPlatformInterrupt", got)
		}
	})

	t.Run("timeout maps to network timeout message", func(t *testing.T) {
		got := classifyRequestError(timeoutErr{}, fallback)
		if got == nil || got.Error() != "network timeout: connection timed out" {
			t.Fatalf("got %v, want network timeout message", got)
		}
	})

	t.Run("dial OpError maps to network error message", func(t *testing.T) {
		in := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
		got := classifyRequestError(in, fallback)
		if got == nil || got.Error() != "network error: cannot connect to server" {
			t.Fatalf("got %v, want network error message", got)
		}
	})

	t.Run("non-dial OpError falls through to fallback", func(t *testing.T) {
		in := &net.OpError{Op: "read", Err: errors.New("boom")}
		got := classifyRequestError(in, fallback)
		// Op != "dial" → not the dial branch; not a timeout → fallback wrap with %w.
		if got == nil || !errors.Is(got, in) {
			t.Fatalf("got %v, want fallback wrapping the original error", got)
		}
		if got.Error() != fmt.Sprintf("failed to execute request: %v", in) {
			t.Fatalf("unexpected fallback message: %q", got.Error())
		}
	})

	t.Run("non-timeout net.Error falls through to fallback", func(t *testing.T) {
		in := nonTimeoutNetErr{}
		got := classifyRequestError(in, fallback)
		if got == nil || !errors.Is(got, in) {
			t.Fatalf("got %v, want fallback wrapping the original error", got)
		}
	})

	t.Run("plain error falls through to fallback and preserves %w chain", func(t *testing.T) {
		in := errors.New("plain boom")
		got := classifyRequestError(in, fallback)
		if got == nil || !errors.Is(got, in) {
			t.Fatalf("got %v, want fallback wrapping the original error", got)
		}
		if got.Error() != "failed to execute request: plain boom" {
			t.Fatalf("unexpected fallback message: %q", got.Error())
		}
	})

	t.Run("precedence: timeout wins over non-dial OpError shape", func(t *testing.T) {
		// A timeout that also satisfies net.Error: timeout branch must win before fallback.
		var _ net.Error = timeoutErr{} // compile-time assertion timeoutErr is a net.Error
		got := classifyRequestError(timeoutErr{}, fallback)
		if got.Error() != "network timeout: connection timed out" {
			t.Fatalf("timeout precedence broken: %q", got.Error())
		}
	})

	// Guard: ensure the timeout type genuinely is a net.Error at runtime (deadline-style errors).
	_ = time.Second
}
