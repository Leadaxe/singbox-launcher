package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/services"
)

// SPEC 097: строка диагностики машины. Ключевое требование — недоступность
// и нерабочее ядро видно СРАЗУ и по-разному: раньше окно показывало статус
// локального демона под заголовком «Remote», то есть данные не той машины.
func TestRenderRemoteHealth(t *testing.T) {
	for _, tc := range []struct {
		name       string
		in         services.RemoteHealth
		wantSubstr string
		wantImp    widget.Importance
	}{
		{
			name:       "unreachable shows the reason",
			in:         services.RemoteHealth{Err: "dial tcp: connection refused"},
			wantSubstr: "connection refused",
			wantImp:    widget.DangerImportance,
		},
		{
			name:       "unreachable without reason still readable",
			in:         services.RemoteHealth{},
			wantSubstr: "✕",
			wantImp:    widget.DangerImportance,
		},
		{
			name:       "healthy shows core and version",
			in:         services.RemoteHealth{Reachable: true, CoreStatus: "started", Version: "1.14.0-lx.25-rc.1"},
			wantSubstr: "1.14.0-lx.25-rc.1",
			wantImp:    widget.MediumImportance,
		},
		{
			name:       "reachable but core fatal is a warning",
			in:         services.RemoteHealth{Reachable: true, CoreStatus: "fatal"},
			wantSubstr: "fatal",
			wantImp:    widget.WarningImportance,
		},
		{
			name: "apply error surfaces even when core runs",
			in: services.RemoteHealth{
				Reachable: true, CoreStatus: "started", LastError: "bad tun name",
			},
			wantSubstr: "bad tun name",
			wantImp:    widget.WarningImportance,
		},
	} {
		text, imp := renderRemoteHealth(tc.in)
		if !strings.Contains(text, tc.wantSubstr) {
			t.Errorf("%s: %q must contain %q", tc.name, text, tc.wantSubstr)
		}
		if imp != tc.wantImp {
			t.Errorf("%s: importance = %v, want %v", tc.name, imp, tc.wantImp)
		}
	}

	// Версия отсутствует (старый демон без /admin/info) — не повод считать
	// машину сломанной.
	text, imp := renderRemoteHealth(services.RemoteHealth{Reachable: true, CoreStatus: "started"})
	if imp != widget.MediumImportance || !strings.Contains(text, "✓") {
		t.Errorf("missing version must stay healthy, got %q / %v", text, imp)
	}
}
