package ui

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"testing/synctest"
)

// tripReader tells when a read has started and counts the reads running at once.
type tripReader struct {
	r       io.Reader
	started chan struct{}
	now     atomic.Int32
	most    atomic.Int32
}

func (t *tripReader) Read(b []byte) (int, error) {
	n := t.now.Add(1)
	defer t.now.Add(-1)
	if n > t.most.Load() {
		t.most.Store(n)
	}
	select {
	case t.started <- struct{}{}:
	default:
	}
	return t.r.Read(b)
}

// A call ends with its context on an input that is never written to, the line written after it is the next
// call's answer, at most one read is pending through many cancelled calls, and the read's goroutine ends when
// the input closes (the bubble ends only when every goroutine in it has, so a goroutine left behind is a
// deadlock there).
//
// What it costs when this fails: a prompt on piped input ignores its cancel, or loses a line typed after it, or
// leaves a goroutine behind for every prompt.
func TestLinesEndWithTheirContextAndKeepTheLineForTheNextCall(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		pr, pw := io.Pipe()
		in := &tripReader{r: pr, started: make(chan struct{}, 1)}
		l := NewLines(in, 100)
		for i := range 3 {
			ctx, cancel := context.WithCancel(t.Context())
			if i == 0 {
				go func() { <-in.started; cancel() }() // the first call starts the read, and ends once it has
			} else {
				cancel() // the read is still pending: a call under an ended context starts no second one
			}
			if line, err := l.Next(ctx); !errors.Is(err, context.Canceled) || line != "" {
				t.Fatalf("call %d: got %q, %v, want the context's error", i, line, err)
			}
		}
		go func() { _, _ = io.WriteString(pw, "after the cancel\nsecond") }()
		if line, err := l.Next(t.Context()); err != nil || line != "after the cancel" {
			t.Fatalf("the line written after the cancels is %q, %v, want it whole", line, err)
		}
		go func() { _ = pw.Close() }()
		if line, err := l.Next(t.Context()); err != nil || line != "second" {
			t.Fatalf("the last line, with no ending, is %q, %v", line, err)
		}
		if _, err := l.Next(t.Context()); !errors.Is(err, io.EOF) {
			t.Fatalf("after the last line: %v, want io.EOF", err)
		}
		if m := in.most.Load(); m != 1 {
			t.Errorf("%d reads ran at once, want 1", m)
		}
	})
}

// endless is an input of x for ever that counts what is read from it.
type endless struct{ n atomic.Int64 }

func (e *endless) Read(b []byte) (int, error) {
	for i := range b {
		b[i] = 'x'
	}
	e.n.Add(int64(len(b)))
	return len(b), nil
}

// A line that passes the limit is refused at once, without reading the rest of it, and the Lines is closed:
// on an input that sends x for ever the call still returns, no more than the limit and one read has been taken
// from the input, and every call after it gets the same refusal and starts no read.
//
// What it costs when this fails: a caller that caps a line to bound its memory is still held for as long as the
// input sends no newline.
func TestLinesRefuseAtTheLimitWithoutReadingTheRestAndClose(t *testing.T) {
	t.Parallel()
	in := &endless{}
	l := NewLines(in, 8)
	line, err := l.Next(t.Context())
	if err == nil || line != "" {
		t.Fatalf("an endless line at a limit of 8 gave %q, %v, want a refusal", line, err)
	}
	took := in.n.Load()
	if limit := int64(8 + 2*4096); took > limit {
		t.Errorf("%d bytes were read from the input, want at most %d (the limit and one read)", took, limit)
	}
	for i := range 2 {
		if _, again := l.Next(t.Context()); again == nil || again.Error() != err.Error() {
			t.Errorf("call %d after the refusal gave %v, want %v", i, again, err)
		}
	}
	if more := in.n.Load(); more != took {
		t.Errorf("%d bytes were read after the refusal, want none", more-took)
	}
}

// The refusal is an error wrapping ErrLineTooLong with the limit in its text, and the input's own failure is not.
//
// What it costs when this fails: a caller cannot tell a line that was too long from a read that failed, and ends
// its guard on the wrong one.
func TestLinesRefusalIsTyped(t *testing.T) {
	t.Parallel()
	_, err := NewLines(strings.NewReader(strings.Repeat("x", 10)+"\nok\n"), 8).Next(t.Context())
	if !errors.Is(err, ErrLineTooLong) || !strings.Contains(err.Error(), "8") {
		t.Errorf("a line of 10 bytes at a limit of 8 gave %v, want one wrapping ErrLineTooLong that names 8", err)
	}
	broken := errors.New("the input broke")
	_, err = NewLines(iotest.ErrReader(broken), 8).Next(t.Context())
	if !errors.Is(err, broken) || errors.Is(err, ErrLineTooLong) {
		t.Errorf("a failing input gave %v, want its own error and not ErrLineTooLong", err)
	}
}

// A last line of the limit's length that ends the input with a carriage return and no line feed is answered
// with the carriage return cut, as a line that ends in \r\n is.
//
// What it costs when this fails: the last line of a pipe that ends in \r is one byte longer than it was sent,
// and a line of exactly the limit is refused.
func TestLinesCutALastLoneCarriageReturn(t *testing.T) {
	t.Parallel()
	l := NewLines(strings.NewReader(strings.Repeat("x", 8)+"\r"), 8)
	if line, err := l.Next(t.Context()); err != nil || line != strings.Repeat("x", 8) {
		t.Fatalf("a last line of 8 bytes and a \\r gave %q, %v, want the 8 bytes", line, err)
	}
	if _, err := l.Next(t.Context()); !errors.Is(err, io.EOF) {
		t.Errorf("after the last line: %v, want io.EOF", err)
	}
	l = NewLines(strings.NewReader("a\r\nb\r"), 8)
	for _, want := range []string{"a", "b"} {
		if line, err := l.Next(t.Context()); err != nil || line != want {
			t.Errorf("got %q, %v, want %q", line, err, want)
		}
	}
}

// Refusing a line one byte past a large limit holds it in no more than the limit and one read, with no
// doubling past it.
//
// What it costs when this fails: a refusal at a 4 MiB limit allocates a multiple of it (21 MB was measured), so a
// cap meant to bound memory does not.
func TestLinesGrowNoFurtherThanTheLimitAndOneRead(t *testing.T) { //nolint:paralleltest // serial: it reads the process's allocation total, which a test running beside it would move
	const most = 1 << 20
	in := strings.NewReader(strings.Repeat("x", most+1))
	l := NewLines(in, most)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := l.Next(t.Context())
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrLineTooLong) {
		t.Fatalf("a line one byte over the limit gave %v, want a refusal", err)
	}
	if got := after.TotalAlloc - before.TotalAlloc; got > 2*most {
		t.Errorf("the refusal allocated %d bytes at a limit of %d, want at most twice it", got, most)
	}
}

// A context that has ended is answered before any read starts.
//
// What it costs when this fails: a call under a cancelled context starts a read it cannot use.
func TestLinesAnswerAnEndedContextBeforeAnyRead(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := NewLines(strings.NewReader("a\n"), 8).Next(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("an ended context gave %v, want its error", err)
	}
}

// The same on an os.Pipe, which on Windows is a synchronous handle that takes no deadline and whose pending
// read cannot be cancelled: the case the context read is for. It runs on every system.
//
// What it costs when this fails: on Windows a prompt reading piped input ignores its cancel.
func TestLinesEndWithTheirContextOnAnOSPipe(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close(); _ = r.Close() })
	in := &tripReader{r: r, started: make(chan struct{}, 1)}
	l := NewLines(in, 100)
	ctx, cancel := context.WithCancel(t.Context())
	go func() { <-in.started; cancel() }()
	if line, err := l.Next(ctx); !errors.Is(err, context.Canceled) || line != "" {
		t.Fatalf("got %q, %v, want the context's error", line, err)
	}
	if _, err := io.WriteString(w, "typed after\n"); err != nil {
		t.Fatal(err)
	}
	if line, err := l.Next(t.Context()); err != nil || line != "typed after" {
		t.Fatalf("the next call got %q, %v, want the line written after the cancel", line, err)
	}
}

// After a refusal at the limit, Discard reads on from where the refusal stopped: a writer with 1 MiB still to
// write into an OS pipe finishes without error, Discard answers nil at the end of the input, and Next goes on
// answering the refusal. It runs on every system.
//
// What it costs when this fails: a program that refuses a long line and stops reading leaves whoever feeds it
// blocked on a full pipe for good.
func TestLinesDiscardLetsAWriterFinishAfterARefusal(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close(); _ = r.Close() })
	wrote := make(chan error, 1)
	go func() {
		_, err := w.Write([]byte(strings.Repeat("x", 1<<20)))
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		wrote <- err
	}()
	l := NewLines(r, 16)
	_, refusal := l.Next(t.Context())
	if !errors.Is(refusal, ErrLineTooLong) {
		t.Fatalf("a line over the limit gave %v, want a refusal", refusal)
	}
	if err := l.Discard(t.Context()); err != nil {
		t.Fatalf("Discard gave %v, want nil at the end of the input", err)
	}
	if err := <-wrote; err != nil {
		t.Errorf("the writer ended on %v, want its 1 MiB written", err)
	}
	if _, err := l.Next(t.Context()); !errors.Is(err, ErrLineTooLong) || err.Error() != refusal.Error() {
		t.Errorf("Next after Discard gave %v, want the refusal %v", err, refusal)
	}
	if err := l.Discard(t.Context()); err != nil {
		t.Errorf("a Discard after the end gave %v, want nil", err)
	}
}

// On an OS pipe that sends nothing, Discard ends with its context and its read goes on; a Discard after the
// writer closes joins that read (the same one, never two at once) and answers nil. A healthy Lines answers
// io.EOF to Next from the first Discard on, also while the input is still open.
//
// What it costs when this fails: on Windows a discard ignores its cancel, or every cancelled discard leaves one
// more goroutine reading the same input.
func TestLinesDiscardEndsWithItsContextAndIsJoinedByTheNext(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close(); _ = r.Close() })
	in := &tripReader{r: r, started: make(chan struct{}, 1)}
	l := NewLines(in, 100)
	ctx, cancel := context.WithCancel(t.Context())
	go func() { <-in.started; cancel() }()
	if err := l.Discard(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Discard on a silent pipe gave %v, want the context's error", err)
	}
	first := l.done
	if err := l.Discard(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("a second Discard under the ended context gave %v, want the context's error", err)
	}
	if line, err := l.Next(t.Context()); !errors.Is(err, io.EOF) || line != "" {
		t.Errorf("Next after Discard, the input still open, gave %q, %v, want io.EOF at once", line, err)
	}
	if _, err := io.WriteString(w, "never seen\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := l.Discard(t.Context()); err != nil {
		t.Fatalf("Discard after the writer closed gave %v, want nil", err)
	}
	if l.done != first || in.most.Load() != 1 {
		t.Errorf("a second read was started: %d reads ran at once, and the discard's channel changed: %v", in.most.Load(), l.done != first)
	}
}

// A Next that ended with its context leaves a read pending; Discard waits for it, drops its line, reads the rest
// away and answers nil. The row runs under -race in the release tier: two reads on the one reader would be seen.
//
// What it costs when this fails: a discard after a cancelled prompt reads the input beside the prompt's own
// read, and the two tear each other's buffer.
func TestLinesDiscardWaitsForAPendingLineAndDropsIt(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close(); _ = r.Close() })
	in := &tripReader{r: r, started: make(chan struct{}, 1)}
	l := NewLines(in, 100)
	ctx, cancel := context.WithCancel(t.Context())
	go func() { <-in.started; cancel() }()
	if _, err := l.Next(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Next on a silent pipe gave %v, want the context's error", err)
	}
	go func() {
		_, _ = io.WriteString(w, "the pending line\n"+strings.Repeat("more\n", 1<<16))
		_ = w.Close()
	}()
	if err := l.Discard(t.Context()); err != nil {
		t.Fatalf("Discard gave %v, want nil", err)
	}
	if line, err := l.Next(t.Context()); !errors.Is(err, io.EOF) || line != "" {
		t.Errorf("Next after Discard gave %q, %v, want io.EOF and no line", line, err)
	}
	if in.most.Load() != 1 {
		t.Errorf("%d reads ran at once, want one", in.most.Load())
	}
}

// failAfter gives n bytes of x and then err.
type failAfter struct {
	n   int
	err error
}

func (f *failAfter) Read(b []byte) (int, error) {
	if f.n <= 0 {
		return 0, f.err
	}
	n := min(len(b), f.n)
	for i := range b[:n] {
		b[i] = 'x'
	}
	f.n -= n
	return n, nil
}

// Discard answers the input's own read error as it is, an ended context before any read is started, and an
// input that already ended before the context is looked at.
//
// What it costs when this fails: a broken input reads as a clean end, or a discard under a cancelled context
// starts a read nobody waits for.
func TestLinesDiscardAnswersTheReadErrorTheEndedContextAndTheEndedInput(t *testing.T) {
	t.Parallel()
	errBroken := errors.New("the input broke")
	if err := NewLines(&failAfter{n: 10000, err: errBroken}, 8).Discard(t.Context()); err != errBroken { //nolint:errorlint // the row is that it is returned as it is
		t.Errorf("Discard on an input that breaks gave %v, want the input's own error", err)
	}
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	in := &tripReader{r: strings.NewReader("a\nb\n"), started: make(chan struct{}, 1)}
	l := NewLines(in, 8)
	if err := l.Discard(ended); !errors.Is(err, context.Canceled) || l.done != nil || in.most.Load() != 0 {
		t.Errorf("Discard under an ended context gave %v after %d reads, want its error and no read", err, in.most.Load())
	}
	if _, err := l.Next(t.Context()); !errors.Is(err, io.EOF) {
		t.Errorf("Next after that Discard gave %v, want io.EOF", err)
	}
	done := NewLines(strings.NewReader("a"), 8)
	if line, err := done.Next(t.Context()); err != nil || line != "a" {
		t.Fatalf("got %q, %v, want the one line", line, err)
	}
	if _, err := done.Next(t.Context()); !errors.Is(err, io.EOF) {
		t.Fatalf("got %v, want io.EOF", err)
	}
	if err := done.Discard(ended); err != nil {
		t.Errorf("Discard on an input that already ended, under an ended context, gave %v, want nil", err)
	}
}

// Discarding 64 MiB allocates a fixed, small amount: the bytes go through the Lines' own buffer and nowhere else.
//
// What it costs when this fails: reading away what a program refused to hold costs the memory the refusal saved.
func TestLinesDiscardAllocatesNothingThatGrowsWithTheInput(t *testing.T) { //nolint:paralleltest // serial: it reads the process's allocation total, which a test running beside it would move
	l := NewLines(&failAfter{n: 64 << 20, err: io.EOF}, 16)
	if _, err := l.Next(t.Context()); !errors.Is(err, ErrLineTooLong) {
		t.Fatalf("a line over the limit gave %v, want a refusal", err)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	err := l.Discard(t.Context())
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("Discard gave %v, want nil", err)
	}
	if got := after.TotalAlloc - before.TotalAlloc; got > 64<<10 {
		t.Errorf("discarding 64 MiB allocated %d bytes, want at most 64 KiB", got)
	}
}
