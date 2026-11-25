package main

import (
	"bytes"
	"strings"
	"testing"
)

// What it costs when this fails: a tier is held to another row of budgets.yaml than its host's, so a release on a
// session passes a budget that was meant to hold it, or a hosted runner is failed by one that was never its own.
func TestATierIsHeldToTheRowOfItsHost(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name         string
		tier         string
		goos, goarch string
		hosted       bool
		want         string
	}{
		{"the named host, pre-push", "pre-push", "darwin", "arm64", false, "pre-push-budget"},
		{"the named host, release", "release", "darwin", "arm64", false, "release-budget"},
		{"a Linux session, release", "release", "linux", "amd64", false, "release-budget-session"},
		{"a Windows session, release", "release", "windows", "amd64", false, "release-budget-session"},
		{"a Linux session, pre-push has no row", "pre-push", "linux", "amd64", false, ""},
		{"a hosted runner has no row", "release", "linux", "amd64", true, ""},
		{"another kind of host has no row", "release", "linux", "arm64", false, ""},
	} {
		if got := row(c.tier, "Darwin-arm64", c.goos, c.goarch, c.hosted); got != c.want {
			t.Errorf("%s: row %q, want %q", c.name, got, c.want)
		}
	}
}

// What it costs when this fails: the host row of budgets.yaml is read as another value, so no host is the named one.
func TestAKeyIsReadAsAwkReadsIt(t *testing.T) {
	t.Parallel()
	const file = "# host: Linux-x86_64\nhost: Darwin-arm64\npre-push-budget: 10\n release-budget: 99\n"
	for name, want := range map[string]string{"host": "Darwin-arm64", "pre-push-budget": "10", "release-budget": "", "none": ""} {
		if got := key(file, name); got != want {
			t.Errorf("key %q is %q, want %q", name, got, want)
		}
	}
}

// What it costs when this fails: the pty help bound is judged by a go test that said "ok" with no tests to run, so
// a test that was renamed or skipped passes the tier.
func TestATestThatDidNotPassIsNamed(t *testing.T) {
	t.Parallel()
	out := "=== RUN   TestA\n--- PASS: TestA (0.10s)\n--- SKIP: TestB (0.00s)\n--- PASS: TestAB (0.10s)\nok\n"
	got := missing(out, []string{"TestA", "TestB", "TestC"})
	if len(got) != 2 || got[0] != "TestB" || got[1] != "TestC" {
		t.Errorf("missing is %v, want [TestB TestC]", got)
	}
}

// What it costs when this fails: a gate is given arguments it does not take and runs another one, or none.
func TestArgumentsItDoesNotTakeAreRefused(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"nope"}, {"fmt", "x"}, {"size"}, {"size", "many"}, {"size", "-1"}, {"tier"}, {"tier", "pre-push"}, {"now"}} {
		var out, errOut bytes.Buffer
		if code := run(t.Context(), args, &out, &errOut); code != 2 {
			t.Errorf("gate %q exits %d, want 2", args, code)
		}
	}
}

// What it costs when this fails: a name that api/marks.txt calls stable has a doc that says Experimental, and no gate
// says so, since apigate holds only that the name exists. The plant is the row of the table that is red.
func TestAMarkedNameWhoseDocSaysExperimentalIsRefused(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		doc  string
		red  bool
	}{
		{"a stable doc", "func Run()\n    Run starts it.\n", false},
		{"a doc that says Experimental", "func Run()\n    Experimental: Run may change.\n", true},
	} {
		if got := experimental(c.doc); got != c.red {
			t.Errorf("%s: experimental is %v, want %v", c.name, got, c.red)
		}
	}
}

// What it costs when this fails: apigate's tab form is read as the old space form, so a marked name is not found or a
// bad line passes.
func TestMarksAreTheLinesOfThreeTabbedFields(t *testing.T) {
	t.Parallel()
	rows, err := marked("# note\n\nui\tRun\tstable\n")
	if err != nil || len(rows) != 1 || rows[0] != [2]string{"ui", "Run"} {
		t.Errorf("rows %v, err %v", rows, err)
	}
	if _, err := marked("ui Run\n"); err == nil {
		t.Error("a line of spaces is no row, and was taken as one")
	}
}

// What it costs when this fails: a gate says it refuses and exits 0, so task size, or a tier that has a refusing
// member, is green over the ceiling. The red lines are the two runs below, on this tree, which has more than one line.
func TestARefusalExitsRedAndAFailedMemberHoldsTheTierBack(t *testing.T) {
	t.Chdir("../..")
	var out, errOut bytes.Buffer
	if code := run(t.Context(), []string{"size", "1"}, &out, &errOut); code != 1 {
		t.Errorf("size over its ceiling exits %d, want 1: %s%s", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(t.Context(), []string{"tier", "release", "size:1"}, &out, &errOut); code != 1 || strings.Contains(out.String(), "budget") {
		t.Errorf("a tier with a refusing member exits %d with %q, want 1 and no budget line", code, out.String())
	}
}

// What it costs when this fails: hatch runs `gate members` under --cpu and reads its exit code, so a members run that
// exits 0 over a failed member passes the pre-push tier red, and one that is refused as usage never runs the tier.
func TestMembersExitsRedOnAFailedMemberAndRunsNoBudget(t *testing.T) {
	t.Chdir("../..")
	var out, errOut bytes.Buffer
	if code := run(t.Context(), []string{"members", "size:1"}, &out, &errOut); code != 1 || strings.Contains(out.String(), "budget") {
		t.Errorf("a members run with a refusing member exits %d with %q, want 1 and no budget line", code, out.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(t.Context(), []string{"members", "size:99999999"}, &out, &errOut); code != 0 {
		t.Errorf("a members run that passes exits %d, want 0: %s%s", code, out.String(), errOut.String())
	}
	if code := run(t.Context(), []string{"members"}, &out, &errOut); code != 2 {
		t.Errorf("members with no member exits %d, want 2", code)
	}
}
