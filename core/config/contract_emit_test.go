package config

// Конформанс-раннер ЭМИССИИ (SPEC 103, фаза 2): entry → share-URI → entry.
//
// Берёт весь корпус URI и для каждого разобранного узла проверяет round-trip:
// нода → share-URI → нода. Совпасть обязаны КАНОНИЧЕСКИЕ представления, а не
// строки URI: порядок query-параметров и регистр не нормируются контрактом
// (CANON §7), а вот потеря поля при эмиссии — баг.
//
// Почему поверх корпуса URI, а не отдельным набором фикстур: эмиттер обязан
// покрывать ровно то, что покрывает парсер. Отдельный набор неминуемо отстал
// бы — новая схема добавляется в парсер, а в emit-корпус её забывают
// (так и появились дыры masque/anytls/ssh — см. memory emitter-parser-pairing).
//
// Узлы, которые эмиттер осознанно не поддерживает (selector, urltest, direct,
// block), пропускаются: ErrShareURINotSupported — это контракт, а не отказ.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
)

// nodeToOutboundMap превращает разобранный узел в outbound-карту, какой её
// видит config.json, — вход эмиттера.
func nodeToOutboundMap(t *testing.T, node *configtypes.ParsedNode) (map[string]interface{}, bool) {
	t.Helper()
	// WireGuard живёт в endpoints[], а не в outbounds[] (sing-box >= 1.11), и
	// у него свой генератор. GenerateNodeJSON на WG-узле отдаёт обрубок
	// {tag,type,server,server_port} без ключей и peers — вызывать его здесь
	// значило бы проверять не тот путь.
	gen := GenerateNodeJSON
	if node.Scheme == "wireguard" || node.Scheme == "wg" {
		gen = GenerateEndpointJSON
	}
	raw, err := gen(node)
	if err != nil || strings.TrimSpace(raw) == "" {
		t.Fatalf("генерация узла: err=%v, raw=%q", err, raw)
	}
	// GenerateNodeJSON отдаёт ФРАГМЕНТ config.json: строка-комментарий с
	// именем узла, отступ и хвостовая запятая. Для эмиттера нужен чистый
	// объект — без этой чистки Unmarshal молча падал, и весь round-trip
	// превращался в 258 тихих скипов.
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(stripNodeJSONFragment(raw)), &out); err != nil {
		t.Fatalf("фрагмент узла не разбирается как JSON: %v\nfragment: %s", err, raw)
	}
	return out, true
}

// emitShareURI строит share-URI из узла: WireGuard живёт в endpoints[] и
// эмитится другой функцией (sing-box >= 1.11).
func emitShareURI(out map[string]interface{}) (string, error) {
	typ := strings.ToLower(strings.TrimSpace(mapString(out, "type")))
	if typ == "wireguard" {
		return subscription.ShareURIFromWireGuardEndpoint(out)
	}
	return subscription.ShareURIFromOutbound(out)
}

// stripNodeJSONFragment вырезает объект узла из фрагмента config.json.
func stripNodeJSONFragment(raw string) string {
	var kept []string
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		kept = append(kept, trimmed)
	}
	joined := strings.Join(kept, "\n")
	return strings.TrimSuffix(strings.TrimSpace(joined), ",")
}

// pathRoundTripLossy сообщает, что путь узла содержит процент-экранирование,
// которое декодер парсера снимает по D-028.
func pathRoundTripLossy(entry map[string]any) bool {
	tr, ok := entry["transport"].(map[string]any)
	if !ok {
		return false
	}
	p, _ := tr["path"].(string)
	return strings.Contains(p, "%")
}

// wsHostAddedFromSNI сообщает, что единственное отличие round-trip — это
// ws-заголовок Host, равный sni, которого в исходном узле не было.
func wsHostAddedFromSNI(want, got map[string]any) bool {
	wantTr, _ := want["transport"].(map[string]any)
	gotTr, _ := got["transport"].(map[string]any)
	if wantTr == nil || gotTr == nil {
		return false
	}
	if _, had := wantTr["headers"]; had {
		return false // Host был и раньше — расхождение настоящее
	}
	gotHeaders, ok := gotTr["headers"].(map[string]any)
	if !ok || len(gotHeaders) != 1 {
		return false
	}
	host, _ := gotHeaders["Host"].(string)
	tls, _ := want["tls"].(map[string]any)
	if tls == nil {
		return false
	}
	sni, _ := tls["server_name"].(string)
	if host == "" || host != sni {
		return false
	}
	// Всё остальное обязано совпасть: сравниваем узлы без заголовков.
	strippedGot := map[string]any{}
	for k, v := range got {
		strippedGot[k] = v
	}
	trCopy := map[string]any{}
	for k, v := range gotTr {
		if k != "headers" {
			trCopy[k] = v
		}
	}
	strippedGot["transport"] = trCopy
	a, _ := json.Marshal(strippedGot)
	b, _ := json.Marshal(want)
	var av, bv any
	_ = json.Unmarshal(a, &av)
	_ = json.Unmarshal(b, &bv)
	am, _ := json.Marshal(av)
	bm, _ := json.Marshal(bv)
	return string(am) == string(bm)
}

func mapString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func TestContractCorpusEmitRoundTrip(t *testing.T) {
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
	sort.Strings(cases)

	var emitted, skipped int
	for _, casePath := range cases {
		name := strings.TrimPrefix(filepath.ToSlash(strings.TrimSuffix(casePath, ".uri")), filepath.ToSlash(root)+"/")
		t.Run(name, func(t *testing.T) {
			uri := readCorpusURI(t, casePath)

			node, err := subscription.ParseNode(uri, nil)
			if err != nil || node == nil {
				t.Skip("кейс не парсится — round-trip неприменим (это фиксирует корпус URI)")
			}
			out, ok := nodeToOutboundMap(t, node)
			if !ok {
				t.Skip("узел не сериализуется в outbound")
			}

			shareURI, err := emitShareURI(out)
			if err != nil {
				if errors.Is(err, subscription.ErrShareURINotSupported) {
					skipped++
					t.Skipf("эмиссия не поддержана по контракту: %v", err)
				}
				t.Fatalf("эмиссия провалилась: %v", err)
			}
			emitted++

			// Обратный разбор: то, что эмиттер выдал, обязан принять парсер.
			// Это и есть cross-emit — ссылка, отданная пользователю, должна
			// импортироваться обратно (и в другое приложение тоже).
			back, err := subscription.ParseNode(shareURI, nil)
			if err != nil {
				t.Fatalf("свой же share-URI не парсится: %v\nURI: %s", err, shareURI)
			}
			if back == nil {
				t.Fatalf("свой же share-URI дал пустой узел\nURI: %s", shareURI)
			}

			gotCanon, err := canonNode(back)
			if err != nil {
				t.Fatalf("канонизация round-trip узла: %v", err)
			}
			wantCanon, err := canonNode(node)
			if err != nil {
				t.Fatalf("канонизация исходного узла: %v", err)
			}

			// By-design асимметрия D-028: парсер декодирует path ДО ДВУХ раз,
			// вылечивая подписки, где панель провайдера закодировала путь
			// дважды (`%2F%252F` → сервер видел бы `%252F` и отдавал 404).
			// Цена — путь, в котором `%2F` является ЗНАЧИМЫМ символом, после
			// round-trip схлопывается. Лечение живых подписок важнее точности
			// экзотического пути, поэтому кейс исключается из round-trip, а не
			// «чинится» отключением декодера.
			if pathRoundTripLossy(wantCanon.Entry) {
				t.Skip("D-028: путь с экранированным %2F — декодер лечит двойное кодирование, round-trip не обратим")
			}

			// By-design асимметрия ws-Host: узел БЕЗ Host-заголовка после
			// эмиссии несёт sni, а парсер подставляет ws-Host из sni, когда
			// своего нет («многие подписки задают только sni=, а обратный
			// прокси ждёт Host = vhost»). Терпимость к таким подпискам важнее
			// точности round-trip: без неё узел ловил бы 404 на реальном
			// сервере. Отличие только в добавленном Host, равном sni.
			if wsHostAddedFromSNI(wantCanon.Entry, gotCanon.Entry) {
				t.Skip("ws-Host подставлен из sni — терпимость парсера (node_parser_transport.go), не потеря данных")
			}

			gotJSON, _ := json.Marshal(gotCanon.Entry)
			wantJSON, _ := json.Marshal(wantCanon.Entry)
			if !equalJSON(t, gotJSON, wantJSON) {
				t.Errorf("round-trip потерял или изменил поля\nURI после эмиссии: %s\n--- got ---\n%s\n--- want ---\n%s",
					shareURI, gotJSON, wantJSON)
			}
		})
	}
	t.Logf("round-trip: %d эмитировано, %d не поддержано контрактом", emitted, skipped)
}
