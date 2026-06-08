package subscription

// isValidShadowsocksMethod checks if the encryption method is supported by sing-box
// This prevents invalid methods (like binary data) from causing sing-box to crash
// Only methods supported by sing-box are allowed (see sing-box documentation)
func isValidShadowsocksMethod(method string) bool {
	validMethods := map[string]bool{
		// 2022 edition (modern, best security)
		"2022-blake3-aes-128-gcm":       true,
		"2022-blake3-aes-256-gcm":       true,
		"2022-blake3-chacha20-poly1305": true,
		// AEAD ciphers
		"none":                    true,
		"aes-128-gcm":             true,
		"aes-192-gcm":             true,
		"aes-256-gcm":             true,
		"chacha20-ietf-poly1305":  true,
		"xchacha20-ietf-poly1305": true,
	}
	return validMethods[method]
}
