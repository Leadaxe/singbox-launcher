package debugapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// logLevelServer spins a server on a fresh port with the given fake facade.
func logLevelServer(t *testing.T, ff *fakeFacade) string {
	t.Helper()
	port := freeLocalPort(t)
	s, err := New(ff, port, "tok")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Start()
	t.Cleanup(s.Stop)
	return "http://127.0.0.1:" + itoa(port)
}

func doLogLevel(t *testing.T, method, base, body string) (*http.Response, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, base+"/state/log-level", rdr)
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// GET reports the stored level plus the default that applies when unset.
func TestGetStateLogLevel(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		base := logLevelServer(t, &fakeFacade{logLevel: "debug", logLevelSet: true})
		resp, body := doLogLevel(t, "GET", base, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if body["level"] != "debug" || body["effective"] != "debug" {
			t.Errorf("level/effective = %v/%v, want debug/debug", body["level"], body["effective"])
		}
		if body["is_set"] != true {
			t.Errorf("is_set = %v, want true", body["is_set"])
		}
	})

	// Unset must not report "" as the level sing-box will use — it falls
	// back to the template default, and a caller reading "effective" needs
	// the real answer.
	t.Run("unset falls back to default", func(t *testing.T) {
		base := logLevelServer(t, &fakeFacade{})
		_, body := doLogLevel(t, "GET", base, "")
		if body["level"] != "" {
			t.Errorf("level = %v, want empty", body["level"])
		}
		if body["is_set"] != false {
			t.Errorf("is_set = %v, want false", body["is_set"])
		}
		if body["effective"] != logLevelDefault {
			t.Errorf("effective = %v, want %s", body["effective"], logLevelDefault)
		}
	})
}

// Every level the template enum allows must be accepted and forwarded to
// core verbatim — including the ones /traffic/verbose can't reach.
func TestPatchStateLogLevel_AllValidLevels(t *testing.T) {
	for _, level := range logLevels {
		t.Run(level, func(t *testing.T) {
			ff := &fakeFacade{}
			base := logLevelServer(t, ff)
			resp, body := doLogLevel(t, "PATCH", base, `{"level":"`+level+`"}`)
			if resp.StatusCode != http.StatusAccepted {
				t.Fatalf("status = %d, want 202", resp.StatusCode)
			}
			if ff.appliedLevel != level {
				t.Errorf("applied = %q, want %q", ff.appliedLevel, level)
			}
			if body["warning"] == nil {
				t.Error("missing connection-reset warning")
			}
		})
	}
}

// A level sing-box doesn't understand would land in config.log.level and
// break the next core start, so it must be rejected before core is touched.
func TestPatchStateLogLevel_RejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"unknown level": `{"level":"verbose"}`,
		"empty string":  `{"level":""}`,
		"wrong case":    `{"level":"DEBUG"}`,
		"missing field": `{}`,
		"empty body":    ``,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			ff := &fakeFacade{}
			base := logLevelServer(t, ff)
			resp, out := doLogLevel(t, "PATCH", base, body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			if ff.appliedLevel != "" {
				t.Errorf("core was mutated with %q on a rejected request", ff.appliedLevel)
			}
			if out["allowed"] == nil {
				t.Error("400 should tell the caller which levels are valid")
			}
		})
	}
}

// Surrounding whitespace is trimmed rather than rejected — a hand-rolled
// curl call shouldn't fail on a stray space.
func TestPatchStateLogLevel_TrimsWhitespace(t *testing.T) {
	ff := &fakeFacade{}
	base := logLevelServer(t, ff)
	resp, _ := doLogLevel(t, "PATCH", base, `{"level":"  trace  "}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	if ff.appliedLevel != "trace" {
		t.Errorf("applied = %q, want trace", ff.appliedLevel)
	}
}

// A core-side failure must surface as 500, not a silent 202 that leaves the
// caller believing the level changed.
func TestPatchStateLogLevel_CoreErrorIs500(t *testing.T) {
	ff := &fakeFacade{applyLevelErr: errors.New("rebuild config: boom")}
	base := logLevelServer(t, ff)
	resp, out := doLogLevel(t, "PATCH", base, `{"level":"info"}`)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if out["error"] == nil {
		t.Error("missing error detail")
	}
}

func TestStateLogLevel_MethodNotAllowed(t *testing.T) {
	base := logLevelServer(t, &fakeFacade{})
	resp, _ := doLogLevel(t, "DELETE", base, "")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

// The level list this endpoint validates against must stay in sync with the
// "log_level" enum in wizard_template.json — a level valid in the wizard but
// rejected here (or vice versa) is a silent divergence.
func TestLogLevelsMatchTemplateEnum(t *testing.T) {
	want := []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}
	if len(logLevels) != len(want) {
		t.Fatalf("logLevels = %v, want %v", logLevels, want)
	}
	for i, l := range want {
		if logLevels[i] != l {
			t.Errorf("logLevels[%d] = %q, want %q", i, logLevels[i], l)
		}
	}
}
