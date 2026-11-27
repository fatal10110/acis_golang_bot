package botclient

import (
	"context"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// The exports below serve a game connection driven outside a brain: the
// live e2e harness (internal/e2e) scripts its bots and its GM client with
// them, and encodes its packets with botwire, the brain's own encoders, so
// every wire layout keeps one owner. The Encode functions here adapt an
// argument a script does not choose; every frame a script sends passes a
// Pacer first.

// Client packet opcodes a scenario finds in the frames a bot wrote
// (FrameTap.RecordSent): each packet's first field after the opcode is the
// object, skill, item or list it acts on (botwire.EncodeAction,
// EncodeAttackRequest, EncodeRequestMagicSkillUse, EncodeUseItem,
// EncodeRequestDropItem, EncodeSkillRequest, EncodeRequestBuyItem,
// EncodeRequestSellItem).
const (
	OpAction               byte = opGameAction
	OpAttackRequest        byte = opGameAttackRequest
	OpRequestMagicSkillUse byte = opGameRequestMagicSkillUse
	OpUseItem              byte = opGameUseItem
	OpRequestDropItem      byte = opGameRequestDropItem
	OpRequestAcquireSkill  byte = opGameRequestAcquireSkill
	OpRequestBuyItem       byte = opGameRequestBuyItem
	OpRequestSellItem      byte = opGameRequestSellItem
	// OpRequestPrivateStoreBuy's first field is the store's owner
	// (botwire.EncodeRequestPrivateStoreBuy); OpRequestHennaEquip's the
	// symbol it draws and OpRequestHennaUnequip's the symbol it removes
	// (botwire.EncodeRequestHennaEquip, EncodeRequestHennaUnequip).
	OpRequestPrivateStoreBuy byte = opGameRequestPrivateStoreBuy
	OpRequestHennaEquip      byte = opGameRequestHennaEquip
	OpRequestHennaUnequip    byte = opGameRequestHennaUnequip
	// OpRequestJoinPartyRoom's first field is the room it enters
	// (botwire.EncodeRequestJoinPartyRoom).
	OpRequestJoinPartyRoom byte = opGameRequestJoinPartyRoom
	// OpRequestEnchantItem's first field is the item it enchants
	// (botwire.EncodeRequestEnchantItem).
	OpRequestEnchantItem byte = opGameRequestEnchantItem
	// OpMoveBackwardToLocation's first fields are the X and Y it goes to
	// (botwire.EncodeMoveBackwardToLocation).
	OpMoveBackwardToLocation byte = opGameMoveBackwardToLocation
)

// RecoveryScroll is the Scroll: Recovery template a character of level
// reads to lift a death penalty level (see recoveryScroll).
func RecoveryScroll(level int32) int32 { return recoveryScroll(level) }

// RecoveryReuse is how long a reader waits between two Scrolls: Recovery
// (see recoveryReuse).
const RecoveryReuse = recoveryReuse

// EncodeWalkRunToggle switches between walking and running (the action
// bar's toggle, RequestActionUse 1).
func EncodeWalkRunToggle() []byte { return botwire.EncodeRequestActionUse(actionWalkRun) }

// EncodeRequestListPartyWaiting opens the sender's rooms window, which puts
// it on the party-matching waiting list (see botwire.EncodeRequestListPartyWaiting).
func EncodeRequestListPartyWaiting(location, levelMode int32) []byte {
	return botwire.EncodeRequestListPartyWaiting(1, location, levelMode)
}

// RoomsAnywhere and RoomsAnyLevel are the rooms window's widest filters
// (partymatch AnyLocation, and a level mode other than LevelsMine).
const (
	RoomsAnywhere int32 = -1
	RoomsAnyLevel int32 = 1
)

// EncodeRequestJoinPledge invites the player whose object id is target
// into the sender's main clan (see botwire.EncodeRequestJoinPledge).
func EncodeRequestJoinPledge(target int32) []byte { return botwire.EncodeRequestJoinPledge(target, 0) }

// StoreBuy is a row of a purchase from a private sell store: the listed
// item object, how many and the store's unit price.
type StoreBuy struct{ ObjectID, Count, Price int32 }

// EncodeRequestPrivateStoreBuy buys rows from the sell store of the player
// owner.
func EncodeRequestPrivateStoreBuy(owner int32, rows []StoreBuy) []byte {
	r := make([]storeRow, len(rows))
	for i, row := range rows {
		r[i] = storeRow{Object: row.ObjectID, Count: row.Count, Price: row.Price}
	}
	return botwire.EncodeRequestPrivateStoreBuy(owner, r)
}

// EncodeSetPrivateStoreListSell lists rows for sale, each an object of the
// seller's at a unit price, and opens the store.
func EncodeSetPrivateStoreListSell(rows []StoreBuy) []byte {
	r := make([]storeRow, len(rows))
	for i, row := range rows {
		r[i] = storeRow{Object: row.ObjectID, Count: row.Count, Price: row.Price}
	}
	return botwire.EncodeSetPrivateStoreListSell(false, r)
}

// DecodeEvent decodes one server game frame (non-empty) as the session's
// reader does: ok is false for an opcode the reader drops, and a frame that
// does not match its layout is an error.
func DecodeEvent(raw []byte) (ev Event, ok bool, err error) { return decodeEvent(raw) }

// Pacer applies the brain's send limits (pacing.go) to a connection a
// script drives: the frame bucket for every frame, and the bypass gap
// (bypassGap) for a command, which keeps GM commands at a dialog click's
// pace. Its methods may be called from any goroutine; waiters go one at a
// time.
type Pacer struct {
	clock Clock
	mu    sync.Mutex
	p     pacer // guarded by mu
}

// NewPacer returns a pacer with a full bucket, timed by clock.
func NewPacer(clock Clock) *Pacer {
	return &Pacer{clock: clock, p: newPacer(clock.Now())}
}

// Frame waits until one more frame may go out and spends it; it fails only
// once ctx ends.
func (p *Pacer) Frame(ctx context.Context) error { return p.wait(ctx, gateNone) }

// Command is Frame for a command frame, also spaced bypassGap after the
// previous command.
func (p *Pacer) Command(ctx context.Context) error { return p.wait(ctx, gateBypass) }

func (p *Pacer) wait(ctx context.Context, g gate) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		now := p.clock.Now()
		if p.p.allow(g, now) {
			p.p.sent(g, now)
			return nil
		}
		timer := p.clock.NewTimer(p.p.readyIn(g, now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C():
		}
		timer.Stop()
	}
}

// readyIn is how long until a frame passing g may go out, at least a
// millisecond: the bucket's next token and g's gap, whichever is later.
func (p *pacer) readyIn(g gate, now time.Time) time.Duration {
	p.refill(now)
	var d time.Duration
	if p.tokens < 1 {
		d = time.Duration((1 - p.tokens) / frameRate * float64(time.Second))
	}
	if last := p.gateSent[g]; gateGaps[g] > 0 && !last.IsZero() {
		d = max(d, last.Add(gateGaps[g]).Sub(now))
	}
	return max(d, time.Millisecond)
}

// CredentialRefused reports a login error the login server counts toward
// its IP ban: the error that trips the credential breaker (LoginGate.Halt).
func CredentialRefused(err error) bool { return credentialRefused(err) }
