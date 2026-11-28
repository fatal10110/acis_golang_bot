package botclient

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Seven Signs member mode (docs/pvp/cp-potions.md "What the bot does",
// X18, D43): a bot named in Persona.SevenSigns.Members signs up with the
// Revolutionaries of Dusk at a Dusk Priestess in the competition, farms
// the catacombs and necropolises its membership opens, through the Dungeon
// Gatekeepers, and keeps the seal stones it loots. It never contributes a
// stone (SevenSigns 6), never pays the Dawn fee (SevenSigns 34) and never
// votes the Seal of Strife.

// Seal is a Seven Signs seal, numbered as the priests' links name it
// (server sevensigns.Seal: NoSeal 0, then Avarice, Gnosis, Strife).
type Seal int8

const (
	SealAvarice Seal = 1
	SealGnosis  Seal = 2
	SealStrife  Seal = 3
)

// DefaultSignsMembers is SevenSignsPolicy.MaxMembers's default (D43): at
// most one member per run, so the bots weigh little on the seals.
const DefaultSignsMembers = 1

// SevenSignsPolicy is Seven Signs member mode (D43): who plays it and the
// seal it votes. The cap is static: a run names its members up front, and
// config loading refuses a list longer than MaxMembers (Check), so no
// count is rebuilt after a restart and no two bots race for a slot.
type SevenSignsPolicy struct {
	// Members are the character names that play the mode; empty, no bot
	// does (every preset's default, D6).
	Members []string
	// MaxMembers is the longest Members list the run accepts.
	MaxMembers int
	// Seal is the seal a member votes when Dusk holds none on the status
	// page: Avarice or Gnosis, Gnosis when unset; never Strife (Check).
	Seal Seal
}

// member reports whether character name plays the mode (case-insensitive,
// as the server's names are).
func (p SevenSignsPolicy) member(name string) bool {
	return name != "" && slices.ContainsFunc(p.Members, func(m string) bool { return strings.EqualFold(m, name) })
}

// Check validates s (D43): no more Members than MaxMembers, and Seal
// unset, Avarice or Gnosis: never Strife.
func (s SevenSignsPolicy) Check() error {
	if s.MaxMembers < 0 {
		return fmt.Errorf("seven signs: max members %d below 0", s.MaxMembers)
	}
	if len(s.Members) > s.MaxMembers {
		return fmt.Errorf("seven signs: %d members, at most %d", len(s.Members), s.MaxMembers)
	}
	switch s.Seal {
	case 0, SealAvarice, SealGnosis:
		return nil
	case SealStrife:
		return fmt.Errorf("seven signs: a member never votes the Seal of Strife")
	}
	return fmt.Errorf("seven signs: unknown seal %d", s.Seal)
}

// seal is the seal the bot votes when Dusk holds none (Seal, Gnosis when
// unset).
func (s SevenSignsPolicy) seal() Seal {
	if s.Seal == SealAvarice {
		return SealAvarice
	}
	return SealGnosis
}

// Seal stone templates (items/6300-6399.xml): 3, 5 and 10 contribution
// points; dropped only by the monsters of the Seven Signs dungeons.
const (
	itemBlueSealStone  int32 = 6360
	itemGreenSealStone int32 = 6361
	itemRedSealStone   int32 = 6362
)

// duskPriestesses are the Dusk Priestess templates (npcs/31000-31999.xml,
// type SignsPriest).
var duskPriestesses = []int32{31085, 31086, 31087, 31088, 31089, 31090, 31091}

const (
	// signsReach is how far the bot walks to a Dusk Priestess's spawn: one
	// in its town, on a supply trip; one in sight is visited from anywhere.
	signsReach = 4000
	// signsFresh is how long what a priest's page told stands (the period,
	// the membership): the period turns once a week, and a member revisits
	// a priest after it to read the next.
	signsFresh = 12 * time.Hour
	// signsCycle is a Seven Signs cycle, a week: how long a denied sign-up
	// (signs_33_dusk_no, signs_33_dusk_firstclass) ends the errand.
	signsCycle = 7 * 24 * time.Hour
	// signsBypasses caps the links one visit sends: the sign-up takes
	// eight with the status page (3, 33, 20, Chat 0, 3, 33, 19, 4).
	signsBypasses = 10
)

// Links of the Dusk Priestess's pages (data/html/seven_signs, the cabal
// argument 1 naming Dusk; server signspriest.Command).
const (
	signsJoin   = "SevenSigns 3 1"  // dusk_priest_1: "I want to participate"
	signsAsk    = "SevenSigns 33 1" // signs_3_dusk: participate
	signsStatus = "SevenSigns 20 1" // signs_33_dusk: the seal status page
	signsBack   = "Chat 0"          // the status page's "Go back"
	// signsHunting is dusk_priest_2a's teleport to the hunting grounds, the
	// one link 2c lacks: the Dusk winner owns the Seal of Gnosis.
	signsHunting = "Quest HuntingGroundsTeleporter"
)

// signsPick is signs_33_dusk's link to the page of seal s.
func signsPick(s Seal) string { return "SevenSigns 19 1 " + strconv.Itoa(int(s)) }

// signsUp is signs_19_<seal>_dusk's sign-up link for seal s.
func signsUp(s Seal) string { return "SevenSigns 4 1 " + strconv.Itoa(int(s)) }

// signsPeriod is the Seven Signs period as a priest's page told it.
type signsPeriod uint8

const (
	// signsCompetition is the competition: dusk_priest_1 links signsJoin.
	// The zero signsPeriod is none read yet.
	signsCompetition signsPeriod = iota + 1
	// signsValidation is seal validation won by Dusk, the bot a member
	// (dusk_priest_2a, 2c).
	signsValidation
	// signsOther is every other page: recruitment, results, a validation
	// Dusk did not win or a tie (2b, 2d, 4, 5, 6): no dungeon is open to
	// the bot.
	signsOther
)

// signsPage is a page of the priest's, as its text names it.
type signsPage uint8

const (
	signsLinks signsPage = iota // led by its links
	// signsMember is signs_33_dusk_member: already a member.
	signsMember
	// signsAccepted is signs_4_dusk: the sign-up succeeded.
	signsAccepted
	// signsDenied is signs_33_dusk_no (a castle-owning clan) or
	// signs_33_dusk_firstclass (no first profession).
	signsDenied
	// signsSealStatus is the seal status page (signspriest sealStatus).
	signsSealStatus
	// signsValidated is dusk_priest_2a or 2c.
	signsValidated
)

// classifySignsPage names the priest's page html by the text that tells
// it apart from the others (data/html/seven_signs, signspriest.sealStatus).
func classifySignsPage(html string) signsPage {
	switch {
	case strings.Contains(html, "already a member"):
		return signsMember
	case strings.Contains(html, "application has been accepted"):
		return signsAccepted
	case strings.Contains(html, "Participation Denied"):
		return signsDenied
	case strings.Contains(html, "[Seal of "):
		return signsSealStatus
	case strings.Contains(html, "seal validation period is underway") && strings.Contains(html, "Revolutionaries of Dusk have vanquished"):
		return signsValidated
	}
	return signsLinks
}

// duskSeals are the seals the status page html names Dusk the owner of,
// Strife left out: a member never votes it.
func duskSeals(html string) []Seal {
	var out []Seal
	for _, s := range []struct {
		seal Seal
		name string
	}{{SealAvarice, "Avarice"}, {SealGnosis, "Gnosis"}} {
		if strings.Contains(html, "[Seal of "+s.name+": Revolutionaries of Dusk]") {
			out = append(out, s.seal)
		}
	}
	return out
}

// signsErrand is the bot's Seven Signs errand (rule "signs") and what the
// priests told it. It is owned by the brain goroutine. The server is the
// only record of the membership: after a restart the bot reads it from a
// priest's page again.
type signsErrand struct {
	v npcVisit
	// page is what the priest's open page is, held reports the seals the
	// status page named Dusk the owner of, statusRead whether the visit read
	// it, and bypasses how many links the visit sent.
	page       signsPage
	held       []Seal
	statusRead bool
	bypasses   int
	// member is set when a priest's page told the bot it is a Dusk member,
	// period the period it told, readAt when, and gnosis that Dusk, the
	// validation's winner, owns the Seal of Gnosis.
	member bool
	period signsPeriod
	readAt time.Time
	gnosis bool
	// restUntil is when the next visit may start.
	restUntil time.Time
	// shunned are the priests, by template id, that served the bot
	// nothing, and refused the seals whose dungeon gatekeeper turned the
	// bot away, until a time.
	shunned holds[int32]
	refused holds[Seal]
}

// end ends the visit; the next one starts no sooner than rest from now.
func (s *signsErrand) end(now time.Time, rest time.Duration) {
	s.v, s.page, s.held, s.statusRead, s.bypasses = npcVisit{}, signsLinks, nil, false, 0
	s.restUntil = now.Add(rest)
}

// shun ends the visit and leaves its priest alone for visitShunFor.
func (s *signsErrand) shun(now time.Time) {
	s.shunned.set(s.v.npcID, now.Add(visitShunFor))
	s.end(now, visitShunFor)
}

// sent records a packet of the visit that went out at now: a bypass or a
// talk leaves the page behind.
func (s *signsErrand) sent(a Action, now time.Time) {
	if !s.v.active() || a.object == 0 || a.object != s.v.obj {
		return
	}
	s.v.sent(a, now)
	if a.kind == actBypass || a.kind == actTalk {
		s.page = signsLinks
	}
}

// fresh reports whether what the priests told is recent enough to act on.
func (s *signsErrand) fresh(now time.Time) bool {
	return !s.readAt.IsZero() && now.Sub(s.readAt) < signsFresh
}

// vote is the seal the bot signs up with: pref when Dusk holds it, else
// another seal Dusk holds (never Strife, duskSeals), else pref.
func (s *signsErrand) vote(pref Seal) Seal {
	if len(s.held) == 0 || slices.Contains(s.held, pref) {
		return pref
	}
	return s.held[0]
}

// signsMode reports whether the bot plays Seven Signs member mode: its
// character is named in Persona.SevenSigns.Members.
func (b *Brain) signsMode() bool {
	return b.persona.SevenSigns.member(b.world.Self.Name)
}

// signsEnters reports whether the bot asks a dungeon gatekeeper of seal in
// at now (server sevensigns.DungeonEntry): a member in the competition, or
// in a validation Dusk won, of the Seal of Gnosis only when Dusk owns it;
// never a seal whose gatekeeper turned it away lately.
func (b *Brain) signsEnters(seal Seal, now time.Time) bool {
	s := &b.signs
	if !b.signsMode() || !s.member || !s.fresh(now) || s.refused.held(seal, now) {
		return false
	}
	switch s.period {
	case signsCompetition:
		return true
	case signsValidation:
		return seal != SealGnosis || s.gnosis
	}
	return false
}

// signsFarms reports whether the bot farms the spots of Seven Signs event
// ev (datapack.SevenSignsEvent) at now: the competition's ssq_event makers
// while it enters, and in a validation Dusk won the makers the seal's Dusk
// ownership switches on (ssq_seal1_twilight in the necropolises,
// ssq_seal2_twilight in the catacombs).
func (b *Brain) signsFarms(ev string, now time.Time) bool {
	switch b.signs.period {
	case signsCompetition:
		return ev == "ssq_event" && (b.signsEnters(SealAvarice, now) || b.signsEnters(SealGnosis, now))
	case signsValidation:
		switch ev {
		case "ssq_seal1_twilight":
			return b.signsEnters(SealAvarice, now)
		case "ssq_seal2_twilight":
			return b.signsEnters(SealGnosis, now)
		}
	}
	return false
}

// ruleSigns is a rule 9 errand of Seven Signs member mode: a member with a
// first profession walks to a Dusk Priestess and reads her pages: in the
// competition it signs up, once, voting the seal Dusk already holds (the
// status page) or SevenSignsPolicy.Seal; on every visit it reads the period
// and its membership, what opens the dungeons (signsEnters). A priest
// whose pages lead nowhere, or that no route reaches, is shunned. Bypasses
// pass the bypass gate (gateBypass, bypassGap above the server's
// ServerBypassTime); legs and clicks the frame pacer, at most
// signsBypasses links a visit.
func (b *Brain) ruleSigns(now time.Time) (Action, bool) {
	s := &b.signs
	if b.world.Self.InCombat || (!s.v.active() && !b.startSigns(now)) {
		return Action{}, false
	}
	v := &s.v
	if v.opened {
		cmd, rest := b.signsNext(now)
		if cmd == "" {
			s.end(now, rest)
			return Action{}, false
		}
		if s.bypasses++; s.bypasses > signsBypasses {
			s.shun(now)
			return Action{}, false
		}
		return bypassTo(v.obj, npcBypass(v.obj, cmd)), true
	}
	if s.bypasses > 0 {
		if now.Before(v.waitUntil) {
			return Action{}, true
		}
		// The link went unanswered.
		s.shun(now)
		return Action{}, false
	}
	a, ok := b.approach(v, now)
	if !ok {
		s.shun(now)
		return Action{}, false
	}
	return a, true
}

// startSigns starts a visit when one is due: at the nearest Dusk
// Priestess in sight, else at the nearest one's spawn within signsReach
// (pickVisit: a bot in a player's party or told to follow walks to none out
// of sight), its shunned priestesses left out, reporting whether it did.
func (b *Brain) startSigns(now time.Time) bool {
	s, self := &b.signs, &b.world.Self
	if !b.signsMode() || now.Before(s.restUntil) || classTier(self.ClassID) < 1 {
		return false
	}
	npcs := make([]int32, 0, len(duskPriestesses))
	for _, npcID := range duskPriestesses {
		if !s.shunned.held(npcID, now) {
			npcs = append(npcs, npcID)
		}
	}
	v, ok := b.pickVisit(npcs, signsReach, now)
	if ok {
		s.v = v
	}
	return ok
}

// signsNext is the next link of the visit on the priest's open page, or ""
// and how long the errand rests after the visit ends. It records what the
// page told: the period, the membership, the seals Dusk holds.
func (b *Brain) signsNext(now time.Time) (string, time.Duration) {
	s := &b.signs
	has := func(cmd string) bool { return b.world.Dialog.Has(npcBypass(s.v.obj, cmd)) }
	switch s.page {
	case signsMember, signsAccepted:
		s.member, s.period, s.readAt = true, signsCompetition, now
		return "", signsFresh
	case signsDenied:
		return "", signsCycle
	case signsSealStatus:
		if has(signsBack) {
			return signsBack, 0
		}
		return "", visitShunFor
	case signsValidated:
		// Only a member of the winning cabal is greeted with 2a or 2c
		// (folk_chat_signs.go priestPage).
		s.member, s.period, s.readAt, s.gnosis = true, signsValidation, now, has(signsHunting)
		return "", signsFresh
	case signsLinks:
	}
	seal := s.vote(b.persona.SevenSigns.seal())
	switch {
	case has(signsUp(seal)):
		return signsUp(seal), 0
	case has(signsPick(seal)):
		if !s.statusRead && has(signsStatus) {
			return signsStatus, 0
		}
		return signsPick(seal), 0
	case has(signsAsk):
		return signsAsk, 0
	case has(signsJoin):
		// The server clears every cabal when a validation ends (server
		// sevensigns.go, the turn out of SealValidation): a membership
		// read in another period, or a cycle ago, is of a past
		// competition; a member's sign-up pages tell it again.
		if s.period != signsCompetition || now.Sub(s.readAt) >= signsCycle {
			s.member = false
		}
		s.period, s.readAt = signsCompetition, now
		if s.member {
			return "", signsFresh
		}
		return signsJoin, 0
	}
	// Any other greeting: no competition, and no validation Dusk won with
	// the bot a member; a validation's 2a or 2c tells a membership again.
	s.member, s.period, s.readAt = false, signsOther, now
	return "", signsFresh
}

// signsPage moves the visit on with the priest's page ev: what it is, and
// the seals the status page names Dusk the owner of.
func (b *Brain) signsPage(ev NpcHtmlMessage) {
	s := &b.signs
	if !s.v.active() || !s.v.page(ev) {
		return
	}
	s.page = classifySignsPage(ev.HTML)
	if s.page == signsSealStatus {
		s.held, s.statusRead = duskSeals(ev.HTML), true
	}
}

// signsRefused records a dungeon gatekeeper of seal turning the bot away at
// now (no teleport after its link): the bot asks none of the seal's again
// until it reads the period anew.
func (b *Brain) signsRefused(seal Seal, now time.Time) {
	b.signs.refused.set(seal, now.Add(signsFresh))
}
