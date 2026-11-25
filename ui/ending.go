package ui

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/birdtable/go-run/exit"
)

// An Ending is the last thing a run prints: the closing word, the sentence
// beside it, the line to type next and the exit code. The program builds one
type Ending interface {
	Word() string
	What() string
	// Next is the line to type, or empty when there is none.
	Next() string
	Code() int
}

// A Session is one run's own record and settings: the ending it printed and
// the changes it reported, which its run log and its Result keep; whether its ending is held; whether it is a dry run; and what
// Execute's flags asked of it (`--plain`, `--accessible`, `--yes`, `--json`,
// `--no-color`). Everything it prints goes through its own methods, so two
// runs in one process, as two tests, each print, keep and answer their own. The program makes one with New and owns it. Its
// methods may be called from more than one goroutine.
type Session struct {
	held bool
	// ends is the program's codes and closing words (Endings), hasEnds whether it gave them.
	ends    exit.Table
	kinds   EndKinds
	hasEnds bool
	// msgs is the program's own words for what ui says (WithMessages), hasMsgs whether it gave them.
	msgs    Messages
	hasMsgs bool
	// flags is the spelling of the flags Execute gives the commands (FlagNames).
	flags    Flags
	onChange func(line string)
	// own is whether an OwnCode subtree returned its own code, which code holds.
	own atomic.Bool
	// handled is whether the program's error handler took the run's error (WithErrorHandler), whose code code holds.
	handled atomic.Bool
	// path is the CommandPath of the command Execute last reached, the
	// command a line to type names (commandLine).
	path atomic.Pointer[string]

	plain      atomic.Bool
	accessible atomic.Bool
	yes        atomic.Bool
	json       atomic.Bool
	// endingsOut is whether every closing word prints on stdout (EndingsOnStdout).
	endingsOut bool
	// noHelpLine is whether a lineless refusal or failure keeps no line (NoHelpLine).
	noHelpLine bool
	// jsonEndingsErr is whether a closing word prints on stderr under --json (JSONEndingsOnStderr).
	jsonEndingsErr bool
	// noHeaderRule is whether a painted table draws no rule under its header (NoHeaderRule).
	noHeaderRule bool
	noColor      atomic.Bool
	// dryRunSet is whether SetDryRun was called, so Result says apply only when
	// the program said how.
	dryRunSet atomic.Bool
	// applying is a dry run's opposite, so a session's zero value is the
	// safe one: a dry run.
	applying atomic.Bool
	// printed is whether the command printed its own JSON document.
	printed atomic.Bool

	code atomic.Int32

	// drawing counts the Captures under way, while End keeps nothing.
	drawing atomic.Int32
	// captured, while set, is where this session's lines for a person go
	// instead of the screen. It is the session's own, so a Capture catches
	// only the lines of the session it was given.
	captured atomic.Pointer[captureTo]

	// out, errOut and in are the session's streams (Out, Err, In); nil is the
	// process's, read when written to (detect.go).
	out, errOut io.Writer
	in          io.Reader

	// term is what the streams can do, as the caller said (On); the zero
	// value is a pipe.
	term Terminal

	theme *Theme

	markupFn func(text string, p Painter) string

	maxWidth int

	now func() time.Time

	program string

	pager []string

	// pagerUsed is whether the last Paged went through the pager (PagerUsed). Atomic, as lastBlank is.
	pagerUsed atomic.Bool

	// lastBlank is whether the last line this session printed was empty, so a
	// gap that follows another gap (the banner's, then an ending's) is one line,
	// not two. Atomic, since a session's methods may run in more than one
	// goroutine.
	lastBlank atomic.Bool

	// shown draws the run's banner at most once, and bannerOnScreen is whether it
	// drew one (banner.go, Arrive). Each session's own, so two runs in one
	// process, as two tests, each arrive.
	shown          sync.Once
	bannerOnScreen atomic.Bool

	// logEvery and logAfter, when set, stand in for a Bar's thresholds off a terminal
	// (progress.go), so a test can make a claim about a five-hour run in milliseconds.
	logEvery int
	logAfter time.Duration

	onPanic func(v any) (what, next string)

	mu      sync.Mutex
	ended   Ending
	runID   string
	data    any
	changes []string
	changed bool
}

// A SessionOption sets how a Session ends its run.
type SessionOption func(*Session)

// Held keeps the session's closing word unprinted: a command run inside
// another, as a setup step's dry run, ends where the one around it says.
func Held() SessionOption { return func(s *Session) { s.held = true } }

// Out is where the session writes what it says to a person: its lines, its
// tables and its ending. Without it, the process's standard output.
func Out(w io.Writer) SessionOption { return func(s *Session) { s.out = w } }

// Err is where the session writes its progress, warnings and failures. Without
// it, the process's standard error.
func Err(w io.Writer) SessionOption { return func(s *Session) { s.errOut = w } }

// EndingsOnStdout sends every closing word End prints to stdout, a refusal or a failure as well as a done
// one, for a program whose contract puts its closing word on stdout. Without it a code-0 ending prints on
// stdout and every other on stderr. The --json and captured cases are as they were, and the colour is
// stdout's. The program decides which stream its contract uses; ui decides none.
func EndingsOnStdout() SessionOption { return func(s *Session) { s.endingsOut = true } }

// NoHelpLine leaves a refusal or a failure that gives no line to type with none, for a program whose
// contract is that such an ending has no line: End and Warn print it, and End keeps it, with an empty
// Next. Without it the line is the command's help (withNext). An ending that gives its own line prints
// it either way, and so do ui's own endings, which always give one.
func NoHelpLine() SessionOption { return func(s *Session) { s.noHelpLine = true } }

// JSONEndingsOnStderr prints the closing word on stderr under --json, painted as stderr can be, for a
// program whose contract puts the word there beside the one document on stdout. It is stderr with
// EndingsOnStdout too, since under --json stdout is the document's. Without it an ending prints nowhere
// under --json. Off --json it changes nothing, and inside a Capture the word is drawn there.
func JSONEndingsOnStderr() SessionOption { return func(s *Session) { s.jsonEndingsErr = true } }

// In is where the session reads a person's answers and a secret. Without it,
// the process's standard input.
func In(r io.Reader) SessionOption { return func(s *Session) { s.in = r } }

// On says what the session's streams can do: colour, size, and whether a
// person is at them (Detect works it out for the process's). Without it the
// streams are a pipe: no colour, width unknown, and nobody to ask.
func On(t Terminal) SessionOption { return func(s *Session) { s.term = t } }

// Clock is the session's clock, for a bar's rate, a wait's time left, a live
// view's ticks and a log line's time. Without it, time.Now.
func Clock(now func() time.Time) SessionOption { return func(s *Session) { s.now = now } }

// Program is the program's name, which a line to type starts with, as
// `app sync --yes`. Without it a line to type is the command and the flag.
func Program(name string) SessionOption { return func(s *Session) { s.program = name } }

// Pager is the command a long page goes through at a terminal, as
// PagerFromEnv gives it. Without it Paged prints the page as it is.
func Pager(argv ...string) SessionOption {
	return func(s *Session) { s.pager = append([]string(nil), argv...) }
}

// Terminal is what the session's streams can do, as On gave it.
func (s *Session) Terminal() Terminal { return s.term }

func (s *Session) clockFunc() func() time.Time { return s.clock }

func (s *Session) outStream() stream {
	return s.drawn(stream{rich: s.term.Colour, width: s.term.Width, interactive: s.term.Interactive})
}

func (s *Session) drawn(st stream) stream {
	st.thm, st.dark, st.maxWidth, st.ascii = s.theme, s.term.Dark, s.maxWidth, !s.term.Unicode
	return st
}

func (s *Session) errStream() stream {
	// The width is the output's, so it is the error stream's only where that is
	// a screen too, one a person is at or that takes colour: `app 2>log` at a
	// terminal is not wrapped to its screen.
	width := 0
	if s.term.Interactive || s.term.ErrColour {
		width = s.term.Width
	}
	return s.drawn(stream{rich: s.term.ErrColour, width: width, interactive: s.term.Interactive})
}

func (s *Session) inTerminal() bool { return s.term.Interactive }

// Plain is --plain: no live view, no pager and no code block's ground, and
// the questions asked as plain lines that do not redraw, the way huh's own
// accessible mode asks them.
func Plain() SessionOption { return func(s *Session) { s.plain.Store(true) } }

// EndKinds names, in the program's own table, the kinds of ending ui ends a run on. ui names no
// kind of its own: each field is the Name of a kind the program gave exit.New, and none has a
// default. Failed is an error a command returned, a panic, or a record that did not close; Usage
// is a command line not understood or a question with nobody to ask; Declined is a person who
// stopped a form or a menu; Busy is a run another holds; NotReady and Refused are the two other
// refusals, which ui does not end on itself but knows as refusals, so a program's own ending of
// either gets the command's help for a line to type when it gives none. Two fields may name the
// same kind. Endings checks every name against the table.
type EndKinds struct {
	Failed, Usage, Declined, Busy, NotReady, Refused string
	// Roles is the theme role each kind of the table is painted in, by the kind's Name; a kind
	// with no entry is painted in none. ui names no exit code, so the colour of an ending is the
	// program's, given with its kinds. Each name must be a kind of the table.
	Roles map[string]EndRole
}

// An EndRole is the theme role a closing word is painted in: none, good, attention (an ending that
// waits on somebody or something) or bad (one that did not get there).
type EndRole int

const (
	// EndPlain paints a closing word in no role.
	EndPlain EndRole = iota
	// EndGood paints it in the theme's good role.
	EndGood
	// EndAttn paints it in the attention role.
	EndAttn
	// EndBad paints it in the bad role.
	EndBad
)

// Endings is the program's table of endings, as exit.New checked it, and the names in it ui ends on
// (EndKinds). The session builds every ending of its own from them: the refusal of a command line
// not understood, or of a question with nobody to ask, the failure of an error a command
// returned, and the endings a menu and a recorder end on. A line to type is the command's path with the flag that
// answers, never the words a person typed, so no value given to the command is printed back.
// There is no default: Execute refuses a session made without Endings, or with a name that is not
// a kind of the table, and a method of it that needs an ending panics without them.
func Endings(t exit.Table, k EndKinds) SessionOption {
	return func(s *Session) { s.ends, s.kinds, s.hasEnds = t, k, true }
}

// endsErr is why the session's Endings cannot be used: none were given, or a name in EndKinds is
// not a kind of the table.
func (s *Session) endsErr() error {
	if !s.hasEnds {
		return errors.New("ui: Execute needs the session's endings: make it with ui.New(ui.Endings(table, kinds)), the table from exit.New")
	}
	for field, name := range map[string]string{"Failed": s.kinds.Failed, "Usage": s.kinds.Usage, "Declined": s.kinds.Declined,
		"Busy": s.kinds.Busy, "NotReady": s.kinds.NotReady, "Refused": s.kinds.Refused} {
		if _, ok := s.kind(name); !ok {
			return fmt.Errorf("ui: EndKinds.%s is %q, and the table has no kind of that name", field, name)
		}
	}
	for name, r := range s.kinds.Roles {
		if _, ok := s.kind(name); !ok {
			return fmt.Errorf("ui: EndKinds.Roles names %q, and the table has no kind of that name", name)
		}
		if r < EndPlain || r > EndBad {
			return fmt.Errorf("ui: EndKinds.Roles gives %q the role %d, which is none of EndPlain, EndGood, EndAttn and EndBad", name, int(r))
		}
	}
	return nil
}

// kind is the table's kind of that name.
func (s *Session) kind(name string) (exit.Kind, bool) {
	for _, k := range s.ends.Kinds() {
		if k.Name == name {
			return k, true
		}
	}
	return exit.Kind{}, false
}

// table is the session's endings, or a panic naming what is missing.
func (s *Session) table() exit.Table {
	if !s.hasEnds {
		panic("ui: the session has no Endings: make it with ui.New(ui.Endings(table, kinds)), the table from exit.New") //nolint:forbidigo // a programmer error: Execute refuses a session without Endings before any command runs
	}
	return s.ends
}

func (s *Session) failed(what, next string) exit.Ending {
	return s.table().End(s.kinds.Failed, what, next)
}
func (s *Session) usage(what, next string) exit.Ending {
	return s.table().End(s.kinds.Usage, what, next)
}
func (s *Session) declined(what string) exit.Ending { return s.table().End(s.kinds.Declined, what, "") }

// failedKind is the table's failed kind, the zero Kind for a session with none.
func (s *Session) failedKind() exit.Kind {
	k, _ := s.kind(s.kinds.Failed)
	return k
}

// failedCode is a failure's code: the table's, or 1 for a session with none, whose error has no ending.
func (s *Session) failedCode() int {
	if k, ok := s.kind(s.kinds.Failed); s.hasEnds && ok {
		return k.Code
	}
	return 1
}

// usageCode is the code of a usage refusal, 1 for a session with none.
func (s *Session) usageCode() int {
	if k, ok := s.kind(s.kinds.Usage); s.hasEnds && ok {
		return k.Code
	}
	return 1
}

// isRefusal is whether e is a refusal: an ending of the usage, busy, not ready or refused kind. Codes
// may repeat between kinds, so the word tells them apart.
func (s *Session) isRefusal(e Ending) bool {
	if !s.hasEnds {
		return false
	}
	for _, name := range []string{s.kinds.Usage, s.kinds.Busy, s.kinds.NotReady, s.kinds.Refused} {
		if k, ok := s.kind(name); ok && e.Word() == k.Word {
			return true
		}
	}
	return false
}

// JSON is a session that answers in JSON, as `--json` asks: stdout holds the
// Result or the command's own document and nothing else, and what is meant
// for a person goes to stderr or nowhere.
func JSON() SessionOption { return func(s *Session) { s.json.Store(true) } }

// OnChange tells f every change the session really makes (never a dry
// run's), as the line Change prints. The run log hangs off it: every change
// comes through Change, so nothing can be changed and not recorded. A
// function rather than an import, because ui depends on nothing above it.
func OnChange(f func(line string)) SessionOption { return func(s *Session) { s.onChange = f } }

// New is a session that has printed no ending.
func New(opts ...SessionOption) *Session {
	s := &Session{}
	for _, o := range opts {
		o(s)
	}
	return s
}

// JSON is whether the session answers in JSON.
func (s *Session) JSON() bool { return s.json.Load() }

// DryRun is whether the session's changes are only planned: true until
// SetDryRun(false), so a session's zero value is the safe one.
func (s *Session) DryRun() bool { return !s.applying.Load() }

// SetDryRun makes the session's changes planned (true) or made (false). A
// setter rather than an option, since a program's own `--dry-run` is read
// after its session is made and handed to Execute.
func (s *Session) SetDryRun(on bool) {
	s.dryRunSet.Store(true)
	s.applying.Store(!on)
}

// SetData is the command's own data, which the run's Result carries under
// "data". The last call wins; nil takes it away. Data that cannot be encoded
// as JSON leaves the run with no document: Result.Write refuses it.
func (s *Session) SetData(v any) {
	s.mu.Lock()
	s.data = v
	s.mu.Unlock()
}

// SetRun is the id of the run's record, which the run's Result carries under
// "run".
func (s *Session) SetRun(id string) {
	s.mu.Lock()
	s.runID = id
	s.mu.Unlock()
}

// PanicWith says what a command's panic ends the run on, in place of the
// words ui gives it: f is handed the panic's value and answers the sentence
// beside the failed word and the line to type, or "" for either to keep ui's.
// The value and the stack are never printed in a --json document; the stack
// goes to stderr (Execute).
func PanicWith(f func(v any) (what, next string)) SessionOption {
	return func(s *Session) { s.onPanic = f }
}

// Changes is every line Change reported, in order, for an answer of the
// program's own shape.
func (s *Session) Changes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.changes...)
}

// Changed is whether the session reported any change, so a run that did
// nothing can say so rather than print a blank screen.
func (s *Session) Changed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.changed
}

// MarkPrinted says the command printed its own JSON document, a shape of its
// own, so Execute prints no Result after it.
func (s *Session) MarkPrinted() { s.printed.Store(true) }

// Ended is the ending this session printed, or nil before it printed one:
// what the run log keeps, so `runs` shows the word the run ended on rather
// than one guessed from its exit code.
func (s *Session) Ended() Ending {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ended
}

// End prints e, last, after everything else,
// and keeps the exit code it stands for (ExitCode), and returns nil, so a command's last line can be
// `return s.End(table.End("done", what, ""))`. An ending with no word prints nothing, and
// so does a held session's. One drawn inside its session's Capture is printed there and
// not kept: drawing a screen again changes nothing about how the run ended.
//
// A done ending (code 0, as done, needs you and stopped at) prints on
// stdout, and every other on stderr, painted as stderr can be, so `app | grep`
// never reads a failure as output. Under `--json` an ending prints nowhere, as
// every line for a person does, or on stderr with JSONEndingsOnStderr, and
// inside a Capture it is drawn there (endingOut).
//
// A refusal (the table's usage, busy, not ready and refused codes) or a failure that gives no line to type is printed,
// and kept, with this command's help as its line (withNext), or with no line under NoHelpLine.
func (s *Session) End(e Ending) error {
	s.end(e)
	return nil
}

// end is End with no result, for the code that ends a run itself.
func (s *Session) end(e Ending) {
	if e.Word() != "" && !s.held {
		e = s.withNext(e)
		if s.drawing.Load() == 0 {
			s.mu.Lock()
			s.ended = e
			s.mu.Unlock()
		}
		w, st := s.endingOut(e)
		if !s.lastBlank.Load() {
			_, _ = fmt.Fprintln(w)
		}
		s.lastBlank.Store(false)
		_, _ = fmt.Fprintln(w, "  "+wrapOn(st, 2, s.markupOn(st, s.safe("%s%s", []any{Column(s.paintOn(st, e, e.Word()), 11), e.What()}))))
		if next := e.Next(); next != "" {
			_, _ = fmt.Fprintln(w, wrapOn(st, 0, s.markupOn(st, s.safe("    %s", []any{roleAction().on(st, next)}))))
		}
	}
	if s.drawing.Load() == 0 {
		s.code.Store(int32(min(max(e.Code(), 0), 255)))
	}
}

// ExitCode is the exit code of the ending the session last printed, 0 before one; an End inside a
// Capture keeps none, as it keeps no ending. Main returns it.
func (s *Session) ExitCode() int { return int(s.code.Load()) }

// withNext is e, or, for a refusal or a failure that gives no line to type,
// e with this command's help as its line: a person stopped with nothing to
// type is stuck. ui's own endings always give a line, and a test
// holds them to it, so this catches a program's, as the table's failed ending with no
// next or exit's lineless. A refusal is a code from 2 to 5 (usage, busy, not
// ready, refused); a failure is what isFailure says. Findings and declined
// have no line by design.
func (s *Session) withNext(e Ending) Ending {
	if e.Next() != "" {
		return e
	}
	if s.noHelpLine || (!s.isRefusal(e) && !s.isFailure(e)) {
		return e
	}
	return nextGiven{e, s.commandLine("--help")}
}

// isFailure is whether e is the session's failed ending: code 1 and its
// word. Findings shares code 1 and has a word of its own, and exit says an
// ending's kind by its word alone, so the word tells them apart. A session
// with no Endings, which only a run outside Execute can be, has no failure
// to tell.
func (s *Session) isFailure(e Ending) bool {
	k := s.failedKind()
	return s.hasEnds && e.Code() == k.Code && e.Word() == k.Word
}

func (s *Session) endingOut(e Ending) (io.Writer, stream) {
	if s.jsonEndingsErr && s.json.Load() && s.captured.Load() == nil {
		return s.stderr(), s.errStream()
	}
	if e.Code() == 0 || s.endingsOut || s.captured.Load() != nil || s.json.Load() {
		return s.stdout(), s.outStream()
	}
	return s.stderr(), s.errStream()
}

type nextGiven struct {
	Ending
	next string
}

func (n nextGiven) Next() string { return n.next }

// Paint is text in e's colour: the role the program gave its closing word's kind (EndKinds.Roles), by kind, never by code.
func (s *Session) Paint(e Ending, text string) string { return s.paintOn(s.outStream(), e, text) }

func (s *Session) paintOn(st stream, e Ending, text string) string {
	if !s.hasEnds {
		return text
	}
	for _, k := range s.ends.Kinds() {
		if k.Word != e.Word() {
			continue
		}
		switch s.kinds.Roles[k.Name] {
		case EndGood:
			return roleGood().on(st, text)
		case EndAttn:
			return roleAttn().on(st, text)
		case EndBad:
			return roleBad().on(st, text)
		case EndPlain:
		}
		break
	}
	return text
}
