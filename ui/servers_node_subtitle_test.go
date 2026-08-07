package ui

import (
	"strings"
	"testing"

	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// SPEC 095 — подзаголовок и значки групп.

func TestLeadingFlagExtraction(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want string
	}{
		{name: "flag at start", tag: "🇩🇪-Германия", want: "🇩🇪"},
		{
			// Провайдеры ставят перед флагом префикс источника — искать надо
			// по всему тегу, а не только в начале.
			name: "flag after source prefix", tag: "AL:🇳🇱-Нидерланды", want: "🇳🇱",
		},
		{name: "flag in the middle", tag: "Auto | 🇫🇮 Finland pool", want: "🇫🇮"},
		{name: "no flag", tag: "GLOBAL", want: ""},
		{name: "empty tag", tag: "", want: ""},
		{
			// Одиночный regional indicator флагом не является.
			name: "single indicator is not a flag", tag: "\U0001F1E9only", want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := leadingFlag(tt.tag); got != tt.want {
				t.Fatalf("leadingFlag(%q) = %q, want %q", tt.tag, got, tt.want)
			}
		})
	}
}

func TestPoolBadges(t *testing.T) {
	t.Run("distinct flags up to the cap", func(t *testing.T) {
		members := []string{"AL:🇩🇪-DE", "AL:🇳🇱-NL", "AL:🇫🇮-FI"}
		got := poolBadges(members)
		if got != "🇩🇪🇳🇱🇫🇮" {
			t.Fatalf("poolBadges() = %q", got)
		}
	})

	t.Run("duplicate flags collapse", func(t *testing.T) {
		// Пул часто состоит из серверов одной страны — четыре одинаковых
		// флага ничего не сообщают.
		members := []string{"AL:🇩🇪-1", "AL:🇩🇪-2", "AL:🇩🇪-3"}
		if got := poolBadges(members); got != "🇩🇪…" {
			t.Fatalf("poolBadges() = %q, want single flag with ellipsis", got)
		}
	})

	t.Run("cap with ellipsis", func(t *testing.T) {
		members := []string{"🇩🇪a", "🇳🇱b", "🇫🇮c", "🇵🇱d", "🇪🇸e", "🇭🇺f"}
		got := poolBadges(members)
		if !strings.HasSuffix(got, "…") {
			t.Fatalf("poolBadges() = %q, want trailing ellipsis", got)
		}
		// Флаг = пара regional indicator; считаем сами индикаторы и делим.
		indicators := 0
		for _, r := range got {
			if isRegionalIndicator(r) {
				indicators++
			}
		}
		if flags := indicators / 2; flags != maxPoolBadges {
			t.Fatalf("показано %d значков, ожидалось %d: %q", flags, maxPoolBadges, got)
		}
	})

	t.Run("members without flags yield nothing", func(t *testing.T) {
		if got := poolBadges([]string{"GLOBAL", "direct"}); got != "" {
			t.Fatalf("poolBadges() = %q, want empty", got)
		}
	})

	t.Run("empty membership", func(t *testing.T) {
		if got := poolBadges(nil); got != "" {
			t.Fatalf("poolBadges(nil) = %q", got)
		}
	})
}

func TestGroupSubtitle(t *testing.T) {
	tests := []struct {
		name string
		node *wizardbusiness.ConfigNode
		want string
	}{
		{
			// urltest выбирает лучший по замерам — «цель».
			name: "urltest",
			node: &wizardbusiness.ConfigNode{
				Type:         "urltest",
				GroupMembers: []string{"🇩🇪a", "🇳🇱b"},
			},
			// urltest без mode — умолчание «самый быстрый по замерам».
			want: "🎯 [2] fastest 🇩🇪🇳🇱",
		},
		{
			// selector отдаёт выбор пользователю — «переключатель».
			name: "selector",
			node: &wizardbusiness.ConfigNode{
				Type:         "selector",
				GroupMembers: []string{"a", "b", "c"},
			},
			want: "🔀 [3]",
		},
		{
			name: "empty group",
			node: &wizardbusiness.ConfigNode{Type: "urltest"},
			want: "🎯 [0] fastest",
		},
		{
			// SPEC 088: round_robin раздаёт трафик по пулу — иконка и
			// подпись обязаны отличаться, иначе смена режима не видна.
			name: "urltest round_robin",
			node: &wizardbusiness.ConfigNode{
				Type:         "urltest",
				GroupMembers: []string{"a", "b"},
				Raw:          map[string]interface{}{"mode": "round_robin"},
			},
			want: "⚖️ [2] balanced",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := groupSubtitle(tt.node); got != tt.want {
				t.Fatalf("groupSubtitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Подзаголовок обычного узла — «протокол·транспорт·security».
func TestNodeSubtitleFormat(t *testing.T) {
	nodes := wizardbusiness.ParseConfigNodes([]byte(`{"outbounds":[
	  {"tag":"reality","type":"vless","server":"e.com","server_port":443,
	   "flow":"xtls-rprx-vision",
	   "tls":{"enabled":true,"reality":{"enabled":true}}},
	  {"tag":"ws","type":"vmess","server":"f.com","server_port":443,
	   "transport":{"type":"ws"},"tls":{"enabled":true}},
	  {"tag":"plain","type":"socks","server":"g.com","server_port":1080}
	]}`))

	tests := []struct {
		tag  string
		want string
	}{
		{tag: "reality", want: "vless·tcp·Reality+Vision"},
		{tag: "ws", want: "vmess·ws·TLS"},
		{tag: "plain", want: "socks"},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			node := nodes.Lookup(tt.tag)
			if node == nil {
				t.Fatalf("узел %q не разобран", tt.tag)
			}
			got := strings.Join(node.SubtitleParts(), "·")
			if got != tt.want {
				t.Fatalf("подзаголовок = %q, want %q", got, tt.want)
			}
		})
	}
}
