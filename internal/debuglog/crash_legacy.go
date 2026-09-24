//go:build !go1.23

package debuglog

// EnableCrashOutput — заглушка для тулчейна до Go 1.23 (Win7-сборка на go1.21):
// runtime/debug.SetCrashOutput там нет, паника уходит только в stderr.
// Возвращает nil, чтобы не шуметь предупреждением на каждом старте.
func EnableCrashOutput(string) error { return nil }

// releaseCrashOutput — заглушка: без SetCrashOutput отпускать нечего.
func releaseCrashOutput() {}
