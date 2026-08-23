package ui

import (
	"strings"
	"testing"

	"singbox-launcher/core"
)

// TestChainDelayTextCost — накопленное время плюс цена хопа; отрицательная
// цена (шум замеров) не показывается минусом, иначе читалась бы как «этот хоп
// ускорил маршрут».
func TestChainDelayTextCost(t *testing.T) {
	if got := chainDelayText(1224, -1); got != "1224 ms" {
		t.Errorf("first layer: %q", got)
	}
	if got := chainDelayText(7617, 162); !strings.Contains(got, "(+7455)") {
		t.Errorf("cost: %q", got)
	}
	if got := chainDelayText(162, 1224); !strings.Contains(got, "(+0)") {
		t.Errorf("negative cost must clamp to +0: %q", got)
	}
}

// TestChainPositionTextResolved — `now` показывается только когда отличается
// от тега: у обычного узла они совпадают и строка удваивалась бы.
func TestChainPositionTextResolved(t *testing.T) {
	plain := chainPositionText(0, core.ChainPositionInfo{Tag: "warp", Now: "warp"})
	if strings.Contains(plain, "●") {
		t.Errorf("same tag must not repeat: %q", plain)
	}
	group := chainPositionText(1, core.ChainPositionInfo{Tag: "vpn", Now: "fi-1", IsGroup: true})
	if !strings.Contains(group, "fi-1") {
		t.Errorf("group must expose its current pick: %q", group)
	}
}
