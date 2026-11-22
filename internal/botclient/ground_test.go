package botclient

import "testing"

// groundShift is how far onGround moves a rule test's scene: the bot at the
// origin and the creatures around it then stand well inside the synthetic
// open ground, whichever way the bot walks.
var groundShift = Point{X: 4000, Y: 2000}

// onGround has brain b walk on the synthetic open ground (walkOn), with the
// bot and every object in sight moved by groundShift: a rule test whose bot
// flees or drifts needs ground under it (D13a).
func onGround(t *testing.T, b *Brain) {
	t.Helper()
	walkOn(t, b)
	shift := func(m *motion) {
		m.from.X, m.from.Y = m.from.X+groundShift.X, m.from.Y+groundShift.Y
		m.to.X, m.to.Y = m.to.X+groundShift.X, m.to.Y+groundShift.Y
	}
	shift(&b.world.Self.move)
	for _, o := range b.world.Objects {
		shift(&o.move)
	}
}
