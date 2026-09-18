package corereject

import "testing"

// TestParseCanonTable — таблица примеров CANON §9.2 целиком плюс формы, о
// которых норма говорит отдельно: `endpoint`, теги с эмодзи, пробелами,
// двоеточиями и `]`, строка без тега (ядра до lx.7), ошибки не про узел.
//
// Таблица данных, а не форматирования: проверяется РЕШЕНИЕ разбора («назван
// ли узел и какой»), от которого зависит, выключит ли страховка чужой узел.
func TestParseCanonTable(t *testing.T) {
	cases := []struct {
		name string
		line string
		tags []string

		wantOK    bool
		wantKind  Kind
		wantIndex int
		wantType  string
		wantTag   string
		wantText  string
	}{
		{
			name:      "CANON: эмодзи в теге",
			line:      "initialize outbound[3] vless[🇩🇪 Frankfurt]: parse encryption: bad",
			tags:      []string{"🇩🇪 Frankfurt"},
			wantOK:    true,
			wantKind:  KindOutbound,
			wantIndex: 3,
			wantType:  "vless",
			wantTag:   "🇩🇪 Frankfurt",
			wantText:  "parse encryption: bad",
		},
		{
			name: "CANON: `]: ` внутри ТЕГА — побеждает длинный кандидат",
			line: "initialize outbound[0] vless[A]: B]: c",
			// В конфиге есть ровно тег `A]: B`, короткого `A` нет.
			tags:      []string{"A]: B"},
			wantOK:    true,
			wantKind:  KindOutbound,
			wantIndex: 0,
			wantType:  "vless",
			wantTag:   "A]: B",
			wantText:  "c",
		},
		{
			name: "CANON: та же строка, но в конфиге короткий тег",
			line: "initialize outbound[0] vless[A]: B]: c",
			tags: []string{"A"},

			wantOK:    true,
			wantKind:  KindOutbound,
			wantIndex: 0,
			wantType:  "vless",
			wantTag:   "A",
			wantText:  "B]: c",
		},
		{
			name:      "CANON: endpoint, двоеточие в теге",
			line:      "initialize endpoint[1] wireguard[wg: home]: bad key",
			tags:      []string{"wg: home"},
			wantOK:    true,
			wantKind:  KindEndpoint,
			wantIndex: 1,
			wantType:  "wireguard",
			wantTag:   "wg: home",
			wantText:  "bad key",
		},
		{
			name:   "CANON: форма без тега (ядра до lx.7) — узел не назван",
			line:   "initialize outbound[26]: unknown uTLS fingerprint",
			tags:   []string{"any"},
			wantOK: false,
		},
		{
			name:   "CANON: ошибка не про узел — inbound",
			line:   "initialize inbound[0] tun: permission denied",
			tags:   []string{"tun"},
			wantOK: false,
		},
		{
			name:   "не про узел: dns",
			line:   "initialize dns server[2]: unknown scheme",
			tags:   []string{"dns-remote"},
			wantOK: false,
		},
		{
			name:   "не про узел: route",
			line:   "initialize route rule[7]: missing action",
			tags:   []string{"proxy"},
			wantOK: false,
		},
		{
			name:   "тег в строке есть, но в конфиге его нет — не сопоставлен",
			line:   "initialize outbound[4] vmess[Ghost]: bad uuid",
			tags:   []string{"Frankfurt", "Amsterdam"},
			wantOK: false,
		},
		{
			name:      "оба кандидата существуют — побеждает правый (длинный тег)",
			line:      "initialize outbound[0] vless[A]: B]: c",
			tags:      []string{"A", "A]: B"},
			wantOK:    true,
			wantTag:   "A]: B",
			wantText:  "c",
			wantKind:  KindOutbound,
			wantIndex: 0,
			wantType:  "vless",
		},
		{
			name:      "скобка в теге без `]: `",
			line:      "initialize outbound[9] trojan[NL [2]]: parse: nope",
			tags:      []string{"NL [2]"},
			wantOK:    true,
			wantKind:  KindOutbound,
			wantIndex: 9,
			wantType:  "trojan",
			wantTag:   "NL [2]",
			wantText:  "parse: nope",
		},
		{
			name:      "текст-цепочка обёрток через `: `",
			line:      "initialize outbound[12] hysteria2[HK 01]: create service: parse config: bad up_mbps",
			tags:      []string{"HK 01"},
			wantOK:    true,
			wantKind:  KindOutbound,
			wantIndex: 12,
			wantType:  "hysteria2",
			wantTag:   "HK 01",
			wantText:  "create service: parse config: bad up_mbps",
		},
		{
			name:   "мусор вместо индекса",
			line:   "initialize outbound[x] vless[A]: bad",
			tags:   []string{"A"},
			wantOK: false,
		},
		{
			name:   "нет фиксированного префикса",
			line:   "FATAL start service: something else entirely",
			tags:   []string{"A"},
			wantOK: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			set := make(map[string]bool, len(tc.tags))
			for _, tag := range tc.tags {
				set[tag] = true
			}
			got, ok := Parse(tc.line, TagsOf(set))
			if ok != tc.wantOK {
				t.Fatalf("Parse(%q) ok=%v, ожидалось %v (получено %+v)", tc.line, ok, tc.wantOK, got)
			}
			if !tc.wantOK {
				return
			}
			if got.Kind != tc.wantKind {
				t.Errorf("Kind = %q, ожидалось %q", got.Kind, tc.wantKind)
			}
			if got.Index != tc.wantIndex {
				t.Errorf("Index = %d, ожидалось %d", got.Index, tc.wantIndex)
			}
			if got.Type != tc.wantType {
				t.Errorf("Type = %q, ожидалось %q", got.Type, tc.wantType)
			}
			if got.Tag != tc.wantTag {
				t.Errorf("Tag = %q, ожидалось %q", got.Tag, tc.wantTag)
			}
			if got.Text != tc.wantText {
				t.Errorf("Text = %q, ожидалось %q", got.Text, tc.wantText)
			}
		})
	}
}

// TestParseMultiline — вывод `check` многострочный: баннер ядра, WARN-строки,
// и нужная строка где-то внутри. Привязки к началу строки нет: ядро
// заворачивает отказ в свою рамку.
func TestParseMultiline(t *testing.T) {
	out := "sing-box version 1.14.0-lx.33\n" +
		"WARN[0000] deprecated: legacy DNS\n" +
		"FATAL[0000] initialize outbound[7] vless[SE Stockholm]: parse encryption: unknown appearance\n"
	got, ok := Parse(out, TagsOf(map[string]bool{"SE Stockholm": true}))
	if !ok {
		t.Fatalf("многострочный вывод: узел не найден")
	}
	if got.Tag != "SE Stockholm" {
		t.Errorf("Tag = %q", got.Tag)
	}
	if got.Text != "parse encryption: unknown appearance" {
		t.Errorf("Text = %q", got.Text)
	}
}

// TestParseEmptyInputs — пустой вход и отсутствующее множество тегов дают
// «узел не назван», а не панику: страховка при этом не действует, и это
// правильный дефолт.
func TestParseEmptyInputs(t *testing.T) {
	if _, ok := Parse("", TagsOf(map[string]bool{"A": true})); ok {
		t.Error("пустой вывод не должен называть узел")
	}
	if _, ok := Parse("initialize outbound[0] vless[A]: bad", nil); ok {
		t.Error("nil TagSet не должен называть узел")
	}
	if _, ok := Parse("initialize outbound[0] vless[A]: bad", TagsOf(nil)); ok {
		t.Error("пустое множество тегов не должно называть узел")
	}
}
