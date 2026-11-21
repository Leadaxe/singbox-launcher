package config

// Паритет вида ссылки бинарного контейнера (контракт 1.1.175).
//
// Круг `parse(emit(body)) == body` сторожит СОДЕРЖАНИЕ ссылки, но не её вид:
// поля в другом порядке или лишний тег читаются нашим разбором так же. Вид
// же есть де-факто формат чужих клиентов (MAPPER_ENGINE §9.1), и у контейнера
// TLV он задан энкодером AdGuard (deeplink/src/encode.rs): ссылки этих кейсов
// собраны им, и обратный ход обязан вернуть их байт в байт.

import (
	"path/filepath"
	"testing"

	"singbox-launcher/core/config/subscription"
)

func TestContractTLVEmitByteParity(t *testing.T) {
	// Кейсы, где тело хранит всё, что несёт ссылка: потерь на круге нет.
	cases := []string{
		"http2_basic",
		"ipv6_address",
		"cert_chain_two",
		"tls_profile_firefox",
		"tls_profile_okhttp",
	}
	root := filepath.Join(contractCorpusRelPath, "uri", "trusttunnel")
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			uri := readCorpusURI(t, filepath.Join(root, name+".uri"))
			node, err := subscription.ParseNode(uri, nil)
			if err != nil || node == nil {
				t.Fatalf("кейс не разобран: %v", err)
			}
			out, ok := nodeToOutboundMap(t, node)
			if !ok {
				t.Fatalf("узел не сериализуется в outbound")
			}
			got, err := emitShareURI(out)
			if err != nil {
				t.Fatalf("эмиссия: %v", err)
			}
			if got != uri {
				t.Fatalf("вид ссылки разошёлся с энкодером AdGuard\n ждали: %s\n вышло: %s", uri, got)
			}
		})
	}
}
