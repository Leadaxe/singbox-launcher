package ui

import (
	"strings"

	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	lanDenialSuspectedText = "macOS is blocking the launcher's access to the local network: the system rejects the connection instantly, before it even leaves this Mac. Enable the launcher in System Settings → Privacy & Security → Local Network. If it is already enabled there, the app bundle's code signature is broken (typical after swapping a dev binary into the installed app) — reinstall the launcher."
	lanDenialRecoveredText = "The machine is answering again — the failure was transient (macOS re-checked its local-network policy or the network blinked). Press Connect again."
)

// lanDenialHint — локализованная подсказка по вердикту диагностики; пустая
// строка, когда сигнатура не подтвердилась и общий текст «машина не отвечает»
// остаётся правильным.
func lanDenialHint(v platform.LanDenialVerdict) string {
	switch v {
	case platform.LanDenialSuspected:
		return locale.T(lanDenialSuspectedText)
	case platform.LanDenialRecovered:
		return locale.T(lanDenialRecoveredText)
	}
	return ""
}

// diagnoseLanDenialFromErr прогоняет платформенную диагностику, если ошибка
// похожа на «no route to host» и из её текста извлекается адрес назначения.
// Внутри контрольный dial с таймаутом — не звать на UI-потоке.
func diagnoseLanDenialFromErr(err error) platform.LanDenialVerdict {
	if err == nil || !strings.Contains(err.Error(), "no route to host") {
		return platform.LanDenialNotApplicable
	}
	hostport := platform.HostPortFromDialErr(err.Error())
	if hostport == "" {
		return platform.LanDenialNotApplicable
	}
	return platform.DiagnoseLanDenial(hostport)
}
