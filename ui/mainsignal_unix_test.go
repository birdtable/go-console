//go:build unix

package ui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// A command that sleeps until its context ends, ends on the failed ending with a non-zero code
// when the process gets SIGTERM, which is what Ctrl+C and a service manager's stop both come to.
//
// What it costs when this fails: a stopped program is killed by the signal's default with no
// ending and no document, or sleeps on past the stop.
func TestSIGTERMStopsTheRunOnTheFailedEnding(t *testing.T) { //nolint:paralleltest // serial: it signals the process and sets its streams.
	// serial: it signals the process and sets its streams.
	started := make(chan struct{})
	code, _, _ := mainRun(t, oneCommand(func(_ *Session, c *cobra.Command) error {
		go func() {
			<-started
			_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		}()
		close(started)
		<-c.Context().Done()
		return c.Context().Err()
	}))
	if code == 0 {
		t.Errorf("a run stopped by SIGTERM returned %d, want a non-zero code", code)
	}
}

// Main cancels the context its command runs under on SIGINT (Ctrl+C) and on SIGTERM (the system's stop), and the
// program sees it: ctx.Done fires while the command is still running, before it returns, and the run ends with a
// non-zero code. The context is the one the command was handed, not a stand-in.
//
// What it costs when this fails: a program that waits on its context for a clean stop is never told to, and is killed
// with its work half done, or runs on past the stop.
func TestMainCancelsTheCommandsContextOnInterruptAndStop(t *testing.T) { //nolint:paralleltest // serial: it signals the process and sets its streams.
	// serial: it signals the process and sets its streams.
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		sawDone := false
		code, _, _ := mainRun(t, oneCommand(func(_ *Session, c *cobra.Command) error {
			ctx := c.Context()
			if ctx.Err() != nil {
				t.Errorf("%v: the context was done before the signal", sig)
			}
			_ = syscall.Kill(os.Getpid(), sig)
			select {
			case <-ctx.Done():
				sawDone = true
			case <-time.After(10 * time.Second):
			}
			return ctx.Err()
		}))
		if !sawDone {
			t.Errorf("%v: the command's context did not end within 10 s of the signal", sig)
		}
		if code == 0 {
			t.Errorf("%v: a run stopped by the signal returned %d, want a non-zero code", sig, code)
		}
	}
}

// Stopped answers true with the signal, SIGINT or SIGTERM, once that signal has ended the context of a running
// command, and false before it; the cause is a SignalError and is context.Canceled to errors.Is.
//
// What it costs when this fails: a program cannot tell a stop by a signal from a run that returned, by any name a
// linter lets it write, so it says nothing when the signal lands or says "stopping" on every clean end.
func TestStoppedNamesTheSignalThatStoppedTheRun(t *testing.T) { //nolint:paralleltest // serial: it signals the process and sets its streams.
	// serial: it signals the process and sets its streams.
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		code, _, _ := mainRun(t, oneCommand(func(_ *Session, c *cobra.Command) error {
			ctx := c.Context()
			if got, ok := Stopped(ctx); ok {
				t.Errorf("%v: Stopped = %v, true before the signal", sig, got)
			}
			_ = syscall.Kill(os.Getpid(), sig)
			<-ctx.Done()
			if got, ok := Stopped(ctx); !ok || got != sig {
				t.Errorf("%v: Stopped = %v, %v after the signal, want it and true", sig, got, ok)
			}
			var se *SignalError
			if cause := context.Cause(ctx); !errors.As(cause, &se) || !errors.Is(cause, context.Canceled) {
				t.Errorf("%v: the cause is %v, want a SignalError that is context.Canceled", sig, cause)
			}
			return ctx.Err()
		}))
		if code == 0 {
			t.Errorf("%v: a run stopped by the signal returned %d, want a non-zero code", sig, code)
		}
	}
}

// A second signal, while the command the first one stopped is still running, does not end the process and does not
// change what Stopped answers: it is held unread until Execute returns, as under fang v2.0.1's WithNotifySignal
// (signal.NotifyContext), which Execute replaced.
//
// What it costs when this fails: a second Ctrl+C or a stop sent twice kills a program in the middle of the clean
// stop the first one asked for.
func TestASecondSignalNeitherEndsTheProcessNorChangesTheCause(t *testing.T) { //nolint:paralleltest // serial: it signals the process and sets its streams.
	// serial: it signals the process and sets its streams.
	code, _, _ := mainRun(t, oneCommand(func(_ *Session, c *cobra.Command) error {
		ctx := c.Context()
		// The row's own registration for SIGTERM, held until both signals have arrived: a signal is delivered
		// after kill returns, and one that lands once nothing is registered takes the default action and ends
		// the test binary. The signals go one at a time, each received before the next, since the runtime
		// merges two that are pending at once. No clock: a signal that never arrives is the test's deadline.
		own := make(chan os.Signal, 2)
		signal.Notify(own, syscall.SIGTERM)
		defer signal.Stop(own)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
		<-ctx.Done()
		for range 2 {
			_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
			<-own
		}
		if got, ok := Stopped(ctx); !ok || got != syscall.SIGINT {
			t.Errorf("after a second and a third signal Stopped = %v, %v, want the first, SIGINT, and true", got, ok)
		}
		return ctx.Err()
	}))
	if code == 0 {
		t.Errorf("a run stopped by a signal returned %d, want a non-zero code", code)
	}
}

// The listening ends when Execute returns: once the stop notifySignals hands back has run, the process is no longer
// registered for SIGINT, so one that comes after the run is the system's default and ends the process, as it did
// before Execute listened at all. The row runs a child of the test binary that listens, stops, then signals itself,
// and the parent reads how the child ended.
//
// What it costs when this fails: a program that returned from Execute and goes on, or runs Execute again, has Ctrl+C
// swallowed for good by a listener nobody reads, and cannot be interrupted.
func TestTheListenerStopsWhenExecuteReturns(t *testing.T) { //nolint:paralleltest // serial: it runs a child that signals itself.
	if slices.Contains(os.Args, "listener-child") { // testkit clears the environment, so the child is named by an argument.
		_, stop := notifySignals(context.Background())
		stop()
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
		time.Sleep(2 * time.Second)
		return // reached only when the signal was swallowed: the child ends well, and the parent reads that as the failure.
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTheListenerStopsWhenExecuteReturns$", "listener-child") //nolint:gosec // the test binary re-running itself, with a fixed argument list.
	err := cmd.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("the child ended with %v, want it killed by SIGINT after the stop: the listener still held the signal", err)
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); !ok || !ws.Signaled() || ws.Signal() != syscall.SIGINT {
		t.Errorf("the child ended with %v, want it killed by SIGINT after the stop", ee)
	}
}

// The listener holds the second and third signal until the run ends: once the first has cancelled the context, the
// listener is still registered, so a SIGTERM that follows has a handler and does not take the system's default. The
// row runs a child of the test binary that listens, takes the first signal, sends two more and ends without ever
// calling the stop, so nothing it sends can come in late; the parent reads that the child ended well.
//
// What it costs when this fails: a listener that lets go before it cancels, or at the first signal, leaves a second
// Ctrl+C or a second stop with no handler, and it kills the program in the middle of the clean stop the first asked
// for. The row above holds its own registration, so it cannot see this.
func TestTheListenerStillHoldsTheSecondSignal(t *testing.T) { //nolint:paralleltest // serial: it runs a child that signals itself.
	if slices.Contains(os.Args, "holder-child") { // testkit clears the environment, so the child is named by an argument.
		ctx, _ := notifySignals(context.Background()) // the stop is left uncalled on purpose: the listener stays registered to the end.
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
		<-ctx.Done()
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTheListenerStillHoldsTheSecondSignal$", "holder-child") //nolint:gosec // the test binary re-running itself, with a fixed argument list.
	if err := cmd.Run(); err != nil {
		t.Errorf("the child ended with %v, want it to end well: a second signal found no handler", err)
	}
}
