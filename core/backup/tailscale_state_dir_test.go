package backup

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"singbox-launcher/core/config"
	"singbox-launcher/core/state"
)

// SPEC 122, «Каталог состояния: жизненный цикл», норма 4 — каталог состояния
// tailnet В БЭКАП НЕ ЕДЕТ.
//
// Свойство держится тем, что `state_directory` штампует ТОЛЬКО config-форма
// эмиссии (GenerateEndpointJSON), а тело узла в состоянии и в файле бэкапа
// его не носит. Это легко потерять одной строкой: стоит эмиттеру начать
// подставлять путь в GenerateEndpointJSONBare (или кому-то «дообогатить» тело
// узла при экспорте) — и в файл, который едет на ДРУГУЮ машину и в LxBox,
// уедет абсолютный путь этой.
//
// Поэтому тест проверяет оба конца сразу: голую эмиссию и байты файла.
func TestTailscaleStateDirNeverReachesBackup(t *testing.T) {
	// Корень заведомо НЕпустой: с пустым корнем эмиссия путь не подставляет
	// вовсе, и тест проходил бы по причине, которая в проде не выполняется.
	prevRoot := config.TailscaleStateDirRoot()
	config.SetTailscaleStateDirRoot("/tmp/never-in-backup/tailscale")
	t.Cleanup(func() { config.SetTailscaleStateDirRoot(prevRoot) })

	body := json.RawMessage(`{"type":"tailscale","auth_key":"tskey-secret","hostname":"lx"}`)

	// 1. Голая эмиссия (её пишет канон и её же читает экспорт) пути НЕ несёт.
	bare, err := config.GenerateEndpointJSONBare(&config.ParsedNode{
		Scheme: config.SchemeTailscale, Tag: "ts",
		Outbound: map[string]interface{}{"type": "tailscale", "auth_key": "tskey-secret"},
	})
	if err != nil {
		t.Fatalf("GenerateEndpointJSONBare: %v", err)
	}
	if bytes.Contains([]byte(bare), []byte("state_directory")) {
		t.Fatalf("голая эмиссия несёт state_directory:\n%s", bare)
	}
	// Контроль: config-форма его как раз подставляет — иначе проверка выше
	// ничего не значила бы (поле могло исчезнуть отовсюду).
	forConfig, err := config.GenerateEndpointJSON(&config.ParsedNode{
		Scheme: config.SchemeTailscale, Tag: "ts",
		Outbound: map[string]interface{}{"type": "tailscale", "auth_key": "tskey-secret"},
	})
	if err != nil {
		t.Fatalf("GenerateEndpointJSON: %v", err)
	}
	if !bytes.Contains([]byte(forConfig), []byte("state_directory")) {
		t.Fatalf("config-форма перестала подставлять state_directory — проверка выше стала бессмысленной:\n%s", forConfig)
	}

	// 2. Файл бэкапа (оба формата) пути не несёт.
	s := &state.State{}
	s.Sources = []state.Source{{
		ID:   "01TSBACKUP0000000000000A",
		Node: state.Node{Kind: state.SourceKindServer, Enabled: true, Tag: "ts", Body: body},
	}}

	b10, _, err := Export10(s, ExportOptions{
		AppVersion: "test", Platform: "darwin", Now: time.Unix(1750000000, 0)})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	raw10, err := json.Marshal(b10)
	if err != nil {
		t.Fatalf("marshal 1.0: %v", err)
	}
	if bytes.Contains(raw10, []byte("state_directory")) {
		t.Fatalf("бэкап 1.0 несёт state_directory:\n%s", raw10)
	}

	b012, _, err := Export012(s, ExportOptions{
		AppVersion: "test", Platform: "darwin", Now: time.Unix(1750000000, 0)})
	if err != nil {
		t.Fatalf("Export012: %v", err)
	}
	raw012, err := json.Marshal(b012)
	if err != nil {
		t.Fatalf("marshal 0.12: %v", err)
	}
	if bytes.Contains(raw012, []byte("state_directory")) {
		t.Fatalf("бэкап 0.12 несёт state_directory:\n%s", raw012)
	}
}
