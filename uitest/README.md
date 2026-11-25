# uitest

Drives a screen with keys in a test, at a fixed size, and reads the last frame back.

    go get github.com/birdtable/go-console

- **Built on:** bubbletea, huh, xpty and vt.
- **Adds:** the screen and wait helpers, so a key that does the wrong thing fails a test.
- **The program configures:** the size, the keys and `mute`.

The API is on [pkg.go.dev](https://pkg.go.dev/github.com/birdtable/go-console/uitest). The [root README](../README.md) says what the module is for, and v0 means the API may change between minor versions.
