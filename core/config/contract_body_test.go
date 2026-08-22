package config

// Конформанс-раннер корпуса ТЕЛ подписки (SPEC 103, фаза 2).
//
// Гоняет contract/corpus/body/**/*.body через тот же путь, что боевой
// загрузчик: декодирование (base64) → классификация → разбор соответствующей
// ветки. Вход именно через слой декодирования, а не через кэш-хук: корпус
// обязан ловить регрессии классификатора, а не только парсеров.
//
// Результат — тот же конверт contractEnvelope, что у корпуса URI, поэтому
// ожидания читаются глазами и диффятся между приложениями.
//
// Регенерация:
//
//	go test ./core/config -run TestContractCorpusBody -update

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
)

// readCorpusBody возвращает тело фикстуры без ведущих строк-комментариев.
//
// Комментарии отрезаются ТОЛЬКО с начала файла: '#' внутри тела — часть
// данных (комментарий провайдера в URI-списке, fragment в URI), и вырезать
// его значило бы проверять не то тело, что лежит в фикстуре.
func readCorpusBody(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(data), "\n")
	start := 0
	for start < len(lines) {
		trimmed := strings.TrimSpace(lines[start])
		if strings.HasPrefix(trimmed, "#") {
			start++
			continue
		}
		break
	}
	return strings.Join(lines[start:], "\n")
}

// parseCorpusBody повторяет решения боевого загрузчика (source_loader.go)
// по одному телу и возвращает разобранные узлы.
func parseCorpusBody(t *testing.T, body string) ([]*configtypes.ParsedNode, subscription.BodyKind) {
	t.Helper()

	// Декодирование base64 идёт до классификации — ровно как в fetcher'е.
	decoded, err := subscription.DecodeSubscriptionContent([]byte(body))
	if err == nil && len(decoded) > 0 {
		body = string(decoded)
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.TrimSpace(body)

	kind := subscription.ClassifySubscriptionBody(body)

	switch {
	case kind == subscription.BodyKindVPNLink:
		nodes, _, err := subscription.ParseAmneziaVPNLinkAll(body, nil)
		if err != nil {
			return nil, kind
		}
		return nodes, kind

	case kind == subscription.BodyKindWGConf:
		uris, _ := subscription.WGConfBodyToURIs(body)
		return parseURILines(strings.Join(uris, "\n")), kind

	case kind.IsSingbox():
		res, err := subscription.ParseSingboxBody(body, kind, nil)
		if err != nil || res == nil {
			return nil, kind
		}
		return res.Nodes, kind

	case kind == subscription.BodyKindXrayArray:
		nodes, err := subscription.ParseNodesFromXrayJSONArray(body, nil)
		if err != nil {
			return nil, kind
		}
		return nodes, kind

	default:
		return parseURILines(body), kind
	}
}

// parseURILines разбирает построчный URI-список, пропуская пустые строки и
// комментарии — как это делает загрузчик.
func parseURILines(body string) []*configtypes.ParsedNode {
	var out []*configtypes.ParsedNode
	for _, line := range strings.Split(body, "\n") {
		line = subscription.NormalizeSubscriptionTextLine(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		node, err := subscription.ParseNode(line, nil)
		if err != nil || node == nil {
			continue
		}
		out = append(out, node)
	}
	return out
}

func TestContractCorpusBody(t *testing.T) {
	root := filepath.Join(contractCorpusRelPath, "body")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skipf("корпус контракта не найден: %s", root)
	}

	var cases []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".body") {
			cases = append(cases, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("обход корпуса тел: %v", err)
	}
	sort.Strings(cases)
	if len(cases) == 0 {
		t.Skip("корпус тел пуст")
	}

	for _, casePath := range cases {
		name := strings.TrimPrefix(filepath.ToSlash(strings.TrimSuffix(casePath, ".body")), filepath.ToSlash(root)+"/")
		t.Run(name, func(t *testing.T) {
			body := readCorpusBody(t, casePath)
			base := strings.TrimSuffix(casePath, ".body")

			nodes, kind := parseCorpusBody(t, body)

			env := contractEnvelope{V: 1, Meta: map[string]any{"body_kind": kind.String()}}
			for _, node := range nodes {
				cn, err := canonNode(node)
				if err != nil {
					env.Dropped = append(env.Dropped, contractDrop{Ref: node.Tag, Reason: "emit_error"})
					continue
				}
				env.Nodes = append(env.Nodes, cn)
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
			if !equalJSON(t, got, want) {
				t.Errorf("расхождение с контрактом\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}
