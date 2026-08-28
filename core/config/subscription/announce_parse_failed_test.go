package subscription

import (
	"net/http"
	"strings"
	"testing"
)

// SPEC 115 — живой кейс: подписка отдаёт HTTP 200 с телом, из которого не
// собирается ни один узел, И заголовок `announce` с человекочитаемым
// объяснением. Раньше сообщение показывалось только в диалоге ОШИБКИ фетча —
// то есть при успешном фетче не показывалось нигде, хотя именно оно и есть
// лучший диагноз.
func TestParseHeaders_AnnounceOnContentfulResponse(t *testing.T) {
	h := http.Header{}
	// "⚠️ Произошла ошибка при получении подписки. Попробуйте позже или
	//  обратитесь в службу поддержки."
	h.Set("Announce", "base64:4pqg77iPINCf0YDQvtC40LfQvtGI0LvQsCDQvtGI0LjQsdC60LAg0L/RgNC4INC/0L7Qu9GD0YfQtdC90LjQuCDQv9C+0LTQv9C40YHQutC4LiDQn9C+0L/RgNC+0LHRg9C50YLQtSDQv9C+0LfQttC1INC40LvQuCDQvtCx0YDQsNGC0LjRgtC10YHRjCDQsiDRgdC70YPQttCx0YMg0L/QvtC00LTQtdGA0LbQutC4Lg==")

	m := ParseHeaders(h)
	if m.ProviderAnnounce == nil {
		t.Fatal("announce на успешном ответе не попал в метаданные — сообщение провайдера нигде не увидеть")
	}
	msg := m.ProviderAnnounce.AnnounceMessage()
	if !strings.Contains(msg, "Произошла ошибка при получении подписки") {
		t.Fatalf("announce декодирован как %q", msg)
	}
	if strings.HasPrefix(msg, "base64:") {
		t.Errorf("announce не раскодирован: %q", msg)
	}
}

// Plain-текст без base64-префикса принимается так же — провайдеры пишут и так.
func TestParseHeaders_AnnouncePlainOnContentfulResponse(t *testing.T) {
	h := http.Header{}
	h.Set("Announce", "Subscription temporarily unavailable, contact support.")

	m := ParseHeaders(h)
	if m.ProviderAnnounce == nil {
		t.Fatal("plain-announce не попал в метаданные")
	}
	if got := m.ProviderAnnounce.AnnounceMessage(); got != "Subscription temporarily unavailable, contact support." {
		t.Errorf("announce = %q", got)
	}
}

// Разбор такого тела даёт ноль узлов и НАСТОЯЩУЮ причину — вместе с announce
// они и составляют полный ответ пользователю (announce ставится первым выше по
// конвейеру, в генераторе).
func TestExpiredXrayBodyGivesZeroNodesAndReason(t *testing.T) {
	body := `[{
	  "remarks": "🇳🇱 Нидерланды",
	  "outbounds": [{
	    "protocol": "vless", "tag": "proxy",
	    "settings": { "vnext": [{ "address": "nl.test", "port": 443,
	      "users": [{ "id": "" }] }] },
	    "streamSettings": { "network": "tcp", "security": "tls" }
	  }]
	}]`

	nodes, reasons, err := ParseNodesFromXrayJSONArrayEx(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("протухшее тело дало %d узлов", len(nodes))
	}
	if len(reasons) == 0 {
		t.Fatal("протухшее тело не дало ни одной причины")
	}
	if !strings.Contains(strings.Join(reasons, " "), "subscription may be expired") {
		t.Errorf("причина не ведёт к протухшей подписке: %v", reasons)
	}
}
