package core

import (
	"strings"
	"testing"

	"singbox-launcher/core/services"
)

// TestAutoDeployDecision — таблица решений SPEC 164 §3.3–3.4: деплоим только
// изменение, вызванное подпиской, и только на спокойную машину.
func TestAutoDeployDecision(t *testing.T) {
	const req = "1.14.3-lx.17"
	healthy := services.RemoteHealth{
		Reachable: true, CoreStatus: "started", Version: req,
		ActiveSHA: "aaa", LastGoodSHA: "aaa",
	}
	cases := []struct {
		name       string
		before     string
		after      string
		health     func(services.RemoteHealth) services.RemoteHealth
		warnAck    string
		wantOK     bool
		wantReason string
	}{
		{"all quiet", "aaa", "bbb", nil, "", true, ""},
		{"config unchanged", "aaa", "aaa", nil, "", false, "config unchanged"},
		{"manual edits pending", "ccc", "bbb", nil, "", false, "differed from active before refresh"},
		{"core not started", "aaa", "bbb", func(h services.RemoteHealth) services.RemoteHealth {
			h.CoreStatus = "fatal"
			return h
		}, "", false, "core status fatal"},
		{"active is not last-good", "aaa", "bbb", func(h services.RemoteHealth) services.RemoteHealth {
			h.LastGoodSHA = "zzz"
			return h
		}, "", false, "not last-good"},
		{"interrupted apply", "aaa", "bbb", func(h services.RemoteHealth) services.RemoteHealth {
			h.InterruptedApply = true
			return h
		}, "", false, "interrupted"},
		{"old core", "aaa", "bbb", func(h services.RemoteHealth) services.RemoteHealth {
			h.Version = "1.14.3-lx.15"
			return h
		}, "", false, "older than required"},
		{"old core acknowledged", "aaa", "bbb", func(h services.RemoteHealth) services.RemoteHealth {
			h.Version = "1.14.3-lx.15"
			return h
		}, "1.14.3-lx.15", true, ""},
		{"unparsable core version is not older", "aaa", "bbb", func(h services.RemoteHealth) services.RemoteHealth {
			h.Version = "1.14.3-lx.15-test128"
			return h
		}, "", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := healthy
			if c.health != nil {
				h = c.health(h)
			}
			ok, reasons := autoDeployDecision(c.before, c.after, h, c.warnAck, req)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v (reasons: %v)", ok, c.wantOK, reasons)
			}
			if c.wantReason != "" && !strings.Contains(strings.Join(reasons, "; "), c.wantReason) {
				t.Fatalf("reasons %v do not mention %q", reasons, c.wantReason)
			}
			if c.wantOK && len(reasons) != 0 {
				t.Fatalf("ok with reasons %v", reasons)
			}
		})
	}
}
