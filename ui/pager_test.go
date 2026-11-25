package ui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"testing"
)

// helperSwitch is the argument that makes this test binary act as a pager that closes its input
// and fails, the standard re-exec pattern: no test here starts a shell, so none needs sh on the
// PATH. It is an argument and not a variable because the tests' start clears the environment.
const helperSwitch = "ui-failing-pager"

// TestFailingPagerHelper is the program TestAFailingPagerStillLeavesThePage starts, not a test:
// without helperSwitch it returns at once.
func TestFailingPagerHelper(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	if !slices.Contains(os.Args, helperSwitch) {
		return
	}
	_ = os.Stdin.Close()
	os.Exit(3)
}

// thisBinary is the path of the running test binary, which stands for a pager that is installed.
func thisBinary(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

// In a pipe the page is printed as it is, whatever pager the session was given:
// a pager there would wait on a terminal nobody is at.
//
// What it costs when this fails: `app help dns | grep` hangs, or prints
// nothing.
func TestAPageInAPipeIsPrintedAsItIs(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	s := New(Out(&out), Pager("false"))
	if err := s.Paged(t.Context(), func(w io.Writer) { fmt.Fprintln(w, "the whole lesson") }); err != nil {
		t.Fatal(err)
	}
	if out.String() != "the whole lesson\n" {
		t.Errorf("a pipe got %q", out.String())
	}
}

// PagerFromEnv reads $PAGER: none for an empty one or cat, its words for any
// other, and none when it is unset, whatever LESS says. At a
// terminal a pager that is not installed is none.
//
// What it costs when this fails: a person's $PAGER is ignored, or a page is
// swallowed by a pager that is not there.
func TestThePagerComesFromTheEnvironmentAndMustBeInstalled(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		env  []string
		want []string
	}{
		{[]string{"PAGER="}, nil},
		{[]string{"PAGER=cat"}, nil},
		{[]string{"PAGER=app-pager -s"}, []string{"app-pager", "-s"}},
		{nil, nil},
		{[]string{"LESS=R"}, nil},
	} {
		if got := PagerFromEnv(c.env); !slices.Equal(got, c.want) {
			t.Errorf("%v: the pager is %q, want %q", c.env, got, c.want)
		}
	}
	at := On(Terminal{Interactive: true})
	if p := New(at, Pager("no-such-pager-anywhere", "-x")).pagerFor(t.Context()); p != nil {
		t.Errorf("a pager that is not installed runs %v", p.Args)
	}
	if p := New(at).pagerFor(t.Context()); p != nil {
		t.Errorf("a session with no pager runs %v", p.Args)
	}
	exe := thisBinary(t)
	if p := New(at, Pager(exe, "-s")).pagerFor(t.Context()); p == nil || !slices.Equal(p.Args, []string{exe, "-s"}) {
		t.Errorf("Pager(%s, -s) runs %v", exe, p)
	}
}

// A pager that takes nothing and fails leaves the page printed anyway.
//
// What it costs when this fails: a broken $PAGER and the lesson is gone.
func TestAFailingPagerStillLeavesThePage(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	failing := exec.CommandContext(t.Context(), thisBinary(t), "-test.run=^TestFailingPagerHelper$", helperSwitch)
	if err := New(Out(&out)).paged(failing, func(w io.Writer) { fmt.Fprintln(w, "the whole lesson") }); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("the whole lesson")) {
		t.Errorf("after a failing pager the page is %q", out.String())
	}
}

// At a terminal a pager that is not on the machine is none: the page is printed as it is and
// Paged returns nil.
//
// What it costs when this fails: a program on a machine without its pager says "not found" or
// prints nothing, and the person loses the page.
func TestAPageWithNoPagerOnThePathIsPrintedAndReturnsNil(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	s := New(On(Terminal{Interactive: true}), Out(&out), Pager("no-such-pager-anywhere", "-x"))
	if err := s.Paged(t.Context(), func(w io.Writer) { fmt.Fprintln(w, "the whole lesson") }); err != nil {
		t.Fatalf("Paged returned %v, want nil", err)
	}
	if out.String() != "the whole lesson\n" {
		t.Errorf("the page is %q", out.String())
	}
}

// Whether the last page went through the pager is a fact the caller can read: a missing pager prints the
// page and says it was not used, and a pager that runs says it was.
//
// What it costs when this fails: a caller cannot tell a page that was paged from one that scrolled past.
func TestPagerUsedSaysWhetherTheLastPageWentThroughThePager(t *testing.T) {
	t.Parallel()
	show := func(w io.Writer) { fmt.Fprintln(w, "the whole lesson") }
	var out bytes.Buffer
	missing := New(On(Terminal{Interactive: true}), Out(&out), Pager("no-such-pager-anywhere", "-x"))
	if missing.PagerUsed() {
		t.Error("PagerUsed is true before any page")
	}
	if err := missing.Paged(t.Context(), show); err != nil {
		t.Fatal(err)
	}
	if out.String() != "the whole lesson\n" || missing.PagerUsed() {
		t.Errorf("a missing pager: the page is %q and PagerUsed is %v, want the page printed and false", out.String(), missing.PagerUsed())
	}
	piped := New(Out(&out), Pager(thisBinary(t), "-s"))
	if err := piped.Paged(t.Context(), show); err != nil || piped.PagerUsed() {
		t.Errorf("a pipe: err %v, PagerUsed %v, want nil and false", err, piped.PagerUsed())
	}
	exe := thisBinary(t)
	ran := New(On(Terminal{Interactive: true}), Out(&out), Pager(exe, "-test.run=^TestFailingPagerHelper$", helperSwitch))
	if err := ran.Paged(t.Context(), show); err != nil || !ran.PagerUsed() {
		t.Errorf("a pager that runs: err %v, PagerUsed %v, want nil and true", err, ran.PagerUsed())
	}
}
