package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// SPEC 113-A §1 — эмиттер в режиме «голый JSON» и подпись узлов, чьё имя
// содержит фигурные скобки.
//
// Находка аудита C3: подпись вырезалась из строки config.json поиском первой
// `{`, а перед объектом печаталась строка-комментарий с ИМЕНЕМ узла. Имя
// «SG {премиум} 1» уводило разбор внутрь комментария, json.Unmarshal падал и
// узел оставался без подписи — молча, навсегда.

func braceNamedNode(tag string) *ParsedNode {
	n := vlessNodeForIdentity(tag)
	n.Label = tag
	return n
}

// Голый режим отдаёт РОВНО объект: ни комментария, ни таба, ни запятой.
func TestGenerateNodeJSONBareIsPlainObject(t *testing.T) {
	bare, err := GenerateNodeJSONBare(vlessNodeForIdentity("node"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(bare, "{") || !strings.HasSuffix(bare, "}") {
		t.Fatalf("голый режим вернул не объект: %q", bare)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(bare), &obj); err != nil {
		t.Fatalf("голый JSON не разбирается: %v (%q)", err, bare)
	}
}

// Обёртка config.json осталась прежней: комментарий с именем, таб, запятая.
// Форма файла — контракт для того, кто правит конфиг руками.
func TestGenerateNodeJSONKeepsConfigWrapping(t *testing.T) {
	node := vlessNodeForIdentity("node")
	node.Label = "🇩🇪 DE"
	wrapped, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatal(err)
	}
	bare, err := GenerateNodeJSONBare(node)
	if err != nil {
		t.Fatal(err)
	}
	want := "\t// 🇩🇪 DE\n\t" + bare + ","
	if wrapped != want {
		t.Fatalf("обёртка config.json изменилась:\n получено %q\n ожидалось %q", wrapped, want)
	}
}

// Endpoint (wireguard) в голом режиме — тоже чистый объект, без строки
// комментария.
func TestGenerateEndpointJSONBareHasNoComment(t *testing.T) {
	node := &ParsedNode{
		Tag:     "wg",
		Scheme:  "wireguard",
		Comment: "заметка",
		Outbound: map[string]interface{}{
			"type":        "wireguard",
			"server":      "1.2.3.4",
			"server_port": 51820,
			"private_key": "k",
		},
	}
	bare, err := GenerateEndpointJSONBare(node)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(bare, "//") {
		t.Fatalf("в голом endpoint осталась строка комментария: %q", bare)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(bare), &obj); err != nil {
		t.Fatalf("голый endpoint не разбирается: %v", err)
	}
}

// Регресс C3: у узла с `{` в имени подпись ЕСТЬ. На старом коде (вырезание
// обёртки поиском первой `{`) разбор начинался внутри имени и хеш был пустым.
//
// Отметки, поставленные на такие узлы СТАРЫМИ версиями, после этой правки не
// находятся by design: старые версии подписи для них не считали вообще — не с
// чем совпадать, терять нечего.
func TestLegacyNodeIdentityHashHandlesBracesInName(t *testing.T) {
	names := []string{"SG {премиум} 1", "}{", `{"json":1}`}
	for _, name := range names {
		node := braceNamedNode(name)
		h := LegacyNodeIdentityHash(node)
		if len(h) != 64 {
			t.Fatalf("узел %q: подпись = %q, ожидались 64 hex-символа", name, h)
		}
	}
}

// Подпись НЕ различает узлы по имени — это её контракт (tag/detour вычеркнуты),
// и скобки его не отменяют: три узла с одинаковым содержимым и разными
// именами-ловушками дают ОДИН хеш. Отдельный пин, потому что соблазн проверить
// «имена дали разные подписи» ведёт ровно в обратную сторону от SPEC 113-A §1.
func TestLegacyNodeIdentityHashIsNameBlindForBracedNames(t *testing.T) {
	names := []string{"SG {премиум} 1", "}{", `{"json":1}`}
	first := LegacyNodeIdentityHash(braceNamedNode(names[0]))
	for _, name := range names[1:] {
		if got := LegacyNodeIdentityHash(braceNamedNode(name)); got != first {
			t.Fatalf("узлы %q и %q дали разные подписи (%q vs %q) — имя протекло в хеш",
				names[0], name, first, got)
		}
	}
}

// ЛОВУШКА SPEC 113-A §1 — стабильность хешей. Переход на голый режим НЕ имеет
// права сдвинуть подпись ни на бит: по ней миграция находит старые ключи
// disabled-отметок и detour_node_hash из state.json, записанных до SPEC 112.
//
// Значения сняты со СТАРОГО кода (вырезание обёртки поиском первой `{`) и
// сверены с новым. Правка этих литералов означает, что у пользователей
// протухли отметки — так делать нельзя, чините эмиттер.
func TestLegacyNodeIdentityHashByteStablePins(t *testing.T) {
	perScheme := vlessNodeForIdentity("node")

	manual := vlessNodeForIdentity("manual")
	manual.EmitRaw = true
	manual.Outbound["packet_encoding"] = "xudp"

	group := &ParsedNode{
		Tag:    "auto",
		Scheme: SchemeGroup,
		Label:  "Авто",
		Outbound: map[string]interface{}{
			"type":          "urltest",
			GroupMembersKey: []interface{}{"a", "b"},
			"url":           "http://x",
		},
	}

	pins := []struct {
		name string
		node *ParsedNode
		want string
	}{
		{"per-scheme vless", perScheme, "52f6f3b6aaaa8fefb3a6b4d4ee621f43a4c56063c6dc70e2327d0d13936f2980"},
		{"ручной config_json", manual, "ba75a8c45cdc6d78c188c459a3c99c3c8c3aafb46a58b0afa9a0143cbf052f4e"},
		{"узел-группа", group, "3f004995c6f5d20b78e9411a2d7ae716f41e2021900354b12f3493da23dc8f16"},
	}
	for _, p := range pins {
		if got := LegacyNodeIdentityHash(p.node); got != p.want {
			t.Errorf("%s: подпись = %s, ожидалась %s — миграция перестанет находить старые отметки",
				p.name, got, p.want)
		}
	}
}

// Имя в подпись не входит: узлы со скобками и без обязаны совпасть по хешу,
// раз всё остальное у них одинаковое.
func TestLegacyNodeIdentityHashIgnoresBraceName(t *testing.T) {
	plain := vlessNodeForIdentity("node")
	braced := braceNamedNode("SG {премиум} 1")
	// Tag в хеш не входит, но Outbound["tag"] у vlessNodeForIdentity разный —
	// он и обязан быть вычеркнут вместе с tag.
	if got, want := LegacyNodeIdentityHash(braced), LegacyNodeIdentityHash(plain); got != want {
		t.Fatalf("имя со скобками увело подпись: %q vs %q", got, want)
	}
}
