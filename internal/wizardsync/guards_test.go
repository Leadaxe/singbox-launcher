package wizardsync

import "testing"

func TestGuiTextAwaitingProgrammaticFill(t *testing.T) {
	t.Parallel()
	if !GuiTextAwaitingProgrammaticFill(false, "", "x") {
		t.Fatal("before ready, empty widget and non-empty model => await fill")
	}
	if GuiTextAwaitingProgrammaticFill(true, "", "x") {
		t.Fatal("after ready, empty widget is user clear")
	}
	if GuiTextAwaitingProgrammaticFill(false, "a", "y") {
		t.Fatal("non-empty widget => not awaiting fill")
	}
}

func TestFinalOutboundSelectReadLooksStale(t *testing.T) {
	t.Parallel()
	if !FinalOutboundSelectReadLooksStale(false, "", "tag", []string{"other"}) {
		t.Fatal("before ready, never clear model from empty Selected when opts not ready")
	}
	if !FinalOutboundSelectReadLooksStale(true, "", "tag", []string{"tag"}) {
		t.Fatal("after ready, empty Selected while model tag is in Options => stale")
	}
	if !FinalOutboundSelectReadLooksStale(true, "", "tag", nil) {
		t.Fatal("after ready, empty Options => stale")
	}
	if FinalOutboundSelectReadLooksStale(true, "", "tag", []string{"other"}) {
		t.Fatal("after ready, model tag not in Options => not stale")
	}
	if FinalOutboundSelectReadLooksStale(true, "x", "tag", []string{"tag"}) {
		t.Fatal("non-empty Selected => not stale")
	}
}
