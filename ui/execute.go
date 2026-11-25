package ui

// Running a command tree in the house style.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/charmbracelet/x/ansi"

	"charm.land/fang/v2"
	"github.com/birdtable/go-run/exit"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// An Option changes how Execute runs the tree.
type Option func(*settings)

type settings struct {
	version       string
	noVersion     bool
	noBuiltins    bool
	versionLine   string
	versionDoc    any
	hasVersionDoc bool
	banner        bool
	session       []SessionOption
	menu          func(*Session) ([]string, bool)
	onError       func(io.Writer, error) (int, bool)
	finish        func(int)
}

// WithVersion is the build string: it goes beside the banner's wordmark and
// answers `--version`.
func WithVersion(v string) Option { return func(s *settings) { s.version = v } }

// WithVersionLine is what `--version` prints, as it is: the one line a program says about its build
// (version.Build.Line from the version block, "v0.1.0 (712a01e) go1.27.4"), with no program name
// before it. The program hands it in, so ui imports no other module of ours.
func WithVersionLine(line string) Option { return func(s *settings) { s.versionLine = line } }

// WithVersionDocument is what `--version` prints, as one indented JSON document, in place of the line
// when `--json` is set (version.Build.Document). A program that
// gives a line and no document prints the line under `--json` too.
func WithVersionDocument(doc any) Option {
	return func(s *settings) { s.versionDoc, s.hasVersionDoc = doc, true }
}

// WithoutVersionFlag leaves `--version` off, for a program whose `version` is a
// command of its own.
func WithoutVersionFlag() Option { return func(s *settings) { s.noVersion = true } }

// WithoutBuiltins leaves help, completion and man to the program's own tree, for a program whose
// tree brings its own help func, help command and completion command. It turns off the hidden `man` command, cobra's
// default `completion` command and go-console's help drawing, so the tree's own help func and help command stand.
// It leaves on cobra's hidden `__complete`, which answers from the tree's own completion functions, and the
// session's flags, `--version` and the endings. Main passes it through as it does every Option.
func WithoutBuiltins() Option { return func(s *settings) { s.noBuiltins = true } }

// WithErrorHandler hands the program the error a run ends on, for a program that words its own
// failures and has its own map from an error to an exit code. Execute, and so Main, calls handle at most once per
// run, for an error the tree returned when no command ended the run (Session.Ended is nil) and no OwnCode subtree
// took it: a command's error, or a command line not understood. It is not called for a command's panic, which stays
// PanicWith's, on a held session too. A held session keeps no ending, so there handle is given every error the tree
// returns. w is the tree's error stream as the commands see it, so --no-color and NO_COLOR strip what handle writes.
//
// When handle answers handled, ui paints no ending and keeps none, and the run's exit code is code, held to 0..255:
// 0 is allowed, and Main then returns 0. Execute still returns the error. When it answers not handled, code is
// ignored and the run ends as it does with no handler, on the usage or the failed ending. Under the json flag
// Execute prints the run's Result either way, with the handled code and the error and no ending, unless handle
// printed a document of its own and called Session.MarkPrinted.
func WithErrorHandler(handle func(w io.Writer, err error) (code int, handled bool)) Option {
	return func(s *settings) { s.onError = handle }
}

// WithFinish is what Main does last: it calls finish exactly once with the code it is about to
// return, on every return: a run, each path of the menu, a session refused before anything ran, and a command's
// panic, which Execute turns into the failed code. Only Main honours it, as WithMenu and WithSession; Execute alone
// does not call it. A panic outside the commands, as in newRoot, in the menu's pick or in finish itself, is no
// return of Main: it goes on as it was and finish is not called for it.
func WithFinish(finish func(code int)) Option { return func(s *settings) { s.finish = finish } }

// WithBanner has Execute draw the run's banner, the Brand's wordmark and mark, as
// the run starts (Session.Arrive). It is decoration, drawn only on a terminal wide
// enough and never in a pipe, a log or under --json. Without it Execute draws none.
func WithBanner() Option { return func(s *settings) { s.banner = true } }

// Execute runs root under fang in the session's theme (WithTheme, ANSI
// when none), first drawing the banner when WithBanner asks. It
// returns the error the command returned, for the caller to turn into an exit
// code: only a program's main exits.
//
// It gives root the flags the program named in the session (FlagNames), persistent, so each
// command takes them, and each sets run: plain, accessible, yes, json and no-color, the last
// stripping the escapes from everything run and the command's tree print. A flag the program left
// unnamed is not registered, and no message names it. A program that defines a flag it named on any command of its tree is refused before
// anything runs. A tree that Session.AddFlags already gave them, or an earlier Execute with the same session, keeps
// those and gets none twice.
//
// Under the json flag, when the command has returned and has not said it printed a
// document of its own (MarkPrinted), Execute prints run's Result on stdout: a
// command ends its run with run.End inside the tree. A run that ran no
// command, as help and --version, prints none, and a command line that failed
// answers with its error, but only when the flag came before the word that
// failed: pflag stops at the first bad word, and an unknown command is found
// before any flag is read. The exit is nonzero either way.
//
// Every error ends the run on an ending with a line to type: a command line
// not understood, before any command ran, ends on the session's usage refusal; an
// error a command returned, when it did not end the run itself, ends on its
// failure. Both are built from the session's Endings, which Execute needs, as it needs its
// Messages: it refuses a session without them before anything runs. The line to type is the help of the command that failed.
//
// os.Interrupt and SIGTERM both cancel the ctx the commands get, on every system, so a test may deliver either:
// os.Interrupt is the one a test can raise on Windows too, and SIGTERM there is not one a process can raise on itself.
// Execute listens for them itself while the tree runs, and Stopped tells a command that one of them ended its ctx,
// which a return of the run does not (SignalError).
func Execute(ctx context.Context, root *cobra.Command, run *Session, b Brand, opts ...Option) error {
	var s settings
	for _, o := range opts {
		o(&s)
	}
	if err := sharedFlags(root, run); err != nil {
		return err
	}
	// fang's own colour profile lets CLICOLOR_FORCE beat NO_COLOR, and it reads
	// neither where it matters to us, so what it writes passes a writer that
	// strips the escapes whenever the environment asks for plain.
	out, errw := root.OutOrStdout(), root.ErrOrStderr()
	raw := rawStreams{root: root, out: out, errOut: errw, ended: new(atomic.Bool)}
	defer raw.ended.Store(true)
	ctx = context.WithValue(ctx, rawKey{}, raw)
	root.SetOut(plainOf(out, run, false))
	root.SetErr(plainOf(errw, run, true))
	if !s.noVersion {
		if err := setVersionOutput(root, s, run.flags.JSON, run.msgs.VersionLine); err != nil {
			return err
		}
	}
	args := []fang.Option{
		fang.WithColorSchemeFunc(helpScheme(run.outStream())),
	}
	switch {
	case s.noVersion:
		args = append(args, fang.WithoutVersion())
	case s.version != "":
		args = append(args, fang.WithVersion(s.version))
	}
	args = append(args, fang.WithErrorHandler(func(io.Writer, fang.Styles, error) {}))
	// The banner waits for the first command to start, which is after the flags are read, so
	// --json has been seen and a document is never preceded by it.
	arrive := func() {}
	if s.banner {
		arrive = func() {
			if !run.json.Load() {
				run.Arrive(b, s.version)
			}
		}
	}
	ran, failing := watchRuns(root, run, arrive)
	// After watchRuns, so a root drawing its own help as its run is not counted as a command that ran.
	if s.noBuiltins {
		args = append(args, fang.WithoutManpage(), fang.WithoutCompletions())
		wireOwnHelp(root)
	} else {
		wireHelp(root, run)
	}
	panicked, err := executeTree(ctx, root, run, errw, failing, args)
	if panicked {
		s.onError = nil
	}
	return closeRun(run, ran, failing, out, err, func(err error) (int, bool) {
		if s.onError == nil {
			return 0, false
		}
		return s.onError(root.ErrOrStderr(), err)
	})
}

// closeRun ends the run on what the command left it with, and prints the --json document the run owes. It
// returns the error the run ended on. handle is the program's (WithErrorHandler), asked once for an error that
// neither a command's ending nor an OwnCode subtree took.
func closeRun(run *Session, ran func() bool, failing func() *cobra.Command, out io.Writer, err error, handle func(error) (int, bool)) error {
	if code, ok := ownCodeOf(failing(), err); ok && run.Ended() == nil {
		run.own.Store(true)
		run.code.Store(int32(code)) //nolint:gosec // ownCodeOf clamps the code to 0..255
	} else if err != nil && run.Ended() == nil {
		if code, handled := handle(err); handled {
			run.handled.Store(true)
			run.code.Store(int32(min(max(code, 0), 255)))
		} else {
			run.end(endingFor(run, ran(), failing(), err))
		}
	}
	if run.json.Load() && !run.printed.Load() && (ran() || err != nil) {
		if werr := run.Result(err).Write(out); werr != nil && err == nil {
			err = werr
		}
	}
	return err
}

// executeTree runs the tree and turns a panic in a command into the error the
// run ends on: a failed run with a sentence that does not hold the panic's
// value, which the program may word (PanicWith). The stack is written to the
// run's own stderr, never to stdout, so a --json run still prints one
// document. recover answers nil for runtime.Goexit and os.Exit never returns,
// so neither is touched. panicked says it was a panic, which a held session's ending does not.
func executeTree(ctx context.Context, root *cobra.Command, run *Session, errw io.Writer, failing func() *cobra.Command, args []fang.Option) (panicked bool, err error) {
	defer func() {
		v := recover()
		if v == nil {
			return
		}
		what, next := run.messages().InternalError, failing().CommandPath()+" --help"
		if run.onPanic != nil {
			w, n := run.onPanic(v)
			if w != "" {
				what = w
			}
			if n != "" {
				next = n
			}
		}
		_, _ = fmt.Fprintf(errw, "panic: %v\n\n%s", v, debug.Stack())
		run.end(run.failed(what, next))
		panicked, err = true, errors.New(what)
	}()
	ctx, stop := notifySignals(ctx)
	defer stop()
	return false, fang.Execute(ctx, root, args...)
}

// endingFor is the ending of a run whose tree returned err and did not end
// the run itself: a failure when a command ran, else a usage refusal, each
// with the help of the command that failed, or root's when none was found.
func endingFor(run *Session, ran bool, failing *cobra.Command, err error) exit.Ending {
	next := failing.CommandPath() + " --help"
	if ran {
		return run.failed(err.Error(), next)
	}
	return run.usage(err.Error(), next)
}

// watchRuns wraps the run functions of every command of root's tree, its
// pre-runs, its argument check and its flag error, and answers whether a
// command ran and which command was last reached, root before any. Help, by
// `--help` or `-h` on any command, by the help command or for a command with
// no run of its own, and `--version`, man pages and completions are printed by
// cobra and fang without running one, so they answer with no document of the
// run's. The help command, man and completion are added inside fang.Execute,
// after this, so none is wrapped. A command with no argument check of its own
// keeps none, since cobra finds an unknown command by its absence. Each
// command reached is run's too, for the line to type (commandLine).
func watchRuns(root *cobra.Command, run *Session, onStart func()) (ran func() bool, failing func() *cobra.Command) {
	started, at := false, root
	reach := func(cmd *cobra.Command) {
		at = cmd
		p := cmd.CommandPath()
		run.path.Store(&p)
	}
	reach(root)
	start := func(cmd *cobra.Command, _ []string) error {
		if !started {
			onStart()
		}
		started = true
		reach(cmd)
		return nil
	}
	var wrap func(c *cobra.Command)
	wrap = func(c *cobra.Command) {
		wrapRuns(c, start)
		if check := c.Args; check != nil {
			c.Args = func(cmd *cobra.Command, args []string) error { reach(cmd); return check(cmd, args) }
		}
		flagErr := c.FlagErrorFunc()
		c.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { reach(cmd); return flagErr(cmd, err) })
		for _, sub := range c.Commands() {
			wrap(sub)
		}
	}
	wrap(root)
	return func() bool { return started }, func() *cobra.Command { return at }
}

// wrapRuns makes each Run, RunE and PreRun of c call start first, and skip the
// command when it refuses.
func wrapRuns(c *cobra.Command, start func(*cobra.Command, []string) error) {
	for _, r := range []*func(*cobra.Command, []string){&c.PersistentPreRun, &c.PreRun, &c.Run} {
		if run := *r; run != nil {
			*r = func(cmd *cobra.Command, args []string) {
				if start(cmd, args) == nil {
					run(cmd, args)
				}
			}
		}
	}
	for _, r := range []*func(*cobra.Command, []string) error{&c.PersistentPreRunE, &c.PreRunE, &c.RunE} {
		if run := *r; run != nil {
			*r = func(cmd *cobra.Command, args []string) error {
				if err := start(cmd, args); err != nil {
					return err
				}
				return run(cmd, args)
			}
		}
	}
}

type sharedFlag struct {
	name, usage string
	set         func(bool)
	get         func() bool
}

// sharedFlags gives root the flags the program named (Flags), persistent. A name left empty gives none.
func sharedFlags(root *cobra.Command, run *Session) error {
	if run == nil {
		return errors.New("ui: Execute needs the run's session, made with ui.New")
	}
	if err := run.endsErr(); err != nil {
		return err
	}
	if err := run.checkMessages(); err != nil {
		return err
	}
	flags := sharedFlagsOf(run)
	// pflag's global set is the program's too: cobra adds it to the root's persistent flags at its first merge, after
	// this check, where a flag of a shared name would be dropped in favour of the session's with nothing said.
	for _, f := range flags {
		if pflag.CommandLine.Lookup(f.name) != nil {
			return refuseOwn(root.CommandPath(), f.name)
		}
	}
	if err := refuseOwnShared(root, run, flags); err != nil {
		return err
	}
	addShared(root, run, flags)
	return nil
}

// AddFlags gives root the shared flags the session names (FlagNames), persistent, before Execute runs: the flags Execute gives it, each setting this session. It is for a program that reads its tree
// before Execute, as for help printed as a document, a second tree run after a menu, an early refusal or a test in
// the process, each of which ends on `unknown flag` without it. A second call adds nothing, and Execute, given a
// tree that already carries them, adds nothing and keeps them: it knows them by their value, which is this
// session's, not by their name. A flag of a shared name that the program defined itself, or that another session's
// AddFlags added, is still refused by Execute, and AddFlags leaves such a flag on root as it found it. A name the
// session left empty gives no flag. Experimental.
func (s *Session) AddFlags(root *cobra.Command) {
	if s == nil || root == nil {
		return
	}
	addShared(root, s, sharedFlagsOf(s))
}

// addShared registers on root each of flags that root's persistent set does not hold yet, bound to run.
func addShared(root *cobra.Command, run *Session, flags []sharedFlag) {
	pf := root.PersistentFlags()
	for _, f := range flags {
		if pf.Lookup(f.name) != nil {
			continue
		}
		pf.Var(boolValue{of: run, set: f.set, get: f.get}, f.name, f.usage)
		pf.Lookup(f.name).NoOptDefVal = "true"
	}
}

// sharedFlagsOf is the flags the program named, each with what it sets on run.
func sharedFlagsOf(run *Session) []sharedFlag {
	named := []sharedFlag{
		{run.flags.Plain, run.msgs.PlainHelp, run.plain.Store, run.plain.Load},
		{run.flags.Accessible, run.msgs.AccessibleHelp, run.accessible.Store, run.accessible.Load},
		{run.flags.Yes, run.msgs.YesHelp, run.yes.Store, run.yes.Load},
		{run.flags.JSON, run.msgs.JSONHelp, run.json.Store, run.json.Load},
		{run.flags.NoColor, run.msgs.NoColorHelp, run.noColor.Store, run.noColor.Load},
	}
	return slices.DeleteFunc(named, func(f sharedFlag) bool { return f.name == "" })
}

// refuseOwnShared refuses a tree in which a command defines a flag of a shared name. It reads c.Flags() and
// c.PersistentFlags() and nothing else of cobra's: LocalFlags, LocalNonPersistentFlags, NonInheritedFlags and
// InheritedFlags each merge the parents' persistent flags into the command's own set as they answer, after which
// every command below one with a persistent flag has a flag and help marks each such row of the command list
// `[--flags]`. A flag whose value is run's own is not the program's definition: AddFlags put it
// there, or a merge of cobra's carried it down from where AddFlags did, and it is known by that value, never by
// its name.
func refuseOwnShared(c *cobra.Command, run *Session, flags []sharedFlag) error {
	for _, f := range flags {
		for _, set := range []*pflag.FlagSet{c.PersistentFlags(), c.Flags()} {
			found := set.Lookup(f.name)
			if found == nil {
				continue
			}
			if v, ok := found.Value.(boolValue); !ok || v.of != run {
				return refuseOwn(c.CommandPath(), f.name)
			}
		}
	}
	for _, sub := range c.Commands() {
		if err := refuseOwnShared(sub, run, flags); err != nil {
			return err
		}
	}
	return nil
}

// boolValue is a shared flag's value. of is the session it sets, by which Execute knows a flag AddFlags gave.
type boolValue struct {
	of  *Session
	set func(bool)
	get func() bool
}

func (v boolValue) String() string {
	if v.get == nil {
		return "false"
	}
	return strconv.FormatBool(v.get())
}

func (v boolValue) Set(s string) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return fmt.Errorf("say true or false, not %q", s)
	}
	v.set(b)
	return nil
}

func (boolValue) Type() string     { return "bool" }
func (boolValue) IsBoolFlag() bool { return true }

// The annotations that carry what --version prints, so the template cobra runs reads them as data
// and a line or a document that holds braces is never taken for template text.
const (
	annotationVersionLine = "ui.version.line"
	annotationVersionJSON = "ui.version.json"
)

// setVersionOutput makes --version print the line the program handed in, and, where --json is set,
// the document. cobra prints the version from a template that
// is given the command, which is the one place that can read the flag: it is parsed before the
// version is printed and the command's own run never starts. With neither handed in, cobra's own
// output stands.
func setVersionOutput(root *cobra.Command, s settings, jsonFlag, versionWord string) error {
	if s.versionLine == "" && !s.hasVersionDoc {
		return nil
	}
	line := `{{with .DisplayName}}{{printf "%s " .}}{{end}}{{printf ` + strconv.Quote(versionWord) + ` .Version}}`
	if root.Annotations == nil {
		root.Annotations = map[string]string{}
	}
	if s.versionLine != "" {
		root.Annotations[annotationVersionLine] = s.versionLine
		line = `{{index $.Annotations "` + annotationVersionLine + `"}}`
	}
	tmpl := line + "\n"
	if s.hasVersionDoc && jsonFlag != "" {
		doc, err := json.MarshalIndent(s.versionDoc, "", "  ")
		if err != nil {
			return fmt.Errorf("ui: the version document cannot be written as JSON: %w", err)
		}
		root.Annotations[annotationVersionJSON] = string(doc)
		tmpl = `{{with .Flags.Lookup "` + jsonFlag + `"}}{{if eq .Value.String "true"}}{{index $.Annotations "` + annotationVersionJSON + `"}}{{else}}` + line + `{{end}}{{else}}` + line + `{{end}}` + "\n"
	}
	root.SetVersionTemplate(tmpl)
	return nil
}

// rawKey is the key of the context value Execute hands the tree: the streams it
// found on root before it wrapped them, for Raw. A value of the run's own
// context, not of the package, so two trees run at once each keep theirs.
type rawKey struct{}

// ended is set when Execute returns: cobra keeps the context on its commands,
// and after Execute Raw is the command's streams as they are then.
type rawStreams struct {
	root        *cobra.Command
	out, errOut io.Writer
	ended       *atomic.Bool
}

// Raw is the streams a command writes bytes to that are not words: an ssh
// session, a copied file, a remote command's output. cmd.OutOrStdout() and
// cmd.ErrOrStderr() under Execute wrap lines at COLUMNS and strip escapes under
// NO_COLOR, which changes a byte stream. Raw returns what Execute found before
// it wrapped them: the process's own streams, or whatever a test set with
// SetOut and SetErr. Words still go through cmd.OutOrStdout(). Outside Execute
// it returns cmd's own streams.
func Raw(cmd *cobra.Command) (stdout, stderr io.Writer) {
	if ctx := cmd.Context(); ctx != nil {
		if r, ok := ctx.Value(rawKey{}).(rawStreams); ok && r.root == cmd.Root() && !r.ended.Load() {
			return r.out, r.errOut
		}
	}
	return cmd.OutOrStdout(), cmd.ErrOrStderr()
}

// plainFile is a stream of the process that strips the escapes written to it
// where the run says the stream takes no colour, asked at each write. It keeps
// the file's descriptor, so fang still finds a terminal behind it: it reads the width and
// switches the Windows console's escapes on. err is whether it is the error
// stream. Each Write is stripped alone; fang writes whole styled strings.
type plainFile struct {
	f   *os.File
	run *Session
	err bool
}

func (p plainFile) Write(b []byte) (int, error) { return writeAsked(p.f, p.run, p.err, b) }

// Read, Fd and Close make it an x/term File, all fang asks of a terminal besides Write. WriteString is
// left out on purpose, so io.WriteString goes through Write and its strip.
func (p plainFile) Read(b []byte) (int, error) { return p.f.Read(b) }

func (p plainFile) Fd() uintptr { return p.f.Fd() }

// Close is there because x/term's File, which colorprofile and fang test a
// stream against, has it; closing a standard stream is the caller's to do.
func (p plainFile) Close() error { return p.f.Close() }

type plainWriter struct {
	io.Writer
	run *Session
	err bool
}

func (p plainWriter) Write(b []byte) (int, error) { return writeAsked(p.Writer, p.run, p.err, b) }

func writeAsked(w io.Writer, run *Session, errStream bool, b []byte) (int, error) {
	colour := run.term.Colour
	if errStream {
		colour = run.term.ErrColour
	}
	if colour && !run.noColor.Load() {
		return w.Write(b)
	}
	// fang pads a help line to its own width, 120 off a terminal; a line wider
	// than the screen loses that padding, and a shorter one is left as it is.
	width := run.outStream().width
	if width == 0 {
		width = 80
	}
	lines := strings.Split(ansi.Strip(string(b)), "\n")
	for i, l := range lines {
		if len(l) > width {
			lines[i] = strings.TrimRight(l, " ")
		}
	}
	if _, err := io.WriteString(w, strings.Join(lines, "\n")); err != nil {
		return 0, err
	}
	return len(b), nil
}

func plainOf(w io.Writer, run *Session, errStream bool) io.Writer {
	if f, ok := w.(*os.File); ok {
		return plainFile{f: f, run: run, err: errStream}
	}
	return plainWriter{w, run, errStream}
}
