# uitest/golden

Compares a test's bytes with a file under `testdata`, exactly, and a folder with a folder.

    go get github.com/birdtable/go-console

- **Built on:** the standard library alone (`io/fs` and `os.Root`).
- **Adds:** `File`, an exact-byte compare with no trimming, and `Tree`, the same for a folder, listing every difference.
- **The program configures:** the file name and the `-update` flag that writes it.

The API is on [pkg.go.dev](https://pkg.go.dev/github.com/birdtable/go-console/uitest/golden). The [root README](../../README.md) says what the module is for, and v0 means the API may change between minor versions.
