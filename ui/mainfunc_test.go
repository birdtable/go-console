package ui

import (
	"context"
	"os"
	"reflect"
	"testing"
)

// A plain function that returns 0, 3 or 130, on a context that ends or on one that does not, ends Main on that code.
//
// What it costs when this fails: a program of one function exits with the kit's failed ending, 1, in place of the code
// its function returned, so a script that branches on 3 or 130 reads another outcome.
func TestMainFuncKeepsTheCodeTheFunctionReturns(t *testing.T) { //nolint:paralleltest // serial: it sets the process's streams.
	for _, want := range []int{0, 3, 130} {
		for _, ends := range []bool{false, true} {
			ctx, cancel := context.WithCancel(context.Background())
			if ends {
				cancel()
			}
			var saw bool
			code := 0
			capture(t, &os.Stdout, func() {
				capture(t, &os.Stderr, func() {
					code = mainFunc(ctx, nil, "tool", func(c context.Context, _ []string) int {
						saw = ends == (c.Err() != nil)
						return want
					}, testBrand(), WithSession(House()...))
				})
			})
			cancel()
			if code != want || !saw {
				t.Errorf("function returned %d, context ended %v: Main returned %d, context seen as given %v", want, ends, code, saw)
			}
		}
	}
}

// Every argument reaches the function as typed, `--help`, `-h` and `--` among them.
//
// What it costs when this fails: the kit takes a flag a program's own parser should see.
func TestMainFuncHandsEveryArgumentToTheFunctionUnparsed(t *testing.T) { //nolint:paralleltest // serial: it sets the process's streams.
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--", "x"}, {"a", "--version", "--json", "-x=1"}, {"__complete", "x"}, {"__completeNoDesc", "x"}, {"help"}, {"completion"}, {}} {
		var got []string
		capture(t, &os.Stdout, func() {
			capture(t, &os.Stderr, func() {
				mainFunc(context.Background(), args, "tool", func(_ context.Context, a []string) int { got = a; return 0 }, testBrand(), WithSession(House()...))
			})
		})
		if len(args) == 0 && len(got) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, args) {
			t.Errorf("args %q reached the function as %q", args, got)
		}
	}
}
