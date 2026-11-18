package botclient

import (
	"context"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// The exports below serve a game connection driven outside a brain: the
// live e2e harness (internal/e2e) scripts its bots and its GM client with
// them. Each encoder wraps the brain's own, so every wire layout keeps one
// owner; every frame a script sends passes a Pacer first.

// EncodeAction selects (or, on the object already selected, acts on)
// objectID, clicked from origin.
func EncodeAction(objectID int32, origin Point) []byte { return encodeAction(objectID, origin) }

// EncodeAttackRequest force-attacks objectID from origin (see
// encodeAttackRequest).
func EncodeAttackRequest(objectID int32, origin Point) []byte {
	return encodeAttackRequest(objectID, origin)
}

// EncodeAppearing ends a teleport: the server keeps a teleported character
// out of the world until it arrives (client_ingame.go handleAppearing).
func EncodeAppearing() []byte { return encodeAppearing() }

// RestartVillage is RequestRestartPoint's point type "to village"
// (restart_selection.go).
const RestartVillage int32 = 0

// EncodeRequestRestartPoint asks a dead character to restart at the point of
// type kind (RestartVillage).
func EncodeRequestRestartPoint(kind int32) []byte { return encodeRequestRestartPoint(kind) }

// EncodeSendBypassBuildCmd is a GM's admin command: cmd is the command line
// without its leading "//" (server clientpackets/admin.go, one string). The
// server runs it only for a character of enough access level
// (network/admin.go sendBypassBuildCmd).
func EncodeSendBypassBuildCmd(cmd string) []byte {
	w := wire.NewPacketWriter(opGameSendBypassBuildCmd)
	w.WriteString(cmd)
	return w.Bytes()
}

// Client packet opcodes a scenario finds in the frames a bot wrote
// (FrameTap.RecordSent): each packet's first field after the opcode is the
// object, skill or item it acts on (encodeAction, encodeAttackRequest,
// encodeRequestMagicSkillUse, encodeUseItem, encodeRequestDropItem).
const (
	OpAction               byte = opGameAction
	OpAttackRequest        byte = opGameAttackRequest
	OpRequestMagicSkillUse byte = opGameRequestMagicSkillUse
	OpUseItem              byte = opGameUseItem
	OpRequestDropItem      byte = opGameRequestDropItem
)

// EncodeMoveBackwardToLocation walks or runs to to from origin, as a mouse
// click (see encodeMoveBackwardToLocation).
func EncodeMoveBackwardToLocation(to, origin Point) []byte {
	return encodeMoveBackwardToLocation(to, origin)
}

// EncodeWalkRunToggle switches between walking and running (the action
// bar's toggle, RequestActionUse 1).
func EncodeWalkRunToggle() []byte { return encodeRequestActionUse(actionWalkRun) }

// EncodeRequestJoinParty invites the player named target into the sender's
// party under loot rule loot (see encodeRequestJoinParty).
func EncodeRequestJoinParty(target string, loot int32) []byte {
	return encodeRequestJoinParty(target, loot)
}

// EncodeSay2Tell whispers text to the player named target.
func EncodeSay2Tell(target, text string) []byte { return encodeSay2Tell(target, text) }

// Say2 channels a scenario speaks on besides a whisper (handler/chat):
// the server delivers shout and trade lines to the players of a region.
const (
	ChatShout int32 = 1
	ChatTrade int32 = 8
)

// EncodeSay2 says text on channel (ChatShout, ChatTrade); a whisper is
// EncodeSay2Tell.
func EncodeSay2(text string, channel int32) []byte { return encodeSay2(text, channel) }

// EncodeTradeRequest asks the player objectID to trade.
func EncodeTradeRequest(objectID int32) []byte { return encodeTradeRequest(objectID) }

// EncodeAddTradeItem adds count of the item objectID to the sender's offer
// in its open trade.
func EncodeAddTradeItem(objectID, count int32) []byte { return encodeAddTradeItem(objectID, count) }

// EncodeTradeDone confirms the open trade, or cancels it.
func EncodeTradeDone(confirm bool) []byte { return encodeTradeDone(confirm) }

// StoreBuy is a row of a purchase from a private sell store: the listed
// item object, how many and the store's unit price.
type StoreBuy struct{ ObjectID, Count, Price int32 }

// EncodeRequestPrivateStoreBuy buys rows from the sell store of the player
// owner.
func EncodeRequestPrivateStoreBuy(owner int32, rows []StoreBuy) []byte {
	r := make([]storeRow, len(rows))
	for i, row := range rows {
		r[i] = storeRow{object: row.ObjectID, count: row.Count, price: row.Price}
	}
	return encodeRequestPrivateStoreBuy(owner, r)
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
