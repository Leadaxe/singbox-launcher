package subscription

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 113-A §2 — xray-ownership считается ПОДПИСЬЮ СОДЕРЖИМОГО, а не ключом
// по кредам.
//
// Находка аудита C2: ключ `схема|сервер|порт|креденшл` игнорировал транспорт и
// TLS, поэтому grpc-вариант сервера отбирал элемент у своего же xhttp-варианта,
// и один из них молча пропадал из подписки. Семантика возвращается к v1.5.0:
// схлопывается только БАЙТ-идентичная запись.
//
// Ловушка (SPEC 113-A §2): подпись зовёт эмиттер хуком
// LegacyNodeIdentityHashFunc, который в пакете subscription не установлен —
// без withContentSignatureHook подпись пуста и ownership выключен целиком.

// Два элемента, один сервер и один uuid, транспорты grpc и xhttp: разные
// способы пройти фильтрацию — обе записи обязаны выжить.
const xrayTransportTwinsSubscription = `[
  {
    "remarks": "DE gRPC",
    "outbounds": [{"protocol":"vless","tag":"proxy","settings":{"vnext":[
      {"address":"1.1.1.1","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]},
      "streamSettings":{"network":"grpc","security":"tls",
        "tlsSettings":{"serverName":"a.example"},
        "grpcSettings":{"serviceName":"svc"}}}]
  },
  {
    "remarks": "DE xhttp",
    "outbounds": [{"protocol":"vless","tag":"proxy","settings":{"vnext":[
      {"address":"1.1.1.1","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]},
      "streamSettings":{"network":"xhttp","security":"tls",
        "tlsSettings":{"serverName":"a.example"},
        "xhttpSettings":{"path":"/x"}}}]
  }
]`

func TestXrayOwnershipKeepsTransportTwins(t *testing.T) {
	withContentSignatureHook(t)

	nodes, err := ParseNodesFromXrayJSONArray(xrayTransportTwinsSubscription, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("получено %d узлов, ожидалось 2 — grpc и xhttp одного сервера это разные записи (теги: %v)",
			len(nodes), tagsOfNodes(nodes))
	}
}

// Байт-идентичная копия: элемент-страна и пул перечисляют ОДИН И ТОТ ЖЕ узел.
// Имя достаётся стране (элемент специфичнее), пул ссылается на её тег.
const xrayByteCopySubscription = `[
  {
    "remarks": "🇪🇺 Авто | Лучший сервер",
    "outbounds": [
      {"protocol":"vless","tag":"proxy-1-1-1-1-direct","settings":{"vnext":[
        {"address":"1.1.1.1","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]},
        "streamSettings":{"network":"tcp","security":"tls","tlsSettings":{"serverName":"a.example"}}},
      {"protocol":"vless","tag":"proxy-2-2-2-2-direct","settings":{"vnext":[
        {"address":"2.2.2.2","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]},
        "streamSettings":{"network":"tcp","security":"tls","tlsSettings":{"serverName":"b.example"}}}
    ],
    "routing": {"balancers": [{"tag":"Auto_Balancer","selector":["proxy"]}]}
  },
  {
    "remarks": "🇩🇪 Германия",
    "outbounds": [{"protocol":"vless","tag":"proxy","settings":{"vnext":[
      {"address":"1.1.1.1","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]},
      "streamSettings":{"network":"tcp","security":"tls","tlsSettings":{"serverName":"a.example"}}}]
  },
  {
    "remarks": "🇫🇮 Финляндия",
    "outbounds": [{"protocol":"vless","tag":"proxy","settings":{"vnext":[
      {"address":"2.2.2.2","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]},
      "streamSettings":{"network":"tcp","security":"tls","tlsSettings":{"serverName":"b.example"}}}]
  }
]`

func TestXrayOwnershipCollapsesByteCopiesIntoCountry(t *testing.T) {
	withContentSignatureHook(t)

	nodes, err := ParseNodesFromXrayJSONArray(xrayByteCopySubscription, nil)
	if err != nil {
		t.Fatal(err)
	}

	tags := tagsOfNodes(nodes)
	// Группа + две страны, техтегов пула быть не должно.
	if len(nodes) != 3 {
		t.Fatalf("получено %d узлов, ожидалось 3 (группа + 2 страны): %v", len(nodes), tags)
	}
	for _, tag := range tags {
		if contains(tag, "proxy-") {
			t.Fatalf("технический тег пула просочился: %q (все: %v)", tag, tags)
		}
	}

	groups := groupNodesOf(nodes)
	if len(groups) != 1 {
		t.Fatalf("ожидалась 1 группа, получено %d (%v)", len(groups), tags)
	}
	members := groupMembersOf(groups[0])
	if len(members) != 2 {
		t.Fatalf("состав группы = %v, ожидалось 2 члена", members)
	}
	alive := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		alive[n.Tag] = true
	}
	for _, m := range members {
		if !alive[m] {
			t.Fatalf("группа ссылается на %q — такого узла нет (%v)", m, tags)
		}
	}
	if members[0] != "🇩🇪-Германия" || members[1] != "🇫🇮-Финляндия" {
		t.Fatalf("состав = %v, ожидались итоговые теги стран", members)
	}
}

// Без хука подписи ownership выключается целиком и элементы выпускают свои
// узлы как есть. Пин на осознанность деградации: пустая подпись — «ключа нет»,
// а не «все узлы одинаковые».
func TestXrayOwnershipDisabledWithoutSignatureHook(t *testing.T) {
	prev := LegacyNodeIdentityHashFunc
	LegacyNodeIdentityHashFunc = nil
	t.Cleanup(func() { LegacyNodeIdentityHashFunc = prev })

	nodes, err := ParseNodesFromXrayJSONArray(xrayByteCopySubscription, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Ни один узел не схлопнут: 2 из пула + 2 страны + группа.
	if len(nodes) != 5 {
		t.Fatalf("получено %d узлов, ожидалось 5 — без подписи ownership не работает (%v)",
			len(nodes), tagsOfNodes(nodes))
	}
}

// xrayServerKey — та же подпись, что у дедупа записей: от тега не зависит,
// у группы пуста.
func TestXrayServerKeyIsContentSignature(t *testing.T) {
	withContentSignatureHook(t)

	newNode := func(tag string) *configtypes.ParsedNode {
		return &configtypes.ParsedNode{
			Tag: tag, Scheme: "vless", Server: "1.1.1.1", Port: 443, UUID: "u1",
			Outbound: map[string]interface{}{"type": "vless", "tag": tag, "server": "1.1.1.1"},
		}
	}
	a := newNode("🇩🇪 Германия")
	b := newNode("proxy-1-1-1-1-direct")
	if xrayServerKey(a) != xrayServerKey(b) {
		t.Fatalf("один узел под двумя именами дал разные подписи: %q и %q",
			xrayServerKey(a), xrayServerKey(b))
	}
	if xrayServerKey(a) != dedupSignature(a) {
		t.Fatal("ownership и дедуп записей обязаны считать один ключ")
	}
}
