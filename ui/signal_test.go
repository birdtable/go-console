package ui

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// executeQuiet runs one command under Execute on ctx, with nothing printed, and returns the context the command got.
func executeQuiet(ctx context.Context, fn func(context.Context)) context.Context {
	var got context.Context
	root := &cobra.Command{Use: "app", SilenceUsage: true, SilenceErrors: true}
	root.RunE = func(c *cobra.Command, _ []string) error {
		got = c.Context()
		fn(got)
		return nil
	}
	root.SetArgs(nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	_ = Execute(ctx, root, New(append(House(), On(Terminal{}), Out(io.Discard), Err(io.Discard))...), testBrand())
	return got
}

// Stopped is false for every end of the context that is not a signal: while the command runs, after the run
// returned, when the caller cancelled the context it gave Execute, and when that context's deadline passed. A run
// that returned leaves context.Canceled as the cause, as it did.
//
// What it costs when this fails: a program that says "stopping on a signal" when Stopped is true says it at the
// end of every clean run, or when its own caller gave up.
func TestStoppedIsFalseWhenNoSignalEndedTheContext(t *testing.T) {
	t.Parallel()
	returned := executeQuiet(context.Background(), func(ctx context.Context) {
		if sig, ok := Stopped(ctx); ok || sig != nil || ctx.Err() != nil {
			t.Errorf("while the command runs: Stopped = %v, %v and Err = %v, want nil, false, nil", sig, ok, ctx.Err())
		}
	})
	if sig, ok := Stopped(returned); ok || sig != nil {
		t.Errorf("after a return: Stopped = %v, %v, want nil, false", sig, ok)
	}
	if cause := context.Cause(returned); !errors.Is(cause, context.Canceled) || returned.Err() == nil {
		t.Errorf("after a return: the cause is %v and Err %v, want context.Canceled", cause, returned.Err())
	}

	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	own := executeQuiet(parent, func(ctx context.Context) {
		cancel()
		<-ctx.Done()
		if sig, ok := Stopped(ctx); ok || sig != nil || !errors.Is(context.Cause(ctx), context.Canceled) {
			t.Errorf("the caller's own cancel: Stopped = %v, %v, cause %v, want nil, false, context.Canceled", sig, ok, context.Cause(ctx))
		}
	})
	if _, ok := Stopped(own); ok {
		t.Error("the caller's own cancel, after the return: Stopped is true")
	}

	late, stop := context.WithDeadline(context.Background(), time.Time{})
	defer stop()
	executeQuiet(late, func(ctx context.Context) {
		<-ctx.Done()
		if sig, ok := Stopped(ctx); ok || sig != nil || !errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
			t.Errorf("a passed deadline: Stopped = %v, %v, cause %v, want nil, false, context.DeadlineExceeded", sig, ok, context.Cause(ctx))
		}
	})
}

// A SignalError is found by errors.As through a wrap, names its signal, and is context.Canceled to errors.Is, and
// Stopped reads it from a context cancelled with one and from a child of that context, on every system.
//
// What it costs when this fails: a caller's errors.Is(err, context.Canceled), which held before the cause had a
// name, stops holding for a run a signal ended.
func TestSignalErrorIsCanceledAndStoppedReadsItFromAChild(t *testing.T) {
	t.Parallel()
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		err := error(&SignalError{Signal: sig})
		if !errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), sig.String()) {
			t.Errorf("%v: the error %q is not context.Canceled, or does not name the signal", sig, err)
		}
		ctx, cancel := context.WithCancelCause(context.Background())
		child, done := context.WithCancel(ctx)
		if _, ok := Stopped(child); ok {
			t.Errorf("%v: Stopped is true before the cancel", sig)
		}
		cancel(err)
		<-child.Done()
		for name, c := range map[string]context.Context{"the context": ctx, "its child": child} {
			if got, ok := Stopped(c); !ok || got != sig {
				t.Errorf("%v, %s: Stopped = %v, %v, want the signal and true", sig, name, got, ok)
			}
		}
		done()
	}
	if got := (&SignalError{}).Error(); got == "" {
		t.Error("a SignalError with no signal has no text")
	}
}
