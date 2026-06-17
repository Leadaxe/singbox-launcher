//go:build !darwin

package core

// initDarwinTLSRoots is a no-op on non-darwin platforms. On darwin it installs
// a custom root pool to avoid the macOS 12+ system certificate verifier (see
// tls_roots_darwin.go).
func initDarwinTLSRoots() {}
