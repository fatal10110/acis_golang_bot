// Package datapack reads the parts of the server's XML datapack the bots plan
// with: NPC templates (npcs/*.xml) and spawn territories and makers
// (spawnlist/*.xml). Parse rules follow the server's loaders
// (acis_golang internal/gameserver/data/xml npc.go and spawnlist.go), reduced
// to the fields bots use.
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
}

// Load reads the datapack whose XML directory is xmlDir (the server's
// aCis_datapack/data/xml): every npcs/*.xml and spawnlist/*.xml file directly
// under it. xmlDir is configuration; the bot ships no datapack of its own.
func Load(xmlDir string) (*Datapack, error) {
	npcs, err := LoadNPCs(filepath.Join(xmlDir, "npcs"))
	if err != nil {
		return nil, err
	}
	spawns, err := LoadSpawnlist(filepath.Join(xmlDir, "spawnlist"))
	if err != nil {
		return nil, err
	}
	return &Datapack{NPCs: npcs, Spawnlist: spawns}, nil
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
