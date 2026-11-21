// Package door holds the static door templates read from doors.xml: the part
// of the server's door model the geodata door overlay needs.
package door

import (
	"fmt"

	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// Kind classifies a door template as a regular door or a wall.
type Kind uint8

const (
	KindDoor Kind = iota
	KindWall
)

// ParseKind maps a door's XML type spelling onto its Kind.
func ParseKind(s string) (Kind, error) {
	for k, name := range kindStrings {
		if name == s {
			return Kind(k), nil
		}
	}
	return 0, fmt.Errorf("door: unknown type %q", s)
}

var kindStrings = [...]string{"DOOR", "WALL"}

// String returns k's canonical XML spelling.
func (k Kind) String() string {
	if int(k) < len(kindStrings) {
		return kindStrings[k]
	}
	return fmt.Sprintf("Kind(%d)", uint8(k))
}

// OpenKind classifies what opens a door.
type OpenKind uint8

const (
	OpenClick OpenKind = iota
	OpenTime
	OpenSkill
	OpenNPC
)

// ParseOpenKind maps a door's XML openType spelling onto its OpenKind.
func ParseOpenKind(s string) (OpenKind, error) {
	for o, name := range openKindStrings {
		if name == s {
			return OpenKind(o), nil
		}
	}
	return 0, fmt.Errorf("door: unknown openType %q", s)
}

var openKindStrings = [...]string{"CLICK", "TIME", "SKILL", "NPC"}

// String returns o's canonical XML spelling.
func (o OpenKind) String() string {
	if int(o) < len(openKindStrings) {
		return openKindStrings[o]
	}
	return fmt.Sprintf("OpenKind(%d)", uint8(o))
}

// Template is one static door entry from doors.xml.
type Template struct {
	ID    int
	Name  string
	Kind  Kind
	Level int

	Position    location.Location
	Coordinates []location.Point

	HP, PDef, MDef, Height int

	CastleID, ClanHallID, TriggeredID int
	Opened                            bool
	OpenKind                          OpenKind
	OpenTime, RandomTime, CloseTime   int
}

// Table stores door templates keyed by door id.
type Table struct {
	doors map[int]*Template
	order []*Template
}

// NewTable builds a door table and rejects duplicate ids.
func NewTable(templates []*Template) (*Table, error) {
	t := &Table{
		doors: make(map[int]*Template, len(templates)),
		order: make([]*Template, 0, len(templates)),
	}
	for _, tmpl := range templates {
		if tmpl == nil {
			return nil, fmt.Errorf("door: nil template")
		}
		if _, exists := t.doors[tmpl.ID]; exists {
			return nil, fmt.Errorf("door: duplicate template id %d", tmpl.ID)
		}
		t.doors[tmpl.ID] = tmpl
		t.order = append(t.order, tmpl)
	}
	return t, nil
}

// Len returns the number of loaded door templates.
func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	return len(t.order)
}

// Get returns the template for id.
func (t *Table) Get(id int) (*Template, bool) {
	if t == nil {
		return nil, false
	}
	tmpl, ok := t.doors[id]
	return tmpl, ok
}

// All returns the loaded templates in file order.
func (t *Table) All() []*Template {
	if t == nil {
		return nil
	}
	return append([]*Template(nil), t.order...)
}
