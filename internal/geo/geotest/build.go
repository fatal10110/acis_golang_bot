package geotest

// Raise sets the ground of the region-local cells x0..x1, y0..y1 to height.
// A later patch over the same cell wins. It returns t for chaining, so a
// test can build its own small shape next to the fixture scenes.
func (t *Terrain) Raise(x0, y0, x1, y1 int, height int16) *Terrain {
	t.patches = append(t.patches, patch{rect{x0, y0, x1, y1}, height})
	return t
}

// AddDeck lays a second layer at height over the region-local cells x0..x1,
// y0..y1, above whatever ground lies there. It returns t for chaining.
func (t *Terrain) AddDeck(x0, y0, x1, y1 int, height int16) *Terrain {
	t.decks = append(t.decks, patch{rect{x0, y0, x1, y1}, height})
	return t
}
