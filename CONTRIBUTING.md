# Contributing

Thank you for helping. Small, focused changes are the easiest to review.

## Run the checks

The checks run with [Task](https://taskfile.dev), pinned in the repository, so nothing is installed on your machine:

```sh
go tool -modfile=tools/task.mod task pre-commit
go tool -modfile=tools/task.mod task pre-push
```

`pre-commit` is quick: format, build, vet, a scan for secrets and the Taskfile check. `pre-push` adds lint, the tests, the module check, the API and contract gates and the size gate. Run the `hooks` task once per clone, and `pre-push` runs before every push. The `release` task runs everything, the pseudo-terminal tests included.

## Commits and pull requests

- One change to a commit, with a message that says what changed and why. Do not amend a commit after a review began; add a new one.
- A pull request says what it changes, and closes the issue it answers. Open an issue first for anything large.
- A change to behaviour comes with a test that fails without it. A change to an exported name comes with a line in `api/contract.txt`, and an entry in `api/breaks/` when it takes a promise back.
- Add a file to `.changes/unreleased` for anything a user of the library would notice: `go tool -modfile=tools/changie.mod changie new`.
- The code is written for Linux, macOS and Windows alike. A file limited to some systems is listed in the README's Systems table.
