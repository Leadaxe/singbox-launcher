package business

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Каталог состояния tailnet на операциях UI (SPEC 122, нормы 1–2).
//
// Данные, не тексты: проверяется, что каталог во ВРЕМЕННОМ корне переехал
// или исчез вместе с узлом. Один тест на три операции — удаление,
// переименование, перенос: они одна норма, и разойтись им нельзя.

func tsUIRoot(t *testing.T) string {
	t.Helper()
	prev := config.TailscaleStateDirRoot()
	root := filepath.Join(t.TempDir(), "tailscale")
	config.SetTailscaleStateDirRoot(root)
	t.Cleanup(func() { config.SetTailscaleStateDirRoot(prev) })
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	return root
}

func tsUIMkDir(t *testing.T, root, name, marker string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(p, "tailscaled.state"), []byte(marker), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
}

func tsUIMarker(root, name string) string {
	b, err := os.ReadFile(filepath.Join(root, name, "tailscaled.state"))
	if err != nil {
		return ""
	}
	return string(b)
}

func tsUIExists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}

// tsUINode — узел tailnet с материализованным телом. Без тела узел для
// NodeIsTailscale не существует, и вся норма прошла бы мимо.
func tsUINode(tag string) corestate.Node {
	body, _ := json.Marshal(map[string]interface{}{"type": "tailscale", "auth_key": "tskey-x"})
	return corestate.Node{
		Kind: corestate.SourceKindServer, Tag: tag, Enabled: true,
		Body: json.RawMessage(body),
	}
}

// TestTailscaleStateDirFollowsNodeUI — удаление узла сносит каталог,
// переименование и перенос его переносят.
func TestTailscaleStateDirFollowsNodeUI(t *testing.T) {
	root := tsUIRoot(t)

	// --- Норма 1: удаление узла папки ---
	folder := moveTestFolder("01SRC", "Folder", tsUINode("ts-doomed"), tsUINode("ts-keep"))
	folder.TagPolicy = &corestate.TagPolicy{Prefix: "F-"}
	tsUIMkDir(t, root, "F-ts-doomed", "key-doomed")
	tsUIMkDir(t, root, "F-ts-keep", "key-keep")

	RemoveTailscaleStateDirForNode(&folder, &folder.Nodes[0])
	if tsUIExists(root, "F-ts-doomed") {
		t.Fatalf("норма 1: каталог удалённого узла остался")
	}
	if tsUIMarker(root, "F-ts-keep") != "key-keep" {
		t.Fatalf("норма 1: снесён каталог СОСЕДА")
	}

	// --- Норма 1: удаление папки вместе с узлами ---
	RemoveTailscaleStateDirsForSource(&folder)
	if tsUIExists(root, "F-ts-keep") {
		t.Fatalf("норма 1: каталог узла удалённой папки остался")
	}

	// --- Норма 2: переименование узла в папке ---
	f2 := moveTestFolder("01F2", "F2", tsUINode("ts-old"))
	f2.TagPolicy = &corestate.TagPolicy{Prefix: "P-"}
	tsUIMkDir(t, root, "P-ts-old", "key-identity")
	f2.Nodes[0].Tag = "ts-new"
	RenameTailscaleStateDirForNode(&f2, "ts-old", &f2, &f2.Nodes[0])
	if got := tsUIMarker(root, "P-ts-new"); got != "key-identity" {
		t.Fatalf("норма 2 (переименование): идентичность не переехала, маркер %q", got)
	}
	if tsUIExists(root, "P-ts-old") {
		t.Fatalf("норма 2 (переименование): старый каталог остался")
	}

	// --- Норма 2: перенос узла из папки в корень (MoveNodeToRoot) ---
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		func() corestate.Source {
			f := moveTestFolder("01MV", "MV", tsUINode("ts-moved"))
			f.TagPolicy = &corestate.TagPolicy{Prefix: "M-"}
			return f
		}(),
	}}
	tsUIMkDir(t, root, "M-ts-moved", "key-move")

	if _, err := MoveNodeToRoot(m, 0, "ts-moved"); err != nil {
		t.Fatalf("MoveNodeToRoot: %v", err)
	}
	// В корне политики нет: финальный тег = сырой.
	if got := tsUIMarker(root, "ts-moved"); got != "key-move" {
		t.Fatalf("норма 2 (перенос в корень): каталог не переехал, маркер %q", got)
	}
	if tsUIExists(root, "M-ts-moved") {
		t.Fatalf("норма 2 (перенос в корень): каталог остался под старым именем")
	}

	// --- Норма 2: перенос корневого узла В ПАПКУ (MoveNodeToFolder) ---
	dst := moveTestFolder("01DST", "Dst")
	dst.TagPolicy = &corestate.TagPolicy{Prefix: "D-"}
	m.Sources = append(m.Sources, dst)
	rootIdx := -1
	for i := range m.Sources {
		if m.Sources[i].Kind == corestate.SourceKindServer && m.Sources[i].Tag == "ts-moved" {
			rootIdx = i
			break
		}
	}
	if rootIdx < 0 {
		t.Fatalf("вынесенный в корень узел не найден")
	}
	if _, err := MoveNodeToFolder(m, rootIdx, "ts-moved", "01DST"); err != nil {
		t.Fatalf("MoveNodeToFolder: %v", err)
	}
	if got := tsUIMarker(root, "D-ts-moved"); got != "key-move" {
		t.Fatalf("норма 2 (перенос в папку): каталог не переехал, маркер %q", got)
	}
	if tsUIExists(root, "ts-moved") {
		t.Fatalf("норма 2 (перенос в папку): каталог остался под корневым именем")
	}

	// --- Норма 2: смена тег-политики контейнера ---
	pol := moveTestFolder("01POL", "Pol", tsUINode("ts-pol"))
	pol.TagPolicy = &corestate.TagPolicy{Prefix: "A-"}
	tsUIMkDir(t, root, "A-ts-pol", "key-pol")
	RenameTailscaleStateDirsForTagPolicy(&pol,
		&corestate.TagPolicy{Prefix: "A-"}, &corestate.TagPolicy{Prefix: "B-", Postfix: "-x"})
	if got := tsUIMarker(root, "B-ts-pol-x"); got != "key-pol" {
		t.Fatalf("норма 2 (тег-политика): каталог не переехал, маркер %q", got)
	}
	if tsUIExists(root, "A-ts-pol") {
		t.Fatalf("норма 2 (тег-политика): каталог остался под старым именем")
	}
}

// TestTailscaleStateDirIgnoresForeignNodes — узлы чужих схем и узлы с ЯВНЫМ
// state_directory лаунчер не трогает.
//
// Второй случай — не косметика: явный путь задал пользователь, он лежит вне
// нашего корня, и снести его значило бы удалить чужой каталог по чужому
// адресу.
func TestTailscaleStateDirIgnoresForeignNodes(t *testing.T) {
	root := tsUIRoot(t)

	vless := moveTestNode("vl", true, "")
	if NodeIsTailscale(&vless) {
		t.Fatalf("vless объявлен узлом tailnet")
	}

	ownBody, _ := json.Marshal(map[string]interface{}{
		"type": "tailscale", "state_directory": "/opt/ts-own"})
	own := corestate.Node{Kind: corestate.SourceKindServer, Tag: "ts-own", Enabled: true,
		Body: json.RawMessage(ownBody)}
	if NodeIsTailscale(&own) {
		t.Fatalf("узел с явным state_directory попал под управление лаунчера")
	}

	// Каталог с таким именем под корнем лаунчер сносить не должен: узел его
	// не «носит», это чужая папка.
	tsUIMkDir(t, root, "ts-own", "not-ours")
	src := corestate.Source{Node: own, ID: "01OWN"}
	RemoveTailscaleStateDirsForSource(&src)
	if tsUIMarker(root, "ts-own") != "not-ours" {
		t.Fatalf("каталог узла с явным state_directory снесён")
	}
}
