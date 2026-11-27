package e2e

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// TestLiveLoginEnterLogout logs a new account in, creates its character on
// the first run, enters the world and logs out (#4). On the way it records
// the first frame of each opcode the server sends and decodes each (#7):
// into BOT_E2E_RECORD when set, else a temporary directory. The world model
// built from the EnterWorld burst matches the server's UserInfo (#8).
func TestLiveLoginEnterLogout(t *testing.T) {
	t.Parallel()
	tg := Live(t)
	ctx := scenarioCtx(t, 2*time.Minute)
	account, _ := tg.Account("a", 1)
	dir := cmp.Or(os.Getenv("BOT_E2E_RECORD"), t.TempDir())
	rec, err := botclient.NewRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := tg.EnterWhenFree(ctx, account, character(t, account, nil), rec)
	if err != nil {
		t.Fatal(err)
	}
	logoutAtEnd(t, c)
	if c.Self.ObjectID == 0 || c.Self.Slots != 1 {
		t.Fatalf("entered as %q object %d with %d characters on the account, want one", c.Self.Name, c.Self.ObjectID, c.Self.Slots)
	}
	now := time.Now()
	w, skips := botclient.NewWorld(c.Self, now)
	if len(skips) != 0 {
		t.Fatalf("world model of the burst: undecodable frames skipped: %v", skips)
	}
	pos := botclient.Point{X: c.Self.X, Y: c.Self.Y, Z: c.Self.Z}
	if s := w.Self; s.ObjectID != c.Self.ObjectID || s.Name != c.Self.Name || s.Level < 1 || s.MaxHP <= 0 || s.HP <= 0 || s.Position(now) != pos {
		t.Fatalf("world self %+v at %v, want %q at %v with level and HP", s, s.Position(now), c.Self.Name, pos)
	}
	t.Logf("entered as %s (class %d, level %d) at %v; %d objects in sight", c.Self.Name, w.Self.ClassID, w.Self.Level, pos, len(w.Objects))
	if err := c.Logout(ctx); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if err := rec.Err(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.bin"))
	if err != nil || len(files) == 0 {
		t.Fatalf("recorded nothing in %s (%v)", dir, err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := botclient.DecodeEvent(raw); err != nil {
			t.Errorf("recorded %s: %v", filepath.Base(f), err)
		}
	}
	t.Logf("recorded and decoded %d opcodes", len(files))
}

// TestLiveLogoutRefusedInCombat puts a bot in combat with a monster the GM
// spawns next to it: its Logout is refused (#4). Its socket then drops in
// the combat stance, which keeps the character in the world 15 s, so a
// login right after meets AccountInUse until the server lets it go (#12).
func TestLiveLogoutRefusedInCombat(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 4*time.Minute)
	account, _ := tg.Account("a", 2)
	bot := enter(t, ctx, tg, account, character(t, account, nil), nil)
	gm := enterGM(t, ctx, tg)

	if err := gm.Command(ctx, "teleportto "+bot.Self.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(bot.Self.ObjectID)); err != nil {
		t.Fatalf("GM next to the bot: %v", err)
	}
	if err := gm.Select(ctx, bot.Self.ObjectID); err != nil {
		t.Fatal(err)
	}
	mob, err := gm.Spawn(ctx, gremlin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bot.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		n, ok := ev.(botclient.NPCInfo)
		return ok && n.ObjectID == mob
	}); err != nil {
		t.Fatalf("bot sees the monster: %v", err)
	}
	if err := bot.Send(ctx, botwire.EncodeAction(mob, bot.Pos())); err != nil {
		t.Fatal(err)
	}
	if err := bot.Send(ctx, botwire.EncodeAttackRequest(mob, bot.Pos())); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		s, ok := ev.(botclient.AutoAttackStart)
		return ok && s.ObjectID == bot.Self.ObjectID
	}); err != nil {
		t.Fatalf("bot in combat: %v", err)
	}
	if err := bot.Logout(ctx); !errors.Is(err, botclient.ErrLogoutRefused) {
		t.Fatalf("logout in combat = %v, want ErrLogoutRefused", err)
	}

	bot.Close() // the socket drops in the combat stance
	early, err := tg.Enter(ctx, account, tg.Password(account), botclient.Character{Name: bot.Self.Name}, nil)
	if err == nil {
		logoutAtEnd(t, early) // give its place back
	}
	if !InUse(err) {
		t.Fatalf("login right after the drop = %v, want AccountInUse", err)
	}
	if err := gm.Delete(ctx, mob); err != nil {
		t.Logf("delete monster: %v", err)
	}
	again := enter(t, ctx, tg, account, botclient.Character{Name: bot.Self.Name}, nil)
	if again.Self.Name != bot.Self.Name {
		t.Fatalf("relogin entered as %q, want %q", again.Self.Name, bot.Self.Name)
	}
}

// TestLiveWrongPassword sends one wrong password for an existing account:
// the login server refuses it and the credential breaker halts every later
// login of the run (#4). It runs only with BOT_E2E_ALLOW_LOGINFAIL=1, alone
// in its process (make e2e-live-loginfail).
func TestLiveWrongPassword(t *testing.T) {
	tg := Live(t, NeedLoginFail)
	ctx := scenarioCtx(t, time.Minute)
	account, password := tg.Account("a", 1)
	if err := tg.CheckCredentials(ctx, account, password); err != nil {
		t.Fatalf("right password first: %v", err)
	}
	err := tg.WrongPassword(ctx, account)
	if le, ok := errors.AsType[*botclient.LoginError](err); !ok || le.Packet != "LoginFail" {
		t.Fatalf("wrong password = %v, want LoginFail", err)
	}
	if !errors.Is(tg.Halted(), botclient.ErrLoginsHalted) {
		t.Fatal("the refusal did not trip the credential breaker")
	}
	if err := tg.CheckCredentials(ctx, account, password); !errors.Is(err, botclient.ErrLoginsHalted) {
		t.Fatalf("login after the breaker = %v, want ErrLoginsHalted", err)
	}
}
