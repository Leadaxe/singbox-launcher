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
	"encoding/json"
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
// по одному телу и возвращает разобранные узлы вместе с отбраковками.
//
// Отбраковки — часть контракта тела, а не деталь реализации: тело, где запись
// объявлена, но непригодна (dialerProxy на несуществующий outbound), обязано
// дать НОЛЬ узлов и одну отбраковку. Без них конверт «пустой nodes[]» был бы
// неотличим от конверта «тело не распознано» — а разница ровно в том, узнала
// ли сторона запись и осознанно её отвергла.
func parseCorpusBody(t *testing.T, body string) ([]*configtypes.ParsedNode, []contractDrop, subscription.BodyKind) {
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
			return nil, nil, kind
		}
		return nodes, nil, kind

	case kind == subscription.BodyKindWGConf:
		uris, _ := subscription.WGConfBodyToURIs(body)
		return parseURILines(strings.Join(uris, "\n")), nil, kind

	case kind.IsSingbox():
		res, err := subscription.ParseSingboxBody(body, kind, nil)
		if err != nil || res == nil {
			return nil, nil, kind
		}
		return res.Nodes, nil, kind

	case kind == subscription.BodyKindXrayArray:
		nodes, err := subscription.ParseNodesFromXrayJSONArray(body, nil)
		if err != nil {
			return nil, nil, kind
		}
		return nodes, corpusXrayDrops(body), kind

	default:
		return parseURILines(body), nil, kind
	}
}

// corpusXrayDrops достаёт поштучные отбраковки Xray-тела.
//
// Сам разбор идёт через ParseNodesFromXrayJSONArray — тем же входом, что у
// боевого загрузчика; отбраковки этот вход не отдаёт (они нужны только
// материализации), поэтому за ними раннер ходит вторым проходом через
// ParseSubscriptionBody. Второй проход детерминирован и дешевле, чем
// расширение публичной сигнатуры парсера ради одного корпуса.
func corpusXrayDrops(body string) []contractDrop {
	pb, err := subscription.ParseSubscriptionBody([]byte(body), nil, 0)
	if err != nil || pb == nil {
		return nil
	}
	var out []contractDrop
	for _, rec := range pb.Rejected {
		// Ref у JSON-ветки — тег отбракованного outbound'а: он же связывает
		// отбраковку с записью тела, которую видно глазами.
		out = append(out, contractDrop{
			Ref:    corpusRejectRef(rec.OriginRaw),
			Code:   rec.Code,
			Reason: rec.Reason,
		})
	}
	return out
}

// corpusRejectRef достаёт тег из исходника отбракованной JSON-записи.
func corpusRejectRef(originRaw string) string {
	var ob struct {
		Tag string `json:"tag"`
	}
	if err := json.Unmarshal([]byte(originRaw), &ob); err != nil {
		return ""
	}
	return ob.Tag
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

// corpusExtensionMark читает пометку meta.extension из существующего ожидания.
//
// Ожидания генерирует раннер, но эта пометка приходит не из разбора, а от
// автора кейса, поэтому единственный способ её не потерять — прочитать из
// файла, который сейчас будет перезаписан. Файла нет (новый кейс) — пометки
// нет: заводится она правкой ожидания руками, один раз.
func corpusExtensionMark(expPath string) string {
	data, err := os.ReadFile(expPath)
	if err != nil {
		return ""
	}
	var envelope struct {
		Meta struct {
			Extension string `json:"extension"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return ""
	}
	return envelope.Meta.Extension
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

			nodes, drops, kind := parseCorpusBody(t, body)

			env := contractEnvelope{V: 1, Meta: map[string]any{"body_kind": kind.String()}}
			// meta.extension — свойство КЕЙСА, а не результата разбора: им
			// помечено тело со схемой, которой у одной из сторон нет (раннер
			// той стороны кейс пропускает). Раннер лаунчера его не вычисляет,
			// поэтому переносит из существующего ожидания — иначе -update
			// стирал бы метку, а обычный прогон падал бы на «лишнем» поле.
			if ext := corpusExtensionMark(expectedPathFor(base)); ext != "" {
				env.Meta["extension"] = ext
			}
			for _, node := range nodes {
				cn, err := canonNode(node)
				if err != nil {
					env.Dropped = append(env.Dropped, contractDrop{Ref: node.Tag, Reason: "emit_error"})
					continue
				}
				env.Nodes = append(env.Nodes, cn)
			}
			env.Dropped = append(env.Dropped, drops...)

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
