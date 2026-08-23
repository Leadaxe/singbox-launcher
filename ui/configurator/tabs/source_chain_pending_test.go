// File source_chain_pending_test.go — «ещё не знаем» ≠ «позиция потеряна».
//
// Кэш узлов разбирается лениво (окно правки не должно ждать разбора всех
// подписок). Пока он пуст, в кандидатах одни Направления — и рабочая
// цепочка из узлов подсвечивалась целиком красным с приговором «эти
// позиции больше не доступны».
package tabs

import "testing"

func TestDescribeChainHop_PendingWhileCacheEmpty(t *testing.T) {
	lookup := map[string]chainHopCandidate{
		"vpn ②": {Tag: "vpn ②", Kind: hopKindDirection},
	}

	// Кэш узлов не готов: про узел судить рано.
	got := describeChainHop("🔥 WARP (AWG)", lookup, false)
	if got.Kind != hopKindPending {
		t.Errorf("вид = %q, ожидали pending — иначе рабочая цепочка красится красным", got.Kind)
	}

	// Кэш готов, тега нет — вот теперь позиция действительно потеряна.
	got = describeChainHop("🔥 WARP (AWG)", lookup, true)
	if got.Kind != hopKindUnknown {
		t.Errorf("вид = %q, ожидали unknown", got.Kind)
	}

	// Известный тег опознаётся одинаково в обоих режимах.
	for _, known := range []bool{false, true} {
		if got := describeChainHop("vpn ②", lookup, known); got.Kind != hopKindDirection {
			t.Errorf("nodesKnown=%v: известный тег = %q", known, got.Kind)
		}
	}
}
