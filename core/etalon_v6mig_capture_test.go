package core

// Эталон W2 для приёмки W8 (SPEC 118 §4.C, PLAN §9 W2).
//
// «Снять эталон ТЕКУЩИМ движком, пока старый движок жив»: тест прогоняет
// v6-состояние с raw-кэшем (SPECS/118-F-N-STATE_V7/etalon/v6mig/) через
// СЕГОДНЯШНИЙ конвейер raw-кэш → парсер → эмиссия outbound'ов
// (buildSnapshotFromState) и сверяет байт-в-байт с зафиксированным
// снимком outbounds.snapshot.json.
//
// Почему снимок эмиссии, а не целый config.json: SPEC 118 меняет ровно слой
// «тело подписки → outbound-объекты» (W4 переводит его на nodes[]);
// слой BuildConfig (шаблон/route/dns) кампания не трогает и он закрыт
// полноформатным эталоном real-v088 (SPECS/118-F-N-STATE_V7/etalon/
// real-v088.config.json). Байт-равенство снимка эмиссии + неизменный
// BuildConfig ⇒ байт-равенство config.json.
//
// Режимы:
//   - по умолчанию тест ПРОПУСКАЕТСЯ (эталон сверяет W8, а в волнах W4–W7
//     расхождение — ожидаемая стадия работ, не повод валить go test ./...);
//   - ETALON_V6MIG=1 — сверка с эталоном (W8, приёмка);
//   - ETALON_V6MIG=capture — перезапись эталона ТЕКУЩИМ движком
//     (использовано один раз в W2, ДО W4; повторный capture после W4
//     означал бы «сравнить новый движок с самим собой» — не делать).
import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/core/state"
)

const etalonV6MigDir = "../SPECS/118-F-N-STATE_V7/etalon/v6mig"

func TestEtalonV6MigOutboundSnapshot(t *testing.T) {
	mode := os.Getenv("ETALON_V6MIG")
	if mode == "" {
		t.Skip("etalon check disabled by default (set ETALON_V6MIG=1 to verify, =capture to re-baseline)")
	}

	fixtureDir, err := filepath.Abs(etalonV6MigDir)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	// Разворачиваем фикстуру во временный execDir-макет:
	// bin/wizard_states/state.json + bin/subscriptions/<id>.raw.
	execDir := t.TempDir()
	subsDir := filepath.Join(execDir, "bin", "subscriptions")
	statesDir := filepath.Join(execDir, "bin", "wizard_states")
	for _, d := range []string{subsDir, statesDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	stateBytes, err := os.ReadFile(filepath.Join(fixtureDir, "state.json"))
	if err != nil {
		t.Fatalf("read fixture state: %v", err)
	}
	statePath := filepath.Join(statesDir, "state.json")
	if err := os.WriteFile(statePath, stateBytes, 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(fixtureDir, "01J00000000000000000000SUB.raw"))
	if err != nil {
		t.Fatalf("read fixture raw: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subsDir, "01J00000000000000000000SUB.raw"), raw, 0o644); err != nil {
		t.Fatalf("write raw: %v", err)
	}

	s, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	// Явный no-op-substituter: эталон не должен зависеть от шаблона на диске.
	noSubst := func(name string) (interface{}, bool) { return nil, false }
	cache, _, err := buildSnapshotFromState(s, execDir, noSubst, nil)
	if err != nil {
		t.Fatalf("buildSnapshotFromState: %v", err)
	}

	snap := struct {
		Outbounds []json.RawMessage `json:"outbounds"`
		Endpoints []json.RawMessage `json:"endpoints"`
	}{Outbounds: cache.Outbounds, Endpoints: cache.Endpoints}
	actual, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	actual = append(actual, '\n')

	etalonPath := filepath.Join(fixtureDir, "outbounds.snapshot.json")
	if mode == "capture" {
		if err := os.WriteFile(etalonPath, actual, 0o644); err != nil {
			t.Fatalf("write etalon: %v", err)
		}
		t.Logf("etalon captured: %s (%d bytes)", etalonPath, len(actual))
		return
	}

	expected, err := os.ReadFile(etalonPath)
	if err != nil {
		t.Fatalf("read etalon (capture it first with ETALON_V6MIG=capture): %v", err)
	}
	if !bytes.Equal(actual, expected) {
		actualPath := filepath.Join(fixtureDir, "outbounds.actual.json")
		_ = os.WriteFile(actualPath, actual, 0o644)
		t.Fatalf("v6mig outbound snapshot diverged from W2 etalon (see %s); "+
			"любое расхождение — блокер O3: поимённо на вердикт, не подгонять молча", actualPath)
	}
}
