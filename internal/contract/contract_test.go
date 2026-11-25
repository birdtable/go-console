// Package contract holds api/contract.txt to the tests: it has no code of its own, and its tests are the gate of the
// promise index (`api/contract.txt`).
package contract

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func allSystems() []string { return []string{"darwin", "linux", "windows"} }

// matches says whether the file name in dir is built for goos, by its suffix and its //go:build line.
func matches(fsys fs.FS, dir, name, goos string) bool {
	ctx := build.Context{
		GOOS: goos, GOARCH: "amd64", CgoEnabled: true, Compiler: "gc", ReleaseTags: build.Default.ReleaseTags,
		JoinPath: path.Join,
		OpenFile: func(p string) (io.ReadCloser, error) { return fsys.Open(p) },
	}
	ok, err := ctx.MatchFile(dir, name)
	return err == nil && ok
}

// declaring is the _test.go files of dir that declare the test function name.
func declaring(fsys fs.FS, dir, name string) []string {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), e.Name(), src, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == name {
				files = append(files, e.Name())
			}
		}
	}
	return files
}

// problems is each fault of a contract file read against the module tree fsys: a line not of four fields, a file not
// sorted or with a repeated line, a test that is not there, and a test only in files that a system the line names does
// not build.
func problems(fsys fs.FS, contract string) []string {
	var out []string
	if contract != "" && !strings.HasSuffix(contract, "\n") {
		out = append(out, "the file does not end in a newline")
	}
	lines := strings.Split(strings.TrimSuffix(contract, "\n"), "\n")
	if contract == "" {
		lines = nil
	}
	for i, l := range lines {
		if i > 0 && l < lines[i-1] {
			out = append(out, "not sorted: "+l)
		}
		if i > 0 && l == lines[i-1] {
			out = append(out, "repeated: "+l)
		}
		f := strings.Split(l, "\t")
		if len(f) != 4 || slices.Contains(f, "") {
			out = append(out, "not four tab-separated fields: "+l)
			continue
		}
		pkg, test, sys := f[0], f[1], allSystems()
		if f[2] != "all" {
			sys = strings.Split(f[2], ",")
		}
		files := declaring(fsys, pkg, test)
		if len(files) == 0 {
			out = append(out, "no test "+test+" in "+pkg)
			continue
		}
		for _, s := range sys {
			if !slices.Contains(allSystems(), s) {
				out = append(out, "unknown system "+s+": "+l)
			} else if !slices.ContainsFunc(files, func(n string) bool { return matches(fsys, pkg, n, s) }) {
				out = append(out, "the test "+test+" is only in files "+s+" does not build: "+strings.Join(files, ", "))
			}
		}
	}
	return out
}

// What it costs when this fails: a promise the index makes to a product has no test behind it, or the test is built
// on fewer systems than the line says, and the product leans on a promise nothing holds.
func TestEveryLineOfTheContractIsHeldByATestOnItsSystems(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("../../api/contract.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems(os.DirFS("../.."), string(b)) {
		t.Error(p)
	}
}

// The gate is red for each fault it names.
func TestTheGateIsRedForWhatItNames(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"ui/a_test.go":         {Data: []byte("package ui\n\nfunc TestA(t *testing.T) {}\n")},
		"ui/w_test.go":         {Data: []byte("//go:build !windows\n\npackage ui\n\nfunc TestW(t *testing.T) {}\n")},
		"ui/x_windows_test.go": {Data: []byte("package ui\n\nfunc TestX(t *testing.T) {}\n")},
	}
	line := func(test, systems string) string { return "ui\t" + test + "\t" + systems + "\tA promise.\n" }
	for _, c := range []struct{ name, file, want string }{
		{"a good file", line("TestA", "all") + line("TestW", "linux,darwin") + line("TestX", "windows"), ""},
		{"a test that is not there", line("TestNone", "all"), "no test TestNone in ui"},
		{"a windows line in a !windows file", line("TestW", "windows"), "the test TestW is only in files windows does not build"},
		{"an all line in a windows file", line("TestX", "all"), "the test TestX is only in files darwin does not build"},
		{"an unsorted file", line("TestW", "linux") + line("TestA", "all"), "not sorted"},
		{"a repeated line", line("TestA", "all") + line("TestA", "all"), "repeated"},
		{"a line of three fields", "ui\tTestA\tall\n", "not four tab-separated fields"},
		{"an unknown system", line("TestA", "plan9"), "unknown system plan9"},
	} {
		got := strings.Join(problems(fsys, c.file), "\n")
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
