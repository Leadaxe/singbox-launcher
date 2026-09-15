package build

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/core/state"
)

// Сценарий golden `real-v088-v8` — тот же живой конфиг, что и `real-v088`, но
// с состоянием, уже переехавшим на state v8 (SPEC 127 §S2.5).
//
// Две папки нужны обе и проверяют разное:
//
//	real-v088     — state.json остался v7: это проверка МИГРАЦИИ на живом
//	                состоянии (Parse видит 7, гонит v7→v8, собирает конфиг);
//	real-v088-v8  — state.json уже v8: это проверка того, что мигрировать
//	                нечего и конфиг из готовой формы получается тот же самый.
//
// Ожидаемый конфиг у них ОДИН И ТОТ ЖЕ файл по содержанию — в этом и смысл
// инварианта §4 п. 1: форма хранения сменилась, байты конфига нет.
const (
	goldenV7Dir = "testdata/golden/real-v088"
	goldenV8Dir = "testdata/golden/real-v088-v8"
)

// TestGenerateGoldenV8Scenario — генератор папки `real-v088-v8`.
//
// По умолчанию пропускается: сценарий лежит в дереве и правится осознанно, а
// не пересчитывается на каждом прогоне. Перегенерация:
//
//	GEN_GOLDEN_V8=1 go test -run TestGenerateGoldenV8Scenario ./core/build/
func TestGenerateGoldenV8Scenario(t *testing.T) {
	if os.Getenv("GEN_GOLDEN_V8") == "" {
		t.Skip("set GEN_GOLDEN_V8=1 to regenerate testdata/golden/real-v088-v8")
	}
	if err := os.MkdirAll(goldenV8Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// template/cache/expected — копии: входы сборки от формы состояния не
	// зависят, а ожидание обязано совпасть с v7-сценарием байт-в-байт.
	for _, name := range []string{"template.json", "cache.json", "expected.config.json"} {
		data, err := os.ReadFile(filepath.Join(goldenV7Dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(goldenV8Dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// state.json — результат Parse(v7) + Save, то есть ровно то, что лаунчер
	// запишет пользователю при первом запуске после обновления.
	raw, err := os.ReadFile(filepath.Join(goldenV7Dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := state.Parse(raw)
	if err != nil {
		t.Fatalf("parse v7 golden state: %v", err)
	}
	out := filepath.Join(goldenV8Dir, "state.json")
	if err := st.Save(out); err != nil {
		t.Fatalf("save v8 golden state: %v", err)
	}
	saved, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, freezeGoldenTimestamps(saved), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("regenerated %s", goldenV8Dir)
}

// freezeGoldenTimestamps — meta.updated_at на константу: Save штампует его
// временем прогона, а сценарий обязан быть стабильным в дереве. На конфиг
// это поле не влияет.
func freezeGoldenTimestamps(data []byte) []byte {
	const prefix = `    "updated_at": "`
	lines := bytes.Split(data, []byte("\n"))
	for i, ln := range lines {
		if !bytes.HasPrefix(ln, []byte(prefix)) {
			continue
		}
		suffix := ""
		if bytes.HasSuffix(ln, []byte(",")) {
			suffix = ","
		}
		lines[i] = []byte(prefix + "2026-08-29T17:58:48Z\"" + suffix)
	}
	return bytes.Join(lines, []byte("\n"))
}

// TestGoldenV8ScenarioMatchesV7Expectation — обе папки описывают ОДИН конфиг.
//
// Без этой сверки `real-v088-v8` мог бы незаметно разъехаться с `real-v088`
// (например, кто-то пересчитал ожидание в одной папке), и тогда «обе зелёные»
// перестало бы значить «форма хранения сменилась, конфиг нет».
func TestGoldenV8ScenarioMatchesV7Expectation(t *testing.T) {
	v7, err := os.ReadFile(filepath.Join(goldenV7Dir, "expected.config.json"))
	if err != nil {
		t.Fatal(err)
	}
	v8, err := os.ReadFile(filepath.Join(goldenV8Dir, "expected.config.json"))
	if err != nil {
		t.Fatalf("сценарий real-v088-v8 не собран (GEN_GOLDEN_V8=1 …): %v", err)
	}
	if !bytes.Equal(v7, v8) {
		t.Error("ожидания real-v088 и real-v088-v8 разошлись — конфиг из v7- и v8-состояния обязан быть одним и тем же")
	}
	// И само состояние сценария обязано быть уже v8: иначе папка проверяла бы
	// ту же миграцию, что и соседняя, а не её отсутствие.
	raw, err := os.ReadFile(filepath.Join(goldenV8Dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	ver, err := state.SchemaVersionOfBytes(raw)
	if err != nil {
		t.Fatalf("шапка состояния сценария: %v", err)
	}
	if ver != state.SchemaVersionV8 {
		t.Errorf("state.json сценария real-v088-v8 несёт версию %d, ожидалась %d", ver, state.SchemaVersionV8)
	}
}
