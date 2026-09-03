// File tag_guard_twin_test.go — SPEC 116 W12, фикс 1: пара «Направление +
// его авто-группа» — ОДНА сущность, а не два претендента на `<tag>-auto`.
//
// Гард строится по сборочной форме, то есть уже после `ExpandDirectionTwins`:
// в списке лежат обе половины пары. Наивная формула `d.Tag+twinSuffix`
// объявляла второго претендента на тег, который уже занят самим твином, и
// каждое Направление с автовыбором давало ложное «тег занят дважды».
// Настоящий дубль при этом обязан остаться конфликтом.
package config

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

func TestTagGuardW12_TwinPairIsOneOwner(t *testing.T) {
	// 1. Развёрнутая пара: твин создан отдельной записью.
	expanded := ExpandDirectionTwins([]Direction{
		{Tag: "proxy-out", Type: "selector", Auto: &configtypes.DirectionAuto{}},
	}, nil)
	guard := BuildTagGuard(expanded, nil, nil, nil)
	if c := guard.Conflicts(); len(c) != 0 {
		t.Errorf("ложное столкновение на развёрнутой паре: %v", c)
	}
	if !guard.Taken("proxy-out") || !guard.Taken("proxy-out-auto") {
		t.Errorf("гард потерял половину пары: %v", guard.Tags())
	}
	if got := guard.Owner("proxy-out-auto"); got != TagOwnerTwin {
		t.Errorf("владелец твина = %q, ожидалась %q", got, TagOwnerTwin)
	}

	// 2. Шаблонная отдельно стоящая авто-группа с тем же именем: твин НЕ
	// разворачивается (`direction_twins.go`), претендент по-прежнему один.
	tmpl := ExpandDirectionTwins([]Direction{
		{Tag: "proxy-out-auto", Type: "urltest"},
		{Tag: "proxy-out", Type: "selector", Auto: &configtypes.DirectionAuto{}},
	}, nil)
	guard = BuildTagGuard(tmpl, nil, nil, nil)
	if c := guard.Conflicts(); len(c) != 0 {
		t.Errorf("шаблонная авто-группа объявлена конфликтом сама с собой: %v", c)
	}

	// 3. Настоящий дубль остаётся отказом: два Направления с одним тегом.
	guard = BuildTagGuard([]Direction{
		{Tag: "proxy-out", Type: "selector"},
		{Tag: "proxy-out", Type: "selector"},
	}, nil, nil, nil)
	if len(guard.Conflicts()) == 0 {
		t.Error("настоящий дубль тега Направления перестал быть конфликтом")
	}

	// 4. И дубль по другому виду владельца: верхний узел занял `x-auto`.
	guard = BuildTagGuard(
		ExpandDirectionTwins([]Direction{
			{Tag: "x", Type: "selector", Auto: &configtypes.DirectionAuto{}},
		}, nil),
		nil, []string{"x-auto"}, nil)
	conflicts := strings.Join(guard.ConflictTexts(), "; ")
	if !strings.Contains(conflicts, `"x-auto"`) {
		t.Errorf("столкновение узла и авто-группы не названо: %q", conflicts)
	}
}
