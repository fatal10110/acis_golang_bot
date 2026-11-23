package e2e

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

func TestCountCasts(t *testing.T) {
	const self, other, skill = int32(1), int32(2), int32(1177)
	cast := func(caster, id int32) Seen {
		return Seen{Event: botclient.MagicSkillUse{CasterID: caster, SkillID: id}}
	}
	walk := func(id int32) Seen { return Seen{Event: botclient.MoveToLocation{ObjectID: id}} }
	pawn := func(id int32) Seen { return Seen{Event: botclient.MoveToPawn{ObjectID: id}} }
	// clicked is a cast seen after sent packets, followed by a move click.
	clicked := func(at Seen, sent int) Seen { at.SentBefore = sent; return at }
	tests := []struct {
		name string
		seen []Seen
		sent []Sent
		want castCount
	}{
		{name: "none"},
		{name: "a cast standing", seen: []Seen{cast(self, skill)}, want: castCount{casts: 1}},
		{name: "a cast after a walk", seen: []Seen{walk(self), cast(self, skill)}, want: castCount{casts: 1, approached: 1}},
		{name: "a cast after a server approach", seen: []Seen{walk(self), pawn(self), cast(self, skill)}, want: castCount{casts: 1, approached: 1, pawn: 1}},
		{name: "one move counts for the next cast only", seen: []Seen{pawn(self), cast(self, skill), cast(self, skill)}, want: castCount{casts: 2, approached: 1, pawn: 1}},
		{name: "another's moves and casts", seen: []Seen{walk(other), pawn(other), cast(other, skill), cast(self, skill)}, want: castCount{casts: 1}},
		{
			name: "a walk on after a server approach",
			seen: []Seen{pawn(self), clicked(cast(self, skill), 1)},
			sent: []Sent{{Op: opMoveBackward}, {Op: opMoveBackward}},
			want: castCount{casts: 1, approached: 1, pawn: 1, walked: 1},
		},
		{
			name: "a click before the cast only",
			seen: []Seen{pawn(self), clicked(cast(self, skill), 1)},
			sent: []Sent{{Op: opMoveBackward}, {Op: 0x2f}},
			want: castCount{casts: 1, approached: 1, pawn: 1},
		},
		{
			name: "a walk on after a cast with no server approach",
			seen: []Seen{walk(self), cast(self, skill)},
			sent: []Sent{{Op: opMoveBackward}},
			want: castCount{casts: 1, approached: 1},
		},
		{name: "another skill", seen: []Seen{walk(self), cast(self, skill+1), cast(self, skill)}, want: castCount{casts: 1, approached: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countCasts(tt.seen, tt.sent, self, skill); got != tt.want {
				t.Errorf("countCasts = %+v, want %+v", got, tt.want)
			}
		})
	}
}
