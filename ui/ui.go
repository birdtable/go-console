package ui

import (
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// stdout is where the session's lines for a person go: standard output, or
// nowhere when the run answers in JSON, which is quiet apart from the
// document (the plan, the changes and the ending are in it), or the writer a
// Capture catches them in. Under `--no-color` each line is written without its
// escapes.
func (s *Session) stdout() io.Writer {
	if c := s.captured.Load(); c != nil {
		return s.colourless(c.w)
	}
	if s.json.Load() {
		return io.Discard
	}
	return s.colourless(s.outW())
}

func (s *Session) stderr() io.Writer { return s.colourless(s.errW()) }

func (s *Session) colourless(w io.Writer) io.Writer {
	if s.noColor.Load() {
		return stripWriter{w}
	}
	return w
}

// stripWriter writes what it is given without its escapes. Each Write is
// stripped alone, as plainWriter's is: the session writes whole lines.
type stripWriter struct{ w io.Writer }

func (p stripWriter) Write(b []byte) (int, error) {
	if _, err := io.WriteString(p.w, ansi.Strip(string(b))); err != nil {
		return 0, err
	}
	return len(b), nil
}

type captureTo struct{ w io.Writer }

// Capture runs f with everything meant for a person's stdout written to w
// instead, and with the run's own record (the changes, and the ending, which
// End keeps none of while its session draws) left as it was, so drawing a
// screen again changes nothing about how the run ended. Only this session's
// lines are caught and only its ending is held: another session that prints
// or ends meanwhile, as a test run beside this one, prints and keeps its own.
func (s *Session) Capture(w io.Writer, f func()) {
	s.mu.Lock()
	wasChanges, wasChanged := s.changes, s.changed
	s.mu.Unlock()
	wasBlank := s.lastBlank.Load()
	was := s.captured.Swap(&captureTo{w})
	s.drawing.Add(1)
	s.lastBlank.Store(true)
	defer func() {
		s.captured.Store(was)
		s.drawing.Add(-1)
		s.mu.Lock()
		s.changes, s.changed = wasChanges, wasChanged
		s.mu.Unlock()
		s.lastBlank.Store(wasBlank)
	}()
	f()
}

// EndsBlank reports whether the last line the session printed was empty, for
// a caller that hands the screen to something printing its own gap first: a
// help page.
func (s *Session) EndsBlank() bool { return s.lastBlank.Load() }

// Log states a fact at the left margin.
func (s *Session) Log(format string, a ...any) { s.say(s.safe(format, a)) }

func (s *Session) say(text string) {
	st := s.outStream()
	line := wrapOn(st, 0, s.markupOn(st, text))
	s.lastBlank.Store(line == "")
	_, _ = fmt.Fprintln(s.stdout(), line)
}

// Info states a supporting fact, indented under the line above it.
func (s *Session) Info(format string, a ...any) {
	s.lastBlank.Store(false)
	st := s.outStream()
	_, _ = fmt.Fprintln(s.stdout(), "  "+wrapOn(st, 2, s.markupOn(st, s.safe(format, a))))
}

// Change reports a mutation and is the only thing that may. Callers pass the
// verb first ("file x to y"); under a dry run it prints the verb with the session's DryWould ("would file") from the
// same call site, so the plan and the act cannot describe different work.
// The session keeps the line for its Result, and a change really made (never
// a dry run's) is told to the OnChange it was made with.
func (s *Session) Change(format string, a ...any) {
	line := s.safe(format, a)
	dry := s.DryRun()
	s.mu.Lock()
	s.changed = true
	s.changes = append(s.changes, line)
	s.mu.Unlock()
	s.lastBlank.Store(false)
	if !dry && s.onChange != nil {
		s.onChange(line)
	}
	verb, rest, _ := strings.Cut(line, " ")
	// A fixed gutter, so the verbs read as a list down the left edge. "would"
	// joins the verb rather than preceding the line, or it would take the
	// gutter and push the real verb in among the paths.
	col := len("quarantine") + 1
	if dry {
		would := s.messages().DryWould
		verb, col = fmt.Sprintf(would, verb), len(fmt.Sprintf(would, "quarantine"))+1
	}
	st := s.outStream()
	_, _ = fmt.Fprintln(s.stdout(), "  "+roleNote().on(st, Column(verb, col))+wrapOn(st, 2+col, s.markupOn(st, rest)))
}

// Warn reports on stderr an ending the run met and went on past: its word,
// its sentence and its line to type, as End prints them, filled as End fills
// one (withNext). It keeps nothing and returns nothing: the run ends where its
// command says.
func (s *Session) Warn(e Ending) {
	e = s.withNext(e)
	s.warn(e.Word()+": ", e.What(), e.Next())
}

func (s *Session) warn(label, what, next string) {
	st := s.errStream()
	text := block(st, label, roleAttn(), s.safe("%s", []any{what}))
	if next != "" {
		text += "    " + roleAction().on(st, s.safe("%s", []any{next})) + "\n"
	}
	fmt.Fprint(s.stderr(), text)
}

// safe takes a []any rather than being variadic so go vet does not treat it as
// a printf wrapper. The functions above that take a format are wrappers and
// are checked as such; this is the layer beneath that passes a built message
// through.
func (s *Session) safe(format string, a []any) string {
	if len(a) == 0 {
		return format
	}
	keep := ownColours(s.outStream(), s.errStream())
	args := make([]any, len(a))
	for i, v := range a {
		args[i] = argument(v, keep)
	}
	return fmt.Sprintf(format, args...)
}

// argument wraps v so it is escaped once formatted. An integer is text too,
// as %c prints it (a rune read from a file), so it is wrapped in an integer
// type of its own, which fmt still reads as a * width. A float, a complex or
// a bool prints no control character by any verb, and is left as it is. A
// value with a method that formats it is wrapped whole, so the method runs.
func argument(v any, keep map[string]bool) any {
	switch v.(type) {
	case fmt.Formatter, fmt.Stringer, fmt.GoStringer, error:
		return outside{v, keep}
	}
	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return signed(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return unsigned(rv.Uint())
	case reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.Bool:
		return v
	default:
		return outside{v, keep}
	}
}

type outside struct {
	v    any
	keep map[string]bool
}

func (o outside) Format(f fmt.State, verb rune) {
	quoted(f, fmt.Sprintf(fmt.FormatString(f, verb), o.v), o.keep)
}

// signed is an integer argument, formatted as an int64 and then escaped. An
// integer prints no colour escape by any verb (%c is one character), so none
// is kept.
type signed int64

func (n signed) Format(f fmt.State, verb rune) {
	quoted(f, fmt.Sprintf(fmt.FormatString(f, verb), int64(n)), nil)
}

type unsigned uint64

func (n unsigned) Format(f fmt.State, verb rune) {
	quoted(f, fmt.Sprintf(fmt.FormatString(f, verb), uint64(n)), nil)
}

func quoted(f fmt.State, s string, keep map[string]bool) {
	if needsEscaping(s) {
		s = escape(s, keep)
	}
	_, _ = io.WriteString(f, s)
}

// ownColours is every colour escape a role paints with, on the streams given
// (a session's stdout and stderr), and the resets: what Bold, Note and the
// rest put around a word. Text from outside that holds one of these exact
// escapes is printed in that colour; it cannot move, erase or end a line.
func ownColours(streams ...stream) map[string]bool {
	keep := map[string]bool{}
	for _, s := range streams {
		for _, r := range []role{roleBad(), roleAttn(), roleGood(), roleAction(), roleNote(), roleBold()} {
			for _, m := range sgr.FindAllString(r.on(s, "x"), -1) {
				keep[m] = true
			}
		}
	}
	return keep
}

func block(st stream, label string, r role, msg string) string {
	indent := strings.Repeat(" ", len(label))
	var b strings.Builder
	for i, line := range strings.Split(msg, "\n") {
		if i == 0 {
			b.WriteString(r.on(st, label))
		} else {
			b.WriteString(indent)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// Passthrough is where a child process's own output goes, untouched: apt's
// progress, Tailscale's login link. ui stays the only thing that prints; this
// is the one door it holds open, so a command somebody is watching run is
// seen as it runs rather than all at once at the end.
// Under `--no-color` its own escapes are left as the child wrote them.
func (s *Session) Passthrough() (out, errOut io.Writer) {
	if c := s.captured.Load(); c != nil {
		return c.w, s.errW()
	}
	if s.json.Load() {
		return io.Discard, s.errW()
	}
	return s.outW(), s.errW()
}
