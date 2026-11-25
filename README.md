<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/mark/go-console-banner-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="docs/mark/go-console-banner-light.svg">
    <img src="docs/mark/go-console-banner-light.svg" width="348" alt="go-console: a terminal window with a prompt and a coral cursor, then a coral bird in flight">
  </picture>
</p>

# go-console

One terminal look for Go command-line programs.

[![Latest release](https://img.shields.io/github/v/release/birdtable/go-console)](https://github.com/birdtable/go-console/releases)
[![Go reference](https://pkg.go.dev/badge/github.com/birdtable/go-console.svg)](https://pkg.go.dev/github.com/birdtable/go-console)
[![License](https://img.shields.io/github/license/birdtable/go-console)](LICENSE)

A command-line program prints lines, tables, charts, forms, menus and a closing word. go-console gives all of them one look. At a terminal they are painted with [Charm](https://charm.sh). In a pipe or a log the same words come out plain, and `--json` makes a run print one document. A program sets its own words, flags and brand, and the library names no program.

## Install

```sh
go get github.com/birdtable/go-console@latest
```

It needs Go 1.27.1 or later.

## Quick start

```go
package main

import (
	"context"
	"os"

	"github.com/birdtable/go-run/exit"
	"github.com/spf13/cobra"

	"github.com/birdtable/go-console/ui"
)

func main() {
	table, err := exit.New(
		exit.Kind{Name: "done", Word: "done", Code: 0},
		exit.Kind{Name: "failed", Word: "failed", Code: 1},
		exit.Kind{Name: "usage", Word: "refused", Code: 2},
	)
	if err != nil {
		panic(err)
	}
	kinds := ui.EndKinds{Failed: "failed", Usage: "usage", Refused: "usage", Declined: "failed", Busy: "failed", NotReady: "failed"}
	flags := ui.Flags{Plain: "plain", JSON: "json", Yes: "yes", NoColor: "no-color"}

	newRoot := func(run *ui.Session) *cobra.Command {
		return &cobra.Command{Use: "app", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
			run.Info("two services")
			run.Log(run.Table(
				[]ui.TableColumn{{Title: "name", Priority: 2}, {Title: "state", Priority: 1}},
				[][]string{{"web", "running"}, {"worker", "stopped"}},
			))
			return run.End(table.End("done", "listed 2 services", ""))
		}}
	}
	os.Exit(ui.Main(context.Background(), newRoot, ui.Brand{Name: "app"},
		ui.WithSession(ui.Endings(table, kinds), ui.FlagNames(flags), ui.WithMessages(words()))))
}

// words is every sentence ui says of its own, in the program's language.
func words() ui.Messages {
	return ui.Messages{
		PlainHelp: "no live view or pager", AccessibleHelp: "questions as plain lines", YesHelp: "answer yes to every question",
		JSONHelp: "one JSON document on stdout", NoColorHelp: "no colour",
		Yes: "Yes", No: "No", AnsweredYes: "yes", AnsweredNo: "no", AnsweredYesBy: "yes, by %s",
		AnsweredYesAdvance: "yes, in advance", AnsweredYesTyped: "yes, typed %s", YesFlagName: "yes",
		TypeNameTitle: "Type %s to %s", TypeNameAdvance: "Or answer in advance with %s", TypeNameExactly: "type %s exactly",
		NobodyToAsk: "this would %s, and there is no terminal to ask on", CouldNotAsk: "this would %s, and the question failed (%s)",
		CouldNotAskNothingDone: "%s, so nothing was done", FormNoTerminal: "this needs answers, and there is no terminal",
		FormNotAnswered: "nothing was answered", FormCouldNot: "the questions failed (%s)", DryWould: "would %s",
		LiveNothingChanged: "nothing was changed", LiveNotTheName: "that is not the name", LiveNothingSelected: "nothing is selected",
		LiveNothingToStepInto: "nothing to step into", LiveNoName: "this row has no name", LiveBack: "back from %s",
		HelpArgs: "[topic or command]", HelpShort: "Help about a command", HelpUnknownTopic: "Unknown help topic %#q",
		NameOnCommandLine: "Or name it on the command line: %s", AnswerWith: "Or answer with %s",
		MenuCouldNotShow: "could not show the menu (%s)", CouldNotAskFor: "could not ask for %s (%s)", MenuNothingPicked: "nothing was picked",
		InternalError: "the command stopped on an internal error", FollowFollowing: "following", FollowPaused: "paused",
		FollowKeys: " · ↑/↓ scroll · ctrl-c leave", PageKeys: "%3.0f%% · ↑/↓ scroll · q leave",
		HelpFlagHelp: "help for %s", HelpCommandArgs: "[command]", HelpAnyShort: "Help about any command", VersionLine: "version %s",
	}
}
```

At a terminal the table is drawn and painted. Piped, it is plain columns:

```text
  two services
name    state
web     running
worker  stopped

  done       listed 2 services
```

`ui.Main` makes the session on the process's terminal, runs the command tree and returns the code of the ending. `ui` has no words and no exit codes of its own: the program gives both, so the same library speaks in any language and any house style.

## What's inside

- [`ui`](ui/README.md): everything a program prints: lines with a role, the banner, tables, charts, menus, forms, a pager, a live view, a wait, a secret read without echo, and the closing words.
- [`shell`](shell/README.md): quotes an argument list as one line a person can paste into POSIX, PowerShell or cmd, and reads it back as the same list.
- [`measure`](measure/README.md): pure text and colour measures with no I/O: a string padded to display cells, the WCAG 2.1 luminance of a colour and the contrast ratio of two.
- [`uitest`](uitest/README.md): drives a screen with keys in a test, at a fixed size, and reads the last frame back.
- [`uitest/golden`](uitest/golden/README.md): compares a test's bytes with a file under `testdata`, exactly, and a folder of files with a folder.

Each package's README and its package comment say what it is built on, what it adds that was missing, and what the calling program configures.

A program that wants only exit codes and run records can take [go-run](https://github.com/birdtable/go-run) and never download Charm. [docs/using.md](docs/using.md) has the rules a program author builds on: endings, sessions, printing, streams and the result under `--json`.

## Built on

Every direct requirement in [`go.mod`](go.mod), and what it is for:

- [bubbles](https://charm.land/bubbles), [bubbletea](https://charm.land/bubbletea), [huh](https://charm.land/huh) and [lipgloss](https://charm.land/lipgloss): the live view, the pager, the forms and menus, and the styles, colours and widths.
- [fang](https://charm.land/fang) and [cobra](https://github.com/spf13/cobra) with [pflag](https://github.com/spf13/pflag): the command tree, its help, version and error page, and its flags.
- [log](https://charm.land/log): the logger a session gives a program, returned as a standard library `*slog.Logger`.
- [colorprofile](https://github.com/charmbracelet/colorprofile) and [x/term](https://github.com/charmbracelet/x/tree/main/term): what colours a terminal takes, and its size.
- [x/ansi](https://github.com/charmbracelet/x/tree/main/ansi): the width of a string in display cells and the stripping of control sequences.
- [x/vt](https://github.com/charmbracelet/x/tree/main/vt) and [x/xpty](https://github.com/charmbracelet/x/tree/main/xpty): the screen and the pseudo-terminal that `uitest` drives a program with.
- [go-humanize](https://github.com/dustin/go-humanize): a count's commas.
- [x/sys](https://pkg.go.dev/golang.org/x/sys): the few system calls the look makes on macOS and Windows.
- [go-run](https://github.com/birdtable/go-run): the exit codes and closing words a program gives `ui`.
- [go-testkit](https://github.com/birdtable/go-testkit) and its `taskfile` package: test only; the checks of the Taskfile and of the API surface.

The `tool` lines of `go.mod` are for development only: `tools/gate`, this repository's own check runner, and go-testkit's `apigate`, which holds the API surface. Neither is in a program that imports go-console.

## Systems

go-console builds and is tested on Linux, macOS and Windows. These files are the only ones limited to some systems, and a test fails when a file is limited and not listed here:

| file | builds on |
|---|---|
| `uitest/ptyout_windows.go` | Windows |
| `uitest/ptyout_other.go` | every system but Windows |
| `ui/detect_windows.go` | Windows |
| `ui/detect_other.go` | every system but Windows |
| `ui/pagersplit_windows.go` | Windows |
| `ui/pagersplit_other.go` | every system but Windows |
| `ui/here_windows.go` | Windows |
| `ui/here_other.go` | every system but Windows |
| `ui/chain_linux.go` | Linux |
| `ui/chain_darwin.go` | macOS |
| `ui/chain_none.go` | every system but Linux, macOS and Windows |
| `internal/bestfit/scan_windows.go` | Windows |
| `internal/bestfit/scan_other.go` | every system but Windows |
| `ui/detect_windows_test.go` | Windows |
| `ui/detect_other_test.go` | every system but Windows |
| `ui/pagersplit_windows_test.go` | Windows |
| `ui/here_windows_test.go` | Windows |
| `ui/here_other_test.go` | every system but Windows |
| `ui/here_linux_test.go` | Linux |
| `ui/testdata/runarg/group_unix.go` | every system but Windows |
| `ui/chain_linux_test.go` | Linux |
| `ui/chain_darwin_test.go` | macOS |
| `ui/mainsignal_unix_test.go` | Unix |
| `ui/mainsignal_windows_test.go` | Windows |
| `ui/quoter_sh_test.go` | every system but Windows |
| `shell/powershell_windows_test.go` | Windows |
| `shell/cmd_windows_test.go` | Windows |
| `shell/bestfit_windows_test.go` | Windows |
| `shell/windows_windows_test.go` | Windows |
| `uitest/pty_windows_test.go` | Windows |
| `ui/testdata/ptyprog/utf8_windows.go` | Windows |

The code files read the terminal: whether it draws UTF-8, how `$PAGER` is split into words, which shell started the program, and the table Windows uses to fit a character into a code page. The test files send signals, ask for `/bin/sh`, or check what only Windows does. Programs run at a terminal are tested on a Unix pty and on a Windows ConPTY. The look has not yet been checked at a native Windows console, so this page does not promise it.

## Stability

v0: the API may change between minor versions.

[`api/contract.txt`](api/contract.txt) lists the promises a program may rely on, one line each, with the test that holds it. A test fails when a line names a test that is missing, and when a line is taken back without an entry in `api/breaks`. A name with no line there may still change before v1. [CHANGELOG.md](CHANGELOG.md) says what each release changed.

## Reporting issues

A bug or a feature request is an issue: [open one](https://github.com/birdtable/go-console/issues/new/choose) and fill in the form. There is no promised time for an answer. A security problem is never an issue: it goes through the private reporting in [SECURITY.md](SECURITY.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md). Please report a problem privately, not in a public issue.

## License

Apache-2.0. See [LICENSE](LICENSE).
