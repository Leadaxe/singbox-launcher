package ui

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2/theme"

	"singbox-launcher/core"
	"singbox-launcher/internal/locale"
)

// Маркер канала к ЛОКАЛЬНОМУ демону в строке «Core Status».
//
// Раньше на вкладке Local в daemon-режиме при остановленном ядре всплывал
// диалог «ядро ещё не запущено». Это штатное состояние, а не сбой: модальным
// окном оно било по рукам при каждом входе на вкладку. Состояние канала
// показывает кружок — тот же, что у машин на вкладке Remote, чтобы «связь есть
// / моргает / нет» читалось одинаково в обеих половинах приложения.
//
// Источник состояния — уже существующий стрим статуса демона
// (DaemonBackend.superviseStatus), а не отдельный опрос: индикатор не добавляет
// в сеть ни одного запроса.

// daemonMarkerFor — состояние маркера локального демона по состоянию канала.
// Зеркалит markerFor у машин: серый — ещё не связывались, жёлтый — промахи без
// вердикта, красный — устойчиво не отвечает ИЛИ ответил, но ядро в FATAL.
func daemonMarkerFor(link core.DaemonLinkState) markerState {
	if !link.EverConnected {
		return markerIdle
	}
	switch {
	case link.FailStreak >= heartbeatFailThreshold:
		return markerDown
	case link.FailStreak > 0:
		return markerFlaky
	}
	// Демон отвечает, но ответ может быть плохой новостью: ядро внутри упало.
	// Зелёный тут значил бы «всё хорошо» ровно там, где всё плохо.
	if link.CoreFatal {
		return markerDown
	}
	return markerLive
}

// daemonMarkerTooltip — подсказка маркера словами (цвет сам себя не объясняет
// и не читается теми, кто различает оттенки хуже).
//
// Зелёный различает запущенное и остановленное ядро: канал в обоих случаях
// в порядке, но подсказка при остановленном ядре — единственное место, где
// вместо исчезнувшего диалога сказано, что делать (нажать Start).
func daemonMarkerTooltip(state markerState, link core.DaemonLinkState, coreRunning bool) string {
	switch state {
	case markerLive:
		if coreRunning {
			return locale.T("The daemon answers; the core is running. Click for connection settings.")
		}
		return locale.T("The daemon is paired and answers; the core is stopped. Press Start to bring the VPN up.")
	case markerFlaky:
		return locale.T("The daemon stopped answering — reconnecting. Click for connection settings.")
	case markerDown:
		if link.CoreFatal {
			return fmt.Sprintf(locale.T("The core inside the daemon failed: %s. Click for connection settings."), link.FatalErr)
		}
		return locale.T("The daemon is not answering. Check that the service is running (⚙).")
	default:
		return locale.T("Connecting to the daemon…")
	}
}

// updateDaemonMarker приводит кружок к состоянию канала. В classic-режиме
// DaemonLink отдаёт ok=false — маркера в строке нет вовсе.
func (tab *CoreDashboardTab) updateDaemonMarker(coreRunning bool) {
	// Маркер создаётся в createStatusRow; в тестах панель бывает без него.
	if tab == nil || tab.daemonMarker == nil || tab.daemonMarkerBox == nil || tab.controller == nil {
		return
	}
	link, ok := tab.controller.DaemonLink()
	if !ok {
		tab.daemonMarkerBox.Hide()
		return
	}
	tab.daemonMarkerBox.Show()
	state := daemonMarkerFor(link)
	var markerColor color.Color = theme.Color(theme.ColorNameDisabled)
	switch state {
	case markerLive:
		markerColor = theme.Color(theme.ColorNameSuccess)
	case markerFlaky:
		markerColor = theme.Color(theme.ColorNameWarning)
	case markerDown:
		markerColor = theme.Color(theme.ColorNameError)
	}
	tab.daemonMarker.setState(markerColor, daemonMarkerTooltip(state, link, coreRunning))
}
