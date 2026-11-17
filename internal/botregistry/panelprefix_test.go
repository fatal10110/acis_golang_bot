package botregistry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

func TestPanelPrefix(t *testing.T) {
	dir := t.TempDir()
	if p, err := PanelPrefix(dir); p != "" || err != nil {
		t.Fatalf("PanelPrefix of an empty dir = %q, %v; want none", p, err)
	}
	reg, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()
	if err := reg.SetPanel("e2ep", botnames.DefaultMarker); err != nil {
		t.Fatal(err)
	}
	if p, err := PanelPrefix(dir); p != "e2ep" || err != nil {
		t.Fatalf("PanelPrefix of a locked registry = %q, %v; want e2ep", p, err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PanelPrefix(dir); err == nil {
		t.Fatal("PanelPrefix of a malformed registry: no error")
	}
}
