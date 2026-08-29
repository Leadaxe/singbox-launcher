package business

import (
	"os"
	"path/filepath"
	"testing"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/platform"
)

// writeCloneState кладёт состояние в каталог машины и возвращает execDir.
func writeCloneState(t *testing.T, execDir, target, machineID string, st *corestate.State) {
	t.Helper()
	path := platform.GetWizardStatePathFor(execDir, target, machineID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := st.Save(path); err != nil {
		t.Fatalf("save %s: %v", path, err)
	}
}

// TestCloneDropsMachineBoundVars — привязки к железу не переносятся.
//
// Ради этого клон и отличается от копирования файла: br-lan роутера на VPS
// не существует, и конфиг с ним не поднимется.
func TestCloneDropsMachineBoundVars(t *testing.T) {
	execDir := t.TempDir()

	donor := corestate.New()
	donor.Vars = []corestate.SettingVar{
		{Name: "tun_interface_name", Value: "tun-router"},
		{Name: "gateway_include_interface", Value: "br-lan"},
		{Name: "bind_interface", Value: "eth0"},
		{Name: "clash_secret", Value: "s3cret"},
		// Не machine-bound: обязано переехать.
		{Name: "tun_mtu", Value: "1280"},
		{Name: "strict_route", Value: "true"},
	}
	writeCloneState(t, execDir, constants.ConfigTargetRemote, "home", donor)

	src := CloneSource{Kind: CloneSourceRemote, MachineID: "home", Name: "Home"}
	got, summary, err := LoadCloneState(execDir, src)
	if err != nil {
		t.Fatalf("LoadCloneState: %v", err)
	}

	kept := map[string]string{}
	for _, v := range got.Vars {
		kept[v.Name] = v.Value
	}
	for _, name := range []string{"gateway_include_interface", "bind_interface", "clash_secret"} {
		if _, ok := kept[name]; ok {
			t.Errorf("machine-bound var %q was cloned; it must stay on its own machine", name)
		}
	}
	// strict_route непереносим в LX Backup (нет пары на мобиле), но между
	// двумя машинами лаунчера переносится — иначе клон молча терял бы
	// настройку пользователя. Имя TUN тоже переносится: его придумывает
	// пользователь, ядро создаёт интерфейс само, и одинаковое имя на всех
	// машинах — обычно цель (общие правила firewall, общие скрипты).
	for _, name := range []string{"tun_mtu", "strict_route", "tun_interface_name"} {
		if _, ok := kept[name]; !ok {
			t.Errorf("var %q must be cloned, but was dropped", name)
		}
	}
	if len(summary.SkippedVars) == 0 {
		t.Error("skipped machine-bound vars must be reported to the user, not dropped silently")
	}
}

// TestCloneClearsDonorIdentity — клон не выдаёт себя за файл донора.
//
// Главное здесь meta.target: если оставить его remote/<донор>, приёмник
// после загрузки стал бы писать состояние в чужой каталог.
func TestCloneClearsDonorIdentity(t *testing.T) {
	execDir := t.TempDir()

	donor := corestate.New()
	donor.ID = "donor-snapshot"
	donor.Comment = "донорский комментарий"
	donor.Target = constants.ConfigTargetRemote
	donor.TargetPlatform = "linux"
	donor.TargetArch = "amd64"
	writeCloneState(t, execDir, constants.ConfigTargetRemote, "home", donor)

	got, _, err := LoadCloneState(execDir, CloneSource{
		Kind: CloneSourceRemote, MachineID: "home", Name: "Home"})
	if err != nil {
		t.Fatalf("LoadCloneState: %v", err)
	}

	if got.ID != "" || got.Comment != "" {
		t.Errorf("donor snapshot identity leaked: ID=%q Comment=%q", got.ID, got.Comment)
	}
	if got.Target != "" || got.TargetPlatform != "" || got.TargetArch != "" {
		t.Errorf("donor target leaked: %q %q %q — the receiver would save into the donor's directory",
			got.Target, got.TargetPlatform, got.TargetArch)
	}
}

// TestCloneCountsChainsAndSources — цепочка живёт среди источников (она
// ВЕДЁТ маршрут, а Направление выбирает между маршрутами), и сводка обязана
// считать её отдельно от серверов.
func TestCloneCountsChainsAndSources(t *testing.T) {
	execDir := t.TempDir()

	donor := corestate.New()
	donor.Sources = []corestate.Source{
		{ID: "a", Node: corestate.Node{Kind: corestate.SourceKindSubscription}},
		{ID: "b", Node: corestate.Node{Kind: corestate.SourceKindServer}},
		{ID: "c", Node: corestate.Node{Kind: corestate.SourceKindChain}},
		{ID: "d", Node: corestate.Node{Kind: corestate.SourceKindChain}},
	}
	donor.Rules = []corestate.Rule{
		{Kind: corestate.RuleKindInline, Enabled: true},
		{Kind: corestate.RuleKindInline, Enabled: true},
		{Kind: corestate.RuleKindInline, Enabled: true},
	}
	writeCloneState(t, execDir, constants.ConfigTargetLocal, "", donor)

	_, summary, err := LoadCloneState(execDir, CloneSource{Kind: CloneSourceLocal, Name: "Local"})
	if err != nil {
		t.Fatalf("LoadCloneState: %v", err)
	}

	if summary.Subscriptions != 1 || summary.Servers != 1 || summary.Chains != 2 {
		t.Errorf("source counts = subs:%d servers:%d chains:%d, want 1/1/2",
			summary.Subscriptions, summary.Servers, summary.Chains)
	}
	if summary.Rules != 3 {
		t.Errorf("Rules = %d, want 3", summary.Rules)
	}
}

// TestCloneCountsLegacyV5Rules — у состояния, записанного до v6, правила
// лежат в custom_rules. corestate.Load выводит из них Rules
// (deriveV6FromLegacy), и сводка обязана показать их, а не «правил: 0» на
// конфиге, где их много.
func TestCloneCountsLegacyV5Rules(t *testing.T) {
	execDir := t.TempDir()

	path := platform.GetWizardStatePathFor(execDir, constants.ConfigTargetLocal, "")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacy := `{
	  "meta": {"version": 5, "schema": "singbox-launcher-state"},
	  "custom_rules": [
	    {"label": "ru-direct", "type": "inline", "rule": {"domain_suffix": [".ru"]}, "outbound": "direct"},
	    {"label": "ads-block", "type": "inline", "rule": {"domain_suffix": [".ads"]}, "outbound": "block"}
	  ]
	}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	_, summary, err := LoadCloneState(execDir, CloneSource{Kind: CloneSourceLocal, Name: "Local"})
	if err != nil {
		t.Fatalf("LoadCloneState: %v", err)
	}
	if summary.Rules != 2 {
		t.Errorf("Rules = %d, want 2 (legacy custom_rules must be counted)", summary.Rules)
	}
}

// TestCloneSourceListExcludesSelf — машина не предлагает себя в доноры:
// клон самого себя ничего не делает, а строка, которая ничего не делает,
// читается как сломанная.
func TestCloneSourceListExcludesSelf(t *testing.T) {
	execDir := t.TempDir()
	writeCloneState(t, execDir, constants.ConfigTargetLocal, "", corestate.New())
	writeCloneState(t, execDir, constants.ConfigTargetRemote, "home", corestate.New())

	machines := []CloneSource{
		{MachineID: "home", Name: "Home"},
		{MachineID: "ira", Name: "IRA"},
	}

	got := ListCloneSources(execDir, machines, constants.ConfigTargetRemote, "ira")
	for _, s := range got {
		if s.Kind == CloneSourceRemote && s.MachineID == "ira" {
			t.Fatal("current machine offered as its own clone source")
		}
	}
	if len(got) != 2 { // Local + Home
		t.Fatalf("got %d sources, want 2 (Local + Home)", len(got))
	}
	if got[0].Kind != CloneSourceLocal {
		t.Errorf("Local must come first, got %q", got[0].Name)
	}
	if !got[0].HasState {
		t.Error("Local has a state file, but HasState is false")
	}

	// Машина без состояния остаётся в списке, но помечена: «нечего
	// клонировать» — это ответ, а исчезнувшая строка — загадка.
	var ira CloneSource
	got2 := ListCloneSources(execDir, machines, constants.ConfigTargetRemote, "home")
	for _, s := range got2 {
		if s.MachineID == "ira" {
			ira = s
		}
	}
	if ira.Name == "" {
		t.Fatal("machine without a state file disappeared from the list")
	}
	if ira.HasState {
		t.Error("machine without a state file reported as configured")
	}
}

// TestCloneSourceListOmitsLocalWhenLocal — на локальной машине Local в
// списке доноров не появляется (см. выше про no-op строку).
func TestCloneSourceListOmitsLocalWhenLocal(t *testing.T) {
	execDir := t.TempDir()
	writeCloneState(t, execDir, constants.ConfigTargetRemote, "home", corestate.New())

	got := ListCloneSources(execDir, []CloneSource{{MachineID: "home", Name: "Home"}},
		constants.ConfigTargetLocal, "")

	for _, s := range got {
		if s.Kind == CloneSourceLocal {
			t.Fatal("Local offered as a clone source while editing Local itself")
		}
	}
	if len(got) != 1 {
		t.Fatalf("got %d sources, want 1 (Home)", len(got))
	}
}
