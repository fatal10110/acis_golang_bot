package botpanel

import (
	"fmt"
	"math"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Catalog is what the inspect view names items, effects and classes with,
// taken from the datapack (NewCatalog): the bots' world models hold ids
// only. A nil Catalog names every item "item #id" and every skill
// "skill #id", and knows no weight, grade or base speed.
type Catalog struct {
	items  map[int32]catalogItem
	skills map[int32]string
	// runSpeeds are the classes' base run speeds by class id.
	runSpeeds map[int32]int32
}

// catalogItem is an item template as the inspect view shows it: weapon,
// fullArmor and arrow are what the server's grade penalty counts it as
// (character_grade_penalty.go refreshExpertisePenalty).
type catalogItem struct {
	name   string
	weight int32
	// grade is the crystal grade, 0 for none, 1 for D up to 5 for S.
	grade                    int
	weapon, fullArmor, arrow bool
}

// NewCatalog takes the names, weights, grades and base speeds the inspect
// view needs from dp. The datapack reads every weapon, armor and arrow
// template but only the etc items bots use: the others stay unnamed.
func NewCatalog(dp *datapack.Datapack) *Catalog {
	c := &Catalog{
		items:     make(map[int32]catalogItem, len(dp.Items)+len(dp.EtcItems)),
		skills:    make(map[int32]string),
		runSpeeds: make(map[int32]int32, len(dp.Classes)),
	}
	for id, it := range dp.Items {
		c.items[id] = catalogItem{
			name: it.Name, weight: it.Weight, grade: it.Grade,
			weapon: it.Type == "Weapon", fullArmor: it.BodyPart == "fullarmor", arrow: it.Type == "EtcItem",
		}
	}
	for id, it := range dp.EtcItems {
		if _, ok := c.items[id]; !ok {
			c.items[id] = catalogItem{name: it.Name, weight: it.Weight, grade: it.Grade}
		}
	}
	// A skill's name is its lowest level's.
	low := make(map[int32]int32)
	for ref, sk := range dp.Skills {
		if l, ok := low[ref.ID]; !ok || ref.Level < l {
			low[ref.ID], c.skills[ref.ID] = ref.Level, sk.Name
		}
	}
	for id, cl := range dp.Classes {
		c.runSpeeds[id] = int32(math.Round(cl.RunSpeed))
	}
	return c
}

// item is the template id, ok false when the catalog does not know it.
func (c *Catalog) item(id int32) (catalogItem, bool) {
	if c == nil {
		return catalogItem{}, false
	}
	it, ok := c.items[id]
	return it, ok
}

// itemName is the template's name, "item #id" when unknown.
func (c *Catalog) itemName(id int32) string {
	if it, ok := c.item(id); ok && it.name != "" {
		return it.name
	}
	return fmt.Sprintf("item #%d", id)
}

// skillName is the skill's name, "skill #id" when unknown.
func (c *Catalog) skillName(id int32) string {
	if c != nil {
		if n := c.skills[id]; n != "" {
			return n
		}
	}
	return fmt.Sprintf("skill #%d", id)
}

// runSpeed is the class's base run speed, 0 when unknown.
func (c *Catalog) runSpeed(class int32) int32 {
	if c == nil {
		return 0
	}
	return c.runSpeeds[class]
}
