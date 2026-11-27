package botclient

import (
	"bytes"
	"testing"
	"time"
)

// TestCraftFailure tells the system messages ahead of RecipeItemMakeInfo
// status 0 apart; the success messages and the consumed notices tell none.
func TestCraftFailure(t *testing.T) {
	tests := []struct {
		msg  int32
		want CraftFailure
	}{
		{msgItemMixingFailed, CraftMixingFailed},
		{msgMissingS2S1ToCreate, CraftMissingMaterial},
		{msgNotEnoughMP, CraftNotEnoughMP},
		{msgCantOperateStoreDuringCombat, CraftInCombat},
		{msgS2S1Disappeared, CraftFailNone},
		{msgS1Disappeared, CraftFailNone},
		{msgEarnedS2S1S, CraftFailNone},
		{msgEarnedItemS1, CraftFailNone},
		{msgS1Added, CraftFailNone},
	}
	for _, tt := range tests {
		if got := craftFailure(tt.msg); got != tt.want {
			t.Errorf("craftFailure(%d) = %d, want %d", tt.msg, got, tt.want)
		}
	}
}

// TestCraftGate spaces crafts craftGap apart, clear of the server's
// ManufactureDelay (300 ms), inside which a craft is dropped in silence.
func TestCraftGate(t *testing.T) {
	a := craftAction(20)
	if a.gate() != gateCraft || !bytes.Equal(a.payload, []byte{0xaf, 20, 0, 0, 0}) {
		t.Fatalf("craftAction = gate %d payload % x", a.gate(), a.payload)
	}
	if craftGap < 300*time.Millisecond {
		t.Fatalf("craftGap %v is inside the server's ManufactureDelay", craftGap)
	}
	if recipeBookAction(true).gate() != gateNone {
		t.Fatal("a book open passes a reuse gate")
	}
	p := newPacer(t0)
	tries := []struct {
		at   time.Duration
		want bool
	}{{0, true}, {craftGap - time.Millisecond, false}, {craftGap, true}, {craftGap + time.Millisecond, false}}
	for _, tr := range tries {
		now := at(tr.at)
		if got := p.allow(gateCraft, now); got != tr.want {
			t.Fatalf("allow(gateCraft) at %v = %v, want %v", tr.at, got, tr.want)
		}
		if tr.want {
			p.sent(gateCraft, now)
		}
	}
}
