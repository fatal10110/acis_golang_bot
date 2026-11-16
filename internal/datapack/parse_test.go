package datapack

import (
	"encoding/xml"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildNPCErrors(t *testing.T) {
	tests := []struct {
		name    string
		xml     string
		wantErr string
	}{
		{"missing id", `<npc name="a"><set name="type" val="Monster"/></npc>`, `attribute "id": required`},
		{"bad id", `<npc id="1x" name="a"><set name="type" val="Monster"/></npc>`, `attribute "id"`},
		{"missing name", `<npc id="1"><set name="type" val="Monster"/></npc>`, `attribute "name": required`},
		{"missing type", `<npc id="1" name="a"><set name="level" val="2"/></npc>`, `attribute "type": required`},
		{"bad level", `<npc id="1" name="a"><set name="type" val="Monster"/><set name="level" val="2.5"/></npc>`, `attribute "level"`},
		{"bad aggroRange", `<npc id="1" name="a"><set name="type" val="Monster"/><set name="aggroRange" val="x"/></npc>`, `attribute "aggroRange"`},
		{"duplicate id", `<npc id="1" name="a"><set name="type" val="Monster"/></npc><npc id="01" name="b"><set name="type" val="Folk"/></npc>`, "npc 1: duplicate id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f npcFile
			if err := xml.Unmarshal([]byte("<list>"+tt.xml+"</list>"), &f); err != nil {
				t.Fatal(err)
			}
			err := addNPCs(make(map[int32]NPC), f.NPCs)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestBuildSpawnlistErrors(t *testing.T) {
	const tri = `<territory name="t" minZ="0" maxZ="1"><node x="0" y="0"/><node x="1" y="0"/><node x="0" y="1"/></territory>`
	tests := []struct {
		name    string
		xml     string
		wantErr string
	}{
		{"territory without name", `<territory minZ="0" maxZ="1"/>`, `attribute "name": required`},
		{"territory without minZ", `<territory name="t" maxZ="1"/>`, `attribute "minZ": required`},
		{"territory bad maxZ", `<territory name="t" minZ="0" maxZ="z"/>`, `attribute "maxZ"`},
		{"node without y", `<territory name="t" minZ="0" maxZ="1"><node x="0"/></territory>`, `attribute "y": required`},
		{"maker without name", tri + `<npcmaker territory="t" maximumNpcs="1"/>`, `attribute "name": required`},
		{"maker without maximumNpcs", tri + `<npcmaker name="m" territory="t"/>`, `attribute "maximumNpcs": required`},
		{"spawn without id", tri + `<npcmaker name="m" territory="t" maximumNpcs="1"><npc total="1"/></npcmaker>`, `attribute "id": required`},
		{"spawn without total", tri + `<npcmaker name="m" territory="t" maximumNpcs="1"><npc id="1"/></npcmaker>`, `attribute "total": required`},
		{"spawn bad respawn", tri + `<npcmaker name="m" territory="t" maximumNpcs="1"><npc id="1" total="1" respawn="xsec"/></npcmaker>`, `attribute "respawn"`},
		{"spawn bad pos", tri + `<npcmaker name="m" territory="t" maximumNpcs="1"><npc id="1" total="1" pos="1;2;3"/></npcmaker>`, "malformed pos"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f spawnlistFile
			if err := xml.Unmarshal([]byte("<list>"+tt.xml+"</list>"), &f); err != nil {
				t.Fatal(err)
			}
			_, err := buildSpawnlist([]string{"f.xml"}, []spawnlistFile{f})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeInt32(t *testing.T) {
	tests := []struct {
		in      string
		want    int32
		wantErr bool
	}{
		{in: "0", want: 0},
		{in: "20001", want: 20001},
		{in: "-182829", want: -182829},
		{in: "+7", want: 7},
		{in: "0x4E85", want: 20101},
		{in: "#ff", want: 255},
		{in: "017", want: 15},
		{in: "2147483647", want: 2147483647},
		{in: "-2147483648", want: -2147483648},
		{in: "2147483648", wantErr: true},
		{in: "-2147483649", wantErr: true},
		{in: "", wantErr: true},
		{in: "-", wantErr: true},
		{in: "--1", wantErr: true},
		{in: "0x", wantErr: true},
		{in: "1_000", wantErr: true},
		{in: "08", wantErr: true},
		{in: " 1", wantErr: true},
	}
	for _, tt := range tests {
		got, err := decodeInt32(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("decodeInt32(%q) = %d, %v; want %d, error %v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestParseGameDuration(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{in: "90sec", want: 90 * time.Second},
		{in: "10min", want: 10 * time.Minute},
		{in: "48hour", want: 48 * time.Hour},
		{in: "0sec", want: 0},
		{in: "no", want: NoRespawn},
		{in: "NO", want: NoRespawn},
		{in: "", want: 0},
		{in: "15", want: 0},
		{in: "xsec", wantErr: true},
		{in: "1.5hour", wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseGameDuration(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("parseGameDuration(%q) = %v, %v; want %v, error %v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestParsePositions(t *testing.T) {
	tests := []struct {
		in      string
		want    []Position
		wantErr bool
	}{
		{in: "", want: nil},
		{in: " ;; ", want: nil},
		{in: "1;2;3;4", want: []Position{{X: 1, Y: 2, Z: 3, Heading: 4}}},
		{in: "1;2;3;4;", want: []Position{{X: 1, Y: 2, Z: 3, Heading: 4}}},
		{in: "1;2;3;4;50%;5;6;7;8;50%;", want: []Position{
			{X: 1, Y: 2, Z: 3, Heading: 4, Chance: 50},
			{X: 5, Y: 6, Z: 7, Heading: 8, Chance: 50},
		}},
		{in: "1;2;3;4;50", want: []Position{{X: 1, Y: 2, Z: 3, Heading: 4, Chance: 50}}},
		// Leftover fields after the last whole weighted tuple are ignored.
		{in: "1;2;3;4;50%;9;9", want: []Position{{X: 1, Y: 2, Z: 3, Heading: 4, Chance: 50}}},
		{in: "1;2;3", wantErr: true},
		{in: "1;2;x;4", wantErr: true},
		{in: "1;2;3;4;x%", wantErr: true},
	}
	for _, tt := range tests {
		got, err := parsePositions(tt.in)
		if (err != nil) != tt.wantErr || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parsePositions(%q) = %+v, %v; want %+v, error %v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}
