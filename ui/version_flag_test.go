package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// versionLine and versionDocument are what a program hands ui for --version: the two values its own
// version package makes, so ui imports none of it.
const versionLine = "v0.0.0-712a01e go1.27.4 untagged"

func versionDocument() map[string]any {
	return map[string]any{"version": "v0.0.0-712a01e", "commit": "712a01e", "toolchain": "go1.27.4", "untagged": true}
}

// versionRun runs a program that hands ui a version line and document, with args, and answers what it
// printed and what Execute returned. Its --json is the one Execute gives every program.
func versionRun(t *testing.T, opts []Option, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := &cobra.Command{Use: "app", RunE: func(c *cobra.Command, _ []string) error { c.Println("ran"); return nil }}
	root.SetOut(&out)
	root.SetArgs(args)
	err := Execute(context.Background(), root, runner(), testBrand(), opts...)
	return out.String(), err
}

// --version prints the one line the program hands in, as it is: no program name and no word
// "version" before it, the line a person reads and a script records.
// Planted failure: cobra's own "app version ..." is printed.
func TestVersionPrintsTheLineTheProgramHandsIn(t *testing.T) {
	t.Parallel()
	got, err := versionRun(t, []Option{WithVersionLine(versionLine), WithVersionDocument(versionDocument())}, "--version")
	if err != nil || got != versionLine+"\n" {
		t.Errorf("--version printed %q (err %v), want %q", got, err, versionLine+"\n")
	}
}

// With --json, the document the program hands in is printed instead, one JSON document with the keys
// it was given, and nothing else.
// Planted failure: the line is printed under --json, or the document is dropped.
func TestVersionWithJSONPrintsTheDocumentTheProgramHandsIn(t *testing.T) {
	t.Parallel()
	got, err := versionRun(t, []Option{WithVersionLine(versionLine), WithVersionDocument(versionDocument())}, "--version", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if jerr := json.Unmarshal([]byte(got), &doc); jerr != nil {
		t.Fatalf("--version --json printed %q, which is not one JSON document: %v", got, jerr)
	}
	if !reflect.DeepEqual(doc, versionDocument()) {
		t.Errorf("the document is %v, want %v", doc, versionDocument())
	}
	if strings.Contains(got, "go1.27.4 untagged") {
		t.Errorf("the line was printed beside the document: %q", got)
	}
}

// A program that gives no document prints its line under --json too, and one that gives no line
// still prints its document: each option stands alone.
func TestEachVersionOptionStandsAlone(t *testing.T) {
	t.Parallel()
	got, err := versionRun(t, []Option{WithVersionLine(versionLine)}, "--version", "--json")
	if err != nil || got != versionLine+"\n" {
		t.Errorf("a line and no document: printed %q (err %v), want the line", got, err)
	}
	got, err = versionRun(t, []Option{WithVersionDocument(versionDocument())}, "--version", "--json")
	if err != nil || !strings.Contains(got, `"toolchain": "go1.27.4"`) {
		t.Errorf("a document and no line: printed %q (err %v), want the document", got, err)
	}
}

// The flag leaves the program's own commands alone: without --version it runs, and a program that
// turns the flag off gets neither the line nor the document.
func TestTheVersionOptionsLeaveACommandAloneAndAreIgnoredWithoutTheFlag(t *testing.T) {
	t.Parallel()
	got, err := versionRun(t, []Option{WithVersionLine(versionLine), WithVersionDocument(versionDocument())})
	if err != nil || !strings.Contains(got, "ran") || strings.Contains(got, "go1.27.4") {
		t.Errorf("without --version: printed %q (err %v)", got, err)
	}
	_, err = versionRun(t, []Option{WithVersionLine(versionLine), WithoutVersionFlag()}, "--version")
	if err == nil {
		t.Error("--version with the flag off was taken")
	}
}

// A document that cannot be written as JSON is the program's mistake, and Execute says so before it
// runs anything.
func TestAVersionDocumentThatCannotBeWrittenIsAnError(t *testing.T) {
	t.Parallel()
	got, err := versionRun(t, []Option{WithVersionDocument(map[string]any{"bad": make(chan int)})}, "--version")
	if err == nil || !strings.Contains(err.Error(), "version document") || got != "" {
		t.Errorf("printed %q, err %v, want an error naming the version document and no output", got, err)
	}
}

// A run without --json prints the program's line and never its document, though the flag is there:
// Execute gives every program one.
func TestAProgramWithNoJSONFlagStillPrintsItsLineAndNeverItsDocument(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	root := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { return nil }}
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})
	err := Execute(context.Background(), root, runner(), testBrand(), WithVersionLine(versionLine), WithVersionDocument(versionDocument()))
	if err != nil || out.String() != versionLine+"\n" {
		t.Errorf("printed %q (err %v), want the line", out.String(), err)
	}
}

// A line or a document that holds template syntax prints as it was handed in, since cobra runs the
// version template and the line and the document reach it as data, never as template text. A line
// `v1 {{.Name}} {{` embedded straight in the template would be run, and fail or print the command's
// name (the review).
// Planted failure: the line is written into the template, and `--version` prints the command's name
// in place of `{{.Name}}`, or fails to parse.
func TestAVersionLineAndDocumentHoldingTemplateSyntaxPrintLiterally(t *testing.T) {
	t.Parallel()
	const line = "v1 {{.Name}} {{"
	doc := map[string]any{"version": "v1", "toolchain": "{{.Name}} {{"}
	opts := []Option{WithVersionLine(line), WithVersionDocument(doc)}
	got, err := versionRun(t, opts, "--version")
	if err != nil || got != line+"\n" {
		t.Errorf("--version printed %q (err %v), want %q", got, err, line+"\n")
	}
	got, err = versionRun(t, opts, "--version", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if jerr := json.Unmarshal([]byte(got), &back); jerr != nil {
		t.Fatalf("--version --json printed %q, which is not one JSON document: %v", got, jerr)
	}
	if !reflect.DeepEqual(back, doc) {
		t.Errorf("the document is %v, want %v", back, doc)
	}
}
