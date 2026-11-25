package uitest

// A program run at a pty: a Unix pty or a ConPTY (xpty), its output read by a terminal emulator (vt), so a
// test compares the screen a person would see and not the bytes that drew it, which differ by system.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/color"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/charmbracelet/x/xpty"
)

// PtyScreenCols is how wide the emulator is, wider than any pty a test asks for, so a row the program drew too
// wide stays one row to be measured and is not wrapped by the emulator into two that fit.
const PtyScreenCols = 200

// endMark is written at the slave when the program has exited. The pty keeps its output in order, so the reader
// that has seen it has read all the program wrote: macOS drops what is unread when the last slave closes.
const endMark = "\x1b]9999;go-console-end\a"

// A Pty is a command started at a pty, its output read into a screen.
type Pty struct {
	ctx  context.Context
	pty  xpty.Pty
	cmd  *exec.Cmd
	mu   sync.Mutex // guards em and last: the emulator is not safe to read while it is written
	em   *vt.Emulator
	last time.Time
	read chan struct{} // closed when the output has been read to its end
	wire *wire         // what is written at the pty: the keys sent and the screen's answers
	log  *os.File      // PtyLog: every read and write with its time, for a run read afterwards
	keep bool          // PtyScrollback was given
}

// wire is what is written at the pty, in the order it came, in a queue its writers never wait on: a program that
// has stopped reading its input holds this queue's goroutine and nothing else, not the screen's lock and not a
// test's deadline.
type wire struct {
	mu   sync.Mutex
	more *sync.Cond
	buf  []byte
	err  error
	shut bool
}

func (w *wire) add(b []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, b...)
	w.more.Signal()
	return w.err
}

func (w *wire) run(write func([]byte) error) {
	for {
		w.mu.Lock()
		for len(w.buf) == 0 && !w.shut {
			w.more.Wait()
		}
		b := w.buf
		w.buf = nil
		w.mu.Unlock()
		if len(b) == 0 {
			return
		}
		if err := write(b); err != nil {
			w.mu.Lock()
			w.err = err
			w.mu.Unlock()
			return
		}
	}
}

func (p *Pty) trace(dir string, b []byte) {
	if p.log != nil {
		fmt.Fprintf(p.log, "%s %p %s %q\n", time.Now().Format("15:04:05.000000"), p, dir, b)
	}
}

// A PtyOption sets one thing of a StartPty that has no default.
type PtyOption func(*ptyOptions)

type ptyOptions struct {
	logPath    string
	scrollback int
}

// PtyScrollback keeps the last lines rows that scroll off the top of the main screen, for Scrollback to give. The
// caller names the count: uitest has none of its own, and with no PtyScrollback, or a count under one, Scrollback
// gives nothing. The count is the cap of vt's scrollback, set with its SetScrollbackSize. With no option vt still
// holds its own 10000 lines, its default and not a value uitest sets, and Scrollback does not give them.
func PtyScrollback(lines int) PtyOption { return func(o *ptyOptions) { o.scrollback = lines } }

// PtyLog appends every read and write of the pty, with its time and its bytes, to the file at path, for a run to be
// read afterwards. The caller names the file, from whatever setting it chooses: uitest reads no environment
// variable, and with no PtyLog nothing is logged. An empty path is none.
func PtyLog(path string) PtyOption { return func(o *ptyOptions) { o.logPath = path } }

// StartPty starts cmd at a pty cols wide and rows high. The screen answers the program's terminal queries as a
// terminal does (a black background, a device attributes report), unless mute is set, and then a program that
// waits for an answer waits out its own timeout. ctx ends the command. The caller ends with Close.
func StartPty(ctx context.Context, cmd *exec.Cmd, cols, rows int, mute bool, opts ...PtyOption) (*Pty, error) {
	pty, err := xpty.NewPty(cols, rows)
	if err != nil {
		return nil, err //nolint:wrapcheck // xpty's words say it
	}
	return startOn(ctx, cmd, pty, rows, mute, opts...)
}

// startOn is StartPty on a pty it is given.
func startOn(ctx context.Context, cmd *exec.Cmd, pty xpty.Pty, rows int, mute bool, opts ...PtyOption) (*Pty, error) {
	var o ptyOptions
	for _, opt := range opts {
		opt(&o)
	}
	p := &Pty{ctx: ctx, pty: pty, cmd: cmd, em: vt.NewEmulator(PtyScreenCols, rows), last: time.Now(), read: make(chan struct{}), wire: &wire{}}
	p.wire.more = sync.NewCond(&p.wire.mu)
	if o.logPath != "" {
		p.log, _ = os.OpenFile(o.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	}
	p.em.SetBackgroundColor(color.Black)
	if o.scrollback > 0 {
		p.em.SetScrollbackSize(o.scrollback)
		p.keep = true
	}
	out, err := ptyOutput(pty)
	if err != nil {
		_ = pty.Close()
		return nil, err
	}
	if err := pty.Start(cmd); err != nil {
		_ = out.Close()
		_ = pty.Close()
		return nil, err //nolint:wrapcheck // exec's words say it
	}
	go p.wire.run(func(b []byte) error {
		p.trace("write", b)
		_, err := pty.Write(b)
		return err //nolint:wrapcheck // xpty's words say it
	})
	go func() {
		buf := make([]byte, 512)
		for {
			n, err := p.em.Read(buf)
			if n > 0 && !mute {
				_ = p.wire.add(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		defer close(p.read)
		defer out.Close()
		buf, tail := make([]byte, 4096), []byte{}
		for {
			n, err := out.Read(buf)
			p.trace("read", buf[:n])
			p.mu.Lock()
			_, _ = p.em.Write(buf[:n])
			p.last = time.Now()
			p.mu.Unlock()
			tail = append(tail, buf[:n]...)
			if err != nil || bytes.Contains(tail, []byte(endMark)) {
				return
			}
			tail = tail[max(0, len(tail)-len(endMark)):]
		}
	}()
	return p, nil
}

// Send types s at the program: it queues the keys and returns, and its error is that of a write that failed before.
func (p *Pty) Send(s string) error { return p.wire.add([]byte(s)) }

// Screen is what the terminal shows: the rows, each without its trailing spaces, and no blank row at the end.
func (p *Pty) Screen() string {
	p.mu.Lock()
	text := p.em.String()
	p.mu.Unlock()
	rows := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	for i, r := range rows {
		rows[i] = strings.TrimRight(r, " ")
	}
	return strings.TrimRight(strings.Join(rows, "\n"), "\n")
}

// Scrollback is the rows that have scrolled off the top of the main screen, oldest first and one to a line, each
// without its trailing spaces, kept up to the count of PtyScrollback. It is empty with no PtyScrollback. A program on
// the alternate screen scrolls nothing into it: the alternate screen has no scrollback. It is vt's own scrollback,
// read as Screen reads the screen.
func (p *Pty) Scrollback() string {
	if !p.keep {
		return ""
	}
	p.mu.Lock()
	var rows []string
	if sb := p.em.Scrollback(); sb != nil {
		for _, l := range sb.Lines() {
			rows = append(rows, strings.TrimRight(l.String(), " "))
		}
	}
	p.mu.Unlock()
	return strings.Join(rows, "\n")
}

// Styled is the screen with each cell's styling as escapes, so it holds none where nothing is coloured or bold,
// whatever the pty itself wrote.
func (p *Pty) Styled() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.em.Render()
}

// WaitFor returns the screen when ok says yes of it and of the time since the pty last gave a byte, or false
// when the timeout comes first or the output has ended.
func (p *Pty) WaitFor(timeout time.Duration, ok func(screen string, idle time.Duration) bool) (string, bool) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		p.mu.Lock()
		idle := time.Since(p.last)
		p.mu.Unlock()
		if s := p.Screen(); ok(s, idle) {
			return s, true
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			return p.Screen(), false
		case <-p.read:
			s := p.Screen()
			return s, ok(s, 0)
		}
	}
}

// Wait waits for the program to exit and for all it wrote to be read, and returns its exit code, or -1 when its
// context or a signal ended it.
func (p *Pty) Wait() int {
	err := xpty.WaitProcess(p.ctx, p.cmd)
	if u, ok := p.pty.(*xpty.UnixPty); ok {
		_, _ = u.Slave().WriteString(endMark) // the parent holds the slave so a child that exited at once leaves its output to read
	} else { // a ConPTY gives no end of output while it is open: closing it flushes its last frame, and the reader, on its own copy of the pipe, reads to the end
		_ = p.pty.Close()
	}
	<-p.read
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

// Close ends the program if it is still running and closes the pty.
func (p *Pty) Close() {
	defer func() { p.wire.mu.Lock(); p.wire.shut = true; p.wire.more.Broadcast(); p.wire.mu.Unlock() }()
	if p.log != nil {
		defer p.log.Close() // a log left open keeps a test's folder from being removed on Windows
	}
	if p.cmd.ProcessState == nil {
		_ = p.cmd.Process.Kill()
	}
	_ = p.pty.Close()
	if c, ok := p.em.InputPipe().(io.Closer); ok { // not em.Close, which races the copy of the replies on a field of its own
		_ = c.Close()
	}
}
