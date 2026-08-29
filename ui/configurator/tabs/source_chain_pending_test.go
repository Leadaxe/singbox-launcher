// File source_chain_pending_test.go — «ещё не знаем» ≠ «позиция потеряна».
//
// Пул узлов собирается лениво (окно правки не должно ждать эмиссии всех
// подписок). Пока он пуст, в кандидатах одни Направления — и рабочая
// цепочка из узлов подсвечивалась целиком красным с приговором «эти
// позиции больше не доступны».
package tabs

import (
	"testing"

	corestate "singbox-launcher/core/state"
)

func TestDescribeChainHop_PendingWhileCacheEmpty(t *testing.T) {
	dirLink := corestate.NodeLink{Tag: "vpn ②"}
	lookup := map[string]chainHopCandidate{
		hopLinkKey(dirLink): {Link: dirLink, Tag: "vpn ②", Kind: hopKindDirection},
	}
	nodeLink := corestate.NodeLink{Tag: "🔥 WARP (AWG)"}

	// Пул узлов не готов: про узел судить рано.
	got := describeChainHop(nodeLink, lookup, false)
	if got.Kind != hopKindPending {
		t.Errorf("вид = %q, ожидали pending — иначе рабочая цепочка красится красным", got.Kind)
	}

	// Пул готов, тега нет — вот теперь позиция действительно потеряна.
	got = describeChainHop(nodeLink, lookup, true)
	if got.Kind != hopKindUnknown {
		t.Errorf("вид = %q, ожидали unknown", got.Kind)
	}

	// Известная ссылка опознаётся одинаково в обоих режимах.
	for _, known := range []bool{false, true} {
		if got := describeChainHop(dirLink, lookup, known); got.Kind != hopKindDirection {
			t.Errorf("nodesKnown=%v: известная ссылка = %q", known, got.Kind)
		}
	}
}

// TestDescribeChainHop_FolderLinkDistinctFromRoot — ссылка на узел ПАПКИ и
// корневая ссылка с тем же тегом это РАЗНЫЕ позиции.
//
// До SPEC 118 W6 позиция была строкой, и обе адресовали одно и то же: хоп на
// узел папки «🇳🇱-1» опознавался как корневой тег «🇳🇱-1» — которого в корне
// нет вовсе. Цепочка при этом выглядела здоровой и деградировала fail-closed
// на сборке.
func TestDescribeChainHop_FolderLinkDistinctFromRoot(t *testing.T) {
	folderLink := corestate.NodeLink{FolderID: "F1", Tag: "🇳🇱-1"}
	lookup := map[string]chainHopCandidate{
		hopLinkKey(folderLink): {Link: folderLink, Tag: "[NL] 🇳🇱-1", Kind: hopKindNode},
	}

	if got := describeChainHop(folderLink, lookup, true); got.Kind != hopKindNode {
		t.Fatalf("папочная ссылка не опознана: %q", got.Kind)
	}
	// Тот же тег, но корневой — другая позиция, и кандидата у неё нет.
	if got := describeChainHop(corestate.NodeLink{Tag: "🇳🇱-1"}, lookup, true); got.Kind != hopKindUnknown {
		t.Errorf("корневая ссылка с тем же тегом = %q, ожидали unknown", got.Kind)
	}
}
