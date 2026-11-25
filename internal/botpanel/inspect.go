package botpanel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// The inspect (#437): GET /api/bots/{account}/inspect asks the bot's live
// session once, over its mailbox, for its inventory, worn gear, penalties
// and effects. The panel keeps none of it: the answer is shaped, written
// and dropped. Each inspect costs the bot one RequestItemList at most, so
// the panel bounds them: one in flight per bot, inspectCooldown between two
// of one bot, and maxInspects at once.
const (
	// DefaultInspectWait is how long the bot waits for the server's
	// ItemList before it answers with what it holds, stale.
	DefaultInspectWait = 3 * time.Second
	// inspectGrace is how long past InspectWait the panel waits for the
	// bot's answer before it calls the bot unresponsive.
	inspectGrace = 2 * time.Second
	// inspectCooldown spaces two inspects of one bot.
	inspectCooldown = 2 * time.Second
	// maxInspects bounds the inspects in flight over every bot.
	maxInspects = 4
)

// errInspectLimit refuses an inspect past the panel's bounds.
var errInspectLimit = errors.New("inspect refused")

// The states of an inspect answer.
const (
	inspectFresh        = "fresh"
	inspectStale        = "stale"
	inspectOffline      = "offline"
	inspectUnresponsive = "unresponsive"
)

// inspectGate bounds the inspects. mu guards the rest.
type inspectGate struct {
	mu  sync.Mutex
	now func() time.Time
	// busy are the bots with an inspect in flight, started when each
	// bot's last one started (kept for inspectCooldown).
	busy    map[string]bool
	started map[string]time.Time
}

func newInspectGate() *inspectGate {
	return &inspectGate{now: time.Now, busy: make(map[string]bool), started: make(map[string]time.Time)}
}

// enter takes an inspect of account, or refuses it with errInspectLimit.
func (g *inspectGate) enter(account string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	for a, t := range g.started {
		if now.Sub(t) >= inspectCooldown {
			delete(g.started, a)
		}
	}
	switch _, cooling := g.started[account]; {
	case g.busy[account]:
		return fmt.Errorf("%w: an inspect of %s is in flight", errInspectLimit, account)
	case cooling:
		return fmt.Errorf("%w: %s was inspected less than %v ago", errInspectLimit, account, inspectCooldown)
	case len(g.busy) >= maxInspects:
		return fmt.Errorf("%w: %d inspects in flight", errInspectLimit, maxInspects)
	}
	g.busy[account], g.started[account] = true, now
	return nil
}

// leave ends the inspect of account.
func (g *inspectGate) leave(account string) {
	g.mu.Lock()
	delete(g.busy, account)
	g.mu.Unlock()
}

// inspectView is an inspect answer: State alone for an offline or
// unresponsive bot, whose page then shows the registry's data only.
type inspectView struct {
	State     string         `json:"state"`
	At        *time.Time     `json:"at,omitempty"`
	Inventory *inventoryView `json:"inventory,omitempty"`
	Equipment []wornView     `json:"equipment,omitempty"`
	Penalties *penaltyView   `json:"penalties,omitempty"`
	Effects   []effectView   `json:"effects,omitempty"`
	// Valuables are a merchant's held valuables (docs/valuables/preset.md
	// "Wiring"), absent for a bot holding none.
	Valuables []valuableView `json:"valuables,omitempty"`
}

// valuableView is one held valuable: where it is (carried, stored, or
// both), its units in each place, its unsold rounds and its ask of one
// unit; Released once the cap gave it to the store.
type valuableView struct {
	ID       int32  `json:"id"`
	Name     string `json:"name"`
	Enchant  int32  `json:"enchant,omitempty"`
	Place    string `json:"place"`
	Carried  int64  `json:"carried"`
	Stored   int64  `json:"stored"`
	Rounds   int    `json:"rounds"`
	Ask      int64  `json:"ask"`
	Released bool   `json:"released,omitempty"`
}

// inventoryView is the carried items, worn ones left to the equipment, and
// the load against the weight limit.
type inventoryView struct {
	Items   []itemView `json:"items"`
	Load    int32      `json:"load"`
	MaxLoad int32      `json:"max_load"`
	// Percent is Load in percent of MaxLoad, 0 while the limit is unknown.
	Percent int32 `json:"percent"`
}

type itemView struct {
	ID      int32  `json:"id"`
	Name    string `json:"name"`
	Count   int32  `json:"count"`
	Enchant int32  `json:"enchant,omitempty"`
	// Weight is the stack's, absent when the catalog does not know it.
	Weight *int64 `json:"weight,omitempty"`
}

type wornView struct {
	Slot    string `json:"slot"`
	ID      int32  `json:"id"`
	Name    string `json:"name"`
	Enchant int32  `json:"enchant,omitempty"`
	// Grade is none, D, C, B, A or S; "" when the catalog does not know it.
	Grade string `json:"grade"`
}

// penaltyView is what slows the bot. GradeWeapon and GradeArmor are the
// server's grade penalty rule applied to the worn items the catalog knows
// (character_grade_penalty.go): an over-grade weapon, and each over-grade
// armor piece, a full armor twice, up to 4.
type penaltyView struct {
	WeightLevel  int32 `json:"weight_level"`
	Grade        bool  `json:"grade"`
	GradeWeapon  bool  `json:"grade_weapon"`
	GradeArmor   int   `json:"grade_armor"`
	Expertise    int32 `json:"expertise"`
	Death        int32 `json:"death"`
	RunSpeed     int32 `json:"run_speed"`
	BaseRunSpeed int32 `json:"base_run_speed,omitempty"`
}

// effectView is one of the bot's effects: LeftS seconds to run, unless
// NoEnd.
type effectView struct {
	SkillID int32  `json:"skill_id"`
	Name    string `json:"name"`
	Level   int32  `json:"level"`
	LeftS   int64  `json:"left_s"`
	NoEnd   bool   `json:"no_end,omitempty"`
}

var gradeNames = [...]string{"none", "D", "C", "B", "A", "S"}

func gradeName(g int) string {
	if g >= 0 && g < len(gradeNames) {
		return gradeNames[g]
	}
	return fmt.Sprintf("grade %d", g)
}

// inspect answers an inspect of the bot: 200 with its view, the state
// alone for an offline or unresponsive bot; 429 past the panel's bounds.
func (p *Panel) inspect(w http.ResponseWriter, r *http.Request) {
	account := r.PathValue("account")
	if err := p.inspects.enter(account); err != nil {
		writeErr(w, err)
		return
	}
	defer p.inspects.leave(account)
	reply, err := p.cfg.Supervisor.Inspect(r.Context(), account, p.cfg.InspectWait)
	switch {
	case errors.Is(err, botregistry.ErrNoRun), errors.Is(err, botclient.ErrOffline), errors.Is(err, botclient.ErrStopped):
		writeJSON(w, http.StatusOK, inspectView{State: inspectOffline})
		return
	case err != nil:
		writeErr(w, err)
		return
	}
	in, state := p.awaitInspection(r.Context(), reply)
	switch state {
	case "":
	case inspectFresh, inspectStale:
		writeJSON(w, http.StatusOK, inspectViewOf(in, state, p.cfg.Catalog))
	default:
		writeJSON(w, http.StatusOK, inspectView{State: state})
	}
}

// awaitInspection waits for the bot's answer on reply, InspectWait plus
// inspectGrace at most: its state, offline when the bot stopped first, or
// "" when the request went away.
func (p *Panel) awaitInspection(ctx context.Context, reply <-chan botclient.Inspection) (botclient.Inspection, string) {
	t := time.NewTimer(p.cfg.InspectWait + inspectGrace)
	defer t.Stop()
	select {
	case in, ok := <-reply:
		switch {
		case !ok:
			return in, inspectOffline
		case in.Stale:
			return in, inspectStale
		}
		return in, inspectFresh
	case <-t.C:
	case <-p.quit:
	case <-ctx.Done():
		return botclient.Inspection{}, ""
	}
	return botclient.Inspection{}, inspectUnresponsive
}

// inspectViewOf is the view of in, named by cat.
func inspectViewOf(in botclient.Inspection, state string, cat *Catalog) inspectView {
	at := in.At
	v := inspectView{
		State: state, At: &at,
		Inventory: &inventoryView{Items: []itemView{}, Load: in.Load, MaxLoad: in.MaxLoad},
		Equipment: []wornView{},
		Penalties: &penaltyView{
			WeightLevel: in.WeightPenalty, Grade: in.GradePenalty, Expertise: in.Expertise, Death: in.DeathPenalty,
			RunSpeed: in.RunSpeed, BaseRunSpeed: cat.runSpeed(in.ClassID),
		},
		Effects: make([]effectView, 0, len(in.Effects)),
	}
	if in.MaxLoad > 0 {
		v.Inventory.Percent = int32(int64(in.Load) * 100 / int64(in.MaxLoad))
	}
	pen := v.Penalties
	for _, it := range in.Items {
		tmpl, known := cat.item(it.TemplateID)
		if !it.Equipped {
			iv := itemView{ID: it.TemplateID, Name: cat.itemName(it.TemplateID), Count: it.Count, Enchant: int32(it.Enchant)}
			if known {
				wt := int64(tmpl.weight) * int64(max(it.Count, 1))
				iv.Weight = &wt
			}
			v.Inventory.Items = append(v.Inventory.Items, iv)
			continue
		}
		wv := wornView{Slot: botclient.BodySlotName(it.BodySlot), ID: it.TemplateID, Name: cat.itemName(it.TemplateID), Enchant: int32(it.Enchant)}
		if known {
			wv.Grade = gradeName(tmpl.grade)
			if !tmpl.arrow && int32(tmpl.grade) > in.Expertise {
				switch {
				case tmpl.weapon:
					pen.GradeWeapon = true
				case tmpl.fullArmor:
					pen.GradeArmor += 2
				default:
					pen.GradeArmor++
				}
			}
		}
		v.Equipment = append(v.Equipment, wv)
	}
	pen.GradeArmor = min(pen.GradeArmor, 4)
	for _, e := range in.Effects {
		ev := effectView{SkillID: e.SkillID, Name: cat.skillName(e.SkillID), Level: e.Level, NoEnd: e.Permanent}
		if !e.Permanent {
			ev.LeftS = int64(e.Left / time.Second)
		}
		v.Effects = append(v.Effects, ev)
	}
	for _, h := range in.Valuables {
		v.Valuables = append(v.Valuables, valuableView{
			ID: h.TemplateID, Name: cat.itemName(h.TemplateID), Enchant: h.Enchant, Place: valuablePlace(h),
			Carried: h.Carried, Stored: h.Stored, Rounds: h.Rounds, Ask: h.Ask, Released: h.Released,
		})
	}
	return v
}

// valuablePlace is where h's units are: "carried", "stored", or
// "carried, stored" when both hold some.
func valuablePlace(h botclient.HeldValuable) string {
	switch {
	case h.Carried > 0 && h.Stored > 0:
		return "carried, stored"
	case h.Stored > 0:
		return "stored"
	}
	return "carried"
}
