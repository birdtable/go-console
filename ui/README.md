# ui

Everything a program prints: the terminal look every command-line program shares.

    go get github.com/birdtable/go-console

- **Built on:** lipgloss, bubbletea and bubbles, huh, fang with cobra, and go-run's `exit`.
- **Adds:** a line with its role that is plain in a pipe, the banner, tables, `Pick`, `Strip` and `Main`: the one look.
- **The program configures:** `Brand`, `Theme`, `EnvRules`, `Endings`, `WithMessages` and `FlagNames`.

The API is on [pkg.go.dev](https://pkg.go.dev/github.com/birdtable/go-console/ui). The [root README](../README.md) says what the module is for, and v0 means the API may change between minor versions.
