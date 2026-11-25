# Using ui

The rules the look follows, one bullet each. `go doc github.com/birdtable/go-console/ui` has every
exported name; this page is what a program author reads before building on them.

- **What it imports**: the Charm libraries and cobra that [layers.yaml](../layers.yaml) names,
  go-humanize for a count's commas, and `go-run`'s `exit`.
- **Endings**: the program makes its codes and closing words with go-run's `exit.New` and gives the
  table to its session as `Endings(table, EndKinds{...})`, which names the kinds `ui` ends on (failed, usage, declined, busy, not ready, refused; several may name one kind) and the colour role of each kind (`Roles`); it builds a run's ending from the table (`table.End("done", what, next)`) and hands it to the `Session`'s `End`, which prints it last and
  returns its code. `ui` holds no closing word and no exit code of its own, and paints an ending
  by its kind (`Paint`), in the role the program gave the kind (`EndKinds.Roles`): good, amber for an ending that waits, red for one that did not
  get there. A done ending (code 0: done, needs you, stopped at) prints on stdout, and every
  other on stderr, painted as stderr can be, so `app | grep` reads no failure as output; under
  `--json` an ending prints nowhere, and the Result holds it; with `JSONEndingsOnStderr()` the word prints on stderr there, beside the one document on stdout. A refusal (the table's usage, busy, not ready and refused codes) or a
  failure (the table's failed ending) that gives no line to type is printed with the
  command's `--help` as its line, or with no line in a session made with `NoHelpLine()`. Findings, declined and done have no line by design and are
  printed as they are. A line ui writes, that help or the flag that answers a
  question nobody could ask, names the command by its path (`app login --help`), never by the
  words typed after it, so a value given on the command line is not printed back.
- **Messages**: the program gives every sentence `ui` says of its own, in its own language, as `WithMessages(Messages{...})`:
  the flags' help, a question's answers and refusals, a dry run's footer and its "would", the live view's notes,
  and a lesson's headings. The live view's frame is drawn by the program, as `Live.Frame`: `ui` keeps the refresh, the
  sizing and the keys, and with no `Frame` a live screen is its body alone. A field with `%s` in its comment is a format: the program moves the words around the verbs.
  There is no default and no English in `ui`: `Execute` refuses a session made without `Messages`, or with a field left
  empty, and names the field; a method of a session without them panics, as it does without `Endings`.
- **Sessions**: the program makes one `Session` per run with `New` (`New(Held())` for a step run
  inside another command, `New(Endings(table, kinds), WithMessages(m), Held())`; a session with no `Endings` or no `Messages` is refused by `Execute`)
  and owns it. The session keeps the run's record, the ending it printed (`Ended`) and its
  changes, so two runs in one process, as two tests, each keep their own; a
  screen it draws again (`Capture`, `RunLive`) keeps no ending, and another session's ending
  meanwhile is its own. What `--plain` changes is asked of the session: its questions
  (`Confirm`, `ConfirmName`, `Pick`, `AskForText`, `Form`), `Accessible`, `Charm`, `Code`,
  `Pages`, `NewFollow` and `NewLogger`.
- **Printing**: every line a run prints is printed by its session, so `--json` and
  `--no-color` are the session's and two sessions in one process each print their own:
  `run.Log`, `run.Info`, `run.Change`, `run.Warn`, `run.Arrive`, `run.Assumed`,
  `run.CanAsk`, `run.Passthrough`, `run.NewBar` and `run.NewWait`. The session keeps the run's
  changes (`Changes`, `Changed`) and whether it is a dry run (`DryRun`, true until
  `SetDryRun(false)`), and tells `New(OnChange(f))`'s `f` each change really made. A program
  mutates only when `!run.DryRun()`, and calls `run.SetDryRun(false)` before its first
  `Change`: in a dry run `OnChange` is told nothing, so a change made anyway is missing from
  the run log. The words that only paint (`s.Bold`, `s.Note`, `s.Table` and the rest) are
  the session's, plain where its Terminal takes no colour.
- **Streams and a terminal it is given**: a session writes to the
  streams it was given and paints for the terminal it was told of. `New(Out(w), Err(w),
  In(r))` gives the streams, the process's when left out; `On(t)` says what they can do,
  colour on each, the size and whether a person is at them, and without it they are a
  pipe: no colour, width unknown, nobody to ask. `Detect(in, out, err, env, rules)`
  works a `Terminal` out for a program's own streams, reading only the environment it is
  handed by the names `rules` gives (`StandardEnv()`: NO_COLOR, CLICOLOR_FORCE, COLUMNS,
  ACCESSIBLE, TERM=dumb plain), and never writes to the terminal. It keeps that
  environment in `Terminal.Env`, which the screens and questions run under, bubbletea
  and huh both, in place of the process's; a Terminal with none runs them under an empty
  one. `Clock`, `Program` and
  `Pager` (with `PagerFromEnv`) give the clock, the name a line to type starts with, and
  the pager. What painted for the process's stdout is a method of the session:
  `s.Table`, `s.List`, `s.Meter`, `s.Charted`, `s.Width`,
  `s.Paint`, `s.Page`, `s.Paged(ctx, func(w io.Writer))` and `s.Ask(form, flags...)`, beside
  `s.Bad` and the other roles. `Secret` reads the session's input.
- **Presets a program changes**: ui's look is a preset, and a program changes
  each part. `New(WithTheme(t))` paints a session in a `Theme`: the four coloured roles,
  `Muted` and a code block's ground, each a `Colour` with a light and a dark ground, in its
  lines, its questions, its tables, its logger and the help. `ANSI()`, the terminal's own
  colours, is the default; `Contrast()` is fixed colours measured at 4.5:1 on both grounds,
  drawn for the ground `Terminal.Dark` says; `Mono()` is bold and faint only. `WithMarkup(f)` marks up each line with
  a program's own `f`, given a `Painter` to paint in a `Tone`; `BacktickMarkup` paints
  commands written between backticks, and without markup a line is printed as written.
  `MaxWidth(n)` caps the width drawn to, help included; without it the terminal's whole width
  is used. `Execute` draws the
  banner only when given `WithBanner()`. It gives only the flags the program names with
  `FlagNames(Flags{...})`, none by default, and a command may define its own of any other name. NO_COLOR beats CLICOLOR_FORCE by default, and rules with `ForceWins` give the
  force the last word.
- **The other libraries' types it shows**, each on purpose: `Execute` and `Raw` take a cobra
  `*cobra.Command`, since a program builds its command tree with cobra and ui runs it, and
  wrapping cobra would mean a second tree model to learn. `RunLive` takes bubbletea's
  `tea.ProgramOption`s, and `LiveDriver`, `FollowDriver` and `uitest`'s `Screen`, `Feed`, `Key` and `Model`
  speak `tea.Msg` and `tea.Cmd`, since a program may build its own view with bubbletea and
  drive ui's from a test the same way. `Session.Ask` takes huh's `*huh.Form`, and
  `uitest.FormScreen` drives one, because a form is built from huh's fields and a second
  form model would be one to learn. `Session.NewLogger` returns the standard library's
  `*slog.Logger`, so charm.land/log's type is not in the surface; `Strip` is plain text
  with every control sequence out. The help's colours follow the session's theme.
- **A program's mark**: the banner's logo is a `Brand`'s `Mark`, any number of
  rows drawn left of the wordmark, with `MarkNote` (runes drawn dim), `MarkInk` (runes in no
  colour) and `MarkGap` (the spaces before the name).
- **Answers and a screen of its own**: a test answers a session's questions by typing them
  into the real prompt: `New(On(Terminal{Interactive: true, Accessible: true}), In(r))`, and the
  secrets it asks (`run.Secret`) are lines of `In`; `run.EndsBlank()` says whether the last line the session printed was blank.
- **Colour off a terminal**: `CLICOLOR_FORCE` set to anything but empty or `0` keeps
  the colour on a pipe or a file, so `yes` and `on` count as on, as `1` does. `NO_COLOR`, set to
  a value, and `--no-color` beat it. `Detect` reads them from the environment it is handed;
  a writer that is not a file is no terminal.
- **The pager**: `s.Paged(ctx, show)` runs `show` with what it writes going through the
  session's `Pager` at a terminal, and prints it as it is anywhere else or with no pager given;
  `ctx` ending ends the pager, and a pager that fails leaves the page printed.
- **Main**: `os.Exit(ui.Main(ctx, newRoot, brand))` is the whole of a program's `main`
  (`ExampleMain` is one that compiles). It makes the session on the process's terminal, hands it to
  `newRoot` to build the tree, runs it under `Execute` and returns the code of the ending
  (`Session.ExitCode`; `End` returns `error`, always nil, and the code is kept in the session).
  Ctrl+C and SIGTERM cancel the context, so a command that stops with it ends on the failed
  ending, and `ui.Stopped(ctx)` tells it that a signal did, and which. `CI` set, to anything but `0` or `false`, is read as no colour and no question, in
  `StandardEnv`, and the banner is drawn when the first command starts, after the flags are read,
  so it never precedes a `--json` document.
- **What the look reads from a code**: 0 goes to stdout and every other to stderr; 2 to 5 carry a
  line to type; 0, 3, 4 and 6 are painted as waiting, and 1 is a failure.
- **The shared flags**: `Execute(ctx, root, run, brand)` gives a program the flags it names in `FlagNames(Flags{Plain, Accessible, Yes, JSON, NoColor})`,
  and only those: a name left empty is no flag, and no message of `ui` names it. The spelling is the
  program's (`--json` or `--machine`); `NoColor` strips the escapes from all the session and the
  command print. A program that defines one of
  the names itself is refused before anything runs. A program that reads its tree before `Execute`
  (help as a document, a second tree after a menu, an early refusal, a test in the process) calls
  `run.AddFlags(root)` first: it gives the tree the same flags, once, and `Execute` keeps them. The yes flag answers `Confirm`, never
  `ConfirmName`. Off a terminal, with no flag to answer it, a question in a session
  asks nothing and returns the usage refusal (the table's usage code), its line
  to type the command with the flag that answers it, for the command to `End` on.
- **Errors**: every error ends the run on an ending with a line to type. `Execute`
  needs the session's `Endings`, and refuses a session without them. A command line it could not read
  ends on the usage refusal, and an error a command returned, when the command did not end the
  run itself, on the failure, each with the failing command's `--help` as its line, and on
  stderr. `run.Warn` takes an ending too: the word, the sentence and the line, on
  stderr, for something the run went on past. Nothing of `ui` ends a run on free text, and no
  string of it says "invalid" (tests hold both).
- **The result**: under `--json`, a command that has not said it printed its own document
  (`run.MarkPrinted`) gets the run's `Result`, printed by `Execute` once the command returns:
  `version` (3), `command`, `word`, `what`, `next`, `code`, `changes`, `dry_run`, and, when set,
  `apply`, `run`, `data` and `error`. Version 1 held `version`, `word`, `what`, `next`, `code`,
  `changes` and `error`; version 2 added `dry_run`, true unless the program applied
  (`SetDryRun(false)`): when it is true, `changes` is a plan and nothing was changed. Version 3
  makes `error` an object, a `Fault` of `kind`, `msg` and `hint`, and adds `command`
  (the path of the command that ran), `apply` (present once the program called `SetDryRun`), `run`
  (`SetRun`, the run record's id) and `data` (`SetData`, the command's own). The four are absent,
  not null, when not set. A command ends its run inside the tree, so the result holds its ending,
  the same words the text form prints, and a failure's `error`, whose `kind` is the word of the
  ending (ui holds no table from a code to a word) and whose `hint` is its line to type. The line
  to type is the command's, never found in the sentence. `ReadResult` reads a document and
  refuses another version with `ErrResultVersion`.
- **A panic**: a command that panics ends the run on the session's failed ending with
  its exit code; under `--json` stdout still holds the one document, whose `error` does not hold
  the panic's value. The stack goes to stderr. `PanicWith` words the sentence and the line for a
  program. `runtime.Goexit` and `os.Exit` are left alone.

