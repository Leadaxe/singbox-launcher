package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// Жизненный цикл каталога состояния tailnet (SPEC 122, нормы 1–3).
//
// Тест ОДИН и комплексный: remove / rename / GC — три половины одной нормы, и
// проверять их порознь значило бы не проверить главного — что GC не сносит то,
// что сохранили rename и правила ожидаемого набора.

// tsStateRoot подменяет корень каталогов состояния на временный и возвращает
// его. Восстановление прежнего значения — через t.Cleanup: корень глобален, и
// оставленный указывать в TempDir он сломал бы соседние тесты пакета.
func tsStateRoot(t *testing.T) string {
	t.Helper()
	prev := TailscaleStateDirRoot()
	root := filepath.Join(t.TempDir(), "tailscale")
	SetTailscaleStateDirRoot(root)
	t.Cleanup(func() { SetTailscaleStateDirRoot(prev) })
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	return root
}

// mkStateDir создаёт каталог состояния с файлом-маркером: пустой каталог не
// отличить от «переехавшего», а маркер доказывает, что переехало СОДЕРЖИМОЕ —
// то есть ключ устройства.
func mkStateDir(t *testing.T, root, name, marker string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(p, "tailscaled.state"), []byte(marker), 0o600); err != nil {
		t.Fatalf("write marker %s: %v", name, err)
	}
}

func stateDirMarker(t *testing.T, root, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name, "tailscaled.state"))
	if err != nil {
		return ""
	}
	return string(b)
}

func stateDirsUnder(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root: %v", err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

func tsBody(t *testing.T, extra map[string]interface{}) json.RawMessage {
	t.Helper()
	b := map[string]interface{}{"type": "tailscale", "auth_key": "tskey-test"}
	for k, v := range extra {
		b[k] = v
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return raw
}

// TestTailscaleStateDirLifecycle — remove (норма 1), rename (норма 2) и GC
// (норма 3) на одном временном корне и состоянии из трёх узлов: корневой,
// член папки с префиксом и ВЫКЛЮЧЕННЫЙ.
func TestTailscaleStateDirLifecycle(t *testing.T) {
	root := tsStateRoot(t)

	// --- Норма 1: удаление ---
	mkStateDir(t, root, "doomed", "key-doomed")
	RemoveTailscaleStateDir(TailscaleStateDirName("doomed"))
	if _, err := os.Stat(filepath.Join(root, "doomed")); !os.IsNotExist(err) {
		t.Fatalf("норма 1: каталог удалённого узла остался (%v)", err)
	}
	// Повторное удаление несуществующего — не ошибка и не паника.
	RemoveTailscaleStateDir(TailscaleStateDirName("doomed"))

	// --- Норма 2: переименование ---
	mkStateDir(t, root, "ts-old", "key-moved")
	RenameTailscaleStateDir(TailscaleStateDirName("ts-old"), TailscaleStateDirName("ts-new"))
	if got := stateDirMarker(t, root, "ts-new"); got != "key-moved" {
		t.Fatalf("норма 2: идентичность не переехала, маркер %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "ts-old")); !os.IsNotExist(err) {
		t.Fatalf("норма 2: старый каталог остался")
	}

	// Новое имя занято — оставляем ОБА, чужую идентичность не затираем.
	mkStateDir(t, root, "ts-a", "key-a")
	mkStateDir(t, root, "ts-b", "key-b")
	RenameTailscaleStateDir("ts-a", "ts-b")
	if got := stateDirMarker(t, root, "ts-a"); got != "key-a" {
		t.Fatalf("норма 2: исходный каталог при занятом имени пропал (%q)", got)
	}
	if got := stateDirMarker(t, root, "ts-b"); got != "key-b" {
		t.Fatalf("норма 2: занятый каталог затёрт переименованием (%q)", got)
	}

	// Тег с символами, недопустимыми в имени каталога, уходит в "_" — и
	// эмиссия, и уборка обязаны звать ОДНУ функцию.
	if TailscaleStateDirName("AL: Liberty/ts") != "AL__Liberty_ts" {
		t.Fatalf("имя каталога по тегу: %q", TailscaleStateDirName("AL: Liberty/ts"))
	}

	// --- Норма 3: GC ---
	// Состояние: корневой `ts-root`, член папки с префиксом `F-` (узел
	// `ts-folder` → финальный `F-ts-folder`), ВЫКЛЮЧЕННЫЙ `ts-off` в той же
	// папке, и узел с ЯВНЫМ state_directory (в GC не участвует).
	pc := &ParserConfig{}
	pc.ParserConfig.Proxies = []ProxySource{
		{
			ID: "01ROOT",
			Canonical: &configtypes.CanonicalSource{
				IsContainer: false,
				Nodes: []configtypes.CanonicalNode{
					{Kind: "server", Tag: "ts-root", Enabled: true, Body: tsBody(t, nil)},
				},
			},
		},
		{
			ID: "01FOLDER",
			Canonical: &configtypes.CanonicalSource{
				FolderID: "01FOLDER", IsContainer: true, TagPrefix: "F-",
				Nodes: []configtypes.CanonicalNode{
					{Kind: "server", Tag: "ts-folder", Enabled: true, Body: tsBody(t, nil)},
					// Выключенный: до эмиссии не доходит, но состояние его
					// живёт — имя обязано попасть в ожидаемый набор.
					{Kind: "server", Tag: "ts-off", Enabled: false, Body: tsBody(t, nil)},
					// Явный путь пользователя — вне корня, в наборе не нужен.
					{Kind: "server", Tag: "ts-own", Enabled: true,
						Body: tsBody(t, map[string]interface{}{"state_directory": "/opt/ts"})},
					// Не tailscale — каталога у него нет вовсе.
					{Kind: "server", Tag: "vl", Enabled: true,
						Body: json.RawMessage(`{"type":"vless","server":"1.2.3.4"}`)},
				},
			},
		},
	}
	// Узел, ДОШЕДШИЙ до эмиссии: финальный тег с суффиксом уникализации —
	// его знает только эмиссия, и набор обязан взять его оттуда.
	emitted := []*ParsedNode{
		{Scheme: SchemeTailscale, Tag: "ts-root", Outbound: map[string]interface{}{"type": "tailscale"}},
		{Scheme: SchemeTailscale, Tag: "F-ts-folder-2", Outbound: map[string]interface{}{"type": "tailscale"}},
	}

	expected, ok := CollectTailscaleStateDirNames(pc, emitted)
	if !ok {
		t.Fatalf("норма 3: набор не построился на исправном состоянии")
	}
	for _, want := range []string{"ts-root", "F-ts-folder", "F-ts-folder-2", "F-ts-off"} {
		if !expected[want] {
			t.Fatalf("норма 3: ожидаемый набор без %q: %v", want, expected)
		}
	}
	if expected["ts-own"] || expected["F-ts-own"] {
		t.Fatalf("норма 3: узел с явным state_directory попал в набор: %v", expected)
	}
	if expected["vl"] || expected["F-vl"] {
		t.Fatalf("норма 3: не-tailscale узел попал в набор: %v", expected)
	}

	// Раскладываем каталоги: живые + сирота + файл (не наш).
	for _, name := range []string{"ts-root", "F-ts-folder", "F-ts-folder-2", "F-ts-off", "orphan"} {
		mkStateDir(t, root, name, "key-"+name)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	GCTailscaleStateDirs(expected, ok)

	got := stateDirsUnder(t, root)
	for _, want := range []string{"ts-root", "F-ts-folder", "F-ts-folder-2", "F-ts-off", "notes.txt"} {
		if !contains(got, want) {
			t.Fatalf("норма 3: GC снёс живое %q; осталось %v", want, got)
		}
	}
	if contains(got, "orphan") {
		t.Fatalf("норма 3: сирота пережила GC; осталось %v", got)
	}

	// --- Норма 3, защита: набор не построился → GC не делает НИЧЕГО ---
	mkStateDir(t, root, "orphan2", "key-orphan2")
	if _, okNil := CollectTailscaleStateDirNames(nil, nil); okNil {
		t.Fatalf("норма 3: набор по nil-состоянию объявлен построенным")
	}
	GCTailscaleStateDirs(nil, false)
	if !contains(stateDirsUnder(t, root), "orphan2") {
		t.Fatalf("норма 3: GC снёс каталоги при НЕпостроенном наборе")
	}
}

// TestTailscaleStateDirStaysInsideRoot — ничего за пределами корня.
//
// Имя каталога приходит из ТЕГА, то есть из чужой подписки. Пропусти путь с
// "..", и os.RemoveAll снёс бы произвольный каталог машины по строке из
// стороннего JSON.
func TestTailscaleStateDirStaysInsideRoot(t *testing.T) {
	root := tsStateRoot(t)
	outside := filepath.Join(filepath.Dir(root), "victim")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir victim: %v", err)
	}

	for _, name := range []string{"../victim", "..", ".", "", "a/b", "../../etc"} {
		RemoveTailscaleStateDir(name)
		RenameTailscaleStateDir(name, "x")
		RenameTailscaleStateDir("x", name)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("каталог ВНЕ корня удалён по имени с обходом: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("сам корень удалён: %v", err)
	}
	// sanitizeStateDirName и сам такие имена не порождает — проверим главное
	// свойство: точечное имя схлопывается в имя схемы, а не в "." или "..".
	if got := TailscaleStateDirName(".."); got != SchemeTailscale {
		t.Fatalf("тег %q дал имя каталога %q", "..", got)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Конфиг для ЧУЖОЙ машины несёт её путь, а не наш.
//
// Дыра, которую закрывает тест: state_directory штамповался из глобального
// корня `<execDir>/bin/tailscale`, посчитанного на машине лаунчера, и уезжал
// в конфиг роутера как есть. Ядро на той стороне создавало его от своего
// корня — состояние узла оказывалось в `/Applications/…/bin/tailscale/<тег>`
// на OpenWrt: рабочем каталоге, который сносит первая же чистка overlay.
func TestTailscaleStateDirRemoteRootWins(t *testing.T) {
	local := tsStateRoot(t) // ставит локальный корень и снимает его после теста
	const remote = "/etc/sing-box/tailscale"

	prev := TailscaleRemoteStateDirRoot()
	SetTailscaleRemoteStateDirRoot(remote)
	defer SetTailscaleRemoteStateDirRoot(prev)

	ep := map[string]interface{}{"type": SchemeTailscale}
	applyTailscaleStateDirectory(ep, SchemeTailscale, "Tailscale LexNet", TailscaleRemoteStateDirRoot())

	got, _ := ep["state_directory"].(string)
	if want := remote + "/Tailscale_LexNet"; got != want {
		t.Fatalf("state_directory = %q, want %q", got, want)
	}
	if strings.Contains(got, local) {
		t.Fatalf("в конфиг чужой машины уехал локальный путь: %q", got)
	}
	// Разделитель пути — целевой машины, не нашей: собранный на Windows
	// конфиг для linux-роутера не должен нести обратные слэши.
	if strings.Contains(got, `\`) {
		t.Fatalf("в пути чужой машины обратные слэши: %q", got)
	}

	// Local (пустой remote-корень) возвращается к локальному корню — иначе
	// после визита в Remote своя же сборка унесла бы путь роутера.
	SetTailscaleRemoteStateDirRoot("")
	ep2 := map[string]interface{}{"type": SchemeTailscale}
	applyTailscaleStateDirectory(ep2, SchemeTailscale, "ts", TailscaleRemoteStateDirRoot())
	if got2, _ := ep2["state_directory"].(string); !strings.HasPrefix(got2, local) {
		t.Fatalf("local-сборка не вернулась к локальному корню: %q", got2)
	}

	// Явное значение пользователя не перебивается ни в одном из режимов.
	SetTailscaleRemoteStateDirRoot(remote)
	ep3 := map[string]interface{}{"type": SchemeTailscale, "state_directory": "/custom/path"}
	applyTailscaleStateDirectory(ep3, SchemeTailscale, "ts", TailscaleRemoteStateDirRoot())
	if got3, _ := ep3["state_directory"].(string); got3 != "/custom/path" {
		t.Fatalf("явный state_directory перебит: %q", got3)
	}
}
