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

// SPEC 113-E, регресс: при ДВОЙНОМ сбое (ядро отвергло переключение и состав
// перечитать не удалось) галочку возвращаем сами.
//
// Раньше в этом случае не делалось ничего: refresh не звался (ok=false),
// приводить состояние было нечем, и чекбокс оставался в положении, которое
// ядро отвергло, — пульт врал про состояние ядра.
func TestChainToggleRevertsOnlyOnDoubleFailure(t *testing.T) {
	failed := testErr("core rejected")

	if !chainToggleNeedsRevert(failed, "", false) {
		t.Error("отказ ядра без перечитанного состава обязан откатывать галочку")
	}
	// Состав перечитан — приведёт applyChainRows; второй источник правды здесь
	// только мешал бы.
	if chainToggleNeedsRevert(failed, "", true) {
		t.Error("состав перечитан — откатывать вручную нельзя")
	}
	// Ядро приняло переключение: галочка права независимо от refresh.
	if chainToggleNeedsRevert(nil, "", false) {
		t.Error("принятое переключение откатывать нечего")
	}
	// Флаг применён, не поднялось звено — это диагноз узла, а не отказ.
	if chainToggleNeedsRevert(nil, "warmup failed", false) {
		t.Error("сбой прогрева не отменяет применённый флаг")
	}
}

// TestChainPositionTextFollowsGroupPick — строка позиции обязана меняться
// вслед за выбором группы.
//
// Цепочка через группу тем и ценна, что путь меняется без перезапуска:
// пользователь переключает участника в списке прокси, жмёт «Замерить
// снова» — и состав, и задержки должны относиться к НОВОМУ пути. Раньше
// проба брала состав, прочитанный при открытии окна, и показывала
// задержку до узла, через который трафик уже не шёл.
func TestChainPositionTextFollowsGroupPick(t *testing.T) {
	before := chainPositionText(1, core.ChainPositionInfo{
		Tag: "vpn ②", Now: "AL:Испания", IsGroup: true,
	})
	after := chainPositionText(1, core.ChainPositionInfo{
		Tag: "vpn ②", Now: "AL:Германия-1", IsGroup: true,
	})

	if before == after {
		t.Fatal("строка не отражает смену выбора группы")
	}
	if !strings.Contains(after, "AL:Германия-1") {
		t.Errorf("новый выбор не показан: %q", after)
	}
	if strings.Contains(after, "AL:Испания") {
		t.Errorf("старый выбор остался в строке: %q", after)
	}
}
