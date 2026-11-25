// Command gate is what Taskfile.yml's shell was: the tasks that were scripts in the
// file are one Go call each, so that go-task's interpreter, which runs shell differently on Windows, reads none
// of them, and a fix is made and proved once. Run it from the module's root:
//
//	go run ./tools/gate fmt                  gofmt finds nothing to change
//	go run ./tools/gate size <ceiling>       the non-test Go lines of the module, red above the ceiling
//	go run ./tools/gate hooks                the pre-push hook, copied into this clone's hooks folder
//	go run ./tools/gate help-pty             the pty help bound and the exits-at-once stress, each held to a PASS line
//	go run ./tools/gate marks                no name of api/marks.txt has a doc that says Experimental
//	go run ./tools/gate contract             api/contract.txt against the last tag's, by internal/contract/removed
//	go tool gate tier <tier> <member>...   a tier: the time and the load taken, the members run side by side, the budget judged
//
// A tier's member is a name, or name:arg. fmt, size:<ceiling>, marks, contract and help-pty are run here, in this
// process; any other name is a task of Taskfile.yml, run by go-task with its arg, if any, as VAR=value. Run as go tool gate,
// the program is built once and kept in the build cache, which a go run of it, linked again on each call, was not.
//
// The exit code is 0 where the check holds, 1 where it does not or could not be made, and 2 for arguments it
// does not take. Each says on its stderr, or its stdout where the shell it replaces did, what is wrong.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// run is the command: args are its arguments.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usage(stderr)
	}
	rest := args[1:]
	switch {
	case args[0] == "fmt" && len(rest) == 0:
		return gofmt(ctx, stdout, stderr)
	case args[0] == "size" && len(rest) == 1:
		return size(ctx, rest[0], stdout, stderr)
	case args[0] == "hooks" && len(rest) == 0:
		return hooks(ctx, stderr)
	case args[0] == "help-pty" && len(rest) == 0:
		return helpPty(ctx, stdout, stderr)
	case args[0] == "marks" && len(rest) == 0:
		return marks(ctx, stdout, stderr)
	case args[0] == "contract" && len(rest) == 0:
		return contract(ctx, stdout, stderr)
	case args[0] == "tier" && len(rest) >= 2:
		return tier(ctx, rest[0], rest[1:], stdout, stderr)
	case args[0] == "members" && len(rest) >= 1:
		return runMembers(ctx, rest, stdout, stderr)
	}
	return usage(stderr)
}

// usage is the refusal of arguments the command does not take.
func usage(stderr io.Writer) int {
	say(stderr, stderr, "usage: gate fmt | size <ceiling> | hooks | help-pty | marks | contract | tier <tier> <member>... | members <member>...")
	return 2
}

// say writes one line to w, and is 1 where the write fails, as a gate that cannot say its answer has not given it.
func say(w, stderr io.Writer, line string) int {
	if _, err := fmt.Fprintln(w, line); err != nil {
		fmt.Fprintln(stderr, "gate: the write failed:", err) //nolint:errcheck // stderr is where a failed write is said; there is nowhere else to say this one
		return 1
	}
	return 0
}

// fail says what is wrong, on stderr, and answers 1: a gate that says it refuses must exit red, which say does not,
// since it answers 0 when its line was written.
func fail(stderr io.Writer, line string) int {
	say(stderr, stderr, line)
	return 1
}

// start runs name with args, its output to stdout and stderr, and gives its exit code: 0 for success, the
// command's own for a failure, and 1 for one that could not start.
func start(ctx context.Context, stdout, stderr io.Writer, env []string, name string, args ...string) int {
	c := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: go, git and gofmt, with this file's own arguments
	c.Stdout, c.Stderr = stdout, stderr
	c.Env = append(os.Environ(), env...)
	err := c.Run()
	if err == nil {
		return 0
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() > 0 { //nolint:errorlint // the process's own failure, not a wrapped one
		return exit.ExitCode()
	}
	say(stderr, stderr, fmt.Sprintf("gate: %s could not run: %v", name, err))
	return 1
}

// output runs name with args and gives its standard output, and its error where it failed.
func output(ctx context.Context, name string, args ...string) (string, error) {
	var out bytes.Buffer
	c := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: git and go, with this file's own arguments
	c.Stdout = &out
	err := c.Run()
	return out.String(), err
}

// gofmt is task fmt: gofmt -l . lists nothing.
func gofmt(ctx context.Context, stdout, stderr io.Writer) int {
	out, err := output(ctx, "gofmt", "-l", ".")
	if err != nil {
		return fail(stderr, fmt.Sprintf("gofmt could not run: %v", err))
	}
	if strings.TrimSpace(out) == "" {
		return 0
	}
	say(stdout, stderr, "gofmt would change:")
	say(stdout, stderr, strings.TrimSuffix(out, "\n"))
	return 1
}

// sizeFiles is the pathspec of the lines task size counts: every Go file git knows or would add that is not a
// test, outside tools/, which holds tool pins and the programs of the gates, ui/testdata, which holds programs the
// tests build and run, and uitest/golden, which is counted apart.
func sizeFiles() []string {
	return []string{"*.go", ":(exclude,glob)**/*_test.go", ":(exclude)tools", ":(exclude)ui/testdata", ":(exclude)uitest/golden"}
}

// size is task size: the count of lines `wc -l` gives the files, one per newline, and red above ceiling.
func size(ctx context.Context, ceiling string, stdout, stderr io.Writer) int {
	limit, err := strconv.Atoi(ceiling)
	if err != nil || limit < 0 {
		return usage(stderr)
	}
	list, err := output(ctx, "git", append([]string{"ls-files", "-z", "--cached", "--others", "--exclude-standard", "--"}, sizeFiles()...)...)
	if err != nil {
		return fail(stderr, fmt.Sprintf("size: git cannot list the files: %v", err))
	}
	n := 0
	for name := range strings.SplitSeq(list, "\x00") {
		if name == "" {
			continue
		}
		b, err := os.ReadFile(filepath.FromSlash(name))
		if err != nil {
			return fail(stderr, fmt.Sprintf("size: %v", err))
		}
		n += bytes.Count(b, []byte("\n"))
	}
	if code := say(stdout, stderr, strconv.Itoa(n)); code != 0 {
		return code
	}
	if n > limit {
		return fail(stderr, fmt.Sprintf("size %d is over the ceiling of %d", n, limit))
	}
	return 0
}

// hooks is task hooks: hooks/pre-push is copied into this clone's hooks folder, executable.
func hooks(ctx context.Context, stderr io.Writer) int {
	dir, err := output(ctx, "git", "rev-parse", "--git-path", "hooks")
	if err != nil {
		return fail(stderr, fmt.Sprintf("hooks: git cannot name the hooks folder: %v", err))
	}
	dir = strings.TrimSpace(dir)
	body, err := os.ReadFile(filepath.Join("hooks", "pre-push"))
	if err != nil {
		return fail(stderr, fmt.Sprintf("hooks: %v", err))
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fail(stderr, fmt.Sprintf("hooks: %v", err))
	}
	to := filepath.Join(dir, "pre-push")
	if err := os.WriteFile(to, body, 0o600); err != nil {
		return fail(stderr, fmt.Sprintf("hooks: %v", err))
	}
	if err := os.Chmod(to, 0o700); err != nil { //nolint:gosec // G302: a hook is run by git, so the owner executes it
		return fail(stderr, fmt.Sprintf("hooks: %v", err))
	}
	return 0
}

// helpTests are the tests task help-pty runs, each held to a PASS line: go test says "ok" with "no tests to run"
// where a name matches nothing. Every host runs both, Windows too.
func helpTests() []string {
	return []string{"TestHelpFitsAPty", "TestAChildThatExitsAtOnceIsReadWholeFifty"}
}

// helpPty is task help-pty.
func helpPty(ctx context.Context, stdout, stderr io.Writer) int {
	pattern := "^(" + strings.Join(helpTests(), "|") + ")$"
	var out bytes.Buffer
	c := exec.CommandContext(ctx, "go", "test", "-race", "-tags", "release", "-run", pattern, "-v", "./ui") //nolint:gosec // G204: go, with this file's own arguments
	c.Stdout, c.Stderr = &out, &out
	c.Env = append(os.Environ(), "CGO_ENABLED=1")
	err := c.Run()
	if _, werr := stdout.Write(out.Bytes()); werr != nil {
		return fail(stderr, fmt.Sprintf("help-pty: the write failed: %v", werr))
	}
	if err != nil {
		return 1
	}
	lost := missing(out.String(), helpTests())
	for _, name := range lost {
		say(stderr, stderr, "help-pty: "+name+" did not run and pass")
	}
	if len(lost) > 0 {
		return 1
	}
	return 0
}

// missing is each of names that out holds no `--- PASS: <name> ` line for.
func missing(out string, names []string) []string {
	var lost []string
	for _, name := range names {
		if !strings.Contains("\n"+out, "\n--- PASS: "+name+" ") {
			lost = append(lost, name)
		}
	}
	return lost
}

// marked is the names api/marks.txt lists, each as its package directory and its name: a line is
// <package><tab><name><tab><mark>, a # line and a blank line are no row, and a line of another shape is an error.
func marked(file string) ([][2]string, error) {
	var rows [][2]string
	for n, line := range strings.Split(file, "\n") {
		if line == "" || strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			return nil, fmt.Errorf("api/marks.txt line %d is not <package><tab><name><tab><mark>", n+1)
		}
		rows = append(rows, [2]string{f[0], f[1]})
	}
	return rows, nil
}

// experimental is whether a doc comment, as go doc prints it, says the name is Experimental.
func experimental(doc string) bool {
	return strings.Contains(doc, "Experimental")
}

// marks is the refusal apigate does not hold: a name api/marks.txt lists as not
// experimental, whose doc says Experimental. That the name exists is apigate's (go doc must find it). It goes when
// apigate holds this, which is a finding sent to go-testkit.
func marks(ctx context.Context, stdout, stderr io.Writer) int {
	b, err := os.ReadFile(filepath.Join("api", "marks.txt"))
	if err != nil {
		return fail(stderr, fmt.Sprintf("marks: %v", err))
	}
	rows, err := marked(string(b))
	if err != nil {
		return fail(stderr, "marks: "+err.Error())
	}
	code := 0
	for _, r := range rows {
		doc, err := output(ctx, "go", "doc", "./"+r[0], r[1])
		if err != nil {
			continue // go doc does not find it: apigate says so
		}
		if experimental(doc) {
			say(stderr, stderr, "api: api/marks.txt names "+r[0]+" "+r[1]+" as not experimental, and its doc says Experimental")
			code = 1
		}
	}
	return code
}

// contract is the second half of task api: a line api/contract.txt had at the last tag and has no longer is listed
// in api/breaks/<tag>.contract.txt, a file beside the apidiff one, which can hold no other line. The comparison is
// internal/contract/removed, whose plants are the rows of its test. A tree with no tag, or a tag with no
// api/contract.txt, has nothing to be taken back.
func contract(ctx context.Context, stdout, stderr io.Writer) int {
	base, err := output(ctx, "git", "describe", "--tags", "--abbrev=0", "--match", "v*")
	base = strings.TrimSpace(base)
	if err != nil || base == "" {
		return 0
	}
	old, err := output(ctx, "git", "show", base+":api/contract.txt")
	if err != nil {
		return 0
	}
	declared := ""
	if b, err := os.ReadFile(filepath.Join("api", "breaks", base+".contract.txt")); err == nil { //nolint:gosec // G304: the break file named by the last tag
		declared = string(b)
	}
	dir, err := os.MkdirTemp("", "contract-")
	if err != nil {
		return fail(stderr, fmt.Sprintf("api: %v", err))
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			say(stderr, stderr, fmt.Sprintf("api: %v", err))
		}
	}()
	files := []string{filepath.Join(dir, "old"), filepath.Join("api", "contract.txt"), filepath.Join(dir, "declared")}
	if err := os.WriteFile(files[0], []byte(old), 0o600); err != nil {
		return fail(stderr, fmt.Sprintf("api: %v", err))
	}
	if err := os.WriteFile(files[2], []byte(declared), 0o600); err != nil {
		return fail(stderr, fmt.Sprintf("api: %v", err))
	}
	return start(ctx, stdout, stderr, nil, "go", append([]string{"run", "./internal/contract/removed", base}, files...)...)
}

// loadBefore is the one-minute load hatch reads, as hatch writes it; "" where hatch cannot read it.
func loadBefore(ctx context.Context) string {
	out, err := output(ctx, "go", "tool", "-modfile=tools/hatch.mod", "hatch", "budget", "--load", "--window", "1")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// tier is a tier of Taskfile.yml in one process: the time it counts from and, for the tiers a busy host excuses, the
// load are taken before the members start; the members run side by side, as go-task's deps did, each one's output
// held and written when it ends; and a tier whose members all passed is held to its budget. A member that fails
// fails the tier, and no budget is judged for it.
func tier(ctx context.Context, name string, members []string, stdout, stderr io.Writer) int {
	since := strconv.FormatInt(time.Now().Unix(), 10)
	before := ""
	if name == "pre-push" || name == "pre-commit" {
		before = loadBefore(ctx)
	}
	if name == "pre-push" {
		return cpuTier(ctx, members, since, before, stdout, stderr)
	}
	if code := runMembers(ctx, members, stdout, stderr); code != 0 {
		return fail(stderr, "gate: "+name+": a member failed")
	}
	return budget(ctx, name, since, before, stdout, stderr)
}

// cpuTier is the pre-push tier judged by its CPU time: hatch runs this program's members in a tree
// of its own and holds the user plus system time of that tree to the pre-push-cpu row of budgets.yaml, which is the
// same on a quiet host and a loaded one. The pre-push-budget row, the wall row, is hatch's hang guard only: a tier
// still running at ten times it is killed. Where no row applies to this host the members run and no budget is judged.
func cpuTier(ctx context.Context, members []string, since, loadBefore string, stdout, stderr io.Writer) int {
	const name = "pre-push"
	b, err := os.ReadFile("budgets.yaml")
	if err != nil {
		return fail(stderr, fmt.Sprintf("budget: %v", err))
	}
	hosted := os.Getenv("GITHUB_ACTIONS") != ""
	if row(name, key(string(b), "host"), runtime.GOOS, runtime.GOARCH, hosted) == "" {
		if code := runMembers(ctx, members, stdout, stderr); code != 0 {
			return fail(stderr, "gate: "+name+": a member failed")
		}
		return say(stdout, stderr, fmt.Sprintf("%s: no budget for this kind of host (%s), so none applies", name, hostKind(runtime.GOOS, runtime.GOARCH)))
	}
	self, err := os.Executable()
	if err != nil {
		return fail(stderr, fmt.Sprintf("gate: %v", err))
	}
	args := []string{
		"tool", "-modfile=tools/hatch.mod", "hatch", "budget", "--cpu",
		"--budgets", "budgets.yaml", "--tier", name, "--since", since, "--window", "1", "--noun", name,
		"--row", `(?m)^(?P<id>pre-push)-budget: *(?P<limit>[0-9]+) *$`,
		"--cpu-row", `(?m)^(?P<id>pre-push)-cpu: *(?P<limit>[0-9]+) *$`,
	}
	if loadBefore != "" {
		args = append(args, "--load-before", loadBefore)
	}
	args = append(args, "--", self, "members")
	return start(ctx, stdout, stderr, nil, "go", append(args, members...)...)
}

// runMembers runs a tier's members side by side, as go-task's deps did, each one's output held and written when it
// ends, and gives 0 where every member passed.
func runMembers(ctx context.Context, members []string, stdout, stderr io.Writer) int {
	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		failed bool
	)
	jobs, tasks := []func(io.Writer, io.Writer) int{}, []string{}
	for _, m := range members {
		if n, _, _ := strings.Cut(m, ":"); !mine(n) {
			tasks = append(tasks, m)
			continue
		}
		jobs = append(jobs, func(out, errOut io.Writer) int { return member(ctx, m, out, errOut) })
	}
	if len(tasks) > 0 {
		jobs = append(jobs, func(out, errOut io.Writer) int { return runTasks(ctx, tasks, out, errOut) })
	}
	for _, job := range jobs {
		wg.Go(func() {
			var out, errOut bytes.Buffer
			code := job(&out, &errOut)
			mu.Lock()
			defer mu.Unlock()
			if _, err := stdout.Write(out.Bytes()); err != nil {
				code = 1
			}
			if _, err := stderr.Write(errOut.Bytes()); err != nil {
				code = 1
			}
			if code != 0 {
				failed = true
			}
		})
	}
	wg.Wait()
	if failed {
		return 1
	}
	return 0
}

// mine is whether this program runs the member itself.
func mine(name string) bool {
	switch name {
	case "fmt", "size", "marks", "contract", "help-pty":
		return true
	}
	return false
}

// member runs one member this program runs itself.
func member(ctx context.Context, token string, stdout, stderr io.Writer) int {
	name, arg, _ := strings.Cut(token, ":")
	switch name {
	case "fmt":
		return gofmt(ctx, stdout, stderr)
	case "size":
		return size(ctx, arg, stdout, stderr)
	case "marks":
		return marks(ctx, stdout, stderr)
	case "contract":
		return contract(ctx, stdout, stderr)
	}
	return helpPty(ctx, stdout, stderr)
}

// runTasks runs the members that are tasks of Taskfile.yml in one go-task, side by side, one start for all of them. A
// member's arg, VAR=value, is a variable of that go-task run, which the tasks that do not read it ignore.
func runTasks(ctx context.Context, tokens []string, stdout, stderr io.Writer) int {
	args := []string{"tool", "-modfile=tools/task.mod", "task", "--parallel"}
	var vars []string
	for _, t := range tokens {
		name, arg, _ := strings.Cut(t, ":")
		args = append(args, name)
		vars = append(vars, strings.Fields(arg)...)
	}
	return start(ctx, stdout, stderr, nil, "go", append(args, vars...)...)
}

// hostKind is the kind of host as `uname -sm` named it, its words joined with a dash: the kind budgets.yaml's host
// row is written in.
func hostKind(goos, goarch string) string {
	sys := map[string]string{"darwin": "Darwin", "linux": "Linux", "windows": "Windows"}[goos]
	if sys == "" {
		sys = goos
	}
	arch := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[goarch]
	if arch == "" {
		arch = goarch
	}
	return sys + "-" + arch
}

// key is the value budgets.yaml gives key, as awk read it: the line's text after the colon and its spaces.
func key(budgets, name string) string {
	for line := range strings.SplitSeq(budgets, "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && k == name {
			return strings.TrimLeft(v, " ")
		}
	}
	return ""
}

// row is the row of budgets.yaml a tier is held to on this kind of host, or "" where none applies. The host named
// there is held to its tier row. A release session, Linux or Windows on x86-64 that is not a hosted runner, is held
// to the session row; any other kind of host, and a hosted runner, has none.
func row(tier, host, goos, goarch string, hosted bool) string {
	switch {
	case hostKind(goos, goarch) == host:
		return tier + "-budget"
	case (goos == "linux" || goos == "windows") && goarch == "amd64" && !hosted && tier == "release":
		return tier + "-budget-session"
	}
	return ""
}

// budget is task budget: hatch holds the tier's elapsed seconds, from since, to its row of budgets.yaml. Which row
// applies to this host is go-console's own: hatch holds no project policy, so this names the row. A pre-commit or
// pre-push tier past its budget is noted, not failed, when the one-minute load read before it was above 15.
func budget(ctx context.Context, tier, since, loadBefore string, stdout, stderr io.Writer) int {
	b, err := os.ReadFile("budgets.yaml")
	if err != nil {
		return fail(stderr, fmt.Sprintf("budget: %v", err))
	}
	hosted := os.Getenv("GITHUB_ACTIONS") != ""
	r := row(tier, key(string(b), "host"), runtime.GOOS, runtime.GOARCH, hosted)
	if r == "" {
		return say(stdout, stderr, fmt.Sprintf("%s: no budget for this kind of host (%s), so none applies", tier, hostKind(runtime.GOOS, runtime.GOARCH)))
	}
	args := []string{
		"tool", "-modfile=tools/hatch.mod", "hatch", "budget",
		"--budgets", "budgets.yaml", "--row", `(?m)^(?P<id>[a-z-]+): *(?P<limit>[0-9]+) *$`,
		"--tier", r, "--since", since, "--window", "1", "--noun", tier, "--excused", "noted, not failed",
	}
	if tier == "pre-push" || tier == "pre-commit" {
		args = append(args, "--limit", "15", "--loaded-when", "before")
		if loadBefore != "" {
			args = append(args, "--load-before", loadBefore)
		}
	}
	return start(ctx, stdout, stderr, nil, "go", args...)
}
