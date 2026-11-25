package botclient

import (
	"bytes"
	"testing"
	"time"
)

// TestPartyJoinReleasesSpot checks the spot release of joining
// (docs/parties/members.md, "Joining"): a bot holding a spot, or farming
// beside one, gives it back on joining a party it does not lead; leading
// the party, or a window of the party it is already in, keeps it, a leader
// of its bot party with its party's weight (docs/parties/spots.md).
func TestPartyJoinReleasesSpot(t *testing.T) {
	player := slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Abbot"})
	tests := []struct {
		name    string
		slot    Command
		before  *PartySmallWindowAll
		window  Event
		beside  bool
		release bool
	}{
		{"member holding", joinSlot, nil, memberParty, false, true},
		{"member beside", joinSlot, nil, memberParty, true, true},
		{"a player's party, no slot", Command{}, nil, player, false, true},
		{"a player's party beside, no slot", Command{}, nil, player, true, true},
		{"leading", leadSlot, nil, leadParty, false, false},
		{"leading beside", leadSlot, nil, leadParty, true, false},
		{"already in the party", joinSlot, &memberParty, PartySmallWindowAdd{LeaderID: slotLeaderID, Member: PartyMember{ObjectID: slotPlayerID, Name: "Abbot"}}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _, _ := clearBrain(t, tt.slot, tt.before, Persona{JoinParty: acceptParty})
			r := b.cfg.Spots
			if tt.beside {
				i, ok := r.farmBesideN([]int{0}, 1)
				if !ok {
					t.Fatal("no room beside wolves")
				}
				b.spot.idx, b.spot.beside = i, true
			} else {
				i, ok := r.claim([]int{0})
				if !ok {
					t.Fatal("wolves not claimed")
				}
				b.spot.idx, b.spot.held = i, true
			}
			handleAt(t, b, at(time.Second), tt.window)
			kept := r.Held(0) + r.Beside(0)
			switch {
			case tt.release && (kept != 0 || b.spot.held || b.spot.beside):
				t.Fatalf("wolves held %d beside %d, spot %+v: want given back", r.Held(0), r.Beside(0), b.spot)
			case !tt.release && (kept != b.spotWeight() || b.spot.held == tt.beside || b.spot.beside != tt.beside):
				t.Fatalf("wolves held %d beside %d, spot %+v: want kept", r.Held(0), r.Beside(0), b.spot)
			}
		})
	}
}

// holdBrain is a farming-persona brain with no errand due, the spot Wolves
// fitting and a monster in sight, whose bot party, led by Leader in sight,
// disperses at the returned time: slot is its party slot.
func holdBrain(t *testing.T, slot Command) (*Brain, *botLink, *recordSender, time.Time) {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	b, l, out := clearBrain(t, slot, &memberParty, p)
	// No errand due: the town preparation gate is not armed.
	b.prep.armed = false
	walkSplit(t, b)
	handleAt(t, b, t0, CharInfo{ObjectID: slotLeaderID, Name: "Leader", Pos: Point{X: -3000}, RunSpeed: 100, WalkSpeed: 50, Running: true})
	gone := at(10 * time.Second)
	handleAt(t, b, gone, PartySmallWindowDeleteAll{}, DeleteObject{ObjectID: slotLeaderID})
	if b.lead.lost.IsZero() {
		t.Fatalf("watch %+v after the dispersal, want the leader lost", b.lead)
	}
	return b, l, out, gone
}

// held fails unless rule leader holds b at now, the bot claiming no spot
// and sending nothing.
func held(t *testing.T, b *Brain, out *recordSender, now time.Time) {
	t.Helper()
	n := len(out.frames)
	if name, a := deathStep(t, b, now); name != "leader" || a.kind != actHold {
		t.Fatalf("at %v: rule %q kind %d, want leader holding", now.Sub(t0), name, a.kind)
	}
	if r := b.cfg.Spots; r.Held(0)+r.Beside(0) != 0 || b.spot.held || b.spot.beside || len(out.frames) != n {
		t.Fatalf("at %v: wolves held %d beside %d, sent % x: want nothing", now.Sub(t0), r.Held(0), r.Beside(0), opcodes(out))
	}
}

// playsOn steps b every 30 seconds from from for ten minutes, sending what
// it decides, and fails if rule leader still holds it, if it never plays
// its persona (a spot or a monster) or if it whispers or casts /unstuck.
func playsOn(t *testing.T, b *Brain, out *recordSender, from time.Time) {
	t.Helper()
	played := false
	var names []string
	for d := time.Duration(0); d <= 10*time.Minute; d += 30 * time.Second {
		name, _ := deathStep(t, b, from.Add(d))
		names = append(names, name)
		if name == "leader" {
			t.Fatalf("at %v: still held by rule leader", from.Add(d).Sub(t0))
		}
		played = played || name == "spot" || name == "farm"
	}
	if !played {
		t.Fatalf("rules %q, never went for a spot or a monster", names)
	}
	noLostLeader(t, out)
}

// TestPartyMemberLeaderLost checks the join hold (docs/parties/members.md,
// "Join hold", "Clearing from no party"): a dispersed join member, no spot,
// no errand due, a fitting spot free and a monster in sight, holds under
// rule leader past LeaderWait, whispering nothing; attacked, it fights back;
// it accepts the returning leader and follows. At JoinWait it clears its
// slot (timeout) and plays its persona, never whispering; so does a bot an
// unparty, a panel leave or follow, or a GM's leave freed in the hold. A
// bot with no slot behaves as today.
func TestPartyMemberLeaderLost(t *testing.T) {
	wait, join := DefaultPersona().Recover.LeaderWait, DefaultPersona().Recover.JoinWait
	if join <= wait {
		t.Fatalf("JoinWait %v, want past LeaderWait %v", join, wait)
	}
	t.Run("holds", func(t *testing.T) {
		b, l, out, gone := holdBrain(t, joinSlot)
		for _, d := range []time.Duration{time.Second, wait, wait + chatGap, wait + 30*time.Second, join - time.Second} {
			held(t, b, out, gone.Add(d))
		}
		noLostLeader(t, out)
		if l.party != joinSlot {
			t.Fatalf("slot %q in the hold, want %q", l.party, joinSlot)
		}
	})
	t.Run("fights back", func(t *testing.T) {
		b, _, out, gone := holdBrain(t, joinSlot)
		b.world.Self.InCombat, b.world.Target = true, npcID
		if name, a := deathStep(t, b, gone.Add(time.Minute)); name != "attacked" || a.kind != actAttack || a.object != npcID {
			t.Fatalf("attacked: rule %q kind %d on %#x, want attacked attacking %#x", name, a.kind, a.object, npcID)
		}
		b.world.Self.InCombat, b.world.Target = false, 0
		held(t, b, out, gone.Add(wait+30*time.Second))
		noLostLeader(t, out)
	})
	t.Run("leader returns", func(t *testing.T) {
		b, l, out, gone := holdBrain(t, joinSlot)
		held(t, b, out, gone.Add(wait))
		back := gone.Add(wait + time.Minute)
		if got := invite(t, b, out, "Leader", back); len(got) != 1 || !bytes.Equal(got[0], acceptFrame) {
			t.Fatalf("leader's invitation answered % x, want accepted", got)
		}
		handleAt(t, b, back.Add(time.Second), memberParty, CharInfo{ObjectID: slotLeaderID, Name: "Leader", Pos: Point{X: -3000}, RunSpeed: 100, WalkSpeed: 50, Running: true})
		if b.lead != (leaderWatch{id: slotLeaderID, name: "Leader"}) {
			t.Fatalf("watch %+v after rejoining, want the leader followed", b.lead)
		}
		if name, a := deathStep(t, b, back.Add(2*time.Second)); name != "follow" {
			t.Fatalf("rejoined: rule %q kind %d, want follow moving", name, a.kind)
		}
		// Past JoinWait from the dispersal, in the party: the slot stays.
		deathStep(t, b, gone.Add(join+time.Minute))
		if l.party != joinSlot {
			t.Fatalf("slot %q in the party past JoinWait, want %q", l.party, joinSlot)
		}
		noLostLeader(t, out)
	})
	t.Run("JoinWait", func(t *testing.T) {
		b, l, out, gone := holdBrain(t, joinSlot)
		held(t, b, out, gone.Add(join-time.Millisecond))
		if name, _ := deathStep(t, b, gone.Add(join)); name == "leader" {
			t.Fatal("at JoinWait: still held by rule leader")
		}
		b.publishStatus(gone.Add(join), true)
		if st := l.board.load(); l.party != (Command{}) || st.Party != "" || st.PartyLeft != partyLeftTimeout {
			t.Fatalf("slot %q status party %q left %q, want cleared by timeout", l.party, st.Party, st.PartyLeft)
		}
		if b.lead != (leaderWatch{}) {
			t.Fatalf("watch %+v at JoinWait, want reset", b.lead)
		}
		playsOn(t, b, out, gone.Add(join))
	})
	clears := []struct {
		name string
		left string
		do   func(*testing.T, *Brain, time.Time)
	}{
		{"unparty", "", func(t *testing.T, b *Brain, now time.Time) { mustPanel(t, b, Command{Kind: CommandUnparty}, now) }},
		{"panel leave", partyLeftLeft, func(t *testing.T, b *Brain, now time.Time) { mustPanel(t, b, Command{Kind: CommandLeave}, now) }},
		{"panel follow", partyLeftLeft, func(t *testing.T, b *Brain, now time.Time) {
			mustPanel(t, b, Command{Kind: CommandFollow, Arg: "GM"}, now)
		}},
		{"gm leave", partyLeftLeft, func(_ *testing.T, b *Brain, now time.Time) { gmSays(b, cmdLeave, "", now) }},
	}
	for _, c := range clears {
		t.Run(c.name, func(t *testing.T) {
			b, l, out, gone := holdBrain(t, joinSlot)
			now := gone.Add(wait + 30*time.Second)
			held(t, b, out, now)
			c.do(t, b, now)
			b.publishStatus(now, true)
			if st := l.board.load(); l.party != (Command{}) || st.PartyLeft != c.left {
				t.Fatalf("slot %q left %q, want cleared, left %q", l.party, st.PartyLeft, c.left)
			}
			if b.lead != (leaderWatch{}) {
				t.Fatalf("watch %+v after the clear, want reset", b.lead)
			}
			if c.name == "panel follow" {
				// The order follows the commander: no persona play to look for.
				for d := time.Duration(0); d <= 10*time.Minute; d += 30 * time.Second {
					deathStep(t, b, now.Add(time.Second+d))
				}
				noLostLeader(t, out)
				return
			}
			playsOn(t, b, out, now.Add(time.Second))
		})
	}
	t.Run("no slot", func(t *testing.T) {
		b, _, _, gone := holdBrain(t, Command{})
		if name, a := deathStep(t, b, gone.Add(time.Second)); name != "leader" || a.kind != actHold {
			t.Fatalf("in LeaderWait: rule %q kind %d, want leader holding", name, a.kind)
		}
		name, a := deathStep(t, b, gone.Add(wait))
		if name != "leader" || !bytes.Equal(a.payload, encodeSay2Tell("Leader", DefaultPersona().Recover.Whisper)) {
			t.Fatalf("at LeaderWait: rule %q % x, want the whisper to Leader", name, a.payload)
		}
	})
}
