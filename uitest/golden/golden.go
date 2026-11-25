// Package golden compares a test's bytes with a file under testdata, or a folder of files with a folder
// under testdata. It is built on the standard library alone (io/fs and os.Root) and adds File, an
// exact-byte compare with no trimming, and Tree, the same for a folder, listing every difference. The test
// configures it with the name and the -update flag that writes it.
package golden

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

var update = flag.Bool("update", false, "write the bytes a test got as the goldens") //nolint:gochecknoglobals // the one -update flag of every golden

// T is the part of *testing.T that File uses, so a test can hand it a recorder.
type T interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// File compares got, exactly as it is, with testdata/<file>, or with -update writes it there. file
// is the whole name under testdata, its extension included, and both go through testdata as an
// os.Root, so a name cannot leave it. A mismatch names the file and the first line that differs.
func File(t T, file string, got []byte) {
	t.Helper()
	if *update {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatalf("%v", err)
		}
	}
	root, err := os.OpenRoot("testdata")
	if err != nil {
		t.Fatalf("no testdata folder for the goldens (run the test with -update to write them): %v", err)
		return
	}
	defer root.Close() //nolint:errcheck // nothing is buffered
	want, err := root.ReadFile(file)
	if *update {
		err = errors.Join(root.MkdirAll(filepath.Dir(file), 0o750), root.WriteFile(file, got, 0o600))
		want = got
	}
	if err != nil {
		t.Fatalf("golden testdata/%s (run the test with -update to write it): %v", file, err)
		return
	}
	g, w := strings.Split(string(got)+"\n", "\n"), strings.Split(string(want)+"\n", "\n") // the extra "" is what a shorter side has to show
	n := 0
	for n < len(g) && n < len(w) && g[n] == w[n] {
		n++
	}
	if bytes.Equal(want, got) {
		return
	} else if n == len(g) || n == len(w) {
		t.Errorf("the bytes are not testdata/%s: got %d bytes, want %d, the same lines as far as one goes", file, len(got), len(want))
	} else {
		t.Errorf("the bytes are not testdata/%s: line %d is\n  got  %q\n  want %q", file, n+1, g[n], w[n])
	}
}

// Tree compares the files under the folder dir with the files under testdata/<name>, or with -update
// replaces testdata/<name> with them. It lists every difference at once: a file the golden has and dir
// lacks, a file dir has and the golden lacks, and a file whose bytes differ, with both lengths and the
// first line that differs. Only files count, and their paths are slash-separated; an empty folder is
// not one. Both sides are read through an os.Root, so a name cannot leave its folder.
func Tree(t T, dir, name string) {
	t.Helper()
	got, err := tree(dir)
	if err != nil {
		t.Fatalf("the folder %s: %v", dir, err)
		return
	}
	if *update {
		err = errors.Join(os.MkdirAll("testdata", 0o750), writeTree("testdata", name, got))
		if err != nil {
			t.Fatalf("testdata/%s: %v", name, err)
		}
		return
	}
	want, err := tree(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("golden testdata/%s (run the test with -update to write it): %v", name, err)
		return
	}
	var diffs []string
	for _, f := range slices.Sorted(maps.Keys(want)) {
		if g, ok := got[f]; !ok {
			diffs = append(diffs, f+": the golden has it, the folder does not")
		} else if !bytes.Equal(g, want[f]) {
			diffs = append(diffs, fmt.Sprintf("%s: got %d bytes, want %d; %s", f, len(g), len(want[f]), firstDiff(g, want[f])))
		}
	}
	for _, f := range slices.Sorted(maps.Keys(got)) {
		if _, ok := want[f]; !ok {
			diffs = append(diffs, f+": the folder has it, the golden does not")
		}
	}
	if len(diffs) > 0 {
		t.Errorf("the files of %s are not testdata/%s:\n  %s", dir, name, strings.Join(diffs, "\n  "))
	}
}

func firstDiff(got, want []byte) string {
	g, w := strings.Split(string(got)+"\n", "\n"), strings.Split(string(want)+"\n", "\n")
	n := 0
	for n < len(g) && n < len(w) && g[n] == w[n] {
		n++
	}
	if n == len(g) || n == len(w) {
		return "the same lines as far as one goes"
	}
	return fmt.Sprintf("line %d is %q, want %q", n+1, g[n], w[n])
}

// tree reads every regular file under dir, keyed by its slash path.
func tree(dir string) (map[string][]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err //nolint:wrapcheck // os's words say it
	}
	defer root.Close() //nolint:errcheck // nothing is buffered
	files := map[string][]byte{}
	fsys := root.FS()
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		files[p], err = fs.ReadFile(fsys, p)
		return err
	})
	return files, err
}

// writeTree replaces base/name with files, through an os.Root at base.
func writeTree(base, name string, files map[string][]byte) error {
	root, err := os.OpenRoot(base)
	if err != nil {
		return err //nolint:wrapcheck // os's words say it
	}
	defer root.Close() //nolint:errcheck // nothing is buffered
	errs := []error{root.RemoveAll(name), root.MkdirAll(name, 0o750)}
	for f, b := range files {
		errs = append(errs, root.MkdirAll(path.Join(name, path.Dir(f)), 0o750), root.WriteFile(path.Join(name, f), b, 0o600))
	}
	return errors.Join(errs...)
}
