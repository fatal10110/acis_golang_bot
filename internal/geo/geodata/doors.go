package geodata

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang_bot/internal/commons"
	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

type doorFile struct {
	Doors []doorElement `xml:"door"`
}

type doorElement struct {
	Attrs       []xml.Attr     `xml:",any,attr"`
	Position    attrsElement   `xml:"position"`
	Coordinates []pointElement `xml:"coordinates>loc"`
	Stats       attrsElement   `xml:"stats"`
	Function    attrsElement   `xml:"function"`
}

type attrsElement struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

// pointElement is a <loc> footprint corner. A missing coordinate is an
// error, never the world origin.
type pointElement struct {
	X *literal32 `xml:"x,attr"`
	Y *literal32 `xml:"y,attr"`
}

func (e pointElement) point() (location.Point, error) {
	if e.X == nil || e.Y == nil {
		return location.Point{}, fmt.Errorf("x and y are required")
	}
	return location.Point{X: int(*e.X), Y: int(*e.Y)}, nil
}

// literal32 is an int32 attribute written as a Java integer literal
// (decimal, 0x/# hex or 0-prefixed octal).
type literal32 int32

func (l *literal32) UnmarshalXMLAttr(attr xml.Attr) error {
	n, err := commons.DecodeInt32(attr.Value)
	if err != nil {
		return fmt.Errorf("%s: %w", attr.Name.Local, err)
	}
	*l = literal32(n)
	return nil
}

// LoadDoors parses doors.xml at path into the static door templates, as the
// server's loader does: a door with any footprint corner out of the world
// is logged and skipped, any other malformed door fails the load.
func LoadDoors(path string, log *slog.Logger) (*door.Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("doors: read %s: %w", path, err)
	}
	var doc doorFile
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("doors: parse %s: %w", path, err)
	}

	templates := make([]*door.Template, 0, len(doc.Doors))
doors:
	for _, el := range doc.Doors {
		tmpl, err := buildDoorTemplate(el)
		if err != nil {
			return nil, fmt.Errorf("doors: %s: %w", path, err)
		}
		for _, point := range tmpl.Coordinates {
			if engine.OutOfWorld(point.X, point.Y) {
				log.Error("geodata: skipping out-of-world door", "door_id", tmpl.ID, "file", path)
				continue doors
			}
		}
		templates = append(templates, tmpl)
	}
	table, err := door.NewTable(templates)
	if err != nil {
		return nil, fmt.Errorf("doors: %s: %w", path, err)
	}
	return table, nil
}

// buildDoorTemplate folds a <door> element's own, <position>, <stats> and
// <function> attributes, in that order with later ones winning, and reads
// the template from them. The position coordinates are integer literals;
// the function attributes default to a closed, NPC-opened door with no
// residence link or timers.
func buildDoorTemplate(el doorElement) (*door.Template, error) {
	vals := foldAttrs(el.Attrs)
	for k, v := range foldAttrs(el.Position.Attrs) {
		vals[k] = v
	}
	for _, key := range []string{"x", "y", "z"} {
		raw, ok := vals[key]
		if !ok {
			continue
		}
		n, err := commons.DecodeInt32(raw)
		if err != nil {
			return nil, fmt.Errorf("door position: %s: %w", key, err)
		}
		vals[key] = strconv.Itoa(int(n))
	}
	for _, attrs := range [][]xml.Attr{el.Stats.Attrs, el.Function.Attrs} {
		for k, v := range foldAttrs(attrs) {
			vals[k] = v
		}
	}

	a := &attrValues{vals: vals, prefix: "door: template"}
	id := a.int("id")
	if a.err != nil {
		return nil, a.err
	}
	a.prefix = fmt.Sprintf("door: template %d", id)

	t := &door.Template{
		ID:       id,
		Name:     a.strDefault("name", ""),
		Kind:     attrEnum(a, "type", door.ParseKind),
		Level:    a.int("level"),
		Position: location.Location{X: a.int("x"), Y: a.int("y"), Z: a.int("z")},

		HP:     a.int("hp"),
		PDef:   a.int("pDef"),
		MDef:   a.int("mDef"),
		Height: a.int("height"),

		CastleID:    a.intDefault("castle", 0),
		ClanHallID:  a.intDefault("clanHall", 0),
		TriggeredID: a.intDefault("triggeredId", 0),
		Opened:      a.boolDefault("opened", false),
		OpenKind:    attrEnumDefault(a, "openType", door.ParseOpenKind, door.OpenNPC),
		OpenTime:    a.intDefault("openTime", 0),
		RandomTime:  a.intDefault("randomTime", 0),
		CloseTime:   a.intDefault("closeTime", 0),
	}
	if t.Name == "" {
		a.fail(fmt.Errorf("name is required"))
	}
	for _, c := range el.Coordinates {
		point, err := c.point()
		if err != nil {
			return nil, err
		}
		t.Coordinates = append(t.Coordinates, point)
	}
	if len(t.Coordinates) < 3 {
		a.fail(fmt.Errorf("coords requires at least 3 points"))
	}
	if a.err != nil {
		return nil, a.err
	}
	return t, nil
}

// foldAttrs folds an attribute list into a name-keyed value map, last value
// winning.
func foldAttrs(attrs []xml.Attr) map[string]string {
	vals := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		vals[attr.Name.Local] = attr.Value
	}
	return vals
}

// attrValues reads typed attributes with a sticky first error: an absent
// optional attribute takes its default, a present one that fails to parse
// is an error.
type attrValues struct {
	vals   map[string]string
	prefix string
	err    error
}

func (a *attrValues) fail(err error) {
	if a.err == nil {
		a.err = fmt.Errorf("%s: %w", a.prefix, err)
	}
}

func (a *attrValues) strDefault(key, def string) string {
	if a.err != nil {
		return def
	}
	if v, ok := a.vals[key]; ok {
		return v
	}
	return def
}

func (a *attrValues) int(key string) int {
	if a.err != nil {
		return 0
	}
	raw, ok := a.vals[key]
	if !ok {
		a.fail(fmt.Errorf("attribute %q is required", key))
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		a.fail(fmt.Errorf("attribute %q: %w", key, err))
		return 0
	}
	return n
}

func (a *attrValues) intDefault(key string, def int) int {
	if a.err != nil {
		return def
	}
	if _, ok := a.vals[key]; !ok {
		return def
	}
	return a.int(key)
}

// boolDefault reads a case-insensitive "true" as true and any other present
// value as false.
func (a *attrValues) boolDefault(key string, def bool) bool {
	if a.err != nil {
		return def
	}
	raw, ok := a.vals[key]
	if !ok {
		return def
	}
	return strings.EqualFold(raw, "true")
}

func attrEnum[E any](a *attrValues, key string, parse func(string) (E, error)) E {
	var zero E
	if a.err != nil {
		return zero
	}
	raw, ok := a.vals[key]
	if !ok {
		a.fail(fmt.Errorf("attribute %q is required", key))
		return zero
	}
	e, err := parse(raw)
	if err != nil {
		a.fail(err)
		return zero
	}
	return e
}

func attrEnumDefault[E any](a *attrValues, key string, parse func(string) (E, error), def E) E {
	if a.err != nil {
		return def
	}
	if _, ok := a.vals[key]; !ok {
		return def
	}
	return attrEnum(a, key, parse)
}
