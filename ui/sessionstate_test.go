package ui

// What a run keeps between its calls is its session's, not the package's: the
// banner it drew, the thresholds its bars report at, and the streams its Execute found. Two
// runs in one process, as two tests, each keep their own.

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/birdtable/go-console/uitest"
	"github.com/spf13/cobra"
)

// Two sessions that arrive and report at once, in goroutines of their own, each draw their own
// banner and say it is on screen, and each bar reports at its own session's thresholds: one
// shrunk for a test, the other as it ships.
//
// What it costs when this fails: a run's banner is drawn by another run in the same process, so
// a command test that runs after another sees no banner and prints its plain heading; or one
// test's shrunk thresholds reach another run, whose log gets a line an item.
func TestTwoSessionsAtOnceEachArriveAndReportAtTheirOwn(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads the process's stdout at a terminal that paints, then at none.
	scr := screenOpt(true, 0)
	a, b := New(scr), New(scr)
	arrived := make([]bool, 2)
	out := capture(t, &os.Stdout, func() {
		var wg sync.WaitGroup
		for i, s := range []*Session{a, b} {
			wg.Go(func() { arrived[i] = s.Arrive(testBrand(), "v1") })
		}
		wg.Wait()
	})
	if n := strings.Count(out, "a made-up tagline"); n != 2 {
		t.Errorf("two sessions drew %d banners, want one each:\n%s", n, out)
	}
	if !arrived[0] || !arrived[1] {
		t.Errorf("the sessions say a banner is on screen: %v, want both", arrived)
	}
	if again := capture(t, &os.Stdout, func() { a.Arrive(testBrand(), "v1") }); strings.TrimSpace(again) != "" {
		t.Errorf("a session drew its banner a second time:\n%q", again)
	}
	shrunk, shipped := New(scr, reportingEvery(4, time.Hour)), New(scr)
	const items = 40
	out = capture(t, &os.Stdout, func() {
		var wg sync.WaitGroup
		for _, run := range []struct {
			s    *Session
			name string
		}{{shrunk, "shrunk"}, {shipped, "shipped"}} {
			wg.Go(func() {
				bar := run.s.NewBar(run.name+"-reading", items)
				for range items {
					bar.Add(1)
				}
				bar.Stop()
			})
		}
		wg.Wait()
	})
	// Forty items at a threshold of four: the opening and closing lines and ten between.
	// At the shipped threshold of 10,000, the opening and closing lines alone.
	if n := strings.Count(out, "shrunk-reading"); n < 10 {
		t.Errorf("the session with its thresholds shrunk reported %d times, want about 12, so its own thresholds are not read:\n%s", n, out)
	}
	if n := strings.Count(out, "shipped-reading"); n != 2 {
		t.Errorf("the session as it ships reported %d times, want its opening and closing lines alone, so another session's thresholds reached it:\n%s", n, out)
	}
}

// Two trees run by Execute at once each find their own streams with Raw, while they run.
//
// What it costs when this fails: a command that writes bytes, as an ssh session or a copied
// file, writes them into another run's stream when two run in one process.
func TestTwoExecutesAtOnceEachHaveTheirOwnRawStreams(t *testing.T) {
	t.Parallel()
	type run struct {
		out, errOut bytes.Buffer
		raw         [2]io.Writer
	}
	runs := []*run{{}, {}}
	var wg sync.WaitGroup
	for _, r := range runs {
		root := &cobra.Command{Use: "app", Run: func(c *cobra.Command, _ []string) {
			r.raw[0], r.raw[1] = Raw(c)
		}}
		root.SetOut(&r.out)
		root.SetErr(&r.errOut)
		root.SetArgs(nil)
		wg.Go(func() {
			if err := Execute(context.Background(), root, runner(), Brand{}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	for i, r := range runs {
		if r.raw[0] != io.Writer(&r.out) || r.raw[1] != io.Writer(&r.errOut) {
			t.Errorf("run %d found streams that are not its own with Raw", i)
		}
	}
	if len(runs) != 2 {
		t.Fatalf("ran %d trees, want 2", len(runs))
	}
}

// A session on a Terminal 40 wide that renders draws into a buffer as that terminal paints: a
// banner and an ending, the bytes Painted(40) drew before it went. The golden was
// written by Painted, so this holds the session to it byte for byte.
//
// What it costs when this fails: a program that draws its look into a buffer
// gets its banner and endings plain or wrapped at the wrong width, and nobody sees it
// until it is on a person's screen.
func TestASessionFortyWideDrawsWhatPaintedDid(t *testing.T) {
	t.Parallel()
	s := New(screenOpt(true, 40), houseEndings())
	var b strings.Builder
	s.Capture(&b, func() {
		s.Arrive(testBrand(), "v1.0.0")
		s.End(needsYou())
	})
	if !strings.Contains(b.String(), "\x1b[") {
		t.Fatalf("the session painted nothing, so the golden proves nothing:\n%q", b.String())
	}
	uitest.Golden(t, "painted-at-40", b.String())
}

// Two sessions that ask and print at once, in goroutines of their own, each hear their own
// answerer and their own replier, and each know whether their own last line was blank.
//
// What it costs when this fails: one test's stand-in answers another test's question, so a
// test that means to decline confirms, or a run puts a blank line before its ending, or
// leaves it out, because another run's last line was blank.
func TestTwoSessionsAtOnceEachHearTheirOwnAnswerAndKeepTheirOwnLastLine(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads the process's stdout.
	const rounds = 50
	yes := answeredBy(strings.Repeat("y\nyes's\n", rounds), io.Discard)
	no := answeredBy(strings.Repeat("n\nno's\n", rounds), io.Discard)
	var heard [2][]bool
	var replied [2][]string
	started := 0
	_ = capture(t, &os.Stdout, func() {
		var wg sync.WaitGroup
		for i, run := range []struct {
			s    *Session
			last string
		}{{yes, ""}, {no, "a line"}} {
			started++
			wg.Go(func() {
				for range rounds {
					ok, _ := run.s.Confirm("delete the cache", "", "--yes")
					heard[i] = append(heard[i], ok)
					v, _ := run.s.Secret("token: ", 64)
					replied[i] = append(replied[i], v)
					run.s.Log("%s", "between")
					run.s.Log("%s", run.last)
				}
			})
		}
		wg.Wait()
	})
	if started != 2 {
		t.Fatalf("started %d sessions, want 2", started)
	}
	for i, c := range []struct {
		name   string
		answer bool
		reply  string
		blank  bool
		s      *Session
	}{{"yes", true, "yes's", true, yes}, {"no", false, "no's", false, no}} {
		if len(heard[i]) != rounds || len(replied[i]) != rounds {
			t.Fatalf("%s asked %d questions and %d secrets, want %d of each", c.name, len(heard[i]), len(replied[i]), rounds)
		}
		for n := range rounds {
			if heard[i][n] != c.answer || replied[i][n] != c.reply {
				t.Errorf("%s, round %d: heard %v and %q, want its own %v and %q", c.name, n, heard[i][n], replied[i][n], c.answer, c.reply)
				break
			}
		}
		if c.s.EndsBlank() != c.blank {
			t.Errorf("%s ends blank: %v, want %v, from its own last line", c.name, c.s.EndsBlank(), c.blank)
		}
	}
}
