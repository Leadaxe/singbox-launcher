//go:build !darwin
// +build !darwin

package platform

// DiagnoseLanDenial: политика «Локальная сеть» есть только на macOS — на
// остальных платформах сигнатура не диагностируется.
func DiagnoseLanDenial(string) LanDenialVerdict { return LanDenialNotApplicable }
