package botwire

import (
	"os/exec"
	"strings"
	"testing"
)

// TestBoundary keeps the wire package free of the brain (docs/structure.md,
// Design 5): botwire imports nothing of botclient, whose import of botwire
// the compiler would refuse as a cycle anyway, nor the packages only the
// brain needs.
func TestBoundary(t *testing.T) {
	const mod = "github.com/fatal10110/acis_golang_bot/internal/"
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	deps := strings.Fields(string(out))
	if len(deps) == 0 {
		t.Fatal("go list -deps listed nothing")
	}
	banned := []string{"botclient", "nav", "charmodel", "datapack"}
	for _, dep := range deps {
		for _, b := range banned {
			if dep == mod+b || strings.HasPrefix(dep, mod+b+"/") {
				t.Errorf("botwire depends on %s", dep)
			}
		}
	}
}
