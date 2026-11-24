package datapack

import (
	"encoding/xml"
	"fmt"
	"time"
)

// NPC is the part of an NPC template (npcs/*.xml) bots plan with.
type NPC struct {
	ID   int32
	Name string
	// Type is the template's instance type, such as "Monster", "RaidBoss",
	// "GrandBoss", "Folk" or "Merchant".
	Type  string
	Level int
	// AggroRange is the distance at which the NPC attacks on sight; 0 means
	// it is passive.
	AggroRange int
	// RunSpeed is the template's run speed (runSpd), 0 when absent.
	RunSpeed float64
	// ClanRange is how far around it an attacked NPC calls the members of
	// its clan to join the fight (the server's makesClanCalls and
	// forEachClanMember, npc/clan.go); 0 for an NPC of no clan.
	ClanRange int
	// Combat is what the NPC fights with.
	Combat Combat
	// CorpseTime is how long the NPC's corpse stays after death (corpseTime,
	// 7 s when absent): the window a spoiler has to sweep it.
	CorpseTime time.Duration
	// TeachTo are the class IDs a trainer teaches skills to (<teachTo
	// classes>), in file order; nil for an NPC that teaches none.
	TeachTo []int32
}

// Combat is an NPC template's base combat stats, at the server's defaults
// (data/xml/npc.go buildNPCTemplate) where the template leaves one out:
// before the server's stat pipeline finalizes them (hostile_stats.go
// defaultBuiltin: max HP by CON, P.Atk by STR and level, P.Def by level,
// attack speed and critical rate by DEX).
type Combat struct {
	HP, PAtk, PDef, MAtk, MDef float64
	// AtkSpd is the base physical attack speed, Crit the base critical rate.
	AtkSpd, Crit float64
	// STR to MEN are the six base stats.
	STR, CON, DEX, INT, WIT, MEN int
}

type npcFile struct {
	NPCs []npcElement `xml:"npc"`
}

// npcElement is one <npc>: its own attributes (id, name, ...), its direct
// <set> children and its <teachTo>. Nested blocks (<ai>, <drops>,
// <skills>) are not read.
type npcElement struct {
	Attrs   []xml.Attr    `xml:",any,attr"`
	Sets    []setElement  `xml:"set"`
	TeachTo *attrsElement `xml:"teachTo"`
}

// LoadNPCs reads every *.xml NPC template file directly under dir, in
// sorted file order. A file that is not well-formed, or an <npc> missing or
// mangling a field bots read, fails the whole load. An id declared twice
// keeps its last declaration, as the server's template table does.
func LoadNPCs(dir string) (map[int32]NPC, error) {
	paths, err := xmlFiles(dir, "npc")
	if err != nil {
		return nil, err
	}
	npcs := make(map[int32]NPC)
	for _, path := range paths {
		var f npcFile
		if err := decodeFile(path, &f); err != nil {
			return nil, err
		}
		if err := addNPCs(npcs, f.NPCs); err != nil {
			return nil, fmt.Errorf("datapack: %s: %w", path, err)
		}
	}
	return npcs, nil
}

// maxClassID is the highest profession ID (the server's player.MaxClassID).
const maxClassID = 118

func addNPCs(npcs map[int32]NPC, els []npcElement) error {
	for _, el := range els {
		npc, err := buildNPC(el)
		if err != nil {
			return err
		}
		npcs[npc.ID] = npc
	}
	return nil
}

// buildNPC mirrors the server's buildNPCTemplate for the fields bots read:
// <set> values override same-named attributes, id, name and type are
// required, level defaults to 1, aggroRange to 0 and runSpd to 0, clanRange
// is required with a clan and read only then, the combat stats default as
// the server's do (hp, pAtk, pDef, mAtk and mDef to 0, which the server
// requires of all but hp; the six base stats to 40, 21, 30, 20, 43 and 20),
// corpseTime defaults to 7 seconds, and a <teachTo> needs its classes, each
// a class ID from 0 to 118.
func buildNPC(el npcElement) (NPC, error) {
	a := foldAttrs(el.Attrs)
	for _, s := range el.Sets {
		a.vals[s.Name] = s.Val
	}
	npc := NPC{
		ID:         a.literal("id"),
		Name:       a.str("name"),
		Type:       a.str("type"),
		Level:      a.intDefault("level", 1),
		AggroRange: a.intDefault("aggroRange", 0),
		RunSpeed:   a.floatDefault("runSpd", 0),
		Combat: Combat{
			HP: a.floatDefault("hp", 0), PAtk: a.floatDefault("pAtk", 0), PDef: a.floatDefault("pDef", 0),
			MAtk: a.floatDefault("mAtk", 0), MDef: a.floatDefault("mDef", 0),
			AtkSpd: a.floatDefault("atkSpd", 300), Crit: a.floatDefault("crit", 4),
			STR: a.intDefault("str", 40), CON: a.intDefault("con", 21), DEX: a.intDefault("dex", 30),
			INT: a.intDefault("int", 20), WIT: a.intDefault("wit", 43), MEN: a.intDefault("men", 20),
		},
		CorpseTime: time.Duration(a.intDefault("corpseTime", 7)) * time.Second,
	}
	if el.TeachTo != nil {
		t := foldAttrs(el.TeachTo.Attrs)
		npc.TeachTo = t.ints("classes")
		for _, id := range npc.TeachTo {
			if id < 0 || id > maxClassID {
				t.fail("classes", fmt.Errorf("class id %d out of range", id))
			}
		}
		if t.err != nil {
			return NPC{}, fmt.Errorf("npc %s: teachTo: %w", a.strDefault("id", "?"), t.err)
		}
	}
	if _, clan := a.vals["clan"]; clan {
		npc.ClanRange = a.int("clanRange")
	}
	if a.err != nil {
		return NPC{}, fmt.Errorf("npc %s: %w", a.strDefault("id", "?"), a.err)
	}
	return npc, nil
}
