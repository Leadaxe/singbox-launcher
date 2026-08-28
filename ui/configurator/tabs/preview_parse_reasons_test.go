package tabs

import (
	"strings"
	"testing"
)

// SPEC 115 — вкладка Preview окна источника обязана объяснять пустоту.
//
// Раньше она показывала «0 server(s) from 1 source(s)» и «No servers found.» —
// то есть дважды повторяла факт пустоты и молчала о причине. Превью разбирает
// тело своим путём (кэшированный .raw, без сети), поэтому причины обязаны
// доезжать и по нему тоже, а не только по боевой сборке.

// Протухшая подписка: ноль узлов И настоящая причина.
func TestPreviewParseReasonsOnExpiredBody(t *testing.T) {
	body := []byte(`[{
	  "remarks": "🇳🇱 Нидерланды",
	  "outbounds": [{
	    "protocol": "vless", "tag": "proxy",
	    "settings": { "vnext": [{ "address": "nl.test", "port": 443,
	      "users": [{ "id": "" }] }] },
	    "streamSettings": { "network": "tcp", "security": "tls" }
	  }]
	}]`)

	nodes, reasons := parsePreviewNodesFromBodyEx(body, nil)
	if len(nodes) != 0 {
		t.Fatalf("протухшее тело дало %d узлов", len(nodes))
	}
	if len(reasons) == 0 {
		t.Fatal("превью не объяснило пустоту — вкладка снова тупик")
	}
	joined := strings.Join(reasons, " | ")
	if !strings.Contains(joined, "empty user id") {
		t.Errorf("причина = %q, ожидалась настоящая", joined)
	}
	if strings.Contains(joined, "unsupported protocol") {
		t.Errorf("битый элемент объявлен неподдерживаемым протоколом: %q", joined)
	}
}

// Чистое тело причин не даёт, и блок причин не строится: пустая рамка над
// списком серверов — шум.
func TestPreviewNoReasonsOnCleanBody(t *testing.T) {
	body := []byte(`[{
	  "remarks": "ok",
	  "outbounds": [{
	    "protocol": "vless", "tag": "proxy",
	    "settings": { "vnext": [{ "address": "nl.test", "port": 443,
	      "users": [{ "id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" }] }] },
	    "streamSettings": { "network": "tcp" }
	  }]
	}]`)

	nodes, reasons := parsePreviewNodesFromBodyEx(body, nil)
	if len(nodes) != 1 {
		t.Fatalf("чистое тело дало %d узлов, ожидался 1", len(nodes))
	}
	if len(reasons) != 0 {
		t.Fatalf("чистое тело дало причины: %v", reasons)
	}
	if block := previewParseReasonsBlock(reasons); block != nil {
		t.Error("блок причин построен без причин")
	}
}

// Битые URI построчной подписки тоже объясняются: до этого строка молча
// пропускалась, и превью показывало пустой список без повода.
func TestPreviewParseReasonsOnBrokenURIList(t *testing.T) {
	body := []byte("vless://not-a-valid-uri\nvmess://@@@broken@@@\n")

	nodes, reasons := parsePreviewNodesFromBodyEx(body, nil)
	if len(nodes) != 0 {
		t.Fatalf("битые URI дали %d узлов", len(nodes))
	}
	if len(reasons) == 0 {
		t.Fatal("битые URI не дали ни одной причины")
	}
}
