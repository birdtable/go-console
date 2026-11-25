package ui

// The shape of ui's surface that its tests hold: no word that blames the person,
// and no way to end a run on free text.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// uiFiles is every Go file of the package that is not a test, parsed.
func uiFiles(t *testing.T) (*token.FileSet, []*ast.File) {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	if len(files) < 10 {
		t.Fatalf("parsed %d files of ui, which is too few to be the package", len(files))
	}
	return fset, files
}

// No string ui prints, or could, says "invalid", in any case: it blames the
// person and says nothing of what to type (cli.errors.6).
//
// What it costs when this fails: a person reads "invalid" and is told what
// was wrong with them, not what to type next.
func TestNoStringOfUiSaysInvalid(t *testing.T) {
	t.Parallel()
	fset, files := uiFiles(t)
	strs := 0
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				strs++
				if strings.Contains(strings.ToLower(lit.Value), "invalid") {
					t.Errorf("%s: a string says invalid: %s", fset.Position(lit.Pos()), lit.Value)
				}
			}
			return true
		})
	}
	if strs < 100 {
		t.Fatalf("read %d strings, which is too few to be ui's", strs)
	}
}

// No exported function or method of ui ends a run on free text: one that
// prints a format returns nothing, and one that returns an int and takes a
// string takes an Ending too, so the only way to an exit code is an Ending,
// with its word and its line. Die(format, a...) int was that
// shape.
//
// What it costs when this fails: a program ends a run on a bare sentence,
// with no closing word and nothing to type next.
func TestNoExportOfUiEndsARunOnFreeText(t *testing.T) {
	t.Parallel()
	fset, files := uiFiles(t)
	funcs := 0
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() {
				continue
			}
			funcs++
			if problem := freeTextEnding(fn.Type); problem != "" {
				t.Errorf("%s: %s %s", fset.Position(fn.Pos()), fn.Name.Name, problem)
			}
		}
	}
	if funcs < 50 {
		t.Fatalf("read %d exported functions, which is too few to be ui's", funcs)
	}
}

// freeTextEnding is what is wrong with a function of type ft as a way to end
// a run, or empty.
func freeTextEnding(ft *ast.FuncType) string {
	var variadic, str, ending, runs, returnsInt bool
	for _, p := range ft.Params.List {
		switch typ := p.Type.(type) {
		case *ast.Ellipsis:
			id, ok := typ.Elt.(*ast.Ident)
			variadic = variadic || ok && id.Name == "any"
		case *ast.FuncType:
			runs = true // a program's function: the int is its code, not a sentence
		case *ast.Ident:
			str = str || typ.Name == "string"
			ending = ending || typ.Name == "Ending"
		}
	}
	results := 0
	if ft.Results != nil {
		for _, r := range ft.Results.List {
			results++
			if id, ok := r.Type.(*ast.Ident); ok && id.Name == "int" {
				returnsInt = true
			}
		}
	}
	switch {
	case variadic && results > 0:
		return "prints a format and returns a result"
	case returnsInt && str && !ending && !runs:
		return "returns an int from a string with no Ending"
	}
	return ""
}

// The negative half: the check finds the shapes it refuses, Die's among
// them, and passes End's, Log's and Width's.
//
// What it costs when this fails: the test above passes whatever ui exports.
func TestTheFreeTextCheckFindsDiesShape(t *testing.T) {
	t.Parallel()
	n := 0
	for _, c := range []struct {
		src  string
		want bool
	}{
		{"func(format string, a ...any) int", true},
		{"func(why string) int", true},
		{"func(format string, a ...any) error", true},
		{"func(e Ending) int", false},
		{"func(name string, run func(context.Context, []string) int) int", false},
		{"func(format string, a ...any)", false},
		{"func() int", false},
		{"func(opts ...SessionOption) *Session", false},
	} {
		expr, err := parser.ParseExpr(c.src)
		if err != nil {
			t.Fatal(err)
		}
		lit, ok := expr.(*ast.FuncType)
		if !ok {
			t.Fatalf("%s did not parse as a function type", c.src)
		}
		if got := freeTextEnding(lit) != ""; got != c.want {
			t.Errorf("%s: refused %v, want %v", c.src, got, c.want)
		}
		n++
	}
	if n != 8 {
		t.Fatalf("checked %d shapes, want 8", n)
	}
}
