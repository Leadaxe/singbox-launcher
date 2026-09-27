package config

// Конформанс-раннер корпуса АВТОРСКИХ тел (контракт 1.1.87,
// PARSING_PRINCIPLES §10): contract/corpus/authored/<case>.body — голое тело
// узла sing-box, сохранённое как свой сервер. Мягкие правила реестра тело не
// меняют и дают коды с applied:false, жёсткие (`core_rejects`) применяются.
//
// Регенерация:
//
//	go test ./core/config -run TestContractCorpusAuthored -update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

func TestContractCorpusAuthored(t *testing.T) {
	root := filepath.Join(contractCorpusRelPath, "authored")
	cases, _ := filepath.Glob(filepath.Join(root, "*.body"))
	sort.Strings(cases)
	if len(cases) == 0 {
		t.Fatalf("корпус авторских тел пуст: %s", root)
	}
	for _, casePath := range cases {
		name := strings.TrimSuffix(filepath.Base(casePath), ".body")
		t.Run(name, func(t *testing.T) {
			body := strings.TrimSpace(readCorpusBody(t, casePath))
			node, err := subscription.NodeFromManualConfigJSON([]byte(body))
			if err != nil {
				t.Fatalf("NodeFromManualConfigJSON: %v", err)
			}
			if !node.Authored {
				t.Fatalf("ручной объект обязан быть авторским")
			}
			env := contractEnvelope{V: 1, Meta: map[string]any{"container": "own"}}
			// meta.extension — свойство кейса (как у корпуса body): hysteria v1
			// только у лаунчера, LxBox такой кейс пропускает (контракт 1.1.96).
			if ext := corpusExtensionMark(expectedPathFor(strings.TrimSuffix(casePath, ".body"))); ext != "" {
				env.Meta["extension"] = ext
			}
			cn, code, err := canonNodeDrop(node)
			if err != nil {
				env.Dropped = append(env.Dropped, contractDrop{Ref: node.Tag, Index: dropIndex(0), Code: code, Reason: "emit_error"})
			} else {
				env.Nodes = append(env.Nodes, cn)
			}
			got, err := marshalEnvelopePretty(env)
			if err != nil {
				t.Fatalf("сериализация: %v", err)
			}
			expPath := expectedPathFor(strings.TrimSuffix(casePath, ".body"))
			if *updateContractGolden {
				if err := os.WriteFile(expPath, got, 0o644); err != nil {
					t.Fatalf("запись %s: %v", expPath, err)
				}
				return
			}
			want, err := os.ReadFile(expPath)
			if err != nil {
				t.Fatalf("нет expected (%s)", filepath.Base(expPath))
			}
			// Сравнение СТРОГОЕ, по значению: `applied` нормативен, и
			// снисходительная сверка (поле, не объявленное ожиданием,
			// срезается) пропустила бы applied:false у жёсткого правила.
			var gv, wv interface{}
			_ = json.Unmarshal(got, &gv)
			if err := json.Unmarshal(want, &wv); err != nil {
				t.Fatalf("expected не JSON: %v", err)
			}
			gb, _ := json.Marshal(gv)
			wb, _ := json.Marshal(wv)
			if string(gb) != string(wb) {
				t.Errorf("расхождение с контрактом\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}
