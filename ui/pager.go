package ui

// A long page at a terminal goes through a pager, as git's does: the command
// the session was given (Pager), which PagerFromEnv reads from $PAGER. In a
// pipe, a redirect, with no pager given, PAGER unset, empty or cat, or with no
// such program on the machine, the page is printed as it is.

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"
)

// PagerFromEnv is the pager the environment env names, for Pager: $PAGER split
// on spaces (on Windows by the command-line rules, so a quoted path with
// spaces is one word). An unset PAGER, an empty one, or cat, is none: nil. The
// caller names any fallback, such as less -FRX; this package has none.
func PagerFromEnv(env []string) []string {
	var pager string
	pagerSet := false
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); k == "PAGER" {
			pager, pagerSet = v, true
		}
	}
	if !pagerSet {
		return nil
	}
	argv := splitPager(pager)
	if len(argv) == 0 || argv[0] == "cat" {
		return nil
	}
	return argv
}

// pagerFor is the pager the session runs for its output, or nil for none: none
// where no pager was given, where nobody is at the terminal, or where the
// program is not on the machine, which would swallow the page and say only
// "not found". ctx ending ends it.
func (s *Session) pagerFor(ctx context.Context) *exec.Cmd {
	program, args := pagerProgram(s.pager)
	if program == "" || !s.outStream().interactive {
		return nil
	}
	if _, err := exec.LookPath(program); err != nil {
		return nil
	}
	return exec.CommandContext(ctx, program, args...) //nolint:gosec // the command the program gave as its pager
}

// pagerProgram is the program a pager command starts and its arguments, or ""
// for none. The one place the choice is made, so the program that is run is
// always a function of this package, so it can be replaced without touching
// the callers.
func pagerProgram(argv []string) (program string, args []string) {
	if len(argv) == 0 {
		return "", nil
	}
	return argv[0], argv[1:]
}

// Paged runs show with what it writes going through the session's pager, when
// there is one, and straight to the session's output when not. A pager that
// fails never costs the page: it is printed after it. ctx ending ends the pager.
// The error is a failure to write the page.
func (s *Session) Paged(ctx context.Context, show func(w io.Writer)) error {
	pager := s.pagerFor(ctx)
	s.pagerUsed.Store(pager != nil)
	return s.paged(pager, show)
}

// PagerUsed is whether the last Paged went through the pager. False means the
// page was printed as it is: in a pipe, with no pager given, or with a pager
// that is not on the machine, which is no error, since the page still reached
// the reader.
func (s *Session) PagerUsed() bool { return s.pagerUsed.Load() }

// paged is Paged through pager, or straight to the output when pager is nil:
// the pager is an argument so a test can say which one this machine has.
func (s *Session) paged(pager *exec.Cmd, show func(w io.Writer)) error {
	out := s.outW()
	if pager == nil {
		show(out)
		return nil
	}
	var page bytes.Buffer
	show(&page)
	pager.Stdin = bytes.NewReader(page.Bytes())
	pager.Stdout, pager.Stderr = out, s.errW()
	// A pager that failed may have shown none of it, and there is no telling
	// how much: the page again is better than no page. Somebody quitting the
	// pager halfway is their answer, as it is to git, and is no failure.
	if err := pager.Run(); err != nil {
		_, werr := out.Write(page.Bytes())
		return werr
	}
	return nil
}
