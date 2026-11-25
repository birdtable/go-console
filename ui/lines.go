package ui

// A line read under a context, on every system. Windows takes no read deadline on a
// pipe or a console, and a pending read there cannot be cancelled, so the read is not cancelled: one goroutine
// does it and the caller waits beside it, for the line or for its context. A read that outlasts its context
// keeps its line for the next call.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Lines reads one line at a time from an input, each call under a context. A call returns when a line is read
// or when its context ends, and the context's error is the answer then. The pending read is not cancelled,
// since an input that takes no deadline cannot be: it goes on, and the line it gets answers the next call, so
// no line is lost. At most one read is pending, and it is started only when a line is asked for. Its goroutine
// ends when its read returns, which for an input that is never written to is when the input is closed.
// Discard reads the rest of the input away, so that whoever writes it is not left blocked.
// A Lines is for one caller at a time.
type Lines struct {
	in      *bufio.Reader
	most    int
	mu      sync.Mutex // guards pending and err
	pending chan lineAnswer
	err     error // the error that ended the input, given to every call after it
	// discarded is set by the first Discard: no line is given after it. done is the one discard's read, closed
	// when it has read the input away, and result is what it ended on.
	discarded bool
	done      chan struct{}
	result    error
}

type lineAnswer struct {
	line string
	err  error
}

// ErrLineTooLong is what a Lines answers when a line is over its limit: the error wraps it, with the limit in its
// text, so errors.Is tells it from the input's own read failing.
var ErrLineTooLong = errors.New("line too long")

// NewLines reads lines of at most most bytes from in, with the line ending cut and nothing else. A line that
// passes most is refused as soon as it does, with an error wrapping ErrLineTooLong, and the rest of it is not
// read: the Lines is closed then, and every call after the refusal gets the same error. At the end of the input a
// last carriage return with no line feed after it is cut with the line ending. The line is held in no more than
// most bytes plus one read of the input.
func NewLines(in io.Reader, most int) *Lines {
	return &Lines{in: bufio.NewReader(in), most: most}
}

// Next is the next line of the input, without its line ending. When ctx ends first it returns ctx.Err() and the
// read goes on for the next call. At the end of the input it returns io.EOF, after a last line that has no line
// ending. A context that has ended is answered before a read is started, and a line already read is answered
// before the context is looked at.
func (l *Lines) Next(ctx context.Context) (string, error) {
	l.mu.Lock()
	if l.discarded {
		defer l.mu.Unlock()
		if errors.Is(l.err, ErrLineTooLong) {
			return "", l.err
		}
		return "", io.EOF
	}
	if l.pending == nil {
		if l.err != nil {
			defer l.mu.Unlock()
			return "", l.err
		}
		if err := ctx.Err(); err != nil {
			l.mu.Unlock()
			return "", err
		}
		l.pending = make(chan lineAnswer, 1)
		go l.read(l.pending)
	}
	pending := l.pending
	l.mu.Unlock()
	select {
	case a := <-pending:
		return l.take(pending, a)
	default:
	}
	select {
	case a := <-pending:
		return l.take(pending, a)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Discard reads the rest of the input away and keeps none of it, through the Lines' own buffer and no other. It
// returns nil when the input ends, the input's own error when a read fails, and ctx.Err() when ctx ends first:
// the read goes on then, as Next's does, and a later Discard joins it and starts no second one. It may be called
// in any state: after a refusal at the limit, where it reads on from where the refusal stopped, before any line,
// or between lines; a read an earlier Next left pending is waited for and its line dropped. Once it has been
// called the Lines gives no more lines: Next answers the refusal when there was one and io.EOF otherwise, at
// once, whether or not the input has been read away. An input that already ended is answered before ctx is
// looked at, and a ctx that has ended before a read is started.
func (l *Lines) Discard(ctx context.Context) error {
	l.mu.Lock()
	l.discarded = true
	if l.done == nil {
		if l.pending == nil && l.err != nil && !errors.Is(l.err, ErrLineTooLong) {
			defer l.mu.Unlock()
			return ended(l.err)
		}
		if err := ctx.Err(); err != nil {
			l.mu.Unlock()
			return err
		}
		l.done = make(chan struct{})
		go l.discard(l.pending, l.done)
	}
	done := l.done
	l.mu.Unlock()
	select {
	case <-done:
	default:
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.result
}

// discard is the one discard's read. It waits for a line read left pending, so two reads never run at once.
func (l *Lines) discard(pending <-chan lineAnswer, done chan<- struct{}) {
	if pending != nil {
		<-pending
	}
	l.mu.Lock()
	l.pending = nil
	err := l.err
	l.mu.Unlock()
	if err == nil || errors.Is(err, ErrLineTooLong) {
		for err = nil; err == nil; {
			_, err = l.in.Discard(l.in.Size())
		}
	}
	l.mu.Lock()
	l.result = ended(err)
	l.mu.Unlock()
	close(done)
}

// ended is err, or nil for the end of the input.
func ended(err error) error {
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// take is a finished read's answer: the read is no longer pending.
func (l *Lines) take(pending chan lineAnswer, a lineAnswer) (string, error) {
	l.mu.Lock()
	if l.pending == pending {
		l.pending = nil
	}
	l.mu.Unlock()
	return a.line, a.err
}

// read is the one pending read, a line to its end or to the limit. The channel holds its answer, so the goroutine
// never waits for a caller. A line is kept as the reads' own pieces, not in one slice that doubles as it grows,
// so a refusal has allocated about the limit and one read, and a line that fits is joined once.
func (l *Lines) read(out chan<- lineAnswer) {
	var pieces [][]byte
	n := 0
	for {
		part, more, err := l.in.ReadLine()
		if err != nil {
			l.mu.Lock()
			l.err = err
			l.mu.Unlock()
			if n > 0 {
				out <- lineAnswer{line: join(pieces, n)} // the last line has no ending; the call after it gets err
				return
			}
			out <- lineAnswer{err: err}
			return
		}
		if !more && len(part) > 0 && part[len(part)-1] == '\r' {
			part = part[:len(part)-1] // a line that ends the input with a carriage return and no line feed
		}
		n += len(part)
		if n > l.most {
			err := fmt.Errorf("%w: over %d bytes", ErrLineTooLong, l.most)
			l.mu.Lock()
			l.err = err // closed: no read is started after this one
			l.mu.Unlock()
			out <- lineAnswer{err: err}
			return
		}
		if !more && pieces == nil {
			out <- lineAnswer{line: string(part)} // the usual line: one read
			return
		}
		pieces = append(pieces, append([]byte(nil), part...)) // the reader reuses part
		if !more {
			out <- lineAnswer{line: join(pieces, n)}
			return
		}
	}
}

// join is the pieces as one string of n bytes.
func join(pieces [][]byte, n int) string {
	var b strings.Builder
	b.Grow(n)
	for _, p := range pieces {
		b.Write(p)
	}
	return b.String()
}
