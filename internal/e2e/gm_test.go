package e2e

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGM(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		mode    os.FileMode
		want    GM
		wantErr string
	}{
		{"three keys", "account=e2egm\npassword=pw x\ncharacter=e2eGM\n", 0o600, GM{Account: "e2egm", password: "pw x", Character: "e2eGM"}, ""},
		{"comments, blanks and spaces", "# gm\n\n account = e2egm \npassword=pw\ncharacter=e2eGM", 0o400, GM{Account: "e2egm", password: "pw", Character: "e2eGM"}, ""},
		{"readable by the group", "account=a\npassword=pw\ncharacter=c\n", 0o640, GM{}, "mode"},
		{"password missing", "account=a\ncharacter=c\n", 0o600, GM{}, "all set"},
		{"unknown key", "account=a\npassword=hunter2\ncharacter=c\nlevel=7\n", 0o600, GM{}, "line 4"},
		{"no equals sign", "account=a\nhunter2\ncharacter=c\n", 0o600, GM{}, "line 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "gm")
			if err := os.WriteFile(path, []byte(tt.body), tt.mode); err != nil {
				t.Fatal(err)
			}
			got, err := LoadGM(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadGM = %v, want an error naming %q", err, tt.wantErr)
				}
				if strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "pw") {
					t.Fatalf("error %q carries the password", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("LoadGM = %+v, %v; want %+v", got, err, tt.want)
			}
			if s := fmt.Sprintf("%v %+v %s", got, got, got); strings.Contains(s, got.Password()) {
				t.Fatalf("GM formats as %q, with its password", s)
			}
		})
	}
}

func TestScopeCheck(t *testing.T) {
	const bot, other, mob, strangerMob = 101, 102, 201, 202
	scope := func(target int32) *Scope {
		s := newScope()
		s.addChar("e2eBot", bot)
		s.addChar("e2eOther", other)
		s.npcs[mob] = true
		s.target = target
		return s
	}
	tests := []struct {
		cmd    string
		target int32
		ok     bool
	}{
		{"teleport -84318 244579 -3730", 0, true},
		{"teleport -84318 244579", 0, false},
		{"teleport a b c", 0, false},
		{"teleportto e2ebot", 0, true},
		{"teleportto SomePlayer", 0, false},
		{"recall e2eBot", 0, true},
		{"recall party e2eBot", 0, false},
		{"recall clan e2eBot", 0, false},
		{"recall SomePlayer", 0, false},
		{"kick e2eBot", 0, true},
		{"kick all", 0, false},
		{"kick SomePlayer", bot, false},
		{"kill e2eBot", 0, true},
		{"kill e2eBot 500", 0, false},
		{"kill", mob, true},
		{"kill", strangerMob, false},
		{"kill", 0, false},
		{"res e2eOther", 0, true},
		{"heal", bot, true},
		{"heal SomePlayer", 0, false},
		{"cancel e2eBot", 0, true},
		{"cancel", mob, true},
		{"cancel SomePlayer", 0, false},
		{"cancel e2eBot 500", 0, false},
		{"set level 20", bot, true},
		{"set level 20", 0, false},
		{"set level 20", mob, false},
		{"set level twenty", bot, false},
		{"set sp 20000", bot, true},
		{"set sp 0", bot, true},
		{"set sp 100000", bot, true},
		{"set sp 100001", bot, false},
		{"set sp -1", bot, false},
		{"set sp 20000", 0, false},
		{"set sp 20000", mob, false},
		{"set exp 20000", bot, false},
		{"set class 1", bot, false},
		{"set sp", bot, false},
		{"set access 7", bot, false},
		{"skill set 1177 1", bot, true},
		{"skill set all", other, true},
		{"skill set 1177 1", 0, false},
		{"skill remove 1177", bot, false},
		{"hide", 0, true},
		{"hide on", 0, false},
		{"pledge create e2eClan7", bot, true},
		{"pledge create e2eClan7", 0, false},
		{"pledge create e2eClan7", mob, false},
		{"pledge create a", bot, false},
		{"pledge create Clan_7", bot, false},
		{"pledge create Clan7Clan7Clan7Cla", bot, false},
		{"pledge create", bot, false},
		{"pledge dismiss", other, true},
		{"pledge dismiss", 0, false},
		{"pledge level 8", bot, false},
		{"pledge transfer", bot, false},
		{"give 1061 5", bot, true},
		{"give 1835", other, true},
		{"give 1061 5", 0, false},
		{"give 1061 5", mob, false},
		{"give 57 10001", bot, false},
		{"give 57 0", bot, false},
		{"give 57 -5", bot, false},
		{"give potion 5", bot, false},
		{"give 1061 5 500", bot, false},
		{"item 1061 5", bot, false},
		{"item coin adena 100", bot, false},
		{"spawn 20001", 0, true},
		{"spawn 20001 60", 0, false},
		{"spawn gremlin", 0, false},
		{"delete", mob, true},
		{"delete", strangerMob, false},
		{"delete", bot, false},
		{"announce hello", 0, false},
		{"shutdown 1", 0, false},
		{"reload npc", 0, false},
		{"tele", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		err := scope(tt.target).check(tt.cmd)
		if tt.ok != (err == nil) || err != nil && !errors.Is(err, ErrCommandRefused) {
			t.Errorf("check(%q) with target %d = %v, want ok %v", tt.cmd, tt.target, err, tt.ok)
		}
	}
}
