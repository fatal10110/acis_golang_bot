package datapack

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// Multisell is one multisell list (multisell/<name>.xml): the NPCs that
// open it and its exchange rows.
type Multisell struct {
	// Name is the file name without ".xml", the list's identity on the
	// server (which keys the list by a hash of it).
	Name string
	// NPCs are the template ids of <npcs><npc>, in file order; nil for none.
	NPCs []int32
	// Rows are its <item> rows in file order.
	Rows []MultisellRow
}

// MultisellRow is one <item> of a multisell list: what the player gives and
// what the player gets.
type MultisellRow struct {
	Ingredients []MultisellItem
	Products    []MultisellItem
}

// MultisellItem is one <ingredient> or <production>: an item template and
// its count.
type MultisellItem struct {
	ItemID int32
	Count  int
}

type multisellFile struct {
	NPCs []npcIDText       `xml:"npcs>npc"`
	Rows []multisellRowElt `xml:"item"`
}

type multisellRowElt struct {
	Ingredients []attrsElement `xml:"ingredient"`
	Products    []attrsElement `xml:"production"`
}

// npcIDText is an <npc> element whose text is a plain base-10 int32
// template id. Empty, padded and non-numeric text fails the load, as on the
// server (data/xml/multisell.go npcIDText).
type npcIDText int32

func (n *npcIDText) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var text string
	if err := d.DecodeElement(&text, &start); err != nil {
		return err
	}
	v, err := strconv.ParseInt(text, 10, 32)
	if err != nil {
		return fmt.Errorf("npc: %w", err)
	}
	*n = npcIDText(v)
	return nil
}

// LoadMultisells reads every *.xml multisell file directly under dir, in
// sorted file order, the way the server's LoadMultiSellLists does
// (data/xml/multisell.go): an <ingredient> or <production> needs its id and
// count, both base-10. A file that is not well-formed, or a row missing or
// mangling one, fails the whole load.
func LoadMultisells(dir string) ([]Multisell, error) {
	paths, err := xmlFiles(dir, "multisell")
	if err != nil {
		return nil, err
	}
	out := make([]Multisell, 0, len(paths))
	for _, path := range paths {
		var f multisellFile
		if err := decodeFile(path, &f); err != nil {
			return nil, err
		}
		m := Multisell{Name: strings.TrimSuffix(filepath.Base(path), ".xml"), Rows: make([]MultisellRow, 0, len(f.Rows))}
		for _, id := range f.NPCs {
			m.NPCs = append(m.NPCs, int32(id))
		}
		for i, el := range f.Rows {
			ingredients, err := buildMultisellItems(el.Ingredients)
			if err != nil {
				return nil, fmt.Errorf("datapack: %s: item %d ingredient: %w", path, i+1, err)
			}
			products, err := buildMultisellItems(el.Products)
			if err != nil {
				return nil, fmt.Errorf("datapack: %s: item %d production: %w", path, i+1, err)
			}
			m.Rows = append(m.Rows, MultisellRow{Ingredients: ingredients, Products: products})
		}
		out = append(out, m)
	}
	return out, nil
}

func buildMultisellItems(els []attrsElement) ([]MultisellItem, error) {
	out := make([]MultisellItem, 0, len(els))
	for _, el := range els {
		a := foldAttrs(el.Attrs)
		it := MultisellItem{ItemID: a.int32("id"), Count: a.int("count")}
		if a.err != nil {
			return nil, a.err
		}
		out = append(out, it)
	}
	return out, nil
}
