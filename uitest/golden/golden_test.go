package golden

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// recorder is the T a test hands File to see what it said.
type recorder struct {
	fatal bool
	msg   string
}

func (r *recorder) Helper()                   {}
func (r *recorder) Errorf(f string, a ...any) { r.msg = fmt.Sprintf(f, a...) }
func (r *recorder) Fatalf(f string, a ...any) { r.fatal, r.msg = true, fmt.Sprintf(f, a...) }

// File holds the bytes exactly: a golden with no final newline, or with a NUL in it, passes as it
// is, and fails when one byte is added at the end, naming the file and the two lengths; a wrong
// line is named with the file and its number; a missing golden is fatal.
//
// What it costs when this fails: a program with a JSON or binary golden is handed a trim it did
// not ask for, and a change in the last byte of its output passes.
func TestAGoldenFileIsTheBytesExactly(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ file, got string }{{"planted.json", `{"a":1}`}, {"planted.bin", "\x00\x01\nfin\xff"}} {
		r := &recorder{}
		File(r, tc.file, []byte(tc.got))
		if r.msg != "" {
			t.Errorf("%s: the golden's own bytes failed: %s", tc.file, r.msg)
		}
		r = &recorder{}
		File(r, tc.file, []byte(tc.got+"\n"))
		for _, want := range []string{"testdata/" + tc.file, "got " + strconv.Itoa(len(tc.got)+1) + " bytes, want " + strconv.Itoa(len(tc.got))} {
			if !strings.Contains(r.msg, want) {
				t.Errorf("%s with one byte more: the message %q does not say %q", tc.file, r.msg, want)
			}
		}
	}
	r := &recorder{}
	File(r, "planted.bin", []byte("\x00\x01\nfun\xff"))
	for _, want := range []string{"testdata/planted.bin", "line 2", `got  "fun\xff"`, `want "fin\xff"`} {
		if !strings.Contains(r.msg, want) {
			t.Errorf("a wrong line: the message %q does not say %q", r.msg, want)
		}
	}
	r = &recorder{}
	File(r, "planted.json", []byte("{\"a\":1}\n{\"b\":2}"))
	for _, want := range []string{"testdata/planted.json", "line 2", `got  "{\"b\":2}"`, `want ""`} {
		if !strings.Contains(r.msg, want) {
			t.Errorf("an extra line: the message %q does not say %q", r.msg, want)
		}
	}
	r = &recorder{}
	File(r, "missing.json", []byte("x"))
	if !r.fatal {
		t.Error("bytes with no golden passed")
	}
}

// A name cannot step out of testdata, even to come back in, and under -update File writes the
// bytes as given below testdata, through the os.Root, in the subfolders its name makes, and
// nowhere else.
//
// What it costs when this fails: -update with a name built from a test's input writes a file
// anywhere the test's account can reach.
func TestAGoldenFileIsWrittenUnderUpdateInsideTestdataOnly(t *testing.T) {
	r := &recorder{}
	File(r, "../testdata/planted.json", []byte(`{"a":1}`))
	if !r.fatal {
		t.Error("a name that leaves testdata and comes back was read")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	*update = true
	t.Cleanup(func() { *update = false })
	r = &recorder{}
	File(r, "sub/new.bin", []byte("{\"b\":2}\x00"))
	if got, err := os.ReadFile(filepath.Join(dir, "testdata", "sub", "new.bin")); err != nil || string(got) != "{\"b\":2}\x00" || r.msg != "" {
		t.Errorf("-update wrote %q, %v, %q, want the bytes as given under testdata/sub", got, err, r.msg)
	}
	r = &recorder{}
	File(r, "../outside.json", []byte("x"))
	if !r.fatal {
		t.Error("-update took a name that leaves testdata")
	}
	if _, err := os.Stat(filepath.Join(dir, "outside.json")); err == nil {
		t.Error("-update wrote a file outside testdata")
	}
}

// Tree lists every difference of a folder from its golden at once: a file gone, a file added, and a file
// whose bytes differ, with the line; a folder equal to its golden passes; and under -update the golden
// becomes the folder, a file the golden had and the folder lacks removed.
//
// What it costs when this fails: a program that writes several files passes with one of them missing or
// changed, or -update leaves a stale file in the golden that a later run fails on.
func TestAGoldenTreeListsEveryDifferenceAndUpdateReplacesTheFolder(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	write := func(name, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("out/a.txt", "one\ntwo")
	write("out/sub/b.txt", "bee")
	write("testdata/t/a.txt", "one\ntwo")
	write("testdata/t/sub/b.txt", "bee")
	r := &recorder{}
	Tree(r, "out", "t")
	if r.msg != "" {
		t.Errorf("a folder equal to its golden failed: %s", r.msg)
	}
	write("out/a.txt", "one\nTWO")
	write("out/new.txt", "x")
	if err := os.Remove("out/sub/b.txt"); err != nil {
		t.Fatal(err)
	}
	r = &recorder{}
	Tree(r, "out", "t")
	for _, want := range []string{"a.txt: got 7 bytes, want 7; line 2", "sub/b.txt: the golden has it, the folder does not", "new.txt: the folder has it, the golden does not"} {
		if !strings.Contains(r.msg, want) {
			t.Errorf("the message %q does not say %q", r.msg, want)
		}
	}
	r = &recorder{}
	Tree(r, "out", "missing")
	if !r.fatal {
		t.Error("a folder with no golden passed")
	}
	*update = true
	t.Cleanup(func() { *update = false })
	r = &recorder{}
	Tree(r, "out", "t")
	*update = false
	if r.msg != "" {
		t.Fatalf("-update said %q", r.msg)
	}
	if _, err := os.Stat("testdata/t/sub/b.txt"); err == nil {
		t.Error("-update left a file the folder lacks in the golden")
	}
	r = &recorder{}
	Tree(r, "out", "t")
	if r.msg != "" {
		t.Errorf("after -update the folder differs from its golden: %s", r.msg)
	}
}

// A golden that holds LF is not matched by what was drawn with CR LF: File translates no line ends, so the one file
// is the same bytes on every system and a Windows run that draws CR LF fails, naming the file and the line.
//
// What it costs when this fails: a golden passes on Windows only because a CR was dropped on the way, so the bytes a
// product checks in are not the bytes it draws, and the same test is red or green by the system.
func TestAGoldenFileTranslatesNoLineEnds(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	File(r, "planted-lf.txt", []byte("a\nb\n"))
	if r.msg != "" {
		t.Errorf("the golden's own bytes failed: %s", r.msg)
	}
	r = &recorder{}
	File(r, "planted-lf.txt", []byte("a\r\nb\n"))
	for _, want := range []string{"testdata/planted-lf.txt", "line 1", `got  "a\r"`, `want "a"`} {
		if !strings.Contains(r.msg, want) {
			t.Errorf("CR LF drawn against an LF golden: the message %q does not say %q", r.msg, want)
		}
	}
}
