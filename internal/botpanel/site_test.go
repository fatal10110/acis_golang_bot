package botpanel

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// pages fetches the page root and every file of the embedded site: the
// built application's assets, or the placeholder.
func (g *rig) pages() []string {
	g.t.Helper()
	web, err := site(webFS)
	if err != nil {
		g.t.Fatal(err)
	}
	out := []string{g.want(http.StatusOK, "GET", "/", nil)}
	err = fs.WalkDir(web, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		out = append(out, g.want(http.StatusOK, "GET", "/"+path, nil))
		return nil
	})
	if err != nil {
		g.t.Fatal(err)
	}
	return out
}

func TestPanelSite(t *testing.T) {
	placeholder := &fstest.MapFile{Data: []byte("run make ui")}
	tests := []struct {
		name string
		root fstest.MapFS
		want string
	}{
		{name: "not built", root: fstest.MapFS{"web/placeholder/index.html": placeholder}, want: "run make ui"},
		{name: "built", root: fstest.MapFS{
			"web/placeholder/index.html": placeholder,
			"web/app/index.html":         {Data: []byte("the app")},
			"web/app/assets/index.js":    {Data: []byte("js")},
		}, want: "the app"},
		{name: "built dir without its page", root: fstest.MapFS{
			"web/placeholder/index.html": placeholder,
			"web/app/assets/index.js":    {Data: []byte("js")},
		}, want: "run make ui"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			web, err := site(tt.root)
			if err != nil {
				t.Fatal(err)
			}
			b, err := fs.ReadFile(web, "index.html")
			if err != nil || string(b) != tt.want {
				t.Fatalf("index.html = %q, %v; want %q", b, err, tt.want)
			}
		})
	}
}

// TestPanelPlaceholderShipped checks the placeholder is in the binary: a
// build without `make ui` still serves a page saying how to build it.
func TestPanelPlaceholderShipped(t *testing.T) {
	b, err := fs.ReadFile(webFS, "web/placeholder/index.html")
	if err != nil || !strings.Contains(string(b), "make ui") {
		t.Fatalf("placeholder = %q, %v", b, err)
	}
	if strings.Contains(string(b), "<script") || strings.Contains(string(b), "style=") {
		t.Fatal("the placeholder carries inline script or style the CSP refuses")
	}
}

func TestPanelSecurityHeaders(t *testing.T) {
	g := newRig(t, Config{})
	defer g.stop()
	tests := []struct {
		name, host, path string
		code             int
	}{
		{name: "page", path: "/", code: http.StatusOK},
		{name: "api", path: "/api/bots", code: http.StatusOK},
		{name: "missing file", path: "/nope.js", code: http.StatusNotFound},
		{name: "refused host", host: "evil.example", path: "/", code: http.StatusMisdirectedRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), "GET", g.srv.URL+tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tt.host != "" {
				req.Host = tt.host
			}
			res, err := g.srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			if res.StatusCode != tt.code {
				t.Fatalf("status %d, want %d", res.StatusCode, tt.code)
			}
			csp := res.Header.Get("Content-Security-Policy")
			for _, want := range []string{"default-src 'none'", "script-src 'self'", "frame-ancestors 'none'", "base-uri 'none'"} {
				if !strings.Contains(csp, want) {
					t.Fatalf("CSP %q lacks %q", csp, want)
				}
			}
			for _, bad := range []string{"unsafe-inline", "unsafe-eval", "*"} {
				if strings.Contains(csp, bad) {
					t.Fatalf("CSP %q allows %q", csp, bad)
				}
			}
			if res.Header.Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("no nosniff")
			}
		})
	}
}

func TestPanelClasses(t *testing.T) {
	g := newRig(t, Config{})
	defer g.stop()
	var got []classView
	if err := json.Unmarshal([]byte(g.want(http.StatusOK, "GET", "/api/classes", nil)), &got); err != nil {
		t.Fatal(err)
	}
	if !slices.IsSortedFunc(got, func(a, b classView) int { return int(a.ID - b.ID) }) || len(got) != len(classNames) {
		t.Fatalf("classes %d, unsorted or short", len(got))
	}
	var starting []int32
	for _, c := range got {
		if c.Starting {
			starting = append(starting, c.ID)
		}
	}
	var want []int32
	for _, c := range botclient.StartingClasses() {
		want = append(want, c.ID)
	}
	slices.Sort(want)
	if !slices.Equal(starting, want) {
		t.Fatalf("starting classes %v, want %v", starting, want)
	}
	for _, c := range []classView{{ID: 0, Name: "Human Fighter", Starting: true}, {ID: 57, Name: "Warsmith"}, {ID: 118, Name: "Maestro"}} {
		if !slices.Contains(got, c) {
			t.Fatalf("classes lack %+v", c)
		}
	}
}

func TestPanelSpots(t *testing.T) {
	tests := []struct {
		name  string
		spots *botclient.SpotRegistry
		want  []spotView
	}{
		{name: "no datapack", want: []spotView{}},
		{
			name: "datapack",
			spots: botclient.NewSpotRegistry([]datapack.Spot{
				{Maker: "Wolves", MinLevel: 3, MaxLevel: 5, AggroLevel: 4, MaximumNPCs: 9},
				{Maker: "Orcs", MinLevel: 10, MaxLevel: 12, MaximumNPCs: 1},
			}, 1),
			want: []spotView{
				{Name: "Wolves", MinLevel: 3, MaxLevel: 5, AggroLevel: 4, Cap: 3},
				{Name: "Orcs", MinLevel: 10, MaxLevel: 12, Cap: 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newRig(t, Config{Spots: tt.spots})
			defer g.stop()
			var got []spotView
			if err := json.Unmarshal([]byte(g.want(http.StatusOK, "GET", "/api/spots", nil)), &got); err != nil {
				t.Fatal(err)
			}
			if got == nil || !slices.Equal(got, tt.want) {
				t.Fatalf("spots %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestPanelDisabledPreset checks a preset listed as disabled is shown with
// its reason and refused, and that nothing the new endpoints answer carries
// a secret.
func TestPanelDisabledPreset(t *testing.T) {
	g := newRig(t, Config{Presets: []Preset{{Name: "support", Help: "farms"}, {Name: "town", Help: "idles", Disabled: "needs -datapack"}}})
	var p struct {
		Presets []Preset `json:"presets"`
	}
	if err := json.Unmarshal([]byte(g.want(http.StatusOK, "GET", "/api/presets", nil)), &p); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(p.Presets, Preset{Name: "town", Help: "idles", Disabled: "needs -datapack"}) {
		t.Fatalf("presets %+v lack the disabled town", p.Presets)
	}
	if b := g.want(http.StatusBadRequest, "POST", "/api/bots", map[string]any{"preset": "town"}); !strings.Contains(b, "needs -datapack") {
		t.Fatalf("disabled preset refusal %s lacks its reason", b)
	}
	g.want(http.StatusCreated, "POST", "/api/bots", map[string]any{"preset": "support"})
	r := g.next()
	defer g.stop(r)
	if b := g.want(http.StatusBadRequest, "POST", "/api/bots/"+r.Account+"/preset", map[string]any{"preset": "town"}); !strings.Contains(b, "needs -datapack") {
		t.Fatalf("disabled preset change refusal %s lacks its reason", b)
	}
	g.want(http.StatusOK, "GET", "/api/classes", nil)
	g.want(http.StatusOK, "GET", "/api/spots", nil)
	g.noSecret()
}
