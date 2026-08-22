package build

import (
	"encoding/json"
	"testing"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

func baseInput(chs ...corestate.Channel) ChannelBuildInput {
	return ChannelBuildInput{
		Channels:  chs,
		NodeTags:  []string{"DE-1", "DE-2", "NL-1", "sub-auto"},
		GroupTags: map[string]bool{"sub-auto": true},
		DirectTag: "direct-out",
		BlockTag:  "block-out",
		Templates: template.ChannelGroupTemplates{
			Channel: template.ChannelGroupSpec{Type: "selector"},
			Auto:    template.ChannelGroupSpec{Type: "urltest"},
		},
	}
}

func ch(tag string, mod func(*corestate.Channel)) corestate.Channel {
	c := corestate.Channel{Tag: tag, Label: tag, Enabled: true, InterruptExistConnections: true}
	if mod != nil {
		mod(&c)
	}
	return c
}

func groupByTag(t *testing.T, res ChannelBuildResult, tag string) map[string]interface{} {
	t.Helper()
	for _, g := range res.Groups {
		if g["tag"] == tag {
			return g
		}
	}
	t.Fatalf("группа %q не построена; есть: %v", tag, tagsOf(res))
	return nil
}

func tagsOf(res ChannelBuildResult) []string {
	out := make([]string, 0, len(res.Groups))
	for _, g := range res.Groups {
		out = append(out, g["tag"].(string))
	}
	return out
}

// Канал без фильтра забирает все узлы, порядок конфига сохраняется:
// он осмыслен (порядок подписки), сортировка перемешала бы локации.
func TestChannelWithoutFilterTakesAllNodes(t *testing.T) {
	res := BuildChannelGroups(baseInput(ch("vpn-1", nil)))
	g := groupByTag(t, res, "vpn-1")
	got := g["outbounds"].([]string)
	want := []string{"DE-1", "DE-2", "NL-1", "sub-auto"}
	if len(got) != len(want) {
		t.Fatalf("состав %v, ожидался %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("порядок нарушен: %v, ожидался %v", got, want)
		}
	}
}

func TestChannelNodeFilter(t *testing.T) {
	res := BuildChannelGroups(baseInput(ch("vpn-1", func(c *corestate.Channel) {
		c.NodeFilter = "^DE-"
	})))
	got := groupByTag(t, res, "vpn-1")["outbounds"].([]string)
	if len(got) != 2 || got[0] != "DE-1" || got[1] != "DE-2" {
		t.Fatalf("фильтр отобрал %v, ожидались DE-1 и DE-2", got)
	}
}

func TestChannelNodeFilterInvert(t *testing.T) {
	res := BuildChannelGroups(baseInput(ch("vpn-1", func(c *corestate.Channel) {
		c.NodeFilter = "^DE-"
		c.NodeFilterInvert = true
	})))
	got := groupByTag(t, res, "vpn-1")["outbounds"].([]string)
	if len(got) != 2 || got[0] != "NL-1" {
		t.Fatalf("инверсия отобрала %v, ожидались NL-1 и sub-auto", got)
	}
}

// Опечатка в фильтре не должна оставлять пользователя без конфига целиком:
// невалидное выражение ведёт себя как пустой фильтр.
func TestChannelInvalidFilterFallsBackToAll(t *testing.T) {
	res := BuildChannelGroups(baseInput(ch("vpn-1", func(c *corestate.Channel) {
		c.NodeFilter = "([unclosed"
	})))
	got := groupByTag(t, res, "vpn-1")["outbounds"].([]string)
	if len(got) != 4 {
		t.Fatalf("при битом фильтре отобрано %v, ожидались все узлы", got)
	}
}

// Пустая группа фатальна для ядра. У канала без единой опции состав
// подставляется запасной, и первым идёт блокировка: заблокировать
// безопаснее, чем выпустить трафик мимо VPN, когда ждали туннель.
func TestEmptyChannelFallsBackToBlockFirst(t *testing.T) {
	res := BuildChannelGroups(baseInput(ch("vpn-1", func(c *corestate.Channel) {
		c.NodeFilter = "^ZZ-"
	})))
	g := groupByTag(t, res, "vpn-1")
	got := g["outbounds"].([]string)
	if len(got) != 2 || got[0] != "block-out" {
		t.Fatalf("запасной состав %v, ожидался [block-out direct-out]", got)
	}
	if g["default"] != "block-out" {
		t.Errorf("default = %v, ожидался block-out", g["default"])
	}
	if len(res.Warnings) == 0 {
		t.Error("фильтр не поймал ни одного узла, а предупреждения нет")
	}
}

// Пустой фильтр при отсутствии узлов — это нет подписки, а не ошибка
// настройки: предупреждать не о чем.
func TestNoWarningWhenNoNodesAtAll(t *testing.T) {
	in := baseInput(ch("vpn-1", nil))
	in.NodeTags = nil
	res := BuildChannelGroups(in)
	if len(res.Warnings) != 0 {
		t.Errorf("предупреждение без подписки: %v", res.Warnings)
	}
}

// Пользователь сам включил direct — врать про блокировку нельзя.
func TestWarningSaysDirectWhenUserEnabledIt(t *testing.T) {
	res := BuildChannelGroups(baseInput(ch("vpn-1", func(c *corestate.Channel) {
		c.NodeFilter = "^ZZ-"
		c.IncludeDirect = true
	})))
	if len(res.Warnings) == 0 {
		t.Fatal("нет предупреждения")
	}
	if !contains(res.Warnings[0], "direct") {
		t.Errorf("предупреждение обещает блокировку, хотя включён direct: %q", res.Warnings[0])
	}
}

// Узел автовыбора подписки не попадает в urltest канала: urltest поверх
// urltest мерил бы уже выбранный узел, а не сервер.
func TestAutoGroupExcludesNestedGroups(t *testing.T) {
	res := BuildChannelGroups(baseInput(ch("vpn-1", func(c *corestate.Channel) {
		c.Auto = &corestate.ChannelAuto{URL: "http://cp/generate_204", Interval: "5m"}
	})))
	auto := groupByTag(t, res, "vpn-1-auto")
	for _, tag := range auto["outbounds"].([]string) {
		if tag == "sub-auto" {
			t.Fatalf("группа выбора попала в urltest канала: %v", auto["outbounds"])
		}
	}
	sel := groupByTag(t, res, "vpn-1")
	last := sel["outbounds"].([]string)
	if last[len(last)-1] != "vpn-1-auto" {
		t.Errorf("auto-группа не добавлена в селектор: %v", last)
	}
}

// Автовыбор выключен — парная группа не эмитится вовсе.
func TestNoAutoGroupWhenDisabled(t *testing.T) {
	res := BuildChannelGroups(baseInput(ch("vpn-1", nil)))
	for _, tag := range tagsOf(res) {
		if tag == "vpn-1-auto" {
			t.Fatal("auto-группа эмитится при выключенном автовыборе")
		}
	}
}

func TestDefaultFilterPicksFirstMatch(t *testing.T) {
	res := BuildChannelGroups(baseInput(ch("vpn-1", func(c *corestate.Channel) {
		c.DefaultFilter = "NL"
	})))
	if got := groupByTag(t, res, "vpn-1")["default"]; got != "NL-1" {
		t.Errorf("default = %v, ожидался NL-1", got)
	}
}

func TestDisabledChannelSkipped(t *testing.T) {
	res := BuildChannelGroups(baseInput(ch("vpn-1", func(c *corestate.Channel) {
		c.Enabled = false
	})))
	if len(res.Groups) != 0 {
		t.Errorf("выключенный канал материализован: %v", tagsOf(res))
	}
}

// Параметры канала важнее шаблонных умолчаний.
func TestChannelAutoOverridesTemplateOptions(t *testing.T) {
	in := baseInput(ch("vpn-1", func(c *corestate.Channel) {
		c.Auto = &corestate.ChannelAuto{URL: "http://mine/204", Tolerance: 150}
	}))
	in.Templates.Auto.Options = map[string]json.RawMessage{}
	res := BuildChannelGroups(in)
	auto := groupByTag(t, res, "vpn-1-auto")
	if auto["url"] != "http://mine/204" {
		t.Errorf("url = %v, ожидался пользовательский", auto["url"])
	}
	if auto["tolerance"] != 150 {
		t.Errorf("tolerance = %v, ожидался 150", auto["tolerance"])
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
