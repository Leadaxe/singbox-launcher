package config

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// SPEC 101 → SPEC 112 → SPEC 112-A — ссылка на узел как ОБЪЕКТ
// («source_id + identity-тег»), резолв в финальный тег на проходе 2 сборки,
// плюс миграция упразднённого detour_node_hash.

const tagDetourHopURI = "masque://cHJpdmF0ZQ==@1.1.1.1:443?publickey=cHVi&sni=example.com&address=172.16.0.2%2F32#hop"

const tagDetourChainedURI = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@p.example.com:443?encryption=none&security=tls&sni=p.example.com#proton"

func tagDetourParserConfig(t *testing.T, sources ...ProxySource) *ParserConfig {
	t.Helper()
	pc := &ParserConfig{}
	pc.ParserConfig.Proxies = sources
	return pc
}

func parseNodeForTagDetour(t *testing.T, uri string) *ParsedNode {
	t.Helper()
	n, err := subscription.ParseNode(uri, nil)
	if err != nil {
		t.Fatalf("parse %q: %v", uri, err)
	}
	return n
}

// The happy path: a source chained through a hop node addressed by
// «source_id + identity-тег» gets the hop's FINAL tag stamped on every node.
func TestResolveNodeDetours_StampsFinalTag(t *testing.T) {
	hop := parseNodeForTagDetour(t, tagDetourHopURI)
	hop.IdentityTag = "hop"
	hop.Tag = "renamed-by-prefix:hop-7" // финальный тег ≠ идентичности
	hop.Outbound["tag"] = hop.Tag
	hop.SourceIndex = 0

	chained := parseNodeForTagDetour(t, tagDetourChainedURI)
	chained.SourceIndex = 1

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01HOP", Connections: []string{tagDetourHopURI}},
		ProxySource{ID: "01SUB", Connections: []string{"..."},
			DetourNodeSourceID: "01HOP", DetourNodeTag: "hop"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {chained}}
	all, _ := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{hop, chained})

	if len(all) != 2 {
		t.Fatalf("no nodes may be dropped, got %d", len(all))
	}
	if got, _ := chained.Outbound["detour"].(string); got != hop.Tag {
		t.Errorf("detour = %q, want hop FINAL tag %q", got, hop.Tag)
	}
	if _, ok := hop.Outbound["detour"]; ok {
		t.Error("hop itself must keep dialing directly")
	}
}

// Ключевой выигрыш SPEC 112-A: смена tag_prefix источника-ЦЕЛИ меняет
// финальный тег хопа, но ссылку не рвёт — она хранит идентичность, а тег
// вычисляется. До 112-A хранимый финальный тег тут протухал, и весь
// зависимый источник выпадал fail-closed.
func TestResolveNodeDetours_SurvivesTargetTagPrefixChange(t *testing.T) {
	hop := parseNodeForTagDetour(t, tagDetourHopURI)
	hop.IdentityTag = "hop"
	// Пользователь дописал источнику-цели префикс — тег узла стал другим.
	hop.Tag = "NEW-PFX: hop"
	hop.Outbound["tag"] = hop.Tag
	hop.SourceIndex = 0

	chained := parseNodeForTagDetour(t, tagDetourChainedURI)
	chained.SourceIndex = 1

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01HOP", Connections: []string{tagDetourHopURI}, TagPrefix: "NEW-PFX: "},
		ProxySource{ID: "01SUB", Connections: []string{"..."},
			DetourNodeSourceID: "01HOP", DetourNodeTag: "hop"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {chained}}
	all, _ := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{hop, chained})

	if len(all) != 2 {
		t.Fatalf("смена tag_prefix цели не должна ронять источник, осталось %d узлов", len(all))
	}
	if got, _ := chained.Outbound["detour"].(string); got != hop.Tag {
		t.Errorf("detour = %q, ожидался ПЕРЕСЧИТАННЫЙ тег %q", got, hop.Tag)
	}
}

// Правка содержимого хопа ссылку не рвёт (критерий приёмки 3): адресуется имя,
// а не отпечаток подключения.
func TestResolveNodeDetours_SurvivesHopContentEdit(t *testing.T) {
	hop := parseNodeForTagDetour(t, tagDetourHopURI)
	hop.IdentityTag = "hop"
	hop.SourceIndex = 0
	// Пользователь поправил mtu и SNI — узел тот же.
	hop.Outbound["mtu"] = 1280
	hop.Outbound["server_name"] = "other.example.com"

	chained := parseNodeForTagDetour(t, tagDetourChainedURI)
	chained.SourceIndex = 1

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01HOP", Connections: []string{tagDetourHopURI}},
		ProxySource{ID: "01SUB", Connections: []string{"..."},
			DetourNodeSourceID: "01HOP", DetourNodeTag: "hop"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {chained}}
	all, _ := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{hop, chained})

	if len(all) != 2 {
		t.Fatalf("правка содержимого хопа не должна ронять источник, осталось %d узлов", len(all))
	}
	if got, _ := chained.Outbound["detour"].(string); got != hop.Tag {
		t.Errorf("detour = %q, want %q", got, hop.Tag)
	}
}

// Резолв СТРОГИЙ (дополнение SPEC 112-A, отменяющее «id главнее»): узел в
// источнике переименован — ссылка НЕ разрешается, источник выпадает
// fail-closed. За честность отвечает UI, который сбрасывает ссылку в момент
// переименования (см. business.ResetDetourNodeRefs).
func TestResolveNodeDetours_RenamedIdentityFailsClosed(t *testing.T) {
	hop := parseNodeForTagDetour(t, tagDetourHopURI)
	hop.IdentityTag = "hop-renamed" // пользователь сменил node_tag
	hop.Tag = "hop-renamed"
	hop.SourceIndex = 0

	chained := parseNodeForTagDetour(t, tagDetourChainedURI)
	chained.SourceIndex = 1

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01HOP", Connections: []string{tagDetourHopURI}},
		ProxySource{ID: "01SUB", Connections: []string{"..."},
			DetourNodeSourceID: "01HOP", DetourNodeTag: "hop"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {chained}}
	all, _ := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{hop, chained})

	if len(all) != 1 || all[0] != hop {
		t.Fatalf("ссылка на исчезнувшую идентичность обязана падать fail-closed; осталось %d узлов", len(all))
	}
	if _, ok := chained.Outbound["detour"]; ok {
		t.Error("узел выпавшего источника не должен нести detour")
	}
}

// Провайдер переименовал узел подписки — «пиновка принятой цены»: ссылка
// fail-closed, а не переезжает на другой узел того же источника.
func TestResolveNodeDetours_SubscriptionNodeRenamedByProviderFailsClosed(t *testing.T) {
	// В подписке два узла; тот, на который ссылались, теперь зовётся иначе.
	stayed := parseNodeForTagDetour(t, tagDetourHopURI)
	stayed.IdentityTag = "🇳🇱 Amsterdam-2"
	stayed.Tag = "AL: 🇳🇱 Amsterdam-2"
	stayed.SourceIndex = 0

	chained := parseNodeForTagDetour(t, tagDetourChainedURI)
	chained.SourceIndex = 1

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01LIB", Source: "https://example.com/liberty", Label: "AL: Liberty"},
		ProxySource{ID: "01PROTON", Source: "https://example.com/proton", Label: "Proton NL",
			DetourNodeSourceID: "01LIB", DetourNodeTag: "🇳🇱 Amsterdam-1"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {stayed}, 1: {chained}}
	all, _ := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{stayed, chained})

	if len(all) != 1 || all[0] != stayed {
		t.Fatalf("исчезнувший узел подписки обязан ронять зависимый источник; осталось %d узлов", len(all))
	}
}

// Тексты диагностики называют ОБЕ стороны человеческими именами (SPEC 112-A,
// «Понятные ошибки»). Проверяется наличие имён, не формулировка.
func TestDetourFailureMessage_NamesBothSides(t *testing.T) {
	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01LIB", Source: "https://example.com/liberty", Label: "AL: Liberty"},
		ProxySource{ID: "01PROTON", Source: "https://example.com/proton", Label: "Proton NL",
			DetourNodeSourceID: "01LIB", DetourNodeTag: "🇳🇱 Amsterdam-1"},
	)
	idx := buildNodeRefIndex(pc, map[int][]*ParsedNode{}, nil)

	msg := detourFailureMessage(pc, idx, pc.ParserConfig.Proxies[1], 1)
	for _, want := range []string{"AL: Liberty", "🇳🇱 Amsterdam-1", "Proton NL"} {
		if !strings.Contains(msg, want) {
			t.Errorf("в сообщении нет %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "01LIB") {
		t.Errorf("ULID не должен попадать в текст, пока у источника есть имя: %s", msg)
	}

	// Источник-цель удалён: имени не осталось, id — единственное, чем его
	// можно назвать, и он обязан быть в тексте.
	pc.ParserConfig.Proxies[1].DetourNodeSourceID = "01GONE"
	idx = buildNodeRefIndex(pc, map[int][]*ParsedNode{}, nil)
	msg = detourFailureMessage(pc, idx, pc.ParserConfig.Proxies[1], 1)
	for _, want := range []string{"01GONE", "Proton NL"} {
		if !strings.Contains(msg, want) {
			t.Errorf("в сообщении об удалённом источнике нет %q: %s", want, msg)
		}
	}
}

// Fail-closed: неразрешимая ссылка роняет узлы зависимого источника, а не
// пускает его трафик напрямую.
func TestResolveNodeDetours_UnresolvedDropsSource(t *testing.T) {
	hop := parseNodeForTagDetour(t, tagDetourHopURI)
	hop.SourceIndex = 0
	chained := parseNodeForTagDetour(t, tagDetourChainedURI)
	chained.SourceIndex = 1

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01HOP", Connections: []string{tagDetourHopURI}},
		ProxySource{ID: "01SUB", Connections: []string{"..."},
			DetourNodeSourceID: "01GONE", DetourNodeTag: "gone-hop"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {chained}}
	all, _ := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{hop, chained})

	if len(all) != 1 || all[0] != hop {
		t.Fatalf("chained source must be dropped, hop kept; got %d node(s)", len(all))
	}
	if _, ok := nodesBySource[1]; ok {
		t.Error("dropped source must leave nodesBySource (selectors would reference ghosts)")
	}
}

// Переходная ссылка без source_id (dev-состояния между SPEC 112 и 112-A)
// резолвится глобальным поиском по ФИНАЛЬНОМУ тегу — дословно как раньше.
func TestResolveNodeDetours_TagOnlyRefResolvesGlobally(t *testing.T) {
	hop := parseNodeForTagDetour(t, tagDetourHopURI)
	hop.Tag = "prefixed:hop"
	hop.IdentityTag = "hop"
	hop.Outbound["tag"] = hop.Tag
	hop.SourceIndex = 0

	chained := parseNodeForTagDetour(t, tagDetourChainedURI)
	chained.SourceIndex = 1

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01HOP", Connections: []string{tagDetourHopURI}},
		// source_id пуст — тег трактуется как финальный.
		ProxySource{ID: "01SUB", Connections: []string{"..."}, DetourNodeTag: "prefixed:hop"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {chained}}
	all, _ := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{hop, chained})

	if len(all) != 2 {
		t.Fatalf("tag-only ссылка обязана резолвиться глобально, осталось %d узлов", len(all))
	}
	if got, _ := chained.Outbound["detour"].(string); got != hop.Tag {
		t.Errorf("detour = %q, want %q", got, hop.Tag)
	}
}

// A wireguard endpoint can be the chained side too — but listen_port nodes stay
// unstamped: the core rejects detour+listen_port and one such node would kill
// the whole config.
func TestResolveNodeDetours_WireGuardChained(t *testing.T) {
	hop := parseNodeForTagDetour(t, tagDetourHopURI)
	hop.IdentityTag = "hop"
	hop.SourceIndex = 0

	wg := parseNodeForTagDetour(t,
		"wireguard://UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=@1.2.3.4:51820?publickey=QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU=&address=10.0.0.2/32&allowedips=0.0.0.0/0#wg")
	wg.SourceIndex = 1
	wgListen := parseNodeForTagDetour(t,
		"wireguard://UFNLMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=@1.2.3.4:51820?publickey=QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU=&address=10.0.0.3/32&allowedips=0.0.0.0/0&listenport=51999#wgl")
	wgListen.SourceIndex = 1

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01HOP", Connections: []string{tagDetourHopURI}},
		ProxySource{ID: "01SUB", Connections: []string{"..."},
			DetourNodeSourceID: "01HOP", DetourNodeTag: "hop"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {wg, wgListen}}
	all, _ := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{hop, wg, wgListen})

	if len(all) != 3 {
		t.Fatalf("nothing may be dropped, got %d", len(all))
	}
	if got, _ := wg.Outbound["detour"].(string); got != hop.Tag {
		t.Errorf("wireguard node detour = %q, want %q", got, hop.Tag)
	}
	if _, ok := wgListen.Outbound["detour"]; ok {
		t.Error("wireguard node with listen_port must stay unstamped")
	}
}

// Группа хопом по этому пути быть не может: chaining через selector — задача
// DetourTag (SPEC 077). Тег группы здесь не резолвится, источник падает
// fail-closed.
func TestResolveNodeDetours_GroupIsNotACandidate(t *testing.T) {
	group := &ParsedNode{Tag: "🚀 Авто", IdentityTag: "🚀 Авто", Scheme: SchemeGroup, SourceIndex: 0,
		Outbound: map[string]interface{}{"type": "urltest", "tag": "🚀 Авто"}}
	chained := parseNodeForTagDetour(t, tagDetourChainedURI)
	chained.SourceIndex = 1

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01GRP", Source: "https://example.com/sub"},
		ProxySource{ID: "01SUB", Connections: []string{"..."},
			DetourNodeSourceID: "01GRP", DetourNodeTag: "🚀 Авто"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {group}, 1: {chained}}
	all, _ := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{group, chained})

	if len(all) != 1 || all[0] != group {
		t.Fatalf("источник, сославшийся на группу, обязан упасть fail-closed; осталось %d узлов", len(all))
	}
}

// --- Миграция SPEC 112 → 112-A ------------------------------------------

// Legacy-хеш, который ещё опознаётся: ссылка переезжает на ПОЛНЫЙ ref —
// id источника узла плюс его identity-тег.
func TestMigrateLegacyDetourNodeHash_ResolvedByHashWritesFullRef(t *testing.T) {
	hop := parseNodeForTagDetour(t, tagDetourHopURI)
	hash := LegacyNodeIdentityHash(hop)
	if hash == "" {
		t.Fatal("legacy-хеш хопа пуст")
	}
	hop.IdentityTag = "hop"
	hop.Tag = "prefixed:hop"
	hop.Outbound["tag"] = hop.Tag
	hop.SourceIndex = 0

	chained := parseNodeForTagDetour(t, tagDetourChainedURI)
	chained.SourceIndex = 1

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01HOP", Connections: []string{tagDetourHopURI}},
		ProxySource{ID: "01SUB", Connections: []string{"..."},
			DetourNodeHash: hash, DetourNodeLabel: "старая подпись"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {chained}}
	all, _ := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{hop, chained})

	if len(all) != 2 {
		t.Fatalf("миграция не должна ронять источник, осталось %d узлов", len(all))
	}
	if got, _ := chained.Outbound["detour"].(string); got != hop.Tag {
		t.Errorf("detour = %q, want %q", got, hop.Tag)
	}
	migrated := pc.ParserConfig.Proxies[1]
	if migrated.DetourNodeSourceID != "01HOP" {
		t.Errorf("DetourNodeSourceID = %q, want %q", migrated.DetourNodeSourceID, "01HOP")
	}
	if migrated.DetourNodeTag != "hop" {
		t.Errorf("DetourNodeTag = %q, want identity %q", migrated.DetourNodeTag, "hop")
	}
	if migrated.DetourNodeHash != "" {
		t.Errorf("detour_node_hash обязан гаснуть после миграции, остался %q", migrated.DetourNodeHash)
	}
}

// Критерий приёмки 1 — стейт IRA: узел лежит как uri+config_json, хеш от него
// не совпадает с записанным (формы хранения хешировались по-разному), а
// DetourNodeLabel несёт ТЕГ хопа. Ссылка обязана вылечиться по label —
// tag-only ref'ом, без source_id: узел под хешем не опознан, и приписывать
// ссылке чужой источник нельзя.
func TestMigrateLegacyDetourNodeHash_LabelFallbackHealsIRAState(t *testing.T) {
	const hopTag = "🔥🎭 WARP (MASQUE)"

	hop := parseNodeForTagDetour(t, tagDetourHopURI)
	hop.Tag = hopTag
	hop.IdentityTag = hopTag
	hop.Outbound["tag"] = hopTag
	hop.Outbound["mtu"] = 1280 // правка, из-за которой хеш и разъехался
	hop.SourceIndex = 0

	chained := parseNodeForTagDetour(t, tagDetourChainedURI)
	chained.Tag = "🇳🇱 Proton NL"
	chained.Outbound["tag"] = chained.Tag
	chained.SourceIndex = 1

	// Протухший хеш из боевого стейта — под него ни один узел не подходит.
	const staleHash = "62bff800" + "0000000000000000000000000000000000000000000000000000000000"

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01WARP", Connections: []string{tagDetourHopURI}, TagMask: hopTag},
		ProxySource{
			ID:              "01PROTON",
			Source:          "https://example.com/proton",
			DetourNodeHash:  staleHash[:64],
			DetourNodeLabel: hopTag,
		},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {chained}}
	all, _ := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{hop, chained})

	if len(all) != 2 {
		t.Fatalf("Proton NL обязан остаться в конфиге, осталось %d узлов", len(all))
	}
	if got, _ := chained.Outbound["detour"].(string); got != hopTag {
		t.Fatalf(`detour у Proton NL = %q, ожидался %q`, got, hopTag)
	}
	if got := pc.ParserConfig.Proxies[1].DetourNodeTag; got != hopTag {
		t.Errorf("DetourNodeTag = %q, want %q", got, hopTag)
	}
	if got := pc.ParserConfig.Proxies[1].DetourNodeSourceID; got != "" {
		t.Errorf("label-fallback обязан давать tag-only ref, а source_id = %q", got)
	}
	if got := pc.ParserConfig.Proxies[1].DetourNodeHash; got != "" {
		t.Errorf("detour_node_hash обязан гаснуть, остался %q", got)
	}
}

// Ни хеш не опознан, ни подписи нет — ссылка теряется, источник падает
// fail-closed (трафик не уходит напрямую).
func TestMigrateLegacyDetourNodeHash_NoLabelFailsClosed(t *testing.T) {
	hop := parseNodeForTagDetour(t, tagDetourHopURI)
	hop.SourceIndex = 0
	chained := parseNodeForTagDetour(t, tagDetourChainedURI)
	chained.SourceIndex = 1

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01HOP", Connections: []string{tagDetourHopURI}},
		ProxySource{ID: "01SUB", Connections: []string{"..."},
			DetourNodeHash: strings.Repeat("f", 64)},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {chained}}
	all, _ := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{hop, chained})

	if len(all) != 1 || all[0] != hop {
		t.Fatalf("источник без опознанной ссылки обязан выпасть; осталось %d узлов", len(all))
	}
}
