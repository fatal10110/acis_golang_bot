package datapack

import (
	"encoding/xml"
	"fmt"
)

// BuyList is one merchant buylist (buyLists.xml <buyList>): the products
// NPC template NPCID sells under list id ID. The server sells a list only
// at its NPC (buylist.List.AllowsNPC); a list with NPCID 0 or less is a
// game master's.
type BuyList struct {
	ID, NPCID int32
	Products  []Product
}

// Product is one item a buylist sells (<product>).
type Product struct {
	ItemID int32
	// Price is the base unit price in adena, before the castle tax.
	Price int32
	// Count is the limited stock a restock refills, -1 for unlimited.
	Count int32
}

type buyListsFile struct {
	Lists []buyListElement `xml:"buyList"`
}

type buyListElement struct {
	Attrs    []xml.Attr     `xml:",any,attr"`
	Products []attrsElement `xml:"product"`
}

// LoadBuyLists reads buyLists.xml at path: every buylist in file order. A
// list missing id or npcId, or a product missing id, fails the load; price
// defaults to 0 and count to -1, as on the server
// (data/xml/economy.go LoadBuyLists).
func LoadBuyLists(path string) ([]BuyList, error) {
	var f buyListsFile
	if err := decodeFile(path, &f); err != nil {
		return nil, err
	}
	out := make([]BuyList, 0, len(f.Lists))
	for _, l := range f.Lists {
		a := foldAttrs(l.Attrs)
		list := BuyList{ID: a.literal("id"), NPCID: a.literal("npcId")}
		if a.err != nil {
			return nil, fmt.Errorf("datapack: %s: buyList: %w", path, a.err)
		}
		list.Products = make([]Product, 0, len(l.Products))
		for _, p := range l.Products {
			r := foldAttrs(p.Attrs)
			prod := Product{ItemID: r.int32("id"), Price: int32(r.intDefault("price", 0)), Count: int32(r.intDefault("count", -1))}
			if r.err != nil {
				return nil, fmt.Errorf("datapack: %s: buyList %d: product: %w", path, list.ID, r.err)
			}
			list.Products = append(list.Products, prod)
		}
		out = append(out, list)
	}
	return out, nil
}
