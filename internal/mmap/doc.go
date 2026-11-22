// Package mmap maps a file read-only, for the caches the bots read in place:
// the geodata cache and the nav cache. Nothing it maps is ever written, so a
// mapping is shared with the page cache.
package mmap
