//go:build liberty

package core

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

func libCopy(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open %s: %v", src, err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
}

func libCopyTree(t *testing.T, src, dst string) {
	t.Helper()
	_ = filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		libCopy(t, p, filepath.Join(dst, rel))
		return nil
	})
}

// TestLibertyLivePipeline — сквозной прогон ТЕКУЩЕГО кода на живой подписке:
// fetch (per-source UA) → MaterializeSubscriptionBody → MergeSubscriptionNodes →
// buildSnapshotFromState → BuildConfig → sing-box check.
func TestLibertyLivePipeline(t *testing.T) {
	subURL := os.Getenv("LIBERTY_URL")
	if subURL == "" {
		t.Skip("LIBERTY_URL not set")
	}
	outDir := os.Getenv("LIBERTY_OUT")
	if outDir == "" {
		outDir = t.TempDir()
	}
	installBin := "/Applications/singbox-launcher.app/Contents/MacOS/bin"
	execDir := filepath.Join(outDir, "execdir")
	binDir := filepath.Join(execDir, "bin")
	_ = os.RemoveAll(execDir)
	libCopy(t, filepath.Join(installBin, "wizard_template.json"), filepath.Join(binDir, "wizard_template.json"))
	libCopy(t, filepath.Join(installBin, "wizard_states", "state.json"), filepath.Join(binDir, "wizard_states", "state.json"))
	libCopy(t, filepath.Join(installBin, "settings.json"), filepath.Join(binDir, "settings.json"))
	libCopyTree(t, filepath.Join(installBin, "rule-sets"), filepath.Join(binDir, "rule-sets"))

	statePath := filepath.Join(binDir, "wizard_states", "state.json")
	s, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	var src *state.Source
	for i := range s.Sources {
		if s.Sources[i].URL == subURL {
			src = &s.Sources[i]
		}
	}
	if src == nil {
		t.Fatalf("source %s not in state", subURL)
	}
	if ua := os.Getenv("LIBERTY_UA"); ua != "" {
		src.UserAgent = ua
	}
	if os.Getenv("LIBERTY_ONLY") == "1" {
		for i := range s.Sources {
			if s.Sources[i].ID != src.ID {
				s.Sources[i].Enabled = false
			}
		}
	}
	if os.Getenv("LIBERTY_FRESH") == "1" {
		src.Nodes = nil
	}
	t.Logf("source id=%s ua=%q prefix=%q existing nodes=%d", src.ID, src.UserAgent, tagPrefixOf(src), len(src.Nodes))

	res, err := subscription.FetchSubscriptionWithMetaFor(src.URL, SourceIdentityOf(src))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	_ = os.WriteFile(filepath.Join(outDir, "body.raw"), res.RawBody, 0o644)
	t.Logf("fetched: http=%d raw=%d decoded=%d title=%q", res.HTTPStatus, len(res.RawBody), len(res.Body), res.Meta.ProfileTitle)

	material, err := config.MaterializeSubscriptionBody(src.ID, res.Body, src.Skip, 0)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	t.Logf("materialized: nodes=%d supported=%d truncated=%v warnings=%d", len(material.Nodes), material.Supported, material.Truncated, len(material.Warnings))
	for _, w := range material.Warnings {
		t.Logf("  WARN parse: %s", w)
	}
	changed, mergeWarns := state.MergeSubscriptionNodes(src, &state.SubFetchMaterial{Nodes: material.Nodes, Truncated: material.Truncated}, true)
	t.Logf("merge: changed=%v warnings=%d", changed, len(mergeWarns))
	for _, w := range mergeWarns {
		t.Logf("  WARN merge: %s", w)
	}
	nodesJSON, _ := json.MarshalIndent(src.Nodes, "", "  ")
	_ = os.WriteFile(filepath.Join(outDir, "nodes.json"), nodesJSON, 0o644)
	for _, n := range src.Nodes {
		var body map[string]interface{}
		_ = json.Unmarshal(n.Body, &body)
		extra := ""
		if n.Group != nil {
			ms := make([]string, 0, len(n.Group.Members))
			for _, m := range n.Group.Members {
				ms = append(ms, m.Tag)
			}
			extra = fmt.Sprintf(" group=%s members=%v", n.Group.GroupType, ms)
		}
		if n.Detour != nil {
			extra += fmt.Sprintf(" detour=%+v", *n.Detour)
		}
		if n.Reason != "" {
			extra += " reason=" + n.Reason
		}
		t.Logf("  node kind=%-11s en=%-5v tag=%-40q type=%-9v server=%v:%v%s", n.Kind, n.Enabled, n.Tag, body["type"], body["server"], body["server_port"], extra)
	}

	if err := s.Save(statePath); err != nil {
		t.Fatalf("save: %v", err)
	}
	s, err = state.Load(statePath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	td, err := template.LoadTemplateData(execDir)
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	cache, genRes, err := buildSnapshotFromState(s, execDir, nil, td)
	if err != nil {
		t.Fatalf("buildSnapshotFromState: %v (res=%+v)", err, genRes)
	}
	t.Logf("generation: sources total=%d ok=%d failed=%d nodes=%d endpoints=%d local=%d global=%d naiveSkipped=%d emptyDirections=%v brokenChains=%d parseFailed=%d excluded=%d",
		genRes.TotalSources, genRes.SucceededSources, genRes.FailedSources, genRes.NodesCount, genRes.EndpointsCount,
		genRes.LocalSelectorsCount, genRes.GlobalSelectorsCount, genRes.SkippedNaiveNodes, genRes.EmptyDirections, len(genRes.BrokenChains), len(genRes.ParseFailedSources), len(genRes.ExcludedSources))
	for _, pf := range genRes.ParseFailedSources {
		t.Logf("  parse-failed: %+v", pf)
	}
	for _, ex := range genRes.ExcludedSources {
		t.Logf("  excluded: %+v", ex)
	}
	for _, w := range cache.Warnings {
		t.Logf("  WARN snapshot: %s", w)
	}

	ac := &AppController{FileService: &services.FileService{
		ExecDir:     execDir,
		ConfigPath:  filepath.Join(binDir, "config.json"),
		SingboxPath: filepath.Join(installBin, "sing-box"),
	}}
	ctx := ac.buildContextFromState(s, cache, td)
	bres, err := build.BuildConfig(ctx)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	cfgPath := filepath.Join(outDir, "config.json")
	if err := os.WriteFile(cfgPath, bres.ConfigJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("config written: %s (%d bytes) errors=%v warnings=%d excluded=%d", cfgPath, len(bres.ConfigJSON), bres.Validation.Errors, len(bres.Validation.Warnings), len(bres.ExcludedSources))
	for _, w := range bres.Validation.Warnings {
		t.Logf("  WARN build: %s", w)
	}
	for _, ex := range bres.ExcludedSources {
		t.Logf("  excluded(build): %+v", ex)
	}

	for _, core := range []string{filepath.Join(installBin, "sing-box"), "../bin/sing-box"} {
		out, err := exec.Command(core, "check", "-c", cfgPath).CombinedOutput()
		ver, _ := exec.Command(core, "version").Output()
		v := strings.SplitN(string(ver), "\n", 2)[0]
		if err != nil {
			t.Errorf("sing-box check FAILED (%s): %v\n%s", v, err, out)
		} else {
			t.Logf("sing-box check OK (%s)", v)
		}
	}
}

func tagPrefixOf(src *state.Source) string {
	if src.TagPolicy == nil {
		return ""
	}
	return src.TagPolicy.Prefix
}
