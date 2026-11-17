// Package datapack reads the parts of the server's XML datapack the bots plan
// with: NPC templates (npcs/*.xml), spawn territories and makers
// (spawnlist/*.xml), gatekeeper destinations (teleports.xml), merchant
// buylists (buyLists.xml) and weapon and armor templates (items/*.xml).
// Parse rules follow the server's loaders (acis_golang
// internal/gameserver/data/xml npc.go, spawnlist.go and item.go), reduced to
// the fields bots use.
//
// Load runs once per process. The returned Datapack is never mutated after
// Load returns, so any number of goroutines may read it without locking.
package datapack

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Datapack is the read-only world knowledge loaded from a datapack.
type Datapack struct {
	// NPCs maps an NPC template id to its template.
	NPCs map[int32]NPC
	// Spawnlist holds the spawn territories and makers.
	Spawnlist *Spawnlist
	// Teleports maps a gatekeeper's NPC template id to its destinations.
	Teleports map[int32][]Teleport
	// BuyLists are the merchant buylists in file order.
	BuyLists []BuyList
	// Items maps a weapon, armor or arrow template id to its template.
	Items map[int32]Item
}

// Load reads the datapack whose XML directory is xmlDir (the server's
// aCis_datapack/data/xml): every npcs/*.xml, spawnlist/*.xml and items/*.xml
// file directly under it, its teleports.xml and its buyLists.xml. xmlDir is configuration; the bot ships no datapack of its own.
func Load(xmlDir string) (*Datapack, error) {
	npcs, err := LoadNPCs(filepath.Join(xmlDir, "npcs"))
	if err != nil {
		return nil, err
	}
	spawns, err := LoadSpawnlist(filepath.Join(xmlDir, "spawnlist"))
	if err != nil {
		return nil, err
	}
	teleports, err := LoadTeleports(filepath.Join(xmlDir, "teleports.xml"))
	if err != nil {
		return nil, err
	}
	buyLists, err := LoadBuyLists(filepath.Join(xmlDir, "buyLists.xml"))
	if err != nil {
		return nil, err
	}
	items, err := LoadItems(filepath.Join(xmlDir, "items"))
	if err != nil {
		return nil, err
	}
	return &Datapack{NPCs: npcs, Spawnlist: spawns, Teleports: teleports, BuyLists: buyLists, Items: items}, nil
}

// xmlFiles lists the *.xml files directly under dir in file name order, the
// order the server loads them in. An empty directory is an error, as on the
// server.
func xmlFiles(dir, kind string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.xml"))
	if err != nil {
		return nil, fmt.Errorf("datapack: list %s files in %s: %w", kind, dir, err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("datapack: no %s files in %s", kind, dir)
	}
	slices.Sort(paths)
	return paths, nil
}

// decodeFile unmarshals the XML file at path into v.
func decodeFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("datapack: %w", err)
	}
	if err := xml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("datapack: decode %s: %w", path, err)
	}
	return nil
}
