package datapack

import (
	"fmt"
	"strconv"
	"strings"
)

// Henna is one dye symbol (hennas.xml <henna>): the dye a draw takes, the
// draw price and the classes that may wear it. The stat changes are not
// read: the bot's persona table picks the symbols.
type Henna struct {
	SymbolID, DyeID int32
	// Price is the draw price in adena; a removal costs a fifth of it
	// (model/henna/henna.go RemovePrice).
	Price int32
	// Classes are the class ids that may draw and use the symbol.
	Classes []int32
}

type hennasFile struct {
	Hennas []attrsElement `xml:"henna"`
}

// LoadHennas reads hennas.xml at path: every symbol by its id. symbolId,
// dyeId and classes are required, price defaults to 0, and classes is a
// ";"-separated list of base-10 class ids with no empty part, as on the
// server (data/xml/economy.go LoadHennas, files.go parseInts). A later
// symbol of the same id replaces the earlier one, as in the server's table
// (model/henna NewTable).
func LoadHennas(path string) (map[int32]Henna, error) {
	var f hennasFile
	if err := decodeFile(path, &f); err != nil {
		return nil, err
	}
	out := make(map[int32]Henna, len(f.Hennas))
	for _, e := range f.Hennas {
		h, err := buildHenna(e)
		if err != nil {
			return nil, fmt.Errorf("datapack: %s: %w", path, err)
		}
		out[h.SymbolID] = h
	}
	return out, nil
}

// buildHenna builds one <henna> from its attributes.
func buildHenna(e attrsElement) (Henna, error) {
	a := foldAttrs(e.Attrs)
	h := Henna{
		SymbolID: a.int32("symbolId"),
		DyeID:    a.int32("dyeId"),
		Price:    a.int32Default("price", 0),
	}
	if raw := a.str("classes"); a.err == nil {
		for part := range strings.SplitSeq(raw, ";") {
			n, err := strconv.ParseInt(part, 10, 32)
			if err != nil {
				a.fail("classes", err)
				break
			}
			h.Classes = append(h.Classes, int32(n))
		}
	}
	if a.err != nil {
		return Henna{}, fmt.Errorf("henna %d: %w", h.SymbolID, a.err)
	}
	return h, nil
}
