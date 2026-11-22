// Package datapack reads the parts of the server's XML datapack the bots plan
// with: NPC templates (npcs/*.xml), spawn territories and makers
// (spawnlist/*.xml), gatekeeper destinations (teleports.xml), merchant
// buylists (buyLists.xml), weapon, armor and etc item templates
// (items/*.xml), skill templates (skills/*.xml), craft recipes
// (recipes.xml), armor sets (armorSets.xml) and water zones
// (zones/WaterZone.xml). Parse rules follow the server's loaders (acis_golang
// internal/gameserver/data/xml npc.go, spawnlist.go, item.go, skill.go,
// economy.go and zone.go), reduced to the fields bots use.
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
	// EtcItems maps the template id of an etc item bots use (shots,
	// potions, scrolls, enchant scrolls, materials, dyes) to its template.
	EtcItems map[int32]EtcItem
	// Skills maps a skill level to its template.
	Skills map[SkillRef]Skill
	// Prices maps the id of every template a private store can list (any
	// type, tradable, priced) to its reference price in adena: the price
	// a store's manage list shows.
	Prices map[int32]int32
	// Recipes maps a craft recipe id to its recipe.
	Recipes map[int32]Recipe
	// ArmorSets maps an armor set's chest template id to the set.
	ArmorSets map[int32]ArmorSet
	// Hennas maps a dye symbol id to its symbol.
	Hennas map[int32]Henna
	// WaterZones are the water zones in file order: where a player swims.
	WaterZones []WaterZone
}

// Load reads the datapack whose XML directory is xmlDir (the server's
// aCis_datapack/data/xml): every npcs/*.xml, spawnlist/*.xml, items/*.xml
// and skills/*.xml file directly under it, its teleports.xml, its
// buyLists.xml, its recipes.xml, its armorSets.xml, its hennas.xml and its
// zones/WaterZone.xml. xmlDir is configuration; the bot ships no datapack of its
// own.
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
	items, etc, prices, err := LoadItems(filepath.Join(xmlDir, "items"))
	if err != nil {
		return nil, err
	}
	skills, err := LoadSkills(filepath.Join(xmlDir, "skills"))
	if err != nil {
		return nil, err
	}
	recipes, err := LoadRecipes(filepath.Join(xmlDir, "recipes.xml"))
	if err != nil {
		return nil, err
	}
	armorSets, err := LoadArmorSets(filepath.Join(xmlDir, "armorSets.xml"))
	if err != nil {
		return nil, err
	}
	hennas, err := LoadHennas(filepath.Join(xmlDir, "hennas.xml"))
	if err != nil {
		return nil, err
	}
	water, err := LoadWaterZones(filepath.Join(xmlDir, "zones", "WaterZone.xml"))
	if err != nil {
		return nil, err
	}
	return &Datapack{
		NPCs: npcs, Spawnlist: spawns, Teleports: teleports, BuyLists: buyLists,
		Items: items, EtcItems: etc, Prices: prices, Skills: skills, Recipes: recipes,
		ArmorSets: armorSets, Hennas: hennas, WaterZones: water,
	}, nil
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
