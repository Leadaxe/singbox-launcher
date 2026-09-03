package config

// Конформанс-раннер общего корпуса контракта (SPEC 103, фаза 1).
//
// Гоняет contract/corpus/uri/**/*.uri через subscription.ParseNode и сравнивает
// результат с <case>.expected.json (или <case>.expected.launcher.json —
// per-app override, contract/docs/CANON.md §7).
//
// Регенерация ожиданий:
//
//	go test ./core/config -run TestContractCorpusURI -update
//
// Это ОСОЗНАННЫЙ шаг: expected нормативны, дифф идёт в PR с ревью
// (contract/README.md §2).

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

var updateContractGolden = flag.Bool("update", false, "перегенерировать expected-файлы корпуса контракта")

const contractCorpusRelPath = "../../contract/corpus"

// readCorpusURI возвращает URI из фикстуры (последняя строка, не начинающаяся с '#').
func readCorpusURI(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var uri string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimRight(line, "\r")
		if trimmed == "" || strings.HasPrefix(strings.TrimSpace(trimmed), "#") {
			continue
		}
		uri = trimmed
	}
	if uri == "" {
		t.Fatalf("%s: не найдена строка с URI", path)
	}
	return uri
}

// expectedPathFor выбирает per-app override, если он есть (CANON §7).
func expectedPathFor(base string) string {
	override := base + ".expected.launcher.json"
	if _, err := os.Stat(override); err == nil {
		return override
	}
	return base + ".expected.json"
}

func TestContractCorpusURI(t *testing.T) {
	root := filepath.Join(contractCorpusRelPath, "uri")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skipf("корпус контракта не найден: %s", root)
	}

	var cases []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".uri") {
			cases = append(cases, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("обход корпуса: %v", err)
	}
	if len(cases) == 0 {
		t.Skip("корпус пуст")
	}

	for _, casePath := range cases {
		name := strings.TrimPrefix(filepath.ToSlash(strings.TrimSuffix(casePath, ".uri")), filepath.ToSlash(root)+"/")
		t.Run(name, func(t *testing.T) {
			uri := readCorpusURI(t, casePath)
			base := strings.TrimSuffix(casePath, ".uri")

			env := contractEnvelope{V: 1}
			node, parseErr := subscription.ParseNode(uri, nil)
			switch {
			case parseErr != nil:
				// CANON §4: битая нода → dropped, подписка живёт.
				env.Dropped = append(env.Dropped, contractDrop{Ref: uri, Reason: "parse_error"})
			case node == nil:
				env.Dropped = append(env.Dropped, contractDrop{Ref: uri, Reason: "filtered"})
			default:
				cn, err := canonNode(node)
				if err != nil {
					env.Dropped = append(env.Dropped, contractDrop{Ref: uri, Reason: "emit_error"})
				} else {
					env.Nodes = append(env.Nodes, cn)
				}
			}

			got, err := marshalEnvelopePretty(env)
			if err != nil {
				t.Fatalf("сериализация конверта: %v", err)
			}

			expPath := expectedPathFor(base)
			if *updateContractGolden {
				if err := os.WriteFile(expPath, got, 0o644); err != nil {
					t.Fatalf("запись %s: %v", expPath, err)
				}
				return
			}

			want, err := os.ReadFile(expPath)
			if err != nil {
				t.Skipf("нет expected (%s) — сгенерируйте флагом -update", filepath.Base(expPath))
			}
			if !equalEnvelopeJSON(t, got, want) {
				t.Errorf("расхождение с контрактом\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}

// marshalEnvelopePretty пишет конверт читаемо (для файла), сохраняя канон
// значений; строгое сравнение идёт по разобранному JSON, не по байтам.
func marshalEnvelopePretty(env contractEnvelope) ([]byte, error) {
	// Пустой список сериализуется как `[]`, а не `null`. Go отдаёт nil-срез
	// как `null`, Dart — как `[]`, и каждый reject-кейс (нод нет, есть
	// только dropped) требовал .expected.lxbox.json, хотя поведение сторон
	// идентично: различалась не логика, а сериализация пустоты. 24 таких
	// override'а — чистый шум, маскировавший настоящие расхождения.
	if env.Nodes == nil {
		env.Nodes = []contractNode{}
	}
	compact, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(compact, &generic); err != nil {
		return nil, err
	}
	canon, err := canonMarshal(canonValue(generic))
	if err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, canon, "", "  "); err != nil {
		return nil, err
	}
	pretty.WriteByte('\n')
	return pretty.Bytes(), nil
}

// equalEnvelopeJSON сравнивает конверты корпуса с поправкой на D-088:
// в записях `dropped[]` нормативны `ref` и `code`, а `reason` — нет.
//
// Почему нормализация перед сравнением, а не отдельный обход двух конвертов:
// сравнение по значению после канонизации (CANON §7) — единственная точка
// правды раннеров, и второй, «почти такой же» путь сверки рано или поздно
// разъехался бы с ней. Поэтому из обеих сторон вычёркивается ровно то, что
// ненормативно: `reason` — всегда, `code` — только если ожидание его не
// объявило (старые URI-golden кодов не знают, и требовать их сейчас значило бы
// перегенерировать весь корпус).
func equalEnvelopeJSON(t *testing.T, got, want []byte) bool {
	t.Helper()
	gv := parseEnvelopeJSON(t, "got", got)
	wv := parseEnvelopeJSON(t, "want", want)
	normalizeDropsForCompare(gv, wv)
	return canonJSONString(t, gv) == canonJSONString(t, wv)
}

// parseEnvelopeJSON разбирает конверт в generic-значение.
func parseEnvelopeJSON(t *testing.T, side string, data []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("разбор %s: %v", side, err)
	}
	return v
}

// canonJSONString — канонический текст значения (тот же канон, что у файла).
func canonJSONString(t *testing.T, v any) string {
	t.Helper()
	b, err := canonMarshal(canonValue(v))
	if err != nil {
		t.Fatalf("канонизация: %v", err)
	}
	return string(b)
}

// normalizeDropsForCompare вычёркивает из `dropped[]` обеих сторон поля,
// сравнением не покрытые (D-088): `reason` всегда, `code` — когда ожидание
// его не объявляет.
func normalizeDropsForCompare(got, want any) {
	gotDrops := envelopeDrops(got)
	wantDrops := envelopeDrops(want)
	for i, d := range gotDrops {
		delete(d, "reason")
		// Ожидание без code — контракт на этот кейс кода не требует; чтобы
		// прогон не падал на «лишнем» поле, снимаем его и у результата.
		if i < len(wantDrops) {
			if _, ok := wantDrops[i]["code"]; !ok {
				delete(d, "code")
			}
			continue
		}
		delete(d, "code")
	}
	for _, d := range wantDrops {
		delete(d, "reason")
	}
}

// envelopeDrops достаёт записи `dropped[]` конверта как изменяемые карты.
func envelopeDrops(v any) []map[string]any {
	root, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	list, ok := root["dropped"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// equalJSON сравнивает по значению (CANON §7), не по байтам файла.
func equalJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		t.Fatalf("разбор got: %v", err)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		t.Fatalf("разбор want: %v", err)
	}
	ab, err := canonMarshal(canonValue(av))
	if err != nil {
		t.Fatalf("канонизация got: %v", err)
	}
	bb, err := canonMarshal(canonValue(bv))
	if err != nil {
		t.Fatalf("канонизация want: %v", err)
	}
	return string(ab) == string(bb)
}
