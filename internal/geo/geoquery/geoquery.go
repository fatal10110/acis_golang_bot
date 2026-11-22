// Package geoquery builds the parity query file `bots geoquery` writes: a
// curated set of geodata questions answered by the bot's engine, in the
// dump format of the server's geoprobe tool, so `geoprobe -expected-dump`
// can check every answer against the server's own engine.
//
// The format is the server's datadiff dump: one line per record, sorted by
// id, "id<TAB>name=value..." with fields sorted by name. A record id is a
// geoprobe query id, "<category>:x,y,z" for height and
// "<category>:x,y,z->x,y,z" for the others (server
// internal/gameserver/geo/probe/query.go, internal/datadiff/dump.go). The
// code is a bot-owned copy of that format; nothing is imported.
package geoquery

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// Category names a kind of geodata question, as geoprobe names it. The
// bot has no server pathfinder, so geoprobe's "path" category is not used.
type Category string

const (
	Height        Category = "height"
	CanMove       Category = "canmove"
	LineOfSight   Category = "los"
	ValidLocation Category = "validlocation"
)

// Query is one geodata question: a single point for Height, or a from/to
// pair for the others.
type Query struct {
	Category Category
	From     location.Location
	To       location.Location // zero for Height
}

// ID renders q as geoprobe's query id, which geoprobe parses back into the
// same query.
func (q Query) ID() string {
	if q.Category == Height {
		return fmt.Sprintf("%s:%s", q.Category, formatPoint(q.From))
	}
	return fmt.Sprintf("%s:%s->%s", q.Category, formatPoint(q.From), formatPoint(q.To))
}

func formatPoint(l location.Location) string {
	return fmt.Sprintf("%d,%d,%d", l.X, l.Y, l.Z)
}

// Record is one answered query: its id and the answer fields geoprobe
// compares.
type Record struct {
	ID     string
	Fields map[string]string
}

// Evaluate answers q with e, under the field names geoprobe's Evaluate
// writes: "height" for Height, "result" for CanMove and LineOfSight, and
// "x", "y", "z" for ValidLocation.
func Evaluate(e *engine.Engine, q Query) Record {
	f := make(map[string]string, 3)
	switch q.Category {
	case Height:
		f["height"] = strconv.Itoa(int(e.Height(q.From.X, q.From.Y, q.From.Z)))
	case CanMove:
		f["result"] = strconv.FormatBool(e.CanMove(q.From.X, q.From.Y, q.From.Z, q.To.X, q.To.Y, q.To.Z))
	case LineOfSight:
		f["result"] = strconv.FormatBool(e.CanSee(q.From.X, q.From.Y, q.From.Z, q.To.X, q.To.Y, q.To.Z))
	case ValidLocation:
		v := e.ValidLocation(q.From.X, q.From.Y, q.From.Z, q.To.X, q.To.Y, q.To.Z)
		f["x"] = strconv.Itoa(v.X)
		f["y"] = strconv.Itoa(v.Y)
		f["z"] = strconv.Itoa(v.Z)
	}
	return Record{ID: q.ID(), Fields: f}
}

// WriteDump writes records to w in the dump format, sorted by id with
// fields sorted by name. An id, name or value holding a tab or newline, or
// a name holding '=', is an error, since the format has no escaping.
func WriteDump(w io.Writer, records []Record) error {
	sorted := slices.Clone(records)
	slices.SortFunc(sorted, func(a, b Record) int { return strings.Compare(a.ID, b.ID) })
	bw := bufio.NewWriter(w)
	for _, r := range sorted {
		if strings.ContainsAny(r.ID, "\t\n") {
			return fmt.Errorf("geoquery: record id %q contains a tab or newline", r.ID)
		}
		var line strings.Builder
		line.WriteString(r.ID)
		names := make([]string, 0, len(r.Fields))
		for name := range r.Fields {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			val := r.Fields[name]
			if strings.ContainsAny(name, "\t\n=") || strings.ContainsAny(val, "\t\n") {
				return fmt.Errorf("geoquery: record %q: field %q holds a reserved character", r.ID, name)
			}
			line.WriteString("\t" + name + "=" + val)
		}
		line.WriteString("\n")
		if _, err := bw.WriteString(line.String()); err != nil {
			return fmt.Errorf("geoquery: write dump: %w", err)
		}
	}
	return bw.Flush()
}
