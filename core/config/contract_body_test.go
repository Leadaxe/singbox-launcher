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
	"singbox-launcher/core/config/linkmap"
	"singbox-launcher/core/config/registry"
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
		// Узлы собирает СЕКЦИЯ из самих блоков — как и боевой разбор тела
		// (SPEC 133). Прежний путь гонял блок через промежуточную ссылку и
		// терял на обратном переводе то, чего в ссылке нет: код
		// `wgconf_dns_ignored` (DNS в тело не едет вовсе).
		converted, _ := subscription.WGConfBodyToConvertedBlocks(body)
		nodes := make([]*configtypes.ParsedNode, 0, len(converted))
		for _, block := range converted {
			if block.Err != nil || block.Node == nil {
				continue
			}
			nodes = append(nodes, block.Node)
		}
		return nodes, nil, kind

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

// corpusDropIndexes проставляет отбраковкам тела `index` — позицию элемента
// в нарезке `elements` вида источника (source_kinds.json, CANON §4).
//
// Нарезку делает движок (`linkmap.ClassifySource`) по той же таблице, что
// читают обе стороны, — раннер свою не выдумывает. Отбраковка находит свой
// элемент по `ref`: у JSON-элемента это `tag`, у текстового — сама строка.
// Элемент, уже занятый предыдущей отбраковкой, пропускается: два outbound'а
// с одним тегом получают два разных адреса по порядку появления.
func corpusDropIndexes(t *testing.T, body string, drops []contractDrop) {
	t.Helper()
	if len(drops) == 0 {
		return
	}
	set, err := registry.LoadMappers()
	if err != nil {
		t.Fatalf("LoadMappers: %v", err)
	}
	res := linkmap.ClassifySource(set, body, map[string]linkmap.Unwrapper{
		"base64_utf8": func(text string) ([]string, error) {
			raw, err := subscription.DecodeSubscriptionContent([]byte(text))
			if err != nil {
				return nil, err
			}
			return []string{string(raw)}, nil
		},
	})
	claimed := make([]bool, len(res.Elements))
	for i := range drops {
		found := -1
		for j, el := range res.Elements {
			if !claimed[j] && corpusElementRef(el) == drops[i].Ref {
				found = j
				break
			}
		}
		// Тег узла мог быть ВЫВЕДЕН (Xray берёт его из remarks элемента-
		// конфига, а не из outbound'а). Тогда элемент ищется по делу: тот,
		// из которого движок собирает то же тело, что у отвергнутого узла.
		if found < 0 && drops[i].node != nil && res.Kind.Mapper != nil {
			for j, el := range res.Elements {
				if !claimed[j] && corpusElementBuilds(t, *res.Kind.Mapper, el, drops[i].node) {
					found = j
					break
				}
			}
		}
		if found < 0 {
			t.Errorf("отбраковка %q: элемента в нарезке вида %q нет", drops[i].Ref, res.Kind.SourceKind)
			continue
		}
		claimed[found] = true
		drops[i].Index = dropIndex(found)
	}
}

// corpusElementBuilds — собирает ли движок из элемента тело отвергнутого узла.
//
// Сравниваются ключи тела, которые дал маппер: сборка документа потом
// дописывает своё (тег, цепочку), и полное равенство здесь не нужно — нужен
// ответ «этот ли элемент». Имён схем нет: секцию выбирает detect реестра.
func corpusElementBuilds(t *testing.T, kind string, el linkmap.SourceElement, node *configtypes.ParsedNode) bool {
	t.Helper()
	if el.Value == nil || node == nil || node.Outbound == nil {
		return false
	}
	plans, err := linkmap.Planes()
	if err != nil {
		t.Fatalf("linkmap.Planes: %v", err)
	}
	reg, err := registry.Get()
	if err != nil {
		t.Fatalf("registry.Get: %v", err)
	}
	scheme, plan, ok := linkmap.SelectElementSection(plans, kind, el.Value)
	if !ok {
		return false
	}
	res, err := linkmap.ParseElement(plan, el.Value, reg.SingboxType(scheme), nil)
	if err != nil || res == nil || len(res.Body) == 0 {
		return false
	}
	for k, v := range res.Body {
		if k == "tag" || k == "type" {
			continue
		}
		if corpusJSONText(v) != corpusJSONText(node.Outbound[k]) {
			return false
		}
	}
	return true
}

// corpusJSONText — значение в JSON-тексте: снимает разницу int/float64 между
// телом движка и телом узла.
func corpusJSONText(v interface{}) string {
	b, _ := json.Marshal(v)
	var back interface{}
	_ = json.Unmarshal(b, &back)
	out, _ := json.Marshal(back)
	return string(out)
}

// corpusElementRef — чем элемент нарезки называет себя в `ref` отбраковки.
func corpusElementRef(el linkmap.SourceElement) string {
	if m, ok := el.Value.(map[string]interface{}); ok {
		tag, _ := m["tag"].(string)
		return tag
	}
	return strings.TrimSpace(el.Text)
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
		t.Fatalf("корпус контракта не найден: %s", root)
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
	// Ноль кейсов — ОТКАЗ, а не пропуск (см. TestContractCorpusURI).
	if len(cases) == 0 {
		t.Fatalf("корпус тел пуст: %s не дал ни одного .body", root)
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
				cn, code, err := canonNodeDrop(node)
				if err != nil {
					// `code` нормативен, `reason` — нет (D-088): см.
					// canonNodeDrop.
					env.Dropped = append(env.Dropped, contractDrop{Ref: node.Tag, Code: code, Reason: "emit_error", node: node})
					continue
				}
				env.Nodes = append(env.Nodes, cn)
			}
			env.Dropped = append(env.Dropped, drops...)
			corpusDropIndexes(t, body, env.Dropped)
			requireDropCodes(t, env)

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
