package ui

// The signals Execute listens for, and the name a program reads the stop by.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// A SignalError is the cause Execute cancels its commands' context with when a signal it listens for stops the run: context.Cause(ctx) is one, errors.As and errors.AsType find it, and Signal is the signal that came. It unwraps to
// context.Canceled, so errors.Is(err, context.Canceled) holds for it as it does for a run that returned.
type SignalError struct{ Signal os.Signal }

func (e *SignalError) Error() string { return fmt.Sprintf("%v signal received", e.Signal) }

// Unwrap is context.Canceled.
func (e *SignalError) Unwrap() error { return context.Canceled }

// Stopped says whether a signal Execute listens for has cancelled ctx, the context of a command Execute runs or one
// made from it, and which signal. It is false while the run goes on, and false when ctx ended another way: the run
// returned, the caller cancelled the context it gave Execute, or a deadline passed.
func Stopped(ctx context.Context) (os.Signal, bool) {
	if e, ok := errors.AsType[*SignalError](context.Cause(ctx)); ok {
		return e.Signal, true
	}
	return nil, false
}

// notifySignals is the context the tree runs under: os.Interrupt or SIGTERM cancels it with a SignalError, once.
// stop cancels it with no cause of its own, so the cause of a run that returned is context.Canceled, and ends the
// listening. It is what fang v2.0.1's WithNotifySignal did through signal.NotifyContext, with the cause exported:
// the first signal is taken, and a later one before stop is held unread, so it neither ends the process nor
// changes the cause.
func notifySignals(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-ch:
			cancel(&SignalError{Signal: sig})
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		cancel(nil)
		signal.Stop(ch)
	}
}
