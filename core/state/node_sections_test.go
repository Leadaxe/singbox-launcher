package state

// Секции узла: подстановка плейсхолдера и чтение старой формы (SPEC 121
// §10.1–10.3).
//
// Два теста — оба data-критичные: подстановка решает, каким тегом узел
// адресован в конфиге, а конвертер старой формы отвечает за то, что связка
// пользователя, поставившего сборку волн 1–2, не пропадёт при обновлении.

import (
	"encoding/json"
	"testing"
)

// TestSubstituteSelf — обе формы плейсхолдера, ключи, чужой текст, порядок.
func TestSubstituteSelf(t *testing.T) {
	const finalTag = "DE-ts-node"

	t.Run("whole string and embedded forms", func(t *testing.T) {
		in := []byte(`{"outbound":"@self","tag":"@{self}-dns","name":"@{self} network"}`)
		got := string(SubstituteSelf(in, finalTag))
		want := `{"outbound":"DE-ts-node","tag":"DE-ts-node-dns","name":"DE-ts-node network"}`
		if got != want {
			t.Errorf("SubstituteSelf =\n%s\nwant\n%s", got, want)
		}
	})

	t.Run("key order survives", func(t *testing.T) {
		// Порядок ключей значим: тела сравниваются с выводом эмиттера
		// байт-в-байт (CODEMAP §10 п. 21).
		in := []byte(`{"zz":1,"aa":"@self","mm":[{"nested":"@{self}!"}]}`)
		got := string(SubstituteSelf(in, finalTag))
		want := `{"zz":1,"aa":"DE-ts-node","mm":[{"nested":"DE-ts-node!"}]}`
		if got != want {
			t.Errorf("SubstituteSelf =\n%s\nwant\n%s", got, want)
		}
	})

	t.Run("foreign text and keys are data", func(t *testing.T) {
		// Ключ `@self` — имя поля, а не ссылка; `@selfish` — не плейсхолдер
		// (форма `@self` подставляется только целой строкой); чужая `@var`
		// остаётся собой: словаря переменных у секции нет.
		in := []byte(`{"@self":"key","a":"@selfish","b":"@other","c":"mail@self.example"}`)
		got := string(SubstituteSelf(in, finalTag))
		if got != string(in) {
			t.Errorf("SubstituteSelf тронул то, что плейсхолдером не является:\n%s\nwant\n%s", got, in)
		}
	})

	t.Run("empty final tag leaves the placeholder alone", func(t *testing.T) {
		// Подстановка пустой строкой дала бы `"outbound": ""` — висячую
		// ссылку вместо честной.
		in := []byte(`{"outbound":"@self"}`)
		if got := string(SubstituteSelf(in, "")); got != string(in) {
			t.Errorf("SubstituteSelf с пустым тегом = %s, want %s", got, in)
		}
	})
}

// TestMigrateLegacyNodeSections — чтение формы волн 1–2 из state.json
// (SPEC 121 §10.3): сырые фрагменты sing-box плюс якорь kind=node в rules[].
//
// Проверяется, что эмитируемый конфиг не потерян: префиксация тега сервера
// повторена плейсхолдером, ссылка правила указывает на свой сервер, правило
// маршрута сохранило цель, а `enabled`/`order_num` пришли из якоря — и сам
// якорь после перевода исчез.
func TestMigrateLegacyNodeSections(t *testing.T) {
	const legacySections = `{
	  "dns_servers": [{"type":"tailscale","tag":"ts-dns","endpoint":"@self"}],
	  "dns_rules":   [{"domain_suffix":[".ts.net"],"server":"ts-dns"}],
	  "rules":       [{"ip_cidr":["100.64.0.0/10"]}]
	}`

	var sections NodeSections
	if err := json.Unmarshal([]byte(legacySections), &sections); err != nil {
		t.Fatalf("старая форма не читается: %v", err)
	}
	if !sections.HasLegacyShape() {
		t.Fatal("старая форма не распознана — конвертер до неё не дойдёт")
	}

	anchorBody, err := json.Marshal(map[string]string{"tag": "ts-node"})
	if err != nil {
		t.Fatalf("кодирование тела якоря: %v", err)
	}
	anchorNum := 960
	st := &State{
		Sources: []Source{{
			ID: "01J00000000000000000000SRV",
			Node: Node{
				Kind:     SourceKindServer,
				Tag:      "ts-node",
				Enabled:  true,
				Sections: &sections,
			},
		}},
		Rules: []Rule{{
			// Упразднённый вид: сборка волн 1–2 держала здесь позицию и
			// тумблер правил узла.
			Kind:     RuleKind("node"),
			Enabled:  false,
			OrderNum: &anchorNum,
			Body:     anchorBody,
		}},
	}

	MigrateLegacyNodeSections(st)

	// Якорь упразднённого вида снят: DecodeBody на нём падает, и оставить его
	// значило бы уронить загрузку.
	for _, r := range st.Rules {
		if r.Kind == RuleKind("node") {
			t.Fatal("запись упразднённого вида kind=node пережила перевод")
		}
	}

	got := st.Sources[0].Node.Sections
	if got.IsEmpty() || got.HasLegacyShape() {
		t.Fatalf("секции после перевода: пусто=%v, старая форма=%v", got.IsEmpty(), got.HasLegacyShape())
	}

	if len(got.DNSServers()) != 1 {
		t.Fatalf("DNS-серверов после перевода: %d", len(got.DNSServers()))
	}
	srv := got.DNSServers()[0]
	// Неявная префиксация волн 1–2 давала `<финальный тег>:<локальный>`;
	// в новой форме то же самое пишется плейсхолдером.
	if want := SelfPlaceholderBraced + ":ts-dns"; srv.Tag != want {
		t.Errorf("тег DNS-сервера %q, ожидался %q", srv.Tag, want)
	}
	if srv.Kind != DNSServerKindUser {
		t.Errorf("вид DNS-сервера %q", srv.Kind)
	}
	if got, _ := srv.Body["endpoint"].(string); got != SelfPlaceholder {
		t.Errorf("endpoint DNS-сервера %q — ссылка на узел обязана уцелеть", got)
	}

	if len(got.DNSRules()) != 1 {
		t.Fatalf("DNS-правил после перевода: %d", len(got.DNSRules()))
	}
	if s, _ := got.DNSRules()[0].Body["server"].(string); s != SelfPlaceholderBraced+":ts-dns" {
		t.Errorf("ссылка DNS-правила %q — она обязана указывать на сервер своей же секции", s)
	}

	if len(got.Rules) != 1 {
		t.Fatalf("правил маршрута после перевода: %d", len(got.Rules))
	}
	r := got.Rules[0]
	if r.Kind != RuleKindInline {
		t.Errorf("вид правила %q, ожидался inline", r.Kind)
	}
	// enabled и order_num пришли из якоря — позиция и тумблер принадлежали
	// пользователю и терять их нельзя.
	if r.Enabled {
		t.Error("правило приехало включённым, хотя якорь был выключен")
	}
	if r.OrderNum == nil || *r.OrderNum != anchorNum {
		t.Errorf("позиция правила %v, ожидалась %d с якоря", r.OrderNum, anchorNum)
	}
	body, err := r.DecodeBody()
	if err != nil {
		t.Fatalf("тело переведённого правила не читается: %v", err)
	}
	inline := body.(*InlineBody)
	// Правило без `outbound` в старой форме означало «про этот узел».
	if inline.Outbound != SelfPlaceholder {
		t.Errorf("цель правила %q, ожидался %s", inline.Outbound, SelfPlaceholder)
	}
	if _, ok := inline.Match["ip_cidr"]; !ok {
		t.Errorf("match правила потерял ip_cidr: %v", inline.Match)
	}

	// Повторный прогон ничего не меняет: перевод одноразовый.
	before := st.Sources[0].Node.Sections
	MigrateLegacyNodeSections(st)
	if st.Sources[0].Node.Sections != before {
		t.Error("повторный перевод тронул уже переведённые секции")
	}
}
