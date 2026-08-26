package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// SPEC 112, критерий приёмки 1 — стейт IRA целиком, через боевой конвейер.
//
// Форма боевого состояния: server-источник «🔥🎭 WARP (MASQUE)» лежит и как
// `uri`, и как `config_json` (в этой ветке парсер берёт ручной JSON), а
// подписка Proton ссылается на него полем `detour_node_hash` со значением
// `62bff800…`, которое ни одному узлу больше не соответствует — формы хранения
// хешировались по-разному. `detour_node_label` при этом несёт ТЕГ хопа.
//
// Ожидание: после сборки Proton NL в конфиге присутствует и его detour
// указывает на тег хопа. До SPEC 112 весь источник Proton выпадал fail-closed.
//
// Реальный стейт машины при этом не трогается — данные здесь синтетические,
// воспроизводится только форма.

const iraHopTag = "🔥🎭 WARP (MASQUE)"

// Значение из боевого стейта, дополненное до 64 hex: ни один узел под него
// не подходит — в этом и суть кейса.
const iraStaleDetourHash = "62bff800" + "1111111111111111111111111111111111111111111111111111111111"

// ULID источников — SPEC 112-A адресует ссылку парой «source_id + тег».
const (
	iraHopSourceID    = "01KXWARP0000000000000000"
	iraProtonSourceID = "01KXPROTON00000000000000"
)

func iraProtonBody() string {
	return "vless://b831381d-6324-4d53-ad4f-8cda48b30811@nl.proton.example:443?encryption=none&security=tls&sni=nl.proton.example#🇳🇱 Proton NL"
}

// iraParserConfig собирает конфиг ровно той формы, что в боевом состоянии.
func iraParserConfig(detourHash, detourLabel, detourSourceID, detourTag string) *ParserConfig {
	pc := &ParserConfig{}
	pc.ParserConfig.Version = ParserConfigVersion
	pc.ParserConfig.Proxies = []ProxySource{
		{
			// Хоп: и URI, и ручной config_json — как в стейте IRA.
			ID: iraHopSourceID,
			Connections: []string{
				"masque://cHJpdmF0ZQ==@1.1.1.1:443?publickey=cHVi&sni=example.com&address=172.16.0.2%2F32#warp",
			},
			ConfigJSON: json.RawMessage(`{
				"type": "socks",
				"server": "127.0.0.1",
				"server_port": 40000,
				"version": "5"
			}`),
			TagMask: iraHopTag,
		},
		{
			ID:                 iraProtonSourceID,
			Label:              "Proton NL",
			Source:             "https://example.invalid/proton",
			DetourNodeHash:     detourHash,
			DetourNodeLabel:    detourLabel,
			DetourNodeSourceID: detourSourceID,
			DetourNodeTag:      detourTag,
		},
	}
	return pc
}

// iraLoadNodes отдаёт узлы боевым загрузчиком: тело подписки подсовывается
// хуком кэша, ручной config_json разбирается штатной веткой.
func iraLoadNodes(t *testing.T) func(ProxySource, map[string]int, func(float64, string), int, int) ([]*ParsedNode, error) {
	t.Helper()
	prev := subscription.LookupCachedBody
	subscription.LookupCachedBody = func(url string) ([]byte, bool) {
		if url == "https://example.invalid/proton" {
			return []byte(iraProtonBody()), true
		}
		return nil, false
	}
	t.Cleanup(func() { subscription.LookupCachedBody = prev })

	return func(ps ProxySource, tagCounts map[string]int, cb func(float64, string), i, total int) ([]*ParsedNode, error) {
		return subscription.LoadNodesFromSource(ps, tagCounts, cb, i, total)
	}
}

// emittedNodeByTag достаёт из результата сборки эмитированный outbound по тегу.
func emittedNodeByTag(t *testing.T, res *OutboundGenerationResult, tag string) map[string]interface{} {
	t.Helper()
	for _, raw := range append(append([]string(nil), res.OutboundsJSON...), res.EndpointsJSON...) {
		start := strings.Index(raw, "{")
		if start < 0 {
			continue
		}
		body := strings.TrimSuffix(strings.TrimSpace(raw[start:]), ",")
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(body), &obj); err != nil {
			continue
		}
		if got, _ := obj["tag"].(string); got == tag {
			return obj
		}
	}
	return nil
}

func TestIRAState_StaleDetourHashHealsByLabel(t *testing.T) {
	pc := iraParserConfig(iraStaleDetourHash, iraHopTag, "", "")

	res, err := GenerateOutboundsFromParserConfig(
		pc, map[string]int{}, nil, iraLoadNodes(t), DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("сборка провалилась (источник Proton выпал fail-closed?): %v", err)
	}

	proton := emittedNodeByTag(t, res, "🇳🇱 Proton NL")
	if proton == nil {
		tags := make([]string, 0, len(res.OutboundsJSON))
		for _, raw := range res.OutboundsJSON {
			tags = append(tags, strings.TrimSpace(raw))
		}
		t.Fatalf("Proton NL отсутствует в конфиге — миграция по label не сработала\n%s",
			strings.Join(tags, "\n"))
	}
	if got, _ := proton["detour"].(string); got != iraHopTag {
		t.Fatalf(`detour у Proton NL = %q, ожидался %q`, got, iraHopTag)
	}

	// И сама ссылка переписана на тег: следующая сборка пойдёт уже без миграции.
	if got := pc.ParserConfig.Proxies[1].DetourNodeTag; got != iraHopTag {
		t.Errorf("DetourNodeTag = %q, ожидался %q", got, iraHopTag)
	}
	if got := pc.ParserConfig.Proxies[1].DetourNodeHash; got != "" {
		t.Errorf("detour_node_hash обязан гаснуть после миграции, остался %q", got)
	}
}

// Уже мигрировавшее состояние (detour_node_tag) собирается тем же путём —
// повторная миграция не нужна и ничего не портит.
func TestIRAState_MigratedTagKeepsWorking(t *testing.T) {
	pc := iraParserConfig("", iraHopTag, iraHopSourceID, iraHopTag)

	res, err := GenerateOutboundsFromParserConfig(
		pc, map[string]int{}, nil, iraLoadNodes(t), DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("сборка провалилась: %v", err)
	}
	proton := emittedNodeByTag(t, res, "🇳🇱 Proton NL")
	if proton == nil {
		t.Fatal("Proton NL отсутствует в конфиге")
	}
	if got, _ := proton["detour"].(string); got != iraHopTag {
		t.Fatalf(`detour у Proton NL = %q, ожидался %q`, got, iraHopTag)
	}
}

// Контроль: без подписи и без опознанного хеша источник обязан выпасть
// fail-closed — трафик не уходит напрямую.
func TestIRAState_NoLabelStillFailsClosed(t *testing.T) {
	pc := iraParserConfig(iraStaleDetourHash, "", "", "")

	res, err := GenerateOutboundsFromParserConfig(
		pc, map[string]int{}, nil, iraLoadNodes(t), DirectionBuildOptions{})
	if err != nil {
		return // все узлы выпали — тоже честный fail-closed
	}
	if proton := emittedNodeByTag(t, res, "🇳🇱 Proton NL"); proton != nil {
		t.Fatalf("Proton NL остался в конфиге без разрешённого хопа: %+v", proton)
	}
}
