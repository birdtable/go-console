# Changelog

What each release of go-console changed.

## v0.18.0 (2026-11-25)

First public release.

Tested on Linux (amd64), macOS (arm64) and Windows (amd64).

- [`ui`](https://github.com/birdtable/go-console/blob/main/ui/README.md): everything a program prints: lines with a role, the banner, tables, charts, menus, forms, a pager, a live view, a wait, a secret read without echo, and the closing words.
- [`shell`](https://github.com/birdtable/go-console/blob/main/shell/README.md): quotes an argument list as one line a person can paste into POSIX, PowerShell or cmd, and reads it back as the same list.
- [`measure`](https://github.com/birdtable/go-console/blob/main/measure/README.md): pure text and colour measures with no I/O.
- [`uitest`](https://github.com/birdtable/go-console/blob/main/uitest/README.md): drives a screen with keys in a test, at a fixed size, and reads the last frame back.
- [`uitest/golden`](https://github.com/birdtable/go-console/blob/main/uitest/golden/README.md): compares a test's bytes with a file under `testdata`, exactly, and a folder of files with a folder.

### Added
- **measure:** a new leaf package of pure text and colour measures: `Column`, `RelativeLuminance` and `ContrastRatio`. It takes only the standard library and `charmbracelet/x/ansi`, does no I/O and has no `init`, so a layer that must stay pure can measure text without taking `ui`. The three moved here from `ui`, and `ui.Column`, `ui.RelativeLuminance` and `ui.ContrastRatio` keep their names and call `measure`, so no caller changes. Experimental.
- **shell:** `Split(d, line)` and `ErrNotQuoted`, the inverse of `Quote` for POSIX, PowerShell and Windows. It reads exactly what `Quote` writes and expands nothing: an unclosed quote, an operator, a `$`, a glob or any other form `Quote` never writes returns `ErrNotQuoted`, naming the byte offset. `Split(d, Quote(d, a))` gives back `a`, held by a fuzz test. Cmd is not read (`ErrDialect`). Experimental.
- **shell:** `Invisible(r)` and `Shown(s)`. `Invisible` is the rune test `Quote` uses to refuse hidden characters, and `Shown` is false exactly when `Quote` would refuse a text for them. A caller that names a value in prose no longer needs its own table. Experimental.
- **ui:** `MainFunc(name, run, b, opts...)`, the whole of `main` for a program that is one function with no command tree. `run` gets the arguments unparsed, the context `Main` gives (Ctrl+C and SIGTERM handled as in `Main`), and returns the exit code. Experimental.
### Changed
- **ui:** `ui.Name` and a followed line hide a rune exactly when `shell.Quote` would refuse it. A name holding an unassigned rune, a space other than U+0020, or a rune that draws as blank is now quoted as its number, where before it printed as it stood. The emoji joiner and selectors stay the one exception.
- The README is rewritten, with an install line, a quick start and a stability note.
