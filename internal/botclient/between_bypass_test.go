package botclient

import (
	"math/rand/v2"
	"testing"
	"time"
)

// TestBetween: between draws lo-hi with both ends included, the same value
// and the same rng stream as the inline form it replaced, and lo when hi is
// not above lo.
func TestBetween(t *testing.T) {
	tests := []struct {
		name   string
		lo, hi time.Duration
	}{
		{"zero lo", 0, 3 * time.Second},
		{"tick", 600 * time.Millisecond, 900 * time.Millisecond},
		{"small range reaches both ends", 5, 7},
		{"equal bounds", 2 * time.Second, 2 * time.Second},
		{"hi below lo", 2 * time.Second, time.Second},
		{"zero both", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Brain{rng: rand.New(rand.NewPCG(11, 13))}
			ref := rand.New(rand.NewPCG(11, 13))
			top := max(tt.hi, tt.lo)
			seen := map[time.Duration]bool{}
			for i := range 2000 {
				got := b.between(tt.lo, tt.hi)
				want := tt.lo + time.Duration(ref.Int64N(int64(max(tt.hi-tt.lo, 0))+1))
				if got != want {
					t.Fatalf("draw %d: between(%v, %v) = %v, inline form %v", i, tt.lo, tt.hi, got, want)
				}
				if got < tt.lo || got > top {
					t.Fatalf("draw %d: between(%v, %v) = %v, out of [%v, %v]", i, tt.lo, tt.hi, got, tt.lo, top)
				}
				seen[got] = true
			}
			if tt.hi <= tt.lo && (len(seen) != 1 || !seen[tt.lo]) {
				t.Errorf("between(%v, %v) drew %v, want only lo", tt.lo, tt.hi, seen)
			}
			if tt.hi > tt.lo && tt.hi-tt.lo < 10 && (!seen[tt.lo] || !seen[tt.hi]) {
				t.Errorf("between(%v, %v) never drew an end: %v", tt.lo, tt.hi, seen)
			}
			if b.rng.Uint64() != ref.Uint64() {
				t.Error("rng stream differs from the inline form after the draws")
			}
		})
	}
}

// TestNPCBypass: npcBypass and the links built on it are byte for byte the
// npc_<object id>_<command> strings the server's pages carry.
func TestNPCBypass(t *testing.T) {
	const obj = 268437457
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"sell", sellCommand(obj), "npc_268437457_Sell"},
		{"buy list", buyCommand(obj, 3), "npc_268437457_Buy 3"},
		{"skill list", skillListBypass(obj), "npc_268437457_SkillList"},
		{"teleport list", teleportListBypass(obj), "npc_268437457_teleport_request"},
		{"teleport", teleportBypass(obj, 2), "npc_268437457_teleport 2"},
		{"clan deposit", clanWarehouseCommand(obj, true), "npc_268437457_DepositC"},
		{"clan withdraw", clanWarehouseCommand(obj, false), "npc_268437457_WithdrawC"},
		{"private deposit", npcBypass(obj, "DepositP"), "npc_268437457_DepositP"},
		{"private withdraw prefix", npcBypass(obj, "WithdrawP"), "npc_268437457_WithdrawP"},
		{"chat prefix", npcBypass(obj, "Chat "), "npc_268437457_Chat "},
		{"chat", npcBypass(obj, "Chat 1"), "npc_268437457_Chat 1"},
		{"draw", npcBypass(obj, drawCommand), "npc_268437457_Draw"},
		{"change class", npcBypass(obj, changeClassCommand+" 4"), "npc_268437457_change_class 4"},
		{"class tier menu", npcBypass(obj, "1stClass"), "npc_268437457_1stClass"},
		{"negative id", npcBypass(-5, "Sell"), "npc_-5_Sell"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}
