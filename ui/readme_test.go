package ui

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	// A file is limited to some systems by a GOOS or GOARCH-free suffix or by a build line that names one.
	goosSuffix = regexp.MustCompile(`_(windows|linux|darwin|unix)(_test)?\.go$`)
	osBuild    = regexp.MustCompile(`(?m)^//go:build .*\b(windows|linux|darwin|unix|freebsd|openbsd|netbsd)\b`)
)

// The README's Systems table lists every file limited to some systems, and no file it does not.
//
// What it costs when this fails: the README says these are the only files limited to some systems,
// and a file that is not in it makes that false.
func TestTheReadmeListsEveryFileLimitedToSomeSystems(t *testing.T) {
	t.Parallel()
	root := ".."
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `([^`]+\\.go)` \\|").FindAllStringSubmatch(string(readme), -1) {
		listed[m[1]] = true
	}
	found := map[string]bool{}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "tools") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		head, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if goosSuffix.MatchString(p) || osBuild.Match(head) {
			rel, _ := filepath.Rel(root, p)
			found[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for f := range found {
		if !listed[f] {
			t.Errorf("%s is limited to some systems and the README's Systems table does not list it", f)
		}
	}
	for f := range listed {
		if !found[f] {
			t.Errorf("the README's Systems table lists %s, which is not limited to some systems", f)
		}
	}
	if len(found) == 0 {
		t.Fatal("no file limited to some systems was found: the walk is wrong")
	}
}

// The README's quick start is the program in ui/testdata/quickstart, letter for letter, and that program builds and runs:
// it prints a line and a table in plain columns, ends on its closing words and exits 0.
//
// What it costs when this fails: a person who copies the quick start gets a program that does not compile, or that
// prints something other than what the README shows.
func TestTheReadmeQuickStartBuildsAndRuns(t *testing.T) {
	t.Parallel()
	readme, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile("(?s)## Quick start.*?```go\n(.*?)```").FindSubmatch(readme)
	if m == nil {
		t.Fatal("the README has no Go block under Quick start")
	}
	want, err := os.ReadFile(filepath.Join("testdata", "quickstart", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(m[1]) != string(want) {
		t.Fatal("the README's quick start is not ui/testdata/quickstart/main.go: copy the file into the block")
	}
	exe := filepath.Join(t.TempDir(), "app.exe")
	if out, err := exec.CommandContext(t.Context(), "go", "build", "-o", exe, "./testdata/quickstart").CombinedOutput(); err != nil {
		t.Fatalf("go build of the quick start: %v: %s", err, out)
	}
	out, err := exec.CommandContext(t.Context(), exe).CombinedOutput()
	if err != nil {
		t.Fatalf("the quick start exits with an error: %v: %s", err, out)
	}
	for _, line := range []string{"two services", "name    state", "web     running", "worker  stopped", "done       listed 2 services"} {
		if !strings.Contains(string(out), line) {
			t.Errorf("the quick start printed no %q: %s", line, out)
		}
	}
}

// TestTheReadmeNamesEveryDirectRequirement holds the README's "Built on" section to go.mod: every
// direct requirement is named in it by its module path or its short name, and no module path it links
// as a requirement is missing from go.mod.
func TestTheReadmeNamesEveryDirectRequirement(t *testing.T) {
	t.Parallel()
	mod, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	readme, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(readme)
	section := text[strings.Index(text, "## Built on"):]
	if i := strings.Index(section[3:], "\n## "); i >= 0 {
		section = section[:i+3]
	}
	short := func(path string) string {
		path = strings.TrimSuffix(path, "/v2")
		return path[strings.LastIndex(path, "/")+1:]
	}
	direct := map[string]bool{}
	in := false
	for line := range strings.SplitSeq(string(mod), "\n") {
		switch {
		case line == "require (":
			in = true
		case line == ")":
			in = false
		case in && !strings.Contains(line, "// indirect"):
			if f := strings.Fields(line); len(f) >= 2 {
				direct[f[0]] = true
			}
		}
	}
	if len(direct) == 0 {
		t.Fatal("no direct requirement found in go.mod")
	}
	for path := range direct {
		if !strings.Contains(section, short(path)) {
			t.Errorf("go.mod requires %s and the README's Built on section does not name it (%q)", path, short(path))
		}
	}
	for _, word := range []string{"bubbles", "bubbletea", "huh", "lipgloss", "fang", "cobra", "pflag", "colorprofile", "x/term", "x/ansi", "x/vt", "x/xpty", "go-humanize", "x/sys", "go-run", "go-testkit"} {
		found := false
		for path := range direct {
			if strings.HasSuffix(strings.TrimSuffix(path, "/v2"), word) || strings.Contains(path, word) {
				found = true
			}
		}
		if strings.Contains(section, word) && !found {
			t.Errorf("the README's Built on section names %s, which go.mod does not require", word)
		}
	}
}
