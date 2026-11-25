package taskcheck

import (
	"os"
	"strings"
	"testing"

	"github.com/birdtable/go-testkit/taskfile"
)

// The release tier and the group run keep -race -shuffle=on -count=1: only the pre-push tier passes CACHED to the
// test task, and the task drops -count=1 and -shuffle=on for CACHED alone.
//
// What it costs when this fails: a pre-push that carries -shuffle reuses no result (go test does not cache a
// shuffled run), and a release or a group run that loses either flag reuses an earlier result or stops proving
// the tests do not depend on their order.
func TestOnlyPrePushLetsGoTestReuseAResult(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("../../Taskfile.yml")
	if err != nil {
		t.Fatal(err)
	}
	var prePush, release, testLine string
	for l := range strings.SplitSeq(string(b), "\n") {
		switch {
		case strings.Contains(l, "hatch tests"):
			testLine = l
		case strings.HasPrefix(l, "      - cmd: go tool gate tier ") && strings.Contains(l, "vet-systems") && strings.Contains(l, "help-pty"):
			release = l
		case strings.HasPrefix(l, "      - cmd: go tool gate tier ") && strings.Contains(l, "vet-systems"):
			prePush = l
		}
	}
	if !strings.Contains(testLine, "{{if not .CACHED}}-count=1 -shuffle=on {{end}}") {
		t.Errorf("the test task does not hold -count=1 and -shuffle=on back for CACHED alone:\n%s", testLine)
	}
	if strings.Count(testLine, "-shuffle") != 1 || strings.Count(testLine, "-count=1") != 1 || !strings.Contains(testLine, "-race") {
		t.Errorf("the test task carries -shuffle or -count=1 outside the CACHED guard, or lost -race:\n%s", testLine)
	}
	if !strings.Contains(prePush, " test:CACHED=1 ") {
		t.Errorf("pre-push does not pass CACHED to the test task:\n%s", prePush)
	}
	if release == "" || !strings.Contains(release, " test ") || strings.Contains(release, "CACHED") {
		t.Errorf("release does not run the plain test task, which keeps -count=1:\n%s", release)
	}
}

// Taskfile.yml calls Go: a task's commands run a Go program, or one binary with its arguments,
// and keep their logic, paths and temporary folders in Go. The check is go-testkit's, the one that every library's
// Taskfile is held to; it refuses a shell utility, a pipe, a redirection to a file, a shell loop, a /tmp path and a path
// joined in shell. It runs in the commit tier, as the task taskfile.
//
// What it costs when this fails: go-task runs the shell in Taskfile.yml through its own interpreter, which runs it
// differently on Windows (it cannot append an external program's output with >> there, and its tar takes no -C), and
// no vet, build or lint reads it, so a gate that passes on one system fails on another.
func TestTheTaskfileCallsGo(t *testing.T) {
	t.Parallel()
	taskfile.Check(t, "../..")
}

// A tier's members are the Taskfile's tasks: the tiers name their members in one line each, which the
// go tool gate tier call reads, so a task the line does not name is run by no tier, and a name that is no task fails
// at the run. The members of task api, which is none of the tiers' own, are in both pre-push and release.
//
// What it costs when this fails: a member added to api and not to the tier is run by neither, so a gate stops
// holding what it held and nothing is red.
func TestTheMembersOfATierAreTheTaskfilesTasks(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("../../Taskfile.yml")
	if err != nil {
		t.Fatal(err)
	}
	tasks := map[string]bool{}
	tiers := map[string][]string{}
	var api []string
	for l := range strings.SplitSeq(string(b), "\n") {
		if name, ok := strings.CutPrefix(l, "  "); ok && strings.HasSuffix(name, ":") && !strings.HasPrefix(name, " ") {
			tasks[strings.TrimSuffix(name, ":")] = true
		}
		if rest, ok := strings.CutPrefix(l, "      - cmd: go tool gate tier "); ok {
			f := strings.Fields(rest)
			tiers[f[0]] = f[1:]
		}
		if rest, ok := strings.CutPrefix(l, "    deps: ["); ok && strings.Contains(rest, "apigate") {
			api = strings.Split(strings.TrimSuffix(rest, "]"), ", ")
		}
	}
	native := map[string]bool{"fmt": true, "size": true, "marks": true, "contract": true, "help-pty": true}
	for tier, members := range tiers {
		have := map[string]bool{}
		for _, m := range members {
			name, _, _ := strings.Cut(m, ":")
			have[name] = true
			if !tasks[name] && !native[name] {
				t.Errorf("%s names %q, which is no task of Taskfile.yml", tier, name)
			}
		}
		if tier == "pre-commit" {
			continue
		}
		for _, a := range api {
			if !have[a] {
				t.Errorf("%s does not run %q, a member of task api", tier, a)
			}
		}
	}
	if len(tiers) != 3 || len(api) != 3 {
		t.Errorf("read %d tiers and %d members of api, want 3 and 3", len(tiers), len(api))
	}
}
