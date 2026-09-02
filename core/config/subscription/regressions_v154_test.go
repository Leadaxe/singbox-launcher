// File regressions_v154_test.go — проверки за ревью diff v1.5.3..HEAD в части
// разбора тела: записи, которые исчезали молча, и счётчик капа, считавший не
// то, что обещал.
package subscription

import (
	"strings"
	"testing"
)

// Битый блок wg-quick (у [Peer] нет Endpoint) обязан остаться в составе
// узлом kind=unsupported — на СВОЕЙ позиции, с исходным текстом блока и
// причиной (модель W11). Раньше он лишь увеличивал skippedBlocks и пропадал:
// в составе на одну запись меньше, в origin.raw ничего, и починить
// провайдерский конфиг было не по чему.
func TestWGConfBrokenBlockBecomesUnsupportedRecord(t *testing.T) {
	// Фикстура своя, а не общая wgConfBody: ключи обязаны быть ВАЛИДНЫМ
	// base64, иначе целый блок отваливается на разборе и проверять
	// отбраковку второго не на чем.
	ok := "[Interface]\nPrivateKey = AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=\nAddress = 10.7.0.2/32\n\n[Peer]\nPublicKey = ICEiIyQlJicoKSorLC0uLzAxMjM0NTY3ODk6Ozw9Pj8=\nEndpoint = 198.51.100.10:51820\nAllowedIPs = 0.0.0.0/0\n"
	// Тот же блок, отличие ровно одно — у [Peer] нет Endpoint.
	broken := "[Interface]\nPrivateKey = QEFCQ0RFRkdISUpLTE1OT1BRUlNUVVZXWFlaW1xdXl8=\nAddress = 10.7.0.9/32\n\n[Peer]\nPublicKey = ICEiIyQlJicoKSorLC0uLzAxMjM0NTY3ODk6Ozw9Pj8=\nAllowedIPs = 0.0.0.0/0\n"
	body := ok + "\n" + broken

	pb, err := ParseSubscriptionBody([]byte(body), nil, 0)
	if err != nil {
		t.Fatalf("ParseSubscriptionBody: %v", err)
	}
	if len(pb.Entries) != 1 {
		t.Fatalf("принято %d записей, want 1 (целый блок)", len(pb.Entries))
	}
	if len(pb.Rejected) != 1 {
		t.Fatalf("отбраковано %d записей, want 1 — битый блок исчез молча", len(pb.Rejected))
	}

	r := pb.Rejected[0]
	// Позиция: запись стояла ПОСЛЕ одной принятой.
	if r.After != 1 {
		t.Errorf("After = %d, want 1 (битый блок стоял вторым)", r.After)
	}
	if r.OriginKind != OriginKindWGIni {
		t.Errorf("OriginKind = %q, want %q", r.OriginKind, OriginKindWGIni)
	}
	if !strings.Contains(r.OriginRaw, "10.7.0.9/32") {
		t.Errorf("OriginRaw не несёт исходный блок: %q", r.OriginRaw)
	}
	// Причина обязана быть ПРЕДМЕТНОЙ — от конвертации блока («нет Endpoint
	// у [Peer]»), а не побочной от разбора пустой ссылки: чинить
	// провайдерский конфиг человек будет по ней.
	if !strings.Contains(r.Reason, "endpoint") {
		t.Errorf("причина = %q, want предметную про отсутствующий Endpoint", r.Reason)
	}
	if strings.TrimSpace(r.Reason) == "" {
		t.Error("причина отбраковки пуста — чинить конфиг не по чему")
	}
}

// Кап считает ЗАПИСИ состава. Пока проверка стояла выше отсечек, каждый
// `#`-комментарий и каждый анонс провайдера после капа увеличивали счётчик
// «beyond the cap», и пользователь читал, что подписка обрезала на десятки
// узлов больше, чем узлов в теле вообще есть.
func TestCapCountsRecordsNotCommentsAndBanners(t *testing.T) {
	body := strings.Join([]string{
		"trojan://pw@a.test:443#A",
		"trojan://pw@b.test:443#B",
		// Всё, что ниже, идёт уже после капа в 1 запись.
		"# profile-title: whatever",
		"# just a comment",
		"Лучшие сервера",
	}, "\n")

	pb, err := ParseSubscriptionBody([]byte(body), nil, 1)
	if err != nil {
		t.Fatalf("ParseSubscriptionBody: %v", err)
	}
	if len(pb.Entries) != 1 {
		t.Fatalf("принято %d записей, want 1 (кап)", len(pb.Entries))
	}
	if !pb.Truncated {
		t.Fatal("тело не помечено обрезанным")
	}

	// Сводка обрезки обязана называть ОДНУ потерянную запись (узел B), а не
	// считать заодно комментарии и баннер.
	joined := strings.Join(pb.Warnings, " | ")
	if strings.Contains(joined, "4") || strings.Contains(joined, "3") {
		t.Errorf("в сводку капа попали комментарии и баннер: %q", joined)
	}
	if !strings.Contains(joined, "1") {
		t.Errorf("сводка капа не называет одну потерянную запись: %q", joined)
	}
}
