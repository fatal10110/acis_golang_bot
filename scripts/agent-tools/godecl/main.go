// Command godecl prints Go declarations by name, with their doc comments.
//
//	godecl <file.go|package dir> <Name|Type.Method>...
//
// A name matches a func, a method (Type.Method, or Method alone), a type, or a
// package-level var or const. Each match prints as "== path:line" followed by
// the declaration's source. Test files are searched only when a _test.go file
// is given or -tests is set.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	tests := flag.Bool("tests", false, "also search _test.go files of a directory")
	maxLines := flag.Int("max", 200, "cap per declaration (lines)")
	flag.Parse()
	if flag.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: godecl [-tests] [-max N] <file|dir> <Name|Type.Method>...")
		os.Exit(2)
	}
	files, err := goFiles(flag.Arg(0), *tests)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fset := token.NewFileSet()
	missing := 0
	for _, name := range flag.Args()[1:] {
		found := 0
		for _, path := range files {
			src, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
			if err != nil {
				continue
			}
			for _, n := range matches(f, name) {
				found++
				print(fset, path, src, n, *maxLines)
			}
		}
		if found == 0 {
			missing++
			fmt.Printf("== %s: no declaration in %s\n", name, flag.Arg(0))
		}
	}
	if missing > 0 {
		os.Exit(1)
	}
}

func goFiles(arg string, tests bool) ([]string, error) {
	st, err := os.Stat(arg)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return []string{arg}, nil
	}
	ents, err := os.ReadDir(arg)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || (!tests && strings.HasSuffix(n, "_test.go")) {
			continue
		}
		out = append(out, filepath.Join(arg, n))
	}
	sort.Strings(out)
	return out, nil
}

// matches returns the declaration nodes (with their doc comment) that name selects.
func matches(f *ast.File, name string) []ast.Node {
	recv, member, isMethod := strings.Cut(name, ".")
	if !isMethod {
		member = recv
		recv = ""
	}
	var out []ast.Node
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Name.Name != member {
				continue
			}
			r := recvName(d)
			if (recv == "" && (r == "" || !isMethod)) || r == recv {
				out = append(out, d)
			}
		case *ast.GenDecl:
			if isMethod {
				continue
			}
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					if s.Name.Name == member {
						out = append(out, genNode(d, s))
					}
				case *ast.ValueSpec:
					for _, id := range s.Names {
						if id.Name == member {
							out = append(out, genNode(d, s))
						}
					}
				}
			}
		}
	}
	return out
}

func recvName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return ""
	}
	t := d.Recv.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	switch t := t.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.IndexListExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

// genNode prints a single spec of a grouped declaration on its own, or the whole
// declaration when it is not grouped.
func genNode(d *ast.GenDecl, s ast.Spec) ast.Node {
	if d.Lparen == token.NoPos {
		return d
	}
	return s
}

func print(fset *token.FileSet, path string, src []byte, n ast.Node, maxLines int) {
	start := n.Pos()
	switch d := n.(type) {
	case *ast.FuncDecl:
		if d.Doc != nil {
			start = d.Doc.Pos()
		}
	case *ast.GenDecl:
		if d.Doc != nil {
			start = d.Doc.Pos()
		}
	case *ast.TypeSpec:
		if d.Doc != nil {
			start = d.Doc.Pos()
		}
	case *ast.ValueSpec:
		if d.Doc != nil {
			start = d.Doc.Pos()
		}
	}
	p := fset.Position(start)
	e := fset.Position(n.End())
	lines := strings.Split(string(src[p.Offset:e.Offset]), "\n")
	fmt.Printf("== %s:%d-%d\n", path, p.Line, e.Line)
	for i, l := range lines {
		if i == maxLines {
			fmt.Printf("... (%d more lines; read %s:%d-%d)\n", len(lines)-maxLines, path, p.Line+maxLines, e.Line)
			break
		}
		fmt.Printf("%d\t%s\n", p.Line+i, l)
	}
}
