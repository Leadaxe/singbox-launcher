package core

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os/exec"
)

// macOS 11 (Big Sur) compatibility:
//
// Go 1.25's crypto/x509 verifies TLS certificates on darwin via the system
// API SecTrustCopyCertificateChain, which only exists on macOS 12+. On Big Sur
// the symbol is missing and the process aborts on the first HTTPS request
// (dyld: Symbol not found: _SecTrustCopyCertificateChain).
//
// To avoid the system verifier entirely we build a root pool from the system
// keychains and install it as TLSClientConfig.RootCAs everywhere. With a
// non-system Roots pool, crypto/x509 uses the pure-Go verifier and never calls
// SecTrust*. We patch BOTH http.DefaultTransport (covers bare &http.Client{}
// callsites, e.g. UI dialogs) AND the launcher's defaultSharedTransport.
//
// This must be paired with a build that targets macOS 11.0 via the external
// linker (-linkmode=external -extldflags=-mmacosx-version-min=11.0); otherwise
// the missing symbol is bound eagerly and the process crashes before main().
//
// init() (not lazy) guarantees the pool is in place before any goroutine —
// including UI/auto-update workers started early — makes its first request.

func init() {
	pool := darwinSystemRootPool()
	if pool == nil {
		// Could not load any roots; leave verification as-is. The pure-Go
		// verifier with no custom roots would reject everything, so it is
		// safer to do nothing and let the (broken) system path run than to
		// silently break all TLS. In practice the keychains always parse.
		return
	}
	tlsConf := &tls.Config{RootCAs: pool}

	// Patch the launcher's shared transport.
	defaultSharedTransport.TLSClientConfig = tlsConf

	// Patch the process-wide default transport so that bare http.Client{}
	// (no explicit Transport) also uses the custom roots.
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		clone := dt.Clone()
		clone.TLSClientConfig = tlsConf
		http.DefaultTransport = clone
	}
}

// darwinSystemRootPool loads trusted roots from the macOS system keychains
// using the `security` CLI (no cgo, no SecTrust API). Returns nil if no roots
// could be parsed.
func darwinSystemRootPool() *x509.CertPool {
	pool := x509.NewCertPool()
	loaded := false
	for _, keychain := range []string{
		"/System/Library/Keychains/SystemRootCertificates.keychain",
		"/Library/Keychains/System.keychain",
	} {
		out, err := exec.Command("/usr/bin/security", "find-certificate", "-a", "-p", keychain).Output()
		if err != nil || len(out) == 0 {
			continue
		}
		if pool.AppendCertsFromPEM(out) {
			loaded = true
		}
	}
	if !loaded {
		return nil
	}
	return pool
}

// initDarwinTLSRoots is retained for the explicit call in CreateHTTPClient but
// is now a no-op: the work happens in init(). Kept to avoid touching the
// cross-platform call site.
func initDarwinTLSRoots() {}
