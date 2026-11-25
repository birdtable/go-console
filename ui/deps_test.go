package ui

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// A program that links ui does not link the standard testing package.
//
// What it costs when this fails: every program using Follow carries testing,
// which a program's own test that its binary does not link it fails on.
func TestUILinksNoTestingPackage(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("the go tool lists the dependencies, and this test needs it (install Go, or put it on the PATH): %v", err)
	}
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "-f", "{{.ImportPath}}", ".").Output()
	if err != nil {
		var stderr []byte
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("go list: %v\n%s", err, stderr)
	}
	deps := strings.Fields(string(out))
	if len(deps) < 50 {
		t.Fatalf("only %d dependencies listed", len(deps))
	}
	if slices.Contains(deps, "testing") {
		t.Errorf("ui links testing")
	}
}

// Of the packages that are ours, ui links go-run/exit and no other: the look reads an ending by
// exit's words and codes and takes nothing else of go-run.
//
// What it costs when this fails: a program that takes the look also links a package of go-run it
// never asked for, and the look can no longer be read apart from the rest.
func TestUILinksOfOursOnlyTheExitPackage(t *testing.T) { //nolint:paralleltest // serial: it runs the go tool, as its neighbour does.
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "-f", "{{.ImportPath}}", ".").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var ours []string
	for p := range strings.FieldsSeq(string(out)) {
		if strings.HasPrefix(p, "github.com/birdtable/") && !strings.HasPrefix(p, "github.com/birdtable/go-console/") {
			ours = append(ours, p)
		}
	}
	if want := []string{"github.com/birdtable/go-run/exit"}; !slices.Equal(ours, want) {
		t.Errorf("ui links %v of ours, want %v", ours, want)
	}
}
