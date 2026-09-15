package state

import (
	"encoding/json"
	"strings"
	"testing"
)

// Идентификация подписки — часть канонической записи: она переживает
// round-trip через state.json. Без этого поле есть в форме, но теряется на
// перезапуске.
//
// С v8 четыре плоских ключа собраны в объект `identity` (SPEC 127 §6.0), и
// проверяется ровно это: ключ вложен, значение читается обратно, пустая
// настройка записи не засоряет.
func TestSubscriptionIdentityRoundTrip(t *testing.T) {
	src := NewSubscriptionSource("Liberty", "https://example.invalid/sub")
	src.SetIdentityUserAgent("Happ/3.3.6")

	b, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"identity":{"user_agent":"Happ/3.3.6"}`) {
		t.Fatalf("UA не попал в JSON объектом identity: %s", b)
	}

	var back Source
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.IdentityUserAgent() != "Happ/3.3.6" {
		t.Errorf("UA потерян при чтении: %q", back.IdentityUserAgent())
	}

	// Прочие настройки объекта переживают тот же круг, и снятие одной не
	// уносит остальные.
	send := true
	src.SetIdentitySendHWID(&send)
	src.SetIdentityHWID("dev-1")
	b2, _ := json.Marshal(src)
	var back2 Source
	if err := json.Unmarshal(b2, &back2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back2.IdentityHWID() != "dev-1" || back2.IdentitySendHWID() == nil || !*back2.IdentitySendHWID() {
		t.Errorf("HWID/send_hwid потеряны: %s", b2)
	}
	if back2.IdentityUserAgent() != "Happ/3.3.6" {
		t.Errorf("правка одной настройки унесла UA: %s", b2)
	}

	// Пустая идентификация не засоряет запись — объект снимается целиком.
	src.SetIdentityUserAgent("")
	src.SetIdentityHWID("")
	src.SetIdentitySendHWID(nil)
	if src.Identity != nil {
		t.Errorf("объект identity остался пустым: %+v", src.Identity)
	}
	b3, _ := json.Marshal(src)
	if strings.Contains(string(b3), "identity") {
		t.Errorf("пустая идентификация попала в JSON: %s", b3)
	}
}

// «Ключа нет» и `null` — одно и то же, а «ключ есть» помнится составом:
// именно на составе стоит предупреждение о неприменённых mobile-only ключах.
func TestSubscriptionIdentityUnmarshalDistinguishesNull(t *testing.T) {
	var id SubscriptionIdentity
	if err := json.Unmarshal([]byte(`{"user_agent":null,"device_os":"android","zzz":1}`), &id); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if id.UserAgent != nil {
		t.Errorf("null прочитан как значение: %v", *id.UserAgent)
	}
	got := id.UnappliedKeys()
	want := []string{"device_os", "zzz"}
	if len(got) != len(want) {
		t.Fatalf("неприменённые ключи = %v, ожидалось %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("неприменённые ключи = %v, ожидалось %v", got, want)
		}
	}
	// user_agent приехал ключом, но со значением null — применять нечего, и
	// в список неприменённых он не попадает: он применяемый по природе.
	// Объект при этом НЕ пуст: mobile-only device_os в нём лежит значением и
	// поедет обратно на телефон файлом 1.0.
	if id.IsEmpty() {
		t.Errorf("объект с device_os считается пустым")
	}
	var allNull SubscriptionIdentity
	if err := json.Unmarshal([]byte(`{"user_agent":null,"hwid":null}`), &allNull); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !allNull.IsEmpty() {
		t.Errorf("объект из одних null считается непустым")
	}
}
