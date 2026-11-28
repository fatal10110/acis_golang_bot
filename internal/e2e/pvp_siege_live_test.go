package e2e

import "testing"

// siegeUnsafe is why TestLivePvPSiege skips: the shared live server cannot
// host a harness siege without touching real players and real castle state
// (#735). Each point is server behavior, read in acis_golang:
//
//   - //siege start runs a real siege (network/admin_siege.go,
//     siege/lifecycle.go startLocked): every player of another clan than
//     the owner's is thrown off the castle zone (model/zone/siege.go
//     BanishForeigners), the start and end are announced to every player,
//     the gates close and the siege guards spawn for whoever stands near.
//   - The scripted defender clan needs a castle some clan owns: a castle
//     with no owner refuses every defender (siege/registration.go
//     RegisterDefender, MsgDefenderSideFull), and a defender only joins
//     once the castle lord confirms it. Giving a castle an owner is
//     //castle set, a real change of ownership the GM scope must not allow.
//   - //siege attack and defend register only while the castle's
//     registration is open (canRegisterLocked, MsgDeadlinePassed), which
//     the real siege schedule decides, and //siege end sets the castle's
//     next siege date and drops every registration (endLocked): the
//     harness would reschedule a castle real clans may be registered for.
//   - The GM client decodes no siege info window, so the harness cannot
//     tell a castle with a siege scheduled or under way from an idle one.
//   - A siege outlives a run that dies before its cleanup (heavy stops a
//     hung run with SIGQUIT, a lost GM connection skips //siege end), and
//     then runs on the shared server for the siege length, 120 minutes by
//     default.
const siegeUnsafe = "a harness siege is not safe on the shared live server: //siege start banishes real players from the castle zone and is announced server-wide, a scripted defender needs a castle owner (//castle set), //siege end reschedules the castle, and a run that dies before its cleanup leaves the siege running up to 120 minutes; the owner decides a dedicated castle or a test-only server (#735)"

// TestLivePvPSiege is the live siege scenario of docs/pvp/siege.md "Tests"
// (X15, D41): a GM registers the bot's clan A (level 4 by the GM) as
// attacker of a castle and a scripted clan B as defender, starts the
// siege; the bot answers the lists without a dialog, walks in, gets 283,
// attacks the scripted defender and never a clanless bot inside, which is
// banished or walks out; //siege end clears the state and the bot leaves.
// It skips until a siege-safe stack exists (siegeUnsafe): the GM scope
// allows no //siege command, and the siege's behavior is covered by the
// X15 and #734 unit tests and recorded frames instead.
func TestLivePvPSiege(t *testing.T) {
	t.Skip(siegeUnsafe)
}
