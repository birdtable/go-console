package measure

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// measure is pure: the standard library and github.com/charmbracelet/x/ansi
// are all its code imports, and it reads no environment and no file, at init
// or otherwise.
//
// What it costs when this fails: a layer that took measure to stay pure takes
// cobra, lipgloss or a writer set up from the environment with it, which is
// what the package was made to avoid.
func TestMeasureImportsOnlyTheStandardLibraryAndAnsi(t *testing.T) {
	t.Parallel()
	const ansiPath = "github.com/charmbracelet/x/ansi"
	// What reads the environment, a file or a device, whether at init or later.
	banned := []string{"os", "os/exec", "os/user", "io/fs", "io/ioutil", "embed", "syscall", "net", "plugin", "unsafe", "runtime/debug"}
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no Go files read (%v)", err)
	}
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		checked++
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			switch {
			case p == ansiPath:
			case slices.Contains(banned, p):
				t.Errorf("%s imports %s, which reads the environment, a file or a device", name, p)
			case strings.Contains(strings.Split(p, "/")[0], "."):
				t.Errorf("%s imports %s: measure takes the standard library and %s only", name, p, ansiPath)
			}
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "func init(") {
			t.Errorf("%s has an init: measure runs nothing at import", name)
		}
	}
	if checked < 2 {
		t.Fatalf("%d code files read, so this proves nothing", checked)
	}
}
