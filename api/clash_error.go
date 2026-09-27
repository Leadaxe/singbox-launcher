package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"singbox-launcher/internal/platform"
)

// ErrPlatformInterrupt is returned when a request is aborted due to system sleep (platform cancelled the context).
var ErrPlatformInterrupt = errors.New("platform: interrupt")

// ErrAPIAuth is wrapped into errors that mean "the core rejected our
// credentials". 401 and 403 are the core's own verdict and must be told apart
// from a refused connection or a wrong address: with a secret the launcher can
// offer to re-sync it, and with an unauthenticated loopback config a 401 means
// the config the launcher read is not the one the core is running (SPEC 143).
var ErrAPIAuth = errors.New("clash api: authentication failed")

// IsAuthError reports whether err means the core rejected our credentials.
func IsAuthError(err error) bool { return errors.Is(err, ErrAPIAuth) }

// authStatusError returns an ErrAPIAuth-wrapped error for 401/403, else nil.
func authStatusError(statusCode int) error {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w (HTTP %d)", ErrAPIAuth, statusCode)
	}
	return nil
}

// requestContext returns the platform power context for an outgoing request, or ErrPlatformInterrupt if the system is sleeping.
func requestContext() (context.Context, error) {
	if platform.IsSleeping() {
		return nil, ErrPlatformInterrupt
	}
	return platform.PowerContext(), nil
}

// normalizeRequestError maps context.Canceled (e.g. sleep) to ErrPlatformInterrupt; other errors unchanged.
func normalizeRequestError(err error) error {
	if err != nil && errors.Is(err, context.Canceled) {
		return ErrPlatformInterrupt
	}
	return err
}

// classifyRequestError maps a non-nil HTTP request error to a user-facing error.
// Order of precedence (identical across all Clash API request sites):
//  1. context.Canceled (e.g. system sleep) → ErrPlatformInterrupt.
//  2. net.Error timeout → "network timeout: connection timed out".
//  3. *net.OpError on "dial" → "network error: cannot connect to server".
//  4. fallback: fmt.Errorf(fallbackFormat, err), where fallbackFormat must contain a %w verb.
//
// Callers wrap the returned error in their own return statement (arity differs per call).
func classifyRequestError(err error, fallbackFormat string) error {
	if e := normalizeRequestError(err); e != err {
		return e
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return fmt.Errorf("network timeout: connection timed out")
	}
	if opErr, ok := err.(*net.OpError); ok && opErr.Op == "dial" {
		return fmt.Errorf("network error: cannot connect to server")
	}
	return fmt.Errorf(fallbackFormat, err)
}
