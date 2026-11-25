package botpanel

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botparty"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// partyBotView is a bot of a party or of the free pool
// (botregistry.PartyBot).
type partyBotView struct {
	Account   string `json:"account"`
	Character string `json:"character,omitempty"`
	ClassID   *int32 `json:"class_id,omitempty"`
	Level     int32  `json:"level"`
	Preset    string `json:"preset"`
	Role      string `json:"role,omitempty"`
	Online    bool   `json:"online"`
	Slot      string `json:"slot,omitempty"`
	Town      *point `json:"town,omitempty"`
}

// partyView is a bot party (botregistry.PartyInfo).
type partyView struct {
	ID       int            `json:"id"`
	Leader   partyBotView   `json:"leader"`
	Members  []partyBotView `json:"members"`
	Players  []string       `json:"players"`
	Spot     string         `json:"spot,omitempty"`
	State    string         `json:"state"`
	Why      string         `json:"why,omitempty"`
	Since    time.Time      `json:"since"`
	FormedAt time.Time      `json:"formed_at"`
	Pinned   bool           `json:"pinned"`
	Group    string         `json:"group,omitempty"`
}

// partiesView is the organizer's view (botregistry.Parties).
type partiesView struct {
	Auto    bool           `json:"auto"`
	Parties []partyView    `json:"parties"`
	Free    []partyBotView `json:"free"`
}

func partyBotOf(b botregistry.PartyBot) partyBotView {
	v := partyBotView{
		Account: b.Account, Character: b.Character, ClassID: b.ClassID, Level: b.Level, Preset: b.Preset,
		Role: b.Role, Online: b.Online, Slot: b.Slot,
	}
	if b.Town != nil {
		t := point(*b.Town)
		v.Town = &t
	}
	return v
}

func partyBotsOf(bs []botregistry.PartyBot) []partyBotView {
	out := make([]partyBotView, 0, len(bs))
	for _, b := range bs {
		out = append(out, partyBotOf(b))
	}
	return out
}

func partiesOf(ps botregistry.Parties) partiesView {
	v := partiesView{Auto: ps.Auto, Parties: make([]partyView, 0, len(ps.Parties)), Free: partyBotsOf(ps.Free)}
	for _, p := range ps.Parties {
		players := p.Players
		if players == nil {
			players = []string{}
		}
		v.Parties = append(v.Parties, partyView{
			ID: p.ID, Leader: partyBotOf(p.Leader), Members: partyBotsOf(p.Members), Players: players, Spot: p.Spot,
			State: p.State, Why: p.Why, Since: p.Since, FormedAt: p.FormedAt, Pinned: p.Pinned, Group: p.Group,
		})
	}
	return v
}

// parties lists the bot parties, the free pool and the auto switch.
func (p *Panel) parties(w http.ResponseWriter, r *http.Request) {
	ps, err := p.cfg.Supervisor.Parties(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, partiesOf(ps))
}

// formParty forms a party by hand (botregistry.Supervisor.FormParty): the
// leader's account, the members' accounts and whether it is a pin.
func (p *Panel) formParty(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Leader  string   `json:"leader"`
		Members []string `json:"members"`
		Pin     bool     `json:"pin"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	id, err := p.cfg.Supervisor.FormParty(r.Context(), req.Leader, req.Members, req.Pin)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

// dissolveParty breaks a party on the organizer's next pass.
func (p *Panel) dissolveParty(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id < 0 {
		writeErr(w, badRequest(fmt.Errorf("party id %q", r.PathValue("id"))))
		return
	}
	if err := p.cfg.Supervisor.DissolveParty(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, okAnswer)
}

// setPartyMode sets a bot's party mode: "auto" (or "") takes part in bot
// parties, "off" never.
func (p *Panel) setPartyMode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	mode := req.Mode
	switch mode {
	case "auto", "":
		mode = ""
	case botparty.ModeOff:
	default:
		writeErr(w, badRequest(fmt.Errorf("party mode %q: want auto or off", req.Mode)))
		return
	}
	if err := p.cfg.Supervisor.SetPartyMode(r.Context(), r.PathValue("account"), mode); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, okAnswer)
}

// setPartyAuto switches auto parties on or off.
func (p *Panel) setPartyAuto(w http.ResponseWriter, r *http.Request) {
	var req struct {
		On *bool `json:"on"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.On == nil {
		writeErr(w, badRequest(fmt.Errorf("auto switch: want on true or false")))
		return
	}
	if err := p.cfg.Supervisor.SetPartyAuto(r.Context(), *req.On); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, okAnswer)
}
