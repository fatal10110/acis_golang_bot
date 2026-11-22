package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

func TestConfigFromEnv(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		name string
		env  map[string]string
		dir  string
		want Config
	}{
		{"nothing set, no dir", nil, "", Config{Prefix: DefaultPrefix}},
		{"files default to the dir", nil, "/p/.e2e", Config{Prefix: DefaultPrefix, SecretFile: "/p/.e2e/secret"}},
		{
			"every variable",
			map[string]string{
				EnvLogin: "h:2106", EnvSecretFile: "/s", EnvPrefix: "e2x", EnvAllowlisted: "1",
				EnvGMFile: "/g", EnvDatapack: "/d", EnvAllowLoginFail: "1",
				EnvGeodata: "/geo", EnvGeoengine: "/ge", EnvGeodataCache: "/gc", EnvNavCache: "/nc",
			},
			"/p/.e2e",
			Config{
				Login: "h:2106", SecretFile: "/s", Prefix: "e2x", Allowlisted: true, GMFiles: []string{"/g"}, Datapack: "/d", AllowLoginFail: true,
				Nav: botclient.NavFiles{Geodata: "/geo", Geoengine: "/ge", Cache: "/gc", NavCache: "/nc"},
			},
		},
		{"only 1 switches on", map[string]string{EnvAllowlisted: "true", EnvAllowLoginFail: "yes"}, "", Config{Prefix: DefaultPrefix}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConfigFromEnv(env(tt.env), tt.dir); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ConfigFromEnv = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestConfigMissing(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	full := Config{Login: "h:2106", SecretFile: file, GMFiles: []string{file}, Datapack: dir, AllowLoginFail: true,
		Nav: botclient.NavFiles{Geodata: dir, Geoengine: file, Cache: file, NavCache: file}}
	tests := []struct {
		name  string
		cfg   Config
		needs []Need
		want  []string // a substring per missing item
	}{
		{"nothing configured", Config{}, []Need{NeedGM}, []string{EnvLogin, EnvSecretFile, EnvGMFile}},
		{"login only", full, nil, nil},
		{"every need", full, []Need{NeedGM, NeedDatapack, NeedLoginFail, NeedNav}, nil},
		{"nav unset", Config{Login: "h:2106", SecretFile: file, Datapack: dir}, []Need{NeedNav}, []string{EnvGeodata, EnvGeoengine, EnvGeodataCache, EnvNavCache}},
		{"nav cache absent", Config{Login: "h:2106", SecretFile: file, Datapack: dir, Nav: botclient.NavFiles{Geodata: dir, Geoengine: file, Cache: file, NavCache: filepath.Join(dir, "none")}}, []Need{NeedNav}, []string{EnvNavCache}},
		{"secret file absent", Config{Login: "h:2106", SecretFile: filepath.Join(dir, "none")}, nil, []string{EnvSecretFile}},
		{"GM not needed is not checked", Config{Login: "h:2106", SecretFile: file}, nil, nil},
		{"GM file absent", Config{Login: "h:2106", SecretFile: file, GMFiles: []string{file, filepath.Join(dir, "none")}}, []Need{NeedGM}, []string{EnvGMFile}},
		{"login fail not allowed", Config{Login: "h:2106", SecretFile: file}, []Need{NeedLoginFail}, []string{EnvAllowLoginFail}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.Missing(tt.needs...)
			if len(got) != len(tt.want) {
				t.Fatalf("Missing = %q, want %d items like %q", got, len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if !strings.Contains(got[i], w) {
					t.Fatalf("Missing[%d] = %q, want it to name %s", i, got[i], w)
				}
			}
		})
	}
}

func TestCheckPrefix(t *testing.T) {
	tests := []struct {
		prefix, panel string
		reserved      bool
		invalid       bool
	}{
		{prefix: "e2e"},
		{prefix: "e2e", panel: "zz"},
		{prefix: "hz", reserved: true},
		{prefix: "hzx", reserved: true},
		{prefix: "h", reserved: true},
		{prefix: "e2e", panel: "e2e", reserved: true},
		{prefix: "e2e", panel: "e2ep", reserved: true},
		{prefix: "e2ex", panel: "e2e", reserved: true},
		{prefix: "E2E", invalid: true},
		{prefix: "e2", invalid: true},
		{prefix: "", invalid: true},
	}
	for _, tt := range tests {
		err := CheckPrefix(tt.prefix, tt.panel)
		switch {
		case tt.reserved && !errors.Is(err, ErrReservedPrefix):
			t.Errorf("CheckPrefix(%q, %q) = %v, want ErrReservedPrefix", tt.prefix, tt.panel, err)
		case tt.invalid && (err == nil || errors.Is(err, ErrReservedPrefix)):
			t.Errorf("CheckPrefix(%q, %q) = %v, want an invalid prefix", tt.prefix, tt.panel, err)
		case !tt.reserved && !tt.invalid && err != nil:
			t.Errorf("CheckPrefix(%q, %q) = %v", tt.prefix, tt.panel, err)
		}
	}
}

func TestDefaultDirIsThePrimaryCheckouts(t *testing.T) {
	dir := DefaultDir()
	if dir == "" {
		t.Skip("not in a git checkout")
	}
	if filepath.Base(dir) != ConfigDirName || slices.Contains(strings.Split(dir, string(filepath.Separator)), "worktrees") && strings.Contains(dir, ".git") {
		t.Fatalf("DefaultDir = %q, want the primary checkout's %s", dir, ConfigDirName)
	}
}
