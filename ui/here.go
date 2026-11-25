package ui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/birdtable/go-console/shell"
)

// ErrNoDialect is what HereDialect wraps when the shell the program was started from is one the kit knows and
// shell.Quote writes no dialect for: fish, csh, tcsh, xonsh, nu, elvish, or wsl, which can run any of them.
// Quoting for POSIX there is not safe, so a caller prints no line to paste, or takes the dialect a person names.
// It is also wrapped for a Windows chain that cannot be trusted, and that error names no image.
var ErrNoDialect = errors.New("no dialect")

// HereDialect is the shell dialect of the shell this program was started from, for quoting a line the person
// is to paste there (shell.Quote). It reads the system, which package shell does not.
//
// It reads the parent process chain, nearest first, at most 8 levels up: through a toolhelp snapshot on
// Windows, from /proc on Linux, through the kern.proc.pid sysctl on macOS, and not at all on any other system,
// which has an empty chain. Off Windows, a read that fails, of a name or of a start time, ends the chain and is never an error. A pid can be reused by an unrelated process, so each system has a rule: on Linux a parent that started later than its child (field 22 of stat, which counts from boot) ends the chain, and equal times pass; on macOS no time is compared, and the chain ends when a child's parent changed between the read of the child and the read of the parent; on Windows, where the creation time is the only tool, a parent younger than its child, a reused pid or a clock set back, or a parent the snapshot names and OpenProcess refuses, makes the answer ErrNoDialect when no known shell was found nearer, never the default, and a shell found nearer decides as ever. On macOS an attached debugger is the parent of the program it attached to, so the walk reads the debugger's tree. The nearest ancestor whose
// name is a shell the kit knows decides, and the walk ends there. A name matches with or without .exe, in any
// case, and the parent chain is the source, not $SHELL, which is the login shell and not the one a person types
// in. Off Windows no known shell in the chain answers shell.POSIX and nil, and cmd, powershell, wsl and wslhost
// are not shells there and are passed over: a Go tool built as cmd is no Windows shell. On Windows cmd.exe answers shell.Cmd;
// powershell.exe answers shell.PowerShell, as pwsh, pwsh-preview and pwsh-lts do on every system, as whole names; bash.exe, sh.exe, zsh.exe, dash.exe, ash.exe, ksh.exe,
// ksh93.exe, mksh.exe and any busybox*.exe answer shell.POSIX, whose quoting is theirs. fish.exe, csh.exe,
// tcsh.exe, xonsh.exe, nu.exe, elvish.exe, wsl.exe and wslhost.exe answer 0 and an error wrapping ErrNoDialect
// that names the image: POSIX single quotes are not literal in the first six, and the image of the last two
// cannot say which shell runs inside. With no such ancestor, or when the snapshot fails, it answers
// shell.PowerShell on Windows, the default shell of Windows Terminal, and nil. It does not read PSModulePath, which is set
// system-wide even under cmd, nor ComSpec, which names cmd on every system. pwsh-preview and pwsh-lts are there for Linux: its preview and LTS packages start PowerShell through a link of that name, and Linux names the process for the link. On macOS the process is named for the link's target and shows as pwsh.
//
// The answer is a guess, and a person corrects it with a dialect setting of the program. A shell the kit does
// not name, in front of a cmd, still answers Cmd. The nearest known shell wins, so a sh.exe that git or make
// started, not the person, answers POSIX: no image tells a recipe's sh from a person's.
//
// Shells are matched by the name of the process, and a wrapper script that runs the program without exec is named
// for its interpreter, sh or bash (on Linux one started by its #! line is named for the script file, which no rule
// knows). So off Windows an ancestor in the program's own process group whose name answers POSIX (bash, sh, zsh,
// dash, ash, ksh, ksh93, mksh, busybox*) is passed over, and the answer is what lies behind it: a wrapper started
// from an interactive shell shares the program's group, and that shell does not. A name that answers PowerShell or
// ErrNoDialect decides wherever it sits. The group is field 5 of stat on Linux and Eproc.Pgid on macOS, read with
// the name, and one that cannot be read is never the program's own; Windows has no process group and passes nothing
// over. The rule has two limits. A person's own POSIX shell that runs with job control off (set +m, or no terminal), or that runs the program in a command substitution ($(...)) at its prompt,
// shares the program's group and is passed over, so the answer is what lies behind it (ErrNoDialect for fish: it fails closed). And a wrapper that is not in
// the program's group, because something between them made a new group (timeout, setsid, sudo with a pty), still
// answers POSIX ahead of the shell behind it. A shell that an interpreter runs, xonsh being Python, shows
// as the interpreter and is not recognised, so xonsh does not always refuse. Off Windows, a read that fails, such as /proc
// mounted with hidepid or no /proc at all, or a process whose start time cannot be read, ends the chain, and an ended chain answers the system's default,
// POSIX off Windows: the answer cannot tell "no shell found" from "could not read".
//
// Experimental.
func HereDialect() (shell.Dialect, error) { return hereDialect() }

// dialectOfChain is dialectOfChainOr with shell.PowerShell for a chain that names no shell the kit knows, the
// default of Windows.
func dialectOfChain(images []string) (shell.Dialect, error) {
	return dialectOfChainOr(images, shell.PowerShell, true)
}

// dialectOfWalk is dialectOfChain for the chain a walk read and whether it ended at a hop that cannot be trusted: with
// no known shell in the chain, an untrusted end is ErrNoDialect and not the default, so no line is printed that a
// wrong shell could run. A known shell found nearer than that hop decides as ever.
func dialectOfWalk(chain []string, suspect bool) (shell.Dialect, error) {
	d, err := dialectOfChainOr(chain, 0, true)
	if d != 0 || err != nil {
		return d, err
	}
	if suspect {
		return 0, fmt.Errorf("here: the parent chain cannot be trusted, a parent is younger than its child or cannot be read: %w", ErrNoDialect)
	}
	return shell.PowerShell, nil
}

// dialectOfGroups is the choice of the systems but Windows, for a chain and whether each ancestor is in the
// program's own process group (walkGroups): an own-group ancestor whose name alone answers POSIX is passed over, as
// a wrapper script's interpreter is, and dialectOfChainOr chooses among the rest, with POSIX for none. A name that
// answers PowerShell or ErrNoDialect decides wherever it sits, so passing over can only turn a POSIX answer into
// what lies behind it. A passed-over ancestor was one of the 8 the walk read.
func dialectOfGroups(chain []string, own []bool) (shell.Dialect, error) {
	kept := make([]string, 0, len(chain))
	for i, name := range chain {
		if d, err := dialectOfChainOr(chain[i:i+1], 0, false); i < len(own) && own[i] && d == shell.POSIX && err == nil {
			continue
		}
		kept = append(kept, name)
	}
	return dialectOfChainOr(kept, shell.POSIX, false)
}

// dialectOfChainOr is the dialect the ancestors' names say, nearest first, in any case, with or without .exe,
// so fish and fish.exe are one name: the first of the 8 nearest that is a shell the kit knows decides (see
// HereDialect). With none, or an empty chain, it is def and nil. Off Windows (windows false) cmd, powershell, wsl
// and wslhost are not shells and are passed over; pwsh, pwsh-preview and pwsh-lts run on every system and stay.
func dialectOfChainOr(images []string, def shell.Dialect, windows bool) (shell.Dialect, error) {
	for _, image := range images[:min(len(images), 8)] {
		name := strings.TrimSuffix(strings.ToLower(image), ".exe")
		switch name {
		case "cmd", "powershell", "wsl", "wslhost":
			if !windows {
				continue
			}
			switch name {
			case "cmd":
				return shell.Cmd, nil
			case "powershell":
				return shell.PowerShell, nil
			}
			return 0, fmt.Errorf("here: %s has no dialect shell.Quote writes: %w", image, ErrNoDialect)
		case "pwsh", "pwsh-preview", "pwsh-lts":
			return shell.PowerShell, nil
		case "bash", "sh", "zsh", "dash", "ash", "ksh", "ksh93", "mksh":
			return shell.POSIX, nil
		case "fish", "csh", "tcsh", "xonsh", "nu", "elvish":
			return 0, fmt.Errorf("here: %s has no dialect shell.Quote writes: %w", image, ErrNoDialect)
		}
		if strings.HasPrefix(name, "busybox") {
			return shell.POSIX, nil
		}
	}
	return def, nil
}
