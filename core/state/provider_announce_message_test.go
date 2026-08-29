package state

import (
	"strings"
	"testing"
)

// SPEC 115 — сообщение провайдера показывается КАК ДАННЫЕ: чужой текст, чью
// длину и форму мы не контролируем.

func TestAnnounceMessageCollapsesAndTruncates(t *testing.T) {
	long := strings.Repeat("很长的消息 ", 400) // заведомо больше потолка, многобайтные руны
	a := &ProviderAnnounce{Message: "  первая строка\n\nвторая\tстрока  "}

	if got := a.AnnounceMessage(); got != "первая строка вторая строка" {
		t.Errorf("переводы строк не схлопнулись: %q", got)
	}

	a = &ProviderAnnounce{Message: long}
	got := a.AnnounceMessage()
	if len([]rune(got)) > MaxAnnounceRunes+1 { // +1 — многоточие
		t.Errorf("сообщение длиной %d рун прошло потолок %d",
			len([]rune(got)), MaxAnnounceRunes)
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("обрезанное сообщение не помечено многоточием — читается как полное")
	}
}

func TestAnnounceMessageEmptyIsEmpty(t *testing.T) {
	var nilAnn *ProviderAnnounce
	if got := nilAnn.AnnounceMessage(); got != "" {
		t.Errorf("nil-announce дал %q", got)
	}
	if got := (&ProviderAnnounce{}).AnnounceMessage(); got != "" {
		t.Errorf("пустой announce дал %q", got)
	}
	// Только URL, без текста — показывать нечего.
	if got := (&ProviderAnnounce{URL: "https://t.me/bot"}).AnnounceMessage(); got != "" {
		t.Errorf("announce без текста дал %q", got)
	}
}

// Сообщение провайдера доезжает до сборочной формы источника: разбору
// метаданные недоступны, и без этого провоза он о сообщении не узнает.
func TestProxySourceCarriesProviderAnnounce(t *testing.T) {
	src := &Source{
		Node:  Node{Kind: SourceKindSubscription, Enabled: true},
		ID:    "01SUB",
		Label: "AL: Liberty",
		URL:   "https://example.com/sub",
		Meta: &SubMeta{
			ProviderAnnounce: &ProviderAnnounce{
				Message: "⚠️ Произошла ошибка при получении подписки.",
			},
		},
	}

	ps := src.ToProxySourceV4()
	if !strings.Contains(ps.ProviderAnnounce, "Произошла ошибка") {
		t.Fatalf("сообщение провайдера не доехало до сборочной формы: %q", ps.ProviderAnnounce)
	}

	// Источник без метаданных провозит пустое поле, а не мусор.
	src.Meta = nil
	if ps := src.ToProxySourceV4(); ps.ProviderAnnounce != "" {
		t.Errorf("источник без метаданных дал announce %q", ps.ProviderAnnounce)
	}
}
