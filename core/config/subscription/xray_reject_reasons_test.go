package subscription

import (
	"strings"
	"testing"
)

// SPEC 115 — разбор обязан РАЗЛИЧАТЬ два класса отбраковки.
//
// Живой кейс: провайдер отдаёт валидный по форме Xray-конфиг с
// users[0].id == "" (подписка протухла, сервер вернул заглушку). До этого
// разделения такой элемент объявлялся «unsupported protocol "vless" skipped»
// — сообщение врало и уводило пользователя искать нехватку поддержки
// протокола вместо продления подписки.

// Пустой user id: причина настоящая, а НЕ «unsupported».
func TestXrayEmptyUserIDGivesRealReason(t *testing.T) {
	raw := `[{
	  "remarks": "expired-node",
	  "outbounds": [{
	    "protocol": "vless",
	    "tag": "proxy",
	    "settings": { "vnext": [{
	      "address": "v.test", "port": 443,
	      "users": [{ "id": "", "flow": "xtls-rprx-vision" }]
	    }] },
	    "streamSettings": { "network": "tcp", "security": "tls" }
	  }]
	}]`

	nodes, reasons, err := ParseNodesFromXrayJSONArrayEx(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("узел с пустым id прошёл разбор: %d узлов", len(nodes))
	}
	if len(reasons) == 0 {
		t.Fatal("разбор отбраковал узел и не назвал причины — пользователь снова остаётся ни с чем")
	}
	joined := strings.Join(reasons, " | ")
	if !strings.Contains(joined, "empty user id") {
		t.Errorf("причина = %q, ожидалась настоящая («empty user id…»)", joined)
	}
	if strings.Contains(joined, "unsupported protocol") {
		t.Errorf("битый элемент объявлен неподдерживаемым протоколом: %q", joined)
	}
	if !strings.Contains(joined, "vless") {
		t.Errorf("причина не называет протокол отбракованного outbound'а: %q", joined)
	}
}

// Неподдерживаемый протокол остаётся ОТДЕЛЬНЫМ классом: в список причин
// подписки он не попадает (чинить пользователю нечем — это свойство лаунчера,
// и место ему в логе, а не в отчёте о сломанной подписке).
func TestXrayUnsupportedProtocolStaysSeparate(t *testing.T) {
	raw := `[{
	  "remarks": "exotic",
	  "outbounds": [{
	    "protocol": "wireguard",
	    "tag": "proxy",
	    "settings": { "peers": [{ "endpoint": "w.test:51820" }] }
	  }]
	}]`

	nodes, reasons, err := ParseNodesFromXrayJSONArrayEx(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("неподдерживаемый протокол дал %d узлов", len(nodes))
	}
	if len(reasons) != 0 {
		t.Fatalf("неподдерживаемый протокол попал в причины подписки: %v", reasons)
	}
}

// Причины компактны: подписка с сотней одинаково битых элементов не должна
// превращать отчёт в стенограмму.
func TestXrayRejectReasonsAreCompact(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < 60; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"remarks":"n`)
		sb.WriteString(string(rune('a' + i%26)))
		sb.WriteString(`","outbounds":[{"protocol":"vless","tag":"p","settings":{"vnext":[{"address":"v.test","port":443,"users":[{"id":""}]}]},"streamSettings":{"network":"tcp"}}]}`)
	}
	sb.WriteString("]")

	nodes, reasons, err := ParseNodesFromXrayJSONArrayEx(sb.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("узлы с пустым id прошли разбор: %d", len(nodes))
	}
	if len(reasons) > MaxParseFailureReasons {
		t.Fatalf("причин %d, потолок %d — отчёт превратился в стенограмму",
			len(reasons), MaxParseFailureReasons)
	}
	if len(reasons) == 0 {
		t.Fatal("60 битых элементов не дали ни одной причины")
	}
}

// Чистая подписка причин не даёт: запись без повода приглашает искать поломку
// там, где её нет.
func TestXrayCleanArrayHasNoReasons(t *testing.T) {
	raw := `[{
	  "remarks": "ok-node",
	  "outbounds": [{
	    "protocol": "vless",
	    "tag": "proxy",
	    "settings": { "vnext": [{
	      "address": "v.test", "port": 443,
	      "users": [{ "id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" }]
	    }] },
	    "streamSettings": { "network": "tcp" }
	  }]
	}]`

	nodes, reasons, err := ParseNodesFromXrayJSONArrayEx(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("чистый элемент дал %d узлов, ожидался 1", len(nodes))
	}
	if len(reasons) != 0 {
		t.Fatalf("чистый разбор вернул причины: %v", reasons)
	}
}

// Накопитель причин: дедуп и потолок — свойства самого типа, а не случайность
// конкретного разбора.
func TestParseFailureReasonsDedupAndCap(t *testing.T) {
	var r ParseFailureReasons
	r.Add("a")
	r.Add("a")
	r.Add(" a ")
	if got := r.List(); len(got) != 1 {
		t.Fatalf("дубли не схлопнулись: %v", got)
	}
	for i := 0; i < 10; i++ {
		r.Add(strings.Repeat("x", i+1))
	}
	if got := len(r.List()); got != MaxParseFailureReasons {
		t.Fatalf("потолок не сработал: %d причин, ожидалось %d", got, MaxParseFailureReasons)
	}
	if !r.Truncated() {
		t.Error("причины отброшены, но признак усечения не выставлен")
	}
}
