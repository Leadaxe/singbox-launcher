package platform

import "testing"

// TestGhostTunDecision is the truth-table for the pure WinTun ghost-adapter
// skip/remove predicate extracted from the windows cleanup loop (audit TG4).
func TestGhostTunDecision(t *testing.T) {
	const (
		ourName    = "singbox-tun0"
		otherName  = "Local Area Connection"
		wintunSvc  = "Wintun"
		otherSvc   = "WireGuard"
		statusIdle = uint32(0)         // not DN_STARTED
		statusUp   = uint32(dnStarted) // DN_STARTED set
	)

	cases := []struct {
		name       string
		aggressive bool
		adapter    string
		service    string
		statusOK   bool
		status     uint32
		problem    uint32
		wantRemove bool
		wantReason string
	}{
		// --- phantom-only mode ---
		{
			name:       "phantom: our name + Wintun + idle + phantom -> remove",
			aggressive: false, adapter: ourName, service: wintunSvc,
			statusOK: true, status: statusIdle, problem: cmProbPhantom,
			wantRemove: true,
		},
		{
			name:       "phantom: wrong name prefix -> skip",
			aggressive: false, adapter: otherName, service: wintunSvc,
			statusOK: true, status: statusIdle, problem: cmProbPhantom,
			wantRemove: false, wantReason: "name-prefix-mismatch",
		},
		{
			name:       "phantom: non-Wintun service -> skip",
			aggressive: false, adapter: ourName, service: otherSvc,
			statusOK: true, status: statusIdle, problem: cmProbPhantom,
			wantRemove: false, wantReason: "service-mismatch",
		},
		{
			name:       "phantom: status readback failed -> skip",
			aggressive: false, adapter: ourName, service: wintunSvc,
			statusOK: false, status: statusIdle, problem: cmProbPhantom,
			wantRemove: false, wantReason: "status-readback-failed",
		},
		{
			name:       "phantom: active (DN_STARTED) -> skip",
			aggressive: false, adapter: ourName, service: wintunSvc,
			statusOK: true, status: statusUp, problem: cmProbPhantom,
			wantRemove: false, wantReason: "active-DN_STARTED",
		},
		{
			name:       "phantom: idle but not CM_PROB_PHANTOM -> skip",
			aggressive: false, adapter: ourName, service: wintunSvc,
			statusOK: true, status: statusIdle, problem: 0,
			wantRemove: false, wantReason: "not-phantom",
		},

		// --- aggressive mode ---
		{
			name:       "aggressive: Wintun + idle, non-phantom problem, wrong name -> remove",
			aggressive: true, adapter: otherName, service: wintunSvc,
			statusOK: true, status: statusIdle, problem: 0,
			wantRemove: true,
		},
		{
			name:       "aggressive: non-Wintun service -> skip (guard holds)",
			aggressive: true, adapter: ourName, service: otherSvc,
			statusOK: true, status: statusIdle, problem: cmProbPhantom,
			wantRemove: false, wantReason: "service-mismatch",
		},
		{
			name:       "aggressive: active (DN_STARTED) -> skip (don't steal running tunnel)",
			aggressive: true, adapter: ourName, service: wintunSvc,
			statusOK: true, status: statusUp, problem: 0,
			wantRemove: false, wantReason: "active-DN_STARTED",
		},
		{
			name:       "aggressive: status readback failed -> skip",
			aggressive: true, adapter: ourName, service: wintunSvc,
			statusOK: false, status: statusIdle, problem: 0,
			wantRemove: false, wantReason: "status-readback-failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			remove, reason := ghostTunDecision(tc.aggressive, tc.adapter, tc.service, tc.statusOK, tc.status, tc.problem)
			if remove != tc.wantRemove {
				t.Errorf("remove = %v, want %v (reason=%q)", remove, tc.wantRemove, reason)
			}
			if !remove && reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", reason, tc.wantReason)
			}
			if remove && reason != "" {
				t.Errorf("remove=true should have empty reason, got %q", reason)
			}
		})
	}
}
