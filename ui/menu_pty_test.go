package ui

// The bare run's menu on a real terminal: the test binary ptyprog on a pty,
// told PTYPROG=menu, with a key sent once the menu is up.

import (
	"context"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"

	"github.com/birdtable/go-console/uitest"
)

// screen is what a terminal shows of raw: the bytes run through an emulator, so the result is the
// same however the renderer got there, in one draw or in a draw and a repaint, in one read or in
// many. The rows are the screen's, with no trailing blank.
func screen(raw string) string {
	e := vt.NewEmulator(uitest.PtyScreenCols, 50)
	defer e.InputPipe().(io.Closer).Close()       // not e.Close, which races the drain below on a field of its own
	go func() { _, _ = io.Copy(io.Discard, e) }() // the answers to the queries in raw, which nobody reads
	_, _ = e.WriteString(raw)
	return menuScreen(e.String())
}

// menuUp reports whether the screen holds the whole menu: the row of the last choice and the two
// rows of the form under it. A form is drawn in one write, but a read can end inside it, and the
// screen is a golden, so it is read only when the form is all there.
func menuUp(screen string) bool {
	rows := strings.Split(screen, "\n")
	for i, r := range rows {
		if strings.Contains(r, "Stay quiet") {
			return i+2 < len(rows) && strings.HasPrefix(rows[i+1], "┃") && strings.HasPrefix(rows[i+2], "┃")
		}
	}
	return false
}

// typed is what the test sends and what the screen shows once the form has taken it. One with no
// shows is the last, the one that ends the run.
type typed struct{ send, shows string }

// menuAt runs ptyprog bare on a pty cols wide with env, sends keys once the menu is on the screen,
// each after the screen shows the one before it (nothing when there are none), and returns what it
// wrote with every escape stripped, and the screen as it was once the menu was up, and the code it
// exited with.
//
// The menu is up when the screen, as an emulator draws it from what the program wrote, holds the
// whole form. That is a fact about the screen and not about the time since the last byte, so a
// loaded machine that draws the form twice, or reads it in pieces, gives the same screen.
func menuAt(t *testing.T, cols int, env []string, keys ...typed) (final, menu string, code int) {
	t.Helper()
	exe, err := ptyProgram()
	if err != nil {
		t.Fatal(err)
	}
	// A run sent no keys is one that ends by itself: the context ends it if it waits for one, and
	// code is then -1, so the test says what the run waited for and no timer decides a frame.
	ctx := t.Context()
	if len(keys) == 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, 10*time.Second)
		defer stop()
	}
	cmd := exec.CommandContext(ctx, exe)
	cmd.Env = env
	p, err := uitest.StartPty(ctx, cmd, cols, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if len(keys) > 0 {
		const patience = 20 * time.Second
		if menu, _ = p.WaitFor(patience, func(s string, _ time.Duration) bool { return menuUp(s) }); menuUp(menu) {
			for _, k := range keys {
				_ = p.Send(k.send)
				if s, ok := p.WaitFor(patience, func(s string, _ time.Duration) bool { return strings.Contains(s, k.shows) }); k.shows != "" && !ok {
					t.Errorf("the screen never showed %q after the key %q; it shows:\n%s", k.shows, k.send, s)
					_ = cmd.Process.Kill()
				}
			}
		} else {
			_ = cmd.Process.Kill() // no menu in time; the screen below says what it wrote
		}
	}
	code = p.Wait()
	return p.Screen(), menu, code
}

// A bare run at a pty shows the banner and the menu, and Enter on a row runs its words on a fresh
// tree: --loud is read again, so the command says loud=true. The banner is on the screen once. At
// 80 columns the whole screen is a golden; at 60 and 40 no row is wider than the screen.
//
// What it costs when this fails: a bare run of a program with a menu shows a page of help, or
// runs the row with its flags unread.
func TestABareRunAtAPtyShowsTheMenuAndRunsThePick(t *testing.T) {
	t.Parallel()
	for _, cols := range []int{80, 60, 40} {
		t.Run(strconv.Itoa(cols), func(t *testing.T) {
			t.Parallel()
			got, menu, code := menuAt(t, cols, []string{"TERM=xterm-256color", "LANG=C.UTF-8", "PTYPROG=menu"}, typed{"noise", "/noise"}, typed{send: "\r"})
			if code != 0 || !strings.Contains(got, "ran loud=true") || !strings.Contains(got, "What now") {
				t.Fatalf("code %d, screen %q: want the menu, then a run with loud=true", code, got)
			}
			if n := strings.Count(got, "a made-up tagline"); n != 1 {
				t.Errorf("the banner is on the screen %d times, want 1", n)
			}
			for i, l := range strings.Split(menu, "\n") {
				if w := ansi.StringWidth(l); w > cols {
					t.Errorf("row %d is %d columns wide on a %d-column screen: %q", i+1, w, cols, l)
				}
			}
			if cols == 80 {
				uitest.Golden(t, "menu-80", menu)
			}
		})
	}
}

// menuScreen is the stripped output as lines with no carriage return and no trailing blank.
func menuScreen(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r", ""), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// A bare run at a pty with CI set prints help, asks nothing and returns 2.
func TestABareRunWithCIPrintsHelpAndReturnsTwo(t *testing.T) {
	t.Parallel()
	got, _, code := menuAt(t, 80, []string{"TERM=xterm-256color", "LANG=C.UTF-8", "PTYPROG=menu", "CI=1"})
	if code == -1 {
		t.Fatal("waited for a menu key with CI set")
	}
	if code != 2 || strings.Contains(got, "What now") || !strings.Contains(got, "--loud") {
		t.Errorf("code %d, screen %q: want help, no menu, code 2", code, got)
	}
}

// A form drawn twice before the snapshot is read, the second time as a repaint that moves the
// cursor up first, gives the screen of the last draw, with the banner above it, and not the two
// draws one after the other. This is the stream that failed on linux-amd64 under load, at pull
// request 27 and again at pull request 32: line 11 read "┃┃ /" where the golden has "┃". Nothing
// here depends on which escapes the renderer repaints with, or on where a read ends.
//
// What it costs when this fails: the menu golden fails wherever the renderer repaints the form.
func TestTheScreenIsWhatTheTerminalShowsOfTheForm(t *testing.T) {
	t.Parallel()
	body := "┃ /\r\n┃ What now?\r\n┃ > Stay quiet\r\n┃\r\n┃"
	draw := "\r\x1b[J" + body
	repaint := "\r\x1b[4A\x1b[J" + body
	once := "banner\r\n\r\n" + draw
	want := "banner\n\n┃ /\n┃ What now?\n┃ > Stay quiet\n┃\n┃"
	for name, raw := range map[string]string{
		"one draw":             once,
		"a draw and a repaint": once + repaint,
		"three draws":          once + repaint + repaint,
		"a repaint by line":    once + "\r\x1b[4A\x1b[2K" + strings.ReplaceAll(body, "\r\n", "\r\n\x1b[2K"),
	} {
		if got := screen(raw); got != want {
			t.Errorf("%s gave %q, want %q", name, got, want)
		}
	}
	if menuUp(screen(once[:len(once)-len("┃\r\n┃")])) {
		t.Error("a form cut before its last rows was taken as all there")
	}
	if !menuUp(screen(once)) {
		t.Error("a whole form was not taken as all there")
	}
}
