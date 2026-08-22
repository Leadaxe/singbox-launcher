package subscription

import "testing"

// SPEC 103 §9.B12: Amnezia vpn://-ссылка как целое тело подписки.
//
// Профиль синтетический: два WG/AWG-контейнера (дефолтный — awg), ключи —
// заглушки правильной длины, хосты из example-N.com.
const testVPNProfileLink = "vpn://AAADPnjarZLfS8MwEMf_lZJn13XVwRzsYagPczAK-rYMuSXXGUiTkmSbWvq_e-kehsikoneB8L0f3PHhGiasCaAMOs-mybo5a5IMKoMfCgZw3LErkvRNk4Zp8OGFCksVNWs4OwlOirP1wgR0JQjccG4Kpw4QcInvySyB3jafUe9cSofeU-MoSydplubD65zi96unGEs7J01vXSC6bt5-q5U4jdv2tm7cg5G1VSZQJ75BVWscjFJhq-l4NMmzuI_W9ohyUcSVsrTzYUwUhE_5gCYsEWvQ6oBUkY_jbgSOs0dxYnPDWcva9iq5APqoHO724GTEfRZ_gi5623fot7-HLnvbJej5P0CPlDfEUGIJex3ufrhqiV44VQdlTcw-ow9J4WypNMb0q_VhBRXG3JezYO0nIBQX8w"

// vpn:// распознаётся ПЕРВЫМ — до base64-эвристики: ':' не входит в
// base64-алфавит, и без явной ветки тело ушло бы в декодер как битый base64.
func TestClassifyVPNLinkBody(t *testing.T) {
	if got := ClassifySubscriptionBody(testVPNProfileLink); got != BodyKindVPNLink {
		t.Fatalf("ClassifySubscriptionBody = %v, want BodyKindVPNLink", got)
	}
	if got := ClassifySubscriptionBody("  " + testVPNProfileLink + "\n"); got != BodyKindVPNLink {
		t.Fatalf("с пробельными краями: %v", got)
	}
}

// Профиль с несколькими локациями — штатный случай Amnezia. Импортируются
// ВСЕ контейнеры: одиночный ParseNode берёт один лишь потому, что отдаёт одну
// ноду, а не потому, что остальные не нужны.
func TestParseAmneziaVPNLinkAllReturnsEveryContainer(t *testing.T) {
	nodes, skipped, err := ParseAmneziaVPNLinkAll(testVPNProfileLink, nil)
	if err != nil {
		t.Fatalf("ParseAmneziaVPNLinkAll: %v", err)
	}
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}
	if len(nodes) != 2 {
		t.Fatalf("получено %d узлов, ожидалось 2", len(nodes))
	}
	// Дефолтный контейнер идёт первым — порядок детерминирован.
	if nodes[0].Server != "example-1.com" {
		t.Errorf("первый узел %q, ожидался example-1.com (defaultContainer)", nodes[0].Server)
	}
	if nodes[1].Server != "example-2.com" {
		t.Errorf("второй узел %q, ожидался example-2.com", nodes[1].Server)
	}
	// Метки различимы: иначе MakeTagUnique размножит одинаковые теги в
	// «…-2»/«…-3», и пользователь не отличит локации друг от друга.
	if nodes[0].Tag == nodes[1].Tag {
		t.Errorf("теги совпали (%q) — локации неразличимы", nodes[0].Tag)
	}
}

// Одиночный путь ParseNode сохраняет прежнее поведение: одна нода,
// дефолтный контейнер.
func TestParseNodeVPNLinkStillSingle(t *testing.T) {
	node, err := ParseNode(testVPNProfileLink, nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	if node == nil {
		t.Fatal("узел не разобран")
	}
	if node.Server != "example-1.com" {
		t.Errorf("server = %q, ожидался example-1.com (defaultContainer)", node.Server)
	}
}

// Несжатый профиль: голый base64(JSON) без qCompress-фрейминга. Amnezia так
// экспортирует часть ссылок; до фазы 2 Go отвергал их как битый zlib
// (§9.B12 — расхождение с LxBox, который принимал обе формы).
const testVPNPlainJSONLink = "vpn://eyJjb250YWluZXJzIjogW3siY29udGFpbmVyIjogImFtbmV6aWEtd2lyZWd1YXJkIiwgIndpcmVndWFyZCI6IHsibGFzdF9jb25maWciOiAie1wiY29uZmlnXCI6IFwiW0ludGVyZmFjZV1cXG5Qcml2YXRlS2V5ID0gZWVlZWVlZWVlZWVlZWVlZWVlZWVlZWVlZWVlZWVlZWVlZWVlZWVlZWVlRT1cXG5BZGRyZXNzID0gMTAuNS4wLjIvMzJcXG5cXG5bUGVlcl1cXG5QdWJsaWNLZXkgPSBmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZFPVxcbkVuZHBvaW50ID0gZXhhbXBsZS00LmNvbTo1MTgyMFxcbkFsbG93ZWRJUHMgPSAwLjAuMC4wLzBcXG5cIn0ifX1dLCAiZGVmYXVsdENvbnRhaW5lciI6ICJhbW5lemlhLXdpcmVndWFyZCIsICJkZXNjcmlwdGlvbiI6ICJQbGFpbiBKU09OIFByb2ZpbGUifQ"

func TestParseAmneziaVPNLinkUncompressedProfile(t *testing.T) {
	nodes, _, err := ParseAmneziaVPNLinkAll(testVPNPlainJSONLink, nil)
	if err != nil {
		t.Fatalf("несжатый профиль отвергнут: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("получено %d узлов, ожидался 1", len(nodes))
	}
	if nodes[0].Server != "example-4.com" {
		t.Errorf("server = %q, ожидался example-4.com", nodes[0].Server)
	}
}
