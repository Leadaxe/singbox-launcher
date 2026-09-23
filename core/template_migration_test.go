package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// refreshedTemplate — the smallest body template.ParseTemplateData accepts:
// what the pinned download returns in these tests.
const refreshedTemplate = `{"parser_config": {}, "config": {"outbounds": [], "route": {"final": "direct"}}, "params": [], "vars": []}`

// installedTemplate — the file already on disk. The refresh never parses it,
// so any content marks "untouched".
const installedTemplate = `{"installed": "by an older launcher"}`

// withAppVersion temporarily overrides constants.AppVersion for a test scope.
func withAppVersion(t *testing.T, v string, fn func()) {
	t.Helper()
	prev := constants.AppVersion
	constants.AppVersion = v
	t.Cleanup(func() { constants.AppVersion = prev })
	fn()
}

type templateFetchStub struct {
	body   string
	status int
	err    error
	calls  int
}

func (f *templateFetchStub) fetch(context.Context, string, time.Duration) ([]byte, int, error) {
	f.calls++
	if f.err != nil {
		return nil, 0, f.err
	}
	return []byte(f.body), f.status, nil
}

type launcherDir struct {
	root string
	t    *testing.T
}

func newLauncherDir(t *testing.T) launcherDir {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	return launcherDir{root: root, t: t}
}

func (d launcherDir) write(rel, body string) {
	d.t.Helper()
	p := filepath.Join(d.root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		d.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		d.t.Fatal(err)
	}
}

// template returns the installed template, "" when there is no file.
func (d launcherDir) template() string {
	d.t.Helper()
	raw, err := os.ReadFile(filepath.Join(d.root, "bin", constants.WizardTemplateFileName))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		d.t.Fatal(err)
	}
	return string(raw)
}

func (d launcherDir) marker() string {
	return locale.LoadSettings(filepath.Join(d.root, "bin")).LastTemplateLauncherVersion
}

// TestRefreshTemplateIfStale — every outcome of the startup refresh. The
// invariant behind the table: the installed template is replaced only by a
// downloaded template that parses, and is never deleted or overwritten by a
// failed fetch (the old invalidation removed it and left the launcher without
// a template until someone pressed Download).
func TestRefreshTemplateIfStale(t *testing.T) {
	ok := func() *templateFetchStub { return &templateFetchStub{body: refreshedTemplate, status: http.StatusOK} }
	cases := []struct {
		name         string
		version      string // AppVersion, "v0.8.8" when empty
		marker       string // last_template_launcher_version; "" = legacy install
		installed    string // template on disk; "" = no file
		bundled      string // bin/wizard_template.version
		shipped      string // split layout: App/bin/wizard_template.json ("" = portable, App == Data)
		shippedMark  string // split layout: App/bin/wizard_template.version
		dataRoot     string // config_data_root in settings.json; "@" = the current Data
		state        bool
		fetch        *templateFetchStub
		wantTemplate string
		wantMarker   string
		wantCalls    int
		wantErr      bool
		wantRes      TemplateRefreshResult
	}{
		{
			name: "upgrade replaces the template", marker: "v0.8.7", installed: installedTemplate, state: true, fetch: ok(),
			wantTemplate: refreshedTemplate, wantMarker: "v0.8.8", wantCalls: 1,
			wantRes: TemplateRefreshResult{RebuildConfig: true, Downloaded: true},
		},
		{
			name: "legacy install without marker is refreshed", installed: installedTemplate, fetch: ok(),
			wantTemplate: refreshedTemplate, wantMarker: "v0.8.8", wantCalls: 1,
			wantRes: TemplateRefreshResult{Downloaded: true},
		},
		{
			name: "network failure keeps the old template and retries next launch", marker: "v0.8.7", installed: installedTemplate, state: true,
			fetch:        &templateFetchStub{err: errors.New("i/o timeout")},
			wantTemplate: installedTemplate, wantMarker: "v0.8.7", wantCalls: 1, wantErr: true,
			wantRes: TemplateRefreshResult{RebuildConfig: true},
		},
		{
			name: "HTTP error keeps the old template", marker: "v0.8.7", installed: installedTemplate,
			fetch:        &templateFetchStub{body: "not found", status: http.StatusNotFound},
			wantTemplate: installedTemplate, wantMarker: "v0.8.7", wantCalls: 1, wantErr: true,
		},
		{
			name: "a stub page with 200 does not overwrite the template", marker: "v0.8.7", installed: installedTemplate,
			fetch:        &templateFetchStub{body: "<html>blocked</html>", status: http.StatusOK},
			wantTemplate: installedTemplate, wantMarker: "v0.8.7", wantCalls: 1, wantErr: true,
		},
		{
			name: "template lost by an older launcher is restored when state needs it", marker: "v0.8.7", state: true, fetch: ok(),
			wantTemplate: refreshedTemplate, wantMarker: "v0.8.8", wantCalls: 1,
			wantRes: TemplateRefreshResult{RebuildConfig: true, Downloaded: true},
		},
		{
			name: "pristine install without state stays offline", marker: "v0.8.7", fetch: ok(),
			wantTemplate: "", wantMarker: "v0.8.8", wantCalls: 0,
		},
		{
			name: "template bundled by the installer for this version is kept", marker: "v0.8.7", installed: installedTemplate,
			bundled: "v0.8.8\n", state: true, fetch: ok(),
			wantTemplate: installedTemplate, wantMarker: "v0.8.8", wantCalls: 0,
			wantRes: TemplateRefreshResult{RebuildConfig: true},
		},
		{
			name: "split: shipped template for this version supersedes the previous download", marker: "v0.8.7", installed: installedTemplate,
			shipped: refreshedTemplate, shippedMark: "v0.8.8\n", state: true, fetch: ok(),
			wantTemplate: "", wantMarker: "v0.8.8", wantCalls: 0,
			wantRes: TemplateRefreshResult{RebuildConfig: true},
		},
		{
			name: "split: shipped template of another version — the download is refreshed", marker: "v0.8.7", installed: installedTemplate,
			shipped: installedTemplate, shippedMark: "v0.8.7\n", state: true, fetch: ok(),
			wantTemplate: refreshedTemplate, wantMarker: "v0.8.8", wantCalls: 1,
			wantRes: TemplateRefreshResult{RebuildConfig: true, Downloaded: true},
		},
		{
			name: "data root moved since the last build forces a rebuild", marker: "v0.8.8", installed: installedTemplate, state: true,
			dataRoot: "/somewhere/else", fetch: ok(),
			wantTemplate: installedTemplate, wantMarker: "v0.8.8", wantCalls: 0,
			wantRes: TemplateRefreshResult{RebuildConfig: true},
		},
		{
			name: "same data root is no reason to rebuild", marker: "v0.8.8", installed: installedTemplate, state: true,
			dataRoot: "@", fetch: ok(),
			wantTemplate: installedTemplate, wantMarker: "v0.8.8", wantCalls: 0,
		},
		{
			name: "same version is untouched", marker: "v0.8.8", installed: installedTemplate, state: true, fetch: ok(),
			wantTemplate: installedTemplate, wantMarker: "v0.8.8", wantCalls: 0,
		},
		{
			name: "downgrade is untouched", marker: "v0.8.9", installed: installedTemplate, state: true, fetch: ok(),
			wantTemplate: installedTemplate, wantMarker: "v0.8.9", wantCalls: 0,
		},
		{
			name: "dev build is untouched", version: "v0.8.7-3-gabc1234-dirty", marker: "v0.8.7", installed: installedTemplate, state: true, fetch: ok(),
			wantTemplate: installedTemplate, wantMarker: "v0.8.7", wantCalls: 0,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			d := newLauncherDir(t)
			dataRoot := tc.dataRoot
			if dataRoot == "@" {
				dataRoot = filepath.Clean(d.root)
			}
			if tc.marker != "" || dataRoot != "" {
				raw, err := json.Marshal(map[string]string{
					"lang": "en", "last_template_launcher_version": tc.marker, "config_data_root": dataRoot,
				})
				if err != nil {
					t.Fatal(err)
				}
				d.write("bin/settings.json", string(raw))
			}
			layout := paths.Layout{App: paths.AppDir(d.root), Data: paths.DataDir(d.root)}
			if tc.shipped != "" {
				app := newLauncherDir(t)
				app.write("bin/"+constants.WizardTemplateFileName, tc.shipped)
				app.write("bin/"+constants.WizardTemplateVersionFileName, tc.shippedMark)
				layout.App = paths.AppDir(app.root)
			}
			if tc.installed != "" {
				d.write("bin/"+constants.WizardTemplateFileName, tc.installed)
			}
			if tc.bundled != "" {
				d.write("bin/"+constants.WizardTemplateVersionFileName, tc.bundled)
			}
			if tc.state {
				if err := os.MkdirAll(filepath.Dir(platform.GetWizardStatePath(paths.DataDir(d.root))), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := state.New().Save(platform.GetWizardStatePath(paths.DataDir(d.root))); err != nil {
					t.Fatal(err)
				}
			}
			version := tc.version
			if version == "" {
				version = "v0.8.8"
			}
			withAppVersion(t, version, func() {
				res, err := RefreshTemplateIfStale(context.Background(), layout, tc.fetch.fetch)
				if (err != nil) != tc.wantErr {
					t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
				}
				if res != tc.wantRes {
					t.Errorf("result = %+v, want %+v", res, tc.wantRes)
				}
			})
			if got := d.template(); got != tc.wantTemplate {
				t.Errorf("template on disk = %q, want %q", got, tc.wantTemplate)
			}
			if got := d.marker(); got != tc.wantMarker {
				t.Errorf("marker = %q, want %q", got, tc.wantMarker)
			}
			if tc.fetch.calls != tc.wantCalls {
				t.Errorf("fetch calls = %d, want %d", tc.fetch.calls, tc.wantCalls)
			}
			if left, _ := filepath.Glob(filepath.Join(d.root, "bin", "*.download")); len(left) > 0 {
				t.Errorf("temporary download files left behind: %v", left)
			}
		})
	}
}

// A failed refresh is retried on the next launch and a later success replaces
// the file; a success is not repeated on the same version, so a template the
// user drops in by hand afterwards survives further launches.
func TestRefreshTemplateIfStale_RetriesUntilSuccessThenOncePerVersion(t *testing.T) {
	d := newLauncherDir(t)
	d.write("bin/settings.json", `{"lang":"en","last_template_launcher_version":"v0.8.7"}`)
	d.write("bin/"+constants.WizardTemplateFileName, installedTemplate)

	withAppVersion(t, "v0.8.8", func() {
		offline := &templateFetchStub{err: errors.New("connection reset")}
		if _, err := RefreshTemplateIfStale(context.Background(), paths.Layout{App: paths.AppDir(d.root), Data: paths.DataDir(d.root)}, offline.fetch); err == nil {
			t.Fatal("launch 1: expected the offline refresh to fail")
		}
		if d.template() != installedTemplate {
			t.Fatal("launch 1: the installed template must survive a failed refresh")
		}

		online := &templateFetchStub{body: refreshedTemplate, status: http.StatusOK}
		if _, err := RefreshTemplateIfStale(context.Background(), paths.Layout{App: paths.AppDir(d.root), Data: paths.DataDir(d.root)}, online.fetch); err != nil {
			t.Fatalf("launch 2: %v", err)
		}
		if d.template() != refreshedTemplate || d.marker() != "v0.8.8" {
			t.Fatalf("launch 2: template %q marker %q — want the refreshed template stamped v0.8.8", d.template(), d.marker())
		}

		const handMade = `{"placed": "by hand"}`
		d.write("bin/"+constants.WizardTemplateFileName, handMade)
		again := &templateFetchStub{body: refreshedTemplate, status: http.StatusOK}
		if _, err := RefreshTemplateIfStale(context.Background(), paths.Layout{App: paths.AppDir(d.root), Data: paths.DataDir(d.root)}, again.fetch); err != nil {
			t.Fatalf("launch 3: %v", err)
		}
		if again.calls != 0 || d.template() != handMade {
			t.Fatalf("launch 3: fetch calls %d, template %q — a stamped version must not refresh again", again.calls, d.template())
		}
	})
}

// Regression: a failed pre-start rebuild used to be logged and sing-box was
// started on the previous config.json anyway. Now the start is abandoned —
// no process is even prepared. A missing state.json stays a legitimate start
// on a hand-managed config.json.
func TestProcessServiceStart_RebuildFailureDoesNotStartCore(t *testing.T) {
	d := newLauncherDir(t)
	if err := os.MkdirAll(filepath.Dir(platform.GetWizardStatePath(paths.DataDir(d.root))), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.New().Save(platform.GetWizardStatePath(paths.DataDir(d.root))); err != nil {
		t.Fatal(err)
	}
	// Present but unusable: the build fails without reaching for the network.
	d.write("bin/"+constants.WizardTemplateFileName, "{ not a template")
	d.write("bin/config.json", `{"outbounds":[]}`)

	ac := &AppController{
		FileService: &services.FileService{
			Layout:      paths.Layout{App: paths.AppDir(d.root), Data: paths.DataDir(d.root)},
			ConfigPath:  filepath.Join(d.root, "bin", "config.json"),
			SingboxPath: filepath.Join(d.root, "bin", "sing-box-absent"),
		},
		StateService: services.NewStateService(),
		RunningState: &RunningState{},
	}
	ac.ProcessService = NewProcessService(ac)

	if err := ac.rebuildConfigBeforeStart(false); err == nil {
		t.Fatal("rebuild with an unusable template must fail")
	}
	ac.ProcessService.Start(true)
	if ac.SingboxCmd != nil || ac.RunningState.IsRunning() {
		t.Fatalf("sing-box must not be started after a failed rebuild (cmd %v, running %v)", ac.SingboxCmd, ac.RunningState.IsRunning())
	}

	if err := os.Remove(platform.GetWizardStatePath(paths.DataDir(d.root))); err != nil {
		t.Fatal(err)
	}
	if err := ac.rebuildConfigBeforeStart(false); err != nil {
		t.Fatalf("no state.json means config.json is managed by hand — start must proceed, got %v", err)
	}
}
