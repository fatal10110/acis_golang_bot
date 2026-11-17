package botpanel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// maxBody bounds a request body: every one is a small JSON object.
const maxBody = 4 << 10

// maxImport bounds the accounts one import request adds.
const maxImport = 1000

func (p *Panel) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/presets", p.presets)
	mux.HandleFunc("GET /api/classes", p.classes)
	mux.HandleFunc("GET /api/spots", p.spots)
	mux.HandleFunc("GET /api/bots", p.bots)
	mux.HandleFunc("POST /api/bots", p.create)
	mux.HandleFunc("GET /api/bots/{account}", p.bot)
	mux.HandleFunc("DELETE /api/bots/{account}", p.act(p.cfg.Supervisor.Delete))
	mux.HandleFunc("POST /api/bots/{account}/spawn", p.act(p.cfg.Supervisor.Spawn))
	mux.HandleFunc("POST /api/bots/{account}/despawn", p.act(p.cfg.Supervisor.Despawn))
	mux.HandleFunc("POST /api/bots/{account}/respawn", p.act(p.cfg.Supervisor.Respawn))
	mux.HandleFunc("POST /api/bots/{account}/preset", p.setPreset)
	mux.HandleFunc("POST /api/bots/{account}/command", p.command)
	mux.HandleFunc("POST /api/import", p.importBots)
	mux.HandleFunc("GET /api/dashboard", p.dashboard)
	mux.HandleFunc("POST /api/resume-logins", p.resumeLogins)
	mux.HandleFunc("GET /api/logs", p.logs)
	mux.HandleFunc("GET /api/events", p.events)
}

// point is a world coordinate in the API.
type point struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
	Z int32 `json:"z"`
}

// live is a bot's snapshot (botclient.Status) in the API.
type live struct {
	At         time.Time `json:"at"`
	State      string    `json:"state"`
	Character  string    `json:"character,omitempty"`
	ClassID    int32     `json:"class_id"`
	Level      int32     `json:"level"`
	HP         int32     `json:"hp"`
	MaxHP      int32     `json:"max_hp"`
	MP         int32     `json:"mp"`
	MaxMP      int32     `json:"max_mp"`
	CP         int32     `json:"cp"`
	MaxCP      int32     `json:"max_cp"`
	Position   point     `json:"position"`
	Town       *point    `json:"town,omitempty"`
	Preset     string    `json:"preset,omitempty"`
	Rule       string    `json:"rule,omitempty"`
	Leader     string    `json:"leader,omitempty"`
	Members    []string  `json:"members,omitempty"`
	Order      string    `json:"order,omitempty"`
	Reconnects int       `json:"reconnects"`
	Err        string    `json:"err,omitempty"`
}

// botView is a bot of the registry, its run state and its live snapshot.
type botView struct {
	botregistry.Bot
	Run       botregistry.RunState `json:"run"`
	RestartAt time.Time            `json:"restart_at,omitzero"`
	Live      *live                `json:"live,omitempty"`
}

func viewOf(s botregistry.Status) botView {
	v := botView{Bot: s.Bot, Run: s.Run, RestartAt: s.RestartAt}
	if st := s.Live; st != nil {
		l := &live{
			At: st.At, State: st.State.String(), Character: st.Character, ClassID: st.ClassID, Level: st.Level,
			HP: st.HP, MaxHP: st.MaxHP, MP: st.MP, MaxMP: st.MaxMP, CP: st.CP, MaxCP: st.MaxCP,
			Position: point(st.Position), Preset: st.Preset, Rule: st.Rule, Leader: st.Leader, Members: st.Members,
			Order: st.Order.String(), Reconnects: st.Reconnects,
		}
		if st.Town != nil {
			t := point(*st.Town)
			l.Town = &t
		}
		if st.Err != nil {
			l.Err = st.Err.Error()
		}
		v.Live = l
	}
	return v
}

// dashboardView is the gate, the breaker and the metrics.
type dashboardView struct {
	// Halted is set while the credential breaker holds every login;
	// HaltErr is why.
	Halted    bool                 `json:"halted"`
	HaltErr   string               `json:"halt_err,omitempty"`
	Bots      int                  `json:"bots"`
	Running   int                  `json:"running"`
	MaxOnline int                  `json:"max_online"`
	Metrics   *botmetrics.Snapshot `json:"metrics"`
}

func (p *Panel) presets(w http.ResponseWriter, _ *http.Request) {
	classes := botclient.StartingClasses()
	ids := make([]int32, len(classes))
	for i, c := range classes {
		ids[i] = c.ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"presets": p.cfg.Presets, "classes": ids})
}

func (p *Panel) classes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, classList())
}

// spotView is a farming spot in the API: name is the argument of "go".
type spotView struct {
	Name       string `json:"name"`
	MinLevel   int    `json:"min_level"`
	MaxLevel   int    `json:"max_level"`
	AggroLevel int    `json:"aggro_level"`
	Cap        int    `json:"cap"`
	Held       int    `json:"held"`
}

// spots lists the farming spots, how many bots each takes and holds; none
// without -datapack.
func (p *Panel) spots(w http.ResponseWriter, _ *http.Request) {
	st := p.cfg.Spots.Statuses()
	out := make([]spotView, len(st))
	for i, s := range st {
		out[i] = spotView(s)
	}
	writeJSON(w, http.StatusOK, out)
}

func (p *Panel) list(r *http.Request) ([]botView, error) {
	l, err := p.cfg.Supervisor.List(r.Context())
	if err != nil {
		return nil, err
	}
	out := make([]botView, len(l))
	for i, s := range l {
		out[i] = viewOf(s)
	}
	return out, nil
}

func (p *Panel) bots(w http.ResponseWriter, r *http.Request) {
	l, err := p.list(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (p *Panel) bot(w http.ResponseWriter, r *http.Request) {
	l, err := p.list(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	i := slices.IndexFunc(l, func(v botView) bool { return v.Account == r.PathValue("account") })
	if i < 0 {
		writeErr(w, botregistry.ErrNoBot)
		return
	}
	writeJSON(w, http.StatusOK, l[i])
}

// offered refuses a preset the panel does not offer.
func (p *Panel) offered(preset string) error {
	i := slices.IndexFunc(p.cfg.Presets, func(o Preset) bool { return o.Name == preset })
	switch {
	case i < 0:
		return badRequest(fmt.Errorf("preset %q is not offered", preset))
	case p.cfg.Presets[i].Disabled != "":
		return badRequest(fmt.Errorf("preset %q is not offered: %s", preset, p.cfg.Presets[i].Disabled))
	}
	return nil
}

func (p *Panel) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Preset string `json:"preset"`
		Class  *int32 `json:"class"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := p.offered(req.Preset); err != nil {
		writeErr(w, err)
		return
	}
	if req.Class != nil && !slices.ContainsFunc(botclient.StartingClasses(), func(c botclient.StartingClass) bool { return c.ID == *req.Class }) {
		writeErr(w, badRequest(fmt.Errorf("class %d is not a starting class", *req.Class)))
		return
	}
	b, err := p.cfg.Supervisor.Create(r.Context(), req.Preset, req.Class)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// act serves a request that takes only the bot's account.
func (p *Panel) act(fn func(ctx context.Context, account string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(r.Context(), r.PathValue("account")); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, okAnswer)
	}
}

func (p *Panel) setPreset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Preset string `json:"preset"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := p.offered(req.Preset); err != nil {
		writeErr(w, err)
		return
	}
	if err := p.cfg.Supervisor.SetPreset(r.Context(), r.PathValue("account"), req.Preset); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, okAnswer)
}

// parseCommand reads a command as the panel writes it: "stay", "go Wolves",
// "follow Name".
func parseCommand(s string) (botclient.Command, error) {
	verb, arg, _ := strings.Cut(strings.TrimSpace(s), " ")
	for k := botclient.CommandStay; k <= botclient.CommandLogout; k++ {
		if k.String() == verb {
			return botclient.Command{Kind: k, Arg: strings.TrimSpace(arg)}, nil
		}
	}
	return botclient.Command{}, badRequest(fmt.Errorf("%w: %q", botclient.ErrBadCommand, s))
}

// command hands a command to the bot and waits up to CommandWait for its
// answer: "done", the bot's refusal (unknown spot, that spot is full, ...),
// or "queued" when the bot has not answered yet (it waits for its next
// session). It never sends a whisper.
func (p *Panel) command(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Command string `json:"command"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	c, err := parseCommand(req.Command)
	if err != nil {
		writeErr(w, err)
		return
	}
	account := r.PathValue("account")
	answer, err := p.cfg.Supervisor.Command(r.Context(), account, c)
	if err != nil {
		writeErr(w, err)
		return
	}
	p.cfg.Log.Info("panel command", "account", account, "command", c.String())
	t := time.NewTimer(p.cfg.CommandWait)
	defer t.Stop()
	select {
	case err := <-answer:
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errAnswer{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, okAnswer)
	case <-t.C:
		writeJSON(w, http.StatusAccepted, answerView{Answer: "queued"})
	case <-r.Context().Done():
	case <-p.quit:
		writeJSON(w, http.StatusAccepted, answerView{Answer: "queued"})
	}
}

func (p *Panel) importBots(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prefix   string `json:"prefix"`
		Count    int    `json:"count"`
		Marker   string `json:"marker"`
		MarkerAt string `json:"marker_at"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	m := botnames.DefaultMarker
	if req.Marker != "" {
		m.Text = req.Marker
	}
	var err error
	if req.MarkerAt != "" {
		if m.Placement, err = botnames.ParsePlacement(req.MarkerAt); err != nil {
			writeErr(w, badRequest(err))
			return
		}
	}
	// The bound is checked on its own, before anything sizes a slice by
	// Count: the registry allocates one name per bot.
	if req.Count < 1 || req.Count > maxImport {
		writeErr(w, badRequest(fmt.Errorf("count %d: want 1 to %d", req.Count, maxImport)))
		return
	}
	if err := errors.Join(botaccounts.ValidatePrefix(req.Prefix, req.Count), m.Validate()); err != nil {
		writeErr(w, badRequest(err))
		return
	}
	added, err := p.cfg.Supervisor.Import(r.Context(), req.Prefix, m, req.Count)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": len(added)})
}

func (p *Panel) dashboardOf(l []botView) dashboardView {
	d := dashboardView{Bots: len(l), MaxOnline: p.cfg.MaxOnline, Metrics: p.cfg.Metrics.Last()}
	for _, v := range l {
		if v.Run == botregistry.RunActive {
			d.Running++
		}
	}
	if err := p.cfg.Gate.Err(); err != nil {
		d.Halted, d.HaltErr = true, err.Error()
	}
	return d
}

func (p *Panel) dashboard(w http.ResponseWriter, r *http.Request) {
	l, err := p.list(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p.dashboardOf(l))
}

func (p *Panel) resumeLogins(w http.ResponseWriter, r *http.Request) {
	if err := p.cfg.Supervisor.ResumeLogins(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, okAnswer)
}

// logs serves the latest log lines: ?account= filters one bot's, ?n= bounds
// how many.
func (p *Panel) logs(w http.ResponseWriter, r *http.Request) {
	n := 0
	if s := r.URL.Query().Get("n"); s != "" {
		var err error
		if n, err = strconv.Atoi(s); err != nil || n < 0 {
			writeErr(w, badRequest(fmt.Errorf("n %q: want a count", s)))
			return
		}
	}
	writeJSON(w, http.StatusOK, p.cfg.Logs.Lines(r.URL.Query().Get("account"), n))
}

// answerView is the answer of a request that returns nothing else.
type answerView struct {
	Answer string `json:"answer"`
}

var okAnswer = answerView{Answer: "done"}

type errAnswer struct {
	Error string `json:"error"`
}

// requestError is a request the panel refuses before the supervisor sees
// it.
type requestError struct{ err error }

func (e requestError) Error() string { return e.err.Error() }
func (e requestError) Unwrap() error { return e.err }

func badRequest(err error) error { return requestError{err} }

func readJSON(r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBody))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return badRequest(fmt.Errorf("request body: %w", err))
	}
	return nil
}

// statusOf maps a refusal to its HTTP status.
func statusOf(err error) int {
	var re requestError
	switch {
	case errors.As(err, &re), errors.Is(err, botclient.ErrBadCommand):
		return http.StatusBadRequest
	case errors.Is(err, botregistry.ErrNoBot):
		return http.StatusNotFound
	case errors.Is(err, botregistry.ErrOnline), errors.Is(err, botregistry.ErrOffline), errors.Is(err, botregistry.ErrMaxOnline),
		errors.Is(err, botregistry.ErrNoRun), errors.Is(err, botclient.ErrBusy), errors.Is(err, botclient.ErrStopped):
		return http.StatusConflict
	case errors.Is(err, botregistry.ErrStopped):
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

func writeErr(w http.ResponseWriter, err error) {
	writeJSON(w, statusOf(err), errAnswer{Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
