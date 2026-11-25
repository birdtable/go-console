# shell

Quotes an argument list as one line a person can paste into a shell, and reads it back.

    go get github.com/birdtable/go-console

- **Built on:** the standard library alone.
- **Adds:** the dialect-correct quoting the standard library leaves out, for POSIX, PowerShell, cmd and one line that cmd and PowerShell both read.
- **The program configures:** the dialect and the argument list.

The API is on [pkg.go.dev](https://pkg.go.dev/github.com/birdtable/go-console/shell). The [root README](../README.md) says what the module is for, and v0 means the API may change between minor versions.
